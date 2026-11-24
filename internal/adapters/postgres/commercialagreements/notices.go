package commercialagreementspg

import (
	"context"
	"errors"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNotice(ctx context.Context, at time.Time) (domain.Notice, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Notice{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE public.contrato_solicitud_notice_outbox SET estado='terminal',ultimo_error='attempt_limit_reached',reclamada_en=NULL WHERE intentos>=8 AND (estado='pending' OR (estado='processing' AND reclamada_en<=$1-interval '60 seconds'))`, at); err != nil {
		return domain.Notice{}, false, err
	}
	var n domain.Notice
	var state string
	var email *string
	err = tx.QueryRow(ctx, `SELECT o.id::text,o.solicitud_id::text,u.estado,u.correo_original,o.intentos FROM public.contrato_solicitud_notice_outbox o JOIN public.usuario u ON u.id=o.destinatario_id WHERE o.intentos<8 AND ((o.estado='pending' AND o.disponible_en<=$1) OR (o.estado='processing' AND o.reclamada_en<=$1-interval '60 seconds')) ORDER BY o.disponible_en,o.creada_en,o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1`, at).Scan(&n.ID, &n.AgreementID, &state, &email, &n.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.Commit(ctx)
		return domain.Notice{}, false, err
	}
	if err != nil {
		return n, false, err
	}
	if state != "activo" || email == nil || *email == "" {
		_, err = tx.Exec(ctx, `UPDATE public.contrato_solicitud_notice_outbox SET estado='cancelled',ultimo_error='recipient_unavailable',reclamada_en=NULL WHERE id=$1`, n.ID)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return domain.Notice{}, false, err
	}
	n.Email = *email
	n.Attempt++
	_, err = tx.Exec(ctx, `UPDATE public.contrato_solicitud_notice_outbox SET estado='processing',intentos=$2,reclamada_en=$3 WHERE id=$1`, n.ID, n.Attempt, at)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return n, err == nil, err
}
func (r *Repository) FinishNotice(ctx context.Context, n domain.Notice, success bool, at time.Time) error {
	if n.Attempt < 1 || n.Attempt > 8 {
		return domain.ErrInvalid
	}
	state, code := "pending", "delivery_failed"
	var delivered *time.Time
	if success {
		state, code = "delivered", ""
		delivered = &at
	} else if n.Attempt >= 8 {
		state, code = "terminal", "attempt_limit_reached"
	}
	delay := time.Second << uint(min(n.Attempt-1, 7))
	tag, err := r.db.Exec(ctx, `UPDATE public.contrato_solicitud_notice_outbox SET estado=$3,ultimo_error=NULLIF($4,''),entregada_en=$5,disponible_en=$6,reclamada_en=NULL WHERE id=$1 AND estado='processing' AND intentos=$2`, n.ID, n.Attempt, state, code, delivered, at.Add(delay))
	if err == nil && tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return err
}
