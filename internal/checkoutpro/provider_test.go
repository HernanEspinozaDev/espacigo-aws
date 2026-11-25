package checkoutpro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPGatewayProviderFailureExposesOnlyAllowlistedDiagnostics(t *testing.T) {
	secretText := `{"error":"invalid_marketplace_fee","message":"do not retain seller@example.invalid access_token=private"}`
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(secretText))}, nil
	})}}
	_, err := g.Create(context.Background(), []byte("private"), PreferenceRequest{})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || !errors.Is(err, ErrProvider) {
		t.Fatalf("expected typed provider error, got %T %v", err, err)
	}
	if providerErr.Stage != "preference_create" || providerErr.HTTPStatus != http.StatusBadRequest || providerErr.ProviderCode != "invalid_marketplace_fee" {
		t.Fatalf("diagnostic=%+v", providerErr)
	}
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "seller@example") || strings.Contains(err.Error(), "marketplace_fee") {
		t.Fatalf("provider error leaked payload detail: %v", err)
	}
}

func TestHTTPGatewayUnknownProviderCodeIsRedacted(t *testing.T) {
	got := readProviderErrorCode(strings.NewReader(`{"error":"credential_not_authorized","message":"secret detail"}`))
	if got != "other" {
		t.Fatalf("unknown provider code was not redacted: %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPGatewayCreatesPreferenceWithMarketplaceSplitAndSellerToken(t *testing.T) {
	called := false
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/checkout/preferences" {
			t.Fatalf("request=%s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer seller-test-token" {
			t.Fatalf("authorization missing")
		}
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		raw := string(body)
		for _, fragment := range []string{`"purpose":"wallet_purchase"`, `"external_reference":"op-123"`, `"marketplace_fee":3570`, `"currency_id":"CLP"`, `"operation_id":"op-123"`, `"installments":1`, `"id":"credit_card"`, `"expires":true`, `"expiration_date_from":"2030-01-01T10:00:00-03:00"`, `"expiration_date_to":"2030-01-01T12:00:00-03:00"`} {
			if !strings.Contains(raw, fragment) {
				t.Errorf("request body missing %s: %s", fragment, raw)
			}
		}
		for _, obsolete := range []string{`"date_of_expiration_from"`, `"date_of_expiration_to"`} {
			if strings.Contains(raw, obsolete) {
				t.Errorf("request body contains obsolete field %s: %s", obsolete, raw)
			}
		}
		if strings.Contains(raw, `"id":"account_money"`) {
			t.Errorf("account money was excluded: %s", raw)
		}
		return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"pref-1","init_point":"https://www.mercadopago.cl/checkout/v1/redirect/pref-1"}`))}, nil
	})}}
	p, err := g.Create(context.Background(), []byte("seller-test-token"), PreferenceRequest{Purpose: "wallet_purchase", ExternalReference: "op-123", MarketplaceFee: 3570, Items: []PreferenceItem{{Title: "Arriendo", Quantity: 1, UnitPrice: 100000, CurrencyID: "CLP"}}, Metadata: map[string]any{"operation_id": "op-123"}, Expires: true, ExpirationDateFrom: "2030-01-01T10:00:00-03:00", ExpirationDateTo: "2030-01-01T12:00:00-03:00", PaymentMethods: PaymentMethods{ExcludedPaymentTypes: []ExcludedPaymentType{{ID: "credit_card"}}, Installments: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !called || p.ID != "pref-1" || !strings.Contains(p.InitPoint, "mercadopago.cl") {
		t.Fatalf("called=%v preference=%+v", called, p)
	}
}

func TestHTTPGatewayListsSellerPaymentTypes(t *testing.T) {
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/payment_methods" {
			t.Fatalf("request=%s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer seller" {
			t.Fatal("seller authorization missing")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"payment_type_id":"credit_card","status":"active"},{"payment_type_id":"account_money","status":"active"},{"payment_type_id":"ticket","status":"active"},{"payment_type_id":"credit_card","status":"active"},{"payment_type_id":"account_money","status":"deactive"}]`))}, nil
	})}}
	types, err := g.PaymentTypes(context.Background(), []byte("seller"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(types) != "[credit_card account_money ticket]" {
		t.Fatalf("payment types=%v", types)
	}
}

func TestHTTPGatewayDoesNotAdvertiseInactivePaymentTypes(t *testing.T) {
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"payment_type_id":"account_money","status":"deactive"},{"payment_type_id":"credit_card","status":"temporally_deactive"}]`))}, nil
	})}}
	types, err := g.PaymentTypes(context.Background(), []byte("seller"))
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 0 {
		t.Fatalf("inactive methods were advertised: %v", types)
	}
}

func TestHTTPGatewayRefundPaymentUsesPersistedIdempotencyKey(t *testing.T) {
	amount := int64(5000)
	key := "0e4561a7-e89b-42d3-a456-426614174000"
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments/123456/refunds" {
			t.Fatalf("request=%s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer seller-token" || r.Header.Get("X-Idempotency-Key") != key {
			t.Fatalf("headers not bound to seller and persisted key")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"amount":5000}` {
			t.Fatalf("body=%s", body)
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":98765,"status":"approved","amount":5000}`))}, nil
	})}}
	got, err := g.RefundPayment(context.Background(), []byte("seller-token"), "123456", &amount, key)
	if err != nil || got.ID != "98765" || got.Status != "approved" || got.Amount != amount {
		t.Fatalf("refund=%+v err=%v", got, err)
	}
}

