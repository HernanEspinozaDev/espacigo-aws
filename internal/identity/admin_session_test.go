package identity_test

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type fakeAdminRepo struct {
	accounts       map[string]identity.Account
	roles          map[string][]identity.Role
	totp           map[string]*identity.AccountTOTPRecord
	backupHashes   map[string]map[string]bool
	adminSessions  map[string]identity.AdminSession // keyed by tokenHash
	challenges     map[string]identity.AdminMFAChallenge
	commercialToks map[string]bool // commercial session hashes (must be rejected by admin repo)
}

func newFakeAdminRepo() *fakeAdminRepo {
	return &fakeAdminRepo{
		accounts:       make(map[string]identity.Account),
		roles:          make(map[string][]identity.Role),
		totp:           make(map[string]*identity.AccountTOTPRecord),
		backupHashes:   make(map[string]map[string]bool),
		adminSessions:  make(map[string]identity.AdminSession),
		challenges:     make(map[string]identity.AdminMFAChallenge),
		commercialToks: make(map[string]bool),
	}
}

func (r *fakeAdminRepo) WithLockedAdminAccount(ctx context.Context, lookup identity.AdminAccountLookup, fn func(identity.Account, identity.AdminSessionTransaction) error) error {
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
	return fn(acc, &fakeAdminTx{repo: r})
}

type fakeAdminTx struct {
	repo *fakeAdminRepo
}

func (t *fakeAdminTx) Roles(_ context.Context, accountID string) ([]identity.Role, error) {
	return t.repo.roles[accountID], nil
}

func (t *fakeAdminTx) GetTOTP(_ context.Context, accountID string) (*identity.AccountTOTPRecord, int, error) {
	rec := t.repo.totp[accountID]
	if rec == nil {
		return nil, 0, nil
	}
	copyRec := *rec
	return &copyRec, len(t.repo.backupHashes[accountID]), nil
}

func (t *fakeAdminTx) UpdateTOTPLastVerifiedStep(_ context.Context, accountID string, step int64) error {
	if rec := t.repo.totp[accountID]; rec != nil {
		rec.LastVerifiedStep = &step
	}
	return nil
}

func (t *fakeAdminTx) ConsumeBackupCode(_ context.Context, accountID, codeHash string) (bool, error) {
	if m := t.repo.backupHashes[accountID]; m != nil && m[codeHash] {
		delete(m, codeHash)
		return true, nil
	}
	return false, nil
}

func (t *fakeAdminTx) CreateAdminSession(_ context.Context, s identity.AdminSession) error {
	t.repo.adminSessions[strings.ToLower(s.TokenHash)] = s
	return nil
}

func (t *fakeAdminTx) AdminSessionByTokenHash(_ context.Context, tokenHash string) (identity.AdminSession, error) {
	s, ok := t.repo.adminSessions[strings.ToLower(tokenHash)]
	if !ok {
		return identity.AdminSession{}, identity.ErrNotFound
	}
	return s, nil
}

func (t *fakeAdminTx) TouchAdminSession(_ context.Context, sessionID string, at time.Time) (bool, error) {
	for h, s := range t.repo.adminSessions {
		if s.ID == sessionID && s.ActiveAt(at) {
			s.LastActivityAt = at
			t.repo.adminSessions[h] = s
			return true, nil
		}
	}
	return false, nil
}

func (t *fakeAdminTx) RevokeAdminSession(_ context.Context, sessionID string, at time.Time) error {
	for h, s := range t.repo.adminSessions {
		if s.ID == sessionID {
			s.RevokedAt = &at
			t.repo.adminSessions[h] = s
			return nil
		}
	}
	return identity.ErrNotFound
}

