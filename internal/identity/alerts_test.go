package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type mockAlertsRepo struct {
	email         string
	prefRecord    *identity.SecurityAlertPreferencesRecord
	events        []identity.SecurityAlertEventRecord
	phoneRecord   *identity.AccountPhoneVerification
	upsertPrefErr error
	recordEvtErr  error
	listEvtErr    error
}

func (m *mockAlertsRepo) CreateWithTerms(context.Context, identity.Account, []identity.TermsAcceptance) error {
	return nil
}
func (m *mockAlertsRepo) TermsVersion(context.Context, string) (identity.TermsVersion, error) {
	return identity.TermsVersion{}, nil
}
func (m *mockAlertsRepo) WithLockedAccount(context.Context, identity.AccountLookup, func(identity.Account, identity.AuthenticationTransaction) error) error {
	return nil
}
func (m *mockAlertsRepo) ListActiveSessions(context.Context, string, time.Time) ([]identity.Session, error) {
	return nil, nil
}
func (m *mockAlertsRepo) RevokeSessionForAccount(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}
func (m *mockAlertsRepo) RevokeOtherSessionsForAccount(context.Context, string, string, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAlertsRepo) PurgeExpiredPasswordHistory(context.Context, time.Time) error { return nil }
func (m *mockAlertsRepo) PurgeExpiredCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (m *mockAlertsRepo) CancelInactiveCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (m *mockAlertsRepo) ListTerminalCredentialNotices(context.Context) ([]identity.CredentialNoticeSummary, error) {
	return nil, nil
}
func (m *mockAlertsRepo) ReopenCredentialNotice(context.Context, identity.ReopenCredentialNoticeInput) (identity.CredentialNoticeCycle, error) {
	return identity.CredentialNoticeCycle{}, nil
}
func (m *mockAlertsRepo) ClaimCredentialNotice(context.Context, time.Time, time.Time) (identity.CredentialNotice, error) {
	return identity.CredentialNotice{}, nil
}
func (m *mockAlertsRepo) ActiveAccountEmail(ctx context.Context, accountID string) (string, error) {
	if m.email == "" {
		return "", identity.ErrNotFound
	}
	return m.email, nil
}
func (m *mockAlertsRepo) CancelInactiveCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (m *mockAlertsRepo) CompleteCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (m *mockAlertsRepo) RetryCredentialNotice(context.Context, string, time.Time, time.Time, time.Time, string) error {
	return nil
}
func (m *mockAlertsRepo) GetSecurityAlertPreferences(ctx context.Context, accountID string) (*identity.SecurityAlertPreferencesRecord, error) {
	return m.prefRecord, nil
}
func (m *mockAlertsRepo) UpsertSecurityAlertPreferences(ctx context.Context, record identity.SecurityAlertPreferencesRecord) error {
	if m.upsertPrefErr != nil {
		return m.upsertPrefErr
	}
	m.prefRecord = &record
	return nil
}
func (m *mockAlertsRepo) ListSecurityAlertEvents(ctx context.Context, accountID string, limit int) ([]identity.SecurityAlertEventRecord, error) {
	if m.listEvtErr != nil {
		return nil, m.listEvtErr
	}
	if limit > 0 && len(m.events) > limit {
		return m.events[:limit], nil
	}
	return m.events, nil
}
func (m *mockAlertsRepo) RecordSecurityAlertEvent(ctx context.Context, event identity.SecurityAlertEventRecord) error {
	if m.recordEvtErr != nil {
		return m.recordEvtErr
	}
	m.events = append([]identity.SecurityAlertEventRecord{event}, m.events...)
	return nil
}
func (m *mockAlertsRepo) GetPhoneVerification(ctx context.Context, accountID string) (*identity.AccountPhoneVerification, error) {
	return m.phoneRecord, nil
}
func (m *mockAlertsRepo) GetTOTP(context.Context, string) (*identity.AccountTOTPRecord, int, error) {
	return nil, 0, nil
}
func (m *mockAlertsRepo) SavePendingTOTPSecret(context.Context, string, []byte, string) error {
	return nil
}
func (m *mockAlertsRepo) ActivateTOTP(context.Context, string, int64, []string) error {
	return nil
}
func (m *mockAlertsRepo) UpdateTOTPLastVerifiedStep(context.Context, string, int64) error {
	return nil
}
func (m *mockAlertsRepo) ConsumeBackupCode(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *mockAlertsRepo) DisableTOTP(context.Context, string) error {
	return nil
}
func (m *mockAlertsRepo) ListPasskeys(context.Context, string) ([]identity.PasskeyRecord, error) {
	return nil, nil
}
func (m *mockAlertsRepo) SaveWebAuthnChallenge(context.Context, identity.WebAuthnChallengeRecord) error {
	return nil
}
func (m *mockAlertsRepo) GetWebAuthnChallenge(context.Context, string) (*identity.WebAuthnChallengeRecord, error) {
	return nil, identity.ErrNotFound
}
func (m *mockAlertsRepo) SavePasskeyAndConsumeChallenge(context.Context, identity.PasskeyRecord, string, time.Time) error {
	return nil
}
func (m *mockAlertsRepo) DeletePasskey(context.Context, string, string) error {
	return nil
}
func (m *mockAlertsRepo) GetAccountDisplayName(context.Context, string) (string, error) {
	return "", nil
}
func (m *mockAlertsRepo) GetQRDevice(context.Context, string) (*identity.AccountQRDeviceRecord, error) {
	return nil, nil
}
func (m *mockAlertsRepo) UpsertQRDevice(context.Context, string, time.Time, string) error {
	return nil
}
func (m *mockAlertsRepo) DisableQRDevice(context.Context, string, time.Time) error {
	return nil
}
func (m *mockAlertsRepo) SaveQRTicket(context.Context, identity.AccountQRTicketRecord) error {
	return nil
}
func (m *mockAlertsRepo) GetQRTicketByID(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (m *mockAlertsRepo) GetQRTicketByTokenHash(context.Context, string) (*identity.AccountQRTicketRecord, error) {
	return nil, nil
}
func (m *mockAlertsRepo) ApproveQRTicket(context.Context, string, string, string, time.Time) error {
	return nil
}
func (m *mockAlertsRepo) ExpireQRTicket(context.Context, string) error {
	return nil
}

type alertsTestHasher struct{}

func (alertsTestHasher) Hash(s identity.Secret) (identity.Secret, error) { return s, nil }
func (alertsTestHasher) Matches(h, s identity.Secret) (bool, error)     { return h == s, nil }

type alertsTestMailer struct{}

func (alertsTestMailer) SendVerification(context.Context, identity.VerificationDelivery) error {
	return nil
}
func (alertsTestMailer) SendRecovery(context.Context, identity.RecoveryDelivery) error { return nil }
func (alertsTestMailer) SendPasswordChanged(context.Context, string) error             { return nil }
func (alertsTestMailer) SendLoginAlert(context.Context, string, time.Time) error       { return nil }

type alertsTestLimiter struct{}

func (alertsTestLimiter) AllowVerification(context.Context, string, time.Time) (bool, error) {
	return true, nil
}

type alertsTestCredGen struct {
	nextID string
}

func (c *alertsTestCredGen) ID() (string, error) {
	if c.nextID != "" {
		id := c.nextID
		c.nextID = ""
		return id, nil
	}
	return "00000000-0000-4000-8000-000000000001", nil
}
func (c *alertsTestCredGen) Token() (identity.Secret, error) {
	return identity.Secret("test-token"), nil
}

func setupAlertsService(repo *mockAlertsRepo) (*identity.AuthenticationService, func()) {
	credGen := &alertsTestCredGen{}
	fixedTime := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	nowFunc := func() time.Time { return fixedTime }
	svc, err := identity.NewAuthenticationService(repo, alertsTestHasher{}, alertsTestMailer{}, alertsTestLimiter{}, credGen, nowFunc)
	if err != nil {
		panic(err)
	}
	return svc, func() {}
}

func TestGetSecurityAlerts_DefaultsAndMasking(t *testing.T) {
	ctx := context.Background()
	repo := &mockAlertsRepo{
		email: "usuario.principal@empresa.cl",
	}
	svc, _ := setupAlertsService(repo)

	// When no preferences record exists, defaults should be:
	// app_notifications_enabled: true, email_alerts_enabled: true, whatsapp_alerts_enabled: false
	res, err := svc.GetSecurityAlerts(ctx, "acc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Preferences.AppNotificationsEnabled {
		t.Error("expected AppNotificationsEnabled to default to true")
	}
	if !res.Preferences.EmailAlertsEnabled {
		t.Error("expected EmailAlertsEnabled to default to true")
	}
	if res.Preferences.WhatsAppAlertsEnabled {
		t.Error("expected WhatsAppAlertsEnabled to default to false")
	}
	if res.Preferences.ExtraEmailConfigured {
		t.Error("expected ExtraEmailConfigured to default to false")
	}
	if res.Preferences.MaskedExtraEmail != "" {
		t.Errorf("expected empty MaskedExtraEmail, got %s", res.Preferences.MaskedExtraEmail)
	}
	if res.Preferences.MaskedPhone != "" {
		t.Errorf("expected empty MaskedPhone when unverified, got %s", res.Preferences.MaskedPhone)
	}

	// Verify primary email is masked and doesn't reveal raw address
	if res.Preferences.MaskedEmail == "usuario.principal@empresa.cl" || !strings.Contains(res.Preferences.MaskedEmail, "••••") {
		t.Errorf("expected masked email, got %s", res.Preferences.MaskedEmail)
	}
	if len(res.Alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(res.Alerts))
	}
}

