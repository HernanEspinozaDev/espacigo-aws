package checkoutapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

type paymentRepoStub struct {
	quote checkoutpro.Quote
	op    *checkoutpro.Operation
	claim bool
}

func (r *paymentRepoStub) ByQuote(_ context.Context, _, _ string) (checkoutpro.Quote, *checkoutpro.Operation, error) {
	return r.quote, r.op, nil
}

func (r *paymentRepoStub) BeginPayment(_ context.Context, _, _, _, id string, _ []byte, _ time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	r.op = &checkoutpro.Operation{
		ID: id, QuoteID: r.quote.ID, Status: "pending", CheckoutMethod: method,
		ExpiresAt: r.quote.ExpiresAt, TestMode: true, Amounts: checkoutpro.Amounts{BaseCLP: r.quote.SubtotalCLP},
	}
	return r.quote, *r.op, true, nil
}

func (r *paymentRepoStub) ClaimPayment(_ context.Context, _ string, _ time.Time) (bool, error) {
	if r.claim {
		return false, nil
	}
	r.claim = true
	return true, nil
}

func (r *paymentRepoStub) ApplyVerifiedPayment(_ context.Context, _ checkoutpro.VerifiedPayment) (bool, error) {
	return false, errors.New("unexpected verification")
}

type testOAuthToken struct{}

func (testOAuthToken) AccessToken(_ context.Context, _ string) ([]byte, error) {
	return []byte("test-seller-token"), nil
}

type testOAuthSellerKey struct {
	testOAuthToken
	key       string
	available bool
	err       error
}

func (s testOAuthSellerKey) PublicKey(context.Context, string) (string, bool, error) {
	return s.key, s.available, s.err
}

type uncertainGateway struct {
	calls   int
	request PaymentRequest
}

func (g *uncertainGateway) Create(_ context.Context, _ []byte, _ string, request PaymentRequest) (Payment, error) {
	g.calls++
	g.request = request
	return Payment{}, errors.New("provider response lost")
}

