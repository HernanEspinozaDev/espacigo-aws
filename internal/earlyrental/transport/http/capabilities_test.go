package earlyrentalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type capabilitiesAuthStub struct {
	principal identity.Principal
	err       error
	secret    identity.Secret
	activity  identity.ActivityKind
	calls     int
}

func (a *capabilitiesAuthStub) Authorize(_ context.Context, secret identity.Secret, _ identity.Role, activity identity.ActivityKind) (identity.Principal, error) {
	a.calls++
	a.secret = secret
	a.activity = activity
	if a.err != nil {
		return identity.Principal{}, a.err
	}
	if secret == "" {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	return a.principal, a.err
}

func TestCapabilitiesRequiresSessionAndReportsDisabledByDefault(t *testing.T) {
	auth := &capabilitiesAuthStub{principal: identity.Principal{AccountID: "account-1"}}
	handler := NewCapabilitiesHandler(auth, CapabilityState{}, []string{"http://127.0.0.1:3000"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
	req.Header.Set("Authorization", "Bearer opaque-session-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("missing private response headers: %v", response.Header())
	}
	if auth.secret != "opaque-session-token" || auth.activity != identity.UserOperation {
		t.Fatalf("authorization input = secret %q, activity %d", auth.secret, auth.activity)
	}
	var body struct {
		Data struct {
			Enabled            bool     `json:"enabled"`
			FlowVersion        string   `json:"flow_version"`
			SupportedRateUnits []string `json:"supported_rate_units"`
			HoldTTLSeconds     int      `json:"hold_ttl_seconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Enabled || body.Data.FlowVersion != "manual_early_v1" || len(body.Data.SupportedRateUnits) != 1 || body.Data.SupportedRateUnits[0] != "hora" || body.Data.HoldTTLSeconds != 1800 {
		t.Fatalf("capabilities = %+v", body.Data)
	}
}

func TestCapabilitiesActivationFailsClosedUntilFlowIsReady(t *testing.T) {
	auth := &capabilitiesAuthStub{principal: identity.Principal{AccountID: "account-1"}}
	handler := NewCapabilitiesHandler(auth, CapabilityState{ActivationRequested: true}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
	req.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("requested but unready flow = status %d, body %s", response.Code, response.Body.String())
	}
}

func TestCapabilitiesReportsEnabledOnlyWhenRequestedAndReady(t *testing.T) {
	auth := &capabilitiesAuthStub{principal: identity.Principal{AccountID: "account-1"}}
	handler := NewCapabilitiesHandler(auth, CapabilityState{ActivationRequested: true, FlowReady: true}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
	req.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	var body struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || !body.Data.Enabled {
		t.Fatalf("ready capability = status %d body %s", response.Code, response.Body.String())
	}
}

func TestCapabilitiesMapsAuthenticationFailuresWithoutLeakingDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "unauthenticated", err: identity.ErrUnauthorized, want: http.StatusUnauthorized},
		{name: "forbidden", err: identity.ErrForbidden, want: http.StatusForbidden},
		{name: "restricted", err: identity.ErrRestrictedMode, want: http.StatusForbidden},
		{name: "dependency unavailable", err: errors.New("private database details"), want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := &capabilitiesAuthStub{err: tc.err}
			handler := NewCapabilitiesHandler(auth, CapabilityState{}, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
			req.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want || strings.Contains(response.Body.String(), "private database details") || strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("auth failure = status %d body %s", response.Code, response.Body.String())
			}
			if tc.want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("unauthenticated response missing challenge: %v", response.Header())
			}
		})
	}
}

func TestCapabilitiesRejectsQueryAndDisallowedOrigin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		url    string
		origin string
		want   int
	}{
		{name: "query", url: "/api/v1/local/early-rentals/capabilities?account_id=someone-else", want: http.StatusBadRequest},
		{name: "origin", url: "/api/v1/local/early-rentals/capabilities", origin: "https://untrusted.example", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := &capabilitiesAuthStub{principal: identity.Principal{AccountID: "account-1"}}
			handler := NewCapabilitiesHandler(auth, CapabilityState{}, []string{"http://127.0.0.1:3000"})
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.Header.Set("Authorization", "Bearer token")
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status = %d body %s, want %d", response.Code, response.Body.String(), tc.want)
			}
		})
	}
}

func TestCapabilitiesOptionsDoesNotRequireSessionAndOtherMethodsAreRejected(t *testing.T) {
	auth := &capabilitiesAuthStub{principal: identity.Principal{AccountID: "account-1"}}
	handler := NewCapabilitiesHandler(auth, CapabilityState{}, []string{"http://127.0.0.1:3000"})
	options := httptest.NewRequest(http.MethodOptions, "/api/v1/local/early-rentals/capabilities", nil)
	options.Header.Set("Origin", "http://127.0.0.1:3000")
	preflight := httptest.NewRecorder()
	handler.ServeHTTP(preflight, options)
	if preflight.Code != http.StatusNoContent || auth.calls != 0 || preflight.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3000" {
		t.Fatalf("preflight = status %d headers %v auth calls %d", preflight.Code, preflight.Header(), auth.calls)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/local/early-rentals/capabilities", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, post)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, OPTIONS" {
		t.Fatalf("POST = status %d allow %q", response.Code, response.Header().Get("Allow"))
	}
}
