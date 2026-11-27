package kybpg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// EnqueueDueReminders persists in-app notices and email intents. It never
// sends email; a separate dispatcher owns delivery.
func (r *Repository) EnqueueDueReminders(ctx context.Context, at time.Time, limit int) (int, error) {
	if r == nil || r.pool == nil || at.IsZero() || limit < 1 || limit > 500 {
		return 0, kyb.ErrInvalid
	}
	var count int
	err := r.pool.QueryRow(ctx, `SELECT public.kyb_enqueue_due_reminders($1,$2)`, at.UTC(), limit).Scan(&count)
	return count, mapError(err)
}

// ClaimReminderEmail leases one queue item for a mailer supplied by runtime.
func (r *Repository) ClaimReminderEmail(ctx context.Context, at time.Time) (kyb.ComplianceReminderEmail, bool, error) {
	if r == nil || r.pool == nil || at.IsZero() {
		return kyb.ComplianceReminderEmail{}, false, kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox SET state='terminal',terminal_at=$1,last_error_code='attempt_limit_reached',claimed_at=NULL WHERE attempts>=8 AND (state='pending' OR (state='processing' AND claimed_at<=$1-interval '15 minutes'))`, at.UTC()); err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	// Permanently cancel intents whose account, identity, membership, or profile
	// can no longer receive this notice.
	if _, err = tx.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox n SET state='cancelled',cancelled_at=$1,last_error_code='recipient_unavailable',claimed_at=NULL
		WHERE n.state IN ('pending','processing') AND (
		  NOT EXISTS (SELECT 1 FROM public.usuario u WHERE u.id=n.recipient_id AND u.estado='activo'
		    AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(u.id,$1) k WHERE k.verified))
		  OR (n.profile_type='personal' AND (n.recipient_id<>n.profile_id OR NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_personal p WHERE p.usuario_id=n.profile_id AND (p.compliance_status IN ('grace_active','in_review') OR (p.compliance_status='disabled_missing_docs' AND p.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND n.reminder_stage='due') OR (p.compliance_status='compliant' AND n.reminder_stage='reactivated')))))
		  OR (n.profile_type='organization' AND (NOT EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=n.profile_id AND m.usuario_id=n.recipient_id AND m.estado='activo' AND m.rol IN ('representante_legal','administrador')) OR NOT EXISTS (SELECT 1 FROM public.organizacion o WHERE o.id=n.profile_id AND (o.compliance_status IN ('grace_active','in_review') OR (o.compliance_status='disabled_missing_docs' AND o.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND n.reminder_stage='due') OR (o.compliance_status='compliant' AND n.reminder_stage='reactivated')))))
		)`, at.UTC()); err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	// If a later stage already exists for this profile and recipient, older queued
	// warnings are obsolete and must not be delivered after a worker outage.
	if _, err = tx.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox n SET state='cancelled',cancelled_at=$1,last_error_code='recipient_unavailable',claimed_at=NULL
		WHERE n.state='pending' AND EXISTS (SELECT 1 FROM public.kyb_compliance_notice_outbox newer
		  WHERE newer.profile_type=n.profile_type AND newer.profile_id=n.profile_id AND newer.recipient_id=n.recipient_id
		    AND CASE newer.reminder_stage WHEN 'day0' THEN 0 WHEN 'month1' THEN 1 WHEN 'day15' THEN 2 WHEN 'day7' THEN 3 WHEN 'day1' THEN 4 WHEN 'due' THEN 5 ELSE 6 END
		      > CASE n.reminder_stage WHEN 'day0' THEN 0 WHEN 'month1' THEN 1 WHEN 'day15' THEN 2 WHEN 'day7' THEN 3 WHEN 'day1' THEN 4 WHEN 'due' THEN 5 ELSE 6 END
		    AND newer.state IN ('pending','processing','delivered'))`, at.UTC()); err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	var item kyb.ComplianceReminderEmail
	var stage string
	err = tx.QueryRow(ctx, `WITH candidate AS (
		SELECT n.id FROM public.kyb_compliance_notice_outbox n
		JOIN public.usuario u ON u.id=n.recipient_id AND u.estado='activo'
          AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(u.id,$1) k WHERE k.verified)
		LEFT JOIN public.anfitrion_cumplimiento_personal p ON n.profile_type='personal' AND p.usuario_id=n.profile_id
		LEFT JOIN public.organizacion o ON n.profile_type='organization' AND o.id=n.profile_id
		WHERE ((n.state='pending' AND n.available_at<=$1) OR (n.state='processing' AND n.claimed_at<=$1-interval '15 minutes'))
		  AND (n.profile_type<>'organization' OR EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=n.profile_id AND m.usuario_id=n.recipient_id AND m.estado='activo' AND m.rol IN ('representante_legal','administrador')))
		  AND ((n.profile_type='personal' AND (p.compliance_status IN ('grace_active','in_review') OR (p.compliance_status='disabled_missing_docs' AND p.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND n.reminder_stage='due') OR (p.compliance_status='compliant' AND n.reminder_stage='reactivated')))
		       OR (n.profile_type='organization' AND (o.compliance_status IN ('grace_active','in_review') OR (o.compliance_status='disabled_missing_docs' AND o.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND n.reminder_stage='due') OR (o.compliance_status='compliant' AND n.reminder_stage='reactivated'))))
		ORDER BY n.available_at,n.created_at,n.id LIMIT 1 FOR UPDATE OF n SKIP LOCKED
	), claimed AS (
		UPDATE public.kyb_compliance_notice_outbox n SET state='processing',claimed_at=$1,attempts=n.attempts+1
		FROM candidate c WHERE n.id=c.id RETURNING n.*
	)
	SELECT c.id::text,c.profile_type,c.profile_id::text,c.recipient_id::text,u.correo_original,c.reminder_stage,
	       COALESCE(p.grace_due_at,o.grace_due_at,c.created_at),c.attempts
	FROM claimed c JOIN public.usuario u ON u.id=c.recipient_id
	LEFT JOIN public.anfitrion_cumplimiento_personal p ON c.profile_type='personal' AND p.usuario_id=c.profile_id
	LEFT JOIN public.organizacion o ON c.profile_type='organization' AND o.id=c.profile_id`, at.UTC()).
		Scan(&item.ID, &item.ProfileType, &item.ProfileID, &item.RecipientID, &item.RecipientEmail, &stage, &item.DueAt, &item.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(ctx); err != nil {
			return kyb.ComplianceReminderEmail{}, false, mapError(err)
		}
		return kyb.ComplianceReminderEmail{}, false, nil
	}
	if err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	item.Stage = kyb.ReminderStage(stage)
	if err = tx.Commit(ctx); err != nil {
		return kyb.ComplianceReminderEmail{}, false, mapError(err)
	}
	return item, true, nil
}

func (r *Repository) FinishReminderEmail(ctx context.Context, item kyb.ComplianceReminderEmail, delivered bool, code string, at time.Time) error {
	if r == nil || r.pool == nil || !kyb.ValidUUID(item.ID) || item.Attempt < 1 || at.IsZero() {
		return kyb.ErrInvalid
	}
	var tag pgconn.CommandTag
	var err error
	if delivered {
		tag, err = r.pool.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox SET state='delivered',delivered_at=$3,claimed_at=NULL,last_error_code=NULL WHERE id=$1 AND state='processing' AND attempts=$2`, item.ID, item.Attempt, at.UTC())
	} else if item.Attempt >= 8 {
		tag, err = r.pool.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox SET state='terminal',terminal_at=$3,claimed_at=NULL,last_error_code='attempt_limit_reached' WHERE id=$1 AND state='processing' AND attempts=$2`, item.ID, item.Attempt, at.UTC())
	} else {
		if code != "recipient_unavailable" && code != "mail_delivery_failed" {
			code = "mail_delivery_failed"
		}
		delay := time.Duration(1<<min(item.Attempt, 6)) * time.Minute
		tag, err = r.pool.Exec(ctx, `UPDATE public.kyb_compliance_notice_outbox SET state='pending',available_at=$3,claimed_at=NULL,last_error_code=$4 WHERE id=$1 AND state='processing' AND attempts=$2`, item.ID, item.Attempt, at.UTC().Add(delay), code)
	}
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: reminder email lease lost", kyb.ErrConflict)
	}
	return nil
}
