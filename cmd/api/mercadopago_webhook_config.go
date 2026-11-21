package main

import (
	"errors"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
)

// mercadoPagoWebhookOnlyEnabled permits the signed payment receiver and its
// worker to run without enabling the Checkout Pro preference API. The
// commercial-flow gate already proves that the runtime is the local loopback
// prototype; this additional opt-in is test-mode only.
func mercadoPagoWebhookOnlyEnabled(getenv func(string) string, commercialFlowEnabled bool) (bool, error) {
	if getenv == nil {
		return false, errors.New("Mercado Pago webhook configuration is unavailable")
	}
	switch strings.TrimSpace(getenv("MERCADOPAGO_WEBHOOK_ENABLED")) {
	case "", "0":
		return false, nil
	case "1":
	default:
		return false, errors.New("MERCADOPAGO_WEBHOOK_ENABLED must be 1")
	}
	if !commercialFlowEnabled {
		return false, errors.New("Mercado Pago webhook-only mode requires local commercial flow")
	}
	settings, err := marketplaceoauth.ConfigFromEnv(getenv)
	if err != nil || settings == nil || !settings.TestMode {
		return false, errors.New("Mercado Pago webhook-only mode requires test OAuth configuration")
	}
	return true, nil
}
