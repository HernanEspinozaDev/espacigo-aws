package identityhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type executionPrivacyRepo struct {
	memoryPublicPrivacyRepo
	ownData      privacy.OwnData
	prefs        map[string]privacy.PurposePreference
	restrictions []privacy.TreatmentRestriction
}

func (m *executionPrivacyRepo) ExportOwnData(_ context.Context, _ string) (privacy.OwnData, error) {
	return m.ownData, nil
}

func (m *executionPrivacyRepo) ListOwnPurposePreferences(_ context.Context, _ string) ([]privacy.PurposePreference, error) {
	out := make([]privacy.PurposePreference, 0, len(m.prefs))
	for _, pref := range m.prefs {
		out = append(out, pref)
	}
	return out, nil
}

func (m *executionPrivacyRepo) UpsertOwnPurposePreference(_ context.Context, _ string, in privacy.UpdatePurposePreferenceInput, now time.Time) (privacy.PurposePreference, error) {
	if m.prefs == nil {
		m.prefs = make(map[string]privacy.PurposePreference)
	}
	var reason *string
	if in.Reason != "" {
		r := in.Reason
		reason = &r
	}
	ts := now.UTC()
	pref := privacy.PurposePreference{
		Purpose:   in.Purpose,
		State:     in.State,
		Reason:    reason,
		UpdatedAt: &ts,
	}
	m.prefs[in.Purpose] = pref
	return pref, nil
}

func (m *executionPrivacyRepo) ListOwnTreatmentRestrictions(_ context.Context, _ string) ([]privacy.TreatmentRestriction, error) {
	return m.restrictions, nil
}

func (m *executionPrivacyRepo) HasActiveTreatmentBlock(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}

func setupPrivacyExecutionHandler(t *testing.T, withPrivacy bool) (http.Handler, string) {
	t.Helper()
	now := time.Date(2026, time.November, 27, 18, 0, 0, 0, time.UTC)

	authRepo := newMockAuthRepo()
	acc := identity.Account{
		ID:              "account-priv-exec-1",
		Email:           "titular@espacigo.cl",
		NormalizedEmail: "titular@espacigo.cl",
		PasswordHash:    "hashed-pass",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	authRepo.accounts[acc.ID] = acc

	token := "token-priv-exec-1"
	sessID := "session-priv-exec-1"
	authRepo.sessions[sessID] = identity.Session{
		ID:             sessID,
		AccountID:      acc.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Linux Desktop",
		IPMasked:       "190.162.•••.•••",
	}
	authRepo.tokens[token] = sessID

	authSvc, err := identity.NewAuthenticationService(
		authRepo,
		testHasher{},
		dummyMailer{},
		dummyLimiter{},
		&testCredGen{nextID: "00000000-0000-4000-8000-000000000099"},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewAuthenticationService: %v", err)
	}

	var privacySvc *privacy.Service
	if withPrivacy {
		privRepo := &executionPrivacyRepo{
			ownData: privacy.OwnData{
				Account: privacy.ExportAccount{
					Email:     acc.Email,
					State:     "activo",
					CreatedAt: now.Add(-24 * time.Hour),
				},
				Roles:       []string{"arrendatario"},
				Acceptances: []privacy.TermsAcceptance{},
				Requests:    []privacy.RightsRequest{},
				Scope:       "identidad_local_v1",
			},
			prefs: map[string]privacy.PurposePreference{
				privacy.PurposeProductAnalytics: {
					Purpose: privacy.PurposeProductAnalytics,
					State:   privacy.PurposeStateWithdrawn,
				},
			},
			restrictions: []privacy.TreatmentRestriction{
				{
					ID:        "00000000-0000-4000-8000-000000000801",
					RequestID: "00000000-0000-4000-8000-000000000701",
					Scope:     privacy.ScopeCommercialComms,
					State:     "activa",
					Reason:    "Bloqueo temporal mientras se revisa solicitud.",
					AppliedAt: now.Add(-time.Hour),
				},
			},
		}
		privacySvc, err = privacy.NewService(privRepo)
		if err != nil {
			t.Fatalf("privacy.NewService: %v", err)
		}
	}

	return NewHandler(authSvc, authRepo, []string{"http://localhost:3000"}, privacySvc), token
}

func doAuthenticatedPrivacyRequest(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPrivacyExecutionEndpointsAuthAndMethods(t *testing.T) {
	h, _ := setupPrivacyExecutionHandler(t, true)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/privacy/preferences"},
		{http.MethodPut, "/api/v1/privacy/preferences"},
		{http.MethodGet, "/api/v1/privacy/restrictions"},
		{http.MethodGet, "/api/v1/privacy/portability"},
	} {
		body := ""
		if tc.method == http.MethodPut {
			body = `{"purpose":"comunicaciones_comerciales","state":"opuesto"}`
		}
		res := doAuthenticatedPrivacyRequest(h, tc.method, tc.path, "", body)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s status = %d, want 401 (body=%s)", tc.method, tc.path, res.Code, res.Body.String())
		}
		if res.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("anonymous %s %s missing Bearer challenge", tc.method, tc.path)
		}
		if res.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("anonymous %s %s missing Cache-Control: no-store", tc.method, tc.path)
		}
	}

	for _, tc := range []struct {
		path      string
		wantAllow string
	}{
		{"/api/v1/privacy/preferences", "GET, PUT"},
		{"/api/v1/privacy/restrictions", "GET"},
		{"/api/v1/privacy/portability", "GET"},
	} {
		res := doAuthenticatedPrivacyRequest(h, http.MethodPost, tc.path, "", `{}`)
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d, want 405", tc.path, res.Code)
		}
		if got := res.Header().Get("Allow"); got != tc.wantAllow {
			t.Fatalf("POST %s Allow = %q, want %q", tc.path, got, tc.wantAllow)
		}
	}
}

