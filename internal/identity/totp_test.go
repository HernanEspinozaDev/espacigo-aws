package identity_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

// RFC 6238 Appendix B test vectors for HMAC-SHA1
func TestTOTP_RFC6238_TestVectors(t *testing.T) {
	// Secret = "12345678901234567890" (20 bytes ASCII)
	secret := []byte("12345678901234567890")

	// Steps from RFC 6238 Appendix B (HMAC-SHA1 8-digit and 6-digit)
	// Time: 59s -> step = 1 -> RFC SHA1 8-digit: 94287082 -> 6-digit: 287082
	code := identity.GenerateTOTPCode(secret, 1, 6)
	if code != "287082" {
		t.Fatalf("step 1 expected 287082, got %s", code)
	}

	// Time: 1111111109s -> step 37037036 -> RFC SHA1 8-digit: 07081804 -> 6-digit: 081804
	code = identity.GenerateTOTPCode(secret, 37037036, 6)
	if code != "081804" {
		t.Fatalf("step 37037036 expected 081804, got %s", code)
	}

	// Time: 1111111111s -> step 37037037 -> RFC SHA1 8-digit: 14050471 -> 6-digit: 050471
	code = identity.GenerateTOTPCode(secret, 37037037, 6)
	if code != "050471" {
		t.Fatalf("step 37037037 expected 050471, got %s", code)
	}
}

func TestTOTP_SecretAndURI(t *testing.T) {
	secret, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if len(secret) != 20 {
		t.Fatalf("expected 20 bytes secret, got %d", len(secret))
	}

	encoded := identity.EncodeBase32Secret(secret)
	decoded, err := identity.DecodeBase32Secret(encoded)
	if err != nil {
		t.Fatalf("DecodeBase32Secret: %v", err)
	}
	if string(decoded) != string(secret) {
		t.Fatal("decoded secret does not match original")
	}

	uri := identity.BuildOTPAuthURI("test@ejemplo.cl", encoded)
	if !strings.HasPrefix(uri, "otpauth://totp/EspaciGo:test@ejemplo.cl?") {
		t.Fatalf("unexpected otpauth URI: %s", uri)
	}
	if !strings.Contains(uri, "secret="+encoded) || !strings.Contains(uri, "issuer=EspaciGo") {
		t.Fatalf("URI missing required params: %s", uri)
	}
}

func TestTOTP_ValidateWindowAndAntiReplay(t *testing.T) {
	secret, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}

	now := time.Unix(1700000000, 0)
	currentStep := now.Unix() / 30

	// Code at current step
	codeCurrent := identity.GenerateTOTPCode(secret, currentStep, 6)
	// Code at step - 1
	codePrev := identity.GenerateTOTPCode(secret, currentStep-1, 6)
	// Code at step + 1
	codeNext := identity.GenerateTOTPCode(secret, currentStep+1, 6)
	// Code at step - 2 (outside window)
	codeTooOld := identity.GenerateTOTPCode(secret, currentStep-2, 6)
	// Code at step + 2 (outside window)
	codeTooNew := identity.GenerateTOTPCode(secret, currentStep+2, 6)

	// Current step valid
	step, err := identity.ValidateTOTP(secret, codeCurrent, now, nil)
	if err != nil || step != currentStep {
		t.Fatalf("ValidateTOTP current step failed: %v", err)
	}

	// Step - 1 valid
	step, err = identity.ValidateTOTP(secret, codePrev, now, nil)
	if err != nil || step != currentStep-1 {
		t.Fatalf("ValidateTOTP prev step failed: %v", err)
	}

	// Step + 1 valid
	step, err = identity.ValidateTOTP(secret, codeNext, now, nil)
	if err != nil || step != currentStep+1 {
		t.Fatalf("ValidateTOTP next step failed: %v", err)
	}

	// Step - 2 rejected
	_, err = identity.ValidateTOTP(secret, codeTooOld, now, nil)
	if !errors.Is(err, identity.ErrTOTPInvalidCode) {
		t.Fatalf("expected ErrTOTPInvalidCode for too old code, got %v", err)
	}

	// Step + 2 rejected
	_, err = identity.ValidateTOTP(secret, codeTooNew, now, nil)
	if !errors.Is(err, identity.ErrTOTPInvalidCode) {
		t.Fatalf("expected ErrTOTPInvalidCode for too new code, got %v", err)
	}

	// Anti-replay test
	lastVerified := currentStep
	// Trying to use current step again must be rejected as replay
	_, err = identity.ValidateTOTP(secret, codeCurrent, now, &lastVerified)
	if !errors.Is(err, identity.ErrTOTPReplayDetected) {
		t.Fatalf("expected ErrTOTPReplayDetected for same step, got %v", err)
	}

	// Trying to use step - 1 when lastVerified is currentStep must be rejected
	_, err = identity.ValidateTOTP(secret, codePrev, now, &lastVerified)
	if !errors.Is(err, identity.ErrTOTPReplayDetected) {
		t.Fatalf("expected ErrTOTPReplayDetected for older step, got %v", err)
	}

	// Next step must be accepted
	step, err = identity.ValidateTOTP(secret, codeNext, now, &lastVerified)
	if err != nil || step != currentStep+1 {
		t.Fatalf("expected next step to succeed, got step=%d err=%v", step, err)
	}
}

