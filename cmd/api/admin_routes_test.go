package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	adminglobalhttp "github.com/HernanEspinozaDev/espaciGo/internal/adminglobal/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestRegisterProductionAdminAccountRoutesIncludesAuthorizedReads(t *testing.T) {
	called := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	mux := http.NewServeMux()
	registerProductionAdminAccountRoutes(mux, handler)

	for _, path := range []string{
		"/api/v1/admin/global/kpis",
		"/api/v1/admin/global/audit-events",
		"/api/v1/admin/global/audit-events/export",
		"/api/v1/admin/global/accounts/account-1/resources/contracts",
		"/api/v1/admin/global/accounts/account-1/resources/contracts/contract-1",
		"/api/v1/admin/global/publications",
	} {
		t.Run(path, func(t *testing.T) {
			called = ""
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusNoContent || called != "GET "+path {
				t.Fatalf("GET route not registered: status=%d dispatched=%q", response.Code, called)
			}
		})
	}

	for _, path := range []string{
		"/api/v1/admin/global/kpis",
		"/api/v1/admin/global/audit-events",
		"/api/v1/admin/global/audit-events/export",
		"/api/v1/admin/global/accounts/account-1/resources/contracts",
		"/api/v1/admin/global/accounts/account-1/resources/contracts/contract-1",
		"/api/v1/admin/global/publications",
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s unexpectedly registered: status=%d", path, response.Code)
		}
	}
}

func TestRegisterProductionPrivacyDispatchReceiptAllowsOnlyPost(t *testing.T) {
	mux := http.NewServeMux()
	const path = "/api/v1/admin/global/privacy-requests/00000000-0000-4000-8000-000000000701/dispatch-receipt"
	registerProductionAdminAccountRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("requestID"); got != "00000000-0000-4000-8000-000000000701" {
			t.Errorf("requestID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodPost, http.StatusNoContent},
		{http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodDelete, http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, path, nil))
		if response.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, path, response.Code, tc.status)
		}
	}
}

func TestRegisterProductionPrivacyDetailAndWorkflowRoutes(t *testing.T) {
	mux := http.NewServeMux()
	const id = "00000000-0000-4000-8000-000000000701"
	const detailPath = "/api/v1/admin/global/privacy-requests/" + id
	const workflowPath = detailPath + "/workflow"
	registerProductionAdminAccountRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("requestID"); got != id {
			t.Errorf("requestID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, detailPath, http.StatusNoContent},
		{http.MethodPost, detailPath, http.StatusMethodNotAllowed},
		{http.MethodPost, workflowPath, http.StatusNoContent},
		{http.MethodGet, workflowPath, http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, response.Code, tc.status)
		}
	}
}

type unauthorizedAdminCapabilityAuth struct{}

func (unauthorizedAdminCapabilityAuth) AuthorizeCapability(context.Context, identity.Secret, identity.Capability, identity.ActivityKind) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthorized
}

func TestRegisterProductionPrivacyNotificationsRetryRouteAndAuth(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000701"
	const path = "/api/v1/admin/global/privacy-requests/" + id + "/notifications/retry"

	mux := http.NewServeMux()
	registerProductionAdminAccountRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("requestID"); got != id {
			t.Errorf("requestID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodPost, http.StatusNoContent},
		{http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodDelete, http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, path, nil))
		if response.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, path, response.Code, tc.status)
		}
	}

	authMux := http.NewServeMux()
	registerProductionAdminAccountRoutes(authMux, adminglobalhttp.NewHandler(unauthorizedAdminCapabilityAuth{}, &adminglobal.Service{}, nil))
	anonResp := httptest.NewRecorder()
	authMux.ServeHTTP(anonResp, httptest.NewRequest(http.MethodPost, path, nil))
	if anonResp.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous POST %s = %d, want %d (body=%s)", path, anonResp.Code, http.StatusUnauthorized, anonResp.Body.String())
	}
}

