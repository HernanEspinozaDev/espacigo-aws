package identity

import (
	"context"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
)

// LinkedDevice represents an active session or device associated with an account.
// Token hashes and full IP addresses are strictly omitted.
type LinkedDevice struct {
	ID             string    `json:"id"`
	DeviceLabel    string    `json:"device_label"`
	IPMasked       string    `json:"ip_masked"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	IsCurrent      bool      `json:"is_current"`
}

// DeviceRepository defines persistence operations for session devices.
type DeviceRepository interface {
	ListActiveSessions(ctx context.Context, accountID string, now time.Time) ([]Session, error)
	RevokeSessionForAccount(ctx context.Context, accountID string, sessionID string, revokedAt time.Time) (bool, error)
	RevokeOtherSessionsForAccount(ctx context.Context, accountID string, keepSessionID string, revokedAt time.Time) (int64, error)
}

// SanitizeUserAgent removes control characters (CR/LF) and limits length to 256 characters.
func SanitizeUserAgent(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range ua {
		if r == '\r' || r == '\n' {
			continue
		}
		if r < 32 && r != '\t' {
			continue
		}
		sb.WriteRune(r)
	}
	cleaned := strings.TrimSpace(sb.String())
	runes := []rune(cleaned)
	if len(runes) > 256 {
		return string(runes[:256])
	}
	return string(runes)
}

// DeriveDeviceLabel produces a human-readable device/browser label from a User-Agent header.
// Examples: "Chrome en Linux", "Firefox en Windows", "Safari en iPhone", "Navegador web", "Cliente API".
func DeriveDeviceLabel(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return "Dispositivo desconocido"
	}

	u := strings.ToLower(ua)

	// API clients / CLI tools
	if strings.Contains(u, "curl/") || strings.Contains(u, "postman") ||
		strings.Contains(u, "insomnia") || strings.Contains(u, "go-http-client") ||
		strings.Contains(u, "python") || strings.Contains(u, "axios") ||
		strings.Contains(u, "node-fetch") || strings.Contains(u, "wget") ||
		strings.Contains(u, "httpie") {
		return "Cliente API"
	}

	// Operating System / Device
	var os string
	switch {
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(u, "android"):
		os = "Android"
	case strings.Contains(u, "windows"):
		os = "Windows"
	case strings.Contains(u, "macintosh") || strings.Contains(u, "mac os"):
		os = "macOS"
	case strings.Contains(u, "cros"):
		os = "ChromeOS"
	case strings.Contains(u, "linux"):
		os = "Linux"
	}

	// Browser
	var browser string
	switch {
	case strings.Contains(u, "edg/") || strings.Contains(u, "edge/"):
		browser = "Edge"
	case strings.Contains(u, "opr/") || strings.Contains(u, "opera/"):
		browser = "Opera"
	case strings.Contains(u, "chrome/") || strings.Contains(u, "crios/"):
		browser = "Chrome"
	case strings.Contains(u, "firefox/") || strings.Contains(u, "fxios/"):
		browser = "Firefox"
	case strings.Contains(u, "safari/") && !strings.Contains(u, "chrome/") && !strings.Contains(u, "crios/") && !strings.Contains(u, "android"):
		browser = "Safari"
	case (os == "iPhone" || os == "iPad") && strings.Contains(u, "safari"):
		browser = "Safari"
	}

	if browser != "" && os != "" {
		return browser + " en " + os
	}
	if browser != "" {
		return browser
	}
	if os != "" {
		return "Navegador web en " + os
	}
	if strings.Contains(u, "mozilla/") || strings.Contains(u, "gecko/") || strings.Contains(u, "applewebkit/") {
		return "Navegador web"
	}
	return "Cliente API"
}

// ListLinkedDevices returns active sessions for the account, ordered by last activity descending.
func (s *AuthenticationService) ListLinkedDevices(ctx context.Context, accountID, currentSessionID string) ([]LinkedDevice, error) {
	if accountID == "" {
		return nil, ErrNotFound
	}
	now := s.now().UTC()
	sessions, err := s.repo.ListActiveSessions(ctx, accountID, now)
	if err != nil {
		return nil, err
	}

	devices := make([]LinkedDevice, 0, len(sessions))
	for _, ses := range sessions {
		label := ses.DeviceLabel
		if label == "" {
			if ses.ClientSummary != "" {
				label = ses.ClientSummary
			} else {
				label = "Navegador web"
			}
		}
		ip := ses.IPMasked
		if ip == "" {
			ip = "••••"
		}
		devices = append(devices, LinkedDevice{
			ID:             ses.ID,
			DeviceLabel:    label,
			IPMasked:       ip,
			CreatedAt:      ses.CreatedAt.UTC(),
			LastActivityAt: ses.LastActivityAt.UTC(),
			ExpiresAt:      ses.ExpiresAt.UTC(),
			IsCurrent:      (currentSessionID != "" && ses.ID == currentSessionID),
		})
	}
	return devices, nil
}

// RevokeLinkedDevice revokes a specific session strictly belonging to accountID.
func (s *AuthenticationService) RevokeLinkedDevice(ctx context.Context, accountID, targetSessionID string) error {
	if accountID == "" || targetSessionID == "" {
		return ErrNotFound
	}
	now := s.now().UTC()
	revoked, err := s.repo.RevokeSessionForAccount(ctx, accountID, targetSessionID, now)
	if err != nil {
		return err
	}
	if !revoked {
		return ErrNotFound
	}
	safelog.Info(ctx, "linked device revoked",
		safelog.F("account_id", accountID),
		safelog.F("session_id", targetSessionID),
	)
	return nil
}

// RevokeOtherLinkedDevices revokes all active sessions of accountID except currentSessionID.
func (s *AuthenticationService) RevokeOtherLinkedDevices(ctx context.Context, accountID, currentSessionID string) (int64, error) {
	if accountID == "" {
		return 0, ErrNotFound
	}
	now := s.now().UTC()
	count, err := s.repo.RevokeOtherSessionsForAccount(ctx, accountID, currentSessionID, now)
	if err != nil {
		return 0, err
	}
	safelog.Info(ctx, "other linked devices revoked",
		safelog.F("account_id", accountID),
		safelog.F("current_session_id", currentSessionID),
		safelog.F("revoked_count", count),
	)
	return count, nil
}
