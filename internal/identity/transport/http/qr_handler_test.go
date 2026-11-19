package identityhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func setupQRTestHandler(now time.Time) (http.Handler, *mockAuthRepo, string, string, string, string) {
	repo := newMockAuthRepo()

	// Account 1 (Owner)
	acc1 := identity.Account{
		ID:              "account-qr-test-1",
		Email:           "owner@espacigo.cl",
		NormalizedEmail: "owner@espacigo.cl",
		PasswordHash:    "hashed-pass-1",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[acc1.ID] = acc1

	token1 := "token-qr-owner"
	sess1 := "session-qr-1"
	repo.sessions[sess1] = identity.Session{
		ID:             sess1,
		AccountID:      acc1.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token1))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "MacBook Pro",
		IPMasked:       "190.162.•••.•••",
	}
	repo.tokens[token1] = sess1

	// Account 2 (Attacker)
	acc2 := identity.Account{
		ID:              "account-qr-test-2",
		Email:           "attacker@espacigo.cl",
		NormalizedEmail: "attacker@espacigo.cl",
		PasswordHash:    "hashed-pass-2",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[acc2.ID] = acc2

	token2 := "token-qr-attacker"
	sess2 := "session-qr-2"
	repo.sessions[sess2] = identity.Session{
		ID:             sess2,
		AccountID:      acc2.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token2))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Linux Laptop",
		IPMasked:       "200.54.•••.•••",
	}
	repo.tokens[token2] = sess2

	credGen := &testCredGen{}
	service, err := identity.NewAuthenticationService(repo, testHasher{}, dummyMailer{}, dummyLimiter{}, credGen, func() time.Time { return now })
	if err != nil {
		panic(err)
	}

	handler := NewHandler(service, repo, nil)
	return handler, repo, acc1.ID, token1, acc2.ID, token2
}

func TestQR_HTTP_FullFlow_And_Isolation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	handler, _, _, tokenOwner, _, tokenAttacker := setupQRTestHandler(now)

	// 1. Unauthorized access checks on all 5 endpoints
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/account/security/qr"},
		{http.MethodPost, "/api/v1/account/security/qr/ticket"},
		{http.MethodGet, "/api/v1/account/security/qr/ticket/00000000-0000-4000-8000-000000000001"},
		{http.MethodPost, "/api/v1/account/security/qr/approve"},
		{http.MethodDelete, "/api/v1/account/security/qr"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected 401, got %d", ep.method, ep.path, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("%s %s expected Cache-Control: no-store, got %s", ep.method, ep.path, cc)
		}
		if reqID := rec.Header().Get("X-Request-ID"); reqID == "" {
			t.Fatalf("%s %s expected X-Request-ID", ep.method, ep.path)
		}
	}

	// 2. GET /api/v1/account/security/qr initially returns enabled: false
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var status identity.AccountQRStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("unmarshal status: %v", err)
		}
		if status.Enabled || status.EnabledAt != nil || status.TrustedDeviceLabel != nil {
			t.Fatalf("expected disabled status, got %+v", status)
		}
	}

	// 3. POST /api/v1/account/security/qr/ticket generates ticket
	var ticketID, qrPayload string
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/ticket", bytes.NewBufferString(`{"device_label":"Chrome Desktop"}`))
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var ticketResp identity.CreateQRTicketResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &ticketResp); err != nil {
			t.Fatalf("unmarshal ticket: %v", err)
		}
		if ticketResp.TicketID == "" || ticketResp.QRPayload == "" {
			t.Fatalf("missing ticket ID or payload: %+v", ticketResp)
		}
		ticketID = ticketResp.TicketID
		qrPayload = ticketResp.QRPayload
	}

	// 4. GET /api/v1/account/security/qr/ticket/{ticket_id}
	// 4a. Owner sees pending status
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr/ticket/"+ticketID, nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var st identity.QRTicketStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatalf("unmarshal ticket status: %v", err)
		}
		if st.Status != "pending" || st.TicketID != ticketID {
			t.Fatalf("expected pending status for %s, got %+v", ticketID, st)
		}
	}

	// 4b. Attacker querying owner's ticket gets 404 (isolation)
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr/ticket/"+ticketID, nil)
		req.Header.Set("Authorization", "Bearer "+tokenAttacker)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for cross-account query, got %d", rec.Code)
		}
	}

	// 4c. Invalid UUID gets 404
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr/ticket/not-a-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for invalid uuid, got %d", rec.Code)
		}
	}

	// 5. POST /api/v1/account/security/qr/approve
	// 5a. Attacker trying to approve owner's ticket gets 404 (isolation)
	{
		body, _ := json.Marshal(identity.ApproveQRTicketInput{
			Token:       qrPayload,
			DeviceLabel: "Attacker Phone",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/approve", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenAttacker)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for cross-account approve, got %d", rec.Code)
		}
	}

	// 5b. Owner approves ticket successfully
	{
		body, _ := json.Marshal(identity.ApproveQRTicketInput{
			Token:       qrPayload,
			DeviceLabel: "iPhone 15 Pro",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/approve", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on approve, got %d: %s", rec.Code, rec.Body.String())
		}
		var appResp identity.ApproveQRTicketResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &appResp); err != nil {
			t.Fatalf("unmarshal approve: %v", err)
		}
		if appResp.Status != "approved" {
			t.Fatalf("expected approved, got %s", appResp.Status)
		}
	}

	// 5c. Re-approving already resolved ticket gets 409 Conflict
	{
		body, _ := json.Marshal(identity.ApproveQRTicketInput{
			Token:       qrPayload,
			DeviceLabel: "iPhone 15 Pro",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/approve", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for resolved ticket, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 6. GET /api/v1/account/security/qr now shows enabled: true
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var status identity.AccountQRStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("unmarshal status: %v", err)
		}
		if !status.Enabled || status.EnabledAt == nil || status.TrustedDeviceLabel == nil || *status.TrustedDeviceLabel != "iPhone 15 Pro" {
			t.Fatalf("expected enabled status with label, got %+v", status)
		}
	}

	// 7. DELETE /api/v1/account/security/qr disables device
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/qr", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on delete, got %d: %s", rec.Code, rec.Body.String())
		}
		var disResp identity.AccountQRDisableResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &disResp); err != nil {
			t.Fatalf("unmarshal disable resp: %v", err)
		}
		if disResp.Enabled {
			t.Fatal("expected Enabled to be false")
		}
	}

	// 8. DELETE again when not enabled returns 400 qr_not_enabled
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/account/security/qr", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 when disabling unenabled QR, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 9. Method not allowed on /api/v1/account/security/qr (e.g. PUT) -> 405 with Allow header
	{
		req := httptest.NewRequest(http.MethodPut, "/api/v1/account/security/qr", nil)
		req.Header.Set("Authorization", "Bearer "+tokenOwner)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow != "GET, DELETE" {
			t.Fatalf("expected Allow: GET, DELETE, got %s", allow)
		}
	}
}

