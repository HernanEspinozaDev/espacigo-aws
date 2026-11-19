package rentalflowpg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/jackc/pgx/v5"
)

var webhookIDText = regexp.MustCompile(`^[0-9]{1,64}$`)
var webhookTimestampText = regexp.MustCompile(`^[0-9]{1,20}$`)
var webhookErrorCodes = map[string]struct{}{
	"seller_hint_missing": {}, "provider_identity_mismatch": {}, "payment_requires_reconciliation": {},
	"seller_token_unavailable": {}, "provider_lookup_failed": {}, "payment_outcome_unavailable": {},
}

// Receive durably stores only a payment notification whose HMAC has already
// been verified by checkoutpro.WebhookReceiver. The provider request ID is the
// delivery idempotency key; conflicting reuse is left unacknowledged.
func (r *Repository) Receive(ctx context.Context, n checkoutpro.PaymentNotification) (checkoutpro.WebhookReceipt, error) {
	if r == nil || ctx == nil || n.Topic != "payment" || !webhookIDText.MatchString(n.PaymentID) ||
		n.RequestID == "" || len(n.RequestID) > 256 || len(n.Action) > 100 ||
		!webhookTimestampText.MatchString(n.SignatureTimestamp) || n.ReceivedAt.IsZero() || r.newID == nil {
		return checkoutpro.WebhookReceipt{}, checkoutpro.ErrInvalid
	}
	if n.SellerIDHint != "" && !webhookIDText.MatchString(n.SellerIDHint) {
		return checkoutpro.WebhookReceipt{}, checkoutpro.ErrInvalid
	}
	var row checkoutpro.WebhookReceipt
	err := r.db.QueryRow(ctx, `INSERT INTO public.mercadopago_payment_webhook_receipt
(id,topic,action,provider_payment_id,provider_seller_hint,provider_request_id,signature_timestamp,estado,intentos,reintentar_en,recibida_en,actualizada_en)
VALUES($1::uuid,'payment',$2,$3,NULLIF($4,''),$5,$6,'received',0,$7,$7,$7)
ON CONFLICT(topic,provider_request_id) DO NOTHING
RETURNING id::text,topic,action,provider_payment_id,COALESCE(provider_seller_hint,''),provider_request_id,signature_timestamp,estado,intentos,reintentar_en`,
		r.newID(), n.Action, n.PaymentID, n.SellerIDHint, n.RequestID, n.SignatureTimestamp, n.ReceivedAt.UTC()).Scan(
		&row.ID, &row.Topic, &row.Action, &row.PaymentID, &row.SellerIDHint, &row.RequestID, &row.SignatureTimestamp, &row.State, &row.Attempts, &row.NextAttemptAt)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return checkoutpro.WebhookReceipt{}, err
	}
	err = r.db.QueryRow(ctx, `SELECT id::text,topic,action,provider_payment_id,COALESCE(provider_seller_hint,''),provider_request_id,signature_timestamp,estado,intentos,reintentar_en
FROM public.mercadopago_payment_webhook_receipt WHERE topic='payment' AND provider_request_id=$1`, n.RequestID).Scan(
		&row.ID, &row.Topic, &row.Action, &row.PaymentID, &row.SellerIDHint, &row.RequestID, &row.SignatureTimestamp, &row.State, &row.Attempts, &row.NextAttemptAt)
	if err != nil {
		return checkoutpro.WebhookReceipt{}, err
	}
	if row.PaymentID != n.PaymentID || row.Action != n.Action {
		return checkoutpro.WebhookReceipt{}, checkoutpro.ErrReconciliationRequired
	}
	return row, nil
}

