package rentalflowhttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type cancellationHandlerStub struct {
	*commercialReservationStub
	previewCalls int
	cancelCalls  int
	cancelErr    error
	preview      rentalflow.CommercialCancellation
}

func (s *cancellationHandlerStub) PreviewCancellation(context.Context, string, string) (rentalflow.CommercialCancellation, error) {
	s.previewCalls++
	return s.preview, nil
}
func (s *cancellationHandlerStub) Cancel(_ context.Context, _, _, reason string, expected int64) (rentalflow.CommercialCancellation, error) {
	s.cancelCalls++
	if s.cancelErr != nil {
		return rentalflow.CommercialCancellation{}, s.cancelErr
	}
	s.preview.Reason = reason + ":" + fmt.Sprint(expected)
	s.preview.RefundStatus = "aprobado"
	return s.preview, nil
}

func TestCommercialCancellationHTTPRoutesAreAuthenticatedAndStrict(t *testing.T) {
	const reservationID = "22222222-2222-4222-8222-222222222222"
	service := &cancellationHandlerStub{commercialReservationStub: &commercialReservationStub{}, preview: rentalflow.CommercialCancellation{ReservationID: reservationID, CanCancel: true, Reason: "eligible", PolicyVersion: "facil_v1", PolicyText: "Devolución total", RefundPercent: 100, TotalChargedCLP: 12000, RefundAmountCLP: 12000, Currency: "CLP", StartsAt: "2030-01-01T00:00:00Z"}}
	h := New(&authStub{principal: identity.Principal{AccountID: testActorID}}, nil, nil, nil, nil, service)
	preview := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+reservationID+"/cancellation-preview", nil)
	preview.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, preview)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cancellation_policy_version":"facil_v1"`) || service.previewCalls != 1 {
		t.Fatalf("preview status=%d calls=%d body=%s", w.Code, service.previewCalls, w.Body.String())
	}
	cancel := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel", strings.NewReader(`{"reason":"Cambio de planes","expected_refund_amount_clp":12000}`))
	cancel.Header.Set("Authorization", "Bearer token")
	cancel.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, cancel)
	if w.Code != http.StatusOK || service.cancelCalls != 1 || !strings.Contains(w.Body.String(), `"refund_status":"aprobado"`) {
		t.Fatalf("cancel status=%d calls=%d body=%s", w.Code, service.cancelCalls, w.Body.String())
	}
	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel", strings.NewReader(`{"reason":"x","expected_refund_amount_clp":-1}`))
	invalid.Header.Set("Authorization", "Bearer token")
	invalid.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, invalid)
	if w.Code != http.StatusBadRequest || service.cancelCalls != 1 {
		t.Fatalf("invalid cancel status=%d calls=%d", w.Code, service.cancelCalls)
	}
	service.cancelErr = rentalflow.ErrRefundRequiresReconciliation
	manual := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel", strings.NewReader(`{"reason":"Cambio de planes","expected_refund_amount_clp":12000}`))
	manual.Header.Set("Authorization", "Bearer token")
	manual.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, manual)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"refund_requires_reconciliation"`) {
		t.Fatalf("reconciliation conflict status=%d body=%s", w.Code, w.Body.String())
	}
	head := httptest.NewRequest(http.MethodHead, "/api/v1/commercial-reservations/"+reservationID+"/cancellation-preview", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, head)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD status=%d", w.Code)
	}
}