func (t *fakeAdminTx) UpdateAdminSessionMFA(_ context.Context, sessionID string, verifiedAt time.Time, method string) error {
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

func (t *fakeAdminTx) CreateAdminMFAChallenge(_ context.Context, c identity.AdminMFAChallenge) error {
	t.repo.challenges[c.ID] = c
	return nil
}

func (t *fakeAdminTx) AdminMFAChallengeByID(_ context.Context, id string) (identity.AdminMFAChallenge, error) {
	c, ok := t.repo.challenges[id]
	if !ok {
		return identity.AdminMFAChallenge{}, identity.ErrNotFound
	}
	return c, nil
}

func (t *fakeAdminTx) IncrementAdminMFAChallengeAttempt(_ context.Context, id string) (int, error) {
	c, ok := t.repo.challenges[id]
	if !ok {
		return 0, identity.ErrNotFound
	}
	c.Attempts++
	t.repo.challenges[id] = c
	return c.Attempts, nil
}

func (t *fakeAdminTx) ConsumeAdminMFAChallenge(_ context.Context, id string, at time.Time) error {
	c, ok := t.repo.challenges[id]
	if !ok {
		return identity.ErrNotFound
	}
	c.ConsumedAt = &at
	t.repo.challenges[id] = c
	return nil
}

type fakeAdminCreds struct{ seq int }

func (c *fakeAdminCreds) ID() (string, error) {
	c.seq++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", c.seq), nil
}

func (c *fakeAdminCreds) Token() (identity.Secret, error) {
	c.seq++
	return identity.Secret(fmt.Sprintf("admin-tok-%06d", c.seq)), nil
}

type fakeAdminPasswords struct{}

func (fakeAdminPasswords) Hash(s identity.Secret) (identity.Secret, error) { return s, nil }
func (fakeAdminPasswords) Matches(hash, raw identity.Secret) (bool, error) {
	return hash == raw, nil
}

func newTestKeyringAndSecret(t *testing.T) (*identity.TOTPKeyring, []byte, string, []byte) {
	t.Helper()
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	kr, err := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": aesKey})
	if err != nil {
		t.Fatal(err)
	}
	totpSecret := []byte("12345678901234567890")
	ver, ct, err := kr.Seal(totpSecret)
	if err != nil {
		t.Fatal(err)
	}
	_ = base32.StdEncoding
	return kr, totpSecret, ver, ct
}

func TestAdminSessionRejectsCommercialTokenAndNonStaff(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	repo := newFakeAdminRepo()
	staffID := "00000000-0000-4000-8000-000000000010"
	userID := "00000000-0000-4000-8000-000000000011"
	repo.accounts[staffID] = identity.Account{
		ID: staffID, Email: "admin@espacigo.cl", NormalizedEmail: "admin@espacigo.cl",
		PasswordHash: "Secret123!", State: identity.AccountActive,
	}
	repo.roles[staffID] = []identity.Role{identity.RoleGlobalAdministrator}
	repo.accounts[userID] = identity.Account{
		ID: userID, Email: "user@espacigo.cl", NormalizedEmail: "user@espacigo.cl",
		PasswordHash: "Secret123!", State: identity.AccountActive,
	}
	repo.roles[userID] = []identity.Role{identity.RoleTenant}
	repo.commercialToks[identity.CredentialHash("commercial-token-123")] = true

	svc, err := identity.NewAdminSessionService(repo, fakeAdminPasswords{}, nil, &fakeAdminCreds{}, func() time.Time { return now }, false)
	if err != nil {
		t.Fatal(err)
	}

	// Commercial token must be rejected on GetSession and AuthorizeAdminCapability.
	if _, err := svc.GetSession(context.Background(), "commercial-token-123"); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for commercial token on GetSession, got %v", err)
	}
	if _, err := svc.AuthorizeAdminCapability(context.Background(), "commercial-token-123", identity.CapabilityAccountsRead, identity.UserOperation); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for commercial token on AuthorizeAdminCapability, got %v", err)
	}

	// Non-staff user must be rejected with ErrAdminStaffRequired.
	if _, err := svc.Start(context.Background(), identity.AdminSessionStartInput{Email: "user@espacigo.cl", Password: "Secret123!"}); !errors.Is(err, identity.ErrAdminStaffRequired) {
		t.Fatalf("expected ErrAdminStaffRequired for non-staff user, got %v", err)
	}
}

