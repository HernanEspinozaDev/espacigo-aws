package rentalflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type pendingCancelRepoStub struct {
	intent         rentalflow.PendingPaymentCancellationIntent
	beginErr       error
	finishErr      error
	expireErr      error
	applyErr       error
	listIntents    []rentalflow.PendingPaymentCancellationIntent
	listErr        error
	applied        []checkoutpro.VerifiedPayment
	finishedStatus string
	finishedIntent rentalflow.PendingPaymentCancellationIntent
	expiredCalls   int
}

func (r *pendingCancelRepoStub) ListCommercialReservations(context.Context, string, int, string) (rentalflow.CommercialReservationPage, error) {
	return rentalflow.CommercialReservationPage{}, nil
}

func (r *pendingCancelRepoStub) GetCommercialReservation(context.Context, string, string) (rentalflow.CommercialReservation, error) {
	return rentalflow.CommercialReservation{}, nil
}

func (r *pendingCancelRepoStub) BeginPendingPaymentCancellation(_ context.Context, _, _, _ string, _ time.Time) (rentalflow.PendingPaymentCancellationIntent, error) {
	if r.beginErr != nil {
		return rentalflow.PendingPaymentCancellationIntent{}, r.beginErr
	}
	return r.intent, nil
}

func (r *pendingCancelRepoStub) FinishPendingPaymentCancellation(_ context.Context, intent rentalflow.PendingPaymentCancellationIntent, outcome string) error {
	if r.finishErr != nil {
		return r.finishErr
	}
	r.finishedIntent = intent
	r.finishedStatus = outcome
	return nil
}

func (r *pendingCancelRepoStub) ExpirePendingPaymentCancellation(_ context.Context, _ rentalflow.PendingPaymentCancellationIntent, _ time.Time) error {
	r.expiredCalls++
	return r.expireErr
}

func (r *pendingCancelRepoStub) ListPendingPaymentCancellationIntents(context.Context, time.Time, int) ([]rentalflow.PendingPaymentCancellationIntent, error) {
	return r.listIntents, r.listErr
}

func (r *pendingCancelRepoStub) ApplyVerifiedPayment(_ context.Context, p checkoutpro.VerifiedPayment) (bool, error) {
	if r.applyErr != nil {
		return false, r.applyErr
	}
	r.applied = append(r.applied, p)
	return false, nil
}

type pendingCancelTokensStub struct {
	token []byte
	err   error
}

func (t pendingCancelTokensStub) AccessToken(context.Context, string) ([]byte, error) {
	if t.err != nil {
		return nil, t.err
	}
	if t.token != nil {
		return append([]byte(nil), t.token...), nil
	}
	return []byte("seller-oauth"), nil
}

type pendingCancelGatewayStub struct {
	lookupPayments []checkoutpro.VerifiedPayment
	lookupErrs     []error
	lookupCalls    int
	cancelErr      error
	cancelCalls    int
	cancelKey      string
	cancelID       string
	searchResults  []checkoutpro.VerifiedPayment
	searchErr      error
	searchCalls    int
}

func (g *pendingCancelGatewayStub) LookupPayment(_ context.Context, _ []byte, _ string) (checkoutpro.VerifiedPayment, error) {
	idx := g.lookupCalls
	g.lookupCalls++
	if idx < len(g.lookupErrs) && g.lookupErrs[idx] != nil {
		return checkoutpro.VerifiedPayment{}, g.lookupErrs[idx]
	}
	if idx < len(g.lookupPayments) {
		return g.lookupPayments[idx], nil
	}
	if len(g.lookupPayments) > 0 {
		return g.lookupPayments[len(g.lookupPayments)-1], nil
	}
	return checkoutpro.VerifiedPayment{}, checkoutpro.ErrNotFound
}

func (g *pendingCancelGatewayStub) CancelPendingPayment(_ context.Context, _ []byte, paymentID, key string) error {
	g.cancelCalls++
	g.cancelID = paymentID
	g.cancelKey = key
	return g.cancelErr
}

func (g *pendingCancelGatewayStub) SearchPaymentsByExternalReference(_ context.Context, _ []byte, _ string) ([]checkoutpro.VerifiedPayment, error) {
	g.searchCalls++
	return g.searchResults, g.searchErr
}

