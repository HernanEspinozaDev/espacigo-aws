package rentalflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/google/uuid"
)

var (
	ErrCancellationUnavailable      = errors.New("rental flow: cancellation unavailable")
	ErrRefundRequiresReconciliation = errors.New("rental flow: refund requires reconciliation")
	ErrRefundInsufficientFunds      = errors.New("rental flow: refund insufficient funds")
)

const (
	CommercialRefundApproved = "aprobado"
	CommercialRefundPending  = "pendiente"
)

type CommercialCancellation struct {
	ReservationID                string `json:"reservation_id"`
	CanCancel                    bool   `json:"can_cancel"`
	Reason                       string `json:"reason"`
	PolicyVersion                string `json:"cancellation_policy_version"`
	PolicyText                   string `json:"cancellation_policy_text"`
	RefundPercent                int    `json:"refund_percent"`
	TotalChargedCLP              int64  `json:"total_charged_clp"`
	RefundAmountCLP              int64  `json:"refund_amount_clp"`
	RefundedAmountCLP            int64  `json:"refunded_amount_clp"`
	RefundableBalanceCLP         int64  `json:"refundable_balance_clp"`
	Currency                     string `json:"currency"`
	StartsAt                     string `json:"starts_at"`
	Status                       string `json:"status,omitempty"`
	RefundStatus                 string `json:"refund_status,omitempty"`
	ProviderPaymentID            string `json:"-"`
	CheckoutOperationID          string `json:"-"`
	CheckoutReconciliationReason string `json:"-"`
	SellerID                     string `json:"-"`
	TestMode                     bool   `json:"-"`
	ProviderRefundID             string `json:"-"`
	HostID                       string `json:"-"`
	ActorID                      string `json:"-"`
	IdempotencyKey               string `json:"-"`
	Motivo                       string `json:"-"`
}

type CommercialCancellationRepository interface {
	PreviewCommercialCancellation(context.Context, string, string, time.Time) (CommercialCancellation, error)
	BeginCommercialCancellation(context.Context, string, string, string, int64, time.Time, string) (CommercialCancellation, error)
	FinishCommercialCancellation(ctx context.Context, reservationID, attemptKey, refundID, status string, at time.Time) (CommercialCancellation, error)
	FailCommercialCancellation(ctx context.Context, reservationID, attemptKey, refundID string, at time.Time) error
	ListPendingCommercialCancellations(context.Context, int) ([]CommercialCancellation, error)
}

type CommercialRefundGateway interface {
	LookupPayment(context.Context, []byte, string) (checkoutpro.VerifiedPayment, error)
	RefundPayment(context.Context, []byte, string, *int64, string) (checkoutpro.RefundResult, error)
	ListRefunds(context.Context, []byte, string) ([]checkoutpro.RefundResult, error)
}
type CommercialSellerTokenProvider interface {
	AccessToken(context.Context, string) ([]byte, error)
}

type commercialCancellationClock func() time.Time

func (s *CommercialReservationService) SetCancellationDependencies(repo CommercialCancellationRepository, tokens CommercialSellerTokenProvider, gateway CommercialRefundGateway, now func() time.Time) {
	if s != nil {
		s.cancellations, s.cancellationTokens, s.refundGateway, s.cancellationNow = repo, tokens, gateway, now
	}
}

func (s *CommercialReservationService) PreviewCancellation(ctx context.Context, actorID, reservationID string) (CommercialCancellation, error) {
	if s == nil || ctx == nil || !validUUID(actorID) || !validUUID(reservationID) || s.cancellations == nil || s.cancellationNow == nil {
		return CommercialCancellation{}, ErrInvalid
	}
	result, err := s.cancellations.PreviewCommercialCancellation(ctx, canonicalUUID(actorID), canonicalUUID(reservationID), s.cancellationNow().UTC())
	return result, mapCancellationError(err)
}

