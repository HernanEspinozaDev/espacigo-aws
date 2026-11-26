package rentalflowhttp

import (
	"context"
	"net/http"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

const pendingPaymentCancellationPath = "/api/v1/commercial-reservations/{reservationID}/cancel-pending-payment"

func (h *Handler) mountPendingPaymentCancellation() {
	h.mux.HandleFunc("POST "+pendingPaymentCancellationPath, h.cancelPendingPayment)
	h.mux.HandleFunc("OPTIONS "+pendingPaymentCancellationPath, func(w http.ResponseWriter, r *http.Request) {
		h.options(w, r, http.MethodPost)
	})
	h.mux.HandleFunc(pendingPaymentCancellationPath, func(w http.ResponseWriter, _ *http.Request) {
		methodNotAllowed(w, "POST, OPTIONS")
	})
}

func (h *Handler) cancelPendingPayment(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || requestBodyPresent(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	principal, ok := h.authorize(w, r)
	if !ok {
		return
	}
	reservationID := r.PathValue("reservationID")
	if !rentalflow.ValidUUID(reservationID) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	service, ok := h.reservations.(interface {
		CancelPendingPayment(context.Context, string, string) (rentalflow.PendingPaymentCancellationResult, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	result, err := service.CancelPendingPayment(r.Context(), principal.AccountID, reservationID)
	if err != nil {
		writeReservationError(w, err)
		return
	}
	status := http.StatusOK
	switch result.PaymentStatus {
	case "approved", "cancelled":
	case "pending":
		status = http.StatusAccepted
	default:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(w, status, map[string]any{"data": result})
}
