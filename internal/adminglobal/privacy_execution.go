package adminglobal

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ExecutePrivacyRight performs an audited, account-linked ARCO+ execution step
// on an open privacy request using optimistic concurrency.
func (s *Service) ExecutePrivacyRight(ctx context.Context, input ExecutePrivacyRightInput) (PrivacyRequestDetail, error) {
	input.Summary = strings.TrimSpace(input.Summary)
	input.TargetReference = strings.TrimSpace(input.TargetReference)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.RequestID) || input.ExpectedVersion < 1 ||
		!utf8.ValidString(input.Summary) || strings.ContainsRune(input.Summary, 0) {
		return PrivacyRequestDetail{}, ErrInvalid
	}
	summaryRunes := utf8.RuneCountInString(input.Summary)
	if summaryRunes < 10 || summaryRunes > 1000 {
		return PrivacyRequestDetail{}, ErrInvalid
	}

	var cleanName, cleanPhone *string
	switch input.Action {
	case "rectify_profile":
		var ok bool
		cleanName, cleanPhone, ok = privacy.ValidateProfileRectification(input.DisplayName, input.Phone)
		if !ok {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "apply_opposition":
		if !privacy.ValidPurpose(input.Purpose) ||
			(input.PurposeState != privacy.PurposeStateOpposed && input.PurposeState != privacy.PurposeStateWithdrawn) {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "apply_block":
		if !privacy.ValidTreatmentScope(input.Scope) {
			return PrivacyRequestDetail{}, ErrInvalid
		}
		if input.ReviewAt != nil && input.ReviewAt.IsZero() {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "lift_block":
		if !canonicalUUID.MatchString(input.BlockID) {
			return PrivacyRequestDetail{}, ErrInvalid
		}
	case "issue_portability":
		if input.TargetReference != "" {
			if !utf8.ValidString(input.TargetReference) || strings.ContainsRune(input.TargetReference, 0) {
				return PrivacyRequestDetail{}, ErrInvalid
			}
			refRunes := utf8.RuneCountInString(input.TargetReference)
			if refRunes < 2 || refRunes > 200 {
				return PrivacyRequestDetail{}, ErrInvalid
			}
		}
	default:
		return PrivacyRequestDetail{}, ErrInvalid
	}

	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.ExecutePrivacyRight(ctx, input)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyRequestDetail{}, err
	}

	var accountID, rightType, rawState string
	var currentVersion int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(usuario_id::text, ''), tipo, estado, version
		FROM public.solicitud_titular
		WHERE id = $1
		FOR UPDATE
	`, input.RequestID).Scan(&accountID, &rightType, &rawState, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyRequestDetail{}, ErrNotFound
	}
	if err != nil {
		return PrivacyRequestDetail{}, err
	}
	if currentVersion != input.ExpectedVersion || accountID == "" {
		return PrivacyRequestDetail{}, ErrConflict
	}

	now := s.currentTime().Truncate(time.Microsecond)
	switch input.Action {
	case "rectify_profile":
		err = s.executeRectifyProfileTx(ctx, tx, input, accountID, rightType, rawState, cleanName, cleanPhone, now)
	case "apply_opposition":
		err = s.executeApplyOppositionTx(ctx, tx, input, accountID, rightType, rawState, now)
	case "apply_block":
		err = s.executeApplyBlockTx(ctx, tx, input, accountID, rightType, rawState, now)
	case "lift_block":
		err = s.executeLiftBlockTx(ctx, tx, input, accountID, rawState, now)
	case "issue_portability":
		err = s.executeIssuePortabilityTx(ctx, tx, input, accountID, rightType, rawState, now)
	}
	if err != nil {
		return PrivacyRequestDetail{}, err
	}

	if _, err := tx.Exec(ctx, `UPDATE public.solicitud_titular SET version = version + 1 WHERE id = $1`, input.RequestID); err != nil {
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

func (s *Service) executeRectifyProfileTx(
	ctx context.Context,
	tx pgx.Tx,
	input ExecutePrivacyRightInput,
	accountID, rightType, rawState string,
	cleanName, cleanPhone *string,
	now time.Time,
) error {
	if rightType != "rectificacion" || (rawState != "recibida" && rawState != "en_revision") {
		return ErrConflict
	}

	var existingName string
	var existingPhone *string
	err := tx.QueryRow(ctx, `
		SELECT nombre_visible, telefono_normalizado
		FROM public.perfil_usuario
		WHERE usuario_id = $1
		FOR UPDATE
	`, accountID).Scan(&existingName, &existingPhone)
	profileExists := true
	if errors.Is(err, pgx.ErrNoRows) {
		profileExists = false
	} else if err != nil {
		return err
	}
	if !profileExists && cleanName == nil {
		return ErrInvalid
	}

	finalName := existingName
	if cleanName != nil {
		finalName = *cleanName
	}
	finalPhone := existingPhone
	if cleanPhone != nil {
		if *cleanPhone == "" {
			finalPhone = nil
		} else {
			phoneCopy := *cleanPhone
			finalPhone = &phoneCopy
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO public.perfil_usuario (usuario_id, nombre_visible, telefono_normalizado, actualizado_en)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (usuario_id) DO UPDATE
		SET nombre_visible = EXCLUDED.nombre_visible,
		    telefono_normalizado = EXCLUDED.telefono_normalizado,
		    actualizado_en = EXCLUDED.actualizado_en
	`, accountID, finalName, finalPhone, now); err != nil {
		return err
	}

	if err := s.recordPrivacyExecutionTx(ctx, tx, input.RequestID, accountID, "rectificacion_perfil", "perfil_usuario", input.Summary, nil, nil, input.ActorID, now); err != nil {
		return err
	}
	eventDetail := "Rectificación aplicada sobre perfil de cuenta: " + input.Summary
	if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "ejecucion_derecho", "titular", nil, nil, &input.ActorID, &eventDetail, nil, now); err != nil {
		return err
	}
	return s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests.execute.rectify_profile", "exito", "rectificacion_perfil", input.CorrelationID, now, accountID)
}

