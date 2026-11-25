package rentalflowpg

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialpolicy"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PreviewCommercialCancellation(ctx context.Context, actor, reservationID string, now time.Time) (rentalflow.CommercialCancellation, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) || !rentalflow.ValidUUID(reservationID) || now.IsZero() {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrInvalid
	}
	return r.readCommercialCancellation(ctx, r.db, strings.ToLower(actor), strings.ToLower(reservationID), now.UTC(), false)
}

type cancellationQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *Repository) readCommercialCancellation(ctx context.Context, q cancellationQuerier, actor, reservationID string, now time.Time, lock bool) (rentalflow.CommercialCancellation, error) {
	locking := ""
	if lock {
		locking = " FOR UPDATE OF r,c,o"
	}
	query := `SELECT r.id::text,r.estado,c.estado,COALESCE(c.provider_payment_id,''),c.total_charged_clp,o.activo,c.payment_verified_at,
 r.inicio,r.anfitrion_id::text,r.arrendatario_id::text,
 COALESCE(x.politica_version,q.politica_cancelacion_version,''),COALESCE(x.politica_texto,q.politica_cancelacion_texto,''),
 CASE WHEN r.anfitrion_id=$2 THEN 'anfitrion' ELSE 'arrendatario' END,
 COALESCE(x.refund_status,''),COALESCE(x.porcentaje_reembolso,0),COALESCE(x.monto_reembolso_clp,0),
 COALESCE(x.actor_id::text,''),COALESCE(x.motivo,''),COALESCE(x.clave_idempotencia,''),COALESCE(x.provider_refund_id,'')
 FROM public.reserva_arriendo_comercial r
 JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
 JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
 JOIN public.ocupacion o ON o.id=r.ocupacion_id
 LEFT JOIN public.reserva_comercial_cancelacion x ON x.reserva_id=r.id
 WHERE r.id=$1::uuid AND (r.anfitrion_id=$2::uuid OR r.arrendatario_id=$2::uuid)` + locking
	var out rentalflow.CommercialCancellation
	var reservationState, checkoutState, paymentID, hostID, renterID, version, policyText, role, existingStatus, existingActor, existingReason, idempotency, providerRefundID string
	var start time.Time
	var total int64
	var occupancyActive bool
	var paymentVerifiedAt sql.NullTime
	var oldPercent int
	var oldAmount int64
	err := q.QueryRow(ctx, query, reservationID, actor).Scan(&out.ReservationID, &reservationState, &checkoutState, &paymentID, &total, &occupancyActive, &paymentVerifiedAt,
		&start, &hostID, &renterID, &version, &policyText, &role,
		&existingStatus, &oldPercent, &oldAmount, &existingActor, &existingReason, &idempotency, &providerRefundID)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrNotFound
	}
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	out.Currency = "CLP"
	out.TotalChargedCLP = total
	out.StartsAt = start.UTC().Format(time.RFC3339Nano)
	out.ProviderPaymentID = paymentID
	out.ProviderRefundID = providerRefundID
	out.HostID = hostID
	out.ActorID = actor
	if existingStatus != "" {
		out.CanCancel = false
		out.Reason = "cancellation_started"
		if existingStatus == "fallido" {
			out.CanCancel = reservationState == "pagada" && now.Before(start) && paymentVerifiedAt.Valid && !now.After(paymentVerifiedAt.Time.Add(180*24*time.Hour))
			out.Reason = "refund_failed"
			if !out.CanCancel {
				out.Reason = "refund_window_expired"
			}
		}
		out.RefundPercent = oldPercent
		out.RefundAmountCLP = oldAmount
		out.Status = reservationState
		out.RefundStatus = existingStatus
		out.PolicyVersion, out.PolicyText = version, policyText
		out.IdempotencyKey = idempotency
		out.Motivo = existingReason
		return out, nil
	}
	if version == "" {
		version, policyText = "sin_snapshot_v1", "Sin snapshot de política; no corresponde devolución automática."
	} else if _, ok := commercialpolicy.ByVersion(version); !ok {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrUnavailable
	}
	out.PolicyVersion, out.PolicyText = version, policyText
	if reservationState != "pagada" || checkoutState != "approved" || !occupancyActive {
		out.Reason = "reservation_not_paid"
		return out, nil
	}
	if !now.Before(start) {
		out.Reason = "reservation_already_started"
		return out, nil
	}
	out.CanCancel = true
	out.Reason = "eligible"
	percent := 0
	if role == "anfitrion" {
		percent = 100
		out.Reason = "host_cancellation"
	} else {
		switch version {
		case "facil_v1":
			percent = 100
		case "flexible_v1":
			if !now.After(start.Add(-24 * time.Hour)) {
				percent = 100
			}
		case "moderada_v1":
			if !now.After(start.Add(-48 * time.Hour)) {
				percent = 50
			}
		case "estricta_v1":
			percent = 0
		case "sin_snapshot_v1":
			percent = 0
		}
	}
	out.RefundPercent = percent
	out.RefundAmountCLP = total/100*int64(percent) + (total % 100 * int64(percent) / 100)
	if percent == 0 {
		out.Reason = "eligible_no_refund"
	} else if !paymentVerifiedAt.Valid || now.After(paymentVerifiedAt.Time.Add(180*24*time.Hour)) {
		out.CanCancel = false
		out.Reason = "refund_window_expired"
	}
	return out, nil
}

