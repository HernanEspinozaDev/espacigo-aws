package postgres

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

var _ privacy.RightsExecutionRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) ListOwnPurposePreferences(ctx context.Context, accountID string) ([]privacy.PurposePreference, error) {
	if r == nil || r.pool == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return nil, privacy.ErrInvalid
	}
	rows, err := r.pool.Query(ctx, `SELECT finalidad, estado, causal, solicitud_id::text, actualizado_en
		FROM public.privacidad_preferencia_finalidad
		WHERE cuenta_id = $1`, accountID)
	if err != nil {
		return nil, mapPrivacyError(err)
	}
	defer rows.Close()

	byPurpose := map[string]privacy.PurposePreference{}
	for rows.Next() {
		var pref privacy.PurposePreference
		var updatedAt time.Time
		if err := rows.Scan(&pref.Purpose, &pref.State, &pref.Reason, &pref.RequestID, &updatedAt); err != nil {
			return nil, mapPrivacyError(err)
		}
		ts := updatedAt.UTC()
		pref.UpdatedAt = &ts
		byPurpose[pref.Purpose] = pref
	}
	if err := rows.Err(); err != nil {
		return nil, mapPrivacyError(err)
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

func (r *IdentityRepository) UpsertOwnPurposePreference(ctx context.Context, accountID string, in privacy.UpdatePurposePreferenceInput, now time.Time) (privacy.PurposePreference, error) {
	if r == nil || r.pool == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return privacy.PurposePreference{}, privacy.ErrInvalid
	}
	if !privacy.ValidPurpose(in.Purpose) || !privacy.ValidPurposeState(in.State) {
		return privacy.PurposePreference{}, privacy.ErrInvalid
	}
	trimmedReason := strings.TrimSpace(in.Reason)
	if !utf8.ValidString(trimmedReason) || strings.ContainsRune(trimmedReason, 0) {
		return privacy.PurposePreference{}, privacy.ErrInvalid
	}
	var reasonPtr *string
	if trimmedReason != "" {
		n := utf8.RuneCountInString(trimmedReason)
		if n < 5 || n > 500 {
			return privacy.PurposePreference{}, privacy.ErrInvalid
		}
		reasonPtr = &trimmedReason
	}
	ts := now.UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		ts = time.Now().UTC().Truncate(time.Microsecond)
	}
	auditID, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.PurposePreference{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.PurposePreference{}, mapPrivacyError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return privacy.PurposePreference{}, mapPrivacyError(err)
	}
	if !active {
		return privacy.PurposePreference{}, privacy.ErrNotFound
	}

	var pref privacy.PurposePreference
	var updatedAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO public.privacidad_preferencia_finalidad (
			cuenta_id, finalidad, estado, causal, solicitud_id, actualizado_por, actualizado_en
		) VALUES ($1, $2, $3, $4, NULL, $1, $5)
		ON CONFLICT (cuenta_id, finalidad) DO UPDATE
		SET estado = EXCLUDED.estado,
			causal = EXCLUDED.causal,
			solicitud_id = NULL,
			actualizado_por = EXCLUDED.actualizado_por,
			actualizado_en = EXCLUDED.actualizado_en
		RETURNING finalidad, estado, causal, solicitud_id::text, actualizado_en`,
		accountID, in.Purpose, in.State, reasonPtr, ts,
	).Scan(&pref.Purpose, &pref.State, &pref.Reason, &pref.RequestID, &updatedAt)
	if err != nil {
		return privacy.PurposePreference{}, mapPrivacyError(err)
	}
	utcUpdated := updatedAt.UTC()
	pref.UpdatedAt = &utcUpdated

	motivoCodigo := "pref_" + in.State + "_" + in.Purpose
	if _, err := tx.Exec(ctx, `INSERT INTO public.evento_auditoria_local (
			id, actor_id, recurso_tipo, recurso_id, accion, resultado, motivo_codigo,
			correlacion_id, ocurrido_en, retirar_en
		) VALUES (
			$1, $2, 'privacidad_finalidad', $2, 'privacy.purpose_preference.update', 'exito', $3,
			$4, $5::timestamptz, $5::timestamptz + interval '5 years'
		)`, auditID, accountID, motivoCodigo, auditID, ts); err != nil {
		return privacy.PurposePreference{}, mapPrivacyError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return privacy.PurposePreference{}, mapPrivacyError(err)
	}
	return pref, nil
}

func (r *IdentityRepository) ListOwnTreatmentRestrictions(ctx context.Context, accountID string) ([]privacy.TreatmentRestriction, error) {
	if r == nil || r.pool == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return nil, privacy.ErrInvalid
	}
	rows, err := r.pool.Query(ctx, `SELECT
			id::text, solicitud_id::text, ambito, estado, motivo,
			aplicado_en, revisar_en, levantado_en, motivo_levantamiento
		FROM public.privacidad_bloqueo_tratamiento
		WHERE cuenta_id = $1
		ORDER BY aplicado_en DESC, id DESC`, accountID)
	if err != nil {
		return nil, mapPrivacyError(err)
	}
	defer rows.Close()

	items := []privacy.TreatmentRestriction{}
	for rows.Next() {
		var item privacy.TreatmentRestriction
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.Scope, &item.State, &item.Reason,
			&item.AppliedAt, &item.ReviewAt, &item.LiftedAt, &item.LiftReason,
		); err != nil {
			return nil, mapPrivacyError(err)
		}
		item.AppliedAt = item.AppliedAt.UTC()
		item.ReviewAt = utcTimePtr(item.ReviewAt)
		item.LiftedAt = utcTimePtr(item.LiftedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPrivacyError(err)
	}
	return items, nil
}

func (r *IdentityRepository) HasActiveTreatmentBlock(ctx context.Context, accountID, scope string) (bool, error) {
	if r == nil || r.pool == nil || ctx == nil || strings.TrimSpace(accountID) == "" || !privacy.ValidTreatmentScope(scope) {
		return false, privacy.ErrInvalid
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM public.privacidad_bloqueo_tratamiento
		WHERE cuenta_id = $1
		  AND estado = 'activo'
		  AND ambito IN ($2, 'tratamiento_general')
	)`, accountID, scope).Scan(&exists)
	if err != nil {
		return false, mapPrivacyError(err)
	}
	return exists, nil
}
