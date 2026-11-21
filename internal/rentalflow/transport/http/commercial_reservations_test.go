package rentalflowhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type commercialReservationStub struct {
	listFn func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error)
	getFn  func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error)
	calls  int
}

func (s *commercialReservationStub) List(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
	s.calls++
	if s.listFn != nil {
		return s.listFn(ctx, actor, pageSize, cursor)
	}
	return rentalflow.CommercialReservationPage{Items: []rentalflow.CommercialReservation{}}, nil
}

func (s *commercialReservationStub) Get(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
	s.calls++
	if s.getFn != nil {
		return s.getFn(ctx, actor, reservationID)
	}
	return rentalflow.CommercialReservation{}, nil
}

func TestCommercialReservations_AuthAndPreflight(t *testing.T) {
	const origin = "https://example.com"
	auth := &authStub{principal: identity.Principal{AccountID: testActorID}}
	resStub := &commercialReservationStub{}
	handler := New(auth, nil, nil, nil, []string{origin}, resStub)

	t.Run("OPTIONS list preflight", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/commercial-reservations", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", http.MethodGet)
		r.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d (body: %s)", w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Methods") != http.MethodGet {
			t.Errorf("expected GET allow method, got %s", w.Header().Get("Access-Control-Allow-Methods"))
		}
	})

	t.Run("OPTIONS detail preflight", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/commercial-reservations/22222222-2222-4222-8222-222222222222", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", http.MethodGet)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d (body: %s)", w.Code, w.Body.String())
		}
	})

	t.Run("methods not allowed", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead} {
			r := httptest.NewRequest(method, "/api/v1/commercial-reservations", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /commercial-reservations: expected 405, got %d", method, w.Code)
			}
			if !strings.Contains(w.Header().Get("Allow"), "GET") {
				t.Errorf("%s /commercial-reservations: expected Allow header containing GET, got %s", method, w.Header().Get("Allow"))
			}

			r2 := httptest.NewRequest(method, "/api/v1/commercial-reservations/22222222-2222-4222-8222-222222222222", nil)
			w2 := httptest.NewRecorder()
			handler.ServeHTTP(w2, r2)
			if w2.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /commercial-reservations/{id}: expected 405, got %d", method, w2.Code)
			}
		}
	})

	t.Run("unauthenticated missing token", func(t *testing.T) {
		unauth := &authStub{err: identity.ErrUnauthorized}
		h := New(unauth, nil, nil, nil, nil, resStub)
		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
		if w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("expected WWW-Authenticate: Bearer, got %q", w.Header().Get("WWW-Authenticate"))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		forbiddenAuth := &authStub{err: identity.ErrForbidden}
		h := New(forbiddenAuth, nil, nil, nil, nil, resStub)
		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations", nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("service unavailable when unconfigured", func(t *testing.T) {
		hWithoutSvc := New(auth, nil, nil, nil, nil)
		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations", nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		hWithoutSvc.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})
}

func TestCommercialReservations_List(t *testing.T) {
	auth := &authStub{principal: identity.Principal{AccountID: testActorID}}

	t.Run("query validations", func(t *testing.T) {
		resStub := &commercialReservationStub{}
		handler := New(auth, nil, nil, nil, nil, resStub)

		invalidCases := []struct {
			name string
			url  string
			body string
		}{
			{"unknown query key", "/api/v1/commercial-reservations?extra=1", ""},
			{"repeated page_size", "/api/v1/commercial-reservations?page_size=10&page_size=20", ""},
			{"repeated cursor", "/api/v1/commercial-reservations?cursor=a&cursor=b", ""},
			{"non-integer page_size", "/api/v1/commercial-reservations?page_size=abc", ""},
			{"page_size zero", "/api/v1/commercial-reservations?page_size=0", ""},
			{"page_size negative", "/api/v1/commercial-reservations?page_size=-5", ""},
			{"page_size too large", "/api/v1/commercial-reservations?page_size=101", ""},
			{"cursor too long", "/api/v1/commercial-reservations?cursor=" + strings.Repeat("x", 4097), ""},
			{"request body present", "/api/v1/commercial-reservations", `{"bad": true}`},
		}

		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var r *http.Request
				if tc.body != "" {
					r = httptest.NewRequest(http.MethodGet, tc.url, strings.NewReader(tc.body))
				} else {
					r = httptest.NewRequest(http.MethodGet, tc.url, nil)
				}
				r.Header.Set("Authorization", "Bearer valid-token")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d (body: %s)", w.Code, w.Body.String())
				}
				var errResp map[string]map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("malformed error JSON: %v", err)
				}
				if errResp["error"]["code"] != "invalid_request" {
					t.Errorf("expected code invalid_request, got %s", errResp["error"]["code"])
				}
			})
		}
	})

	t.Run("success empty list with default page_size", func(t *testing.T) {
		var capturedPageSize int
		var capturedCursor string
		resStub := &commercialReservationStub{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				capturedPageSize = pageSize
				capturedCursor = cursor
				return rentalflow.CommercialReservationPage{Items: []rentalflow.CommercialReservation{}}, nil
			},
		}
		handler := New(auth, nil, nil, nil, nil, resStub)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations", nil)
		r.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		if capturedPageSize != 20 {
			t.Errorf("expected default pageSize 20, got %d", capturedPageSize)
		}
		if capturedCursor != "" {
			t.Errorf("expected empty cursor, got %s", capturedCursor)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control no-store, got %s", w.Header().Get("Cache-Control"))
		}

		var resp struct {
			Data struct {
				Items      []rentalflow.CommercialReservation `json:"items"`
				NextCursor string                             `json:"next_cursor"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Data.Items == nil || len(resp.Data.Items) != 0 {
			t.Errorf("expected empty items slice, got %#v", resp.Data.Items)
		}
	})

	t.Run("success with items and custom pagination", func(t *testing.T) {
		resStub := &commercialReservationStub{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				if pageSize != 5 || cursor != "cur123" {
					t.Errorf("unexpected args: pageSize=%d cursor=%s", pageSize, cursor)
				}
				return rentalflow.CommercialReservationPage{
					Items: []rentalflow.CommercialReservation{
						{
							ID:              "c1387d7b-99f4-41d6-847e-4074218f26a1",
							QuoteID:         "84724d16-628b-4b24-b152-cb4db6e1e779",
							SpaceID:         "d0e14db8-2d88-4447-975d-5fbddaa1e012",
							SpaceTitle:      "Oficina El Golf",
							ParticipantRole: "arrendatario",
							Status:          "pendiente_de_pago",
							CheckoutStatus:  "iniciada",
							RateUnit:        "hora",
							UnitPriceCLP:    15000,
							Units:           2,
							SubtotalCLP:     30000,
							Currency:        "CLP",
							StartAt:         "2026-11-20T10:00:00Z",
							EndAt:           "2026-11-20T12:00:00Z",
							TimeZone:        "America/Santiago",
							CreatedAt:       "2026-11-19T18:00:00Z",
							UpdatedAt:       "2026-11-19T18:00:00Z",
						},
					},
					NextCursor: "next456",
				}, nil
			},
		}
		handler := New(auth, nil, nil, nil, nil, resStub)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations?page_size=5&cursor=cur123", nil)
		r.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				Items      []rentalflow.CommercialReservation `json:"items"`
				NextCursor string                             `json:"next_cursor"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Data.Items) != 1 || resp.Data.NextCursor != "next456" {
			t.Fatalf("unexpected response data: %+v", resp)
		}
		item := resp.Data.Items[0]
		if item.ID != "c1387d7b-99f4-41d6-847e-4074218f26a1" || item.UnitPriceCLP != 15000 || item.SubtotalCLP != 30000 {
			t.Errorf("item fields mismatch: %+v", item)
		}
	})

	t.Run("service error returns 503", func(t *testing.T) {
		resStub := &commercialReservationStub{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				return rentalflow.CommercialReservationPage{}, errors.New("db timeout")
			},
		}
		handler := New(auth, nil, nil, nil, nil, resStub)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations", nil)
		r.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})
}

