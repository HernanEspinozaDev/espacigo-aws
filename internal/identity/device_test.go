package identity_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestMaskIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"IPv4 standard", "190.162.12.45", "190.162.•••.•••"},
		{"IPv4 loopback", "127.0.0.1", "127.0.•••.•••"},
		{"IPv4 private", "10.0.1.254", "10.0.•••.•••"},
		{"IPv4 with spaces", "  172.16.50.4  ", "172.16.•••.•••"},
		{"IPv6 standard", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", "2001:db8:••••:••••"},
		{"IPv6 short", "2001:db8::1", "2001:db8:••••:••••"},
		{"IPv6 localhost", "::1", "0:0:••••:••••"},
		{"IPv4-mapped IPv6", "::ffff:190.162.12.45", "190.162.•••.•••"},
		{"Empty IP", "", ""},
		{"Whitespace only", "   ", ""},
		{"Invalid IP", "not-an-ip", "••••"},
		{"Invalid IP numbers", "999.999.999.999", "••••"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identity.MaskIP(tt.input)
			if got != tt.expected {
				t.Errorf("MaskIP(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeUserAgent(t *testing.T) {
	t.Run("strips CR and LF", func(t *testing.T) {
		input := "Mozilla/5.0\r\n(X11; Linux)\nChrome/120.0"
		got := identity.SanitizeUserAgent(input)
		if strings.Contains(got, "\r") || strings.Contains(got, "\n") {
			t.Errorf("SanitizeUserAgent did not strip CRLF: %q", got)
		}
		expected := "Mozilla/5.0(X11; Linux)Chrome/120.0"
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("limits length to 256 characters", func(t *testing.T) {
		input := strings.Repeat("A", 300)
		got := identity.SanitizeUserAgent(input)
		if len(got) != 256 {
			t.Errorf("expected length 256, got %d", len(got))
		}
	})

	t.Run("empty or whitespace returns empty", func(t *testing.T) {
		if got := identity.SanitizeUserAgent("   "); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestDeriveDeviceLabel(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		expected string
	}{
		{
			name:     "Chrome on Linux",
			ua:       "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expected: "Chrome en Linux",
		},
		{
			name:     "Firefox on Windows",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/119.0",
			expected: "Firefox en Windows",
		},
		{
			name:     "Safari on iPhone",
			ua:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			expected: "Safari en iPhone",
		},
		{
			name:     "Safari on iPad",
			ua:       "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			expected: "Safari en iPad",
		},
		{
			name:     "Chrome on Android",
			ua:       "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			expected: "Chrome en Android",
		},
		{
			name:     "Edge on Windows",
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			expected: "Edge en Windows",
		},
		{
			name:     "Safari on macOS",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			expected: "Safari en macOS",
		},
		{
			name:     "Chrome on macOS",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expected: "Chrome en macOS",
		},
		{
			name:     "curl CLI",
			ua:       "curl/7.88.1",
			expected: "Cliente API",
		},
		{
			name:     "Postman",
			ua:       "PostmanRuntime/7.32.3",
			expected: "Cliente API",
		},
		{
			name:     "Go HTTP Client",
			ua:       "Go-http-client/1.1",
			expected: "Cliente API",
		},
		{
			name:     "Generic Mozilla",
			ua:       "Mozilla/5.0",
			expected: "Navegador web",
		},
		{
			name:     "Empty",
			ua:       "",
			expected: "Dispositivo desconocido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identity.DeriveDeviceLabel(tt.ua)
			if got != tt.expected {
				t.Errorf("DeriveDeviceLabel(%q) = %q, want %q", tt.ua, got, tt.expected)
			}
		})
	}
}

// fakeDeviceRepo implements identity.AuthenticationRepository for testing service methods.
type fakeDeviceRepo struct {
	sessions []identity.Session
}

func (f *fakeDeviceRepo) CreateWithTerms(context.Context, identity.Account, []identity.TermsAcceptance) error {
	return nil
}
func (f *fakeDeviceRepo) TermsVersion(context.Context, string) (identity.TermsVersion, error) {
	return identity.TermsVersion{}, nil
}
func (f *fakeDeviceRepo) WithLockedAccount(context.Context, identity.AccountLookup, func(identity.Account, identity.AuthenticationTransaction) error) error {
	return nil
}
func (f *fakeDeviceRepo) PurgeExpiredPasswordHistory(context.Context, time.Time) error {
	return nil
}
func (f *fakeDeviceRepo) PurgeExpiredCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepo) CancelInactiveCredentialNotices(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepo) ListTerminalCredentialNotices(context.Context) ([]identity.CredentialNoticeSummary, error) {
	return nil, nil
}
func (f *fakeDeviceRepo) ReopenCredentialNotice(context.Context, identity.ReopenCredentialNoticeInput) (identity.CredentialNoticeCycle, error) {
	return identity.CredentialNoticeCycle{}, nil
}
func (f *fakeDeviceRepo) ClaimCredentialNotice(context.Context, time.Time, time.Time) (identity.CredentialNotice, error) {
	return identity.CredentialNotice{}, nil
}
func (f *fakeDeviceRepo) ActiveAccountEmail(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeDeviceRepo) CancelInactiveCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (f *fakeDeviceRepo) CompleteCredentialNotice(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (f *fakeDeviceRepo) RetryCredentialNotice(context.Context, string, time.Time, time.Time, time.Time, string) error {
	return nil
}

func (f *fakeDeviceRepo) ListActiveSessions(_ context.Context, accountID string, now time.Time) ([]identity.Session, error) {
	var out []identity.Session
	for _, s := range f.sessions {
		if s.AccountID == accountID && s.RevokedAt == nil && s.ExpiresAt.After(now) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeDeviceRepo) RevokeSessionForAccount(_ context.Context, accountID string, sessionID string, revokedAt time.Time) (bool, error) {
	for i, s := range f.sessions {
		if s.AccountID == accountID && s.ID == sessionID && s.RevokedAt == nil {
			f.sessions[i].RevokedAt = &revokedAt
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeDeviceRepo) RevokeOtherSessionsForAccount(_ context.Context, accountID string, keepSessionID string, revokedAt time.Time) (int64, error) {
	var count int64
	for i, s := range f.sessions {
		if s.AccountID == accountID && s.ID != keepSessionID && s.RevokedAt == nil && s.ExpiresAt.After(revokedAt) {
			f.sessions[i].RevokedAt = &revokedAt
			count++
		}
	}
	return count, nil
}

type dummyHasher struct{}

func (dummyHasher) Hash(s identity.Secret) (identity.Secret, error)      { return s, nil }
func (dummyHasher) Matches(h, s identity.Secret) (bool, error)          { return h == s, nil }

type dummyMailer struct{}

func (dummyMailer) SendVerification(context.Context, identity.VerificationDelivery) error { return nil }
func (dummyMailer) SendRecovery(context.Context, identity.RecoveryDelivery) error         { return nil }
func (dummyMailer) SendPasswordChanged(context.Context, string) error                     { return nil }
func (dummyMailer) SendLoginAlert(context.Context, string, time.Time) error               { return nil }

type dummyLimiter struct{}

func (dummyLimiter) AllowVerification(context.Context, string, time.Time) (bool, error) { return true, nil }

type dummyCredGen struct{}

func (dummyCredGen) ID() (string, error)          { return "test-id", nil }
func (dummyCredGen) Token() (identity.Secret, error) { return identity.Secret("test-token"), nil }

func TestLinkedDevicesService(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	accountA := "account-aaa"
	accountB := "account-bbb"

	repo := &fakeDeviceRepo{
		sessions: []identity.Session{
			{
				ID:             "ses-1",
				AccountID:      accountA,
				DeviceLabel:    "Chrome en Linux",
				IPMasked:       "190.162.•••.•••",
				CreatedAt:      now.Add(-2 * time.Hour),
				LastActivityAt: now.Add(-10 * time.Minute),
				ExpiresAt:      now.Add(6 * time.Hour),
			},
			{
				ID:             "ses-2",
				AccountID:      accountA,
				DeviceLabel:    "Safari en iPhone",
				IPMasked:       "200.54.•••.•••",
				CreatedAt:      now.Add(-1 * time.Hour),
				LastActivityAt: now.Add(-5 * time.Minute),
				ExpiresAt:      now.Add(7 * time.Hour),
			},
			{
				ID:             "ses-b1",
				AccountID:      accountB,
				DeviceLabel:    "Firefox en Windows",
				IPMasked:       "186.105.•••.•••",
				CreatedAt:      now.Add(-30 * time.Minute),
				LastActivityAt: now.Add(-2 * time.Minute),
				ExpiresAt:      now.Add(8 * time.Hour),
			},
		},
	}

	service, err := identity.NewAuthenticationService(repo, dummyHasher{}, dummyMailer{}, dummyLimiter{}, dummyCredGen{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAuthenticationService: %v", err)
	}

	ctx := context.Background()

	t.Run("ListLinkedDevices returns active sessions with is_current flag", func(t *testing.T) {
		devices, err := service.ListLinkedDevices(ctx, accountA, "ses-2")
		if err != nil {
			t.Fatalf("ListLinkedDevices: %v", err)
		}
		if len(devices) != 2 {
			t.Fatalf("expected 2 devices for accountA, got %d", len(devices))
		}

		var foundCurrent bool
		for _, d := range devices {
			if d.ID == "ses-2" && d.IsCurrent {
				foundCurrent = true
			}
			if d.ID == "ses-1" && d.IsCurrent {
				t.Errorf("ses-1 should not be marked as current")
			}
			if d.IPMasked == "" || !strings.Contains(d.IPMasked, "•••") {
				t.Errorf("expected masked IP, got %q", d.IPMasked)
			}
		}
		if !foundCurrent {
			t.Errorf("ses-2 was not marked as current")
		}
	})

	t.Run("RevokeLinkedDevice isolation: accountA cannot revoke accountB session", func(t *testing.T) {
		err := service.RevokeLinkedDevice(ctx, accountA, "ses-b1")
		if err == nil {
			t.Fatalf("expected error revoking another account's session, got nil")
		}
		// Check that ses-b1 is still active
		bDevices, err := service.ListLinkedDevices(ctx, accountB, "")
		if err != nil || len(bDevices) != 1 {
			t.Fatalf("accountB session was unexpectedly revoked or list failed")
		}
	})

	t.Run("RevokeLinkedDevice succeeds for owned session", func(t *testing.T) {
		err := service.RevokeLinkedDevice(ctx, accountA, "ses-1")
		if err != nil {
			t.Fatalf("RevokeLinkedDevice failed: %v", err)
		}

		devices, err := service.ListLinkedDevices(ctx, accountA, "ses-2")
		if err != nil {
			t.Fatalf("ListLinkedDevices: %v", err)
		}
		if len(devices) != 1 || devices[0].ID != "ses-2" {
			t.Fatalf("expected only ses-2 active, got %+v", devices)
		}
	})

	t.Run("RevokeOtherLinkedDevices revokes others keeping current", func(t *testing.T) {
		// Add another session for accountA
		repo.sessions = append(repo.sessions, identity.Session{
			ID:             "ses-3",
			AccountID:      accountA,
			DeviceLabel:    "Chrome en Windows",
			IPMasked:       "190.162.•••.•••",
			CreatedAt:      now,
			LastActivityAt: now,
			ExpiresAt:      now.Add(8 * time.Hour),
		})

		count, err := service.RevokeOtherLinkedDevices(ctx, accountA, "ses-2")
		if err != nil {
			t.Fatalf("RevokeOtherLinkedDevices: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 session revoked (ses-3), got %d", count)
		}

		devices, err := service.ListLinkedDevices(ctx, accountA, "ses-2")
		if err != nil {
			t.Fatalf("ListLinkedDevices: %v", err)
		}
		if len(devices) != 1 || devices[0].ID != "ses-2" {
			t.Fatalf("expected only ses-2 remaining, got %+v", devices)
		}
	})
}
