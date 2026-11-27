package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ privacy.NoticeOutboxRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) ClaimPrivacyNotice(ctx context.Context, now time.Time) (privacy.PrivacyNoticeJob, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return privacy.PrivacyNoticeJob{}, false, privacy.ErrInvalid
	}
	at := now.UTC().Truncate(time.Microsecond)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox
		SET estado = 'terminal',
		    terminal_en = $1,
		    reclamado_en = NULL,
		    ultimo_error_codigo = 'attempt_limit_reached'
		WHERE intentos >= 6
		  AND ((estado = 'pending' AND disponible_en <= $1)
		    OR (estado = 'processing' AND reclamado_en <= $1 - interval '5 minutes'))`, at); err != nil {
		return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
	}

	var job privacy.PrivacyNoticeJob
	var attempts int16
	err = tx.QueryRow(ctx, `SELECT
		id::text, solicitud_id::text, tipo_aviso, clave_idempotencia,
		destinatario_correo, asunto, cuerpo_texto
	FROM public.privacidad_notificacion_outbox
	WHERE intentos < 6
	  AND ((estado = 'pending' AND disponible_en <= $1)
	    OR (estado = 'processing' AND reclamado_en <= $1 - interval '5 minutes'))
	ORDER BY disponible_en ASC, creado_en ASC, id ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1`, at).Scan(
		&job.ID, &job.RequestID, &job.NoticeType, &job.IdempotencyKey,
		&job.RecipientEmail, &job.Subject, &job.Body,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
		}
		return privacy.PrivacyNoticeJob{}, false, nil
	}
	if err != nil {
		return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
	}

	if err := tx.QueryRow(ctx, `UPDATE public.privacidad_notificacion_outbox
		SET estado = 'processing',
		    reclamado_en = $2,
		    intentos = intentos + 1
		WHERE id = $1
		RETURNING intentos`, job.ID, at).Scan(&attempts); err != nil {
		return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
	}
	job.Attempt = int(attempts)

	if err := tx.Commit(ctx); err != nil {
		return privacy.PrivacyNoticeJob{}, false, mapPrivacyError(err)
	}
	return job, true, nil
}

func (r *IdentityRepository) FinishPrivacyNotice(ctx context.Context, job privacy.PrivacyNoticeJob, delivered bool, errorCode string, now time.Time) error {
	if r == nil || r.pool == nil || ctx == nil || job.ID == "" || job.Attempt < 1 {
		return privacy.ErrInvalid
	}
	at := now.UTC().Truncate(time.Microsecond)

	var (
		tag pgconn.CommandTag
		err error
	)
	switch {
	case delivered:
		tag, err = r.pool.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox
			SET estado = 'delivered',
			    entregado_en = $3,
			    reclamado_en = NULL,
			    ultimo_error_codigo = NULL
			WHERE id = $1 AND estado = 'processing' AND intentos = $2`,
			job.ID, job.Attempt, at)
	case job.Attempt < 6:
		backoff := time.Duration(job.Attempt) * time.Minute
		errCode := "mail_delivery_failed"
		if errorCode == "mail_delivery_failed" {
			errCode = errorCode
		}
		tag, err = r.pool.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox
			SET estado = 'pending',
			    reclamado_en = NULL,
			    disponible_en = $3,
			    ultimo_error_codigo = $4
			WHERE id = $1 AND estado = 'processing' AND intentos = $2`,
			job.ID, job.Attempt, at.Add(backoff), errCode)
	default:
		tag, err = r.pool.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox
			SET estado = 'terminal',
			    terminal_en = $3,
			    reclamado_en = NULL,
			    ultimo_error_codigo = 'attempt_limit_reached'
			WHERE id = $1 AND estado = 'processing' AND intentos = $2`,
			job.ID, job.Attempt, at)
	}
	if err != nil {
		return mapPrivacyError(err)
	}
	if tag.RowsAffected() != 1 {
		return privacy.ErrNotFound
	}
	return nil
}