func (r *Repository) BeginCommercialCancellation(ctx context.Context, actor, reservationID, reason string, expectedRefundAmountCLP int64, now time.Time, requestID string) (rentalflow.CommercialCancellation, error) {
	if r == nil || r.beginner == nil || ctx == nil || !rentalflow.ValidUUID(actor) || !rentalflow.ValidUUID(reservationID) || !rentalflow.ValidUUID(requestID) || len(strings.TrimSpace(reason)) < 1 || len(reason) > 500 || expectedRefundAmountCLP < 0 || now.IsZero() {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrInvalid
	}
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	preview, err := r.readCommercialCancellation(ctx, tx, strings.ToLower(actor), strings.ToLower(reservationID), now.UTC(), true)
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	if preview.RefundStatus != "" {
		if preview.RefundAmountCLP != expectedRefundAmountCLP {
			return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
		}
		// Replays are bound to the same actor and reason; they reuse the durable key.
		var savedActor, savedReason string
		if err = tx.QueryRow(ctx, `SELECT actor_id::text,motivo FROM public.reserva_comercial_cancelacion WHERE reserva_id=$1::uuid`, reservationID).Scan(&savedActor, &savedReason); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		if !strings.EqualFold(savedActor, actor) || savedReason != strings.TrimSpace(reason) {
			return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
		}
		if preview.RefundStatus == "fallido" {
			if !preview.CanCancel {
				return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
			}
			_, err = tx.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET refund_status='pendiente',clave_idempotencia=$3,provider_refund_id=NULL,intentos=intentos+1,actualizada_en=$2 WHERE reserva_id=$1::uuid`, reservationID, now.UTC(), requestID)
			if err != nil {
				return rentalflow.CommercialCancellation{}, err
			}
			preview.RefundStatus = "pendiente"
			preview.IdempotencyKey = requestID
			preview.ProviderRefundID = ""
		} else if preview.RefundStatus == "pendiente" {
			// Move claimed work to the back of the ordered queue even when an
			// external token or provider call is temporarily unavailable.
			if _, err = tx.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET actualizada_en=$2 WHERE reserva_id=$1::uuid AND refund_status='pendiente'`, reservationID, now.UTC()); err != nil {
				return rentalflow.CommercialCancellation{}, err
			}
			preview.RefundStatus = "pendiente"
		}
		if err = tx.Commit(ctx); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		return preview, nil
	}
	if !preview.CanCancel {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
	}
	if preview.RefundAmountCLP != expectedRefundAmountCLP {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
	}
	preview.IdempotencyKey = uuid.NewString()
	preview.Motivo = strings.TrimSpace(reason)
	preview.RefundStatus = "pendiente"
	preview.Status = "pagada"
	role := "arrendatario"
	if strings.EqualFold(actor, preview.HostID) {
		role = "anfitrion"
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.reserva_comercial_cancelacion
(id,reserva_id,actor_id,rol_actor,politica_version,politica_texto,motivo,porcentaje_reembolso,monto_total_clp,monto_reembolso_clp,clave_idempotencia,refund_status,intentos,creada_en,actualizada_en)
VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,'pendiente',1,$12,$12)`, requestID, reservationID, actor, role, preview.PolicyVersion, preview.PolicyText, strings.TrimSpace(reason), preview.RefundPercent, preview.TotalChargedCLP, preview.RefundAmountCLP, preview.IdempotencyKey, now.UTC())
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	if preview.RefundAmountCLP == 0 {
		if _, err = tx.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET refund_status='no_aplica',resuelta_en=$2,actualizada_en=$2 WHERE reserva_id=$1::uuid`, reservationID, now.UTC()); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		if err = cancelCommercialReservationTx(ctx, tx, preview, now.UTC()); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		preview.RefundStatus = "no_aplica"
		preview.Status = "cancelada"
	}
	if err = tx.Commit(ctx); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	return preview, nil
}