func (s *CommercialReservationService) Cancel(ctx context.Context, actorID, reservationID, reason string, expectedRefundAmountCLP int64) (CommercialCancellation, error) {
	if s == nil || ctx == nil || !validUUID(actorID) || !validUUID(reservationID) || s.cancellations == nil || s.cancellationNow == nil || s.cancellationTokens == nil || s.refundGateway == nil {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	reason = strings.TrimSpace(reason)
	if len(reason) < 1 || len(reason) > 500 || expectedRefundAmountCLP < 0 {
		return CommercialCancellation{}, ErrInvalid
	}
	requestID := uuid.NewString()
	now := s.cancellationNow().UTC()
	operation, err := s.cancellations.BeginCommercialCancellation(ctx, canonicalUUID(actorID), canonicalUUID(reservationID), reason, expectedRefundAmountCLP, now, requestID)
	if err != nil {
		return CommercialCancellation{}, mapCancellationError(err)
	}
	if operation.RefundStatus == CommercialRefundApproved || operation.RefundStatus == "no_aplica" {
		return operation, nil
	}
	if operation.RefundAmountCLP <= 0 || operation.RefundAmountCLP > operation.TotalChargedCLP ||
		operation.ProviderPaymentID == "" || operation.CheckoutOperationID == "" || operation.SellerID == "" ||
		operation.HostID == "" || operation.IdempotencyKey == "" {
		return CommercialCancellation{}, ErrCancellationUnavailable
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
	if payment.Status != "approved" && (operation.ProviderRefundID == "" || payment.Status != "refunded") {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	if payment.Settlement != nil && payment.Settlement.RefundedCLP != nil {
		providerRefunded := *payment.Settlement.RefundedCLP
		if providerRefunded < 0 || providerRefunded > operation.TotalChargedCLP {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		if operation.ProviderRefundID == "" && providerRefunded != operation.RefundedAmountCLP {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		if providerBalance := operation.TotalChargedCLP - providerRefunded; providerBalance < refundableBalance {
			refundableBalance = providerBalance
		}
		if operation.ProviderRefundID == "" && operation.RefundAmountCLP > refundableBalance {
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
	}
	refunds, err := s.refundGateway.ListRefunds(ctx, token, operation.ProviderPaymentID)
	if err != nil {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	if operation.ProviderRefundID != "" {
		for _, item := range refunds {
			if item.ID == operation.ProviderRefundID {
				return s.applyCommercialRefundResult(ctx, operation, item)
			}
		}
		// A known refund may be temporarily absent from the listing. Keep the
		// saved intent and let reconciliation query the provider again.
		operation.RefundStatus = CommercialRefundPending
		return operation, nil
	}
	var approvedSum int64
	for _, item := range refunds {
		if item.Status == "rejected" || item.Status == "cancelled" {
			continue
		}
		if item.Status != "approved" {
			// Any pending or unrecognized refund blocks issuing another POST.
			return CommercialCancellation{}, ErrRefundRequiresReconciliation
		}
		approvedSum += item.Amount
	}
	if len(refunds) > 0 && approvedSum != operation.RefundedAmountCLP {
		// An unlinked or mismatched approved refund cannot be assumed to match
		// our local state. Keep the attempt for reconciliation without another POST.
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	var amount *int64
	if operation.RefundAmountCLP < operation.TotalChargedCLP || operation.RefundedAmountCLP > 0 {
		explicitAmount := operation.RefundAmountCLP
		amount = &explicitAmount
	}
	refund, err := s.refundGateway.RefundPayment(ctx, token, operation.ProviderPaymentID, amount, operation.IdempotencyKey)
	if err != nil {
		var providerErr *checkoutpro.ProviderError
		if errors.As(err, &providerErr) {
			log.Printf("mercadopago_refund stage=refund_create payment_id=%q provider_http_status=%d provider_code=%q", operation.ProviderPaymentID, providerErr.HTTPStatus, providerErr.ProviderCode)
			if providerErr.ProviderCode == "insufficient_funds" {
				_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, operation.IdempotencyKey, operation.ProviderRefundID, s.cancellationNow().UTC())
				return CommercialCancellation{}, ErrRefundInsufficientFunds
			}
			if providerErr.HTTPStatus >= 400 && providerErr.HTTPStatus < 500 && providerErr.HTTPStatus != 408 && providerErr.HTTPStatus != 409 && providerErr.HTTPStatus != 429 {
				_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, operation.IdempotencyKey, operation.ProviderRefundID, s.cancellationNow().UTC())
			}
		}
		// The persisted idempotency key makes the next identical request safe even
		// when the provider outcome was lost after accepting the refund.
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	return s.applyCommercialRefundResult(ctx, operation, refund)
}

func (s *CommercialReservationService) verifiedCommercialRefundPayment(ctx context.Context, operation CommercialCancellation, token []byte) (checkoutpro.VerifiedPayment, error) {
	if operation.ProviderPaymentID == "" || operation.CheckoutOperationID == "" || operation.SellerID == "" {
		return checkoutpro.VerifiedPayment{}, ErrCancellationUnavailable
	}
	payment, err := s.refundGateway.LookupPayment(ctx, token, operation.ProviderPaymentID)
	if err != nil {
		return checkoutpro.VerifiedPayment{}, ErrCancellationUnavailable
	}
	if payment.ProviderPaymentID != operation.ProviderPaymentID || payment.OperationID != operation.CheckoutOperationID ||
		payment.ExternalReference != operation.CheckoutOperationID || payment.SellerID != operation.SellerID || payment.Currency != "CLP" ||
		payment.TransactionAmountCLP != operation.TotalChargedCLP || !checkoutpro.PaymentModeMatches(operation.TestMode, payment.LiveMode) {
		return checkoutpro.VerifiedPayment{}, ErrRefundRequiresReconciliation
	}
	return payment, nil
}

func (s *CommercialReservationService) applyCommercialRefundResult(ctx context.Context, operation CommercialCancellation, refund checkoutpro.RefundResult) (CommercialCancellation, error) {
	if refund.ID == "" || refund.Amount != operation.RefundAmountCLP {
		return CommercialCancellation{}, ErrRefundRequiresReconciliation
	}
	now := s.cancellationNow().UTC()
	var status string
	switch refund.Status {
	case "rejected", "cancelled":
		_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, operation.IdempotencyKey, operation.ProviderRefundID, now)
		return CommercialCancellation{}, ErrCancellationUnavailable
	case "in_process", "authorized":
		status = CommercialRefundPending
	case "approved":
		status = CommercialRefundApproved
	default:
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	result, err := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, operation.IdempotencyKey, refund.ID, status, now)
	return result, mapCancellationError(err)
}

// ReconcilePendingCancellations retries durable requests with their original
// idempotency keys. Multiple instances may run it safely: retries use the same
// provider key and never create a second refund operation.
func (s *CommercialReservationService) ReconcilePendingCancellations(ctx context.Context, limit int) error {
	if s == nil || ctx == nil || s.cancellations == nil || limit < 1 || limit > 100 {
		return ErrInvalid
	}
	pending, err := s.cancellations.ListPendingCommercialCancellations(ctx, limit)
	if err != nil {
		return ErrCancellationUnavailable
	}
	var firstErr error
	for _, op := range pending {
		if _, err := s.Cancel(ctx, op.ActorID, op.ReservationID, op.Motivo, op.RefundAmountCLP); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func mapCancellationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrCancellationUnavailable, err)
}

func (s *CommercialReservationService) cancellationClock() time.Time {
	if s != nil && s.cancellationNow != nil {
		return s.cancellationNow().UTC()
	}
	return time.Now().UTC()
}
