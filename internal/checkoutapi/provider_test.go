package checkoutapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testOperationID = "8f25bbf2-a142-4b0a-86fd-1c93ab11742c"

func TestCreatePaymentSendsOAuthSellerTokenAndTrustedCLPAmounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments" {
			t.Errorf("unexpected provider route: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer seller-oauth-test" {
			t.Errorf("unexpected authorization header")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("unexpected content type: %q", got)
		}
		if got := r.Header.Get("X-Idempotency-Key"); got != testOperationID {
			t.Errorf("unexpected idempotency key: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		for field, want := range map[string]any{
			"transaction_amount": float64(12000), "application_fee": float64(428),
			"external_reference": testOperationID, "payment_method_id": "visa",
			"issuer_id": "123", "installments": float64(1), "binary_mode": false, "token": "single-use-token",
		} {
			if payload[field] != want {
				t.Errorf("payload[%q]=%v, want %v", field, payload[field], want)
			}
		}
		payer, ok := payload["payer"].(map[string]any)
		if !ok || payer["email"] != "buyer@example.test" {
			t.Errorf("payer email was not sent")
		}
		if _, exists := payload["currency_id"]; exists {
			t.Error("Payments API request should not send currency_id; CLP is seller/site configured")
		}
		w.Header().Set("x-request-id", "req-1234-abcd")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":987654321,"collector_id":3727396802,"external_reference":"`+testOperationID+`","status":"approved","status_detail":"accredited","transaction_amount":12000,"currency_id":"CLP","live_mode":false,"payment_type_id":"credit_card","application_fee":428,"metadata":{"operation_id":"`+testOperationID+`"},"date_last_updated":"2026-11-20T12:34:56.000-03:00"}`)
	}))
	defer server.Close()

	gateway := newGateway(server.URL, server.Client())
	fee := int64(428)
	result, err := gateway.Create(context.Background(), []byte("seller-oauth-test"), testOperationID, PaymentRequest{
		Token: "single-use-token", PaymentMethodID: "visa", IssuerID: "123",
		Payer: Payer{Email: "buyer@example.test"}, TransactionAmountCLP: 12000,
		ApplicationFeeCLP: fee, ExternalReference: testOperationID,
		Metadata: map[string]string{"operation_id": testOperationID},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.Verified.ProviderPaymentID != "987654321" || result.Verified.SellerID != "3727396802" || result.Verified.Status != "approved" ||
		result.Verified.Currency != "CLP" || result.Verified.TransactionAmountCLP != 12000 || result.Verified.ExternalReference != testOperationID ||
		result.Verified.OperationID != testOperationID || result.Verified.PaymentType != "credit_card" {
		t.Fatalf("unexpected verified payment projection: %+v", result.Verified)
	}
	if !result.LiveModePresent || result.Verified.LiveMode || result.StatusDetail != "accredited" || result.RequestID != "req-1234-abcd" {
		t.Fatalf("unexpected safe provider metadata: %+v", result)
	}
	if result.ApplicationFeeCLP == nil || *result.ApplicationFeeCLP != 428 || result.Verified.ApplicationFeeCLP == nil || *result.Verified.ApplicationFeeCLP != 428 {
		t.Fatalf("application fee was not parsed/preserved: %+v", result)
	}
}

func TestCreatePaymentOmitsApplicationFeeWhenDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if _, exists := payload["application_fee"]; exists {
			t.Error("zero-fee test payment must omit application_fee entirely")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":987654323,"collector_id":3727396802,"external_reference":"`+testOperationID+`","status":"approved","status_detail":"accredited","transaction_amount":12000,"currency_id":"CLP","live_mode":false,"payment_type_id":"credit_card","metadata":{"operation_id":"`+testOperationID+`"}}`)
	}))
	defer server.Close()

	request := validPaymentRequest()
	request.ApplicationFeeCLP = 0
	result, err := newGateway(server.URL, server.Client()).Create(context.Background(), []byte("seller-oauth-test"), testOperationID, request)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.ApplicationFeePresent || result.ApplicationFeeCLP != nil || result.Verified.ApplicationFeeCLP != nil {
		t.Fatalf("omitted application fee should remain absent in provider projection: %+v", result)
	}
}

func TestCreatePaymentPreservesMercadoPagoOtherIdentificationType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload struct {
			Payer Payer `json:"payer"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if payload.Payer.Identification == nil || payload.Payer.Identification.Type != "Otro" || payload.Payer.Identification.Number != "123456789" {
			t.Errorf("payer identification was not preserved: %+v", payload.Payer.Identification)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":987654322,"collector_id":3727396802,"external_reference":"`+testOperationID+`","status":"approved","status_detail":"accredited","transaction_amount":12000,"currency_id":"CLP","live_mode":false,"payment_type_id":"credit_card","application_fee":428,"metadata":{"operation_id":"`+testOperationID+`"}}`)
	}))
	defer server.Close()

	payment := validPaymentRequest()
	payment.Payer.Identification = &Identification{Type: "Otro", Number: "123456789"}
	if _, err := newGateway(server.URL, server.Client()).Create(context.Background(), []byte("seller-oauth-test"), testOperationID, payment); err != nil {
		t.Fatalf("Create returned error for Mercado Pago test identification type: %v", err)
	}
}

