package identity_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type passkeyTestRepo struct {
	accounts           map[string]string // accountID -> email
	displayNames       map[string]string // accountID -> displayName
	passkeys           map[string][]identity.PasskeyRecord
	challenges         map[string]*identity.WebAuthnChallengeRecord
	eventsRecorded     []identity.SecurityAlertEventRecord
	saveChallengeErr   error
	deletePasskeyErr   error
}

func newPasskeyTestRepo() *passkeyTestRepo {
	return &passkeyTestRepo{
		accounts:     make(map[string]string),
		displayNames: make(map[string]string),
		passkeys:     make(map[string][]identity.PasskeyRecord),
		challenges:   make(map[string]*identity.WebAuthnChallengeRecord),
	}
}

func (r *passkeyTestRepo) CreateWithTerms(context.Context, identity.Account, []identity.TermsAcceptance) error {
	return nil
}
func (r *passkeyTestRepo) TermsVersion(context.Context, string) (identity.TermsVersion, error) {
	return identity.TermsVersion{}, nil
}
func (r *passkeyTestRepo) WithLockedAccount(context.Context, identity.AccountLookup, func(identity.Account, identity.AuthenticationTransaction) error) error {
	return nil
}
func (r *passkeyTestRepo) PurgeExpiredPasswordHistory(context.Context, time.Time) error { return nil }
func (r *passkeyTestRepo) PurgeExpiredCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (r *passkeyTestRepo) CancelInactiveCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (r *passkeyTestRepo) ListTerminalCredentialNotices(context.Context) ([]identity.CredentialNoticeSummary, error) {
	return nil, nil
}
func (r *passkeyTestRepo) ReopenCredentialNotice(context.Context, identity.ReopenCredentialNoticeInput) (identity.CredentialNoticeCycle, error) {
	return identity.CredentialNoticeCycle{}, nil
}
func (r *passkeyTestRepo) ClaimCredentialNotice(context.Context, time.Time, time.Time) (identity.CredentialNotice, error) {
	return identity.CredentialNotice{}, nil
}
func (r *passkeyTestRepo) CancelInactiveCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (r *passkeyTestRepo) CompleteCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (r *passkeyTestRepo) RetryCredentialNotice(context.Context, string, time.Time, time.Time, time.Time, string) error {
	return nil
}

func (r *passkeyTestRepo) ActiveAccountEmail(_ context.Context, accountID string) (string, error) {
	email, ok := r.accounts[accountID]
	if !ok {
		return "", identity.ErrNotFound
	}
	return email, nil
}

func (r *passkeyTestRepo) ListActiveSessions(context.Context, string, time.Time) ([]identity.Session, error) {
	return nil, nil
}
func (r *passkeyTestRepo) RevokeSessionForAccount(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}
func (r *passkeyTestRepo) RevokeOtherSessionsForAccount(context.Context, string, string, time.Time) (int64, error) {
	return 0, nil
}

func (r *passkeyTestRepo) GetSecurityAlertPreferences(context.Context, string) (*identity.SecurityAlertPreferencesRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) UpsertSecurityAlertPreferences(context.Context, identity.SecurityAlertPreferencesRecord) error {
	return nil
}
func (r *passkeyTestRepo) ListSecurityAlertEvents(context.Context, string, int) ([]identity.SecurityAlertEventRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) RecordSecurityAlertEvent(_ context.Context, event identity.SecurityAlertEventRecord) error {
	r.eventsRecorded = append(r.eventsRecorded, event)
	return nil
}
func (r *passkeyTestRepo) GetPhoneVerification(context.Context, string) (*identity.AccountPhoneVerification, error) {
	return nil, nil
}

func (r *passkeyTestRepo) GetTOTP(context.Context, string) (*identity.AccountTOTPRecord, int, error) {
	return nil, 0, nil
}
func (r *passkeyTestRepo) SavePendingTOTPSecret(context.Context, string, []byte, string) error {
	return nil
}
func (r *passkeyTestRepo) ActivateTOTP(context.Context, string, int64, []string) error {
	return nil
}
func (r *passkeyTestRepo) UpdateTOTPLastVerifiedStep(context.Context, string, int64) error {
	return nil
}
func (r *passkeyTestRepo) ConsumeBackupCode(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *passkeyTestRepo) DisableTOTP(context.Context, string) error {
	return nil
}