func (r *Repository) FinishCommercialCancellation(ctx context.Context, reservationID, providerRefundID, status string, now time.Time) (rentalflow.CommercialCancellation, error) {
	if r == nil || r.beginner == nil || ctx == nil || !rentalflow.ValidUUID(reservationID) || now.IsZero() || status != "approved" && status != "pendiente" && status != "no_aplica" {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrInvalid
	}
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var out rentalflow.CommercialCancellation
	var reservationState, actorID, hostID, renterID string
	var start time.Time
	var refundAmount int64
	err = tx.QueryRow(ctx, `SELECT c.reserva_id::text,c.actor_id::text,c.porcentaje_reembolso,c.monto_total_clp,c.monto_reembolso_clp,c.refund_status,c.clave_idempotencia,COALESCE(c.provider_refund_id,''),r.estado,r.inicio,r.anfitrion_id::text,r.arrendatario_id::text,c.politica_version,c.politica_texto
FROM public.reserva_comercial_cancelacion c JOIN public.reserva_arriendo_comercial r ON r.id=c.reserva_id WHERE c.reserva_id=$1::uuid FOR UPDATE OF c,r`, reservationID).Scan(&out.ReservationID, &actorID, &out.RefundPercent, &out.TotalChargedCLP, &refundAmount, &out.RefundStatus, &out.IdempotencyKey, &out.ProviderRefundID, &reservationState, &start, &hostID, &renterID, &out.PolicyVersion, &out.PolicyText)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrNotFound
	}
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	out.ActorID, out.HostID = actorID, hostID
	out.StartsAt, out.Currency, out.RefundAmountCLP = start.UTC().Format(time.RFC3339Nano), "CLP", refundAmount
	if out.RefundStatus == "aprobado" || out.RefundStatus == "no_aplica" {
		out.Status = reservationState
		if err = tx.Commit(ctx); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		return out, nil
	}
	if providerRefundID != "" && !validProviderRefundID(providerRefundID) {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrInvalid
	}
	if status == "pendiente" {
		_, err = tx.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET provider_refund_id=COALESCE(NULLIF($2,''),provider_refund_id),refund_status='pendiente',actualizada_en=$3 WHERE reserva_id=$1::uuid`, reservationID, providerRefundID, now.UTC())
		if err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		out.RefundStatus = "pendiente"
		out.Status = reservationState
		out.RefundAmountCLP = refundAmount
		out.Currency = "CLP"
	} else {
		_, err = tx.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET provider_refund_id=NULLIF($2,''),refund_status='aprobado',resuelta_en=$3,actualizada_en=$3 WHERE reserva_id=$1::uuid`, reservationID, providerRefundID, now.UTC())
		if err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		out.RefundStatus = "aprobado"
		if err = cancelCommercialReservationTx(ctx, tx, out, now.UTC()); err != nil {
			return rentalflow.CommercialCancellation{}, err
		}
		out.Status = "reembolsada"
		if refundAmount < out.TotalChargedCLP {
			out.Status = "cancelada"
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	return out, nil
}

