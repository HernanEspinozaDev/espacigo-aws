package identityhttp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type passkeyTestCredGen struct{ seq int }

func (g *passkeyTestCredGen) ID() (string, error) {
	g.seq++
	return fmt.Sprintf("test-uuid-%d", g.seq), nil
}
func (g *passkeyTestCredGen) Token() (identity.Secret, error) {
	return identity.Secret("test-token"), nil
}

type passkeyUserSession struct {
	AccountID string
	Token     string
	SessionID string
}

func setupPasskeyTestHandler(now time.Time) (http.Handler, *mockAuthRepo, passkeyUserSession, passkeyUserSession) {
	repo := newMockAuthRepo()

	// User A
	accA := identity.Account{
		ID:              "account-passkey-a",
		Email:           "alice@espacigo.cl",
		NormalizedEmail: "alice@espacigo.cl",
		PasswordHash:    "hashed-pass-a",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[accA.ID] = accA
	tokenA := "bearer-token-alice"
	sessionA := "session-alice"
	repo.sessions[sessionA] = identity.Session{
		ID:             sessionA,
		AccountID:      accA.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(tokenA))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Chrome en macOS",
		IPMasked:       "190.162.•••.•••",
	}
	repo.tokens[tokenA] = sessionA

	// User B
	accB := identity.Account{
		ID:              "account-passkey-b",
		Email:           "bob@espacigo.cl",
		NormalizedEmail: "bob@espacigo.cl",
		PasswordHash:    "hashed-pass-b",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[accB.ID] = accB
	tokenB := "bearer-token-bob"
	sessionB := "session-bob"
	repo.sessions[sessionB] = identity.Session{
		ID:             sessionB,
		AccountID:      accB.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(tokenB))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Safari en iOS",
		IPMasked:       "200.54.•••.•••",
	}
	repo.tokens[tokenB] = sessionB

	credGen := &passkeyTestCredGen{}
	service, err := identity.NewAuthenticationService(repo, testHasher{}, dummyMailer{}, dummyLimiter{}, credGen, func() time.Time { return now })
	if err != nil {
		panic(err)
	}

	origins := []string{"http://localhost:3000", "https://espacigo.com"}
	handler := NewHandler(service, repo, origins)

	userA := passkeyUserSession{AccountID: accA.ID, Token: tokenA, SessionID: sessionA}
	userB := passkeyUserSession{AccountID: accB.ID, Token: tokenB, SessionID: sessionB}
	return handler, repo, userA, userB
}