func TestAdminSessionMFAChallengeLimitsExpirationStepUpAndRevocation(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	repo := newFakeAdminRepo()
	kr, totpSecret, ver, ct := newTestKeyringAndSecret(t)
	staffID := "00000000-0000-4000-8000-000000000020"
	enabledAt := now.Add(-time.Hour)
	repo.accounts[staffID] = identity.Account{
		ID: staffID, Email: "staff@espacigo.cl", NormalizedEmail: "staff@espacigo.cl",
		PasswordHash: "Secret123!", State: identity.AccountActive,
	}
	repo.roles[staffID] = []identity.Role{identity.RoleGlobalAdministrator}
	repo.totp[staffID] = &identity.AccountTOTPRecord{
		AccountID: staffID, SecretCiphertext: ct, KeyVersion: &ver, EnabledAt: &enabledAt,
	}

	svc, err := identity.NewAdminSessionService(repo, fakeAdminPasswords{}, kr, &fakeAdminCreds{}, clock, true)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start returns MFA challenge.
	startRes, err := svc.Start(context.Background(), identity.AdminSessionStartInput{Email: "staff@espacigo.cl", Password: "Secret123!"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if startRes.Status != "mfa_required" || startRes.ChallengeID == "" {
		t.Fatalf("expected mfa_required challenge, got %+v", startRes)
	}

	// 2. Exhaust 5 bad attempts -> 5th returns ErrAdminMFAChallengeExhausted (429).
	for i := 1; i <= 4; i++ {
		_, err := svc.VerifyMFA(context.Background(), identity.AdminSessionMFAInput{ChallengeID: startRes.ChallengeID, Code: "000000"})
		if !errors.Is(err, identity.ErrAdminMFACodeInvalid) {
			t.Fatalf("attempt %d: expected ErrAdminMFACodeInvalid, got %v", i, err)
		}
	}
	_, err = svc.VerifyMFA(context.Background(), identity.AdminSessionMFAInput{ChallengeID: startRes.ChallengeID, Code: "000000"})
	if !errors.Is(err, identity.ErrAdminMFAChallengeExhausted) {
		t.Fatalf("attempt 5: expected ErrAdminMFAChallengeExhausted, got %v", err)
	}

	// 3. New challenge expires after 5 minutes.
	start2, err := svc.Start(context.Background(), identity.AdminSessionStartInput{Email: "staff@espacigo.cl", Password: "Secret123!"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	code := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	if _, err := svc.VerifyMFA(context.Background(), identity.AdminSessionMFAInput{ChallengeID: start2.ChallengeID, Code: code}); !errors.Is(err, identity.ErrAdminMFAChallengeExpired) {
		t.Fatalf("expected ErrAdminMFAChallengeExpired, got %v", err)
	}

	// 4. Fresh challenge succeeds.
	start3, err := svc.Start(context.Background(), identity.AdminSessionStartInput{Email: "staff@espacigo.cl", Password: "Secret123!"})
	if err != nil {
		t.Fatal(err)
	}
	code = identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	authRes, err := svc.VerifyMFA(context.Background(), identity.AdminSessionMFAInput{ChallengeID: start3.ChallengeID, Code: code})
	if err != nil {
		t.Fatalf("VerifyMFA failed: %v", err)
	}
	if authRes.Status != "authenticated" || authRes.Token == "" || authRes.MFAVerifiedAt == nil || authRes.RecentAuthExpiresAt == nil {
		t.Fatalf("unexpected authRes: %+v", authRes)
	}

	// 5. Advance 15 minutes (MFA stale, within 30m idle) -> step-up MFA refreshes AuthenticatedAt.
	now = now.Add(15 * time.Minute)
	principal, err := svc.AuthorizeAdminCapability(context.Background(), authRes.Token, identity.CapabilityAccountsEmailReveal, identity.UserOperation)
	if err != nil {
		t.Fatalf("AuthorizeAdminCapability failed: %v", err)
	}
	if now.Sub(principal.AuthenticatedAt) <= 10*time.Minute {
		t.Fatalf("expected stale AuthenticatedAt, got %v at %v", principal.AuthenticatedAt, now)
	}

	stepUpCode := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	stepUpRes, err := svc.VerifyMFA(context.Background(), identity.AdminSessionMFAInput{AdminToken: authRes.Token, Code: stepUpCode})
	if err != nil {
		t.Fatalf("step-up VerifyMFA failed: %v", err)
	}
	if stepUpRes.MFAVerifiedAt == nil || !stepUpRes.MFAVerifiedAt.Equal(now) {
		t.Fatalf("expected refreshed MFAVerifiedAt=%v, got %v", now, stepUpRes.MFAVerifiedAt)
	}

	// 6. Revocation invalidates session immediately.
	if err := svc.RevokeSession(context.Background(), authRes.Token); err != nil {
		t.Fatalf("RevokeSession failed: %v", err)
	}
	if _, err := svc.GetSession(context.Background(), authRes.Token); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized after revocation, got %v", err)
	}
}
