package checkoutpro

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
		body, _ := io.ReadAll(r.Body)
		raw := string(body)
		for _, fragment := range []string{`"external_reference":"op-123"`, `"marketplace_fee":3570`, `"currency_id":"CLP"`, `"operation_id":"op-123"`, `"installments":1`, `"id":"credit_card"`} {
			if !strings.Contains(raw, fragment) {
				t.Errorf("request body missing %s: %s", fragment, raw)
			}
		}
		if strings.Contains(raw, `"id":"account_money"`) {
			t.Errorf("account money was excluded: %s", raw)
		}
		return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"pref-1","init_point":"https://www.mercadopago.cl/checkout/v1/redirect/pref-1"}`))}, nil
	})}}
	p, err := g.Create(context.Background(), []byte("seller-test-token"), PreferenceRequest{ExternalReference: "op-123", MarketplaceFee: 3570, Items: []PreferenceItem{{Title: "Arriendo", Quantity: 1, UnitPrice: 100000, CurrencyID: "CLP"}}, Metadata: map[string]any{"operation_id": "op-123"}, PaymentMethods: PaymentMethods{ExcludedPaymentTypes: []ExcludedPaymentType{{ID: "credit_card"}}, Installments: 1}})
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
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"payment_type_id":"credit_card"},{"payment_type_id":"account_money"},{"payment_type_id":"ticket"},{"payment_type_id":"credit_card"}]`))}, nil
	})}}
	types, err := g.PaymentTypes(context.Background(), []byte("seller"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(types) != "[credit_card account_money ticket]" {
		t.Fatalf("payment types=%v", types)
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