func TestHTTPGatewayFullRefundOmitsAmount(t *testing.T) {
	key := "0e4561a7-e89b-42d3-a456-426614174000"
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		if len(body) != 0 || r.Header.Get("X-Idempotency-Key") != key {
			t.Fatalf("full refund request body=%s key=%s", body, r.Header.Get("X-Idempotency-Key"))
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"98765","status":"approved","amount":10000}`))}, nil
	})}}
	got, err := g.RefundPayment(context.Background(), []byte("seller-token"), "123456", nil, key)
	if err != nil || got.Amount != 10000 {
		t.Fatalf("refund=%+v err=%v", got, err)
	}
}

func TestHTTPGatewayListsRefundsForReconciliation(t *testing.T) {
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/payments/123456/refunds" || r.Header.Get("Authorization") != "Bearer seller-token" {
			t.Fatalf("request=%s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"id":98765,"status":"in_process","amount":5000}]`))}, nil
	})}}
	refunds, err := g.ListRefunds(context.Background(), []byte("seller-token"), "123456")
	if err != nil || len(refunds) != 1 || refunds[0].ID != "98765" || refunds[0].Status != "in_process" || refunds[0].Amount != 5000 {
		t.Fatalf("refunds=%+v err=%v", refunds, err)
	}
}

func TestHTTPGatewayLookupUsesVerifiedCollectorPaymentTypeAndUpdateTime(t *testing.T) {
	g := &HTTPGateway{BaseURL: "https://api.test", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/payments/12345" {
			t.Fatalf("request=%s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":12345,"collector_id":67890,"external_reference":"op-1","status":"approved","transaction_amount":100000,"currency_id":"CLP","live_mode":false,"payment_type_id":"account_money","metadata":{"operation_id":"op-1"},"date_last_updated":"2030-01-01T12:34:00Z"}`))}, nil
	})}}
	p, err := g.LookupPayment(context.Background(), []byte("seller"), "12345")
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderPaymentID != "12345" || p.SellerID != "67890" || p.TransactionAmountCLP != 100000 || p.PaymentType != "account_money" || p.PaymentUpdatedAt.IsZero() || p.OperationID != "op-1" {
		t.Fatalf("verified payment=%+v", p)
	}
}

func TestParseCLPAmountIsExact(t *testing.T) {
	for _, tc := range []struct {
		value     string
		allowZero bool
		want      int64
		ok        bool
	}{
		{value: "12.0", want: 12, ok: true},
		{value: "12.5", want: 0, ok: false},
		{value: "0", allowZero: true, want: 0, ok: true},
		{value: "9223372036854775808", want: 0, ok: false},
	} {
		got, ok := parseCLPAmount(json.Number(tc.value), tc.allowZero)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseCLPAmount(%q, %v)=(%d,%v), want (%d,%v)", tc.value, tc.allowZero, got, ok, tc.want, tc.ok)
		}
	}
}
