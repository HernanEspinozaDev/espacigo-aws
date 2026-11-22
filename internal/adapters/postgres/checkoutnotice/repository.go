package checkoutnoticepg

import (
	"context"
	"errors"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/checkoutnotice"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Claim(ctx context.Context, at time.Time) (domain.Notice, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return domain.Notice{}, false, domain.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Notice{}, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `UPDATE public.checkout_payment_notice_outbox
		SET state='terminal',terminal_at=$1,last_error_code='attempt_limit_reached',claimed_at=NULL
		WHERE attempts>=8 AND ((state='pending' AND available_at<=$1) OR (state='processing' AND claimed_at<=$1-interval '60 seconds'))`, at); err != nil {
		return domain.Notice{}, false, err
	}
	var id, userID, userState string
	var email pgtype.Text
	err = tx.QueryRow(ctx, `SELECT o.id::text,o.recipient_id::text,u.estado,u.correo_original
		FROM public.checkout_payment_notice_outbox o
		JOIN public.usuario u ON u.id=o.recipient_id
		WHERE o.attempts<8 AND ((o.state='pending' AND o.available_at<=$1) OR (o.state='processing' AND o.claimed_at<=$1-interval '60 seconds'))
		ORDER BY o.available_at,o.created_at,o.id
		FOR UPDATE OF o SKIP LOCKED LIMIT 1`, at).Scan(&id, &userID, &userState, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(ctx); err != nil {
			return domain.Notice{}, false, err
		}
		return domain.Notice{}, false, nil
	}
	if err != nil {
		return domain.Notice{}, false, err
	}
	if userState != "activo" || !email.Valid || email.String == "" {
		_, err = tx.Exec(ctx, `UPDATE public.checkout_payment_notice_outbox SET state='cancelled',last_error_code='recipient_unavailable',claimed_at=NULL WHERE id=$1`, id)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return domain.Notice{}, false, err
	}
	var n domain.Notice
	n.Email = email.String
	err = tx.QueryRow(ctx, `UPDATE public.checkout_payment_notice_outbox o
		SET state='processing',attempts=attempts+1,claimed_at=$2
		FROM public.reserva_arriendo_comercial r
		JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
		WHERE o.id=$1 AND r.id=o.reservation_id
		RETURNING o.id::text,r.id::text,q.id::text,o.recipient_role,o.transition,q.titulo_espacio,
			r.subtotal_clp,r.inicio,r.termino,r.zona_horaria,o.attempts`, id, at).Scan(
		&n.ID, &n.ReservationID, &n.QuoteID, &n.RecipientRole, &n.Transition, &n.SpaceTitle,
		&n.AmountCLP, &n.StartAt, &n.EndAt, &n.TimeZone, &n.Attempts,
	)
	if err != nil {
		return domain.Notice{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Notice{}, false, err
	}
	return n, true, nil
}

func (r *Repository) Finish(ctx context.Context, n domain.Notice, success bool, code string, at time.Time) error {
	if r == nil || r.pool == nil || n.ID == "" || n.Attempts < 1 {
		return domain.ErrInvalid
	}
	var (
		err error
		tag pgconn.CommandTag
	)
	if success {
		tag, err = r.pool.Exec(ctx, `UPDATE public.checkout_payment_notice_outbox
			SET state='delivered',delivered_at=$3,claimed_at=NULL,last_error_code=NULL
			WHERE id=$1 AND state='processing' AND attempts=$2`, n.ID, n.Attempts, at)
	} else if n.Attempts >= 8 {
		tag, err = r.pool.Exec(ctx, `UPDATE public.checkout_payment_notice_outbox
			SET state='terminal',terminal_at=$3,claimed_at=NULL,last_error_code=$4
			WHERE id=$1 AND state='processing' AND attempts=$2`, n.ID, n.Attempts, at, errorCode(code))
	} else {
		delay := time.Second << uint(min(n.Attempts-1, 7))
		tag, err = r.pool.Exec(ctx, `UPDATE public.checkout_payment_notice_outbox
			SET state='pending',available_at=$3,claimed_at=NULL,last_error_code=$4
			WHERE id=$1 AND state='processing' AND attempts=$2`, n.ID, n.Attempts, at.Add(delay), errorCode(code))
	}
	if err == nil && tag.RowsAffected() != 1 {
		return domain.ErrInvalid
	}
	return err
}

func errorCode(code string) string {
	if code == "recipient_unavailable" || code == "attempt_limit_reached" {
		return code
	}
	return "mail_delivery_failed"
}
