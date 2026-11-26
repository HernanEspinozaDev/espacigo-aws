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

var ErrCancellationUnavailable = errors.New("rental flow: cancellation unavailable")

type CommercialCancellation struct {
	ReservationID     string `json:"reservation_id"`
	CanCancel         bool   `json:"can_cancel"`
	Reason            string `json:"reason"`
	PolicyVersion     string `json:"cancellation_policy_version"`
	PolicyText        string `json:"cancellation_policy_text"`
	RefundPercent     int    `json:"refund_percent"`
	TotalChargedCLP   int64  `json:"total_charged_clp"`
	RefundAmountCLP   int64  `json:"refund_amount_clp"`
	Currency          string `json:"currency"`
	StartsAt          string `json:"starts_at"`
	Status            string `json:"status,omitempty"`
	RefundStatus      string `json:"refund_status,omitempty"`
	ProviderPaymentID string `json:"-"`
	ProviderRefundID  string `json:"-"`
	HostID            string `json:"-"`
	ActorID           string `json:"-"`
	IdempotencyKey    string `json:"-"`
	Motivo            string `json:"-"`
}

type CommercialCancellationRepository interface {
	PreviewCommercialCancellation(context.Context, string, string, time.Time) (CommercialCancellation, error)
	BeginCommercialCancellation(context.Context, string, string, string, int64, time.Time, string) (CommercialCancellation, error)
	FinishCommercialCancellation(context.Context, string, string, string, time.Time) (CommercialCancellation, error)
	FailCommercialCancellation(context.Context, string, time.Time) error
	ListPendingCommercialCancellations(context.Context, int) ([]CommercialCancellation, error)
}

type CommercialRefundGateway interface {
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
	if operation.RefundStatus == "aprobado" || operation.RefundStatus == "no_aplica" {
		return operation, nil
	}
	if operation.RefundAmountCLP == 0 {
		result, finishErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, "", "no_aplica", s.cancellationNow().UTC())
		return result, mapCancellationError(finishErr)
	}
	if operation.ProviderPaymentID == "" || operation.HostID == "" || operation.IdempotencyKey == "" {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	token, err := s.cancellationTokens.AccessToken(ctx, operation.HostID)
	if err != nil || len(token) == 0 {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	defer clear(token)
	if operation.ProviderRefundID != "" {
		refunds, lookupErr := s.refundGateway.ListRefunds(ctx, token, operation.ProviderPaymentID)
		if lookupErr != nil {
			return CommercialCancellation{}, ErrCancellationUnavailable
		}
		for _, item := range refunds {
			if item.ID != operation.ProviderRefundID {
				continue
			}
			if item.Amount != operation.RefundAmountCLP {
				return CommercialCancellation{}, ErrCancellationUnavailable
			}
			switch item.Status {
			case "approved":
				result, finishErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, item.ID, "aprobado", s.cancellationNow().UTC())
				return result, mapCancellationError(finishErr)
			case "in_process", "authorized":
				result, finishErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, item.ID, "pendiente", s.cancellationNow().UTC())
				return result, mapCancellationError(finishErr)
			case "rejected", "cancelled":
				_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, s.cancellationNow().UTC())
				return CommercialCancellation{}, ErrCancellationUnavailable
			}
		}
		// A persisted provider refund ID means the request was accepted. If it
		// is temporarily absent from the provider listing, keep waiting instead
		// of issuing another POST.
		result, finishErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, operation.ProviderRefundID, "pendiente", s.cancellationNow().UTC())
		return result, mapCancellationError(finishErr)
	}
	var amount *int64
	if operation.RefundAmountCLP < operation.TotalChargedCLP {
		partialAmount := operation.RefundAmountCLP
		amount = &partialAmount
	}
	refund, err := s.refundGateway.RefundPayment(ctx, token, operation.ProviderPaymentID, amount, operation.IdempotencyKey)
	if err != nil {
		var providerErr *checkoutpro.ProviderError
		if errors.As(err, &providerErr) {
			log.Printf("mercadopago_refund stage=refund_create payment_id=%q provider_http_status=%d provider_code=%q", operation.ProviderPaymentID, providerErr.HTTPStatus, providerErr.ProviderCode)
			if providerErr.HTTPStatus >= 400 && providerErr.HTTPStatus < 500 && providerErr.HTTPStatus != 408 && providerErr.HTTPStatus != 409 && providerErr.HTTPStatus != 429 {
				_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, s.cancellationNow().UTC())
			}
		}
		// The persisted idempotency key makes the next identical request safe even
		// when the provider outcome was lost after accepting the refund.
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	if refund.Amount != operation.RefundAmountCLP {
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	if refund.Status == "rejected" || refund.Status == "cancelled" {
		_ = s.cancellations.FailCommercialCancellation(ctx, operation.ReservationID, s.cancellationNow().UTC())
		return CommercialCancellation{}, ErrCancellationUnavailable
	}
	if refund.Status == "in_process" || refund.Status == "authorized" {
		result, pendingErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, refund.ID, "pendiente", s.cancellationNow().UTC())
		return result, mapCancellationError(pendingErr)
	}
	result, finishErr := s.cancellations.FinishCommercialCancellation(ctx, operation.ReservationID, refund.ID, "aprobado", s.cancellationNow().UTC())
	return result, mapCancellationError(finishErr)
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