func newPendingCancelTestFixtures(now time.Time) (rentalflow.PendingPaymentCancellationIntent, checkoutpro.VerifiedPayment) {
	const (
		resID  = "11111111-1111-4111-8111-111111111111"
		opID   = "22222222-2222-4222-8222-222222222222"
		actor  = "33333333-3333-4333-8333-333333333333"
		hostID = "44444444-4444-4444-8444-444444444444"
		key    = "55555555-5555-4555-8555-555555555555"
	)
	intent := rentalflow.PendingPaymentCancellationIntent{
		ReservationID:  resID,
		ActorID:        actor,
		OperationID:    opID,
		PaymentID:      "10001",
		HostID:         hostID,
		SellerID:       "91",
		IdempotencyKey: key,
		AmountCLP:      12000,
		ExpiresAt:      now.Add(10 * time.Minute),
		TestMode:       false,
		Claimed:        true,
	}
	payment := checkoutpro.VerifiedPayment{
		OperationID:          opID,
		ProviderPaymentID:    "10001",
		SellerID:             "91",
		ExternalReference:    opID,
		Status:               "pending",
		TransactionAmountCLP: 12000,
		Currency:             "CLP",
		PaymentType:          "account_money",
		LiveMode:             true,
		PaymentUpdatedAt:     now,
	}
	return intent, payment
}

func TestPendingPaymentCancellationAcceptedAndIdempotent(t *testing.T) {
	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)
	intent, pending := newPendingCancelTestFixtures(now)
	cancelled := pending
	cancelled.Status = "cancelled"

	repo := &pendingCancelRepoStub{intent: intent}
	gateway := &pendingCancelGatewayStub{lookupPayments: []checkoutpro.VerifiedPayment{pending, cancelled}}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

	got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "cancelled" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if gateway.cancelCalls != 1 || gateway.cancelKey != intent.IdempotencyKey || len(repo.applied) != 1 || repo.finishedStatus != "cancelled" {
		t.Fatalf("unexpected side effects: cancelCalls=%d key=%s applied=%d finished=%q", gateway.cancelCalls, gateway.cancelKey, len(repo.applied), repo.finishedStatus)
	}

	// Repeated request when outcome already persisted does not call provider again.
	repo.intent.Outcome = "cancelled"
	got, err = service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "cancelled" || gateway.cancelCalls != 1 || gateway.lookupCalls != 2 {
		t.Fatalf("replay mutated provider state: got=%+v err=%v calls=%d/%d", got, err, gateway.cancelCalls, gateway.lookupCalls)
	}
}

func TestPendingPaymentCancellationUnclaimedLeaseAndTimeoutRecovery(t *testing.T) {
	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)
	intent, pending := newPendingCancelTestFixtures(now)

	// Unclaimed lease returns pending without contacting gateway.
	intent.Claimed = false
	repo := &pendingCancelRepoStub{intent: intent}
	gateway := &pendingCancelGatewayStub{}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

	got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "pending" || gateway.lookupCalls != 0 || gateway.cancelCalls != 0 {
		t.Fatalf("unclaimed lease: got=%+v err=%v lookups=%d cancels=%d", got, err, gateway.lookupCalls, gateway.cancelCalls)
	}

	// Ambiguous PUT timeout followed by GET confirming cancellation succeeds.
	repo.intent.Claimed = true
	cancelled := pending
	cancelled.Status = "cancelled"
	gateway = &pendingCancelGatewayStub{
		lookupPayments: []checkoutpro.VerifiedPayment{pending, cancelled},
		cancelErr:      errors.New("timeout"),
	}
	service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })
	got, err = service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "cancelled" || repo.finishedStatus != "cancelled" {
		t.Fatalf("timeout PUT recovered by GET: got=%+v err=%v finished=%q", got, err, repo.finishedStatus)
	}

	// Second GET timeout leaves it pending without finalizing.
	repo2 := &pendingCancelRepoStub{intent: repo.intent}
	gateway2 := &pendingCancelGatewayStub{
		lookupPayments: []checkoutpro.VerifiedPayment{pending},
		lookupErrs:     []error{nil, errors.New("timeout")},
		cancelErr:      errors.New("timeout"),
	}
	service.SetPendingPaymentCancellation(repo2, pendingCancelTokensStub{}, gateway2, func() time.Time { return now })
	got, err = service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "pending" || len(repo2.applied) != 0 || repo2.finishedStatus != "" {
		t.Fatalf("ambiguous GET should remain pending: got=%+v err=%v applied=%d finished=%q", got, err, len(repo2.applied), repo2.finishedStatus)
	}
}

func TestPendingPaymentCancellationConcurrentApprovalPreservesPaidReservation(t *testing.T) {
	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)
	intent, pending := newPendingCancelTestFixtures(now)
	approved := pending
	approved.Status = "approved"

	repo := &pendingCancelRepoStub{intent: intent}
	gateway := &pendingCancelGatewayStub{
		lookupPayments: []checkoutpro.VerifiedPayment{pending, approved},
		cancelErr:      &checkoutpro.ProviderError{Stage: "payment_cancel", HTTPStatus: 400, ProviderCode: "bad_request"},
	}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

	got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
	if err != nil || got.PaymentStatus != "approved" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(repo.applied) != 1 || repo.applied[0].Status != "approved" || repo.finishedStatus != "approved" {
		t.Fatalf("approved payment was not persisted: applied=%+v finished=%q", repo.applied, repo.finishedStatus)
	}
}

