package adminglobal

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
)

func computePrivacyOverdue(item PrivacyRequestItem, now time.Time) bool {
	if item.Status == "completada" || item.Status == "rechazada" {
		return false
	}
	eff := privacy.EffectiveDeadline(item.DueAt, item.ExtendedDueAt)
	if eff == nil {
		return false
	}
	return now.After(*eff)
}

func buildPrivacyRequestWhere(f PrivacyRequestFilter, now time.Time) (string, []any) {
	conds := make([]string, 0, 6)
	args := make([]any, 0, 8)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "p.estado = $"+strconv.Itoa(len(args)))
	}
	if f.RightType != "" {
		args = append(args, f.RightType)
		conds = append(conds, "p.tipo_derecho = $"+strconv.Itoa(len(args)))
	}
	if f.AccountID != "" {
		args = append(args, f.AccountID)
		conds = append(conds, "p.cuenta_id = $"+strconv.Itoa(len(args)))
	}
	if f.AssignedToID != "" {
		args = append(args, f.AssignedToID)
		conds = append(conds, "p.asignado_a_id = $"+strconv.Itoa(len(args)))
	}
	if f.Channel != "" {
		args = append(args, f.Channel)
		conds = append(conds, "p.canal = $"+strconv.Itoa(len(args)))
	}
	if f.Overdue != nil {
		args = append(args, now.UTC())
		n := strconv.Itoa(len(args))
		if *f.Overdue {
			conds = append(conds, "COALESCE(p.prorrogada_hasta, p.vence_en) < $"+n+"::timestamptz AND p.estado NOT IN ('completada','rechazada')")
		} else {
			conds = append(conds, "(COALESCE(p.prorrogada_hasta, p.vence_en) IS NULL OR COALESCE(p.prorrogada_hasta, p.vence_en) >= $"+n+"::timestamptz OR p.estado IN ('completada','rechazada'))")
		}
	}
	return joinWhere(conds), args
}

