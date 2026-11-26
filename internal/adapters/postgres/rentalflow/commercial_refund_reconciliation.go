package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RecordExternalFullCommercialRefund(ctx context.Context, actor, reservationID string, payment checkoutpro.VerifiedPayment, refund checkoutpro.RefundResult, now time.Time) (rentalflow.CommercialCancellation, error) {
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result, err := r.readCommercialCancellation(ctx, tx, actor, reservationID, now, true)
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	if result.HostID != actor {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrNotFound
	}
	if result.RefundStatus == rentalflow.CommercialRefundApproved && result.ProviderRefundID == refund.ID {
		return result, tx.Commit(ctx)
	}
	if !result.CanLinkExternalFullRefund(now) || payment.Status != "refunded" || refund.Status != "approved" ||
		!validProviderRefundID(refund.ID) || refund.Amount != result.TotalChargedCLP ||
		result.CheckoutOperationID != payment.OperationID || payment.ExternalReference != result.CheckoutOperationID ||
		result.ProviderPaymentID != payment.ProviderPaymentID || result.SellerID != payment.SellerID || result.SellerID == "" ||
		payment.Currency != "CLP" || payment.TransactionAmountCLP != result.TotalChargedCLP ||
		!checkoutpro.PaymentModeMatches(result.TestMode, payment.LiveMode) || payment.PaymentUpdatedAt.IsZero() {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
	}
	var marketplaceFee int64
	err = tx.QueryRow(ctx, `UPDATE public.checkout_preferencia_comercial
SET estado='approved',provider_payment_status='refunded',provider_payment_updated_at=$3,actualizada_en=$4,reconciliation_reason=NULL
WHERE id=$1::uuid AND provider_payment_id=$2 AND (provider_payment_updated_at IS NULL OR provider_payment_updated_at<=$3)
  AND (estado='approved' OR (estado='reconciliation_required' AND reconciliation_reason='refund_without_cancellation'))
RETURNING marketplace_fee_clp`, payment.OperationID, payment.ProviderPaymentID, payment.PaymentUpdatedAt.UTC(), now.UTC()).Scan(&marketplaceFee)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.CommercialCancellation{}, rentalflow.ErrConflict
	}
	if err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	if err := r.storePaymentSettlement(ctx, tx, payment, marketplaceFee); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO public.reserva_comercial_cancelacion
(id,reserva_id,actor_id,rol_actor,politica_version,politica_texto,motivo,porcentaje_reembolso,monto_total_clp,monto_reembolso_clp,clave_idempotencia,provider_refund_id,refund_status,creada_en,actualizada_en,resuelta_en)
VALUES($1,$2,$3,'anfitrion',$4,$5,'Reembolso total verificado en Mercado Pago',100,$6,$6,$1::uuid::text,$7,'aprobado',$8,$8,$8)`,
		uuid.NewString(), reservationID, actor, result.PolicyVersion, result.PolicyText, result.TotalChargedCLP, refund.ID, now.UTC()); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	result.RefundPercent, result.RefundAmountCLP, result.ProviderRefundID = 100, refund.Amount, refund.ID
	result.RefundedAmountCLP, result.RefundableBalanceCLP = computeRefundableBalances(result.TotalChargedCLP, refund.Amount)
	if err := closeCommercialReservationTx(ctx, tx, result, result.Status, now); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	result.Status, result.RefundStatus, result.CanCancel, result.Reason = "reembolsada", rentalflow.CommercialRefundApproved, false, ""
	if err := tx.Commit(ctx); err != nil {
		return rentalflow.CommercialCancellation{}, err
	}
	return result, nil
}
