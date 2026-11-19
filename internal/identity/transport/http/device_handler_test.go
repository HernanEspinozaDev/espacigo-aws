package identityhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestDeviceEndpointsRoutingAndAuthentication(t *testing.T) {
	h := NewHandler(nil, nil, nil).(*Handler)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/account/security/devices"},
		{http.MethodDelete, "/api/v1/account/security/devices/test-session-id"},
		{http.MethodPost, "/api/v1/account/security/devices/revoke-others"},
	}

	for _, rt := range routes {
		t.Run("missing_auth_"+rt.path, func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			if rt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401, got %d (path: %s)", rec.Code, rt.path)
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
			wrongMethod := http.MethodPut
			req := httptest.NewRequest(wrongMethod, rt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected status 405, got %d (path: %s)", rec.Code, rt.path)
			}
		})
	}
}

func TestDeviceErrorMapping(t *testing.T) {
	h := NewHandler(nil, nil, nil).(*Handler)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found",
			err:        identity.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "device_not_found",
		},
		{
			name:       "invalid",
			err:        identity.ErrInvalid,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("X-Request-ID", "req-test-123")
			h.deviceError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body struct {
				Error struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s", body.Error.Code, tt.wantCode)
			}
			if len(body.Error.Message) > 100 {
				t.Fatalf("message length > 100: %d chars", len(body.Error.Message))
			}
			if body.Error.RequestID != "req-test-123" {
				t.Fatalf("request_id = %s, want req-test-123", body.Error.RequestID)
			}
		})
	}
}

func TestClientIPForDevice(t *testing.T) {
	t.Run("prefers X-Espacigo-Client-IP if present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Espacigo-Client-IP", "203.0.113.195")
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		req.RemoteAddr = "127.0.0.1:8080"

		got := clientIPForDevice(req)
		if got != "203.0.113.195" {
			t.Errorf("got %q, want 203.0.113.195", got)
		}
	})

	t.Run("uses first X-Forwarded-For if internal header missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "190.162.12.45, 10.0.0.1")
		req.RemoteAddr = "127.0.0.1:8080"

		got := clientIPForDevice(req)
		if got != "190.162.12.45" {
			t.Errorf("got %q, want 190.162.12.45", got)
		}
	})

	t.Run("falls back to RemoteAddr host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.100:54321"

		got := clientIPForDevice(req)
		if got != "192.168.1.100" {
			t.Errorf("got %q, want 192.168.1.100", got)
		}
	})
}

// mockAuthRepo implements identity.AuthenticationRepository and identity.AuthenticationTransaction for integration-style handler tests.
type mockAuthRepo struct {
	accounts map[string]identity.Account
	sessions map[string]identity.Session
	tokens   map[string]string // token -> sessionID
	prefs    map[string]*identity.SecurityAlertPreferencesRecord
	events   map[string][]identity.SecurityAlertEventRecord
	totpRecords map[string]*identity.AccountTOTPRecord
	backupCodes map[string]map[string]bool // accountID -> codeHash -> true
	passkeys    map[string][]identity.PasskeyRecord
	challenges  map[string]*identity.WebAuthnChallengeRecord
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		accounts: make(map[string]identity.Account),
		sessions: make(map[string]identity.Session),
		tokens:   make(map[string]string),
		prefs:    make(map[string]*identity.SecurityAlertPreferencesRecord),
		events:   make(map[string][]identity.SecurityAlertEventRecord),
		totpRecords: make(map[string]*identity.AccountTOTPRecord),
		backupCodes: make(map[string]map[string]bool),
		passkeys:    make(map[string][]identity.PasskeyRecord),
		challenges:  make(map[string]*identity.WebAuthnChallengeRecord),
	}
}

func (m *mockAuthRepo) CreateWithTerms(context.Context, identity.Account, []identity.TermsAcceptance) error {
	return nil
}
func (m *mockAuthRepo) TermsVersion(context.Context, string) (identity.TermsVersion, error) {
	return identity.TermsVersion{}, nil
}
func (m *mockAuthRepo) PurgeExpiredPasswordHistory(context.Context, time.Time) error { return nil }
func (m *mockAuthRepo) PurgeExpiredCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (m *mockAuthRepo) CancelInactiveCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (m *mockAuthRepo) ListTerminalCredentialNotices(context.Context) ([]identity.CredentialNoticeSummary, error) {
	return nil, nil
}
func (m *mockAuthRepo) ReopenCredentialNotice(context.Context, identity.ReopenCredentialNoticeInput) (identity.CredentialNoticeCycle, error) {
	return identity.CredentialNoticeCycle{}, nil
}
func (m *mockAuthRepo) ClaimCredentialNotice(context.Context, time.Time, time.Time) (identity.CredentialNotice, error) {
	return identity.CredentialNotice{}, nil
}
func (m *mockAuthRepo) ActiveAccountEmail(_ context.Context, accountID string) (string, error) {
	if a, ok := m.accounts[accountID]; ok {
		return a.Email, nil
	}
	return "", identity.ErrNotFound
}
func (m *mockAuthRepo) CancelInactiveCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (m *mockAuthRepo) CompleteCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (m *mockAuthRepo) RetryCredentialNotice(context.Context, string, time.Time, time.Time, time.Time, string) error {
	return nil
}