func TestTOTP_BackupCodes(t *testing.T) {
	codes, hashes, err := identity.GenerateBackupCodes(8)
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if len(codes) != 8 || len(hashes) != 8 {
		t.Fatalf("expected 8 codes and hashes, got %d and %d", len(codes), len(hashes))
	}

	for i, code := range codes {
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("invalid code format: %s", code)
		}
		expectedHash := identity.HashBackupCode(code)
		if hashes[i] != expectedHash {
			t.Fatalf("hash mismatch for code %s: %s != %s", code, hashes[i], expectedHash)
		}

		// Test normalization without hyphen or in lowercase
		noHyphen := strings.ReplaceAll(code, "-", "")
		if identity.HashBackupCode(noHyphen) != expectedHash {
			t.Fatalf("normalization failed for code %s", noHyphen)
		}
		lower := strings.ToLower(code)
		if identity.HashBackupCode(lower) != expectedHash {
			t.Fatalf("normalization failed for lowercase code %s", lower)
		}
	}
}

func TestTOTPKeyring_SealAndOpen(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	kr, err := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("NewTOTPKeyring: %v", err)
	}

	secret := []byte("01234567890123456789")
	version, ciphertext, err := kr.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if version != "v1" {
		t.Fatalf("expected version v1, got %s", version)
	}

	opened, err := kr.Open(version, ciphertext)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(opened) != string(secret) {
		t.Fatalf("decrypted secret does not match original")
	}

	// Tampered ciphertext fails
	corrupted := append([]byte(nil), ciphertext...)
	corrupted[len(corrupted)-1] ^= 0xFF
	if _, err := kr.Open(version, corrupted); err == nil {
		t.Fatal("expected decryption error on tampered ciphertext")
	}

	// Unknown version fails
	if _, err := kr.Open("v2", ciphertext); err == nil {
		t.Fatal("expected error on unknown key version")
	}
}

func TestTOTPKeyring_ProductionRequirement(t *testing.T) {
	// In production, missing keyring returns error
	getenv := func(key string) string {
		if key == "APP_PROFILE" {
			return "production"
		}
		return ""
	}
	_, err := identity.LoadTOTPKeyring(getenv)
	if err == nil {
		t.Fatal("expected error in production when keyring is not configured")
	}
}

func TestTOTPKeyring_DevelopmentFallback(t *testing.T) {
	// In development, fallback returns valid working keyring
	getenv := func(key string) string {
		return ""
	}
	kr, err := identity.LoadTOTPKeyring(getenv)
	if err != nil {
		t.Fatalf("expected working fallback keyring in development, got: %v", err)
	}
	ver, ct, err := kr.Seal([]byte("test-secret"))
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}
	dec, err := kr.Open(ver, ct)
	if err != nil || string(dec) != "test-secret" {
		t.Fatalf("Open failed: %v", err)
	}
}

