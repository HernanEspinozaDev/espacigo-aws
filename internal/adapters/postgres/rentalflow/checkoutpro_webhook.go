package rentalflowpg

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/jackc/pgx/v5"
)

// ApplyVerifiedPayment is the atomic boundary used by the webhook worker after
// a signed notification has been resolved through Mercado Pago's payment API.
// It never accepts state from a browser redirect or raw notification body.
func (r *Repository) ApplyVerifiedPayment(ctx context.Context, p checkoutpro.VerifiedPayment) (bool, error) {
	if r == nil || ctx == nil || r.beginner == nil || !validUUIDText(p.OperationID) || !validUUIDText(p.ExternalReference) || p.OperationID != p.ExternalReference || !positiveDecimal(p.ProviderPaymentID) || !positiveDecimal(p.SellerID) || p.PaymentUpdatedAt.IsZero() || p.TransactionAmountCLP <= 0 {
		return false, checkoutpro.ErrInvalid
	}
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Serialize globally unique provider payment IDs before checking the owner;
	// the partial unique index remains the final database guard.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('mercadopago-payment:' || $1, 0))`, p.ProviderPaymentID); err != nil {
		return false, err
	}
	// Provider payment IDs are globally unique. Keep an already bound ID on its
	// original operation and retain the conflicting receipt for reconciliation.
	var paymentOwner string
	err = tx.QueryRow(ctx, `SELECT id::text FROM public.checkout_preferencia_comercial WHERE provider_payment_id=$1 AND id<>$2`, p.ProviderPaymentID, p.OperationID).Scan(&paymentOwner)
	if err == nil {
		return r.markCheckoutReconciliation(ctx, tx, p.OperationID, "", p.Status, p.PaymentUpdatedAt, "provider_payment_already_bound")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var opID, hostID, renterID, spaceID, opState, checkoutMethod, providerPaymentID, providerStatus string
	var expires, previousUpdated time.Time
	var testMode bool
	var base, fee int64
	err = tx.QueryRow(ctx, `SELECT id::text,host_id::text,arrendatario_id::text,espacio_id::text,estado,checkout_method,vence_en,test_mode,base_clp,marketplace_fee_clp,COALESCE(provider_payment_id,''),COALESCE(provider_payment_status,''),COALESCE(provider_payment_updated_at,'0001-01-01 00:00:00+00'::timestamptz) FROM public.checkout_preferencia_comercial WHERE id=$1 FOR UPDATE`, p.OperationID).Scan(&opID, &hostID, &renterID, &spaceID, &opState, &checkoutMethod, &expires, &testMode, &base, &fee, &providerPaymentID, &providerStatus, &previousUpdated)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, checkoutpro.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	log.Printf("mercadopago_checkout_api_reconciliation stage=provider_facts_received operation_id=%s payment_id=%s amount_clp=%d status=%s application_fee_expected_set=%t application_fee_present=%t",
		p.OperationID, p.ProviderPaymentID, p.TransactionAmountCLP, p.Status, fee > 0, p.ApplicationFeeCLP != nil)
	if providerPaymentID != "" && providerPaymentID != p.ProviderPaymentID {
		return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, p.Status, p.PaymentUpdatedAt, "multiple_provider_payments")
	}
	if providerPaymentID == p.ProviderPaymentID && p.PaymentUpdatedAt.Before(previousUpdated) {
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	var expectedSeller string
	var liveMode bool
	err = tx.QueryRow(ctx, `SELECT seller_id,live_mode FROM public.mercadopago_seller_connection WHERE anfitrion_id=$1 AND connection_state='active'`, hostID).Scan(&expectedSeller, &liveMode)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (expectedSeller != p.SellerID || liveMode == testMode) {
		return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, p.Status, p.PaymentUpdatedAt, "seller_or_mode_mismatch")
	}
	if err != nil {
		return false, err
	}
	var quoteTotal int64
	var quoteCurrency string
	err = tx.QueryRow(ctx, `SELECT subtotal_clp,moneda FROM public.cotizacion_arriendo_comercial WHERE id=(SELECT cotizacion_id FROM public.checkout_preferencia_comercial WHERE id=$1)`, p.OperationID).Scan(&quoteTotal, &quoteCurrency)
	if err != nil {
		return false, err
	}
	modeMatches := checkoutpro.PaymentModeMatches(testMode, p.LiveMode)
	if (checkoutMethod != "checkout_pro" && checkoutMethod != "payments_api") ||
		checkoutMethod == "payments_api" && !testMode ||
		p.OperationID != opID || p.ProviderPaymentID == "" || p.TransactionAmountCLP != base || p.TransactionAmountCLP != quoteTotal || p.Currency != quoteCurrency || p.Currency != "CLP" || !checkoutpro.AllowedPaymentType(testMode, p.PaymentType) || !modeMatches {
		return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, p.Status, p.PaymentUpdatedAt, "payment_facts_mismatch")
	}
	if providerPaymentID == p.ProviderPaymentID && p.PaymentUpdatedAt.Before(previousUpdated) {
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	status := strings.ToLower(strings.TrimSpace(p.Status))
	if status != "approved" && status != "rejected" && status != "cancelled" && status != "pending" && status != "in_process" && status != "authorized" {
		return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt, "unsupported_provider_status")
	}
	if checkoutMethod == "payments_api" && status == "approved" {
		if !applicationFeeMatches(fee, p.ApplicationFeeCLP) {
			log.Printf("mercadopago_checkout_api_reconciliation stage=provider_facts_verified operation_id=%s payment_id=%s result=application_fee_mismatch application_fee_expected_set=%t application_fee_present=%t",
				p.OperationID, p.ProviderPaymentID, fee > 0, p.ApplicationFeeCLP != nil)
			return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt, "application_fee_mismatch")
		}
	}
	log.Printf("mercadopago_checkout_api_reconciliation stage=provider_facts_verified operation_id=%s payment_id=%s result=matched application_fee_expected_set=%t application_fee_present=%t",
		p.OperationID, p.ProviderPaymentID, fee > 0, p.ApplicationFeeCLP != nil)
	if opState == "approved" || opState == "rejected" || opState == "cancelled" || opState == "expired" || opState == "reconciliation_required" {
		if opState == "approved" && status == "approved" {
			if err = tx.Commit(ctx); err != nil {
				return false, err
			}
			return true, nil
		}
		if status == "approved" || opState == "reconciliation_required" || opState == "approved" {
			return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt, "late_or_conflicting_payment_state")
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	if r.contractsRequired {
		valid, bindingErr := commercialContractPaymentBindingValid(ctx, tx, p.OperationID, renterID)
		if bindingErr != nil {
			return false, bindingErr
		}
		if !valid {
			return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt, "contract_binding_mismatch")
		}
	}
	if status == "pending" || status == "in_process" || status == "authorized" {
		_, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET provider_payment_id=$2,provider_payment_status=$3,provider_payment_updated_at=$4,payment_verified_at=$5,actualizada_en=$5 WHERE id=$1 AND estado='pending'`, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt.UTC(), r.now().UTC())
		if err != nil {
			return false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if status == "approved" {
		var reservationState string
		var occupancyActive bool
		var occupancyExpiry *time.Time
		err = tx.QueryRow(ctx, `SELECT r.estado,o.activo,o.expira_en FROM public.reserva_arriendo_comercial r JOIN public.ocupacion o ON o.id=r.ocupacion_id WHERE r.operacion_checkout_id=$1 FOR UPDATE OF r,o`, p.OperationID).Scan(&reservationState, &occupancyActive, &occupancyExpiry)
		if err != nil {
			return false, err
		}
		if !p.PaymentUpdatedAt.Before(expires) || reservationState != "pendiente_de_pago" || !occupancyActive || occupancyExpiry == nil || !occupancyExpiry.After(p.PaymentUpdatedAt) {
			return r.markCheckoutReconciliation(ctx, tx, p.OperationID, p.ProviderPaymentID, status, p.PaymentUpdatedAt, "payment_after_hold_expiry")
		}
		_, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET estado='approved',provider_payment_id=$2,provider_payment_status='approved',provider_payment_updated_at=$3,payment_verified_at=$4,actualizada_en=$4 WHERE id=$1 AND estado='pending'`, p.OperationID, p.ProviderPaymentID, p.PaymentUpdatedAt.UTC(), r.now().UTC())
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado='pagada',actualizada_en=$2 WHERE operacion_checkout_id=$1 AND estado='pendiente_de_pago'`, p.OperationID, r.now().UTC())
		if err != nil {
			return false, err
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE public.ocupacion SET tipo='reserva_comercial',expira_en=NULL WHERE reserva_arriendo_comercial_id=$1 AND activo AND tipo='retencion_comercial'`, p.OperationID)
		if updateErr != nil {
			return false, updateErr
		}
		if tag.RowsAffected() != 1 {
			return false, checkoutpro.ErrConflict
		}
		if r.contractsRequired {
			if err = enqueueCommercialContractDocument(ctx, tx, p.OperationID, r.now().UTC()); err != nil {
				return false, err
			}
		}
		if err = enqueueCheckoutPaymentNotices(ctx, tx, p.OperationID, "approved", r.now().UTC()); err != nil {
			return false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	// A provider rejection/cancellation releases the hold in the same commit.
	newState, newReservation, notice := "rejected", "cancelada_por_pago", "rejected"
	if status == "cancelled" {
		newState, newReservation = "cancelled", "cancelada_por_pago"
	}
	_, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET estado=$2,provider_payment_id=$3,provider_payment_status=$4,provider_payment_updated_at=$5,payment_verified_at=$6,actualizada_en=$6 WHERE id=$1 AND estado='pending'`, p.OperationID, newState, p.ProviderPaymentID, status, p.PaymentUpdatedAt.UTC(), r.now().UTC())
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado=$2,actualizada_en=$3 WHERE operacion_checkout_id=$1 AND estado='pendiente_de_pago'`, p.OperationID, newReservation, r.now().UTC())
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$2 WHERE reserva_arriendo_comercial_id=$1 AND activo`, p.OperationID, r.now().UTC())
	if err != nil {
		return false, err
	}
	if err = enqueueCheckoutPaymentNotices(ctx, tx, p.OperationID, notice, r.now().UTC()); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}

func applicationFeeMatches(expected int64, observed *int64) bool {
	if expected == 0 {
		return observed == nil || *observed == 0
	}
	return observed != nil && *observed == expected
}

func (r *Repository) markCheckoutReconciliation(ctx context.Context, tx pgx.Tx, id, paymentID, status string, updatedAt time.Time, reason string) (bool, error) {
	now := r.now().UTC()
	if paymentID == "" {
		_, err := tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET estado='reconciliation_required',reconciliation_reason=$2,actualizada_en=$3 WHERE id=$1`, id, reason, now)
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado='reconciliation_required',actualizada_en=$2 WHERE operacion_checkout_id=$1`, id, now)
		if err != nil {
			return false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	_, err := tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET estado='reconciliation_required',provider_payment_id=COALESCE(provider_payment_id,NULLIF($2,'')),provider_payment_status=$3,provider_payment_updated_at=GREATEST(COALESCE(provider_payment_updated_at,'-infinity'::timestamptz),$4),reconciliation_reason=$5,payment_verified_at=$6,actualizada_en=$6 WHERE id=$1`, id, paymentID, status, updatedAt.UTC(), reason, now)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado='reconciliation_required',actualizada_en=$2 WHERE operacion_checkout_id=$1`, id, now)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}

func validUUIDText(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range v {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
func positiveDecimal(v string) bool {
	if v == "" || len(v) > 32 {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return strings.Trim(v, "0") != ""
}