func TestPendingPaymentCancellationMissingPaymentIDExternalReference(t *testing.T) {
	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("finds pending payment via external_reference without OperationID and cancels via LookupPayment", func(t *testing.T) {
		intent, pending := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		searchResult := pending
		searchResult.OperationID = ""
		cancelled := pending
		cancelled.Status = "cancelled"

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{
			searchResults:  []checkoutpro.VerifiedPayment{searchResult},
			lookupPayments: []checkoutpro.VerifiedPayment{pending, cancelled},
		}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
		if err != nil || got.PaymentStatus != "cancelled" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if gateway.searchCalls != 1 || gateway.cancelCalls != 1 || gateway.cancelID != "10001" || repo.finishedIntent.PaymentID != "10001" {
			t.Fatalf("unexpected calls: search=%d cancel=%d id=%q finishedID=%q", gateway.searchCalls, gateway.cancelCalls, gateway.cancelID, repo.finishedIntent.PaymentID)
		}
	})

	t.Run("prefers approved over rejected and pending in multiple search results", func(t *testing.T) {
		intent, pending := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		rejected := pending
		rejected.ProviderPaymentID = "9999"
		rejected.Status = "rejected"
		approved := pending
		approved.ProviderPaymentID = "10002"
		approved.Status = "approved"

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{
			searchResults: []checkoutpro.VerifiedPayment{rejected, pending, approved},
		}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
		if err != nil || got.PaymentStatus != "approved" || gateway.cancelCalls != 0 || repo.finishedIntent.PaymentID != "10002" {
			t.Fatalf("got=%+v err=%v cancelCalls=%d finishedID=%q", got, err, gateway.cancelCalls, repo.finishedIntent.PaymentID)
		}
	})

	t.Run("no payment found and not expired stays pending without releasing availability", func(t *testing.T) {
		intent, _ := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		intent.ExpiresAt = now.Add(5 * time.Minute)

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{searchResults: nil}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
		if err != nil || got.PaymentStatus != "pending" || repo.expiredCalls != 0 {
			t.Fatalf("got=%+v err=%v expiredCalls=%d", got, err, repo.expiredCalls)
		}
	})

	t.Run("no payment found and local checkout expired closes locally as cancelled", func(t *testing.T) {
		intent, _ := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		intent.ExpiresAt = now.Add(-time.Second)

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{searchResults: nil}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
		if err != nil || got.PaymentStatus != "cancelled" || repo.expiredCalls != 1 {
			t.Fatalf("got=%+v err=%v expiredCalls=%d", got, err, repo.expiredCalls)
		}
	})

	t.Run("uncertain provider search failure does not expire locally even past expiry", func(t *testing.T) {
		intent, _ := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		intent.ExpiresAt = now.Add(-time.Minute)

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{searchErr: errors.New("provider timeout")}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		got, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID)
		if err != nil || got.PaymentStatus != "pending" || repo.expiredCalls != 0 {
			t.Fatalf("got=%+v err=%v expiredCalls=%d", got, err, repo.expiredCalls)
		}
	})

	t.Run("mismatched seller or amount in search results returns ErrConflict", func(t *testing.T) {
		intent, pending := newPendingCancelTestFixtures(now)
		intent.PaymentID = ""
		bad := pending
		bad.SellerID = "999"

		repo := &pendingCancelRepoStub{intent: intent}
		gateway := &pendingCancelGatewayStub{searchResults: []checkoutpro.VerifiedPayment{bad}}
		service, _ := rentalflow.NewCommercialReservationService(repo)
		service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

		if _, err := service.CancelPendingPayment(context.Background(), intent.ActorID, intent.ReservationID); !errors.Is(err, rentalflow.ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})
}

func TestReconcilePendingPaymentCancellationsProcessesList(t *testing.T) {
	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)
	intent, pending := newPendingCancelTestFixtures(now)
	cancelled := pending
	cancelled.Status = "cancelled"

	repo := &pendingCancelRepoStub{
		intent:      intent,
		listIntents: []rentalflow.PendingPaymentCancellationIntent{{ReservationID: intent.ReservationID, ActorID: intent.ActorID}},
	}
	gateway := &pendingCancelGatewayStub{lookupPayments: []checkoutpro.VerifiedPayment{pending, cancelled}}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetPendingPaymentCancellation(repo, pendingCancelTokensStub{}, gateway, func() time.Time { return now })

	if err := service.ReconcilePendingPaymentCancellations(context.Background(), 10); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if repo.finishedStatus != "cancelled" {
		t.Fatalf("expected finished cancelled, got %q", repo.finishedStatus)
	}
}