func TestGetSecurityAlerts_WithConfiguredPreferencesAndEvents(t *testing.T) {
	ctx := context.Background()
	extra := "seguridad.backup@empresa.cl"
	phoneVerified := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	repo := &mockAlertsRepo{
		email: "titular@empresa.cl",
		prefRecord: &identity.SecurityAlertPreferencesRecord{
			AccountID:               "acc-123",
			AppNotificationsEnabled: false,
			EmailAlertsEnabled:      true,
			WhatsAppAlertsEnabled:   true,
			ExtraEmail:              &extra,
			UpdatedAt:               time.Now().UTC(),
		},
		phoneRecord: &identity.AccountPhoneVerification{
			AccountID:  "acc-123",
			PhoneE164:  "+56912345678",
			VerifiedAt: &phoneVerified,
		},
		events: []identity.SecurityAlertEventRecord{
			{
				ID:          "ev-1",
				AccountID:   "acc-123",
				EventCode:   "alert_channel_updated",
				Title:       "Canales de alerta actualizados",
				DeviceLabel: "Chrome en Linux",
				IPMasked:    "190.162.12.45", // raw IP should get masked
				CreatedAt:   time.Now().UTC(),
			},
		},
	}
	svc, _ := setupAlertsService(repo)

	res, err := svc.GetSecurityAlerts(ctx, "acc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Preferences.AppNotificationsEnabled {
		t.Error("expected AppNotificationsEnabled to be false")
	}
	if !res.Preferences.EmailAlertsEnabled {
		t.Error("expected EmailAlertsEnabled to be true")
	}
	if !res.Preferences.WhatsAppAlertsEnabled {
		t.Error("expected WhatsAppAlertsEnabled to be true")
	}
	if !res.Preferences.ExtraEmailConfigured {
		t.Error("expected ExtraEmailConfigured to be true")
	}
	if res.Preferences.MaskedExtraEmail == extra || !strings.Contains(res.Preferences.MaskedExtraEmail, "••••") {
		t.Errorf("expected masked extra email, got %s", res.Preferences.MaskedExtraEmail)
	}
	if res.Preferences.MaskedPhone != "+56 ••••5678" {
		t.Errorf("expected masked phone '+56 ••••5678', got %q", res.Preferences.MaskedPhone)
	}

	// Verify event IP is masked
	if len(res.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(res.Alerts))
	}
	if res.Alerts[0].IPMasked == "190.162.12.45" || !strings.Contains(res.Alerts[0].IPMasked, "••") {
		t.Errorf("expected masked IP in event, got %s", res.Alerts[0].IPMasked)
	}
}