func TestQR_HTTP_Expiration(t *testing.T) {
	now := time.Unix(1700000000, 0)
	repo := newMockAuthRepo()

	acc := identity.Account{
		ID:              "account-qr-exp-1",
		Email:           "exp@espacigo.cl",
		NormalizedEmail: "exp@espacigo.cl",
		PasswordHash:    "hashed-pass",
		State:           identity.AccountActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	repo.accounts[acc.ID] = acc

	token := "token-qr-exp"
	sess := "session-qr-exp"
	repo.sessions[sess] = identity.Session{
		ID:             sess,
		AccountID:      acc.ID,
		TokenHash:      string(identity.CredentialHash(identity.Secret(token))),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		DeviceLabel:    "Device",
		IPMasked:       "10.0.0.1",
	}
	repo.tokens[token] = sess

	credGen := &testCredGen{}
	clockTime := now
	service, _ := identity.NewAuthenticationService(repo, testHasher{}, dummyMailer{}, dummyLimiter{}, credGen, func() time.Time { return clockTime })
	handler := NewHandler(service, repo, nil)

	// Create ticket at T=0
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create ticket failed: %d", rec.Code)
	}
	var ticketResp identity.CreateQRTicketResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ticketResp)

	// Advance clock past 120s TTL
	clockTime = now.Add(125 * time.Second)

	// GET status reports "expired"
	{
		reqStatus := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/qr/ticket/"+ticketResp.TicketID, nil)
		reqStatus.Header.Set("Authorization", "Bearer "+token)
		recStatus := httptest.NewRecorder()
		handler.ServeHTTP(recStatus, reqStatus)
		if recStatus.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recStatus.Code)
		}
		var st identity.QRTicketStatusResponse
		_ = json.Unmarshal(recStatus.Body.Bytes(), &st)
		if st.Status != "expired" {
			t.Fatalf("expected status expired, got %s", st.Status)
		}
	}

	// POST approve returns 422 Unprocessable Entity (ticket_expired)
	{
		body, _ := json.Marshal(identity.ApproveQRTicketInput{
			Token: ticketResp.QRPayload,
		})
		reqApprove := httptest.NewRequest(http.MethodPost, "/api/v1/account/security/qr/approve", bytes.NewReader(body))
		reqApprove.Header.Set("Authorization", "Bearer "+token)
		reqApprove.Header.Set("Content-Type", "application/json")
		recApprove := httptest.NewRecorder()
		handler.ServeHTTP(recApprove, reqApprove)
		if recApprove.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity for expired ticket, got %d: %s", recApprove.Code, recApprove.Body.String())
		}
	}
}
