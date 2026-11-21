package checkoutpro

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestCommissionUsesPesosAndRoundsHalfUp(t *testing.T) {
	for _, tc := range []struct{ base, commission, vat, gross int64 }{
		{100000, 3000, 570, 3570}, {12000, 360, 68, 428}, {1, 0, 0, 0}, {17, 1, 0, 1}, {50, 2, 0, 2}, {1000000, 30000, 5700, 35700},
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
		if !bytes.Equal(r.fingerprint, f) {
			return Quote{}, Operation{}, false, ErrIdempotencyConflict
		}
		return r.quote, r.op, false, nil
	}
	r.key = key
	r.fingerprint = f
	r.op = Operation{ID: id, QuoteID: r.quote.ID, Status: "pending", ExpiresAt: r.quote.ExpiresAt, UpdatedAt: now, TestMode: testMode}
	return r.quote, r.op, true, nil
}

func TestWalletPurchaseRequiresTestGateAndCreatesRegisteredBuyerPreference(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	quote := Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 12000, Units: 2, SubtotalCLP: 24000, StartAt: now.Add(time.Hour), EndAt: now.Add(3 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}
	repo := &memoryRepository{quote: quote}
	gateway := &memoryGateway{}
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true, WalletPurchaseEnabled: true}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.StartWalletPurchase(context.Background(), quote.RenterID, quote.ID, "wallet-key")
	if err != nil {
		t.Fatal(err)
	}
	if result.InitPoint != "https://www.mercadopago.cl/checkout/pref-1" || gateway.calls != 1 {
		t.Fatalf("result=%+v provider_calls=%d", result, gateway.calls)
	}
	if gateway.preferenceRequest.Purpose != "wallet_purchase" || gateway.preferenceRequest.MarketplaceFee != 857 || gateway.preferenceRequest.ExternalReference != result.OperationID {
		t.Fatalf("wallet preference fields=%+v", gateway.preferenceRequest)
	}
	if gateway.preferenceRequest.Items[0].UnitPrice != 24000 || gateway.preferenceRequest.Items[0].CurrencyID != "CLP" {
		t.Fatalf("wallet preference item was not quote-derived: %+v", gateway.preferenceRequest.Items)
	}
	reused, err := svc.StartWalletPurchase(context.Background(), quote.RenterID, quote.ID, "wallet-key")
	if err != nil || reused.OperationID != result.OperationID || gateway.calls != 1 {
		t.Fatalf("wallet retry reused=%+v err=%v provider_calls=%d", reused, err, gateway.calls)
	}
	if _, err := svc.Start(context.Background(), quote.RenterID, quote.ID, "wallet-key"); err != ErrIdempotencyConflict {
		t.Fatalf("same key crossed checkout modes with err=%v", err)
	}
	if _, err := svc.Start(context.Background(), quote.RenterID, quote.ID, "card-key"); err != ErrConflict {
		t.Fatalf("second checkout mode bypassed quote hold with err=%v", err)
	}
}

func TestWalletPurchaseIsDisabledOutsideExplicitTestGate(t *testing.T) {
	for _, config := range []Config{
		{FrontendOrigin: "https://espacigo.test", TestMode: true},
		{FrontendOrigin: "https://espacigo.test", TestMode: false, WalletPurchaseEnabled: true},
	} {
		svc, err := New(&memoryRepository{}, memoryTokens{}, &memoryGateway{}, config, time.Now, func() string { return "55555555-5555-4555-8555-555555555555" })
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.StartWalletPurchase(context.Background(), "44444444-4444-4444-8444-444444444444", "11111111-1111-4111-8111-111111111111", "wallet-key")
		if err != ErrWalletPurchaseDisabled {
			t.Fatalf("wallet gate config=%+v error=%v", config, err)
		}
	}
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
	paymentTypes      []string
	paymentTypesErr   error
}

func (g *memoryGateway) PaymentTypes(context.Context, []byte) ([]string, error) {
	if g.paymentTypesErr != nil {
		return nil, g.paymentTypesErr
	}
	if g.paymentTypes != nil {
		return g.paymentTypes, nil
	}
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

func TestStartReportsPaymentMethodsPreflightWithoutClaimingPreference(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 12000, Units: 1, SubtotalCLP: 12000, StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{paymentTypes: []string{"credit_card", "debit_card"}}
	var diagnostics []Diagnostic
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: false, Diagnostics: func(_ context.Context, d Diagnostic) { diagnostics = append(diagnostics, d) }}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != ErrPaymentMethodUnavailable {
		t.Fatalf("Start error=%v, want ErrPaymentMethodUnavailable", err)
	}
	if repo.claimed || gateway.calls != 0 || len(diagnostics) != 1 {
		t.Fatalf("claimed=%v create_calls=%d diagnostics=%+v", repo.claimed, gateway.calls, diagnostics)
	}
	if diagnostics[0].Stage != "payment_methods" || diagnostics[0].Reason != "account_money_unavailable" || diagnostics[0].HTTPStatus != 0 || diagnostics[0].ProviderCode != "" {
		t.Fatalf("unexpected diagnostic: %+v", diagnostics[0])
	}
}