func (s *Service) executeApplyOppositionTx(
	ctx context.Context,
	tx pgx.Tx,
	input ExecutePrivacyRightInput,
	accountID, rightType, rawState string,
	now time.Time,
) error {
	if rightType != "oposicion" || (rawState != "recibida" && rawState != "en_revision") {
		return ErrConflict
	}

	causal := truncateRunesTrimmed(input.Summary, 500)
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.privacidad_preferencia_finalidad (
			cuenta_id, finalidad, estado, causal, solicitud_id, actualizado_por, actualizado_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (cuenta_id, finalidad) DO UPDATE
		SET estado = EXCLUDED.estado,
		    causal = EXCLUDED.causal,
		    solicitud_id = EXCLUDED.solicitud_id,
		    actualizado_por = EXCLUDED.actualizado_por,
		    actualizado_en = EXCLUDED.actualizado_en
	`, accountID, input.Purpose, input.PurposeState, causal, input.RequestID, input.ActorID, now); err != nil {
		return err
	}

	if err := s.recordPrivacyExecutionTx(ctx, tx, input.RequestID, accountID, "oposicion_finalidad", input.Purpose, input.Summary, nil, nil, input.ActorID, now); err != nil {
		return err
	}
	eventDetail := "Preferencia de finalidad actualizada (" + input.Purpose + ": " + input.PurposeState + "): " + input.Summary
	if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "ejecucion_derecho", "titular", nil, nil, &input.ActorID, &eventDetail, nil, now); err != nil {
		return err
	}
	return s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests.execute.apply_opposition", "exito", "oposicion_finalidad", input.CorrelationID, now, accountID)
}

func (s *Service) executeApplyBlockTx(
	ctx context.Context,
	tx pgx.Tx,
	input ExecutePrivacyRightInput,
	accountID, rightType, rawState string,
	now time.Time,
) error {
	if (rightType != "bloqueo" && rightType != "rectificacion" && rightType != "supresion" && rightType != "oposicion") ||
		(rawState != "recibida" && rawState != "en_revision") {
		return ErrConflict
	}

	blockID, err := s.ids.ID()
	if err != nil {
		return err
	}
	var reviewAt *time.Time
	if input.ReviewAt != nil {
		utc := input.ReviewAt.UTC().Truncate(time.Microsecond)
		reviewAt = &utc
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.privacidad_bloqueo_tratamiento (
			id, cuenta_id, solicitud_id, ambito, estado, motivo, aplicado_por, aplicado_en, revisar_en
		) VALUES ($1, $2, $3, $4, 'activo', $5, $6, $7, $8)
	`, blockID, accountID, input.RequestID, input.Scope, input.Summary, input.ActorID, now, reviewAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return err
	}

	if err := s.recordPrivacyExecutionTx(ctx, tx, input.RequestID, accountID, "bloqueo_aplicado", input.Scope, input.Summary, nil, nil, input.ActorID, now); err != nil {
		return err
	}
	eventDetail := "Bloqueo de tratamiento aplicado (" + input.Scope + "): " + input.Summary
	if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "ejecucion_derecho", "titular", nil, nil, &input.ActorID, &eventDetail, nil, now); err != nil {
		return err
	}
	return s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests.execute.apply_block", "exito", "bloqueo_aplicado", input.CorrelationID, now, accountID)
}