func TestCreatePaymentProviderFailureIsAllowlistedAndDoesNotEchoPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req-error-9")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":"bad_request","message":"buyer@example.test card token private-token","cause":[{"code":2059,"description":"do not log"}]}`)
	}))
	defer server.Close()

	_, err := newGateway(server.URL, server.Client()).Create(context.Background(), []byte("seller-oauth-test"), testOperationID, validPaymentRequest())
	var providerErr *ProviderFailure
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderFailure, got %T", err)
	}
	if providerErr.Stage != "payment_create" || providerErr.HTTPStatus != http.StatusUnprocessableEntity || providerErr.Code != "2059" || providerErr.RequestID != "req-error-9" ||
		providerErr.ProviderError != "bad_request" || providerErr.CauseCount != 1 || strings.Join(providerErr.CauseCodes, ",") != "2059" || providerErr.UnknownFieldCount != 0 || providerErr.BodyTruncated {
		t.Fatalf("unexpected provider diagnostics: %+v", providerErr)
	}
	for _, forbidden := range []string{"buyer@example.test", "private-token", "do not log", "card token"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("provider failure leaked %q", forbidden)
		}
	}
}

func TestProviderCodePrefersAllowlistedCauseOverGenericBadRequest(t *testing.T) {
	got := providerCode([]byte(`{"error":"bad_request","message":"sensitive summary","cause":[{"code":2059,"description":"private details","data":"private context"}]}`))
	if got != "2059" {
		t.Fatalf("providerCode()=%q, want specific allowlisted cause code", got)
	}
	if got := providerCode([]byte(`{"error":"invalid_card_token","cause":[{"code":2059}]}`)); got != "invalid_card_token" {
		t.Fatalf("specific top-level code got %q", got)
	}
	if got := providerCode([]byte(`{"error":"bad_request","cause":[{"code":999999,"description":"private"}]}`)); got != "999999" {
		t.Fatalf("new numeric cause code was not retained safely: %q", got)
	}
}

func TestProviderDiagnosticsKeepOnlyBoundedStructure(t *testing.T) {
		diagnostic := providerDiagnostics([]byte(`{"error":"bad_request","message":"private text\nAuthorization: Bearer signature_secret","unknown":"sensitive","cause":[{"code":12345678,"description":"buyer@example.test card token PAN=fixture-pan CVV=fixture-cvv","data":"private","extra":"sensitive"},{"code":"123456789","description":"not retained"}]}`))
	if diagnostic.ProviderError != "bad_request" || diagnostic.Code != "12345678" || diagnostic.CauseCount != 2 ||
		strings.Join(diagnostic.CauseCodes, ",") != "12345678" || diagnostic.UnknownFieldCount != 2 {
		t.Fatalf("unexpected sanitized diagnostics: %+v", diagnostic)
	}
	encoded := fmt.Sprintf("%+v", diagnostic)
	for _, forbidden := range []string{"private text", "Authorization", "Bearer", "signature_secret", "buyer@example.test", "card token", "4111111111111111", "CVV", "sensitive", "not retained"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("sanitized diagnostic retained %q", forbidden)
		}
	}
}

func TestProviderDiagnosticsClassifyDescriptionsWithoutRetainingText(t *testing.T) {
	diagnostic := providerDiagnostics([]byte(`{"error":"bad_request","cause":[{"code":2198,"description":"Invalid users involved: seller TESTUSER123 and buyer buyer@example.test; token=secret-value PAN=fixture-pan"},{"code":3,"description":"Payment method issuer is not authorized; card token private-value"},{"code":4,"description":"unpublished provider reason private@example.test"}]}`))
	if diagnostic.Code != "2198" || strings.Join(diagnostic.CauseCategories, ",") != "involved_users,payment_method,unclassified" {
		t.Fatalf("unexpected diagnostic labels: %+v", diagnostic)
	}
	encoded := fmt.Sprintf("%+v", diagnostic)
	for _, forbidden := range []string{"TESTUSER123", "buyer@example.test", "secret-value", "4111111111111111", "private-value", "private@example.test", "unpublished provider reason"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("diagnostic leaked provider text %q", forbidden)
		}
	}
}

func TestCreateProviderFailureIncludesOnlyRequestPresenceFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad_request","cause":[{"code":2198,"description":"opaque"}]}`)
	}))
	defer server.Close()

	request := validPaymentRequest()
	request.IssuerID = "9876"
	request.Payer.Identification = &Identification{Type: "RUT", Number: "123456789"}
	request.ApplicationFeeCLP = 720
	_, err := newGateway(server.URL, server.Client()).Create(context.Background(), []byte("oauth-secret"), testOperationID, request)
	var providerErr *ProviderFailure
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderFailure, got %T", err)
	}
	facts := providerErr.RequestFacts
	if !facts.SellerTokenPresent || !facts.CardTokenPresent || !facts.PaymentMethodSet || !facts.IssuerSet || !facts.PayerEmailSet || !facts.IdentificationSet || !facts.ApplicationFeeSet {
		t.Fatalf("missing presence facts: %+v", facts)
	}
	encoded := fmt.Sprintf("%+v", providerErr)
	for _, forbidden := range []string{"oauth-secret", "9876", request.Token, request.Payer.Email, request.Payer.Identification.Number} {
		if forbidden != "" && strings.Contains(encoded, forbidden) {
			t.Fatalf("provider failure leaked request value %q", forbidden)
		}
	}
}