func (s *Service) appendPrivacyEventTx(ctx context.Context, tx pgx.Tx, requestID, eventType, visibility string, stateFrom, stateTo, actorID, detail *string, newDueAt *time.Time, at time.Time) error {
	eventID, err := s.ids.ID()
	if err != nil {
		return err
	}
	var nextSeq int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(secuencia), 0) + 1 FROM public.solicitud_titular_evento WHERE solicitud_id=$1`, requestID).Scan(&nextSeq); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.solicitud_titular_evento (
			id, solicitud_id, secuencia, tipo_evento, visibilidad, estado_anterior, estado_nuevo, actor_id, detalle, vence_en_nuevo, creado_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, eventID, requestID, nextSeq, eventType, visibility, stateFrom, stateTo, actorID, detail, newDueAt, at.UTC())
	return err
}

func loadPrivacyDetailFromTx(ctx context.Context, tx pgx.Tx, requestID string, now time.Time) (PrivacyRequestDetail, error) {
	var d PrivacyRequestDetail
	var rawState, response, rawEmail string
	var hasReceipt bool
	err := tx.QueryRow(ctx, `
		SELECT s.id::text, s.folio_publico, COALESCE(s.usuario_id::text, ''), s.tipo, s.estado,
		       COALESCE(s.motivo_administrativo,''), COALESCE(s.caso_ref_administrativo,''),
		       COALESCE(s.solicitado_por::text,''), s.canal,
		       s.solicitante_tipo, COALESCE(s.solicitante_nombre,''), COALESCE(s.solicitante_correo,''),
		       COALESCE(s.representante_nombre,''), COALESCE(s.representante_relacion,''), COALESCE(s.titular_referencia,''),
		       COALESCE(s.asignado_a_id::text,''),
		       s.vence_en, s.prorrogada_hasta, s.prorroga_motivo, s.version,
		       s.solicitada_en, COALESCE(s.resuelta_en,s.solicitada_en), s.resuelta_en,
		       NULLIF(to_jsonb(s)->>'detalle_titular',''),
		       COALESCE(s.respuesta_titular,''),
		       (d.request_id IS NOT NULL), d.dispatch_recorded_at
		FROM public.solicitud_titular s
		LEFT JOIN public.privacy_response_dispatch_receipt d ON d.request_id = s.id
		WHERE s.id = $1
	`, requestID).Scan(
		&d.ID, &d.Folio, &d.AccountID, &d.RightType, &rawState,
		&d.Reason, &d.CaseRef, &d.RequestedBy, &d.Channel,
		&d.RequesterType, &d.RequesterName, &rawEmail,
		&d.RepresentativeName, &d.RepresentativeRelation, &d.SubjectReference,
		&d.AssignedToID,
		&d.DueAt, &d.ExtendedDueAt, &d.ExtensionReason, &d.Version,
		&d.CreatedAt, &d.UpdatedAt, &d.ResolvedAt, &d.Detail,
		&response, &hasReceipt, &d.DispatchRecordedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		var legacy PrivacyRequestItem
		legacyErr := tx.QueryRow(ctx, `
			SELECT id::text, COALESCE(cuenta_id::text, ''), tipo_derecho, estado, motivo, caso_ref,
			       COALESCE(solicitado_por::text,''), creado_en, actualizado_en, resuelto_en
			FROM public.solicitud_privacidad_administrativa
			WHERE id = $1
		`, requestID).Scan(
			&legacy.ID, &legacy.AccountID, &legacy.RightType, &legacy.Status, &legacy.Reason,
			&legacy.CaseRef, &legacy.RequestedBy, &legacy.CreatedAt, &legacy.UpdatedAt, &legacy.ResolvedAt,
		)
		if errors.Is(legacyErr, pgx.ErrNoRows) {
			return PrivacyRequestDetail{}, ErrNotFound
		}
		if legacyErr != nil {
			return PrivacyRequestDetail{}, legacyErr
		}
		legacy.Channel = "api"
		legacy.Version = 1
		return PrivacyRequestDetail{PrivacyRequestItem: legacy, Events: []PrivacyRequestEvent{}}, nil
	}
	if err != nil {
		return PrivacyRequestDetail{}, err
	}

	d.RequesterEmailMasked = maskEmail(rawEmail)
	d.Status = mapPrivacyAdminStatus(rawState)
	d.CanPublishResponse = rawState == "recibida" || rawState == "en_revision"
	d.CanRecordDispatch = (rawState == "resuelta" || rawState == "denegada_fundada") && response != "" && !hasReceipt
	d.IsOverdue = computePrivacyOverdue(d.PrivacyRequestItem, now)
	if response != "" {
		resp := response
		d.Response = &resp
	}

	var summary PrivacyNotificationSummary
	if err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE estado IN ('pending', 'processing')), 0)::int,
			COALESCE(COUNT(*) FILTER (WHERE estado = 'delivered'), 0)::int,
			COALESCE(COUNT(*) FILTER (WHERE estado = 'terminal'), 0)::int,
			(SELECT ultimo_error_codigo
			 FROM public.privacidad_notificacion_outbox
			 WHERE solicitud_id = $1 AND ultimo_error_codigo IS NOT NULL
			 ORDER BY COALESCE(terminal_en, disponible_en, creado_en) DESC, id DESC
			 LIMIT 1),
			MAX(entregado_en)
		FROM public.privacidad_notificacion_outbox
		WHERE solicitud_id = $1
	`, requestID).Scan(
		&summary.Pending,
		&summary.Delivered,
		&summary.Terminal,
		&summary.LastErrorCode,
		&summary.LastDeliveredAt,
	); err != nil {
		return PrivacyRequestDetail{}, err
	}
	if summary.LastDeliveredAt != nil {
		utc := summary.LastDeliveredAt.UTC()
		summary.LastDeliveredAt = &utc
	}
	d.NotificationSummary = &summary

	rows, err := tx.Query(ctx, `
		SELECT id::text, secuencia, tipo_evento, visibilidad, estado_anterior, estado_nuevo,
		       actor_id::text, detalle, vence_en_nuevo, creado_en
		FROM public.solicitud_titular_evento
		WHERE solicitud_id = $1
		ORDER BY secuencia ASC
	`, requestID)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	defer rows.Close()
	events := []PrivacyRequestEvent{}
	for rows.Next() {
		var ev PrivacyRequestEvent
		if err := rows.Scan(&ev.ID, &ev.Sequence, &ev.EventType, &ev.Visibility, &ev.StateFrom, &ev.StateTo, &ev.ActorID, &ev.Detail, &ev.NewDueAt, &ev.CreatedAt); err != nil {
			return PrivacyRequestDetail{}, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return PrivacyRequestDetail{}, err
	}
	d.Events = events

	executions, err := loadPrivacyExecutionsTx(ctx, tx, requestID)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	d.Executions = executions

	blocks, err := loadPrivacyTreatmentBlocksTx(ctx, tx, requestID, d.AccountID)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	d.TreatmentBlocks = blocks

	if d.AccountID != "" {
		prefs, err := loadPrivacyPurposePreferencesTx(ctx, tx, d.AccountID)
		if err != nil {
			return PrivacyRequestDetail{}, err
		}
		d.PurposePreferences = prefs
	}

	return d, nil
}

