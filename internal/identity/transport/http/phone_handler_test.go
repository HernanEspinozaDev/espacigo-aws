package identityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPhoneEndpointsRoutingAndAuthentication(t *testing.T) {
	h := NewHandler(nil, nil, nil).(*Handler)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/account/security/phone"},
		{http.MethodPost, "/api/v1/account/security/phone/send-code"},
		{http.MethodPost, "/api/v1/account/security/phone/verify"},
	}

	for _, rt := range routes {
		t.Run("missing_auth_"+rt.path, func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`))
			if rt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401, got %d", rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("expected WWW-Authenticate: Bearer header")
			}
			var body struct {
				Error struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response error: %v", err)
			}
			if body.Error.Code != "unauthenticated" {
				t.Fatalf("expected code unauthenticated, got %s", body.Error.Code)
			}
			if len(body.Error.Message) > 100 {
				t.Fatalf("error message too long: %d chars", len(body.Error.Message))
			}
			if body.Error.RequestID == "" {
				t.Fatalf("expected non-empty request_id")
			}
		})

		t.Run("wrong_method_"+rt.path, func(t *testing.T) {
			wrongMethod := http.MethodDelete
			req := httptest.NewRequest(wrongMethod, rt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected status 405, got %d", rec.Code)
			}
		})
	}
}

func TestPhoneErrorMapping(t *testing.T) {
	h := NewHandler(nil, nil, nil).(*Handler)

	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantCode       string
		wantRetryAfter string
	}{
		{
			name:       "invalid phone",
			err:        identity.ErrInvalidPhone,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_phone",
		},
		{
			name:           "cooldown active",
			err:            identity.ErrPhoneCooldownActive,
			wantStatus:     http.StatusTooManyRequests,
			wantCode:       "cooldown_active",
			wantRetryAfter: "60",
		},
		{
			name:       "not pending",
			err:        identity.ErrPhoneNotPending,
			wantStatus: http.StatusBadRequest,
			wantCode:   "no_pending_verification",
		},
		{
			name:       "max attempts exceeded",
			err:        identity.ErrPhoneMaxAttempts,
			wantStatus: http.StatusTooManyRequests,
			wantCode:   "max_attempts_exceeded",
		},
		{
			name:       "code expired",
			err:        identity.ErrPhoneCodeExpired,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "code_expired",
		},
		{
			name:       "code invalid",
			err:        identity.ErrPhoneCodeInvalid,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "invalid_code",
		},
		{
			name:       "unexpected error",
			err:        errors.New("db connection down"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("X-Request-ID", "req-test-123")
			h.phoneError(rec, tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantRetryAfter != "" && rec.Header().Get("Retry-After") != tc.wantRetryAfter {
				t.Fatalf("Retry-After = %q, want %q", rec.Header().Get("Retry-After"), tc.wantRetryAfter)
			}

			var body struct {
				Error struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Error.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Error.Code, tc.wantCode)
			}
			if len(body.Error.Message) == 0 || len(body.Error.Message) > 100 {
				t.Fatalf("message length = %d, want 1..100: %q", len(body.Error.Message), body.Error.Message)
			}
			if body.Error.RequestID != "req-test-123" {
				t.Fatalf("request_id = %q, want req-test-123", body.Error.RequestID)
			}
		})
	}
}