func (s *Service) executeLiftBlockTx(
	ctx context.Context,
	tx pgx.Tx,
	input ExecutePrivacyRightInput,
	accountID, rawState string,
	now time.Time,
) error {
	if rawState != "recibida" && rawState != "en_revision" {
		return ErrConflict
	}

	var liftedScope string
	err := tx.QueryRow(ctx, `
		UPDATE public.privacidad_bloqueo_tratamiento
		SET estado = 'levantado',
		    levantado_por = $3,
		    levantado_en = $4,
		    motivo_levantamiento = $5
		WHERE id = $1 AND cuenta_id = $2 AND estado = 'activo'
		RETURNING ambito
	`, input.BlockID, accountID, input.ActorID, now, input.Summary).Scan(&liftedScope)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}

	if err := s.recordPrivacyExecutionTx(ctx, tx, input.RequestID, accountID, "bloqueo_levantado", liftedScope, input.Summary, nil, nil, input.ActorID, now); err != nil {
		return err
	}
	eventDetail := "Bloqueo de tratamiento levantado (" + liftedScope + "): " + input.Summary
	if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "ejecucion_derecho", "titular", nil, nil, &input.ActorID, &eventDetail, nil, now); err != nil {
		return err
	}
	return s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests.execute.lift_block", "exito", "bloqueo_levantado", input.CorrelationID, now, accountID)
}

func (s *Service) executeIssuePortabilityTx(
	ctx context.Context,
	tx pgx.Tx,
	input ExecutePrivacyRightInput,
	accountID, rightType, rawState string,
	now time.Time,
) error {
	if rightType != "portabilidad" || (rawState != "recibida" && rawState != "en_revision") {
		return ErrConflict
	}

	pkg, err := buildAccountPortabilityPackageTx(ctx, tx, accountID, now)
	if err != nil {
		return err
	}
	_, sha256Hex, err := privacy.BuildPortabilityJSON(pkg)
	if err != nil {
		return err
	}

	execType := "portabilidad_emitida"
	var targetRef *string
	eventPrefix := "Paquete de portabilidad emitido"
	if input.TargetReference != "" {
		execType = "transferencia_portabilidad"
		ref := input.TargetReference
		targetRef = &ref
		eventPrefix = "Transferencia de portabilidad registrada"
	}

	if err := s.recordPrivacyExecutionTx(ctx, tx, input.RequestID, accountID, execType, "paquete_portabilidad_v1", input.Summary, targetRef, &sha256Hex, input.ActorID, now); err != nil {
		return err
	}
	eventDetail := eventPrefix + " (SHA-256: " + sha256Hex + "): " + input.Summary
	if err := s.appendPrivacyEventTx(ctx, tx, input.RequestID, "ejecucion_derecho", "titular", nil, nil, &input.ActorID, &eventDetail, nil, now); err != nil {
		return err
	}
	return s.auditWithTarget(ctx, tx, input.ActorID, input.RequestID, "admin.global.privacy_requests.execute.issue_portability", "exito", execType, input.CorrelationID, now, accountID)
}

