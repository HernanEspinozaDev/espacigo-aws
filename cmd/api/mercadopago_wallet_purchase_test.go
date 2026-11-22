package main

import "testing"

func TestMercadoPagoWalletPurchaseFlagRequiresTestCheckoutPro(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		want    bool
		wantErr bool
	}{
		{name: "unset is disabled", values: map[string]string{}},
		{name: "zero is disabled", values: map[string]string{"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "0"}},
		{name: "invalid value rejected", values: map[string]string{"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "true"}, wantErr: true},
		{name: "requires Checkout Pro", values: map[string]string{"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "1", "MERCADOPAGO_TEST_MODE": "true"}, wantErr: true},
		{name: "requires test mode", values: map[string]string{"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "1", "MERCADOPAGO_CHECKOUT_ENABLED": "1"}, wantErr: true},
		{name: "test Checkout Pro enables wallet purchase", values: map[string]string{"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "1", "MERCADOPAGO_CHECKOUT_ENABLED": "1", "MERCADOPAGO_TEST_MODE": "true"}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mercadoPagoWalletPurchaseEnabled(func(key string) string { return tc.values[key] })
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("enabled=%v err=%v, want enabled=%v error=%v", got, err != nil, tc.want, tc.wantErr)
			}
		})
	}
}

func TestPaymentsAndWalletCheckoutCanBeEnabledTogetherInTestWebhookMode(t *testing.T) {
	values := map[string]string{
		"MERCADOPAGO_PAYMENTS_API_ENABLED":    "1",
		"MERCADOPAGO_CHECKOUT_ENABLED":        "1",
		"MERCADOPAGO_WALLET_PURCHASE_ENABLED": "1",
		"MERCADOPAGO_TEST_MODE":               "true",
	}
	got, err := mercadoPagoPaymentsEnabled(func(key string) string { return values[key] }, true)
	if err != nil || !got {
		t.Fatalf("payments enabled=%v error=%v", got, err)
	}
	values["MERCADOPAGO_WALLET_PURCHASE_ENABLED"] = "0"
	if got, err = mercadoPagoPaymentsEnabled(func(key string) string { return values[key] }, true); err == nil || got {
		t.Fatalf("payments and Checkout Pro were enabled without the explicit wallet test gate: enabled=%v err=%v", got, err)
	}
}