func (r *Repository) FailCommercialCancellation(ctx context.Context, reservationID string, now time.Time) error {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(reservationID) || now.IsZero() {
		return rentalflow.ErrInvalid
	}
	_, err := r.db.Exec(ctx, `UPDATE public.reserva_comercial_cancelacion SET refund_status='fallido',actualizada_en=$2 WHERE reserva_id=$1::uuid AND refund_status='pendiente'`, reservationID, now.UTC())
	return err
}

func (r *Repository) ListPendingCommercialCancellations(ctx context.Context, limit int) ([]rentalflow.CommercialCancellation, error) {
	if r == nil || r.db == nil || ctx == nil || limit < 1 || limit > 100 {
		return nil, rentalflow.ErrInvalid
	}
	rows, err := r.db.Query(ctx, `SELECT c.reserva_id::text,c.actor_id::text,c.motivo,c.porcentaje_reembolso,c.monto_total_clp,c.monto_reembolso_clp,c.refund_status,c.clave_idempotencia,COALESCE(c.provider_refund_id,''),r.anfitrion_id::text,r.inicio,c.politica_version,c.politica_texto,r.estado,COALESCE(p.provider_payment_id,'')
FROM public.reserva_comercial_cancelacion c
JOIN public.reserva_arriendo_comercial r ON r.id=c.reserva_id
JOIN public.checkout_preferencia_comercial p ON p.id=r.operacion_checkout_id
WHERE c.refund_status='pendiente' AND COALESCE(p.provider_payment_status,'') <> 'refunded'
ORDER BY c.actualizada_en,c.reserva_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]rentalflow.CommercialCancellation, 0)
	for rows.Next() {
		var item rentalflow.CommercialCancellation
		var start time.Time
		if err = rows.Scan(&item.ReservationID, &item.ActorID, &item.Motivo, &item.RefundPercent, &item.TotalChargedCLP, &item.RefundAmountCLP, &item.RefundStatus, &item.IdempotencyKey, &item.ProviderRefundID, &item.HostID, &start, &item.PolicyVersion, &item.PolicyText, &item.Status, &item.ProviderPaymentID); err != nil {
			return nil, err
		}
		item.StartsAt = start.UTC().Format(time.RFC3339Nano)
		item.Currency = "CLP"
		items = append(items, item)
	}
	return items, rows.Err()
}

func cancelCommercialReservationTx(ctx context.Context, tx pgx.Tx, out rentalflow.CommercialCancellation, now time.Time) error {
	status := "cancelada"
	if out.RefundAmountCLP >= out.TotalChargedCLP {
		status = "reembolsada"
	}
	tag, err := tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado=$2,actualizada_en=$3 WHERE id=$1::uuid AND estado='pagada'`, out.ReservationID, status, now.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return rentalflow.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE public.reserva_cuota_mensual SET estado='cancelada' WHERE reserva_arriendo_comercial_id=$1::uuid AND estado='pendiente'`, out.ReservationID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$2 WHERE reserva_arriendo_comercial_id=$1::uuid AND activo`, out.ReservationID, now.UTC()); err != nil {
		return err
	}
	message := "La reserva fue cancelada. Reembolso aplicado: $" + strconv.FormatInt(out.RefundAmountCLP, 10) + " CLP."
	_, err = tx.Exec(ctx, `INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
SELECT gen_random_uuid(), r.actor_id, 'reserva_cancelada', 'Reserva cancelada', $2,
 CASE WHEN r.actor_id=booking.anfitrion_id THEN '/anfitrion/reservas/' || booking.id::text ELSE '/reservas/' || booking.id::text END,
 booking.id,$3
FROM (SELECT anfitrion_id,arrendatario_id,id FROM public.reserva_arriendo_comercial WHERE id=$1::uuid) booking
CROSS JOIN LATERAL (VALUES (booking.anfitrion_id),(booking.arrendatario_id)) r(actor_id)
ON CONFLICT (referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING`, out.ReservationID, message, now.UTC())
	return err
}

func validProviderRefundID(value string) bool {
	if len(value) < 1 || len(value) > 100 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