func TestGetSecurityAlerts_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := &mockAlertsRepo{email: ""}
	svc, _ := setupAlertsService(repo)

	_, err := svc.GetSecurityAlerts(ctx, "")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("expected ErrNotFound for empty accountID, got %v", err)
	}

	_, err = svc.GetSecurityAlerts(ctx, "nonexistent")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("expected ErrNotFound for nonexistent account, got %v", err)
	}
}

func TestUpdateSecurityAlertChannels_ValidationAndAuditing(t *testing.T) {
	ctx := context.Background()
	repo := &mockAlertsRepo{
		email: "titular@empresa.cl",
	}
	svc, _ := setupAlertsService(repo)

	// 1. Invalid extra email format
	badEmail := "not-an-email"
	_, err := svc.UpdateSecurityAlertChannels(ctx, "acc-123", identity.UpdateSecurityAlertChannelsInput{
		AppNotificationsEnabled: true,
		EmailAlertsEnabled:      true,
		WhatsAppAlertsEnabled:   false,
		ExtraEmail:              &badEmail,
	})
	if !errors.Is(err, identity.ErrInvalidExtraEmail) {
		t.Errorf("expected ErrInvalidExtraEmail for bad email, got %v", err)
	}

	// 2. Extra email matching primary email
	sameEmail := "  TITULAR@empresa.cl  "
	_, err = svc.UpdateSecurityAlertChannels(ctx, "acc-123", identity.UpdateSecurityAlertChannelsInput{
		AppNotificationsEnabled: true,
		EmailAlertsEnabled:      true,
		WhatsAppAlertsEnabled:   false,
		ExtraEmail:              &sameEmail,
	})
	if !errors.Is(err, identity.ErrExtraEmailMatchesPrimary) {
		t.Errorf("expected ErrExtraEmailMatchesPrimary for same email, got %v", err)
	}

	// 3. Valid update with extra email
	validExtra := "seguridad@empresa.cl"
	prefs, err := svc.UpdateSecurityAlertChannels(ctx, "acc-123", identity.UpdateSecurityAlertChannelsInput{
		AppNotificationsEnabled: false,
		EmailAlertsEnabled:      true,
		WhatsAppAlertsEnabled:   true,
		ExtraEmail:              &validExtra,
		DeviceLabel:             "Firefox en Windows",
		IPMasked:                "190.162.•••.•••",
	})
	if err != nil {
		t.Fatalf("unexpected error on valid update: %v", err)
	}

	if prefs.AppNotificationsEnabled {
		t.Error("expected AppNotificationsEnabled to be false")
	}
	if !prefs.WhatsAppAlertsEnabled {
		t.Error("expected WhatsAppAlertsEnabled to be true")
	}
	if !prefs.ExtraEmailConfigured {
		t.Error("expected ExtraEmailConfigured to be true")
	}
	if prefs.MaskedExtraEmail == validExtra || !strings.Contains(prefs.MaskedExtraEmail, "••••") {
		t.Errorf("expected masked extra email, got %s", prefs.MaskedExtraEmail)
	}

	// Verify audit event was recorded
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 audit event recorded, got %d", len(repo.events))
	}
	ev := repo.events[0]
	if ev.EventCode != "alert_channel_updated" {
		t.Errorf("expected event_code 'alert_channel_updated', got %s", ev.EventCode)
	}
	if ev.Title != "Canales de alerta actualizados" {
		t.Errorf("expected title 'Canales de alerta actualizados', got %s", ev.Title)
	}
	if ev.DeviceLabel != "Firefox en Windows" {
		t.Errorf("expected device label 'Firefox en Windows', got %s", ev.DeviceLabel)
	}

	// 4. Clearing extra email with empty string
	emptyEmail := "   "
	prefs, err = svc.UpdateSecurityAlertChannels(ctx, "acc-123", identity.UpdateSecurityAlertChannelsInput{
		AppNotificationsEnabled: true,
		EmailAlertsEnabled:      true,
		WhatsAppAlertsEnabled:   false,
		ExtraEmail:              &emptyEmail,
	})
	if err != nil {
		t.Fatalf("unexpected error on clearing email: %v", err)
	}
	if prefs.ExtraEmailConfigured {
		t.Error("expected ExtraEmailConfigured to be false after clearing")
	}
	if prefs.MaskedExtraEmail != "" {
		t.Errorf("expected empty MaskedExtraEmail, got %s", prefs.MaskedExtraEmail)
	}
	if repo.prefRecord.ExtraEmail != nil {
		t.Errorf("expected extra_email nil in DB, got %v", *repo.prefRecord.ExtraEmail)
	}
}

