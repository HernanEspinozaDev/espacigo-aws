package main

import "testing"

func TestSellerPublicKeyTokenizationIsOptInAndSandboxOnly(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
		fail bool
	}{
		{name: "default disabled", env: map[string]string{}, want: false},
		{name: "explicit disabled", env: map[string]string{"MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY": "0"}, want: false},
		{name: "sandbox enabled", env: map[string]string{"MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY": "1", "MERCADOPAGO_PAYMENTS_API_ENABLED": "1", "MERCADOPAGO_TEST_MODE": "true"}, want: true},
		{name: "payments api disabled", env: map[string]string{"MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY": "1", "MERCADOPAGO_TEST_MODE": "true"}, fail: true},
		{name: "production mode blocked", env: map[string]string{"MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY": "1", "MERCADOPAGO_PAYMENTS_API_ENABLED": "1", "MERCADOPAGO_TEST_MODE": "false"}, fail: true},
		{name: "invalid value", env: map[string]string{"MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY": "yes", "MERCADOPAGO_PAYMENTS_API_ENABLED": "1", "MERCADOPAGO_TEST_MODE": "true"}, fail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mercadoPagoSellerPublicKeyTokenizationEnabled(func(key string) string { return tc.env[key] })
			if (err != nil) != tc.fail || got != tc.want {
				t.Fatalf("got enabled=%t err=%v; want enabled=%t error=%t", got, err, tc.want, tc.fail)
			}
		})
	}
}
