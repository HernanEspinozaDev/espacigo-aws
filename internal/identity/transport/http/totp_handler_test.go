package identityhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func setupTOTPTestHandler(now time.Time) (http.Handler, *mockAuthRepo, string, string) {
	repo := newMockAuthRepo()

	account := identity.Account{
		ID:              "account-totp-test-1",
		Email:           "seguridad@espacigo.cl",
		NormalizedEmail: "seguridad@espacigo.cl",
		PasswordHash:    "hashed-pass",
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

	key := make([]byte, 32)
	kr, _ := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": key})
	service.SetTOTPKeyring(kr)

	sessionID := "session-totp-1"
	token := "token-totp-1"
	repo.sessions[sessionID] = identity.Session{
		ID:             sessionID,
		AccountID:      account.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Chrome en macOS",
		IPMasked:       "190.162.•••.•••",
	}
	repo.tokens[token] = sessionID

	handler := NewHandler(service, repo, nil)
	return handler, repo, account.ID, token
}

func TestTOTP_HTTP_Flow(t *testing.T) {
	now := time.Unix(1700000000, 0)
	handler, _, _, token := setupTOTPTestHandler(now)

	// 1. GET /api/v1/account/security/totp without auth returns 401
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/totp", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("expected Cache-Control: no-store, got %s", cc)
		}
		if reqID := rec.Header().Get("X-Request-ID"); reqID == "" {
			t.Fatal("expected X-Request-ID header")
		}
	}

	// 2. GET /api/v1/account/security/totp with auth returns initially disabled status
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/totp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var status identity.AccountTOTPStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if status.Enabled || status.PendingSetup || status.RemainingBackupCodes != 0 {
			t.Fatalf("expected disabled status, got %+v", status)
		}
	}

	// 3. POST /api/v1/account/security/totp/setup generates pending secret
	var manualKey string
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/totp/setup", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("setup expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var setupResp identity.AccountTOTPSetupResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &setupResp); err != nil {
			t.Fatalf("decode setup response: %v", err)
		}
		if setupResp.ManualKey == "" || !strings.Contains(setupResp.OTPAuthURI, "otpauth://") {
			t.Fatalf("invalid setup response: %+v", setupResp)
		}
		manualKey = setupResp.ManualKey
	}

	// 4. POST /api/v1/account/security/totp/enable with invalid code returns 422
	{
		body, _ := json.Marshal(identity.AccountTOTPEnableInput{Code: "000000"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/totp/enable", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
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
		if errResp.Error.Code != "invalid_code" {
			t.Fatalf("expected code invalid_code, got %s", errResp.Error.Code)
		}
		if len(errResp.Error.Message) > 100 {
			t.Fatalf("error message too long: %d chars", len(errResp.Error.Message))
		}
		if errResp.Error.RequestID == "" {
			t.Fatal("expected request_id in error body")
		}
	}

	// 5. POST /api/v1/account/security/totp/enable with valid code activates TOTP
	secret, err := identity.DecodeBase32Secret(manualKey)
	if err != nil {
		t.Fatalf("decode manual key: %v", err)
	}
	currentStep := now.Unix() / 30
	validCode := identity.GenerateTOTPCode(secret, currentStep, 6)

	var backupCodes []string
	{
		body, _ := json.Marshal(identity.AccountTOTPEnableInput{Code: validCode})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/totp/enable", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("X-Espacigo-Client-IP", "200.54.12.89")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("enable expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var enableResp identity.AccountTOTPEnableResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &enableResp); err != nil {
			t.Fatalf("decode enable response: %v", err)
		}
		if !enableResp.Enabled || len(enableResp.BackupCodes) != 8 {
			t.Fatalf("unexpected enable response: %+v", enableResp)
		}
		backupCodes = enableResp.BackupCodes
	}

	// 6. GET status returns enabled with 8 backup codes
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/totp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("get expected 200, got %d", rec.Code)
		}
		var status identity.AccountTOTPStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if !status.Enabled || status.PendingSetup || status.RemainingBackupCodes != 8 {
			t.Fatalf("expected enabled with 8 codes, got %+v", status)
		}
	}

	// 7. POST /api/v1/account/security/totp/disable with backup code
	{
		body, _ := json.Marshal(identity.AccountTOTPDisableInput{Code: backupCodes[0]})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/totp/disable", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("disable expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var disableResp identity.AccountTOTPDisableResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &disableResp); err != nil {
			t.Fatalf("decode disable response: %v", err)
		}
		if disableResp.Enabled {
			t.Fatal("expected enabled: false")
		}
	}

	// 8. GET status confirms disabled
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/totp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var status identity.AccountTOTPStatus
		_ = json.Unmarshal(rec.Body.Bytes(), &status)
		if status.Enabled || status.RemainingBackupCodes != 0 {
			t.Fatalf("expected disabled status, got %+v", status)
		}
	}
}
