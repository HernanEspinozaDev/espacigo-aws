package checkoutpro

import (
	"context"
	"testing"
	"time"
)

func TestCommissionUsesPesosAndRoundsHalfUp(t *testing.T) {
	for _, tc := range []struct{ base, commission, vat, gross int64 }{
		{100000, 3000, 570, 3570}, {1, 0, 0, 0}, {17, 1, 0, 1}, {50, 2, 0, 2}, {1000000, 30000, 5700, 35700},
	} {
		got, err := CalculateAmounts(tc.base)
		if err != nil {
			t.Fatalf("CalculateAmounts(%d): %v", tc.base, err)
		}
		if got.CommissionCLP != tc.commission || got.VATCLP != tc.vat || got.MarketplaceFeeCLP != tc.gross {
			t.Errorf("base=%d got=%+v", tc.base, got)
		}
	}
}

func TestInitPointRequiresApprovedMercadoPagoHTTPSHost(t *testing.T) {
	for _, tc := range []struct {
		url        string
		test, want bool
	}{
		{"https://www.mercadopago.cl/checkout/v1/redirect?pref_id=x", true, true},
		{"https://www.mercadopago.cl/checkout/v1/redirect", false, true},
		{"http://www.mercadopago.cl/checkout", true, false},
		{"https://evil.example/checkout", true, false},
		{"https://www.mercadopago.cl.evil.example/checkout", true, false},
		{"https://www.mercadopago.cl:8443/checkout", true, false},
	} {
		if got := validInitPoint(tc.url, tc.test); got != tc.want {
			t.Errorf("validInitPoint(%q,%v)=%v want %v", tc.url, tc.test, got, tc.want)
		}
	}
}

type memoryRepository struct {
	quote       Quote
	op          Operation
	fingerprint []byte
	key         string
	claimed     bool
}

func (r *memoryRepository) Begin(_ context.Context, _, _, key, id string, f []byte, testMode bool, now time.Time) (Quote, Operation, bool, error) {
	if r.op.ID != "" {
		if key != r.key {
			return Quote{}, Operation{}, false, ErrConflict
		}
		return r.quote, r.op, false, nil
	}
	r.key = key
	r.fingerprint = f
	r.op = Operation{ID: id, QuoteID: r.quote.ID, Status: "pending", ExpiresAt: r.quote.ExpiresAt, UpdatedAt: now, TestMode: testMode}
	return r.quote, r.op, true, nil
}
func (r *memoryRepository) ClaimPreference(context.Context, string, time.Time, time.Duration) (bool, error) {
	if r.claimed {
		return false, nil
	}
	r.claimed = true
	return true, nil
}
func (r *memoryRepository) SavePreference(_ context.Context, id, pref, url string, now time.Time) error {
	r.op.PreferenceID = pref
	r.op.InitPoint = url
	r.op.UpdatedAt = now
	return nil
}
func (r *memoryRepository) ByQuote(context.Context, string, string) (Quote, *Operation, error) {
	op := r.op
	return r.quote, &op, nil
}

type memoryTokens struct{}

func (memoryTokens) AccessToken(context.Context, string) ([]byte, error) { return []byte("token"), nil }

type memoryGateway struct {
	calls             int
	findCalls         int
	createErr         error
	foundPreference   *Preference
	preferenceRequest PreferenceRequest
}

func (g *memoryGateway) PaymentTypes(context.Context, []byte) ([]string, error) {
	return []string{"credit_card", "debit_card", "ticket", "account_money"}, nil
}

func (g *memoryGateway) Create(_ context.Context, _ []byte, p PreferenceRequest) (Preference, error) {
	g.calls++
	g.preferenceRequest = p
	if g.createErr != nil {
		return Preference{}, g.createErr
	}
	return Preference{ID: "pref-1", InitPoint: "https://www.mercadopago.cl/checkout/pref-1"}, nil
}
func (g *memoryGateway) FindByExternalReference(context.Context, []byte, string) (Preference, error) {
	g.findCalls++
	if g.foundPreference != nil {
		return *g.foundPreference, nil
	}
	return Preference{}, ErrNotFound
}

func TestStartPersistsAndReusesQuotePreferenceWithoutTrustingClientAmounts(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 100000, Units: 1, SubtotalCLP: 100000, StartAt: now.Add(24 * time.Hour), EndAt: now.Add(25 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{}
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID || gateway.calls != 1 {
		t.Fatalf("not reused: first=%+v second=%+v provider calls=%d", first, second, gateway.calls)
	}
	if gateway.preferenceRequest.MarketplaceFee != 3570 || gateway.preferenceRequest.Items[0].UnitPrice != 100000 || gateway.preferenceRequest.ExternalReference != first.OperationID {
		t.Fatalf("provider payload not derived from quote: %+v", gateway.preferenceRequest)
	}
	if len(gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes) != 3 {
		t.Fatalf("payment types were not excluded except account_money: %+v", gateway.preferenceRequest.PaymentMethods)
	}
	if first.InitPoint != "https://www.mercadopago.cl/checkout/pref-1" || first.Status != "pending" {
		t.Fatalf("response=%+v", first)
	}
}

func TestStartRecoversProviderTimeoutByExternalReference(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 100000, Units: 1, SubtotalCLP: 100000, StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{createErr: ErrProvider, foundPreference: &Preference{ID: "pref-recovered", InitPoint: "https://www.mercadopago.cl/checkout/recovered"}}
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if result.InitPoint != "https://www.mercadopago.cl/checkout/recovered" || gateway.calls != 1 || gateway.findCalls != 1 {
		t.Fatalf("recovery=%+v create=%d find=%d", result, gateway.calls, gateway.findCalls)
	}
}
