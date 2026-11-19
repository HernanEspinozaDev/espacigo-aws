package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type mockQRRepo struct {
	mockAlertsRepo
	qrDevice  *identity.AccountQRDeviceRecord
	qrTickets map[string]*identity.AccountQRTicketRecord
	events    []identity.SecurityAlertEventRecord
}

func newMockQRRepo() *mockQRRepo {
	return &mockQRRepo{
		qrTickets: make(map[string]*identity.AccountQRTicketRecord),
	}
}

func (m *mockQRRepo) GetQRDevice(_ context.Context, accountID string) (*identity.AccountQRDeviceRecord, error) {
	if m.qrDevice != nil && m.qrDevice.AccountID == accountID {
		return m.qrDevice, nil
	}
	return nil, nil
}

func (m *mockQRRepo) UpsertQRDevice(_ context.Context, accountID string, enabledAt time.Time, trustedDeviceLabel string) error {
	var label *string
	if trustedDeviceLabel != "" {
		label = &trustedDeviceLabel
	}
	m.qrDevice = &identity.AccountQRDeviceRecord{
		AccountID:          accountID,
		EnabledAt:          &enabledAt,
		TrustedDeviceLabel: label,
		UpdatedAt:          enabledAt,
	}
	return nil
}

func (m *mockQRRepo) DisableQRDevice(_ context.Context, accountID string, updatedAt time.Time) error {
	if m.qrDevice != nil && m.qrDevice.AccountID == accountID {
		m.qrDevice.EnabledAt = nil
		m.qrDevice.UpdatedAt = updatedAt
	}
	return nil
}

func (m *mockQRRepo) SaveQRTicket(_ context.Context, ticket identity.AccountQRTicketRecord) error {
	t := ticket
	m.qrTickets[ticket.ID] = &t
	return nil
}

func (m *mockQRRepo) GetQRTicketByID(_ context.Context, ticketID string) (*identity.AccountQRTicketRecord, error) {
	if t, ok := m.qrTickets[ticketID]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *mockQRRepo) GetQRTicketByTokenHash(_ context.Context, tokenHash string) (*identity.AccountQRTicketRecord, error) {
	for _, t := range m.qrTickets {
		if t.TokenHash == tokenHash {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockQRRepo) ApproveQRTicket(_ context.Context, accountID, ticketID, deviceLabel string, resolvedAt time.Time) error {
	t, ok := m.qrTickets[ticketID]
	if !ok || t.AccountID != accountID || t.Status != "pending" || t.ResolvedAt != nil {
		return identity.ErrQRTicketResolved
	}
	t.Status = "approved"
	t.ResolvedAt = &resolvedAt

	var label *string
	if deviceLabel != "" {
		label = &deviceLabel
	}
	m.qrDevice = &identity.AccountQRDeviceRecord{
		AccountID:          accountID,
		EnabledAt:          &resolvedAt,
		TrustedDeviceLabel: label,
		UpdatedAt:          resolvedAt,
	}
	return nil
}

func (m *mockQRRepo) ExpireQRTicket(_ context.Context, ticketID string) error {
	if t, ok := m.qrTickets[ticketID]; ok && t.Status == "pending" {
		t.Status = "expired"
	}
	return nil
}

func (m *mockQRRepo) GetLocationPermission(context.Context, string) (*identity.LocationPermissionRecord, error) {
	return nil, nil
}
func (m *mockQRRepo) UpsertLocationPermission(context.Context, identity.LocationPermissionRecord) error {
	return nil
}
func (m *mockQRRepo) ListActiveTrustedContacts(context.Context, string) ([]identity.TrustedContactRecord, error) {
	return nil, nil
}
func (m *mockQRRepo) CountActiveTrustedContacts(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockQRRepo) GetTrustedContact(context.Context, string, string) (*identity.TrustedContactRecord, error) {
	return nil, nil
}
func (m *mockQRRepo) InsertTrustedContact(context.Context, identity.TrustedContactRecord) error {
	return nil
}
func (m *mockQRRepo) RevokeTrustedContact(context.Context, string, string, time.Time) error {
	return nil
}

func (m *mockQRRepo) RecordSecurityAlertEvent(_ context.Context, event identity.SecurityAlertEventRecord) error {
	m.events = append(m.events, event)
	return nil
}

func setupQRTestService(repo *mockQRRepo, now time.Time) *identity.AuthenticationService {
	hasher := alertsTestHasher{}
	mailer := alertsTestMailer{}
	limiter := alertsTestLimiter{}
	credGen := &alertsTestCredGen{nextID: "00000000-0000-4000-8000-000000000001"}
	svc, _ := identity.NewAuthenticationService(repo, hasher, mailer, limiter, credGen, func() time.Time { return now })
	return svc
}

func TestQR_GetQRStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	// Empty accountID
	if _, err := svc.GetQRStatus(ctx, ""); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty accountID, got %v", err)
	}

	// Unenabled account
	status, err := svc.GetQRStatus(ctx, "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Enabled || status.EnabledAt != nil || status.TrustedDeviceLabel != nil {
		t.Fatalf("expected disabled QR status, got %+v", status)
	}

	// Enabled account
	label := "Pixel 8 Pro"
	repo.qrDevice = &identity.AccountQRDeviceRecord{
		AccountID:          "acc-1",
		EnabledAt:          &now,
		TrustedDeviceLabel: &label,
		UpdatedAt:          now,
	}

	status, err = svc.GetQRStatus(ctx, "acc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Enabled || status.EnabledAt == nil || *status.TrustedDeviceLabel != label {
		t.Fatalf("expected enabled QR status with label, got %+v", status)
	}
}