func loadPrivacyExecutionsTx(ctx context.Context, tx pgx.Tx, requestID string) ([]PrivacyRightExecution, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, solicitud_id::text, COALESCE(cuenta_id::text, ''), tipo_ejecucion, estado,
		       ambito, resumen_operativo, destino_referencia, sha256_artefacto, ejecutado_por::text, ejecutado_en
		FROM public.privacidad_ejecucion_derecho
		WHERE solicitud_id = $1
		ORDER BY ejecutado_en ASC, id ASC
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PrivacyRightExecution
	for rows.Next() {
		var item PrivacyRightExecution
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.AccountID, &item.ExecutionType, &item.Status,
			&item.Scope, &item.Summary, &item.TargetReference, &item.ArtifactSHA256,
			&item.ExecutedBy, &item.ExecutedAt,
		); err != nil {
			return nil, err
		}
		item.ExecutedAt = item.ExecutedAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func loadPrivacyTreatmentBlocksTx(ctx context.Context, tx pgx.Tx, requestID, accountID string) ([]PrivacyTreatmentBlock, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, solicitud_id::text, cuenta_id::text, ambito, estado, motivo,
		       aplicado_por::text, aplicado_en, revisar_en, levantado_por::text, levantado_en, motivo_levantamiento
		FROM public.privacidad_bloqueo_tratamiento
		WHERE solicitud_id = $1 OR (cuenta_id = NULLIF($2,'')::uuid)
		ORDER BY aplicado_en DESC, id DESC
	`, requestID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PrivacyTreatmentBlock
	for rows.Next() {
		var item PrivacyTreatmentBlock
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.AccountID, &item.Scope, &item.State, &item.Reason,
			&item.AppliedBy, &item.AppliedAt, &item.ReviewAt, &item.LiftedBy, &item.LiftedAt, &item.LiftReason,
		); err != nil {
			return nil, err
		}
		item.AppliedAt = item.AppliedAt.UTC()
		item.ReviewAt = utcPointer(item.ReviewAt)
		item.LiftedAt = utcPointer(item.LiftedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func loadPrivacyPurposePreferencesTx(ctx context.Context, tx pgx.Tx, accountID string) ([]privacy.PurposePreference, error) {
	rows, err := tx.Query(ctx, `
		SELECT finalidad, estado, causal, solicitud_id::text, actualizado_en
		FROM public.privacidad_preferencia_finalidad
		WHERE cuenta_id = $1
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byPurpose := map[string]privacy.PurposePreference{}
	for rows.Next() {
		var pref privacy.PurposePreference
		var updatedAt time.Time
		if err := rows.Scan(&pref.Purpose, &pref.State, &pref.Reason, &pref.RequestID, &updatedAt); err != nil {
			return nil, err
		}
		ts := updatedAt.UTC()
		pref.UpdatedAt = &ts
		byPurpose[pref.Purpose] = pref
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	purposes := privacy.AllPurposes()
	out := make([]privacy.PurposePreference, 0, len(purposes))
	for _, purpose := range purposes {
		if pref, ok := byPurpose[purpose]; ok {
			out = append(out, pref)
			continue
		}
		out = append(out, privacy.PurposePreference{
			Purpose: purpose,
			State:   privacy.PurposeStateAllowed,
		})
	}
	return out, nil
}

// GetPrivacyRequest returns the canonical privacy case detail and its full
// internal + titular event timeline for authorized administrators.
func (s *Service) GetPrivacyRequest(ctx context.Context, actor, correlation, requestID string) (PrivacyRequestDetail, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) || !canonicalUUID.MatchString(requestID) {
		return PrivacyRequestDetail{}, ErrInvalid
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.GetPrivacyRequest(ctx, actor, correlation, requestID)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	return loadPrivacyDetailFromTx(ctx, tx, requestID, s.currentTime())
}

// TransitionPrivacyRequest advances or annotates a canonical privacy request with
// optimistic concurrency and append-only event history.
func (s *Service) TransitionPrivacyRequest(ctx context.Context, input TransitionPrivacyRequestInput) (PrivacyRequestDetail, error) {
	input.Note = strings.TrimSpace(input.Note)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.RequestID) || input.ExpectedVersion < 1 || strings.ContainsRune(input.Note, 0) {
		return PrivacyRequestDetail{}, ErrInvalid
	}
	noteRunes := utf8.RuneCountInString(input.Note)
	switch input.Action {
	case "assign":
		if !canonicalUUID.MatchString(input.AssignedToID) || noteRunes > 500 {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "verify_identity":
		if noteRunes > 1000 || (input.AccountID != "" && !canonicalUUID.MatchString(input.AccountID)) {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "start_review":
		if noteRunes > 1000 {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "request_clarification":
		if noteRunes < 10 || noteRunes > 1000 {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "add_internal_note":
		if noteRunes < 5 || noteRunes > 2000 {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "extend_deadline":
		if input.ExtraDays < 1 || input.ExtraDays > 30 || noteRunes < 10 || noteRunes > 1000 {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	default:
		return PrivacyRequestDetail{}, ErrInvalid
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.TransitionPrivacyRequest(ctx, input)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyRequestDetail{}, err
	}

	var accountID, folio, rightType, rawState, recipientEmail string
	var createdAt time.Time
	var dueAt, extendedDueAt *time.Time
	var currentVersion int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(s.usuario_id::text, ''), s.folio_publico, s.tipo, s.estado,
		       s.solicitada_en, s.vence_en, s.prorrogada_hasta, s.version,
		       COALESCE(NULLIF(BTRIM(s.solicitante_correo), ''), u.correo_original, '')
		FROM public.solicitud_titular s
		LEFT JOIN public.usuario u ON u.id = s.usuario_id
		WHERE s.id = $1
		FOR UPDATE OF s
	`, input.RequestID).Scan(&accountID, &folio, &rightType, &rawState, &createdAt, &dueAt, &extendedDueAt, &currentVersion, &recipientEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyRequestDetail{}, ErrNotFound
	}
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	if currentVersion != input.ExpectedVersion {
		return PrivacyRequestDetail{}, ErrConflict
	}

	now := s.currentTime()
	actorPtr := &input.ActorID
	var reasonCode string
	switch input.Action {
	case "assign":
		var assigneeExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id=$1)`, input.AssignedToID).Scan(&assigneeExists); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if !assigneeExists {
			return PrivacyRequestDetail{}, ErrNotFound
		}
		if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET asignado_a_id=$2, version=version+1 WHERE id=$1`, input.RequestID, input.AssignedToID); err != nil {
			return PrivacyRequestDetail{}, err
		}
		detail := "Responsable asignado"
		if input.Note != "" {
			detail = input.Note
		}
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "asignacion", "interna", nil, nil, actorPtr, &detail, nil, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		reasonCode = "asignacion_caso"

	case "verify_identity":
		if rawState != "identidad_pendiente" {
			return PrivacyRequestDetail{}, ErrConflict
		}
		const nextState = "en_revision"
		if input.AccountID != "" {
			if accountID != "" && accountID != input.AccountID {
				return PrivacyRequestDetail{}, ErrConflict
			}
			var accountExists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id=$1)`, input.AccountID).Scan(&accountExists); err != nil {
				return PrivacyRequestDetail{}, err
			}
			if !accountExists {
				return PrivacyRequestDetail{}, ErrNotFound
			}
			if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET estado=$2, usuario_id=COALESCE(usuario_id, $3::uuid), version=version+1 WHERE id=$1`, input.RequestID, nextState, input.AccountID); err != nil {
				return PrivacyRequestDetail{}, err
			}
			accountID = input.AccountID
		} else {
			if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET estado=$2, version=version+1 WHERE id=$1`, input.RequestID, nextState); err != nil {
				return PrivacyRequestDetail{}, err
			}
		}
		var detailPtr *string
		if input.Note != "" {
			detailPtr = &input.Note
		}
		prev, next := rawState, nextState
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "identidad_verificada", "titular", &prev, &next, actorPtr, detailPtr, nil, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		reasonCode = "identidad_verificada"

	case "start_review":
		if rawState != "recibida" {
			return PrivacyRequestDetail{}, ErrConflict
		}
		const nextState = "en_revision"
		if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET estado=$2, version=version+1 WHERE id=$1`, input.RequestID, nextState); err != nil {
			return PrivacyRequestDetail{}, err
		}
		var detailPtr *string
		if input.Note != "" {
			detailPtr = &input.Note
		}
		prev, next := rawState, nextState
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "en_revision", "titular", &prev, &next, actorPtr, detailPtr, nil, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		reasonCode = "inicio_revision"

	case "request_clarification":
		if rawState != "recibida" && rawState != "identidad_pendiente" && rawState != "en_revision" {
			return PrivacyRequestDetail{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET version=version+1 WHERE id=$1`, input.RequestID); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "aclaracion_solicitada", "titular", nil, nil, actorPtr, &input.Note, nil, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if accountID != "" {
			notificationID, err := s.ids.ID()
			if err != nil {
				return PrivacyRequestDetail{}, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
				VALUES($1,$2,'privacidad_aclaracion','Aclaración sobre tu solicitud de privacidad','Revisa tu historial de privacidad para responder la aclaración solicitada.','/mi-cuenta/privacidad',$3,$4)
				ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING
			`, notificationID, accountID, input.RequestID, now); err != nil {
				return PrivacyRequestDetail{}, err
			}
		}
		if validOutboxEmail(recipientEmail) {
			subject, body := formatClarificationNotice(folio)
			if err := enqueuePrivacyNoticeTx(ctx, tx, input.RequestID, "aclaracion_solicitada", "clarif:"+input.RequestID+":v"+strconv.Itoa(currentVersion), recipientEmail, subject, body, now); err != nil {
				return PrivacyRequestDetail{}, err
			}
		}
		reasonCode = "aclaracion_solicitada"

	case "add_internal_note":
		if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET version=version+1 WHERE id=$1`, input.RequestID); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "nota_interna", "interna", nil, nil, actorPtr, &input.Note, nil, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		reasonCode = "nota_interna"

	case "extend_deadline":
		if (rawState != "recibida" && rawState != "identidad_pendiente" && rawState != "en_revision") || extendedDueAt != nil {
			return PrivacyRequestDetail{}, ErrConflict
		}
		baseDue := privacy.ComputeInitialDeadline(createdAt, rightType)
		if dueAt != nil {
			baseDue = *dueAt
		}
		newDueAt, err := privacy.ComputeExtensionDeadline(baseDue, input.ExtraDays)
		if err != nil {
			return PrivacyRequestDetail{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE public.solicitud_titular
			SET vence_en=$2, prorrogada_hasta=$3, prorroga_motivo=$4, version=version+1
			WHERE id=$1
		`, input.RequestID, baseDue, newDueAt, input.Note); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "prorroga", "titular", nil, nil, actorPtr, &input.Note, &newDueAt, now); err != nil {
			return PrivacyRequestDetail{}, err
		}
		if accountID != "" {
			notificationID, err := s.ids.ID()
			if err != nil {
				return PrivacyRequestDetail{}, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
				VALUES($1,$2,'privacidad_prorroga','Actualización de plazo en tu solicitud de privacidad','Se registró una prórroga operativa en tu solicitud de privacidad.','/mi-cuenta/privacidad',$3,$4)
				ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING
			`, notificationID, accountID, input.RequestID, now); err != nil {
				return PrivacyRequestDetail{}, err
			}
		}
		if validOutboxEmail(recipientEmail) {
			subject, body := formatExtensionNotice(folio, newDueAt)
			if err := enqueuePrivacyNoticeTx(ctx, tx, input.RequestID, "prorroga_plazo", "ext:"+input.RequestID, recipientEmail, subject, body, now); err != nil {
				return PrivacyRequestDetail{}, err
			}
		}
		reasonCode = "prorroga_plazo"
	}

	if err := s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests."+input.Action, "exito", reasonCode, input.CorrelationID, now, accountID); err != nil {
		return PrivacyRequestDetail{}, err
	}
	detail, err := loadPrivacyDetailFromTx(ctx, tx, input.RequestID, now)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyRequestDetail{}, err
	}
	return detail, nil
}

