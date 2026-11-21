package main

import (
	"errors"
	"strings"
)

func mercadoPagoWalletPurchaseEnabled(getenv func(string) string) (bool, error) {
	if getenv == nil {
		return false, errors.New("Mercado Pago configuration is unavailable")
	}
	switch strings.TrimSpace(getenv("MERCADOPAGO_WALLET_PURCHASE_ENABLED")) {
	case "", "0":
		return false, nil
	case "1":
		if strings.TrimSpace(getenv("MERCADOPAGO_CHECKOUT_ENABLED")) != "1" || strings.TrimSpace(getenv("MERCADOPAGO_TEST_MODE")) != "true" {
			return false, errors.New("wallet purchase requires test-mode Checkout Pro")
		}
		return true, nil
	default:
		return false, errors.New("MERCADOPAGO_WALLET_PURCHASE_ENABLED must be 1")
	}
}
