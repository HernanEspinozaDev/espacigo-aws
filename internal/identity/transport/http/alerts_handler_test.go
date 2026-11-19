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

func setupAlertsTestHandler() (http.Handler, *mockAuthRepo, string, string) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockAuthRepo()

	account := identity.Account{
		ID:              "account-alerts-1",
		Email:           "usuario@empresa.cl",
		NormalizedEmail: "usuario@empresa.cl",
		PasswordHash:    "secret123",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[account.ID] = account

	credGen := &testCredGen{}
	service, err := identity.NewAuthenticationService(repo, testHasher{}, dummyMailer{}, dummyLimiter{}, credGen, func() time.Time { return now })
	if err != nil {
		panic(err)
	}

	sessionID := "session-alerts-1"
	token := "token-alerts-1"
	repo.sessions[sessionID] = identity.Session{
		ID:             sessionID,
		AccountID:      account.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Chrome en Linux",
		IPMasked:       "190.162.•••.•••",
	}
	repo.tokens[token] = sessionID

	handler := NewHandler(service, repo, nil)
	return handler, repo, account.ID, token
}

func TestGetSecurityAlerts_HTTP(t *testing.T) {
	handler, _, _, token := setupAlertsTestHandler()

	t.Run("401 when unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/account/security/alerts", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		var body struct {
			Error struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.Code != "unauthenticated" {
			t.Errorf("expected error code 'unauthenticated', got %v", body.Error.Code)
		}
		if body.Error.RequestID == "" {
			t.Error("expected request_id in error body")
		}
	})

	t.Run("405 on wrong method", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/account/security/alerts", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		if rec.Header().Get("Allow") != "GET" {
			t.Errorf("expected Allow: GET, got %q", rec.Header().Get("Allow"))
		}
	})

	t.Run("200 returns defaults when no preferences saved", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/account/security/alerts", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body: %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID header")
		}

		var resp identity.SecurityAlertsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !resp.Preferences.AppNotificationsEnabled {
			t.Error("expected AppNotificationsEnabled to be true by default")
		}
		if !resp.Preferences.EmailAlertsEnabled {
			t.Error("expected EmailAlertsEnabled to be true by default")
		}
		if resp.Preferences.WhatsAppAlertsEnabled {
			t.Error("expected WhatsAppAlertsEnabled to be false by default")
		}
		if resp.Preferences.ExtraEmailConfigured {
			t.Error("expected ExtraEmailConfigured to be false by default")
		}
		if !strings.Contains(resp.Preferences.MaskedEmail, "••••") {
			t.Errorf("expected masked email, got %s", resp.Preferences.MaskedEmail)
		}
		if len(resp.Alerts) != 0 {
			t.Errorf("expected 0 alerts, got %d", len(resp.Alerts))
		}
	})
}