func TestUncertainPaymentUsesOfficialPriceAndCannotBeSentTwice(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	quoteID := "11111111-1111-4111-8111-111111111111"
	renterID := "22222222-2222-4222-8222-222222222222"
	key := "33333333-3333-4333-8333-333333333333"
	opID := "44444444-4444-4444-8444-444444444444"
	repo := &paymentRepoStub{quote: checkoutpro.Quote{
		ID: quoteID, RenterID: renterID, Currency: "CLP", SubtotalCLP: 12000, ExpiresAt: now.Add(time.Minute),
	}}
	gateway := &uncertainGateway{}
	service, err := New(repo, testOAuthToken{}, gateway, func() time.Time { return now }, func() (string, error) { return opID, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Token: "fake-card-token-1234", PaymentMethodID: "master", Payer: Payer{Email: "buyer@example.test"}}
	result, err := service.Pay(context.Background(), renterID, quoteID, key, input)
	if err != nil || result.Status != "reconciliation_required" {
		t.Fatalf("unexpected uncertain result: %+v, %v", result, err)
	}
	if gateway.request.TransactionAmountCLP != 12000 || gateway.request.ApplicationFeeCLP != 0 || gateway.request.ExternalReference != opID {
		t.Fatal("test payment should charge the trusted quote total without an EspaciGo application fee")
	}
	input.Token = "another-fake-token-1234"
	replayed, err := service.Pay(context.Background(), renterID, quoteID, key, input)
	if err != nil || replayed.Status != "pending" || gateway.calls != 1 {
		t.Fatalf("unexpected replay: %+v, %v, calls %d", replayed, err, gateway.calls)
	}
}

func TestPaymentStatusExposesOnlyActiveSellerPublicKeyWhenOptedIn(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	quote := checkoutpro.Quote{ID: "11111111-1111-4111-8111-111111111111", HostID: "host", RenterID: "22222222-2222-4222-8222-222222222222", Currency: "CLP", SubtotalCLP: 12000, ExpiresAt: now.Add(time.Minute)}
	key := "TEST-12345678901234567890"
	service, err := NewWithOptions(&paymentRepoStub{quote: quote}, testOAuthSellerKey{key: key, available: true}, &uncertainGateway{}, func() time.Time { return now }, func() (string, error) { return "44444444-4444-4444-8444-444444444444", nil }, nil, Options{SellerPublicKeyTokenization: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.PaymentStatus(context.Background(), quote.RenterID, quote.ID)
	if err != nil || result.CardTokenization == nil || result.CardTokenization.Mode != "seller_public_key" || result.CardTokenization.PublicKey != key || result.CardTokenization.ReconnectRequired {
		t.Fatalf("seller key mode was not returned: %+v err=%v", result.CardTokenization, err)
	}

	service, err = NewWithOptions(&paymentRepoStub{quote: quote}, testOAuthSellerKey{}, &uncertainGateway{}, func() time.Time { return now }, func() (string, error) { return "44444444-4444-4444-8444-444444444444", nil }, nil, Options{SellerPublicKeyTokenization: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.PaymentStatus(context.Background(), quote.RenterID, quote.ID)
	if err != nil || result.CardTokenization == nil || !result.CardTokenization.ReconnectRequired || result.CardTokenization.PublicKey != "" {
		t.Fatalf("legacy seller connection should require reconnect without revealing another key: %+v err=%v", result.CardTokenization, err)
	}
}

func TestSellerPublicKeyModeRejectsStaleTokenizationKeyBeforeCreatingOperation(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	quote := checkoutpro.Quote{ID: "11111111-1111-4111-8111-111111111111", HostID: "host", RenterID: "22222222-2222-4222-8222-222222222222", Currency: "CLP", SubtotalCLP: 12000, ExpiresAt: now.Add(time.Minute)}
	repo := &paymentRepoStub{quote: quote}
	gateway := &uncertainGateway{}
	service, err := NewWithOptions(repo, testOAuthSellerKey{key: "TEST-12345678901234567890", available: true}, gateway, func() time.Time { return now }, func() (string, error) { return "44444444-4444-4444-8444-444444444444", nil }, nil, Options{SellerPublicKeyTokenization: true})
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Token: "fake-card-token-1234", PaymentMethodID: "master", TokenizationPublicKey: "TEST-oldkey123456789012", Payer: Payer{Email: "buyer@example.test"}}
	_, err = service.Pay(context.Background(), quote.RenterID, quote.ID, "33333333-3333-4333-8333-333333333333", input)
	if !errors.Is(err, checkoutpro.ErrHostNotConnected) || repo.op != nil || repo.claim || gateway.calls != 0 {
		t.Fatalf("stale tokenized card was not stopped before payment creation: err=%v op=%+v claimed=%t gateway_calls=%d", err, repo.op, repo.claim, gateway.calls)
	}
}

type idempotentRepoStub struct {
	quote        checkoutpro.Quote
	op           *checkoutpro.Operation
	key          string
	fingerprint  []byte
	claimed      bool
	claimErr     error
	applyErr     error
	appliedCount int
	lastVerified checkoutpro.VerifiedPayment
}

func (r *idempotentRepoStub) ByQuote(_ context.Context, _, _ string) (checkoutpro.Quote, *checkoutpro.Operation, error) {
	if r.op == nil {
		return r.quote, nil, nil
	}
	copyOp := *r.op
	return r.quote, &copyOp, nil
}

func (r *idempotentRepoStub) BeginPayment(_ context.Context, _, _, key, id string, fingerprint []byte, _ time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	if r.op != nil {
		if key == r.key {
			if !bytes.Equal(r.fingerprint, fingerprint) {
				return r.quote, checkoutpro.Operation{}, false, checkoutpro.ErrIdempotencyConflict
			}
			return r.quote, *r.op, false, nil
		}
		return r.quote, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	r.key = key
	r.fingerprint = append([]byte(nil), fingerprint...)
	r.op = &checkoutpro.Operation{
		ID: id, QuoteID: r.quote.ID, Status: "pending", CheckoutMethod: method,
		ExpiresAt: r.quote.ExpiresAt, TestMode: true, Amounts: checkoutpro.Amounts{BaseCLP: r.quote.SubtotalCLP},
	}
	return r.quote, *r.op, true, nil
}

func (r *idempotentRepoStub) ClaimPayment(_ context.Context, _ string, _ time.Time) (bool, error) {
	if r.claimErr != nil {
		return false, r.claimErr
	}
	if r.claimed {
		return false, nil
	}
	r.claimed = true
	return true, nil
}

func (r *idempotentRepoStub) ApplyVerifiedPayment(_ context.Context, v checkoutpro.VerifiedPayment) (bool, error) {
	r.appliedCount++
	r.lastVerified = v
	if r.applyErr != nil {
		return false, r.applyErr
	}
	if r.op != nil {
		r.op.Status = v.Status
		r.op.ProviderPaymentID = v.ProviderPaymentID
	}
	return false, nil
}

type stubGateway struct {
	calls   int
	request PaymentRequest
	payment Payment
	err     error
}

func (g *stubGateway) Create(_ context.Context, _ []byte, _ string, request PaymentRequest) (Payment, error) {
	g.calls++
	g.request = request
	if g.err != nil {
		return Payment{}, g.err
	}
	return g.payment, nil
}

func testServiceFixture(now time.Time, repo Repository, gateway PaymentGateway) *Service {
	opID := "44444444-4444-4444-8444-444444444444"
	svc, err := New(repo, testOAuthToken{}, gateway, func() time.Time { return now }, func() (string, error) { return opID, nil }, nil)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestPayUnclaimedOperationEnforcesIdempotencyAndRetriesClaim(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	quoteID := "11111111-1111-4111-8111-111111111111"
	renterID := "22222222-2222-4222-8222-222222222222"
	key := "33333333-3333-4333-8333-333333333333"
	otherKey := "55555555-5555-4555-8555-555555555555"
	opID := "44444444-4444-4444-8444-444444444444"
	fee := int64(428)

	repo := &idempotentRepoStub{
		quote: checkoutpro.Quote{
			ID: quoteID, RenterID: renterID, Currency: "CLP", SubtotalCLP: 12000, ExpiresAt: now.Add(time.Minute),
		},
		claimErr: errors.New("transient db error"),
	}
	gateway := &stubGateway{
		payment: Payment{
			Verified: checkoutpro.VerifiedPayment{
				OperationID: opID, ProviderPaymentID: "987654321", SellerID: "3727396802",
				ExternalReference: opID, Status: "approved", TransactionAmountCLP: 12000, Currency: "CLP",
				PaymentType: "credit_card", LiveMode: false, PaymentUpdatedAt: now,
			},
			StatusDetail:      "accredited",
			RequestID:         "req-ok-1",
			ApplicationFeeCLP: &fee,
			LiveModePresent:   true,
		},
	}
	service := testServiceFixture(now, repo, gateway)
	input := Input{Token: "fake-card-token-1234", PaymentMethodID: "master", Payer: Payer{Email: "buyer@example.test"}}

	if _, err := service.Pay(context.Background(), renterID, quoteID, key, input); !errors.Is(err, checkoutpro.ErrProvider) {
		t.Fatalf("expected ErrProvider when claim fails, got %v", err)
	}
	if gateway.calls != 0 || repo.op == nil {
		t.Fatalf("expected pending unclaimed op with 0 gateway calls, got calls=%d op=%+v", gateway.calls, repo.op)
	}

	// Mismatched fingerprint with same key must not be bypassed by existing != nil.
	changedInput := input
	changedInput.Token = "different-card-token-1234"
	if _, err := service.Pay(context.Background(), renterID, quoteID, key, changedInput); !errors.Is(err, checkoutpro.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict on changed input, got %v", err)
	}
	if _, err := service.Pay(context.Background(), renterID, quoteID, otherKey, input); !errors.Is(err, checkoutpro.ErrConflict) {
		t.Fatalf("expected ErrConflict on different key, got %v", err)
	}

	// Retrying with the exact same key and input claims and executes the payment.
	repo.claimErr = nil
	res, err := service.Pay(context.Background(), renterID, quoteID, key, input)
	if err != nil {
		t.Fatalf("unexpected retry error: %v", err)
	}
	if res.Status != "approved" || res.PaymentID != "987654321" || res.StatusDetail != "accredited" || res.ProviderRequestID != "req-ok-1" {
		t.Fatalf("unexpected approved result: %+v", res)
	}
	if gateway.calls != 1 || gateway.request.Metadata["operation_id"] != opID || repo.appliedCount != 1 {
		t.Fatalf("unexpected gateway/repo state: calls=%d metadata=%+v applied=%d", gateway.calls, gateway.request.Metadata, repo.appliedCount)
	}
}

func TestPayProviderErrorAndReconciliationStates(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	quoteID := "11111111-1111-4111-8111-111111111111"
	renterID := "22222222-2222-4222-8222-222222222222"
	key := "33333333-3333-4333-8333-333333333333"
	opID := "44444444-4444-4444-8444-444444444444"
	input := Input{Token: "fake-card-token-1234", PaymentMethodID: "master", Payer: Payer{Email: "buyer@example.test"}}

	cases := []struct {
		name       string
		gatewayErr error
		payment    Payment
		applyErr   error
		wantStatus string
		wantCode   string
	}{
		{
			name:       "422 maps to provider_error",
			gatewayErr: &ProviderFailure{Stage: "payment_create", HTTPStatus: http.StatusUnprocessableEntity, Code: "invalid_card_token", RequestID: "req-422"},
			wantStatus: "provider_error",
			wantCode:   "invalid_card_token",
		},
		{
			name:       "429 maps to reconciliation_required",
			gatewayErr: &ProviderFailure{Stage: "payment_create", HTTPStatus: http.StatusTooManyRequests, Code: "unknown", RequestID: "req-429"},
			wantStatus: "reconciliation_required",
			wantCode:   "unknown",
		},
		{
			name: "live_mode true in test mode maps to reconciliation_required",
			payment: Payment{
				Verified: checkoutpro.VerifiedPayment{
					OperationID: opID, ExternalReference: opID, ProviderPaymentID: "123", LiveMode: true,
				},
				LiveModePresent: true,
				RequestID:       "req-live",
			},
			wantStatus: "reconciliation_required",
		},
		{
			name: "apply persistence error maps to reconciliation_required",
			payment: Payment{
				Verified: checkoutpro.VerifiedPayment{
					OperationID: opID, ExternalReference: opID, ProviderPaymentID: "123", LiveMode: false,
				},
				LiveModePresent: true,
				RequestID:       "req-db",
			},
			applyErr:   errors.New("db unavailable"),
			wantStatus: "reconciliation_required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &idempotentRepoStub{
				quote: checkoutpro.Quote{
					ID: quoteID, RenterID: renterID, Currency: "CLP", SubtotalCLP: 12000, ExpiresAt: now.Add(time.Minute),
				},
				applyErr: tc.applyErr,
			}
			gateway := &stubGateway{payment: tc.payment, err: tc.gatewayErr}
			service := testServiceFixture(now, repo, gateway)
			res, err := service.Pay(context.Background(), renterID, quoteID, key, input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Status != tc.wantStatus || (tc.wantCode != "" && res.ProviderCode != tc.wantCode) {
				t.Fatalf("got status=%q code=%q, want status=%q code=%q (%+v)", res.Status, res.ProviderCode, tc.wantStatus, tc.wantCode, res)
			}
		})
	}
}
