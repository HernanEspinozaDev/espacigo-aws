package rentalflow

import (
	"context"
	"regexp"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

var providerRefundIDPattern = regexp.MustCompile(`^[0-9]{1,64}$`)

type externalFullRefundRepository interface {
	RecordExternalFullCommercialRefund(context.Context, string, string, checkoutpro.VerifiedPayment, checkoutpro.RefundResult, time.Time) (CommercialCancellation, error)
}

func (p CommercialCancellation) CanLinkExternalFullRefund(now time.Time) bool {
	startsAt, err := time.Parse(time.RFC3339Nano, p.StartsAt)
	return err == nil && now.Before(startsAt) && p.RefundStatus == "" && p.TotalChargedCLP > 0 &&
		(p.Status == "pagada" || p.Status == "reconciliation_required" && p.CheckoutReconciliationReason == "refund_without_cancellation")
}

// ReconcileExternalRefund only reads from Mercado Pago. A refund already
// approved there can close a matching intent or a future reservation after
// the entire original payment was refunded through a single provider record.
func (s *CommercialReservationService) ReconcileExternalRefund(ctx context.Context, actorID, reservationID, refundID string) (CommercialCancellation, error) {
	if ctx == nil || !validUUID(actorID) || !validUUID(reservationID) ||
		!providerRefundIDPattern.MatchString(refundID) {
		return CommercialCancellation{}, ErrInvalid
	}
	if s == nil || s.cancellations == nil || s.cancellationNow == nil || s.cancellationTokens == nil || s.refundGateway == nil {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	actor := canonicalUUID(actorID)
	now := s.cancellationNow().UTC()
	operation, err := s.cancellations.PreviewCommercialCancellation(ctx, actor, canonicalUUID(reservationID), now)
	if err != nil {
		return CommercialCancellation{}, mapCancellationError(err)
	}
	if operation.HostID != actor {
		return CommercialCancellation{}, ErrNotFound
	}
	if operation.RefundStatus == CommercialRefundApproved && operation.ProviderRefundID == refundID {
		return operation, nil
	}
	externalFull := operation.CanLinkExternalFullRefund(now)
	if !externalFull {
		if operation.RefundStatus != CommercialRefundPending || operation.IdempotencyKey == "" ||
			operation.RefundAmountCLP <= 0 || operation.RefundAmountCLP > operation.TotalChargedCLP ||
			operation.ProviderRefundID != "" && operation.ProviderRefundID != refundID {
			return CommercialCancellation{}, ErrConflict
		}
		refundableBalance := operation.TotalChargedCLP - operation.RefundedAmountCLP
		if operation.RefundedAmountCLP < 0 || refundableBalance < 0 {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		if operation.RefundableBalanceCLP > 0 && operation.RefundableBalanceCLP < refundableBalance {
			refundableBalance = operation.RefundableBalanceCLP
		}
		if operation.RefundAmountCLP > refundableBalance {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
	}
	token, err := s.cancellationTokens.AccessToken(ctx, operation.HostID)
	if err != nil || len(token) == 0 {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	defer clear(token)
	payment, err := s.verifiedCommercialRefundPayment(ctx, operation, token)
	if err != nil {
		return CommercialCancellation{}, err
	}
	if payment.Settlement != nil && payment.Settlement.PaymentStatus == "charged_back" {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	if payment.Status != "approved" && payment.Status != "refunded" {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	refunds, err := s.refundGateway.ListRefunds(ctx, token, operation.ProviderPaymentID)
	if err != nil {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	var match checkoutpro.RefundResult
	var priorApprovedSum int64
	for _, refund := range refunds {
		if refund.Status == "rejected" || refund.Status == "cancelled" {
			continue
		}
		if refund.Status != "approved" {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		if refund.ID == refundID {
			if match.ID != "" {
				return CommercialCancellation{}, ErrRefundRequiresReconciliation
			}
			match = refund
			continue
		}
		if externalFull || operation.RefundedAmountCLP == 0 {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		priorApprovedSum += refund.Amount
	}
	if match.ID == "" {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	if !externalFull && operation.RefundedAmountCLP > 0 && priorApprovedSum > 0 && priorApprovedSum != operation.RefundedAmountCLP {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	if payment.Settlement != nil && payment.Settlement.RefundedCLP != nil {
		expectedRefunded := match.Amount
		if !externalFull && operation.RefundedAmountCLP > 0 {
			expectedRefunded = operation.RefundedAmountCLP + match.Amount
		}
		if *payment.Settlement.RefundedCLP != expectedRefunded || *payment.Settlement.RefundedCLP > operation.TotalChargedCLP {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
	}
	if externalFull {
		if payment.Status != "refunded" || match.Amount != operation.TotalChargedCLP {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		writer, ok := s.cancellations.(externalFullRefundRepository)
		if !ok {
			return CommercialCancellation{}, ErrCancellationUnavailable
		}
		result, err := writer.RecordExternalFullCommercialRefund(ctx, actor, operation.ReservationID, payment, match, now)
		return result, mapCancellationError(err)
	}
	return s.applyCommercialRefundResult(ctx, operation, match)
}
