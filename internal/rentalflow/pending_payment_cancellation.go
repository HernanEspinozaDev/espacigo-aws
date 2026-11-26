package rentalflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/google/uuid"
)

type PendingPaymentCancellationResult struct {
	PaymentStatus string `json:"payment_status"`
}

type PendingPaymentCancellationIntent struct {
	ReservationID, ActorID, OperationID, PaymentID, HostID, SellerID, IdempotencyKey, Outcome string
	AmountCLP                                                                                 int64
	ExpiresAt                                                                                 time.Time
	TestMode, Claimed, Expired                                                                bool
}

type PendingPaymentCancellationRepository interface {
	BeginPendingPaymentCancellation(ctx context.Context, actorID, reservationID, attemptKey string, now time.Time) (PendingPaymentCancellationIntent, error)
	FinishPendingPaymentCancellation(ctx context.Context, intent PendingPaymentCancellationIntent, outcome string) error
	ExpirePendingPaymentCancellation(ctx context.Context, intent PendingPaymentCancellationIntent, now time.Time) error
	ListPendingPaymentCancellationIntents(ctx context.Context, now time.Time, limit int) ([]PendingPaymentCancellationIntent, error)
	checkoutpro.PaymentOutcomeSink
}

type pendingPaymentCancellationGateway interface {
	checkoutpro.ProviderLookup
	CancelPendingPayment(ctx context.Context, token []byte, paymentID, idempotencyKey string) error
	SearchPaymentsByExternalReference(ctx context.Context, token []byte, externalReference string) ([]checkoutpro.VerifiedPayment, error)
}

type pendingPaymentCancellationService struct {
	repository PendingPaymentCancellationRepository
	tokens     CommercialSellerTokenProvider
	gateway    pendingPaymentCancellationGateway
	now        func() time.Time
}

func (s *CommercialReservationService) SetPendingPaymentCancellation(repo PendingPaymentCancellationRepository, tokens CommercialSellerTokenProvider, gateway pendingPaymentCancellationGateway, now func() time.Time) {
	if s != nil && repo != nil && tokens != nil && gateway != nil && now != nil {
		s.pendingPaymentCancellation = &pendingPaymentCancellationService{repository: repo, tokens: tokens, gateway: gateway, now: now}
	}
}

func (s *CommercialReservationService) CancelPendingPayment(ctx context.Context, actorID, reservationID string) (PendingPaymentCancellationResult, error) {
	if s == nil || s.pendingPaymentCancellation == nil {
		return PendingPaymentCancellationResult{}, ErrUnavailable
	}
	if ctx == nil || !validUUID(actorID) || !validUUID(reservationID) {
		return PendingPaymentCancellationResult{}, ErrInvalid
	}
	return s.pendingPaymentCancellation.cancel(ctx, canonicalUUID(actorID), canonicalUUID(reservationID))
}

func (s *pendingPaymentCancellationService) cancel(ctx context.Context, actorID, reservationID string) (PendingPaymentCancellationResult, error) {
	now := s.now().UTC()
	intent, err := s.repository.BeginPendingPaymentCancellation(ctx, actorID, reservationID, uuid.NewString(), now)
	if err != nil {
		return PendingPaymentCancellationResult{}, mapCancellationError(err)
	}
	if intent.Outcome != "" {
		return PendingPaymentCancellationResult{PaymentStatus: intent.Outcome}, nil
	}
	pending := PendingPaymentCancellationResult{PaymentStatus: "pending"}
	if !intent.Claimed {
		return pending, nil
	}

	// The network attempt is shorter than the durable lease. A different
	// process can resume after the lease expires with the original key.
	attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := s.tokens.AccessToken(attemptCtx, intent.HostID)
	if err != nil || len(token) == 0 {
		return pending, nil
	}
	defer clear(token)

	if intent.PaymentID != "" {
		payment, lookupErr := s.gateway.LookupPayment(attemptCtx, token, intent.PaymentID)
		if lookupErr != nil {
			return pending, nil
		}
		return s.settlePayment(attemptCtx, token, intent, payment)
	}

	payments, err := s.gateway.SearchPaymentsByExternalReference(attemptCtx, token, intent.OperationID)
	if err != nil {
		return pending, nil
	}
	chosen, found, err := s.selectCandidatePayment(attemptCtx, token, intent, payments)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return PendingPaymentCancellationResult{}, ErrConflict
		}
		return pending, nil
	}
	if !found {
		closeAt := s.now().UTC()
		if intent.Expired || (!intent.ExpiresAt.IsZero() && !intent.ExpiresAt.After(closeAt)) {
			if err := s.repository.ExpirePendingPaymentCancellation(attemptCtx, intent, closeAt); err != nil {
				return pending, nil
			}
			return PendingPaymentCancellationResult{PaymentStatus: "cancelled"}, nil
		}
		return pending, nil
	}
	intent.PaymentID = chosen.ProviderPaymentID
	return s.settlePayment(attemptCtx, token, intent, chosen)
}

