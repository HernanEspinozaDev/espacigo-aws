package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
)

var _ privacy.PublicIntakeRepository = (*IdentityRepository)(nil)

func mapPrivacyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return privacy.ErrNotFound
	}
	mapped := mapError(err)
	if errors.Is(mapped, identity.ErrInvalid) {
		return privacy.ErrInvalid
	}
	if errors.Is(mapped, identity.ErrNotFound) {
		return privacy.ErrNotFound
	}
	return mapped
}

func optionalTrimmed(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func utcTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func (r *IdentityRepository) CreatePublicRightsRequest(ctx context.Context, in privacy.PublicRightsRequestInput, folio string, trackingHash []byte, now, dueAt time.Time) (privacy.PublicRightsReceipt, error) {
	if r == nil || r.pool == nil || ctx == nil || len(trackingHash) != 32 {
		return privacy.PublicRightsReceipt{}, privacy.ErrInvalid
	}
	id, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.PublicRightsReceipt{}, err
	}
	eventID, err := (credentials.Generator{}).ID()
	if err != nil {
		return privacy.PublicRightsReceipt{}, err
	}
	createdAt := now.UTC().Truncate(time.Microsecond)
	dueAtUTC := dueAt.UTC().Truncate(time.Microsecond)

	var repName, repRel *string
	if in.RequesterType == "representante" {
		repName = optionalTrimmed(in.RepresentativeName)
		repRel = optionalTrimmed(in.RepresentativeRelation)
	}
	subjectRef := optionalTrimmed(in.SubjectReference)
	detail := strings.TrimSpace(in.Detail)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.PublicRightsReceipt{}, mapPrivacyError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var receipt privacy.PublicRightsReceipt
	err = tx.QueryRow(ctx, `INSERT INTO public.solicitud_titular (
		id, usuario_id, folio_publico, tipo, canal, estado,
		solicitante_tipo, solicitante_nombre, solicitante_correo,
		representante_nombre, representante_relacion, titular_referencia,
		consulta_token_hash, detalle_titular, solicitada_en, vence_en
	) VALUES (
		$1, NULL, $2, $3, 'publico', 'identidad_pendiente',
		$4, $5, $6,
		$7, $8, $9,
		$10, $11, $12, $13
	)
	RETURNING id::text, folio_publico, tipo, solicitante_tipo, estado, canal, solicitada_en, vence_en`,
		id, folio, in.RightType,
		in.RequesterType, strings.TrimSpace(in.RequesterName), strings.TrimSpace(in.RequesterEmail),
		repName, repRel, subjectRef,
		trackingHash, detail, createdAt, dueAtUTC,
	).Scan(
		&receipt.ID, &receipt.Folio, &receipt.RightType, &receipt.RequesterType,
		&receipt.State, &receipt.Channel, &receipt.CreatedAt, &receipt.DueAt,
	)
	if err != nil {
		return privacy.PublicRightsReceipt{}, mapPrivacyError(err)
	}
	receipt.CreatedAt = receipt.CreatedAt.UTC()
	receipt.DueAt = receipt.DueAt.UTC()

	if _, err := tx.Exec(ctx, `INSERT INTO public.solicitud_titular_evento (
		id, solicitud_id, secuencia, tipo_evento, visibilidad, estado_anterior, estado_nuevo, actor_id, detalle, vence_en_nuevo, creado_en
	) VALUES ($1, $2, 1, 'recepcion', 'titular', NULL, 'identidad_pendiente', NULL, NULL, $3, $4)`,
		eventID, receipt.ID, receipt.DueAt, receipt.CreatedAt,
	); err != nil {
		return privacy.PublicRightsReceipt{}, mapPrivacyError(err)
	}

	subject, body := privacy.FormatAcuseRecepcionNotice(receipt.Folio, receipt.RightType, receipt.DueAt)
	if _, err := tx.Exec(ctx, `INSERT INTO public.privacidad_notificacion_outbox (
		solicitud_id, tipo_aviso, clave_idempotencia, destinatario_correo, asunto, cuerpo_texto, disponible_en, creado_en
	) VALUES ($1, 'acuse_recepcion', $2, $3, $4, $5, $6, $6)`,
		receipt.ID, "acuse:"+receipt.ID, strings.TrimSpace(in.RequesterEmail), subject, body, receipt.CreatedAt,
	); err != nil {
		return privacy.PublicRightsReceipt{}, mapPrivacyError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return privacy.PublicRightsReceipt{}, mapPrivacyError(err)
	}
	return receipt, nil
}

func (r *IdentityRepository) LookupPublicRightsRequest(ctx context.Context, folio string, trackingHash []byte) (privacy.PublicRightsStatus, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return privacy.PublicRightsStatus{}, privacy.ErrInvalid
	}
	if len(trackingHash) != 32 {
		return privacy.PublicRightsStatus{}, privacy.ErrNotFound
	}

	var requestID string
	var status privacy.PublicRightsStatus
	var storedHash []byte
	err := r.pool.QueryRow(ctx, `SELECT
		id::text, folio_publico, tipo, solicitante_tipo, estado,
		solicitada_en, vence_en, prorrogada_hasta, prorroga_motivo,
		resuelta_en, respuesta_titular, consulta_token_hash
	FROM public.solicitud_titular
	WHERE folio_publico = $1`, folio).Scan(
		&requestID, &status.Folio, &status.RightType, &status.RequesterType, &status.State,
		&status.CreatedAt, &status.DueAt, &status.ExtendedDueAt, &status.ExtensionReason,
		&status.ResolvedAt, &status.Response, &storedHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privacy.PublicRightsStatus{}, privacy.ErrNotFound
		}
		return privacy.PublicRightsStatus{}, mapPrivacyError(err)
	}
	if len(storedHash) != 32 || subtle.ConstantTimeCompare(storedHash, trackingHash) != 1 {
		return privacy.PublicRightsStatus{}, privacy.ErrNotFound
	}

	status.CreatedAt = status.CreatedAt.UTC()
	status.DueAt = utcTimePtr(status.DueAt)
	status.ExtendedDueAt = utcTimePtr(status.ExtendedDueAt)
	status.ResolvedAt = utcTimePtr(status.ResolvedAt)
	status.Events = []privacy.RightsRequestEvent{}

	rows, err := r.pool.Query(ctx, `SELECT
		id::text, secuencia, tipo_evento, estado_anterior, estado_nuevo, detalle, vence_en_nuevo, creado_en
	FROM public.solicitud_titular_evento
	WHERE solicitud_id = $1 AND visibilidad = 'titular'
	ORDER BY secuencia ASC`, requestID)
	if err != nil {
		return privacy.PublicRightsStatus{}, mapPrivacyError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var ev privacy.RightsRequestEvent
		if err := rows.Scan(&ev.ID, &ev.Sequence, &ev.EventType, &ev.StateFrom, &ev.StateTo, &ev.Detail, &ev.NewDueAt, &ev.CreatedAt); err != nil {
			return privacy.PublicRightsStatus{}, mapPrivacyError(err)
		}
		ev.CreatedAt = ev.CreatedAt.UTC()
		ev.NewDueAt = utcTimePtr(ev.NewDueAt)
		status.Events = append(status.Events, ev)
	}
	if err := rows.Err(); err != nil {
		return privacy.PublicRightsStatus{}, mapPrivacyError(err)
	}
	return status, nil
}