func (r *passkeyTestRepo) ListPasskeys(_ context.Context, accountID string) ([]identity.PasskeyRecord, error) {
	return r.passkeys[accountID], nil
}

func (r *passkeyTestRepo) SaveWebAuthnChallenge(_ context.Context, challenge identity.WebAuthnChallengeRecord) error {
	if r.saveChallengeErr != nil {
		return r.saveChallengeErr
	}
	r.challenges[challenge.ID] = &challenge
	return nil
}

func (r *passkeyTestRepo) GetWebAuthnChallenge(_ context.Context, challengeID string) (*identity.WebAuthnChallengeRecord, error) {
	ch, ok := r.challenges[challengeID]
	if !ok {
		return nil, identity.ErrNotFound
	}
	return ch, nil
}

func (r *passkeyTestRepo) SavePasskeyAndConsumeChallenge(_ context.Context, passkey identity.PasskeyRecord, challengeID string, consumedAt time.Time) error {
	ch, ok := r.challenges[challengeID]
	if !ok {
		return identity.ErrPasskeyChallengeInvalid
	}
	if ch.ConsumedAt != nil {
		return identity.ErrPasskeyChallengeConsumed
	}
	ch.ConsumedAt = &consumedAt

	for _, pks := range r.passkeys {
		for _, pk := range pks {
			if pk.CredentialID == passkey.CredentialID {
				return identity.ErrPasskeyCredentialConflict
			}
		}
	}
	r.passkeys[passkey.AccountID] = append(r.passkeys[passkey.AccountID], passkey)
	return nil
}

func (r *passkeyTestRepo) DeletePasskey(_ context.Context, accountID, passkeyID string) error {
	if r.deletePasskeyErr != nil {
		return r.deletePasskeyErr
	}
	pks := r.passkeys[accountID]
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
	r.passkeys[accountID] = append(pks[:idx], pks[idx+1:]...)
	return nil
}

func (r *passkeyTestRepo) GetAccountDisplayName(_ context.Context, accountID string) (string, error) {
	return r.displayNames[accountID], nil
}
func (r *passkeyTestRepo) GetQRDevice(context.Context, string) (*identity.AccountQRDeviceRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) UpsertQRDevice(context.Context, string, time.Time, string) error {
	return nil
}
func (r *passkeyTestRepo) DisableQRDevice(context.Context, string, time.Time) error {
	return nil
}
func (r *passkeyTestRepo) SaveQRTicket(context.Context, identity.AccountQRTicketRecord) error {
	return nil
}
func (r *passkeyTestRepo) GetQRTicketByID(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) GetQRTicketByTokenHash(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) ApproveQRTicket(context.Context, string, string, string, time.Time) error {
	return nil
}
func (r *passkeyTestRepo) ExpireQRTicket(context.Context, string) error {
	return nil
}
func (r *passkeyTestRepo) GetLocationPermission(context.Context, string) (*identity.LocationPermissionRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) UpsertLocationPermission(context.Context, identity.LocationPermissionRecord) error {
	return nil
}
func (r *passkeyTestRepo) ListActiveTrustedContacts(context.Context, string) ([]identity.TrustedContactRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) CountActiveTrustedContacts(context.Context, string) (int, error) {
	return 0, nil
}
func (r *passkeyTestRepo) GetTrustedContact(context.Context, string, string) (*identity.TrustedContactRecord, error) {
	return nil, nil
}
func (r *passkeyTestRepo) InsertTrustedContact(context.Context, identity.TrustedContactRecord) error {
	return nil
}
func (r *passkeyTestRepo) RevokeTrustedContact(context.Context, string, string, time.Time) error {
	return nil
}

type testCredGen struct{ seq int }

func (g *testCredGen) ID() (string, error) {
	g.seq++
	return "test-id-" + string(rune('0'+g.seq)), nil
}
func (g *testCredGen) Token() (identity.Secret, error) {
	return identity.Secret("test-token"), nil
}

type testHasher struct{}

func (testHasher) Hash(s identity.Secret) (identity.Secret, error) { return s, nil }
func (testHasher) Matches(h, s identity.Secret) (bool, error)     { return h == s, nil }

