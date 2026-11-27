package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

type policyTestStore struct {
	accepted bool
	writes   int
}

func (s *policyTestStore) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	if strings.Contains(query, "INSERT INTO public.aceptacion_terminos") {
		s.writes++
		s.accepted = true
		return policyTestRow{true}
	}
	return policyTestRow{s.accepted, true}
}

type policyTestRow []bool

func (row policyTestRow) Scan(dest ...any) error {
	for i, value := range row {
		*dest[i].(*bool) = value
	}
	return nil
}

type policyTestIdentity struct{}

func (policyTestIdentity) Authorize(_ context.Context, secret identity.Secret, _ identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if secret != "test-session" {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	return identity.Principal{AccountID: "10000000-0000-4000-8000-000000000001"}, nil
}

func TestHostPolicyOnlyGatesCommercialSpaceWrites(t *testing.T) {
	store := &policyTestStore{}
	document := identity.ConsentDocument{ID: "10000000-0000-4000-8000-000000000002"}
	policy := hostPolicy{db: store, auth: policyTestIdentity{}, document: document}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/kyb/policy-consent", policy)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	handler := policy.requireForSpaceWrites(mux)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer test-session")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		return recorder
	}
	if result := request(http.MethodPut, "/api/v1/spaces/a/publication", ""); result.Code != http.StatusForbidden {
		t.Fatalf("commercial publication before consent: %d", result.Code)
	}
	for _, path := range []string{"/api/v1/rental-quotes", "/api/v1/kyb/spaces/a/verification", "/api/v1/spaces/a/reviews"} {
		if result := request(http.MethodPost, path, ""); result.Code != http.StatusNoContent {
			t.Errorf("tenant or regularization route blocked: %s (%d)", path, result.Code)
		}
	}
	if result := request(http.MethodPost, "/api/v1/kyb/policy-consent", `{"version_id":"old"}`); result.Code != http.StatusConflict || store.writes != 0 {
		t.Fatalf("stale terms accepted: %d, writes %d", result.Code, store.writes)
	}
	if result := request(http.MethodPost, "/api/v1/kyb/policy-consent", `{"version_id":"`+document.ID+`"}`); result.Code != http.StatusOK {
		t.Fatalf("failed to accept terms: %d", result.Code)
	}
	if result := request(http.MethodPut, "/api/v1/spaces/a/publication", ""); result.Code != http.StatusNoContent {
		t.Fatalf("commercial publication after consent: %d", result.Code)
	}
}
