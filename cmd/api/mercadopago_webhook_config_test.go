package main

import (
	"encoding/base64"
	"testing"
)

func TestMercadoPagoWebhookOnlyConfiguration(t *testing.T) {
	valid := map[string]string{
		"MERCADOPAGO_WEBHOOK_ENABLED":      "1",
		"MERCADOPAGO_CLIENT_ID":            "test-client-id",
		"MERCADOPAGO_CLIENT_SECRET":        "test-client-secret",
		"MERCADOPAGO_OAUTH_REDIRECT_URI":   "https://preview.example.test/api/backend/host/mercadopago/oauth/callback",
		"MERCADOPAGO_TOKEN_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"MERCADOPAGO_TEST_MODE":            "true",
	}

	tests := []struct {
		name        string
		values      map[string]string
		commercial  bool
		wantEnabled bool
		wantErr     bool
	}{
		{name: "unset is disabled", values: map[string]string{}, wantEnabled: false},
		{name: "zero is disabled", values: map[string]string{"MERCADOPAGO_WEBHOOK_ENABLED": "0"}, wantEnabled: false},
		{name: "invalid truthy value rejected", values: map[string]string{"MERCADOPAGO_WEBHOOK_ENABLED": "true"}, wantErr: true},
		{name: "requires local commercial flow", values: valid, wantErr: true},
		{name: "requires complete OAuth config", values: map[string]string{"MERCADOPAGO_WEBHOOK_ENABLED": "1"}, commercial: true, wantErr: true},
		{name: "rejects live OAuth mode", values: withWebhookMode(valid, "false"), commercial: true, wantErr: true},
		{name: "accepts test OAuth mode", values: valid, commercial: true, wantEnabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mercadoPagoWebhookOnlyEnabled(func(key string) string { return tc.values[key] }, tc.commercial)
			if (err != nil) != tc.wantErr || got != tc.wantEnabled {
				t.Fatalf("enabled=%v err=%v, want enabled=%v error=%v", got, err != nil, tc.wantEnabled, tc.wantErr)
			}
		})
	}
}

func TestWebhookOnlyFlagDoesNotEnableCheckoutProPreferences(t *testing.T) {
	values := map[string]string{
		"MERCADOPAGO_WEBHOOK_ENABLED": "1",
		"MERCADOPAGO_TEST_MODE":       "true",
	}
	service, err := newCheckoutPreferenceServiceWithGateway(nil, nil, config{}, func(key string) string { return values[key] }, nil)
	if err != nil || service != nil {
		t.Fatalf("webhook-only flag unexpectedly enabled Checkout Pro: service=%v err=%v", service != nil, err)
	}
}

func TestReadMercadoPagoWebhookSecretSupportsEnvAndFile(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef"
	got, err := readMercadoPagoWebhookSecret(func(key string) string {
		if key == "MERCADOPAGO_WEBHOOK_SECRET" {
			return "  " + valid + "\n"
		}
		return ""
	})
	if err != nil || string(got) != valid {
		t.Fatalf("expected env webhook secret to succeed: got=%q err=%v", string(got), err)
	}
	if _, err := readMercadoPagoWebhookSecret(func(key string) string {
		if key == "MERCADOPAGO_WEBHOOK_SECRET" {
			return "too-short"
		}
		return ""
	}); err == nil {
		t.Fatal("expected short env webhook secret to fail")
	}
	if _, err := readMercadoPagoWebhookSecret(func(string) string { return "" }); err == nil {
		t.Fatal("expected missing webhook secret to fail")
	}
}

func withWebhookMode(values map[string]string, mode string) map[string]string {
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	copy["MERCADOPAGO_TEST_MODE"] = mode
	return copy
}