type testMailer struct{}

func (testMailer) SendVerification(context.Context, identity.VerificationDelivery) error { return nil }
func (testMailer) SendRecovery(context.Context, identity.RecoveryDelivery) error         { return nil }
func (testMailer) SendPasswordChanged(context.Context, string) error                     { return nil }
func (testMailer) SendLoginAlert(context.Context, string, time.Time) error               { return nil }

type testLimiter struct{}

func (testLimiter) AllowVerification(context.Context, string, time.Time) (bool, error) { return true, nil }

func setupPasskeyService(t *testing.T, now time.Time) (*identity.AuthenticationService, *passkeyTestRepo) {
	t.Helper()
	repo := newPasskeyTestRepo()
	svc, err := identity.NewAuthenticationService(
		repo,
		testHasher{},
		testMailer{},
		testLimiter{},
		&testCredGen{},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}
	return svc, repo
}

func TestPasskey_ListEmpty(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	svc, _ := setupPasskeyService(t, now)

	res, err := svc.ListPasskeys(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("ListPasskeys failed: %v", err)
	}
	if len(res.Passkeys) != 0 {
		t.Fatalf("expected 0 passkeys, got %d", len(res.Passkeys))
	}

	_, err = svc.ListPasskeys(context.Background(), "")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty account, got %v", err)
	}
}

func TestPasskey_BeginRegistration(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	svc, repo := setupPasskeyService(t, now)

	accountID := "acc-1"
	repo.accounts[accountID] = "hernan.espinoza@ejemplo.cl"
	repo.displayNames[accountID] = "Hernán Espinoza"

	// Add an existing passkey to test excludeCredentials
	repo.passkeys[accountID] = []identity.PasskeyRecord{
		{
			ID:           "pk-existing",
			AccountID:    accountID,
			CredentialID: "cred-existing-id",
			Transports:   []string{"internal"},
		},
	}

	opts, err := svc.BeginPasskeyRegistration(context.Background(), accountID, "espacigo.com")
	if err != nil {
		t.Fatalf("BeginPasskeyRegistration failed: %v", err)
	}

	if opts.ChallengeID == "" {
		t.Errorf("expected non-empty challenge_id")
	}
	if len(opts.Challenge) == 0 {
		t.Errorf("expected non-empty challenge")
	}
	if opts.RP.ID != "espacigo.com" || opts.RP.Name != "EspaciGo" {
		t.Errorf("unexpected RP: %+v", opts.RP)
	}
	if opts.User.ID != accountID {
		t.Errorf("expected user.id %s, got %s", accountID, opts.User.ID)
	}
	if opts.User.Name != "h••••l" {
		t.Errorf("expected masked email 'h••••l', got %s", opts.User.Name)
	}
	if opts.User.DisplayName != "Hernán Espinoza" {
		t.Errorf("expected displayName 'Hernán Espinoza', got %s", opts.User.DisplayName)
	}
	if len(opts.PubKeyCredParams) != 2 || opts.PubKeyCredParams[0].Alg != -7 || opts.PubKeyCredParams[1].Alg != -257 {
		t.Errorf("unexpected pubKeyCredParams: %+v", opts.PubKeyCredParams)
	}
	if opts.Timeout != 60000 {
		t.Errorf("expected timeout 60000, got %d", opts.Timeout)
	}
	if len(opts.ExcludeCredentials) != 1 || opts.ExcludeCredentials[0].ID != "cred-existing-id" {
		t.Errorf("unexpected excludeCredentials: %+v", opts.ExcludeCredentials)
	}

	// Verify challenge was saved with TTL 5m and purpose 'register'
	savedCh, ok := repo.challenges[opts.ChallengeID]
	if !ok {
		t.Fatalf("challenge was not saved in repo")
	}
	if savedCh.AccountID != accountID || savedCh.Purpose != "register" {
		t.Errorf("unexpected stored challenge: %+v", savedCh)
	}
	if !savedCh.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Errorf("expected expiresAt %v, got %v", now.Add(5*time.Minute), savedCh.ExpiresAt)
	}
}