func TestCommercialReservations_GetDetail(t *testing.T) {
	auth := &authStub{principal: identity.Principal{AccountID: testActorID}}
	validID := "c1387d7b-99f4-41d6-847e-4074218f26a1"

	t.Run("request validations", func(t *testing.T) {
		resStub := &commercialReservationStub{}
		handler := New(auth, nil, nil, nil, nil, resStub)

		// query params forbidden on detail
		r1 := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+validID+"?extra=1", nil)
		r1.Header.Set("Authorization", "Bearer token")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, r1)
		if w1.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on raw query, got %d", w1.Code)
		}

		// body present forbidden on detail
		r2 := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+validID, strings.NewReader("bad"))
		r2.Header.Set("Authorization", "Bearer token")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, r2)
		if w2.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on body present, got %d", w2.Code)
		}

		// invalid uuid
		r3 := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/not-a-uuid", nil)
		r3.Header.Set("Authorization", "Bearer token")
		w3 := httptest.NewRecorder()
		handler.ServeHTTP(w3, r3)
		if w3.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on invalid uuid, got %d", w3.Code)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		resStub := &commercialReservationStub{
			getFn: func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
				return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
			},
		}
		handler := New(auth, nil, nil, nil, nil, resStub)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+validID, nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
		var errResp map[string]map[string]string
		json.Unmarshal(w.Body.Bytes(), &errResp)
		if errResp["error"]["code"] != "not_found" {
			t.Errorf("expected error code not_found, got %s", errResp["error"]["code"])
		}
	})

	t.Run("success detail", func(t *testing.T) {
		expected := rentalflow.CommercialReservation{
			ID:              validID,
			QuoteID:         "84724d16-628b-4b24-b152-cb4db6e1e779",
			SpaceID:         "d0e14db8-2d88-4447-975d-5fbddaa1e012",
			SpaceTitle:      "Oficina El Golf",
			ParticipantRole: "anfitrion",
			Status:          "pagada",
			CheckoutStatus:  "approved",
			RateUnit:        "mes",
			UnitPriceCLP:    500000,
			Units:           1,
			SubtotalCLP:     500000,
			Currency:        "CLP",
			StartAt:         "2026-12-01T00:00:00Z",
			EndAt:           "2027-01-01T00:00:00Z",
			TimeZone:        "America/Santiago",
			CreatedAt:       "2026-11-19T18:00:00Z",
			UpdatedAt:       "2026-11-19T18:30:00Z",
		}
		resStub := &commercialReservationStub{
			getFn: func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
				if actor != testActorID || reservationID != validID {
					t.Fatalf("unexpected call args actor=%s id=%s", actor, reservationID)
				}
				return expected, nil
			},
		}
		handler := New(auth, nil, nil, nil, nil, resStub)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+validID, nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}
		if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("expected json content-type, got %s", w.Header().Get("Content-Type"))
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control no-store, got %s", w.Header().Get("Cache-Control"))
		}

		var resp struct {
			Data rentalflow.CommercialReservation `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Data.ID != validID || resp.Data.ParticipantRole != "anfitrion" || resp.Data.Status != "pagada" {
			t.Errorf("response data mismatch: %+v", resp.Data)
		}
	})
}