func TestQR_CreateQRTicket(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	// Empty accountID
	if _, err := svc.CreateQRTicket(ctx, "", "Chrome", "192.168.1.1"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty accountID, got %v", err)
	}

	// Successful ticket generation
	resp, err := svc.CreateQRTicket(ctx, "acc-1", "Chrome Desktop", "192.168.1.0/24")
	if err != nil {
		t.Fatalf("CreateQRTicket failed: %v", err)
	}
	if resp.TicketID != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("unexpected ticket ID: %s", resp.TicketID)
	}
	if resp.QRPayload == "" {
		t.Fatal("empty qr_payload")
	}
	expectedExpiry := now.Add(120 * time.Second)
	if !resp.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected expires_at %v, got %v", expectedExpiry, resp.ExpiresAt)
	}

	// Check saved record in repo
	ticket, ok := repo.qrTickets[resp.TicketID]
	if !ok {
		t.Fatal("ticket not found in repo")
	}
	if ticket.Status != "pending" {
		t.Fatalf("expected status pending, got %s", ticket.Status)
	}
	h := sha256.Sum256([]byte(resp.QRPayload))
	expectedHash := hex.EncodeToString(h[:])
	if ticket.TokenHash != expectedHash {
		t.Fatalf("expected token hash %s, got %s", expectedHash, ticket.TokenHash)
	}
}

func TestQR_GetQRTicketStatus_AndIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	createResp, err := svc.CreateQRTicket(ctx, "acc-owner", "Safari", "10.0.0.1")
	if err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	// Query from owner -> pending
	statusResp, err := svc.GetQRTicketStatus(ctx, "acc-owner", createResp.TicketID)
	if err != nil {
		t.Fatalf("GetQRTicketStatus failed: %v", err)
	}
	if statusResp.Status != "pending" {
		t.Fatalf("expected status pending, got %s", statusResp.Status)
	}

	// Account Isolation: Query from another account -> ErrQRTicketNotFound
	if _, err := svc.GetQRTicketStatus(ctx, "acc-attacker", createResp.TicketID); !errors.Is(err, identity.ErrQRTicketNotFound) {
		t.Fatalf("expected ErrQRTicketNotFound for different account, got %v", err)
	}

	// Query non-existent ticket -> ErrQRTicketNotFound
	if _, err := svc.GetQRTicketStatus(ctx, "acc-owner", "00000000-0000-4000-8000-999999999999"); !errors.Is(err, identity.ErrQRTicketNotFound) {
		t.Fatalf("expected ErrQRTicketNotFound for nonexistent ticket, got %v", err)
	}

	// Advance time past 120s TTL -> reports "expired"
	expiredTime := now.Add(125 * time.Second)
	svcExpired := setupQRTestService(repo, expiredTime)
	statusExpired, err := svcExpired.GetQRTicketStatus(ctx, "acc-owner", createResp.TicketID)
	if err != nil {
		t.Fatalf("GetQRTicketStatus expired failed: %v", err)
	}
	if statusExpired.Status != "expired" {
		t.Fatalf("expected status expired, got %s", statusExpired.Status)
	}
}