func (m *mockAuthRepo) WithLockedAccount(ctx context.Context, lookup identity.AccountLookup, fn func(identity.Account, identity.AuthenticationTransaction) error) error {
	var target identity.Account
	var found bool
	if lookup.Email != "" {
		for _, a := range m.accounts {
			if a.NormalizedEmail == lookup.Email {
				target = a
				found = true
				break
			}
		}
	} else if lookup.SessionHash != "" {
		for _, s := range m.sessions {
			if s.TokenHash == lookup.SessionHash {
				target = m.accounts[s.AccountID]
				found = true
				break
			}
		}
	}
	if !found {
		return identity.ErrNotFound
	}
	return fn(target, m)
}

func (m *mockAuthRepo) SaveLoginState(_ context.Context, accountID string, state identity.AccountState, failed int, blocked *time.Time) error {
	a := m.accounts[accountID]
	a.State = state
	a.FailedAttempts = failed
	a.BlockedUntil = blocked
	m.accounts[accountID] = a
	return nil
}

func (m *mockAuthRepo) CreateSession(_ context.Context, s identity.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockAuthRepo) SessionByTokenHash(_ context.Context, hash string) (identity.Session, error) {
	for _, s := range m.sessions {
		if s.TokenHash == hash {
			return s, nil
		}
	}
	return identity.Session{}, identity.ErrNotFound
}

func (m *mockAuthRepo) RevokeSession(_ context.Context, id string, at time.Time) error {
	s, ok := m.sessions[id]
	if !ok {
		return identity.ErrNotFound
	}
	s.RevokedAt = &at
	m.sessions[id] = s
	return nil
}

func (m *mockAuthRepo) TouchSession(_ context.Context, id string, at time.Time) (bool, error) {
	s, ok := m.sessions[id]
	if !ok || s.RevokedAt != nil {
		return false, nil
	}
	s.LastActivityAt = at
	m.sessions[id] = s
	return true, nil
}

