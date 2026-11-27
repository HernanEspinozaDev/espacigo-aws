package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	dbgen "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity/dbgen"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ privacy.Repository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetProfile(ctx context.Context, accountID string) (privacy.Profile, error) {
	row, err := r.queries.GetProfile(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.Profile{}, privacy.ErrNotFound
		}
		return privacy.Profile{}, mapError(err)
	}
	return privacy.Profile{AccountID: row.AccountID, Name: row.NombreVisible, Phone: row.TelefonoNormalizado, UpdatedAt: row.ActualizadoEn.Time}, nil
}

func (r *IdentityRepository) SaveProfile(ctx context.Context, accountID, name string, phone *string) (privacy.Profile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.Profile{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return privacy.Profile{}, mapError(err)
	}
	if !active {
		return privacy.Profile{}, privacy.ErrNotFound
	}
	row, err := r.queries.WithTx(tx).UpsertProfile(ctx, dbgen.UpsertProfileParams{AccountID: accountID, DisplayName: name, Phone: phone})
	if err != nil {
		return privacy.Profile{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return privacy.Profile{}, mapError(err)
	}
	return privacy.Profile{AccountID: row.AccountID, Name: row.NombreVisible, Phone: row.TelefonoNormalizado, UpdatedAt: row.ActualizadoEn.Time}, nil
}

func (r *IdentityRepository) CreateRightsRequest(ctx context.Context, accountID, kind, channel string) (privacy.RightsRequest, error) {
	return r.CreateRightsRequestWithDetail(ctx, accountID, kind, channel, nil)
}

func (r *IdentityRepository) CreateRightsRequestWithDetail(ctx context.Context, accountID, kind, channel string, detail *string) (privacy.RightsRequest, error) {
	id, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.RightsRequest{}, err
	}
	eventID, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.RightsRequest{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	dueAt := privacy.ComputeInitialDeadline(now, kind)
	folio, err := privacy.GeneratePublicFolioAt(now)
	if err != nil {
		return privacy.RightsRequest{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.RightsRequest{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	row := tx.QueryRow(ctx, `INSERT INTO public.solicitud_titular (id, usuario_id, folio_publico, tipo, canal, detalle_titular, solicitada_en, vence_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text, tipo, estado, solicitada_en, vence_en`, id, accountID, folio, kind, channel, detail, now, dueAt)
	var created privacy.RightsRequest
	if err := row.Scan(&created.ID, &created.Kind, &created.State, &created.CreatedAt, &created.DueAt); err != nil {
		return privacy.RightsRequest{}, mapError(err)
	}
	created.CreatedAt = created.CreatedAt.UTC()
	created.Detail = detail
	if _, err := tx.Exec(ctx, `INSERT INTO public.solicitud_titular_evento (
		id, solicitud_id, secuencia, tipo_evento, visibilidad, estado_nuevo, actor_id, vence_en_nuevo, creado_en
	) VALUES ($1, $2, 1, 'recepcion', 'titular', $3, $4, $5, $6)`,
		eventID, created.ID, created.State, accountID, dueAt, created.CreatedAt); err != nil {
		return privacy.RightsRequest{}, mapError(err)
	}
	var recipientEmail pgtype.Text
	if err := tx.QueryRow(ctx, `SELECT correo_original FROM public.usuario WHERE id = $1`, accountID).Scan(&recipientEmail); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return privacy.RightsRequest{}, mapError(err)
	}
	if recipientEmail.Valid {
		email := strings.TrimSpace(recipientEmail.String)
		if n := utf8.RuneCountInString(email); n >= 5 && n <= 254 {
			subject, body := privacy.FormatAcuseRecepcionNotice(folio, created.Kind, dueAt)
			if _, err := tx.Exec(ctx, `INSERT INTO public.privacidad_notificacion_outbox (
				solicitud_id, tipo_aviso, clave_idempotencia, destinatario_correo, asunto, cuerpo_texto, disponible_en, creado_en
			) VALUES ($1, 'acuse_recepcion', $2, $3, $4, $5, $6, $6)
			ON CONFLICT (clave_idempotencia) DO NOTHING`,
				created.ID, "acuse:"+created.ID, email, subject, body, created.CreatedAt); err != nil {
				return privacy.RightsRequest{}, mapError(err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return privacy.RightsRequest{}, mapError(err)
	}
	stateCopy := created.State
	dueCopy := dueAt
	created.Events = []privacy.RightsRequestEvent{{
		ID:        eventID,
		Sequence:  1,
		EventType: "recepcion",
		StateTo:   &stateCopy,
		NewDueAt:  &dueCopy,
		CreatedAt: created.CreatedAt,
	}}
	return created, nil
}

func (r *IdentityRepository) ListOwnRightsRequests(ctx context.Context, accountID string) ([]privacy.RightsRequest, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, tipo, estado, solicitada_en, resuelta_en, respuesta_titular, detalle_titular, vence_en, prorrogada_hasta, prorroga_motivo
        FROM public.solicitud_titular WHERE usuario_id=$1 ORDER BY solicitada_en DESC, id DESC`, accountID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	items := []privacy.RightsRequest{}
	idxByID := map[string]int{}
	ids := []string{}
	for rows.Next() {
		var item privacy.RightsRequest
		if err := rows.Scan(&item.ID, &item.Kind, &item.State, &item.CreatedAt, &item.ResolvedAt, &item.Response, &item.Detail, &item.DueAt, &item.ExtendedDueAt, &item.ExtensionReason); err != nil {
			return nil, mapError(err)
		}
		idxByID[item.ID] = len(items)
		ids = append(ids, item.ID)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	if len(ids) == 0 {
		return items, nil
	}
	eventRows, err := r.pool.Query(ctx, `SELECT id::text, solicitud_id::text, secuencia, tipo_evento, estado_anterior, estado_nuevo, detalle, vence_en_nuevo, creado_en
		FROM public.solicitud_titular_evento
		WHERE solicitud_id = ANY($1::uuid[]) AND visibilidad = 'titular'
		ORDER BY solicitud_id, secuencia ASC`, ids)
	if err != nil {
		return nil, mapError(err)
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var solicitudID string
		var ev privacy.RightsRequestEvent
		if err := eventRows.Scan(&ev.ID, &solicitudID, &ev.Sequence, &ev.EventType, &ev.StateFrom, &ev.StateTo, &ev.Detail, &ev.NewDueAt, &ev.CreatedAt); err != nil {
			return nil, mapError(err)
		}
		if idx, ok := idxByID[solicitudID]; ok {
			items[idx].Events = append(items[idx].Events, ev)
		}
	}
	return items, mapError(eventRows.Err())
}

// ExportOwnData reads one consistent, owner-scoped identity snapshot. It never
// selects password hashes, sessions, action tokens, or unrelated domain rows.
func (r *IdentityRepository) ExportOwnData(ctx context.Context, accountID string) (privacy.OwnData, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var data privacy.OwnData
	data.Scope = "identidad_local_v1"
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `SELECT correo_original, estado, preferencia_uso, creado_en
		FROM public.usuario WHERE id=$1`, accountID).Scan(
		&data.Account.Email, &data.Account.State, &data.Account.UsePreference, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.OwnData{}, privacy.ErrNotFound
		}
		return privacy.OwnData{}, mapError(err)
	}
	data.Account.CreatedAt = createdAt.UTC()

	data.Roles = []string{}
	rows, err := tx.Query(ctx, `SELECT rol FROM public.rol_usuario WHERE usuario_id=$1 ORDER BY rol`, accountID)
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return privacy.OwnData{}, mapError(err)
		}
		data.Roles = append(data.Roles, role)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return privacy.OwnData{}, mapError(err)
	}
	rows.Close()

	data.Acceptances = []privacy.TermsAcceptance{}
	rows, err = tx.Query(ctx, `SELECT v.codigo, v.tipo, a.aceptada_en
		FROM public.aceptacion_terminos a JOIN public.version_terminos v ON v.id=a.version_id
		WHERE a.usuario_id=$1 ORDER BY a.aceptada_en, v.codigo`, accountID)
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	for rows.Next() {
		var item privacy.TermsAcceptance
		if err := rows.Scan(&item.Code, &item.Type, &item.AcceptedAt); err != nil {
			rows.Close()
			return privacy.OwnData{}, mapError(err)
		}
		item.AcceptedAt = item.AcceptedAt.UTC()
		data.Acceptances = append(data.Acceptances, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return privacy.OwnData{}, mapError(err)
	}
	rows.Close()

	var profile privacy.ExportProfile
	if err := tx.QueryRow(ctx, `SELECT nombre_visible, telefono_normalizado, actualizado_en
		FROM public.perfil_usuario WHERE usuario_id=$1`, accountID).Scan(&profile.Name, &profile.Phone, &profile.UpdatedAt); err == nil {
		profile.UpdatedAt = profile.UpdatedAt.UTC()
		data.Profile = &profile
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return privacy.OwnData{}, mapError(err)
	}

	data.Requests = []privacy.RightsRequest{}
	rows, err = tx.Query(ctx, `SELECT id::text, tipo, estado, solicitada_en, resuelta_en, motivo_resolucion_codigo, detalle_titular
		FROM public.solicitud_titular WHERE usuario_id=$1 ORDER BY solicitada_en, id`, accountID)
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	for rows.Next() {
		var item privacy.RightsRequest
		if err := rows.Scan(&item.ID, &item.Kind, &item.State, &item.CreatedAt, &item.ResolvedAt, &item.DecisionCode, &item.Detail); err != nil {
			rows.Close()
			return privacy.OwnData{}, mapError(err)
		}
		item.CreatedAt = item.CreatedAt.UTC()
		data.Requests = append(data.Requests, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return privacy.OwnData{}, mapError(err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT finalidad, estado, causal, solicitud_id::text, actualizado_en
		FROM public.privacidad_preferencia_finalidad
		WHERE cuenta_id=$1
		ORDER BY finalidad ASC`, accountID)
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	var prefs []privacy.PurposePreference
	for rows.Next() {
		var item privacy.PurposePreference
		var updatedAt time.Time
		if err := rows.Scan(&item.Purpose, &item.State, &item.Reason, &item.RequestID, &updatedAt); err != nil {
			rows.Close()
			return privacy.OwnData{}, mapError(err)
		}
		utcUpdated := updatedAt.UTC()
		item.UpdatedAt = &utcUpdated
		prefs = append(prefs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return privacy.OwnData{}, mapError(err)
	}
	rows.Close()
	if len(prefs) > 0 {
		data.PurposePreferences = prefs
	}

	rows, err = tx.Query(ctx, `SELECT id::text, solicitud_id::text, ambito, estado, motivo, aplicado_en, revisar_en, levantado_en, motivo_levantamiento
		FROM public.privacidad_bloqueo_tratamiento
		WHERE cuenta_id=$1
		ORDER BY aplicado_en DESC, id DESC`, accountID)
	if err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	var restrictions []privacy.TreatmentRestriction
	for rows.Next() {
		var item privacy.TreatmentRestriction
		if err := rows.Scan(&item.ID, &item.RequestID, &item.Scope, &item.State, &item.Reason, &item.AppliedAt, &item.ReviewAt, &item.LiftedAt, &item.LiftReason); err != nil {
			rows.Close()
			return privacy.OwnData{}, mapError(err)
		}
		item.AppliedAt = item.AppliedAt.UTC()
		item.ReviewAt = utcTimePtr(item.ReviewAt)
		item.LiftedAt = utcTimePtr(item.LiftedAt)
		restrictions = append(restrictions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return privacy.OwnData{}, mapError(err)
	}
	rows.Close()
	if len(restrictions) > 0 {
		data.TreatmentRestrictions = restrictions
	}

	if err := tx.Commit(ctx); err != nil {
		return privacy.OwnData{}, mapError(err)
	}
	return data, nil
}

// ReviewSuppression persists a read-only assessment of existing obligations.
// It serializes on the subject account, captures time after the locks and does
// not delete, scrub or change the rights request. A later executor must repeat
// these checks in the same transaction as any suppression effects.
func (r *IdentityRepository) ReviewSuppression(ctx context.Context, reviewerID, requestID, idempotencyKey, correlationID string, now func() time.Time) (privacy.SuppressionReview, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.SuppressionReview{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var subjectID, requestKind string
	if err := tx.QueryRow(ctx, `SELECT usuario_id::text, tipo FROM public.solicitud_titular WHERE id=$1`, requestID).Scan(&subjectID, &requestKind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.SuppressionReview{}, privacy.ErrNotFound
		}
		return privacy.SuppressionReview{}, mapError(err)
	}
	if requestKind != "supresion" {
		return privacy.SuppressionReview{}, privacy.ErrNotFound
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM public.usuario WHERE id=$1 FOR UPDATE`, subjectID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.SuppressionReview{}, privacy.ErrNotFound
		}
		return privacy.SuppressionReview{}, mapError(err)
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT estado FROM public.solicitud_titular WHERE id=$1 AND usuario_id=$2 AND tipo='supresion'`, requestID, subjectID).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.SuppressionReview{}, privacy.ErrNotFound
		}
		return privacy.SuppressionReview{}, mapError(err)
	}

	var existing privacy.SuppressionReview
	var existingReviewer, existingReason string
	var existingDetails []byte
	err = tx.QueryRow(ctx, `SELECT actor_id::text, motivo_codigo, ocurrido_en, detalle_codigos
		FROM public.evento_auditoria_local
		WHERE recurso_id=$1 AND accion='privacy.suppression.review' AND clave_idempotencia=$2`, requestID, idempotencyKey).Scan(
		&existingReviewer, &existingReason, &existing.ReviewedAt, &existingDetails)
	if err == nil {
		if existingReviewer != reviewerID {
			return privacy.SuppressionReview{}, identity.ErrConflict
		}
		existing.RequestID = requestID
		switch existingReason {
		case "supresion_con_obligaciones":
			existing.Outcome = "bloqueada"
		case "supresion_revision_incompleta", "supresion_elegible":
			if existingReason == "supresion_elegible" {
				existing.Outcome = "elegible"
			} else {
				existing.Outcome = "revision_incompleta"
			}
		default:
			return privacy.SuppressionReview{}, identity.ErrConflict
		}
		var details struct {
			Obligations   []string `json:"obligations_detected"`
			PendingChecks []string `json:"pending_checks"`
		}
		if err := json.Unmarshal(existingDetails, &details); err != nil {
			return privacy.SuppressionReview{}, mapError(err)
		}
		existing.Obligations = details.Obligations
		existing.PendingChecks = details.PendingChecks
		existing.Reused = true
		if err := tx.Commit(ctx); err != nil {
			return privacy.SuppressionReview{}, mapError(err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return privacy.SuppressionReview{}, mapError(err)
	}
	if state != "en_revision" {
		return privacy.SuppressionReview{}, identity.ErrConflict
	}
	if now == nil {
		now = time.Now
	}
	checkedAt := now().UTC().Truncate(time.Microsecond) // Sample post-lock and match PostgreSQL timestamp precision.
	obligations, err := activeSuppressionObligations(ctx, tx, subjectID)
	if err != nil {
		return privacy.SuppressionReview{}, err
	}
	pendingChecks := []string{}
	outcome := "elegible"
	if len(obligations) > 0 {
		outcome = "bloqueada"
	}
	persisted := privacy.SuppressionReview{
		RequestID: requestID, Outcome: outcome, Obligations: obligations,
		PendingChecks: pendingChecks, ReviewedAt: checkedAt,
	}
	auditID, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.SuppressionReview{}, err
	}
	reason := "supresion_elegible"
	result := "exito"
	if outcome == "bloqueada" {
		reason = "supresion_con_obligaciones"
	}
	detailJSON, err := json.Marshal(struct {
		Obligations   []string `json:"obligations_detected"`
		PendingChecks []string `json:"pending_checks"`
	}{Obligations: obligations, PendingChecks: pendingChecks})
	if err != nil {
		return privacy.SuppressionReview{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.evento_auditoria_local (
		id, actor_id, recurso_tipo, recurso_id, accion, resultado, motivo_codigo,
		correlacion_id, ocurrido_en, retirar_en, clave_idempotencia, detalle_codigos
	) VALUES ($1,$2,'solicitud_titular',$3,'privacy.suppression.review',$4,$5,$6,$7,$8,$9,$10::jsonb)`,
		auditID, reviewerID, requestID, result, reason, correlationID, checkedAt, identity.AddCalendarMonthsUTC(checkedAt, 60), idempotencyKey, string(detailJSON)); err != nil {
		return privacy.SuppressionReview{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return privacy.SuppressionReview{}, mapError(err)
	}
	return persisted, nil
}

func (r *IdentityRepository) ListPendingSuppressions(ctx context.Context) ([]privacy.SuppressionQueueItem, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, solicitada_en
		FROM public.solicitud_titular
		WHERE tipo='supresion' AND estado='en_revision'
		ORDER BY solicitada_en, id`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	items := make([]privacy.SuppressionQueueItem, 0)
	for rows.Next() {
		var item privacy.SuppressionQueueItem
		if err := rows.Scan(&item.RequestID, &item.CreatedAt); err != nil {
			return nil, mapError(err)
		}
		item.CreatedAt = item.CreatedAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return items, nil
}