func TestProviderCodeAllowsOfficialChilePaymentsCodes(t *testing.T) {
	for _, code := range []string{"3", "2067", "4033", "4038", "4039", "4050", "4051"} {
		t.Run(code, func(t *testing.T) {
			body := []byte(`{"error":"bad_request","cause":[{"code":` + code + `,"description":"private detail"}]}`)
			if got := providerCode(body); got != code {
				t.Fatalf("providerCode()=%q, want official allowlisted code %s", got, code)
			}
		})
	}
}

func TestCreatePaymentUnknownErrorCodeAndUnsafeRequestIDAreRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "bad request id\r\nprivate")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"unknown_provider_failure","message":"not for logs"}`)
	}))
	defer server.Close()

	_, err := newGateway(server.URL, server.Client()).Create(context.Background(), []byte("seller-oauth-test"), testOperationID, validPaymentRequest())
	var providerErr *ProviderFailure
	if !errors.As(err, &providerErr) || providerErr.Code != "unknown" || providerErr.RequestID != "" {
		t.Fatalf("unsafe error metadata was not redacted: %#v", providerErr)
	}
}

func TestCreatePaymentRejectsInvalidRequestBeforeProviderCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusCreated) }))
	defer server.Close()
	gateway := newGateway(server.URL, server.Client())
	tests := []struct {
		name      string
		access    []byte
		operation string
		mutate    func(*PaymentRequest)
	}{
		{name: "missing seller token", operation: testOperationID},
		{name: "seller token contains header control", access: []byte("seller\r\noauth"), operation: testOperationID},
		{name: "non UUID idempotency key", access: []byte("seller-oauth-test"), operation: "not-a-uuid"},
		{name: "reference mismatch", access: []byte("seller-oauth-test"), operation: testOperationID, mutate: func(p *PaymentRequest) { p.ExternalReference = "other" }},
		{name: "non integer CLP amount", access: []byte("seller-oauth-test"), operation: testOperationID, mutate: func(p *PaymentRequest) { p.TransactionAmountCLP = 0 }},
		{name: "fee is not below amount", access: []byte("seller-oauth-test"), operation: testOperationID, mutate: func(p *PaymentRequest) { p.ApplicationFeeCLP = p.TransactionAmountCLP }},
		{name: "invalid payer email", access: []byte("seller-oauth-test"), operation: testOperationID, mutate: func(p *PaymentRequest) { p.Payer.Email = "buyer@example.test\r\n" }},
		{name: "empty token", access: []byte("seller-oauth-test"), operation: testOperationID, mutate: func(p *PaymentRequest) { p.Token = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := validPaymentRequest()
			if tc.mutate != nil {
				tc.mutate(&request)
			}
			if _, err := gateway.Create(context.Background(), tc.access, tc.operation, request); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached provider %d times", calls)
	}
}

func TestCreatePaymentDoesNotFollowRedirects(t *testing.T) {
	followed := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer redirectTarget.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/capture", http.StatusFound)
	}))
	defer provider.Close()

	_, err := newGateway(provider.URL, provider.Client()).Create(context.Background(), []byte("seller-oauth-test"), testOperationID, validPaymentRequest())
	var providerErr *ProviderFailure
	if !errors.As(err, &providerErr) || providerErr.HTTPStatus != http.StatusFound || followed {
		t.Fatalf("redirect was followed or mishandled: err=%v followed=%v", err, followed)
	}
}

func TestParsePaymentDistinguishesMissingFeeAndLiveMode(t *testing.T) {
	result, err := parsePayment([]byte(`{"id":"12345","collector_id":3727396802,"external_reference":"`+testOperationID+`","status":"processing","status_detail":"private_unknown_detail","transaction_amount":12000,"currency_id":"CLP","metadata":{"operation_id":"`+testOperationID+`"}}`), "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.ApplicationFeeCLP != nil || result.LiveModePresent || result.Verified.LiveMode {
		t.Fatalf("missing provider flags got conflated with false/zero: %+v", result)
	}
	if result.Verified.Status != "unknown" || result.StatusDetail != "other" || strings.Contains(result.StatusDetail, "private_unknown") {
		t.Fatalf("unrecognized provider values were not normalized: %+v", result)
	}
}

func validPaymentRequest() PaymentRequest {
	return PaymentRequest{
		Token: "single-use-token", PaymentMethodID: "visa", Payer: Payer{Email: "buyer@example.test"},
		TransactionAmountCLP: 12000, ApplicationFeeCLP: 428, ExternalReference: testOperationID,
		Metadata: map[string]string{"operation_id": testOperationID},
	}
}