func TestStartReportsAllowlistedPaymentMethodHTTPFailure(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 24000, Units: 1, SubtotalCLP: 24000, StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{paymentTypesErr: &ProviderError{Stage: "payment_methods", HTTPStatus: 401, ProviderCode: "invalid_access_token"}}
	var diagnostics []Diagnostic
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true, Diagnostics: func(_ context.Context, d Diagnostic) { diagnostics = append(diagnostics, d) }}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != ErrProvider || repo.claimed || gateway.calls != 0 || len(diagnostics) != 1 {
		t.Fatalf("err=%v claimed=%v create_calls=%d diagnostics=%+v", err, repo.claimed, gateway.calls, diagnostics)
	}
	if diagnostics[0].Stage != "payment_methods" || diagnostics[0].Reason != "provider_unavailable" || diagnostics[0].HTTPStatus != 401 || diagnostics[0].ProviderCode != "invalid_access_token" {
		t.Fatalf("unexpected diagnostic: %+v", diagnostics[0])
	}
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
	if gateway.preferenceRequest.Purpose != "" {
		t.Fatalf("existing Checkout Pro flow gained wallet_purchase purpose: %+v", gateway.preferenceRequest)
	}
	if len(gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes) != 1 {
		t.Fatalf("payment types were not excluded except account_money: %+v", gateway.preferenceRequest.PaymentMethods)
	}
	if first.InitPoint != "https://www.mercadopago.cl/checkout/pref-1" || first.Status != "pending" {
		t.Fatalf("response=%+v", first)
	}
}

func TestTestModeAllowsOnlyKnownCardsAndAccountMoney(t *testing.T) {
	for _, tc := range []struct {
		paymentType string
		want        bool
	}{
		{"account_money", true}, {"credit_card", true}, {"debit_card", true}, {"prepaid_card", true},
		{"ticket", false}, {"bank_transfer", false}, {"digital_currency", false}, {"unknown", false},
	} {
		if got := AllowedPaymentType(true, tc.paymentType); got != tc.want {
			t.Errorf("AllowedPaymentType(test,%q)=%v want %v", tc.paymentType, got, tc.want)
		}
		if tc.paymentType != "account_money" && AllowedPaymentType(false, tc.paymentType) {
			t.Errorf("production accepted %q", tc.paymentType)
		}
	}
	for _, tc := range []struct {
		testMode, liveMode, want bool
	}{
		{true, false, true}, {true, true, false}, {false, true, true}, {false, false, false},
	} {
		if got := PaymentModeMatches(tc.testMode, tc.liveMode); got != tc.want {
			t.Errorf("PaymentModeMatches(test=%v, live=%v)=%v want %v", tc.testMode, tc.liveMode, got, tc.want)
		}
	}
}

func TestStartTestModeCreatesPreferenceWithCardTypesAndNoOfflineTypes(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 12000, Units: 1, SubtotalCLP: 12000, StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{paymentTypes: []string{"credit_card", "debit_card", "ticket", "bank_transfer"}}
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.calls != 1 || !repo.claimed {
		t.Fatalf("test checkout should create preference after preflight: calls=%d claimed=%v", gateway.calls, repo.claimed)
	}
	if len(gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes) != 2 || gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes[0].ID != "ticket" || gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes[1].ID != "bank_transfer" {
		t.Fatalf("test preference did not exclude only offline/non-card types: %+v", gateway.preferenceRequest.PaymentMethods)
	}
	if gateway.preferenceRequest.MarketplaceFee != 428 {
		t.Fatalf("test card preference has wrong marketplace fee: %v", gateway.preferenceRequest.MarketplaceFee)
	}
	if result.InitPoint != "https://www.mercadopago.cl/checkout/pref-1" {
		t.Fatalf("test card checkout did not return init_point: %q", result.InitPoint)
	}
}

func TestStartTestModeAllowsCardOnlyCatalogWithoutExclusions(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{quote: Quote{ID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", RenterID: "44444444-4444-4444-8444-444444444444", Title: "Sala", Currency: "CLP", UnitPriceCLP: 12000, Units: 1, SubtotalCLP: 12000, StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), ExpiresAt: now.Add(15 * time.Minute)}}
	gateway := &memoryGateway{paymentTypes: []string{"credit_card", "debit_card"}}
	svc, err := New(repo, memoryTokens{}, gateway, Config{FrontendOrigin: "https://espacigo.test", TestMode: true}, func() time.Time { return now }, func() string { return "55555555-5555-4555-8555-555555555555" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Start(context.Background(), repo.quote.RenterID, repo.quote.ID, "stable-key")
	if err != nil || !repo.claimed || gateway.calls != 1 || len(gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes) != 0 {
		t.Fatalf("result=%+v err=%v claimed=%v create_calls=%d exclusions=%+v", result, err, repo.claimed, gateway.calls, gateway.preferenceRequest.PaymentMethods.ExcludedPaymentTypes)
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