func (s *Service) recordPrivacyExecutionTx(
	ctx context.Context,
	tx pgx.Tx,
	requestID, accountID, execType, scope, summary string,
	targetRef, sha256Hex *string,
	actorID string,
	now time.Time,
) error {
	execID, err := s.ids.ID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.privacidad_ejecucion_derecho (
			id, solicitud_id, cuenta_id, tipo_ejecucion, estado, ambito, resumen_operativo,
			destino_referencia, sha256_artefacto, ejecutado_por, ejecutado_en
		) VALUES ($1, $2, $3, $4, 'ejecutada', $5, $6, $7, $8, $9, $10)
	`, execID, requestID, accountID, execType, scope, summary, targetRef, sha256Hex, actorID, now)
	return err
}

func buildAccountPortabilityPackageTx(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (privacy.PortabilityPackage, error) {
	pkg := privacy.PortabilityPackage{
		SchemaVersion:         privacy.PortabilitySchemaVersion,
		GeneratedAt:           now.UTC(),
		Roles:                 []string{},
		Acceptances:           []privacy.TermsAcceptance{},
		PurposePreferences:    []privacy.PurposePreference{},
		TreatmentRestrictions: []privacy.TreatmentRestriction{},
		Requests:              []privacy.RightsRequest{},
	}

	var accountCreatedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT correo_original, estado, preferencia_uso, creado_en
		FROM public.usuario
		WHERE id = $1
	`, accountID).Scan(&pkg.Account.Email, &pkg.Account.State, &pkg.Account.UsePreference, &accountCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return privacy.PortabilityPackage{}, ErrNotFound
	}
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	pkg.Account.CreatedAt = accountCreatedAt.UTC()

	var profile privacy.ExportProfile
	var profileUpdatedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT nombre_visible, telefono_normalizado, actualizado_en
		FROM public.perfil_usuario
		WHERE usuario_id = $1
	`, accountID).Scan(&profile.Name, &profile.Phone, &profileUpdatedAt)
	if err == nil {
		profile.UpdatedAt = profileUpdatedAt.UTC()
		pkg.Profile = &profile
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return privacy.PortabilityPackage{}, err
	}

	roleRows, err := tx.Query(ctx, `SELECT rol FROM public.rol_usuario WHERE usuario_id = $1 ORDER BY rol`, accountID)
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	for roleRows.Next() {
		var role string
		if err := roleRows.Scan(&role); err != nil {
			roleRows.Close()
			return privacy.PortabilityPackage{}, err
		}
		pkg.Roles = append(pkg.Roles, role)
	}
	if err := roleRows.Err(); err != nil {
		roleRows.Close()
		return privacy.PortabilityPackage{}, err
	}
	roleRows.Close()

	accRows, err := tx.Query(ctx, `
		SELECT v.codigo, v.tipo, a.aceptada_en
		FROM public.aceptacion_terminos a
		JOIN public.version_terminos v ON v.id = a.version_id
		WHERE a.usuario_id = $1
		ORDER BY a.aceptada_en, v.codigo
	`, accountID)
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	for accRows.Next() {
		var item privacy.TermsAcceptance
		if err := accRows.Scan(&item.Code, &item.Type, &item.AcceptedAt); err != nil {
			accRows.Close()
			return privacy.PortabilityPackage{}, err
		}
		item.AcceptedAt = item.AcceptedAt.UTC()
		pkg.Acceptances = append(pkg.Acceptances, item)
	}
	if err := accRows.Err(); err != nil {
		accRows.Close()
		return privacy.PortabilityPackage{}, err
	}
	accRows.Close()

	prefs, err := loadPrivacyPurposePreferencesTx(ctx, tx, accountID)
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	pkg.PurposePreferences = prefs

	blockRows, err := tx.Query(ctx, `
		SELECT id::text, solicitud_id::text, ambito, estado, motivo,
		       aplicado_en, revisar_en, levantado_en, motivo_levantamiento
		FROM public.privacidad_bloqueo_tratamiento
		WHERE cuenta_id = $1
		ORDER BY aplicado_en DESC, id DESC
	`, accountID)
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	for blockRows.Next() {
		var item privacy.TreatmentRestriction
		if err := blockRows.Scan(
			&item.ID, &item.RequestID, &item.Scope, &item.State, &item.Reason,
			&item.AppliedAt, &item.ReviewAt, &item.LiftedAt, &item.LiftReason,
		); err != nil {
			blockRows.Close()
			return privacy.PortabilityPackage{}, err
		}
		item.AppliedAt = item.AppliedAt.UTC()
		item.ReviewAt = utcPointer(item.ReviewAt)
		item.LiftedAt = utcPointer(item.LiftedAt)
		pkg.TreatmentRestrictions = append(pkg.TreatmentRestrictions, item)
	}
	if err := blockRows.Err(); err != nil {
		blockRows.Close()
		return privacy.PortabilityPackage{}, err
	}
	blockRows.Close()

	reqRows, err := tx.Query(ctx, `
		SELECT id::text, tipo, estado, solicitada_en, resuelta_en, respuesta_titular,
		       motivo_resolucion_codigo, detalle_titular, vence_en, prorrogada_hasta, prorroga_motivo
		FROM public.solicitud_titular
		WHERE usuario_id = $1
		ORDER BY solicitada_en, id
	`, accountID)
	if err != nil {
		return privacy.PortabilityPackage{}, err
	}
	idxByID := map[string]int{}
	ids := []string{}
	for reqRows.Next() {
		var item privacy.RightsRequest
		if err := reqRows.Scan(
			&item.ID, &item.Kind, &item.State, &item.CreatedAt, &item.ResolvedAt, &item.Response,
			&item.DecisionCode, &item.Detail, &item.DueAt, &item.ExtendedDueAt, &item.ExtensionReason,
		); err != nil {
			reqRows.Close()
			return privacy.PortabilityPackage{}, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.ResolvedAt = utcPointer(item.ResolvedAt)
		item.DueAt = utcPointer(item.DueAt)
		item.ExtendedDueAt = utcPointer(item.ExtendedDueAt)
		idxByID[item.ID] = len(pkg.Requests)
		ids = append(ids, item.ID)
		pkg.Requests = append(pkg.Requests, item)
	}
	if err := reqRows.Err(); err != nil {
		reqRows.Close()
		return privacy.PortabilityPackage{}, err
	}
	reqRows.Close()

	if len(ids) > 0 {
		evRows, err := tx.Query(ctx, `
			SELECT id::text, solicitud_id::text, secuencia, tipo_evento, estado_anterior, estado_nuevo,
			       detalle, vence_en_nuevo, creado_en
			FROM public.solicitud_titular_evento
			WHERE solicitud_id = ANY($1::uuid[]) AND visibilidad = 'titular'
			ORDER BY solicitud_id, secuencia ASC
		`, ids)
		if err != nil {
			return privacy.PortabilityPackage{}, err
		}
		for evRows.Next() {
			var reqID string
			var ev privacy.RightsRequestEvent
			if err := evRows.Scan(&ev.ID, &reqID, &ev.Sequence, &ev.EventType, &ev.StateFrom, &ev.StateTo, &ev.Detail, &ev.NewDueAt, &ev.CreatedAt); err != nil {
				evRows.Close()
				return privacy.PortabilityPackage{}, err
			}
			ev.NewDueAt = utcPointer(ev.NewDueAt)
			ev.CreatedAt = ev.CreatedAt.UTC()
			if idx, ok := idxByID[reqID]; ok {
				pkg.Requests[idx].Events = append(pkg.Requests[idx].Events, ev)
			}
		}
		if err := evRows.Err(); err != nil {
			evRows.Close()
			return privacy.PortabilityPackage{}, err
		}
		evRows.Close()
	}

	return pkg, nil
}

func truncateRunesTrimmed(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxRunes]))
}