func TestUpdateSecurityAlertChannels_HTTP(t *testing.T) {
	handler, repo, accountID, token := setupAlertsTestHandler()

	t.Run("401 when unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/api/v1/account/security/alerts/channels", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("405 on wrong method", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/account/security/alerts/channels", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		if rec.Header().Get("Allow") != "PUT" {
			t.Errorf("expected Allow: PUT, got %q", rec.Header().Get("Allow"))
		}
	})

	t.Run("400 on malformed JSON", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/api/v1/account/security/alerts/channels", strings.NewReader(`{invalid`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("400 on invalid extra email format (error <= 100 chars, stable code, request_id)", func(t *testing.T) {
		body := `{"app_notifications_enabled":true,"email_alerts_enabled":true,"whatsapp_alerts_enabled":false,"extra_email":"bad-email"}`
		req := httptest.NewRequest("PUT", "/api/v1/account/security/alerts/channels", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body: %s", rec.Code, rec.Body.String())
		}
		var errResp struct {
			Error struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.Error.Code != "invalid_email" {
			t.Errorf("expected code 'invalid_email', got %v", errResp.Error.Code)
		}
		if len(errResp.Error.Message) > 100 || len(errResp.Error.Message) == 0 {
			t.Errorf("expected error message <= 100 chars, got length %d: %q", len(errResp.Error.Message), errResp.Error.Message)
		}
		if errResp.Error.RequestID == "" {
			t.Error("expected request_id in error response")
		}
	})

	t.Run("400 when extra email matches primary email (error <= 100 chars, stable code)", func(t *testing.T) {
		body := `{"app_notifications_enabled":true,"email_alerts_enabled":true,"whatsapp_alerts_enabled":false,"extra_email":"usuario@empresa.cl"}`
		req := httptest.NewRequest("PUT", "/api/v1/account/security/alerts/channels", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body: %s", rec.Code, rec.Body.String())
		}
		var errResp struct {
			Error struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.Error.Code != "email_same_as_primary" {
			t.Errorf("expected code 'email_same_as_primary', got %v", errResp.Error.Code)
		}
		if len(errResp.Error.Message) > 100 || len(errResp.Error.Message) == 0 {
			t.Errorf("expected error message <= 100 chars, got length %d: %q", len(errResp.Error.Message), errResp.Error.Message)
		}
		if errResp.Error.RequestID == "" {
			t.Error("expected request_id in error response")
		}
	})

	t.Run("200 on success updates channels and audits event", func(t *testing.T) {
		body := `{"app_notifications_enabled":false,"email_alerts_enabled":true,"whatsapp_alerts_enabled":true,"extra_email":"seguridad.backup@empresa.cl"}`
		req := httptest.NewRequest("PUT", "/api/v1/account/security/alerts/channels", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
		req.Header.Set("X-Forwarded-For", "190.162.12.45")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body: %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID header")
		}

		var prefs identity.SecurityAlertPreferences
		if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if prefs.AppNotificationsEnabled {
			t.Error("expected AppNotificationsEnabled to be false")
		}
		if !prefs.EmailAlertsEnabled {
			t.Error("expected EmailAlertsEnabled to be true")
		}
		if !prefs.WhatsAppAlertsEnabled {
			t.Error("expected WhatsAppAlertsEnabled to be true")
		}
		if !prefs.ExtraEmailConfigured {
			t.Error("expected ExtraEmailConfigured to be true")
		}
		if !strings.Contains(prefs.MaskedExtraEmail, "••••") {
			t.Errorf("expected masked extra email, got %s", prefs.MaskedExtraEmail)
		}

		// Verify audit event was recorded in repo
		events := repo.events[accountID]
		if len(events) == 0 {
			t.Fatal("expected at least 1 audit event in repo")
		}
		latestEv := events[0]
		if latestEv.EventCode != "alert_channel_updated" {
			t.Errorf("expected event_code 'alert_channel_updated', got %s", latestEv.EventCode)
		}
		if latestEv.DeviceLabel != "Chrome en Linux" {
			t.Errorf("expected device label 'Chrome en Linux', got %s", latestEv.DeviceLabel)
		}
		if latestEv.IPMasked != "190.162.•••.•••" {
			t.Errorf("expected masked ip '190.162.•••.•••', got %s", latestEv.IPMasked)
		}

		// Now verify GET /alerts returns the updated preferences and the event
		getReq := httptest.NewRequest("GET", "/api/v1/account/security/alerts", nil)
		getReq.Header.Set("Authorization", "Bearer "+token)
		getRec := httptest.NewRecorder()
		handler.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusOK {
			t.Fatalf("GET /alerts failed: %d body: %s", getRec.Code, getRec.Body.String())
		}
		var alertsResp identity.SecurityAlertsResponse
		if err := json.Unmarshal(getRec.Body.Bytes(), &alertsResp); err != nil {
			t.Fatalf("decode alerts response: %v", err)
		}
		if alertsResp.Preferences.AppNotificationsEnabled {
			t.Error("expected AppNotificationsEnabled to remain false")
		}
		if len(alertsResp.Alerts) != 1 {
			t.Fatalf("expected 1 alert in feed, got %d", len(alertsResp.Alerts))
		}
		if alertsResp.Alerts[0].EventCode != "alert_channel_updated" {
			t.Errorf("expected alert event_code 'alert_channel_updated', got %s", alertsResp.Alerts[0].EventCode)
		}
	})
}