// RetryPrivacyNotifications re-queues terminal outbox notices for a privacy request
// back to pending state so the outbox worker can re-attempt delivery.
func (s *Service) RetryPrivacyNotifications(ctx context.Context, actor, correlation, requestID string) (PrivacyRequestDetail, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) || !canonicalUUID.MatchString(requestID) {
		return PrivacyRequestDetail{}, ErrInvalid
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.RetryPrivacyNotifications(ctx, actor, correlation, requestID)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, actor); err != nil {
		return PrivacyRequestDetail{}, err
	}

	var accountID string
	err = tx.QueryRow(ctx, `SELECT COALESCE(usuario_id::text, '') FROM public.solicitud_titular WHERE id = $1 FOR UPDATE`, requestID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyRequestDetail{}, ErrNotFound
	}
	if err != nil {
		return PrivacyRequestDetail{}, err
	}

	now := s.currentTime()
	tag, err := tx.Exec(ctx, `
		UPDATE public.privacidad_notificacion_outbox
		SET estado = 'pending',
		    intentos = 0,
		    disponible_en = $2,
		    reclamado_en = NULL,
		    terminal_en = NULL,
		    ultimo_error_codigo = NULL
		WHERE solicitud_id = $1 AND estado = 'terminal'
	`, requestID, now)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	if tag.RowsAffected() == 0 {
		return PrivacyRequestDetail{}, ErrConflict
	}

	if err := s.auditWithTarget(ctx, tx, actor, requestID, "admin.global.privacy_requests.notifications.retry", "exito", "reintento_notificacion", correlation, now, accountID); err != nil {
		return PrivacyRequestDetail{}, err
	}
	detail, err := loadPrivacyDetailFromTx(ctx, tx, requestID, now)
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyRequestDetail{}, err
	}
	return detail, nil
}