func TestQR_ApproveQRTicket_AndAlerts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	createResp, err := svc.CreateQRTicket(ctx, "acc-owner", "Firefox", "10.0.0.1")
	if err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	// Empty token -> ErrQRTicketInvalid
	if _, err := svc.ApproveQRTicket(ctx, "acc-owner", "   ", "Mobile App", "10.0.0.2"); !errors.Is(err, identity.ErrQRTicketInvalid) {
		t.Fatalf("expected ErrQRTicketInvalid for empty token, got %v", err)
	}

	// Account isolation: Another account tries to approve -> ErrQRTicketNotFound
	if _, err := svc.ApproveQRTicket(ctx, "acc-attacker", createResp.QRPayload, "Mobile App", "10.0.0.2"); !errors.Is(err, identity.ErrQRTicketNotFound) {
		t.Fatalf("expected ErrQRTicketNotFound for attacker account, got %v", err)
	}

	// Successful approval
	approveResp, err := svc.ApproveQRTicket(ctx, "acc-owner", createResp.QRPayload, "iPhone 15 Pro", "10.0.0.2")
	if err != nil {
		t.Fatalf("ApproveQRTicket failed: %v", err)
	}
	if approveResp.Status != "approved" {
		t.Fatalf("expected status approved, got %s", approveResp.Status)
	}
	if !approveResp.ResolvedAt.Equal(now) {
		t.Fatalf("expected resolvedAt %v, got %v", now, approveResp.ResolvedAt)
	}

	// Verify device is enabled
	if repo.qrDevice == nil || repo.qrDevice.EnabledAt == nil || *repo.qrDevice.TrustedDeviceLabel != "iPhone 15 Pro" {
		t.Fatalf("expected qrDevice to be enabled with label, got %+v", repo.qrDevice)
	}

	// Verify audit alert event recorded
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(repo.events))
	}
	if repo.events[0].EventCode != "qr_device_verified" {
		t.Fatalf("expected eventCode qr_device_verified, got %s", repo.events[0].EventCode)
	}

	// Re-approving already resolved ticket -> ErrQRTicketResolved
	if _, err := svc.ApproveQRTicket(ctx, "acc-owner", createResp.QRPayload, "iPhone 15 Pro", "10.0.0.2"); !errors.Is(err, identity.ErrQRTicketResolved) {
		t.Fatalf("expected ErrQRTicketResolved on re-approval, got %v", err)
	}
}

func TestQR_ApproveQRTicket_Expired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	createResp, err := svc.CreateQRTicket(ctx, "acc-owner", "Firefox", "10.0.0.1")
	if err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	// Advance time past TTL
	afterExpiry := now.Add(125 * time.Second)
	svcLate := setupQRTestService(repo, afterExpiry)

	if _, err := svcLate.ApproveQRTicket(ctx, "acc-owner", createResp.QRPayload, "Mobile App", "10.0.0.2"); !errors.Is(err, identity.ErrQRTicketExpired) {
		t.Fatalf("expected ErrQRTicketExpired for expired ticket, got %v", err)
	}
}

func TestQR_DisableQR_Lifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	repo := newMockQRRepo()
	svc := setupQRTestService(repo, now)

	// Attempt to disable when not enabled -> ErrQRDeviceNotEnabled
	if _, err := svc.DisableQR(ctx, "acc-1", "Web", "10.0.0.1"); !errors.Is(err, identity.ErrQRDeviceNotEnabled) {
		t.Fatalf("expected ErrQRDeviceNotEnabled, got %v", err)
	}

	// Enable device
	label := "Android Phone"
	repo.qrDevice = &identity.AccountQRDeviceRecord{
		AccountID:          "acc-1",
		EnabledAt:          &now,
		TrustedDeviceLabel: &label,
		UpdatedAt:          now,
	}

	// Disable device
	disableResp, err := svc.DisableQR(ctx, "acc-1", "Web Browser", "10.0.0.1")
	if err != nil {
		t.Fatalf("DisableQR failed: %v", err)
	}
	if disableResp.Enabled {
		t.Fatal("expected Enabled to be false")
	}
	if repo.qrDevice.EnabledAt != nil {
		t.Fatal("expected EnabledAt in repo to be nil")
	}

	// Check alert event recorded
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(repo.events))
	}
	if repo.events[0].EventCode != "qr_device_disabled" {
		t.Fatalf("expected eventCode qr_device_disabled, got %s", repo.events[0].EventCode)
	}
}
