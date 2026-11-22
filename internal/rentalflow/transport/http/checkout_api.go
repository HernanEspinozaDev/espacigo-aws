package rentalflowhttp

import (
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutapi"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

func (h *Handler) getPaymentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || requestBodyPresent(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	principal, ok := h.authorize(w, r)
	if !ok {
		return
	}
	id := r.PathValue("quoteID")
	if !rentalflow.ValidUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "checkout_not_configured")
		return
	}
	result, err := h.payments.PaymentStatus(r.Context(), principal.AccountID, strings.ToLower(id))
	if err != nil {
		writeCheckoutError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) createPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("quoteID")
	keys := r.Header.Values("Idempotency-Key")
	if r.URL.RawQuery != "" || !rentalflow.ValidUUID(id) || len(keys) != 1 || !rentalflow.ValidUUID(keys[0]) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	principal, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "checkout_not_configured")
		return
	}
	var input checkoutapi.Input
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.payments.Pay(r.Context(), principal.AccountID, strings.ToLower(id), strings.ToLower(keys[0]), input)
	if err != nil {
		writeCheckoutError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
