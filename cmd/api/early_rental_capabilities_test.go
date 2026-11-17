package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type earlyRentalCapabilitiesAuthStub struct{}

func (earlyRentalCapabilitiesAuthStub) Authorize(_ context.Context, secret identity.Secret, _ identity.Role, activity identity.ActivityKind) (identity.Principal, error) {
	if secret != "session-token" || activity != identity.UserOperation {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	return identity.Principal{AccountID: "account-1"}, nil
}

func TestEarlyRentalCapabilitiesFlagIsLocalOnlyAndDisabledByDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
		bad  bool
	}{
		{name: "unset"},
		{name: "zero", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "0"}},
		{name: "local requested", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "1", "LOCAL_AUTH_PROTOTYPE": "1"}, want: true},
		{name: "cloud profile denied", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "1", "APP_PROFILE": "cloud-test"}, bad: true},
		{name: "auth missing", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "1"}, bad: true},
		{name: "dedicated runtime denied", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "1", "LOCAL_AUTH_PROTOTYPE": "1", "API_RUNTIME_PROFILE": "local-kyc-worker-free"}, bad: true},
		{name: "invalid flag", env: map[string]string{"LOCAL_EARLY_RENTAL_FLOW_ENABLED": "true"}, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			got, err := localEarlyRentalActivationRequested(getenv)
			if tc.bad {
				if err == nil {
					t.Fatal("expected fail-closed flag validation")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("activation = %v, err %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestEarlyRentalCapabilitiesRouteIsRestrictedToStandardLocalRuntime(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "local standard", env: map[string]string{"LOCAL_AUTH_PROTOTYPE": "1"}, want: true},
		{name: "cloud profile", env: map[string]string{"LOCAL_AUTH_PROTOTYPE": "1", "APP_PROFILE": "cloud-test"}},
		{name: "dedicated runtime", env: map[string]string{"LOCAL_AUTH_PROTOTYPE": "1", "API_RUNTIME_PROFILE": "local-kyc-worker-free"}},
		{name: "auth disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := localEarlyRentalRuntime(func(key string) string { return tc.env[key] })
			if got != tc.want {
				t.Fatalf("local runtime = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEarlyRentalCapabilitiesRouteIsMountedAndNeverAdvertisesUnreadyFlow(t *testing.T) {
	mux := http.NewServeMux()
	if err := registerLocalEarlyRentalCapabilitiesRoute(mux, earlyRentalCapabilitiesAuthStub{}, false, nil); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("disabled route = status %d body %s", response.Code, response.Body.String())
	}

	requested := http.NewServeMux()
	if err := registerLocalEarlyRentalCapabilitiesRoute(requested, earlyRentalCapabilitiesAuthStub{}, true, nil); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/local/early-rentals/capabilities", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	response = httptest.NewRecorder()
	requested.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("requested but unready route = status %d body %s", response.Code, response.Body.String())
	}
}
