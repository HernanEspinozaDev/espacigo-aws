package rentalflowhttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

func (h *Handler) reconcileCommercialRefund(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	principal, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body struct {
		RefundID string `json:"refund_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	service, ok := h.reservations.(interface {
		ReconcileExternalRefund(context.Context, string, string, string) (rentalflow.CommercialCancellation, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	result, err := service.ReconcileExternalRefund(r.Context(), principal.AccountID, r.PathValue("reservationID"), body.RefundID)
	if errors.Is(err, rentalflow.ErrRefundRequiresReconciliation) {
		writeError(w, http.StatusConflict, "refund_requires_reconciliation")
		return
	}
	if err != nil {
		writeReservationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