func TestPasskey_CompleteRegistration_Success(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	svc, repo := setupPasskeyService(t, now)

	accountID := "acc-1"
	repo.accounts[accountID] = "usuario@espacigo.com"

	opts, err := svc.BeginPasskeyRegistration(context.Background(), accountID, "localhost")
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	clientData := map[string]any{
		"type":        "webauthn.create",
		"challenge":   opts.Challenge,
		"origin":      "http://localhost:3000",
		"crossOrigin": false,
	}
	clientDataBytes, _ := json.Marshal(clientData)
	clientDataBase64 := base64.RawURLEncoding.EncodeToString(clientDataBytes)
	pubKeyBase64 := base64.RawURLEncoding.EncodeToString([]byte("dummy-spki-public-key-bytes-1234567890"))

	input := identity.PasskeyRegistrationVerifyInput{
		ChallengeID:    opts.ChallengeID,
		Label:          "Touch ID MacBook",
		CredentialID:   "cred-new-uuid-1",
		PublicKey:      pubKeyBase64,
		ClientDataJSON: clientDataBase64,
		Transports:     []string{"internal", "hybrid"},
		Algorithm:      -7,
	}

	passkey, err := svc.CompletePasskeyRegistration(context.Background(), accountID, input, []string{"http://localhost:3000"}, "MacBook Pro", "190.162.•••.•••")
	if err != nil {
		t.Fatalf("CompletePasskeyRegistration failed: %v", err)
	}

	if passkey.ID == "" {
		t.Errorf("expected non-empty passkey id")
	}
	if passkey.Label != "Touch ID MacBook" {
		t.Errorf("expected label 'Touch ID MacBook', got %s", passkey.Label)
	}
	if len(passkey.Transports) != 2 || passkey.Transports[0] != "internal" {
		t.Errorf("unexpected transports: %+v", passkey.Transports)
	}

	// Challenge must now be marked consumed
	savedCh := repo.challenges[opts.ChallengeID]
	if savedCh.ConsumedAt == nil {
		t.Errorf("challenge was not marked consumed")
	}

	// Audit event must be recorded
	if len(repo.eventsRecorded) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(repo.eventsRecorded))
	}
	ev := repo.eventsRecorded[0]
	if ev.EventCode != "passkey_registered" || ev.AccountID != accountID {
		t.Errorf("unexpected audit event: %+v", ev)
	}

	// Second attempt with same challenge must fail (consumed)
	_, err = svc.CompletePasskeyRegistration(context.Background(), accountID, input, []string{"http://localhost:3000"}, "MacBook Pro", "")
	if !errors.Is(err, identity.ErrPasskeyChallengeConsumed) {
		t.Errorf("expected ErrPasskeyChallengeConsumed on replay, got %v", err)
	}
}

