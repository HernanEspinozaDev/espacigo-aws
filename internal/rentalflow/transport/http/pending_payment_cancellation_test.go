package rentalflowhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type pendingCancellationHandlerStub struct {
	*commercialReservationStub
	calls         int
	lastActor     string
	lastResID     string
	result        rentalflow.PendingPaymentCancellationResult
	err           error
}

func (s *pendingCancellationHandlerStub) CancelPendingPayment(_ context.Context, actorID, reservationID string) (rentalflow.PendingPaymentCancellationResult, error) {
	s.calls++
	s.lastActor = actorID
	s.lastResID = reservationID
	return s.result, s.err
}

func TestPendingPaymentCancellationHTTPRoute(t *testing.T) {
	const reservationID = "22222222-2222-4222-8222-222222222222"
	const origin = "https://example.com"

	t.Run("200 cancelled and approved", func(t *testing.T) {
		for _, status := range []string{"cancelled", "approved"} {
			stub := &pendingCancellationHandlerStub{
				commercialReservationStub: &commercialReservationStub{},
				result:                    rentalflow.PendingPaymentCancellationResult{PaymentStatus: status},
			}
			h := New(&authStub{principal: identity.Principal{AccountID: testActorID}}, nil, nil, nil, nil, stub)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"payment_status":"`+status+`"`) {
				t.Fatalf("status=%q code=%d body=%s", status, w.Code, w.Body.String())
			}
			if stub.calls != 1 || stub.lastActor != testActorID || stub.lastResID != reservationID {
				t.Fatalf("unexpected service call: %+v", stub)
			}
		}
	})

	t.Run("202 pending", func(t *testing.T) {
		stub := &pendingCancellationHandlerStub{
			commercialReservationStub: &commercialReservationStub{},
			result:                    rentalflow.PendingPaymentCancellationResult{PaymentStatus: "pending"},
		}
		h := New(&authStub{principal: identity.Principal{AccountID: testActorID}}, nil, nil, nil, nil, stub)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"payment_status":"pending"`) {
			t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("error mappings", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			err      error
			wantCode int
			wantBody string
		}{
			{name: "non-participant 404", err: rentalflow.ErrNotFound, wantCode: http.StatusNotFound, wantBody: `"code":"not_found"`},
			{name: "conflict 409", err: rentalflow.ErrConflict, wantCode: http.StatusConflict, wantBody: `"code":"conflict"`},
			{name: "unavailable 503", err: rentalflow.ErrCancellationUnavailable, wantCode: http.StatusServiceUnavailable, wantBody: `"code":"service_unavailable"`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stub := &pendingCancellationHandlerStub{commercialReservationStub: &commercialReservationStub{}, err: tc.err}
				h := New(&authStub{principal: identity.Principal{AccountID: testActorID}}, nil, nil, nil, nil, stub)
				r := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
				r.Header.Set("Authorization", "Bearer token")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != tc.wantCode || !strings.Contains(w.Body.String(), tc.wantBody) {
					t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
				}
			})
		}
	})

	t.Run("validation, auth, options, and method guards", func(t *testing.T) {
		stub := &pendingCancellationHandlerStub{commercialReservationStub: &commercialReservationStub{}}
		h := New(&authStub{principal: identity.Principal{AccountID: testActorID}}, nil, nil, nil, []string{origin}, stub)

		// Reject query params and body.
		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment?extra=1", nil),
			httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", strings.NewReader(`{}`)),
			httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/not-a-uuid/cancel-pending-payment", nil),
		} {
			req.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest || stub.calls != 0 {
				t.Fatalf("%s: expected 400 with 0 calls, got %d (calls=%d)", req.URL, w.Code, stub.calls)
			}
		}

		// OPTIONS preflight.
		opt := httptest.NewRequest(http.MethodOptions, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
		opt.Header.Set("Origin", origin)
		opt.Header.Set("Access-Control-Request-Method", http.MethodPost)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, opt)
		if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Methods") != http.MethodPost {
			t.Fatalf("OPTIONS code=%d allow=%q", w.Code, w.Header().Get("Access-Control-Allow-Methods"))
		}

		// Disallowed methods.
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete} {
			req := httptest.NewRequest(method, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusMethodNotAllowed || !strings.Contains(w.Header().Get("Allow"), "POST") {
				t.Fatalf("%s: code=%d allow=%q", method, w.Code, w.Header().Get("Allow"))
			}
		}

		// Auth rejection.
		unauthHandler := New(&authStub{err: identity.ErrUnauthorized}, nil, nil, nil, nil, stub)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+reservationID+"/cancel-pending-payment", nil)
		w = httptest.NewRecorder()
		unauthHandler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized || stub.calls != 0 {
			t.Fatalf("unauth code=%d calls=%d", w.Code, stub.calls)
		}
	})
}
