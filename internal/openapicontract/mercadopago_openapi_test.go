package openapicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutapi"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"gopkg.in/yaml.v3"
)

func TestMercadoPagoOpenAPIContractMatchesBackendImplementation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "planning", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	root := document.Content[0]
	paths := nodeAt(t, root, "paths")
	schemas := nodeAt(t, nodeAt(t, root, "components"), "schemas")

	expectedOps := []struct {
		path        string
		method      string
		operationID string
		public      bool
	}{
		{path: "/host/mercadopago", method: "get", operationID: "getHostMercadoPagoConnection"},
		{path: "/host/mercadopago/oauth/start", method: "post", operationID: "startHostMercadoPagoOAuth"},
		{path: "/host/mercadopago/oauth/callback", method: "get", operationID: "completeHostMercadoPagoOAuth", public: true},
		{path: "/webhooks/mercadopago", method: "post", operationID: "receiveMercadoPagoPaymentWebhook", public: true},
		{path: "/rental-quotes/{quoteID}/checkout", method: "post", operationID: "startCommercialQuoteCheckout"},
		{path: "/rental-quotes/{quoteID}/checkout", method: "get", operationID: "getCommercialQuoteCheckoutStatus"},
		{path: "/rental-quotes/{quoteID}/checkout-wallet", method: "post", operationID: "startCommercialQuoteWalletPurchase"},
		{path: "/rental-quotes/{quoteID}/checkout-api", method: "get", operationID: "getCommercialPayment"},
		{path: "/rental-quotes/{quoteID}/checkout-api", method: "post", operationID: "createCommercialPayment"},
	}

	for _, exp := range expectedOps {
		t.Run(exp.operationID, func(t *testing.T) {
			pathNode := nodeAt(t, paths, exp.path)
			opNode := nodeAt(t, pathNode, exp.method)
			if got := scalarAt(t, opNode, "operationId"); got != exp.operationID {
				t.Fatalf("operationId=%q, want %q", got, exp.operationID)
			}
			sec, hasSec := optionalMappingValue(opNode, "security")
			if exp.public {
				if !hasSec || sec.Kind != yaml.SequenceNode || len(sec.Content) != 0 {
					t.Fatalf("%s %s must explicitly disable inherited bearer security with security: []", exp.method, exp.path)
				}
			} else if hasSec && sec.Kind == yaml.SequenceNode && len(sec.Content) == 0 {
				t.Fatalf("%s %s must require authentication", exp.method, exp.path)
			}
		})
	}

	t.Run("webhook_declares_signed_query_and_header_parameters", func(t *testing.T) {
		webhookPost := nodeAt(t, nodeAt(t, paths, "/webhooks/mercadopago"), "post")
		params := nodeAt(t, webhookPost, "parameters")
		if params.Kind != yaml.SequenceNode {
			t.Fatal("webhook parameters must be a sequence")
		}
		found := map[string]string{}
		for _, item := range params.Content {
			found[scalarAt(t, item, "name")] = scalarAt(t, item, "in")
		}
		for name, location := range map[string]string{
			"data.id":      "query",
			"type":         "query",
			"x-signature":  "header",
			"x-request-id": "header",
		} {
			if got := found[name]; got != location {
				t.Fatalf("webhook parameter %q in=%q, want %q", name, got, location)
			}
		}
	})

	t.Run("runtime_dtos_match_mercadopago_schemas", func(t *testing.T) {
		expiresTime := time.Date(2026, time.November, 24, 18, 0, 0, 0, time.UTC)
		expiresAt := expiresTime.Format(time.RFC3339)
		pref := checkoutpro.StartResult{
			QuoteID:       "38a4edef-6291-48db-b7ed-e0a24911b399",
			OperationID:   "713e10df-8244-4489-87c9-5a8ed4c9d788",
			ReservationID: "713e10df-8244-4489-87c9-5a8ed4c9d788",
			Status:        "pending",
			InitPoint:     "https://sandbox.mercadopago.cl/checkout/v1/redirect?pref_id=123",
			ExpiresAt:     expiresTime,
		}
		prefBytes, err := json.Marshal(pref)
		if err != nil {
			t.Fatalf("marshal StartResult: %v", err)
		}
		var prefFields map[string]any
		if err := json.Unmarshal(prefBytes, &prefFields); err != nil {
			t.Fatalf("unmarshal StartResult: %v", err)
		}
		prefProps := nodeAt(t, nodeAt(t, schemas, "CheckoutPreference"), "properties")
		for k := range prefFields {
			if !hasMappingKey(prefProps, k) {
				t.Fatalf("CheckoutPreference schema missing runtime field %q", k)
			}
		}

		apiReq := checkoutapi.Input{
			Token:                 "a1b2c3d4e5f60718293a4b5c6d7e8f90",
			PaymentMethodID:       "visa",
			IssuerID:              "123",
			TokenizationPublicKey: "TEST-00000000-1111-2222-3333-444444444444",
			Payer: checkoutapi.Payer{
				Email:          "comprador.prueba@example.cl",
				Identification: &checkoutapi.Identification{Type: "RUT", Number: "123456785"},
			},
		}
		reqBytes, err := json.Marshal(apiReq)
		if err != nil {
			t.Fatalf("marshal checkoutapi.Input: %v", err)
		}
		var reqFields map[string]any
		if err := json.Unmarshal(reqBytes, &reqFields); err != nil {
			t.Fatalf("unmarshal checkoutapi.Input: %v", err)
		}
		reqProps := nodeAt(t, nodeAt(t, schemas, "CheckoutAPIPaymentRequest"), "properties")
		for k := range reqFields {
			if !hasMappingKey(reqProps, k) {
				t.Fatalf("CheckoutAPIPaymentRequest schema missing runtime field %q", k)
			}
		}

		apiRes := checkoutapi.Result{
			QuoteID:            "38a4edef-6291-48db-b7ed-e0a24911b399",
			AmountCLP:          24000,
			Status:             "approved",
			SpaceTitle:         "Sala Providencia",
			StartAt:            expiresAt,
			EndAt:              expiresAt,
			TimeZone:           "America/Santiago",
			ExpiresAt:          expiresAt,
			Units:              2,
			RateUnit:           "hora",
			PaymentID:          "1324567890",
			StatusDetail:       "accredited",
			ProviderHTTPStatus: 201,
			ProviderCode:       "0",
			ProviderRequestID:  "req-1",
			CardTokenization: &checkoutapi.CardTokenization{
				Mode:              "seller_public_key",
				PublicKey:         "TEST-00000000-1111-2222-3333-444444444444",
				ReconnectRequired: true,
			},
		}
		resBytes, err := json.Marshal(apiRes)
		if err != nil {
			t.Fatalf("marshal checkoutapi.Result: %v", err)
		}
		var resFields map[string]any
		if err := json.Unmarshal(resBytes, &resFields); err != nil {
			t.Fatalf("unmarshal checkoutapi.Result: %v", err)
		}
		envelopeDataProps := nodeAt(t, nodeAt(t, nodeAt(t, nodeAt(t, schemas, "CheckoutAPIPaymentEnvelope"), "properties"), "data"), "properties")
		for k := range resFields {
			if !hasMappingKey(envelopeDataProps, k) {
				t.Fatalf("CheckoutAPIPaymentEnvelope.data schema missing runtime field %q", k)
			}
		}
	})
}