func TestTOTPKeyring_FileLoading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keyring.json")
	content := `{
		"current": "v1",
		"keys": {
			"v1": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	kr, err := identity.LoadTOTPKeyringFromFile(path)
	if err != nil {
		t.Fatalf("LoadTOTPKeyringFromFile: %v", err)
	}
	if kr == nil {
		t.Fatal("expected non-nil keyring")
	}
}

type mockTOTPRepo struct {
	mockAlertsRepo
	totpRecord     *identity.AccountTOTPRecord
	remainingCodes int
	pendingCipher  []byte
	pendingVersion string
	lastStep       *int64
	backupHashes   map[string]bool
	eventsRecorded []identity.SecurityAlertEventRecord
}

func (m *mockTOTPRepo) GetTOTP(ctx context.Context, accountID string) (*identity.AccountTOTPRecord, int, error) {
	return m.totpRecord, m.remainingCodes, nil
}

func (m *mockTOTPRepo) SavePendingTOTPSecret(ctx context.Context, accountID string, ciphertext []byte, keyVersion string) error {
	m.pendingCipher = ciphertext
	m.pendingVersion = keyVersion
	m.totpRecord = &identity.AccountTOTPRecord{
		AccountID:               accountID,
		PendingSecretCiphertext: ciphertext,
		PendingKeyVersion:      &m.pendingVersion,
	}
	return nil
}

func (m *mockTOTPRepo) ActivateTOTP(ctx context.Context, accountID string, verifiedStep int64, backupCodeHashes []string) error {
	if m.totpRecord == nil || len(m.totpRecord.PendingSecretCiphertext) == 0 {
		return identity.ErrTOTPNotPending
	}
	now := time.Now()
	m.lastStep = &verifiedStep
	m.totpRecord.SecretCiphertext = m.totpRecord.PendingSecretCiphertext
	m.totpRecord.KeyVersion = m.totpRecord.PendingKeyVersion
	m.totpRecord.PendingSecretCiphertext = nil
	m.totpRecord.PendingKeyVersion = nil
	m.totpRecord.EnabledAt = &now
	m.totpRecord.LastVerifiedStep = &verifiedStep

	m.backupHashes = make(map[string]bool)
	for _, h := range backupCodeHashes {
		m.backupHashes[h] = true
	}
	m.remainingCodes = len(backupCodeHashes)
	return nil
}

func (m *mockTOTPRepo) UpdateTOTPLastVerifiedStep(ctx context.Context, accountID string, verifiedStep int64) error {
	m.lastStep = &verifiedStep
	if m.totpRecord != nil {
		m.totpRecord.LastVerifiedStep = &verifiedStep
	}
	return nil
}

func (m *mockTOTPRepo) ConsumeBackupCode(ctx context.Context, accountID string, codeHash string) (bool, error) {
	if m.backupHashes != nil && m.backupHashes[codeHash] {
		delete(m.backupHashes, codeHash)
		m.remainingCodes--
		return true, nil
	}
	return false, nil
}

func (m *mockTOTPRepo) DisableTOTP(ctx context.Context, accountID string) error {
	if m.totpRecord != nil {
		m.totpRecord.SecretCiphertext = nil
		m.totpRecord.KeyVersion = nil
		m.totpRecord.PendingSecretCiphertext = nil
		m.totpRecord.PendingKeyVersion = nil
		m.totpRecord.EnabledAt = nil
		m.totpRecord.LastVerifiedStep = nil
	}
	m.backupHashes = nil
	m.remainingCodes = 0
	return nil
}

func (m *mockTOTPRepo) RecordSecurityAlertEvent(ctx context.Context, event identity.SecurityAlertEventRecord) error {
	m.eventsRecorded = append(m.eventsRecorded, event)
	return nil
}

func (m *mockTOTPRepo) ActiveAccountEmail(ctx context.Context, accountID string) (string, error) {
	return "usuario@ejemplo.cl", nil
}

func (m *mockTOTPRepo) ListPasskeys(context.Context, string) ([]identity.PasskeyRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) SaveWebAuthnChallenge(context.Context, identity.WebAuthnChallengeRecord) error {
	return nil
}
func (m *mockTOTPRepo) GetWebAuthnChallenge(context.Context, string) (*identity.WebAuthnChallengeRecord, error) {
	return nil, identity.ErrNotFound
}
func (m *mockTOTPRepo) SavePasskeyAndConsumeChallenge(context.Context, identity.PasskeyRecord, string, time.Time) error {
	return nil
}
func (m *mockTOTPRepo) DeletePasskey(context.Context, string, string) error {
	return nil
}
func (m *mockTOTPRepo) GetAccountDisplayName(context.Context, string) (string, error) {
	return "", nil
}
func (m *mockTOTPRepo) GetQRDevice(context.Context, string) (*identity.AccountQRDeviceRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) UpsertQRDevice(context.Context, string, time.Time, string) error {
	return nil
}
func (m *mockTOTPRepo) DisableQRDevice(context.Context, string, time.Time) error {
	return nil
}
func (m *mockTOTPRepo) SaveQRTicket(context.Context, identity.AccountQRTicketRecord) error {
	return nil
}
func (m *mockTOTPRepo) GetQRTicketByID(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) GetQRTicketByTokenHash(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) ApproveQRTicket(context.Context, string, string, string, time.Time) error {
	return nil
}
func (m *mockTOTPRepo) ExpireQRTicket(context.Context, string) error {
	return nil
}
func (m *mockTOTPRepo) GetLocationPermission(context.Context, string) (*identity.LocationPermissionRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) UpsertLocationPermission(context.Context, identity.LocationPermissionRecord) error {
	return nil
}
func (m *mockTOTPRepo) ListActiveTrustedContacts(context.Context, string) ([]identity.TrustedContactRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) CountActiveTrustedContacts(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockTOTPRepo) GetTrustedContact(context.Context, string, string) (*identity.TrustedContactRecord, error) {
	return nil, nil
}
func (m *mockTOTPRepo) InsertTrustedContact(context.Context, identity.TrustedContactRecord) error {
	return nil
}
func (m *mockTOTPRepo) RevokeTrustedContact(context.Context, string, string, time.Time) error {
	return nil
}

func TestTOTPService_Lifecycle(t *testing.T) {
	repo := &mockTOTPRepo{}
	hasher := alertsTestHasher{}
	mailer := alertsTestMailer{}
	limiter := alertsTestLimiter{}
	credGen := &alertsTestCredGen{}
	fixedTime := time.Unix(1700000000, 0)
	nowFunc := func() time.Time { return fixedTime }

	svc, err := identity.NewAuthenticationService(repo, hasher, mailer, limiter, credGen, nowFunc)
	if err != nil {
		t.Fatalf("NewAuthenticationService: %v", err)
	}

	key := make([]byte, 32)
	kr, _ := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": key})
	svc.SetTOTPKeyring(kr)

	ctx := context.Background()
	accountID := "acc-totp-123"

	// 1. Initial state: not enabled
	status, err := svc.GetTOTPStatus(ctx, accountID)
	if err != nil {
		t.Fatalf("GetTOTPStatus: %v", err)
	}
	if status.Enabled || status.PendingSetup || status.RemainingBackupCodes != 0 {
		t.Fatalf("expected disabled TOTP, got %+v", status)
	}

	// 2. Setup
	setupResp, err := svc.SetupTOTP(ctx, accountID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if setupResp.ManualKey == "" || !strings.Contains(setupResp.OTPAuthURI, "otpauth://") {
		t.Fatalf("invalid setup response: %+v", setupResp)
	}

	status, err = svc.GetTOTPStatus(ctx, accountID)
	if err != nil || !status.PendingSetup || status.Enabled {
		t.Fatalf("expected pending setup, got %+v", status)
	}

	// 3. Enable with invalid code fails
	_, err = svc.EnableTOTP(ctx, accountID, "000000", "MacBook", "190.162.1.1")
	if !errors.Is(err, identity.ErrTOTPInvalidCode) {
		t.Fatalf("expected ErrTOTPInvalidCode, got %v", err)
	}

	// 4. Enable with valid code
	secret, err := identity.DecodeBase32Secret(setupResp.ManualKey)
	if err != nil {
		t.Fatalf("DecodeBase32Secret: %v", err)
	}
	currentStep := fixedTime.Unix() / 30
	validCode := identity.GenerateTOTPCode(secret, currentStep, 6)

	enableResp, err := svc.EnableTOTP(ctx, accountID, validCode, "MacBook", "190.162.1.1")
	if err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}
	if !enableResp.Enabled || len(enableResp.BackupCodes) != 8 {
		t.Fatalf("invalid enable response: %+v", enableResp)
	}

	// Check alert event recorded
	var foundEnabledEvent bool
	for _, ev := range repo.eventsRecorded {
		if ev.EventCode == "totp_enabled" {
			foundEnabledEvent = true
			break
		}
	}
	if !foundEnabledEvent {
		t.Fatal("expected totp_enabled alert event")
	}

	// Status should now be enabled
	status, err = svc.GetTOTPStatus(ctx, accountID)
	if err != nil || !status.Enabled || status.PendingSetup || status.RemainingBackupCodes != 8 {
		t.Fatalf("expected enabled TOTP with 8 backup codes, got %+v", status)
	}

	// 5. Anti-replay: replaying same code fails
	fixedTime = fixedTime.Add(5 * time.Second) // still in same window
	_, err = svc.DisableTOTP(ctx, accountID, validCode, "MacBook", "190.162.1.1")
	if !errors.Is(err, identity.ErrTOTPInvalidCode) {
		t.Fatalf("expected ErrTOTPInvalidCode on replayed code, got %v", err)
	}

	// 6. Disable using backup code
	backupCode := enableResp.BackupCodes[0]
	disableResp, err := svc.DisableTOTP(ctx, accountID, backupCode, "MacBook", "190.162.1.1")
	if err != nil {
		t.Fatalf("DisableTOTP with backup code: %v", err)
	}
	if disableResp.Enabled {
		t.Fatal("expected disabled TOTP")
	}

	// Check alert event totp_disabled recorded
	var foundDisabledEvent bool
	for _, ev := range repo.eventsRecorded {
		if ev.EventCode == "totp_disabled" {
			foundDisabledEvent = true
			break
		}
	}
	if !foundDisabledEvent {
		t.Fatal("expected totp_disabled alert event")
	}

	// Status should now be disabled
	status, err = svc.GetTOTPStatus(ctx, accountID)
	if err != nil || status.Enabled || status.PendingSetup || status.RemainingBackupCodes != 0 {
		t.Fatalf("expected disabled TOTP after disable, got %+v", status)
	}
}
