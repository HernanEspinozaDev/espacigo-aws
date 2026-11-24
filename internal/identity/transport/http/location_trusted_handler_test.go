package identityhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func setupLocationTrustedHandler(now time.Time) (http.Handler, *mockAuthRepo, string, string, string, string) {
	repo := newMockAuthRepo()

	// Account A (Owner)
	accA := identity.Account{
		ID:              "account-loc-test-a",
		Email:           "owner.a@espacigo.cl",
		NormalizedEmail: "owner.a@espacigo.cl",
		PasswordHash:    "hashed-pass-a",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[accA.ID] = accA

	tokenA := "token-bearer-owner-a"
	sessA := "session-loc-a"
	repo.sessions[sessA] = identity.Session{
		ID:             sessA,
		AccountID:      accA.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(tokenA))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Linux Desktop",
		IPMasked:       "190.162.•••.•••",
	}
	repo.tokens[tokenA] = sessA

	// Account B (Attacker / Other user)
	accB := identity.Account{
		ID:              "account-loc-test-b",
		Email:           "attacker.b@espacigo.cl",
		NormalizedEmail: "attacker.b@espacigo.cl",
		PasswordHash:    "hashed-pass-b",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[accB.ID] = accB

	tokenB := "token-bearer-attacker-b"
	sessB := "session-loc-b"
	repo.sessions[sessB] = identity.Session{
		ID:             sessB,
		AccountID:      accB.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(tokenB))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "iPhone",
		IPMasked:       "200.54.•••.•••",
	}
	repo.tokens[tokenB] = sessB

	svc, err := identity.NewAuthenticationService(
		repo,
		testHasher{},
		dummyMailer{},
		dummyLimiter{},
		&testCredGen{nextID: "00000000-0000-4000-8000-000000000099"},
		func() time.Time { return now },
	)
	if err != nil {
		panic(err)
	}

	origins := []string{"http://127.0.0.1:3000"}
	handler := NewHandler(svc, repo, origins)
	return handler, repo, accA.ID, tokenA, accB.ID, tokenB
}

func TestHTTP_LocationPermission_Endpoints(t *testing.T) {
	now := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	handler, _, _, tokenA, _, tokenB := setupLocationTrustedHandler(now)

	t.Run("GET /api/v1/account/security/location unauthorized without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/location", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Fatalf("missing X-Request-ID")
		}
	})

	t.Run("GET /api/v1/account/security/location returns default disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/location", nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}
		var resp identity.AccountLocationPermissionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Enabled {
			t.Fatalf("expected Enabled: false")
		}
	})

	t.Run("PUT /api/v1/account/security/location rejects GPS coordinates", func(t *testing.T) {
		body := `{"enabled":true,"region_label":"-33.4489, -70.6693"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/account/security/location", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "coordinates_not_allowed") {
			t.Fatalf("expected coordinates_not_allowed code, got: %s", rec.Body.String())
		}
	})

	t.Run("PUT /api/v1/account/security/location succeeds and updates region", func(t *testing.T) {
		body := `{"enabled":true,"region_label":"Región Metropolitana"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/account/security/location", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}
		var resp identity.AccountLocationPermissionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if !resp.Enabled || resp.LastRegionLabel == nil || *resp.LastRegionLabel != "Región Metropolitana" {
			t.Fatalf("expected enabled=true and Región Metropolitana, got %+v", resp)
		}
	})

	t.Run("Isolation: Account B does not see Account A's location", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/location", nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var resp identity.AccountLocationPermissionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Enabled {
			t.Fatalf("account B unexpectedly saw enabled location from account A")
		}
	})
}

func TestHTTP_TrustedContacts_Endpoints(t *testing.T) {
	now := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	handler, _, _, tokenA, _, tokenB := setupLocationTrustedHandler(now)

	var contactID1 string

	t.Run("POST /api/v1/account/security/trusted-contacts validates inputs", func(t *testing.T) {
		// Invalid email
		body := `{"full_name":"Carlos Soto","email":"not-an-email","relationship":"Hermano"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for bad email", rec.Code)
		}

		// Invalid phone
		body = `{"full_name":"Carlos Soto","email":"carlos@espacigo.cl","phone":"123","relationship":"Hermano"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for bad phone", rec.Code)
		}
	})

	t.Run("POST /api/v1/account/security/trusted-contacts creates contact successfully", func(t *testing.T) {
		body := `{"full_name":"María González","email":"maria@espacigo.cl","phone":"+56987654321","relationship":"Hermana"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}
		var created identity.TrustedContact
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if created.FullName != "María González" {
			t.Fatalf("full_name = %q, want María González", created.FullName)
		}
		if created.MaskedEmail != "m••••l" {
			t.Fatalf("masked_email = %q, want m••••l", created.MaskedEmail)
		}
		if created.MaskedPhone != "+56 ••••4321" {
			t.Fatalf("masked_phone = %q, want +56 ••••4321", created.MaskedPhone)
		}
		contactID1 = created.ID
	})

	t.Run("GET /api/v1/account/security/trusted-contacts returns list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/trusted-contacts", nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var list identity.TrustedContactsListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(list.Contacts) != 1 {
			t.Fatalf("expected 1 contact, got %d", len(list.Contacts))
		}
	})

	t.Run("Enforce maximum 3 active trusted contacts limit -> 409 Conflict", func(t *testing.T) {
		// Add 2nd contact
		body2 := `{"full_name":"Contacto Dos","email":"dos@espacigo.cl","relationship":"Amigo"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body2))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("add contact 2 failed: %d, body: %s", rec.Code, rec.Body.String())
		}

		// Add 3rd contact
		body3 := `{"full_name":"Contacto Tres","email":"tres@espacigo.cl","relationship":"Primo"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body3))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("add contact 3 failed: %d, body: %s", rec.Code, rec.Body.String())
		}

		// 4th contact must return 409 Conflict
		body4 := `{"full_name":"Contacto Cuatro","email":"cuatro@espacigo.cl","relationship":"Tío"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/account/security/trusted-contacts", strings.NewReader(body4))
		req.Header.Set("Authorization", "Bearer "+tokenA)
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 Conflict for 4th contact, body: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "trusted_contact_limit") {
			t.Fatalf("expected error code trusted_contact_limit, got %s", rec.Body.String())
		}
	})

	t.Run("Isolation: Account B cannot delete Account A's contact", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/trusted-contacts/"+contactID1, nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 for cross-account delete, body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /api/v1/account/security/trusted-contacts/{contact_id} revokes contact", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/trusted-contacts/"+contactID1, nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}
		var resp identity.RemoveTrustedContactResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Status != "revoked" || resp.ContactID != contactID1 {
			t.Fatalf("expected status revoked for %s, got %+v", contactID1, resp)
		}
	})

	t.Run("DELETE with invalid UUID returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/trusted-contacts/not-a-valid-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 for invalid UUID", rec.Code)
		}
	})
}

func TestHTTP_SecurityMethods_Endpoint(t *testing.T) {
	now := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	handler, _, _, tokenA, _, _ := setupLocationTrustedHandler(now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/methods", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}

	var resp identity.SecurityMethodsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.TotalCount != 6 {
		t.Fatalf("TotalCount = %d, want 6", resp.TotalCount)
	}
	if !resp.Methods.Password.Enabled {
		t.Fatalf("Password.Enabled = false, want true")
	}
	if resp.Methods.FacialKYC.Status != "pending_kyc_module" {
		t.Fatalf("FacialKYC.Status = %q, want pending_kyc_module", resp.Methods.FacialKYC.Status)
	}
}