func TestRegisterProductionPrivacyExecutionsRouteAndAuth(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000701"
	const path = "/api/v1/admin/global/privacy-requests/" + id + "/executions"

	mux := http.NewServeMux()
	registerProductionAdminAccountRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("requestID"); got != id {
			t.Errorf("requestID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodPost, http.StatusNoContent},
		{http.MethodGet, http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, path, nil))
		if response.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, path, response.Code, tc.status)
		}
	}

	authMux := http.NewServeMux()
	registerProductionAdminAccountRoutes(authMux, adminglobalhttp.NewHandler(unauthorizedAdminCapabilityAuth{}, &adminglobal.Service{}, nil))
	anonResp := httptest.NewRecorder()
	authMux.ServeHTTP(anonResp, httptest.NewRequest(http.MethodPost, path, nil))
	if anonResp.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous POST %s = %d, want %d (body=%s)", path, anonResp.Code, http.StatusUnauthorized, anonResp.Body.String())
	}
}

func TestRegisterProductionPrivacyGovernanceRoutesAndAuth(t *testing.T) {
	const incidentID = "00000000-0000-4000-8000-000000000711"
	const holdID = "00000000-0000-4000-8000-000000000712"

	const incidentsPath = "/api/v1/admin/global/privacy-incidents"
	const incidentDetailPath = incidentsPath + "/" + incidentID
	const incidentTransitionsPath = incidentDetailPath + "/transitions"
	const legalHoldsPath = "/api/v1/admin/global/privacy-legal-holds"
	const legalHoldReleasePath = legalHoldsPath + "/" + holdID + "/release"

	mux := http.NewServeMux()
	called := ""
	registerProductionAdminAccountRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = r.Method + " " + r.URL.Path
		if r.URL.Path == incidentDetailPath || r.URL.Path == incidentTransitionsPath {
			if got := r.PathValue("incidentID"); got != incidentID {
				t.Errorf("incidentID = %q, want %q", got, incidentID)
			}
		}
		if r.URL.Path == legalHoldReleasePath {
			if got := r.PathValue("holdID"); got != holdID {
				t.Errorf("holdID = %q, want %q", got, holdID)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := []struct {
		method string
		path   string
	}{
		{http.MethodGet, incidentsPath},
		{http.MethodPost, incidentsPath},
		{http.MethodGet, incidentDetailPath},
		{http.MethodPost, incidentTransitionsPath},
		{http.MethodGet, legalHoldsPath},
		{http.MethodPost, legalHoldsPath},
		{http.MethodPost, legalHoldReleasePath},
	}

	for _, tc := range allowed {
		called = ""
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != http.StatusNoContent || called != tc.method+" "+tc.path {
			t.Errorf("%s %s = status %d (called=%q), want %d", tc.method, tc.path, response.Code, called, http.StatusNoContent)
		}
	}

	disallowed := []struct {
		method string
		path   string
	}{
		{http.MethodDelete, incidentsPath},
		{http.MethodPut, incidentsPath},
		{http.MethodPost, incidentDetailPath},
		{http.MethodDelete, incidentDetailPath},
		{http.MethodGet, incidentTransitionsPath},
		{http.MethodDelete, incidentTransitionsPath},
		{http.MethodDelete, legalHoldsPath},
		{http.MethodPatch, legalHoldsPath},
		{http.MethodGet, legalHoldReleasePath},
		{http.MethodDelete, legalHoldReleasePath},
	}

	for _, tc := range disallowed {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, response.Code, http.StatusMethodNotAllowed)
		}
	}

	authMux := http.NewServeMux()
	registerProductionAdminAccountRoutes(authMux, adminglobalhttp.NewHandler(unauthorizedAdminCapabilityAuth{}, &adminglobal.Service{}, nil))
	for _, tc := range allowed {
		anonResp := httptest.NewRecorder()
		authMux.ServeHTTP(anonResp, httptest.NewRequest(tc.method, tc.path, nil))
		if anonResp.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s = %d, want %d (body=%s)", tc.method, tc.path, anonResp.Code, http.StatusUnauthorized, anonResp.Body.String())
		}
	}
}