func TestPasskey_CompleteRegistration_Validations(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	svc, repo := setupPasskeyService(t, now)

	accountID := "acc-1"
	repo.accounts[accountID] = "test@ejemplo.cl"

	opts, err := svc.BeginPasskeyRegistration(context.Background(), accountID, "localhost")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	pubKeyBase64 := base64.RawURLEncoding.EncodeToString([]byte("dummy-key-bytes-12345678901234567890"))

	// 1. Expired challenge (> 5 minutes)
	{
		lateSvc, _ := setupPasskeyService(t, now.Add(6*time.Minute))
		clientDataBytes, _ := json.Marshal(map[string]any{
			"type":      "webauthn.create",
			"challenge": opts.Challenge,
			"origin":    "http://localhost:3000",
		})
		input := identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			CredentialID:   "cred-1",
			PublicKey:      pubKeyBase64,
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		}
		// Put challenge in lateSvc's repo
		lateRepo := lateSvc
		_ = lateRepo
		// With svc where now is after expiration:
		svcExpired, _ := identity.NewAuthenticationService(
			repo, testHasher{}, testMailer{}, testLimiter{}, &testCredGen{},
			func() time.Time { return now.Add(6 * time.Minute) },
		)
		_, err := svcExpired.CompletePasskeyRegistration(context.Background(), accountID, input, nil, "", "")
		if !errors.Is(err, identity.ErrPasskeyChallengeExpired) {
			t.Errorf("expected ErrPasskeyChallengeExpired, got %v", err)
		}
	}

	// 2. Invalid clientDataJSON type (e.g. "webauthn.get")
	{
		clientDataBytes, _ := json.Marshal(map[string]any{
			"type":      "webauthn.get",
			"challenge": opts.Challenge,
			"origin":    "http://localhost:3000",
		})
		input := identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			CredentialID:   "cred-2",
			PublicKey:      pubKeyBase64,
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		}
		_, err := svc.CompletePasskeyRegistration(context.Background(), accountID, input, nil, "", "")
		if !errors.Is(err, identity.ErrPasskeyInvalidClientData) {
			t.Errorf("expected ErrPasskeyInvalidClientData for type webauthn.get, got %v", err)
		}
	}

	// 3. Challenge mismatch
	{
		clientDataBytes, _ := json.Marshal(map[string]any{
			"type":      "webauthn.create",
			"challenge": "different-challenge-value",
			"origin":    "http://localhost:3000",
		})
		input := identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			CredentialID:   "cred-3",
			PublicKey:      pubKeyBase64,
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		}
		_, err := svc.CompletePasskeyRegistration(context.Background(), accountID, input, nil, "", "")
		if !errors.Is(err, identity.ErrPasskeyChallengeInvalid) {
			t.Errorf("expected ErrPasskeyChallengeInvalid for challenge mismatch, got %v", err)
		}
	}

	// 4. Disallowed origin
	{
		clientDataBytes, _ := json.Marshal(map[string]any{
			"type":      "webauthn.create",
			"challenge": opts.Challenge,
			"origin":    "https://malicious-site.com",
		})
		input := identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			CredentialID:   "cred-4",
			PublicKey:      pubKeyBase64,
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		}
		_, err := svc.CompletePasskeyRegistration(context.Background(), accountID, input, []string{"https://espacigo.com"}, "", "")
		if !errors.Is(err, identity.ErrPasskeyInvalidClientData) {
			t.Errorf("expected ErrPasskeyInvalidClientData for disallowed origin, got %v", err)
		}
	}

	// 5. Allowed origin
	{
		clientDataBytes, _ := json.Marshal(map[string]any{
			"type":      "webauthn.create",
			"challenge": opts.Challenge,
			"origin":    "https://espacigo.com",
		})
		input := identity.PasskeyRegistrationVerifyInput{
			ChallengeID:    opts.ChallengeID,
			CredentialID:   "cred-5",
			PublicKey:      pubKeyBase64,
			ClientDataJSON: base64.RawURLEncoding.EncodeToString(clientDataBytes),
		}
		pk, err := svc.CompletePasskeyRegistration(context.Background(), accountID, input, []string{"https://espacigo.com"}, "", "")
		if err != nil {
			t.Fatalf("expected success for allowed origin, got %v", err)
		}
		if pk.Label != "Llave de acceso" {
			t.Errorf("expected default label 'Llave de acceso', got %s", pk.Label)
		}
	}
}

func TestPasskey_Delete(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	svc, repo := setupPasskeyService(t, now)

	accountID := "acc-1"
	passkeyID := "pk-101"
	repo.passkeys[accountID] = []identity.PasskeyRecord{
		{
			ID:           passkeyID,
			AccountID:    accountID,
			CredentialID: "cred-101",
			Label:        "Mi llave",
		},
	}

	// 1. Delete existing passkey
	err := svc.DeletePasskey(context.Background(), accountID, passkeyID, "Navegador", "190.162.•••.•••")
	if err != nil {
		t.Fatalf("DeletePasskey failed: %v", err)
	}

	if len(repo.passkeys[accountID]) != 0 {
		t.Errorf("expected passkey to be deleted from repo")
	}

	if len(repo.eventsRecorded) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(repo.eventsRecorded))
	}
	if repo.eventsRecorded[0].EventCode != "passkey_revoked" {
		t.Errorf("expected passkey_revoked audit event, got %s", repo.eventsRecorded[0].EventCode)
	}

	// 2. Delete non-existent passkey returns ErrPasskeyNotFound
	err = svc.DeletePasskey(context.Background(), accountID, passkeyID, "Navegador", "")
	if !errors.Is(err, identity.ErrPasskeyNotFound) {
		t.Errorf("expected ErrPasskeyNotFound, got %v", err)
	}
}