func TestPasskey_HTTP_Flow_And_Isolation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	handler, _, userA, userB := setupPasskeyTestHandler(now)

	// 1. Unauthenticated requests return 401 with Cache-Control: no-store
	{
		endpoints := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/v1/account/security/passkeys"},
			{http.MethodPost, "/api/v1/account/security/passkeys/register/options"},
			{http.MethodPost, "/api/v1/account/security/passkeys/register/verify"},
			{http.MethodDelete, "/api/v1/account/security/passkeys/pk-dummy"},
		}
		for _, ep := range endpoints {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: expected 401, got %d (body: %s)", ep.method, ep.path, rec.Code, rec.Body.String())
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("%s %s: expected Cache-Control: no-store, got %s", ep.method, ep.path, cc)
			}
			if reqID := rec.Header().Get("X-Request-ID"); reqID == "" {
				t.Errorf("%s %s: expected X-Request-ID header", ep.method, ep.path)
			}
		}
	}

	// 2. GET /api/v1/account/security/passkeys with User A returns initially empty list
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/passkeys", nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET passkeys: expected 200, got %d", rec.Code)
		}
		var list identity.PasskeyListResponse
		if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(list.Passkeys) != 0 {
			t.Fatalf("expected 0 passkeys, got %d", len(list.Passkeys))
		}
	}

	// 3. POST /api/v1/account/security/passkeys/register/options with User A
	var opts identity.PasskeyRegistrationOptions
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/passkeys/register/options", nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		req.Host = "espacigo.com"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST options: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&opts); err != nil {
			t.Fatalf("decode options: %v", err)
		}
		if opts.ChallengeID == "" || opts.Challenge == "" {
			t.Fatalf("empty challenge in options: %+v", opts)
		}
		if opts.RP.Name != "EspaciGo" {
			t.Fatalf("expected RP name EspaciGo, got %s", opts.RP.Name)
		}
	}

	// 4. POST /api/v1/account/security/passkeys/register/verify: User B attempts to use User A's challenge -> 422
	{
		clientData := map[string]any{
			"type":      "webauthn.create",
			"challenge": opts.Challenge,
			"origin":    "http://localhost:3000",
		}
		clientDataBytes, _ := json.Marshal(clientData)
		body, _ := json.Marshal(identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			Label:          "Bob's stolen key",
			CredentialID:   "cred-bob-steal",
			PublicKey:      base64.RawURLEncoding.EncodeToString([]byte("fake-key-12345678901234567890")),
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/passkeys/register/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+userB.Token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for cross-user challenge consumption, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 5. POST /api/v1/account/security/passkeys/register/verify with User A -> 200 created Passkey
	var createdPasskey identity.Passkey
	{
		clientData := map[string]any{
			"type":      "webauthn.create",
			"challenge": opts.Challenge,
			"origin":    "http://localhost:3000",
		}
		clientDataBytes, _ := json.Marshal(clientData)
		body, _ := json.Marshal(identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			Label:          "MacBook Touch ID",
			CredentialID:   "cred-alice-1",
			PublicKey:      base64.RawURLEncoding.EncodeToString([]byte("alice-spki-key-bytes-1234567890")),
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
			Transports:     []string{"internal"},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/passkeys/register/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("verify for User A: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&createdPasskey); err != nil {
			t.Fatalf("decode created passkey: %v", err)
		}
		if createdPasskey.ID == "" || createdPasskey.Label != "MacBook Touch ID" {
			t.Fatalf("unexpected passkey: %+v", createdPasskey)
		}
	}

	// 6. User Isolation: User B calls GET /passkeys and sees 0 passkeys (User A's passkey is isolated)
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/passkeys", nil)
		req.Header.Set("Authorization", "Bearer "+userB.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("User B GET passkeys: expected 200, got %d", rec.Code)
		}
		var list identity.PasskeyListResponse
		_ = json.NewDecoder(rec.Body).Decode(&list)
		if len(list.Passkeys) != 0 {
			t.Fatalf("User B should see 0 passkeys, saw %d", len(list.Passkeys))
		}
	}

	// 7. User Isolation: User B tries to DELETE User A's passkey -> 404
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/passkeys/"+createdPasskey.ID, nil)
		req.Header.Set("Authorization", "Bearer "+userB.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("User B deleting User A's passkey: expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 8. User A calls GET /passkeys and sees 1 passkey
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/passkeys", nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("User A GET passkeys: expected 200, got %d", rec.Code)
		}
		var list identity.PasskeyListResponse
		_ = json.NewDecoder(rec.Body).Decode(&list)
		if len(list.Passkeys) != 1 || list.Passkeys[0].ID != createdPasskey.ID {
			t.Fatalf("User A expected 1 passkey, got %+v", list.Passkeys)
		}
	}

	// 9. User A deletes own passkey -> 200 {"status": "deleted", "passkey_id": ...}
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/passkeys/"+createdPasskey.ID, nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("DELETE passkey: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var delResp identity.PasskeyDeleteResponse
		if err := json.NewDecoder(rec.Body).Decode(&delResp); err != nil {
			t.Fatalf("decode delete response: %v", err)
		}
		if delResp.Status != "deleted" || delResp.PasskeyID != createdPasskey.ID {
			t.Fatalf("unexpected delete response: %+v", delResp)
		}
	}

	// 10. Deleting already deleted passkey returns 404
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/passkeys/"+createdPasskey.ID, nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("repeat DELETE passkey: expected 404, got %d", rec.Code)
		}
	}

	// 11. Method Not Allowed (405) checks
	{
		// POST on /api/v1/account/security/passkeys (expected GET)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/passkeys", nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for POST /passkeys, got %d", rec.Code)
		}

		// GET on /api/v1/account/security/passkeys/register/options (expected POST)
		req = httptest.NewRequest(http.MethodGet, "/api/v1/account/security/passkeys/register/options", nil)
		req.Header.Set("Authorization", "Bearer "+userA.Token)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for GET /register/options, got %d", rec.Code)
		}
	}
}