func (m *mockAuthRepo) Roles(_ context.Context, _ string) ([]identity.Role, error) {
	return []identity.Role{identity.RoleTenant}, nil
}
func (m *mockAuthRepo) ActionToken(context.Context, string) (identity.ActionToken, error) {
	return identity.ActionToken{}, nil
}
func (m *mockAuthRepo) ReplaceActionToken(context.Context, identity.ActionToken) error { return nil }
func (m *mockAuthRepo) UpdatePasswordHash(context.Context, string, identity.Secret) error {
	return nil
}
func (m *mockAuthRepo) DeleteExpiredPasswordHistory(context.Context, string, time.Time) error {
	return nil
}
func (m *mockAuthRepo) PreviousPasswordHashes(context.Context, string, time.Time) ([]identity.Secret, error) {
	return nil, nil
}
func (m *mockAuthRepo) StorePreviousPasswordHash(context.Context, string, string, identity.Secret, time.Time, time.Time, time.Time) error {
	return nil
}
func (m *mockAuthRepo) EnqueueCredentialChanged(context.Context, string, string, string, time.Time) error {
	return nil
}
func (m *mockAuthRepo) RecordCredentialChangeAudit(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (m *mockAuthRepo) RevokeActiveSessions(_ context.Context, accountID string, at time.Time) error {
	for k, s := range m.sessions {
		if s.AccountID == accountID && s.RevokedAt == nil {
			s.RevokedAt = &at
			m.sessions[k] = s
		}
	}
	return nil
}
func (m *mockAuthRepo) CountActionTokenEmissions(context.Context, string, string, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAuthRepo) RecordActionTokenFailure(context.Context, string, time.Time) (bool, error) {
	return false, nil
}
func (m *mockAuthRepo) ConsumeActionToken(context.Context, string, time.Time) (bool, error) {
	return false, nil
}

func (m *mockAuthRepo) ListActiveSessions(_ context.Context, accountID string, now time.Time) ([]identity.Session, error) {
	var out []identity.Session
	for _, s := range m.sessions {
		if s.AccountID == accountID && s.RevokedAt == nil && s.ExpiresAt.After(now) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *mockAuthRepo) RevokeSessionForAccount(_ context.Context, accountID string, sessionID string, revokedAt time.Time) (bool, error) {
	s, ok := m.sessions[sessionID]
	if !ok || s.AccountID != accountID || s.RevokedAt != nil {
		return false, nil
	}
	s.RevokedAt = &revokedAt
	m.sessions[sessionID] = s
	return true, nil
}

func (m *mockAuthRepo) RevokeOtherSessionsForAccount(_ context.Context, accountID string, keepSessionID string, revokedAt time.Time) (int64, error) {
	var count int64
	for k, s := range m.sessions {
		if s.AccountID == accountID && s.ID != keepSessionID && s.RevokedAt == nil && s.ExpiresAt.After(revokedAt) {
			s.RevokedAt = &revokedAt
			m.sessions[k] = s
			count++
		}
	}
	return count, nil
}

func (m *mockAuthRepo) GetSecurityAlertPreferences(_ context.Context, accountID string) (*identity.SecurityAlertPreferencesRecord, error) {
	return m.prefs[accountID], nil
}

func (m *mockAuthRepo) UpsertSecurityAlertPreferences(_ context.Context, record identity.SecurityAlertPreferencesRecord) error {
	rec := record
	m.prefs[record.AccountID] = &rec
	return nil
}

func (m *mockAuthRepo) ListSecurityAlertEvents(_ context.Context, accountID string, limit int) ([]identity.SecurityAlertEventRecord, error) {
	evs := m.events[accountID]
	if limit > 0 && len(evs) > limit {
		return evs[:limit], nil
	}
	return evs, nil
}

func (m *mockAuthRepo) RecordSecurityAlertEvent(_ context.Context, event identity.SecurityAlertEventRecord) error {
	m.events[event.AccountID] = append([]identity.SecurityAlertEventRecord{event}, m.events[event.AccountID]...)
	return nil
}

func (m *mockAuthRepo) GetPhoneVerification(_ context.Context, _ string) (*identity.AccountPhoneVerification, error) {
	return nil, nil
}

func (m *mockAuthRepo) GetTOTP(_ context.Context, accountID string) (*identity.AccountTOTPRecord, int, error) {
	rec := m.totpRecords[accountID]
	var remaining int
	if m.backupCodes != nil && m.backupCodes[accountID] != nil {
		remaining = len(m.backupCodes[accountID])
	}
	return rec, remaining, nil
}

func (m *mockAuthRepo) SavePendingTOTPSecret(_ context.Context, accountID string, ciphertext []byte, keyVersion string) error {
	rec := m.totpRecords[accountID]
	if rec == nil {
		rec = &identity.AccountTOTPRecord{AccountID: accountID}
		m.totpRecords[accountID] = rec
	}
	ver := keyVersion
	rec.PendingSecretCiphertext = ciphertext
	rec.PendingKeyVersion = &ver
	return nil
}

func (m *mockAuthRepo) ActivateTOTP(_ context.Context, accountID string, verifiedStep int64, backupCodeHashes []string) error {
	rec := m.totpRecords[accountID]
	if rec == nil || len(rec.PendingSecretCiphertext) == 0 {
		return identity.ErrTOTPNotPending
	}
	now := time.Now()
	rec.SecretCiphertext = rec.PendingSecretCiphertext
	rec.KeyVersion = rec.PendingKeyVersion
	rec.PendingSecretCiphertext = nil
	rec.PendingKeyVersion = nil
	rec.EnabledAt = &now
	rec.LastVerifiedStep = &verifiedStep

	m.backupCodes[accountID] = make(map[string]bool)
	for _, h := range backupCodeHashes {
		m.backupCodes[accountID][h] = true
	}
	return nil
}

func (m *mockAuthRepo) UpdateTOTPLastVerifiedStep(_ context.Context, accountID string, verifiedStep int64) error {
	if rec := m.totpRecords[accountID]; rec != nil {
		rec.LastVerifiedStep = &verifiedStep
	}
	return nil
}

func (m *mockAuthRepo) ConsumeBackupCode(_ context.Context, accountID string, codeHash string) (bool, error) {
	if m.backupCodes != nil && m.backupCodes[accountID] != nil && m.backupCodes[accountID][codeHash] {
		delete(m.backupCodes[accountID], codeHash)
		return true, nil
	}
	return false, nil
}

func (m *mockAuthRepo) DisableTOTP(_ context.Context, accountID string) error {
	if rec := m.totpRecords[accountID]; rec != nil {
		rec.SecretCiphertext = nil
		rec.KeyVersion = nil
		rec.PendingSecretCiphertext = nil
		rec.PendingKeyVersion = nil
		rec.EnabledAt = nil
		rec.LastVerifiedStep = nil
	}
	delete(m.backupCodes, accountID)
	return nil
}

func (m *mockAuthRepo) ListPasskeys(_ context.Context, accountID string) ([]identity.PasskeyRecord, error) {
	return m.passkeys[accountID], nil
}

func (m *mockAuthRepo) SaveWebAuthnChallenge(_ context.Context, challenge identity.WebAuthnChallengeRecord) error {
	m.challenges[challenge.ID] = &challenge
	return nil
}

func (m *mockAuthRepo) GetWebAuthnChallenge(_ context.Context, challengeID string) (*identity.WebAuthnChallengeRecord, error) {
	ch, ok := m.challenges[challengeID]
	if !ok {
		return nil, identity.ErrNotFound
	}
	return ch, nil
}

func (m *mockAuthRepo) SavePasskeyAndConsumeChallenge(_ context.Context, passkey identity.PasskeyRecord, challengeID string, consumedAt time.Time) error {
	ch, ok := m.challenges[challengeID]
	if !ok {
		return identity.ErrPasskeyChallengeInvalid
	}
	if ch.ConsumedAt != nil {
		return identity.ErrPasskeyChallengeConsumed
	}
	ch.ConsumedAt = &consumedAt

	for _, pks := range m.passkeys {
		for _, pk := range pks {
			if pk.CredentialID == passkey.CredentialID {
				return identity.ErrPasskeyCredentialConflict
			}
		}
	}
	m.passkeys[passkey.AccountID] = append(m.passkeys[passkey.AccountID], passkey)
	return nil
}

func (m *mockAuthRepo) DeletePasskey(_ context.Context, accountID, passkeyID string) error {
	pks := m.passkeys[accountID]
	idx := -1
	for i, pk := range pks {
		if pk.ID == passkeyID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return identity.ErrPasskeyNotFound
	}
	m.passkeys[accountID] = append(pks[:idx], pks[idx+1:]...)
	return nil
}

func (m *mockAuthRepo) GetAccountDisplayName(_ context.Context, _ string) (string, error) {
	return "", nil
}

type testHasher struct{}

func (testHasher) Hash(s identity.Secret) (identity.Secret, error) { return s, nil }
func (testHasher) Matches(h, s identity.Secret) (bool, error)     { return h == s, nil }

type dummyMailer struct{}

func (dummyMailer) SendVerification(context.Context, identity.VerificationDelivery) error { return nil }
func (dummyMailer) SendRecovery(context.Context, identity.RecoveryDelivery) error         { return nil }
func (dummyMailer) SendPasswordChanged(context.Context, string) error                     { return nil }
func (dummyMailer) SendLoginAlert(context.Context, string, time.Time) error               { return nil }

type dummyLimiter struct{}

func (dummyLimiter) AllowVerification(context.Context, string, time.Time) (bool, error) {
	return true, nil
}

type testCredGen struct {
	nextID    string
	nextToken string
}

func (g *testCredGen) ID() (string, error)          { return g.nextID, nil }
func (g *testCredGen) Token() (identity.Secret, error) { return identity.Secret(g.nextToken), nil }

func TestDeviceEndpointsEndToEnd(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockAuthRepo()

	accountA := identity.Account{
		ID:              "account-a",
		Email:           "alice@example.com",
		NormalizedEmail: "alice@example.com",
		PasswordHash:    "secret123",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	accountB := identity.Account{
		ID:              "account-b",
		Email:           "bob@example.com",
		NormalizedEmail: "bob@example.com",
		PasswordHash:    "secret123",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[accountA.ID] = accountA
	repo.accounts[accountB.ID] = accountB

	credGen := &testCredGen{}
	service, err := identity.NewAuthenticationService(repo, testHasher{}, dummyMailer{}, dummyLimiter{}, credGen, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAuthenticationService: %v", err)
	}
	handler := NewHandler(service, repo, nil)

	// 1. Login as Account A from Chrome on Linux
	credGen.nextID = "session-a1"
	credGen.nextToken = "token-a1"
	loginBody := `{"email":"alice@example.com","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	req.Header.Set("X-Forwarded-For", "190.162.12.45")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login A1 failed: status %d body: %s", rec.Code, rec.Body.String())
	}

	// Verify session-a1 device metadata in repo
	sesA1 := repo.sessions["session-a1"]
	if sesA1.DeviceLabel != "Chrome en Linux" {
		t.Fatalf("expected device label 'Chrome en Linux', got %q", sesA1.DeviceLabel)
	}
	if sesA1.IPMasked != "190.162.•••.•••" {
		t.Fatalf("expected ip_masked '190.162.•••.•••', got %q", sesA1.IPMasked)
	}

	// 2. Login as Account A from Safari on iPhone (session A2)
	credGen.nextID = "session-a2"
	credGen.nextToken = "token-a2"
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Safari/604.1")
	req.Header.Set("X-Forwarded-For", "200.54.10.88")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login A2 failed: status %d", rec.Code)
	}

	// 3. Login as Account B (session B1)
	credGen.nextID = "session-b1"
	credGen.nextToken = "token-b1"
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"bob@example.com","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/119.0")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login B1 failed: status %d", rec.Code)
	}

	// 4. Test GET /api/v1/account/security/devices with session A1
	req = httptest.NewRequest("GET", "/api/v1/account/security/devices", nil)
	req.Header.Set("Authorization", "Bearer token-a1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET devices failed: status %d body: %s", rec.Code, rec.Body.String())
	}
	var devResp struct {
		Devices []identity.LinkedDevice `json:"devices"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&devResp); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
	if len(devResp.Devices) != 2 {
		t.Fatalf("expected 2 devices for account A, got %d", len(devResp.Devices))
	}
	for _, d := range devResp.Devices {
		if d.ID == "session-a1" && !d.IsCurrent {
			t.Errorf("session-a1 should have is_current = true")
		}
		if d.ID == "session-a2" && d.IsCurrent {
			t.Errorf("session-a2 should have is_current = false")
		}
		if d.ID == "session-b1" {
			t.Errorf("session-b1 of Account B must never leak into Account A devices")
		}
	}

	// 5. Isolation Test: Account A cannot revoke Account B's session
	req = httptest.NewRequest("DELETE", "/api/v1/account/security/devices/session-b1", nil)
	req.Header.Set("Authorization", "Bearer token-a1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when revoking another account's device, got %d", rec.Code)
	}
	if repo.sessions["session-b1"].RevokedAt != nil {
		t.Fatalf("session-b1 was unexpectedly revoked by Account A!")
	}

	// 6. Revoke own session A2
	req = httptest.NewRequest("DELETE", "/api/v1/account/security/devices/session-a2", nil)
	req.Header.Set("Authorization", "Bearer token-a1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when revoking session A2, got %d body: %s", rec.Code, rec.Body.String())
	}
	var revResp struct {
		Status    string `json:"status"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&revResp); err != nil {
		t.Fatalf("decode revoke response: %v", err)
	}
	if revResp.Status != "revoked" || revResp.SessionID != "session-a2" {
		t.Fatalf("unexpected revoke payload: %+v", revResp)
	}
	if repo.sessions["session-a2"].RevokedAt == nil {
		t.Fatalf("session-a2 was not marked as revoked in repository")
	}

	// 7. Revoke Other Devices
	// Re-add a session A3
	credGen.nextID = "session-a3"
	credGen.nextToken = "token-a3"
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login A3 failed: status %d", rec.Code)
	}

	// Now call revoke-others using session A1
	req = httptest.NewRequest("POST", "/api/v1/account/security/devices/revoke-others", nil)
	req.Header.Set("Authorization", "Bearer token-a1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for revoke-others, got %d body: %s", rec.Code, rec.Body.String())
	}
	var revOthersResp struct {
		Status       string `json:"status"`
		RevokedCount int64  `json:"revoked_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&revOthersResp); err != nil {
		t.Fatalf("decode revoke-others: %v", err)
	}
	if revOthersResp.Status != "revoked_others" || revOthersResp.RevokedCount != 1 {
		t.Fatalf("unexpected revoke-others response: %+v", revOthersResp)
	}
	if repo.sessions["session-a3"].RevokedAt == nil {
		t.Fatalf("session-a3 should have been revoked")
	}
	if repo.sessions["session-a1"].RevokedAt != nil {
		t.Fatalf("current session-a1 should NOT have been revoked")
	}
}
