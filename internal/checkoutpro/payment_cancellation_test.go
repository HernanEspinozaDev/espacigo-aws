package checkoutpro

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPGatewayCancelPendingPayment(t *testing.T) {
	const key = "0e4561a7-e89b-42d3-a456-426614174000"

	g := &HTTPGateway{
		BaseURL: "https://api.test",
		Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPut || r.URL.Path != "/v1/payments/190475305601" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			if r.Header.Get("Authorization") != "Bearer seller-oauth-secret" {
				t.Fatalf("unexpected Authorization header")
			}
			if r.Header.Get("X-Idempotency-Key") != key {
				t.Fatalf("unexpected X-Idempotency-Key: %q", r.Header.Get("X-Idempotency-Key"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"status":"cancelled"}` {
				t.Fatalf("unexpected body: %s", body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":190475305601,"status":"cancelled"}`)),
			}, nil
		})},
	}

	if err := g.CancelPendingPayment(context.Background(), []byte("seller-oauth-secret"), "190475305601", key); err != nil {
		t.Fatalf("CancelPendingPayment failed: %v", err)
	}
}

func TestHTTPGatewayCancelPendingPaymentValidatesInputsAndRedactsErrors(t *testing.T) {
	const validKey = "0e4561a7-e89b-42d3-a456-426614174000"
	g := &HTTPGateway{
		BaseURL: "https://api.test",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad_request","message":"secret-token-leak"}`)),
			}, nil
		})},
	}

	for _, tc := range []struct {
		name  string
		ctx   context.Context
		token []byte
		id    string
		key   string
	}{
		{name: "nil context", ctx: nil, token: []byte("t"), id: "123", key: validKey},
		{name: "empty token", ctx: context.Background(), token: nil, id: "123", key: validKey},
		{name: "bad payment id", ctx: context.Background(), token: []byte("t"), id: "abc", key: validKey},
		{name: "bad key", ctx: context.Background(), token: []byte("t"), id: "123", key: "not-a-uuid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := g.CancelPendingPayment(tc.ctx, tc.token, tc.id, tc.key); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}

	err := g.CancelPendingPayment(context.Background(), []byte("seller-secret"), "12345", validKey)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Stage != "payment_cancel" || providerErr.HTTPStatus != http.StatusBadRequest || providerErr.ProviderCode != "bad_request" {
		t.Fatalf("unexpected provider error: %T %+v", err, err)
	}
	if strings.Contains(err.Error(), "secret-token-leak") || strings.Contains(err.Error(), "seller-secret") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

func TestHTTPGatewaySearchPaymentsByExternalReference(t *testing.T) {
	const opID = "op-123/abc+xyz"
	g := &HTTPGateway{
		BaseURL: "https://api.test",
		Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/payments/search" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			if r.Header.Get("Authorization") != "Bearer seller-oauth-token" {
				t.Fatalf("missing seller Authorization header")
			}
			if got := r.URL.Query().Get("external_reference"); got != opID {
				t.Fatalf("external_reference=%q, want %q (raw=%s)", got, opID, r.URL.RawQuery)
			}
			if r.URL.Query().Get("sort") != "date_created" || r.URL.Query().Get("criteria") != "desc" {
				t.Fatalf("unexpected query params: %s", r.URL.RawQuery)
			}
			payload := `{"results":[
				{"id":1001,"collector_id":91,"external_reference":"other-op","status":"approved","transaction_amount":5000,"currency_id":"CLP","live_mode":false,"payment_type_id":"account_money","metadata":{"operation_id":"other-op"},"date_last_updated":"2030-01-01T12:00:00Z"},
				{"id":1002,"collector_id":91,"external_reference":"op-123/abc+xyz","status":"pending","transaction_amount":12000,"currency_id":"CLP","live_mode":false,"payment_type_id":"account_money","metadata":{"operation_id":"op-123/abc+xyz"},"date_last_updated":"2030-01-01T12:05:00Z"},
				{"id":1003,"collector_id":91,"external_reference":"op-123/abc+xyz","status":"rejected","transaction_amount":12000,"currency_id":"CLP","live_mode":false,"payment_type_id":"credit_card","metadata":{"operation_id":"op-123/abc+xyz"},"date_last_updated":"2030-01-01T12:01:00Z"}
			]}`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}, nil
		})},
	}

	got, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("seller-oauth-token"), opID)
	if err != nil {
		t.Fatalf("SearchPaymentsByExternalReference failed: %v", err)
	}
	if len(got) != 2 || got[0].ProviderPaymentID != "1002" || got[0].Status != "pending" || got[1].ProviderPaymentID != "1003" || got[1].Status != "rejected" {
		t.Fatalf("unexpected search results: %+v", got)
	}
}

func TestHTTPGatewaySearchPaymentsByExternalReferenceEmptyAndFailures(t *testing.T) {
	t.Run("empty results", func(t *testing.T) {
		g := &HTTPGateway{
			BaseURL: "https://api.test",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[]}`))}, nil
			})},
		}
		got, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("seller"), "op-1")
		if err != nil || len(got) != 0 {
			t.Fatalf("got=%+v err=%v, want empty nil", got, err)
		}
	})

	t.Run("missing results array fails as ErrProvider", func(t *testing.T) {
		g := &HTTPGateway{
			BaseURL: "https://api.test",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})},
		}
		if _, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("seller"), "op-1"); !errors.Is(err, ErrProvider) {
			t.Fatalf("expected ErrProvider, got %v", err)
		}
	})

	t.Run("malformed matching payment fails instead of returning empty", func(t *testing.T) {
		g := &HTTPGateway{
			BaseURL: "https://api.test",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1002,"external_reference":"op-1","status":"pending"}]}`))}, nil
			})},
		}
		if _, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("seller"), "op-1"); !errors.Is(err, ErrProvider) {
			t.Fatalf("expected ErrProvider, got %v", err)
		}
	})

	t.Run("http 500 returns redacted ProviderError", func(t *testing.T) {
		g := &HTTPGateway{
			BaseURL: "https://api.test",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"internal_error","message":"secret"}`))}, nil
			})},
		}
		_, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("seller-secret"), "op-1")
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Stage != "payment_search" || pe.HTTPStatus != http.StatusInternalServerError {
			t.Fatalf("unexpected error: %T %v", err, err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaked payload: %v", err)
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		g := &HTTPGateway{BaseURL: "https://api.test"}
		if _, err := g.SearchPaymentsByExternalReference(nil, []byte("s"), "op-1"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid, got %v", err)
		}
		if _, err := g.SearchPaymentsByExternalReference(context.Background(), nil, "op-1"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid, got %v", err)
		}
		if _, err := g.SearchPaymentsByExternalReference(context.Background(), []byte("s"), "   "); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid, got %v", err)
		}
	})
}
