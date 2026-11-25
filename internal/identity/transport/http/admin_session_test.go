package identityhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type httpFakeAdminRepo struct {
	accounts      map[string]identity.Account
	roles         map[string][]identity.Role
	totp          map[string]*identity.AccountTOTPRecord
	adminSessions map[string]identity.AdminSession
	challenges    map[string]identity.AdminMFAChallenge
}

func newHTTPFakeAdminRepo() *httpFakeAdminRepo {
	return &httpFakeAdminRepo{
		accounts:      make(map[string]identity.Account),
		roles:         make(map[string][]identity.Role),
		totp:          make(map[string]*identity.AccountTOTPRecord),
		adminSessions: make(map[string]identity.AdminSession),
		challenges:    make(map[string]identity.AdminMFAChallenge),
	}
}

func (r *httpFakeAdminRepo) WithLockedAdminAccount(ctx context.Context, lookup identity.AdminAccountLookup, fn func(identity.Account, identity.AdminSessionTransaction) error) error {
	var accountID string
	switch {
	case lookup.Email != "":
		for _, a := range r.accounts {
			if a.NormalizedEmail == lookup.Email {
				accountID = a.ID
				break
			}
		}
	case lookup.ChallengeID != "":
		if c, ok := r.challenges[lookup.ChallengeID]; ok {
			accountID = c.AccountID
		}
	case lookup.AdminSessionHash != "":
		if s, ok := r.adminSessions[strings.ToLower(lookup.AdminSessionHash)]; ok {
			accountID = s.AccountID
		}
	}
	if accountID == "" {
		return identity.ErrNotFound
	}
	acc, ok := r.accounts[accountID]
	if !ok {
		return identity.ErrNotFound
	}
	return fn(acc, &httpFakeAdminTx{repo: r})
}

type httpFakeAdminTx struct{ repo *httpFakeAdminRepo }

func (t *httpFakeAdminTx) Roles(_ context.Context, id string) ([]identity.Role, error) {
	return t.repo.roles[id], nil
}
func (t *httpFakeAdminTx) GetTOTP(_ context.Context, id string) (*identity.AccountTOTPRecord, int, error) {
	rec := t.repo.totp[id]
	if rec == nil {
		return nil, 0, nil
	}
	cp := *rec
	return &cp, 0, nil
}
func (t *httpFakeAdminTx) UpdateTOTPLastVerifiedStep(_ context.Context, id string, step int64) error {
	if rec := t.repo.totp[id]; rec != nil {
		rec.LastVerifiedStep = &step
	}
	return nil
}
func (t *httpFakeAdminTx) ConsumeBackupCode(context.Context, string, string) (bool, error) {
	return false, nil
}
func (t *httpFakeAdminTx) CreateAdminSession(_ context.Context, s identity.AdminSession) error {
	t.repo.adminSessions[strings.ToLower(s.TokenHash)] = s
	return nil
}
func (t *httpFakeAdminTx) AdminSessionByTokenHash(_ context.Context, hash string) (identity.AdminSession, error) {
	s, ok := t.repo.adminSessions[strings.ToLower(hash)]
	if !ok {
		return identity.AdminSession{}, identity.ErrNotFound
	}
	return s, nil
}
func (t *httpFakeAdminTx) TouchAdminSession(_ context.Context, sessionID string, at time.Time) (bool, error) {
	for h, s := range t.repo.adminSessions {
		if s.ID == sessionID && s.ActiveAt(at) {
			s.LastActivityAt = at
			t.repo.adminSessions[h] = s
			return true, nil
		}
	}
	return false, nil
}
func (t *httpFakeAdminTx) RevokeAdminSession(_ context.Context, sessionID string, at time.Time) error {
	for h, s := range t.repo.adminSessions {
		if s.ID == sessionID {
			s.RevokedAt = &at
			t.repo.adminSessions[h] = s
			return nil
		}
	}
	return identity.ErrNotFound
}
func (t *httpFakeAdminTx) UpdateAdminSessionMFA(_ context.Context, sessionID string, verifiedAt time.Time, method string) error {
	for h, s := range t.repo.adminSessions {
		if s.ID == sessionID && s.RevokedAt == nil {
			s.MFAVerifiedAt = &verifiedAt
			s.MFAMethod = method
			t.repo.adminSessions[h] = s
			return nil
		}
	}
	return identity.ErrNotFound
}
func (t *httpFakeAdminTx) CreateAdminMFAChallenge(_ context.Context, c identity.AdminMFAChallenge) error {
	t.repo.challenges[c.ID] = c
	return nil
}
func (t *httpFakeAdminTx) AdminMFAChallengeByID(_ context.Context, id string) (identity.AdminMFAChallenge, error) {
	c, ok := t.repo.challenges[id]
	if !ok {
		return identity.AdminMFAChallenge{}, identity.ErrNotFound
	}
	return c, nil
}
func (t *httpFakeAdminTx) IncrementAdminMFAChallengeAttempt(_ context.Context, id string) (int, error) {
	c, ok := t.repo.challenges[id]
	if !ok {
		return 0, identity.ErrNotFound
	}
	c.Attempts++
	t.repo.challenges[id] = c
	return c.Attempts, nil
}
func (t *httpFakeAdminTx) ConsumeAdminMFAChallenge(_ context.Context, id string, at time.Time) error {
	c, ok := t.repo.challenges[id]
	if !ok {
		return identity.ErrNotFound
	}
	c.ConsumedAt = &at
	t.repo.challenges[id] = c
	return nil
}