func (s *pendingPaymentCancellationService) selectCandidatePayment(ctx context.Context, token []byte, intent PendingPaymentCancellationIntent, payments []checkoutpro.VerifiedPayment) (checkoutpro.VerifiedPayment, bool, error) {
	var pendingCandidate *checkoutpro.VerifiedPayment
	var cancelledCandidate *checkoutpro.VerifiedPayment
	for _, p := range payments {
		if p.ExternalReference != intent.OperationID || p.ProviderPaymentID == "" {
			continue
		}
		if p.OperationID == "" {
			full, err := s.gateway.LookupPayment(ctx, token, p.ProviderPaymentID)
			if err != nil {
				return checkoutpro.VerifiedPayment{}, false, ErrUnavailable
			}
			p = full
		}
		candidateIntent := intent
		candidateIntent.PaymentID = p.ProviderPaymentID
		if !pendingCancellationPaymentMatches(candidateIntent, p) {
			return checkoutpro.VerifiedPayment{}, false, ErrConflict
		}
		status := strings.ToLower(strings.TrimSpace(p.Status))
		switch status {
		case "approved":
			return p, true, nil
		case "pending", "in_process", "authorized":
			if pendingCandidate == nil {
				copyPayment := p
				pendingCandidate = &copyPayment
			}
		case "cancelled":
			if cancelledCandidate == nil {
				copyPayment := p
				cancelledCandidate = &copyPayment
			}
		case "rejected":
		default:
			return checkoutpro.VerifiedPayment{}, false, ErrConflict
		}
	}
	if pendingCandidate != nil {
		return *pendingCandidate, true, nil
	}
	if cancelledCandidate != nil {
		return *cancelledCandidate, true, nil
	}
	return checkoutpro.VerifiedPayment{}, false, nil
}

func (s *pendingPaymentCancellationService) settlePayment(attemptCtx context.Context, token []byte, intent PendingPaymentCancellationIntent, payment checkoutpro.VerifiedPayment) (PendingPaymentCancellationResult, error) {
	pending := PendingPaymentCancellationResult{PaymentStatus: "pending"}
	if !pendingCancellationPaymentMatches(intent, payment) {
		return PendingPaymentCancellationResult{}, ErrConflict
	}
	status := strings.ToLower(strings.TrimSpace(payment.Status))
	switch status {
	case "pending", "in_process", "authorized":
		// A concurrent approval can make PUT fail, and a timeout can hide an
		// accepted cancellation. The subsequent GET governs in either case.
		_ = s.gateway.CancelPendingPayment(attemptCtx, token, intent.PaymentID, intent.IdempotencyKey)
		var err error
		payment, err = s.gateway.LookupPayment(attemptCtx, token, intent.PaymentID)
		if err != nil {
			return pending, nil
		}
		if !pendingCancellationPaymentMatches(intent, payment) {
			return PendingPaymentCancellationResult{}, ErrConflict
		}
		status = strings.ToLower(strings.TrimSpace(payment.Status))
	case "approved", "cancelled":
	default:
		return PendingPaymentCancellationResult{}, ErrConflict
	}
	if status != "cancelled" && status != "approved" {
		return pending, nil
	}
	if _, err := s.repository.ApplyVerifiedPayment(attemptCtx, payment); err != nil {
		return pending, nil
	}
	if err := s.repository.FinishPendingPaymentCancellation(attemptCtx, intent, status); err != nil {
		return pending, nil
	}
	return PendingPaymentCancellationResult{PaymentStatus: status}, nil
}

func pendingCancellationPaymentMatches(intent PendingPaymentCancellationIntent, p checkoutpro.VerifiedPayment) bool {
	return p.ProviderPaymentID == intent.PaymentID && p.OperationID == intent.OperationID &&
		p.ExternalReference == intent.OperationID && p.SellerID == intent.SellerID && p.Currency == "CLP" &&
		intent.AmountCLP > 0 && p.TransactionAmountCLP == intent.AmountCLP &&
		!p.PaymentUpdatedAt.IsZero() && checkoutpro.PaymentModeMatches(intent.TestMode, p.LiveMode) &&
		checkoutpro.AllowedPaymentType(intent.TestMode, p.PaymentType)
}

func (s *CommercialReservationService) ReconcilePendingPaymentCancellations(ctx context.Context, limit int) error {
	if s == nil || s.pendingPaymentCancellation == nil || ctx == nil || limit < 1 || limit > 100 {
		return ErrInvalid
	}
	worker := s.pendingPaymentCancellation
	intents, err := worker.repository.ListPendingPaymentCancellationIntents(ctx, worker.now().UTC(), limit)
	if err != nil {
		return err
	}
	var firstErr error
	for _, intent := range intents {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := worker.cancel(ctx, intent.ActorID, intent.ReservationID)
		if err != nil && !errors.Is(err, ErrConflict) && !errors.Is(err, ErrNotFound) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
