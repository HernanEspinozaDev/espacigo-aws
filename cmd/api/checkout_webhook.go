package main

import (
	"errors"
	"net/http"
)

func mountCheckoutWebhook(mux *http.ServeMux, handler http.Handler) error {
	if mux == nil || handler == nil {
		return errors.New("Checkout Pro webhook route is unavailable")
	}
	mux.Handle("POST /api/v1/webhooks/mercadopago", handler)
	return nil
}
