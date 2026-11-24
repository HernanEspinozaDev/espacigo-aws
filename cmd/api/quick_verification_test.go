package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
)

type mockAuthorizer struct {
	principal identity.Principal
	err       error
}

func (m mockAuthorizer) Authorize(_ context.Context, token identity.Secret, _ identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if m.err != nil {
		return identity.Principal{}, m.err
	}
	if token == "" {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	return m.principal, nil
}

type mockReviewer struct {
	reviewCalls int
	reviewErr   error
	kind        string
}

func (m *mockReviewer) Review(_ context.Context, id, reviewer string, approved bool, _ string, _ func() time.Time) (verification.Case, error) {
	m.reviewCalls++
	if m.reviewErr != nil {
		return verification.Case{}, m.reviewErr
	}
	state := "rechazada"
	if approved {
		state = "aprobada"
	}
	kind := m.kind
	if kind == "" {
		kind = "kyc"
	}
	return verification.Case{
		ID:        id,
		OwnerID:   reviewer,
		Type:      kind,
		State:     state,
		CreatedAt: time.Now().UTC(),
	}, nil
}

type mockVerificationRepo struct {
	cases map[string]verification.Case
}

func (m *mockVerificationRepo) Create(_ context.Context, c verification.Case) (verification.Case, error) {
	if m.cases == nil {
		m.cases = make(map[string]verification.Case)
	}
	m.cases[c.ID] = c
	return c, nil
}

func (m *mockVerificationRepo) GetOwn(_ context.Context, _, id string) (verification.Case, error) {
	if c, ok := m.cases[id]; ok {
		return c, nil
	}
	return verification.Case{}, verification.ErrNotFound
}

func (m *mockVerificationRepo) ListOwn(_ context.Context, owner string) ([]verification.Case, error) {
	var list []verification.Case
	for _, c := range m.cases {
		if c.OwnerID == owner {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *mockVerificationRepo) ListPending(_ context.Context) ([]verification.Case, error) {
	return nil, nil
}

func (m *mockVerificationRepo) ListRejected(_ context.Context) ([]verification.Case, error) {
	return nil, nil
}

func (m *mockVerificationRepo) ListEligibility(_ context.Context, _ string) ([]verification.Eligibility, error) {
	return nil, nil
}

func (m *mockVerificationRepo) ListHistory(_ context.Context, _, _ string) ([]verification.HistoryEntry, error) {
	return nil, nil
}

func (m *mockVerificationRepo) Review(_ context.Context, _, _ string, _ bool, _ string, _ func() time.Time) (verification.Case, error) {
	return verification.Case{}, nil
}

func (m *mockVerificationRepo) Revoke(_ context.Context, _, _, _, _, _ string, _ func() time.Time) (verification.Case, error) {
	return verification.Case{}, nil
}

func (m *mockVerificationRepo) Retry(_ context.Context, _, _ string, c verification.Case, _ func() time.Time) (verification.Case, error) {
	return c, nil
}

type testGen struct{ count int }

func (g *testGen) ID() (string, error) {
	g.count++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", g.count), nil
}

func TestHandleQuickVerificationAuthRejection(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	rec := httptest.NewRecorder()

	handleQuickVerification(rec, req, mockAuthorizer{}, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for missing token, got %d", rec.Code)
	}

	reqBadHeader := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	reqBadHeader.Header.Set("Authorization", "Basic abc123")
	recBadHeader := httptest.NewRecorder()

	handleQuickVerification(recBadHeader, reqBadHeader, mockAuthorizer{}, nil, nil)
	if recBadHeader.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for non-Bearer auth, got %d", recBadHeader.Code)
	}

	reqForbidden := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	reqForbidden.Header.Set("Authorization", "Bearer valid-token")
	recForbidden := httptest.NewRecorder()

	handleQuickVerification(recForbidden, reqForbidden, mockAuthorizer{err: identity.ErrForbidden}, nil, nil)
	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for forbidden principal, got %d", recForbidden.Code)
	}
}

func TestHandleQuickVerificationInvalidJSON(t *testing.T) {
	auth := mockAuthorizer{
		principal: identity.Principal{
			AccountID: "11111111-1111-4111-8111-111111111111",
			Roles:     []identity.Role{"arrendatario"},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handleQuickVerification(rec, req, auth, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid json, got %d", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errObj, ok := out["error"].(map[string]any)
	if !ok || errObj["code"] != "invalid_json" {
		t.Fatalf("expected code=invalid_json, got %v", out)
	}
}

func TestHandleQuickVerificationSuccessAutoApproval(t *testing.T) {
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	auth := mockAuthorizer{
		principal: identity.Principal{
			AccountID: accountID,
			Roles:     []identity.Role{"arrendatario"},
		},
	}
	verRepo := &mockVerificationRepo{}
	verService, err := verification.NewService(verRepo, &testGen{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		t.Fatalf("verification service: %v", err)
	}
	reviewer := &mockReviewer{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	req.Header.Set("Authorization", "Bearer session-token-valid-abc")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-0001")
	rec := httptest.NewRecorder()

	handleQuickVerificationWithLocalKYC(rec, req, auth, verService, reviewer, true)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if reviewer.reviewCalls != 1 {
		t.Fatalf("expected 1 call to reviewer.Review, got %d", reviewer.reviewCalls)
	}

	var item verification.Case
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	if item.State != "aprobada" {
		t.Fatalf("expected item.State to be auto-approved as 'aprobada', got %q", item.State)
	}
	if item.Type != "kyc" {
		t.Fatalf("expected item.Type to be 'kyc', got %q", item.Type)
	}
	if item.ID == "" {
		t.Fatal("expected item.ID to be non-empty")
	}
}

func TestHandleQuickVerificationReviewFailureGraceful(t *testing.T) {
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	auth := mockAuthorizer{
		principal: identity.Principal{
			AccountID: accountID,
			Roles:     []identity.Role{"arrendatario"},
		},
	}
	verRepo := &mockVerificationRepo{}
	verService, err := verification.NewService(verRepo, &testGen{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		t.Fatalf("verification service: %v", err)
	}
	reviewer := &mockReviewer{reviewErr: errors.New("db review locked")}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	req.Header.Set("Authorization", "Bearer session-token-valid-abc")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-0002")
	rec := httptest.NewRecorder()

	handleQuickVerificationWithLocalKYC(rec, req, auth, verService, reviewer, true)

	// Even if reviewer returns an error, the created case in "en_revision" is returned with 201
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}
	var item verification.Case
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	if item.State != "en_revision" {
		t.Fatalf("expected state 'en_revision' when review fails, got %q", item.State)
	}
}

func TestHandleQuickVerificationBlocksKYCOutsideLocalFixtureGate(t *testing.T) {
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	auth := mockAuthorizer{principal: identity.Principal{AccountID: accountID}}
	repo := &mockVerificationRepo{}
	service, err := verification.NewService(repo, &testGen{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := &mockReviewer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyc"}`)))
	req.Header.Set("Authorization", "Bearer session-token-valid-abc")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-0003")
	rec := httptest.NewRecorder()

	handleQuickVerification(rec, req, auth, service, reviewer)
	if rec.Code != http.StatusUnprocessableEntity || reviewer.reviewCalls != 0 || len(repo.cases) != 0 {
		t.Fatalf("KYC outside local fixture gate: status=%d review_calls=%d cases=%d body=%s", rec.Code, reviewer.reviewCalls, len(repo.cases), rec.Body.String())
	}
}

func TestHandleQuickVerificationKeepsKYBFlowUnchangedWithoutLocalKYCGate(t *testing.T) {
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	auth := mockAuthorizer{principal: identity.Principal{AccountID: accountID}}
	repo := &mockVerificationRepo{}
	service, err := verification.NewService(repo, &testGen{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := &mockReviewer{kind: "kyb"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewReader([]byte(`{"type":"kyb"}`)))
	req.Header.Set("Authorization", "Bearer session-token-valid-abc")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-0004")
	rec := httptest.NewRecorder()

	handleQuickVerification(rec, req, auth, service, reviewer)
	if rec.Code != http.StatusCreated || reviewer.reviewCalls != 1 {
		t.Fatalf("KYB flow changed: status=%d review_calls=%d body=%s", rec.Code, reviewer.reviewCalls, rec.Body.String())
	}
	var item verification.Case
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if item.Type != "kyb" || item.State != "aprobada" {
		t.Fatalf("KYB case=%+v", item)
	}
}

func TestLocalSyntheticKYCRequiresExactLocalRuntime(t *testing.T) {
	values := map[string]string{
		"ENVIRONMENT": "development", "LOCAL_AUTH_PROTOTYPE": "1",
		"DATABASE_CONNECTION_MODE": "direct",
	}
	getenv := func(key string) string { return values[key] }
	base := config{
		httpAddr: "127.0.0.1:8080", dbHost: "172.24.0.3", dbPort: 5432,
		dbName: "espacigo_aws_local", dbUser: "espacigo_api_login", dbSetRole: "espacigo_runtime",
	}
	if !localSyntheticKYCEnabled(getenv, base) {
		t.Fatal("documented local-dev target should permit synthetic KYC")
	}
	tests := []struct {
		name   string
		mutate func()
	}{
		{"production config", func() { base.production = true }},
		{"production environment", func() { values["ENVIRONMENT"] = "production" }},
		{"app profile", func() { values["APP_PROFILE"] = "production" }},
		{"local auth disabled", func() { values["LOCAL_AUTH_PROTOTYPE"] = "" }},
		{"non-direct database", func() { values["DATABASE_CONNECTION_MODE"] = "proxy" }},
		{"wrong database", func() { base.dbName = "other" }},
		{"wrong database role", func() { base.dbUser = "postgres" }},
		{"wrong set role", func() { base.dbSetRole = "postgres" }},
		{"non-loopback API", func() { base.httpAddr = "0.0.0.0:8080" }},
		{"non-local database", func() { base.dbHost = "db.example.invalid" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := base
			originalValues := map[string]string{}
			for key, value := range values {
				originalValues[key] = value
			}
			tt.mutate()
			if localSyntheticKYCEnabled(getenv, base) {
				t.Fatal("non-local configuration enabled synthetic KYC")
			}
			base = original
			values = originalValues
		})
	}
}