func TestPrivacyExecutionPreferencesRestrictionsAndPortability(t *testing.T) {
	h, token := setupPrivacyExecutionHandler(t, true)

	// GET /api/v1/privacy/preferences
	prefsRes := doAuthenticatedPrivacyRequest(h, http.MethodGet, "/api/v1/privacy/preferences", token, "")
	if prefsRes.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/privacy/preferences status = %d, body = %s", prefsRes.Code, prefsRes.Body.String())
	}
	if prefsRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET preferences Cache-Control = %q", prefsRes.Header().Get("Cache-Control"))
	}
	var prefsPayload struct {
		Items []privacy.PurposePreference `json:"items"`
	}
	if err := json.Unmarshal(prefsRes.Body.Bytes(), &prefsPayload); err != nil {
		t.Fatalf("decode preferences: %v", err)
	}
	if len(prefsPayload.Items) != 4 {
		t.Fatalf("preferences items len = %d, want 4", len(prefsPayload.Items))
	}

	// PUT /api/v1/privacy/preferences -> 200 OK
	putOK := doAuthenticatedPrivacyRequest(h, http.MethodPut, "/api/v1/privacy/preferences", token, `{
		"purpose": "comunicaciones_comerciales",
		"state": "opuesto",
		"reason": "Prefiero no recibir comunicaciones promocionales"
	}`)
	if putOK.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/privacy/preferences status = %d, body = %s", putOK.Code, putOK.Body.String())
	}
	if putOK.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("PUT preferences Cache-Control = %q", putOK.Header().Get("Cache-Control"))
	}
	var putPayload struct {
		Data privacy.PurposePreference `json:"data"`
	}
	if err := json.Unmarshal(putOK.Body.Bytes(), &putPayload); err != nil {
		t.Fatalf("decode put preference: %v", err)
	}
	if putPayload.Data.Purpose != privacy.PurposeCommercialComms || putPayload.Data.State != privacy.PurposeStateOpposed {
		t.Fatalf("unexpected updated preference: %+v", putPayload.Data)
	}

	// PUT /api/v1/privacy/preferences -> 422 on invalid input
	for _, badBody := range []string{
		`{"purpose":"desconocido","state":"opuesto"}`,
		`{"purpose":"comunicaciones_comerciales","state":"bloqueado"}`,
		`{"purpose":"comunicaciones_comerciales","state":"opuesto","reason":"abc"}`,
	} {
		badRes := doAuthenticatedPrivacyRequest(h, http.MethodPut, "/api/v1/privacy/preferences", token, badBody)
		if badRes.Code != http.StatusUnprocessableEntity {
			t.Fatalf("PUT preferences bad body %s status = %d, want 422 (body=%s)", badBody, badRes.Code, badRes.Body.String())
		}
	}

	// GET /api/v1/privacy/restrictions -> 200 OK
	restRes := doAuthenticatedPrivacyRequest(h, http.MethodGet, "/api/v1/privacy/restrictions", token, "")
	if restRes.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/privacy/restrictions status = %d, body = %s", restRes.Code, restRes.Body.String())
	}
	if restRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET restrictions Cache-Control = %q", restRes.Header().Get("Cache-Control"))
	}
	var restPayload struct {
		Items []privacy.TreatmentRestriction `json:"items"`
	}
	if err := json.Unmarshal(restRes.Body.Bytes(), &restPayload); err != nil {
		t.Fatalf("decode restrictions: %v", err)
	}
	if len(restPayload.Items) != 1 || restPayload.Items[0].Scope != privacy.ScopeCommercialComms {
		t.Fatalf("unexpected restrictions payload: %+v", restPayload.Items)
	}

	// GET /api/v1/privacy/portability -> 200 OK with headers and digest
	portRes := doAuthenticatedPrivacyRequest(h, http.MethodGet, "/api/v1/privacy/portability", token, "")
	if portRes.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/privacy/portability status = %d, body = %s", portRes.Code, portRes.Body.String())
	}
	if got := portRes.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("portability Content-Type = %q", got)
	}
	if got := portRes.Header().Get("Content-Disposition"); got != `attachment; filename="espacigo-portabilidad-v1.json"` {
		t.Fatalf("portability Content-Disposition = %q", got)
	}
	if got := portRes.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("portability Cache-Control = %q", got)
	}
	digest := portRes.Header().Get("X-Content-SHA256")
	if len(digest) != 64 {
		t.Fatalf("portability X-Content-SHA256 len = %d (%q), want 64 hex", len(digest), digest)
	}
	sum := sha256.Sum256(portRes.Body.Bytes())
	if hex.EncodeToString(sum[:]) != digest {
		t.Fatalf("portability X-Content-SHA256 = %q, want %q", digest, hex.EncodeToString(sum[:]))
	}
	var portDoc map[string]any
	if err := json.Unmarshal(portRes.Body.Bytes(), &portDoc); err != nil {
		t.Fatalf("portability body is not valid JSON: %v", err)
	}
	if portDoc["schema_version"] != privacy.PortabilitySchemaVersion {
		t.Fatalf("portability schema_version = %v, want %s", portDoc["schema_version"], privacy.PortabilitySchemaVersion)
	}
}

func TestPrivacyExecutionUnavailableReturns503(t *testing.T) {
	h, token := setupPrivacyExecutionHandler(t, false)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/privacy/preferences", ""},
		{http.MethodPut, "/api/v1/privacy/preferences", `{"purpose":"comunicaciones_comerciales","state":"opuesto"}`},
		{http.MethodGet, "/api/v1/privacy/restrictions", ""},
		{http.MethodGet, "/api/v1/privacy/portability", ""},
	} {
		res := doAuthenticatedPrivacyRequest(h, tc.method, tc.path, token, tc.body)
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s with nil privacy status = %d, want 503 (body=%s)", tc.method, tc.path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), `"privacy_unavailable"`) {
			t.Fatalf("%s %s expected privacy_unavailable, got %s", tc.method, tc.path, res.Body.String())
		}
	}
}
