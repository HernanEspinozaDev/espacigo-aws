package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
)

// The worker and endpoint share this lease. The service bounds each network
// attempt to less than this interval, and retries use the original key.
const pendingCancellationLease = 30 * time.Second

func (r *Repository) BeginPendingPaymentCancellation(ctx context.Context, actorID, reservationID, key string, now time.Time) (rentalflow.PendingPaymentCancellationIntent, error) {
	var intent rentalflow.PendingPaymentCancellationIntent
	if r == nil || r.beginner == nil || !rentalflow.ValidUUID(actorID) ||
		!rentalflow.ValidUUID(reservationID) || !rentalflow.ValidUUID(key) || now.IsZero() {
		return intent, rentalflow.ErrInvalid
	}
	now = now.UTC()
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return intent, err
	}
	defer tx.Rollback(context.Background())

	// Lock the same checkout row used by the webhook before binding an
	// intent to a payment. A participant can never supply the provider ID.
	var reservationState, checkoutState, method, providerStatus string
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT r.id::text,r.estado,c.id::text,c.estado,c.checkout_method,c.host_id::text,
	COALESCE(s.seller_id,''),COALESCE(c.provider_payment_id,''),COALESCE(c.provider_payment_status,''),c.total_charged_clp,c.test_mode,
	LEAST(c.vence_en,r.pago_vence_en)
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
LEFT JOIN public.mercadopago_seller_connection s ON s.anfitrion_id=c.host_id AND s.connection_state='active' AND s.live_mode<>c.test_mode
WHERE r.id=$1::uuid AND $2::uuid IN (r.anfitrion_id,r.arrendatario_id)
FOR UPDATE OF c`, reservationID, actorID).Scan(&intent.ReservationID, &reservationState, &intent.OperationID, &checkoutState, &method,
		&intent.HostID, &intent.SellerID, &intent.PaymentID, &providerStatus, &intent.AmountCLP, &intent.TestMode, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return intent, rentalflow.ErrNotFound
	}
	if err != nil {
		return intent, err
	}
	if method != "checkout_pro" {
		return intent, rentalflow.ErrConflict
	}
	intent.ExpiresAt = expiresAt.UTC()
	intent.Expired = checkoutState == "expired"

	// Both participants may observe a terminal outcome. In a replay, the
	// original actor remains the recorded requester.
	var savedSeller, savedPayment, savedOutcome string
	var lastAttempt *time.Time
	err = tx.QueryRow(ctx, `SELECT actor_id::text,seller_id,COALESCE(provider_payment_id,''),clave_idempotencia::text,
	ultimo_intento_en,COALESCE(resultado,'') FROM public.checkout_cancelacion_pendiente WHERE operacion_id=$1::uuid`, intent.OperationID).
		Scan(&intent.ActorID, &savedSeller, &savedPayment, &intent.IdempotencyKey, &lastAttempt, &savedOutcome)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return intent, err
	}
	intent.Outcome = savedOutcome
	switch {
	case checkoutState == "approved" && providerStatus == "approved":
		intent.Outcome = "approved"
	case checkoutState == "cancelled" && providerStatus == "cancelled":
		intent.Outcome = "cancelled"
	}
	if intent.Outcome != "" {
		if found && savedOutcome == "" {
			if _, err = tx.Exec(ctx, `UPDATE public.checkout_cancelacion_pendiente SET resultado=$2,provider_payment_id=COALESCE(provider_payment_id,NULLIF($3,'')) WHERE operacion_id=$1::uuid AND resultado IS NULL`,
				intent.OperationID, intent.Outcome, intent.PaymentID); err != nil {
				return intent, err
			}
		}
		return intent, tx.Commit(ctx)
	}
	eligible := checkoutState == "pending" && reservationState == "pendiente_de_pago"
	existingExpired := found && checkoutState == "expired" && reservationState == "vencida_pago"
	if !eligible && !existingExpired {
		return intent, rentalflow.ErrConflict
	}
	if intent.SellerID == "" || intent.AmountCLP <= 0 ||
		(found && (savedSeller != intent.SellerID || (savedPayment != "" && savedPayment != intent.PaymentID))) {
		return intent, rentalflow.ErrConflict
	}
	if !found {
		intent.ActorID, intent.IdempotencyKey = actorID, key
		_, err = tx.Exec(ctx, `INSERT INTO public.checkout_cancelacion_pendiente(operacion_id,actor_id,seller_id,clave_idempotencia,creada_en)
VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5)`, intent.OperationID, actorID, intent.SellerID, key, now)
		if err != nil {
			return intent, err
		}
	}
	if lastAttempt == nil || !lastAttempt.Add(pendingCancellationLease).After(now) {
		var paymentIDArg any
		if intent.PaymentID != "" {
			paymentIDArg = intent.PaymentID
		}
		_, err = tx.Exec(ctx, `UPDATE public.checkout_cancelacion_pendiente SET ultimo_intento_en=$2,provider_payment_id=COALESCE(provider_payment_id,$3) WHERE operacion_id=$1::uuid`,
			intent.OperationID, now, paymentIDArg)
		if err != nil {
			return intent, err
		}
		intent.Claimed = true
	}
	return intent, tx.Commit(ctx)
}

func (r *Repository) FinishPendingPaymentCancellation(ctx context.Context, intent rentalflow.PendingPaymentCancellationIntent, outcome string) error {
	if r == nil || r.db == nil || !rentalflow.ValidUUID(intent.OperationID) || !rentalflow.ValidUUID(intent.IdempotencyKey) ||
		intent.PaymentID == "" || (outcome != "approved" && outcome != "cancelled") {
		return rentalflow.ErrInvalid
	}
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	// ApplyVerifiedPayment has already verified and persisted the outcome
	// or stopped it for reconciliation. An expired checkout has already
	// released its hold through the ordinary lifecycle and can safely
	// record a verified remote cancellation on the intent.
	var updatedCount int
	err := r.db.QueryRow(ctx, `WITH updated AS (
	UPDATE public.checkout_cancelacion_pendiente i
	SET resultado=$4,provider_payment_id=COALESCE(i.provider_payment_id,$3)
	FROM public.checkout_preferencia_comercial c
	WHERE i.operacion_id=c.id AND i.operacion_id=$1::uuid AND i.clave_idempotencia=$2::uuid
	  AND (i.provider_payment_id IS NULL OR i.provider_payment_id=$3)
	  AND (i.resultado IS NULL OR i.resultado=$4)
	  AND ((c.estado=$4 AND c.provider_payment_status=$4 AND c.provider_payment_id=$3)
	    OR ($4='cancelled' AND c.estado='expired' AND (c.provider_payment_id IS NULL OR c.provider_payment_id=$3)))
	RETURNING i.operacion_id
), inserted AS (
	INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
	SELECT gen_random_uuid(), r.actor_id, 'pago_pendiente_cancelado', 'Pago pendiente cancelado',
	 'Se confirmó la cancelación del pago pendiente y la reserva quedó liberada.',
	 CASE WHEN r.actor_id=booking.anfitrion_id THEN '/anfitrion/reservas/' || booking.id::text ELSE '/reservas/' || booking.id::text END,
	 booking.id, $5
	FROM updated
	JOIN public.reserva_arriendo_comercial booking ON booking.operacion_checkout_id=updated.operacion_id
	CROSS JOIN LATERAL (VALUES (booking.anfitrion_id),(booking.arrendatario_id)) r(actor_id)
	WHERE $4='cancelled'
	ON CONFLICT (referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING
)
SELECT count(*) FROM updated`,
		intent.OperationID, intent.IdempotencyKey, intent.PaymentID, outcome, now).Scan(&updatedCount)
	if err != nil {
		return err
	}
	if updatedCount != 1 {
		return rentalflow.ErrConflict
	}
	return nil
}

// ExpirePendingPaymentCancellation closes an expired Checkout Pro operation
// locally after Mercado Pago confirms no payment exists for its external
// reference, releasing availability and emitting the expiration notice once.
func (r *Repository) ExpirePendingPaymentCancellation(ctx context.Context, intent rentalflow.PendingPaymentCancellationIntent, now time.Time) error {
	if r == nil || r.beginner == nil || !rentalflow.ValidUUID(intent.OperationID) ||
		!rentalflow.ValidUUID(intent.IdempotencyKey) || intent.PaymentID != "" || now.IsZero() {
		return rentalflow.ErrInvalid
	}
	now = now.UTC()
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var checkoutState, providerPaymentID, reservationState, savedKey, savedPaymentID, savedOutcome string
	var checkoutExpires, paymentExpires time.Time
	err = tx.QueryRow(ctx, `SELECT c.estado,COALESCE(c.provider_payment_id,''),c.vence_en,r.estado,r.pago_vence_en,
	i.clave_idempotencia::text,COALESCE(i.provider_payment_id,''),COALESCE(i.resultado,'')
FROM public.checkout_preferencia_comercial c
JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id
JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=c.id
WHERE c.id=$1::uuid
FOR UPDATE OF c,r,i`, intent.OperationID).Scan(
		&checkoutState, &providerPaymentID, &checkoutExpires, &reservationState, &paymentExpires,
		&savedKey, &savedPaymentID, &savedOutcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.ErrNotFound
	}
	if err != nil {
		return err
	}
	if savedKey != intent.IdempotencyKey || providerPaymentID != "" || savedPaymentID != "" {
		return rentalflow.ErrConflict
	}
	if savedOutcome == "cancelled" {
		return tx.Commit(ctx)
	}
	if savedOutcome != "" {
		return rentalflow.ErrConflict
	}
	switch {
	case checkoutState == "pending" && reservationState == "pendiente_de_pago":
		if checkoutExpires.After(now) && paymentExpires.After(now) {
			return rentalflow.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET estado='expired',actualizada_en=$2 WHERE id=$1::uuid AND estado='pending' AND provider_payment_id IS NULL`, intent.OperationID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado='vencida_pago',actualizada_en=$2 WHERE operacion_checkout_id=$1::uuid AND estado='pendiente_de_pago'`, intent.OperationID, now); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE public.reserva_cuota_mensual SET estado='vencida' WHERE reserva_arriendo_comercial_id=$1::uuid AND estado='pendiente'`, intent.OperationID)
		if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$2 WHERE reserva_arriendo_comercial_id=$1::uuid AND activo`, intent.OperationID, now); err != nil {
			return err
		}
		if err = enqueueCheckoutPaymentNotices(ctx, tx, intent.OperationID, "expired", now); err != nil {
			return err
		}
	case checkoutState == "expired" && reservationState == "vencida_pago":
	default:
		return rentalflow.ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE public.checkout_cancelacion_pendiente SET resultado='cancelled' WHERE operacion_id=$1::uuid AND clave_idempotencia=$2::uuid AND provider_payment_id IS NULL AND (resultado IS NULL OR resultado='cancelled')`,
		intent.OperationID, intent.IdempotencyKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return rentalflow.ErrConflict
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListPendingPaymentCancellationIntents(ctx context.Context, now time.Time, limit int) ([]rentalflow.PendingPaymentCancellationIntent, error) {
	if r == nil || r.db == nil || now.IsZero() || limit < 1 || limit > 100 {
		return nil, rentalflow.ErrInvalid
	}
	rows, err := r.db.Query(ctx, `SELECT r.id::text,i.actor_id::text
FROM public.checkout_cancelacion_pendiente i
JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=i.operacion_id
JOIN public.checkout_preferencia_comercial c ON c.id=i.operacion_id
WHERE i.resultado IS NULL AND c.estado IN ('pending','expired')
  AND (c.provider_payment_id IS NULL OR i.provider_payment_id IS NULL OR i.provider_payment_id=c.provider_payment_id)
  AND (i.ultimo_intento_en IS NULL OR i.ultimo_intento_en<=$1)
ORDER BY i.ultimo_intento_en NULLS FIRST,i.operacion_id LIMIT $2`, now.UTC().Add(-pendingCancellationLease), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var intents []rentalflow.PendingPaymentCancellationIntent
	for rows.Next() {
		var i rentalflow.PendingPaymentCancellationIntent
		if err := rows.Scan(&i.ReservationID, &i.ActorID); err != nil {
			return nil, err
		}
		intents = append(intents, i)
	}
	return intents, rows.Err()
}