func TestRecordSecurityAlertEvent(t *testing.T) {
	ctx := context.Background()
	repo := &mockAlertsRepo{email: "titular@empresa.cl"}
	svc, _ := setupAlertsService(repo)

	// Missing required fields
	if err := svc.RecordSecurityAlertEvent(ctx, "", "code", "title", "label", ""); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("expected ErrInvalid for empty accountID, got %v", err)
	}
	if err := svc.RecordSecurityAlertEvent(ctx, "acc", "", "title", "label", ""); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("expected ErrInvalid for empty code, got %v", err)
	}
	if err := svc.RecordSecurityAlertEvent(ctx, "acc", "code", "", "label", ""); !errors.Is(err, identity.ErrInvalid) {
		t.Errorf("expected ErrInvalid for empty title, got %v", err)
	}

	// Raw IP address should be automatically masked
	err := svc.RecordSecurityAlertEvent(ctx, "acc-123", "test_event", "Evento de prueba", "", "200.54.12.89")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.events))
	}
	if repo.events[0].DeviceLabel != "Dispositivo desconocido" {
		t.Errorf("expected fallback device label, got %s", repo.events[0].DeviceLabel)
	}
	if repo.events[0].IPMasked != "200.54.•••.•••" {
		t.Errorf("expected masked IP '200.54.•••.•••', got %s", repo.events[0].IPMasked)
	}
}