func (r *Repository) Pending(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]checkoutpro.WebhookReceipt, error) {
	if r == nil || ctx == nil || now.IsZero() || lease <= 0 || limit < 1 || limit > 100 {
		return nil, checkoutpro.ErrInvalid
	}
	rows, err := r.db.Query(ctx, `SELECT id::text,topic,action,provider_payment_id,COALESCE(provider_seller_hint,''),provider_request_id,signature_timestamp,estado,intentos,reintentar_en
FROM public.mercadopago_payment_webhook_receipt
WHERE (estado IN ('received','retry') AND reintentar_en <= $1)
   OR (estado='processing' AND claimed_at <= $1-($2*interval '1 second'))
ORDER BY recibida_en,id LIMIT $3`, now.UTC(), lease.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []checkoutpro.WebhookReceipt
	for rows.Next() {
		var row checkoutpro.WebhookReceipt
		if err := rows.Scan(&row.ID, &row.Topic, &row.Action, &row.PaymentID, &row.SellerIDHint, &row.RequestID, &row.SignatureTimestamp, &row.State, &row.Attempts, &row.NextAttemptAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *Repository) Claim(ctx context.Context, id string, now time.Time, lease time.Duration) (bool, error) {
	if r == nil || ctx == nil || id == "" || now.IsZero() || lease <= 0 {
		return false, checkoutpro.ErrInvalid
	}
	var claimed string
	err := r.db.QueryRow(ctx, `UPDATE public.mercadopago_payment_webhook_receipt
SET estado='processing',claimed_at=$2,intentos=intentos+1,actualizada_en=$2
WHERE id=$1::uuid AND ((estado IN ('received','retry') AND reintentar_en <= $2)
   OR (estado='processing' AND claimed_at <= $2-($3*interval '1 second')))
RETURNING id::text`, id, now.UTC(), lease.Seconds()).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) MarkProcessed(ctx context.Context, id, providerStatus string, now time.Time) error {
	if !validWebhookStatus(providerStatus) {
		return checkoutpro.ErrInvalid
	}
	return r.updateWebhookReceipt(ctx, id, "processed", "", providerStatus, now, now)
}

func (r *Repository) MarkRetry(ctx context.Context, id, errorCode string, next time.Time) error {
	if !validWebhookError(errorCode) {
		return checkoutpro.ErrInvalid
	}
	return r.updateWebhookReceipt(ctx, id, "retry", errorCode, "", next, time.Time{})
}

func (r *Repository) MarkReconciliationRequired(ctx context.Context, id, errorCode string, now time.Time) error {
	if !validWebhookError(errorCode) {
		return checkoutpro.ErrInvalid
	}
	return r.updateWebhookReceipt(ctx, id, "reconciliation_required", errorCode, "", now, time.Time{})
}

func (r *Repository) updateWebhookReceipt(ctx context.Context, id, state, code, providerStatus string, when, processed time.Time) error {
	if r == nil || ctx == nil || id == "" || when.IsZero() {
		return checkoutpro.ErrInvalid
	}
	var errorValue, statusValue any
	if code != "" {
		errorValue = code
	}
	if providerStatus != "" {
		statusValue = providerStatus
	}
	var processedValue any
	if !processed.IsZero() {
		processedValue = processed.UTC()
	}
	now := r.now().UTC()
	var nextAttempt time.Time
	if state == "retry" {
		nextAttempt = when.UTC()
	} else {
		nextAttempt = now
	}
	tag, err := r.db.Exec(ctx, `UPDATE public.mercadopago_payment_webhook_receipt
SET estado=$2,last_error_code=$3,provider_status=COALESCE($4,provider_status),reintentar_en=$5,
    claimed_at=NULL,procesada_en=$6,actualizada_en=$7
WHERE id=$1::uuid AND estado='processing'`, id, state, errorValue, statusValue, nextAttempt, processedValue, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("webhook receipt state changed concurrently")
	}
	return nil
}

func validWebhookError(code string) bool {
	_, ok := webhookErrorCodes[code]
	return ok
}

func validWebhookStatus(status string) bool {
	if len(status) == 0 || len(status) > 40 {
		return false
	}
	for _, c := range status {
		if !((c >= 'a' && c <= 'z') || c == '_') {
			return false
		}
	}
	return true
}

var _ checkoutpro.WebhookInbox = (*Repository)(nil)