type httpFakeCreds struct{ n int }

func (c *httpFakeCreds) ID() (string, error) {
	c.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", c.n), nil
}
func (c *httpFakeCreds) Token() (identity.Secret, error) {
	c.n++
	return identity.Secret(fmt.Sprintf("adm-http-tok-%06d", c.n)), nil
}

type httpFakePass struct{}

func (httpFakePass) Hash(s identity.Secret) (identity.Secret, error)          { return s, nil }
func (httpFakePass) Matches(hash, raw identity.Secret) (bool, error) { return hash == raw, nil }

func TestAdminSessionHTTPRoutes(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	repo := newHTTPFakeAdminRepo()

	aesKey := make([]byte, 32)
	_, _ = rand.Read(aesKey)
	kr, err := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": aesKey})
	if err != nil {
		t.Fatal(err)
	}
	totpSecret := []byte("12345678901234567890")
	ver, ct, err := kr.Seal(totpSecret)
	if err != nil {
		t.Fatal(err)
	}

	staffID := "00000000-0000-4000-8000-000000000100"
	enabledAt := now.Add(-time.Hour)
	repo.accounts[staffID] = identity.Account{
		ID: staffID, Email: "gadm@espacigo.cl", NormalizedEmail: "gadm@espacigo.cl",
		PasswordHash: "Secret123!", State: identity.AccountActive,
	}
	repo.roles[staffID] = []identity.Role{identity.RoleGlobalAdministrator}
	repo.totp[staffID] = &identity.AccountTOTPRecord{
		AccountID: staffID, SecretCiphertext: ct, KeyVersion: &ver, EnabledAt: &enabledAt,
	}

	svc, err := identity.NewAdminSessionService(repo, httpFakePass{}, kr, &httpFakeCreds{}, func() time.Time { return now }, true)
	if err != nil {
		t.Fatal(err)
	}
	h := NewAdminSessionHandler(svc, nil)

	// 1. Commercial token rejected with 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	req.Header.Set("Authorization", "Bearer commercial-session-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for commercial token, got %d %s", rec.Code, rec.Body.String())
	}

	// 2. Start -> 200 mfa_required.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/start", bytes.NewBufferString(`{"email":"gadm@espacigo.cl","password":"Secret123!"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on start, got %d %s", rec.Code, rec.Body.String())
	}
	var startBody struct {
		Status      string `json:"status"`
		ChallengeID string `json:"challenge_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &startBody)
	if startBody.Status != "mfa_required" || startBody.ChallengeID == "" {
		t.Fatalf("unexpected start body: %+v", startBody)
	}

	// 3. Verify MFA -> 200 authenticated with token.
	code := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/mfa", bytes.NewBufferString(fmt.Sprintf(`{"challenge_id":%q,"code":%q}`, startBody.ChallengeID, code)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on mfa verify, got %d %s", rec.Code, rec.Body.String())
	}
	var mfaBody struct {
		Status   string `json:"status"`
		Audience string `json:"audience"`
		Token    string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mfaBody)
	if mfaBody.Status != "authenticated" || mfaBody.Audience != "admin" || mfaBody.Token == "" {
		t.Fatalf("unexpected mfa body: %+v", mfaBody)
	}

	// 4. GET /api/v1/admin/session -> 200.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+mfaBody.Token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"audience":"admin"`) {
		t.Fatalf("expected 200 with admin audience, got %d %s", rec.Code, rec.Body.String())
	}

	// 5. Step-up MFA with Bearer token -> 200.
	now = now.Add(15 * time.Minute)
	stepCode := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/mfa", bytes.NewBufferString(fmt.Sprintf(`{"code":%q}`, stepCode)))
	req.Header.Set("Authorization", "Bearer "+mfaBody.Token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on step-up mfa, got %d %s", rec.Code, rec.Body.String())
	}

	// 6. DELETE /api/v1/admin/session -> 204, then GET -> 401.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+mfaBody.Token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on delete, got %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+mfaBody.Token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revocation, got %d %s", rec.Code, rec.Body.String())
	}
}
