package postgres_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPostgresQR_Lifecycle(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)

	accountID := randomUUID()
	email := fmt.Sprintf("qr-%s@ejemplo.invalid", accountID[:8])
	account := testAccount(accountID, email)
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create account: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_qr_ticket WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_qr_dispositivo WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id = $1", accountID)
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Initial QR device state: nil
	dev, err := repo.GetQRDevice(ctx, accountID)
	if err != nil {
		t.Fatalf("GetQRDevice initially: %v", err)
	}
	if dev != nil {
		t.Fatalf("expected nil device initially, got %+v", dev)
	}

	// 2. UpsertQRDevice
	label := "iPhone 15 Pro"
	if err := repo.UpsertQRDevice(ctx, accountID, now, label); err != nil {
		t.Fatalf("UpsertQRDevice: %v", err)
	}

	dev, err = repo.GetQRDevice(ctx, accountID)
	if err != nil {
		t.Fatalf("GetQRDevice after upsert: %v", err)
	}
	if dev == nil || dev.EnabledAt == nil || dev.TrustedDeviceLabel == nil || *dev.TrustedDeviceLabel != label {
		t.Fatalf("expected enabled device with label %s, got %+v", label, dev)
	}

	// 3. DisableQRDevice
	disabledTime := now.Add(time.Minute)
	if err := repo.DisableQRDevice(ctx, accountID, disabledTime); err != nil {
		t.Fatalf("DisableQRDevice: %v", err)
	}

	dev, err = repo.GetQRDevice(ctx, accountID)
	if err != nil {
		t.Fatalf("GetQRDevice after disable: %v", err)
	}
	if dev == nil || dev.EnabledAt != nil {
		t.Fatalf("expected disabled device (enabled_at=nil), got %+v", dev)
	}

	// 4. SaveQRTicket and retrieve by ID & TokenHash
	ticketID := randomUUID()
	token := "secret-qr-token-32-bytes-long-string-test"
	h := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(h[:])
	expiresAt := now.Add(120 * time.Second)

	ticketRec := identity.AccountQRTicketRecord{
		ID:          ticketID,
		AccountID:   accountID,
		TokenHash:   tokenHash,
		Status:      "pending",
		DeviceLabel: "Chrome Desktop",
		IPMasked:    "190.162.•••.•••",
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	if err := repo.SaveQRTicket(ctx, ticketRec); err != nil {
		t.Fatalf("SaveQRTicket: %v", err)
	}

	// Retrieve by ID
	byID, err := repo.GetQRTicketByID(ctx, ticketID)
	if err != nil {
		t.Fatalf("GetQRTicketByID: %v", err)
	}
	if byID == nil || byID.ID != ticketID || byID.Status != "pending" || byID.TokenHash != tokenHash {
		t.Fatalf("retrieved by ID mismatch: %+v", byID)
	}

	// Retrieve by TokenHash
	byHash, err := repo.GetQRTicketByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetQRTicketByTokenHash: %v", err)
	}
	if byHash == nil || byHash.ID != ticketID || byHash.Status != "pending" {
		t.Fatalf("retrieved by hash mismatch: %+v", byHash)
	}

	// 5. ApproveQRTicket atomically marks ticket approved and enables device
	approvedAt := now.Add(10 * time.Second)
	trustedDevice := "Pixel 9 Pro"
	if err := repo.ApproveQRTicket(ctx, accountID, ticketID, trustedDevice, approvedAt); err != nil {
		t.Fatalf("ApproveQRTicket: %v", err)
	}

	// Verify ticket status
	byID, err = repo.GetQRTicketByID(ctx, ticketID)
	if err != nil || byID == nil || byID.Status != "approved" || byID.ResolvedAt == nil {
		t.Fatalf("expected approved ticket, got %+v", byID)
	}

	// Verify device enabled
	dev, err = repo.GetQRDevice(ctx, accountID)
	if err != nil || dev == nil || dev.EnabledAt == nil || dev.TrustedDeviceLabel == nil || *dev.TrustedDeviceLabel != trustedDevice {
		t.Fatalf("expected enabled device %s, got %+v", trustedDevice, dev)
	}

	// 6. Re-approving already approved ticket returns ErrQRTicketResolved
	if err := repo.ApproveQRTicket(ctx, accountID, ticketID, trustedDevice, approvedAt); !errors.Is(err, identity.ErrQRTicketResolved) {
		t.Fatalf("expected ErrQRTicketResolved on re-approval, got %v", err)
	}

	// 7. ExpireQRTicket
	ticketID2 := randomUUID()
	h2 := sha256.Sum256([]byte("token-2"))
	tokenHash2 := hex.EncodeToString(h2[:])
	ticketRec2 := identity.AccountQRTicketRecord{
		ID:          ticketID2,
		AccountID:   accountID,
		TokenHash:   tokenHash2,
		Status:      "pending",
		DeviceLabel: "Desktop",
		IPMasked:    "10.0.0.1",
		ExpiresAt:   now.Add(120 * time.Second),
		CreatedAt:   now,
	}
	if err := repo.SaveQRTicket(ctx, ticketRec2); err != nil {
		t.Fatalf("SaveQRTicket 2: %v", err)
	}

	if err := repo.ExpireQRTicket(ctx, ticketID2); err != nil {
		t.Fatalf("ExpireQRTicket: %v", err)
	}

	byID2, err := repo.GetQRTicketByID(ctx, ticketID2)
	if err != nil || byID2 == nil || byID2.Status != "expired" {
		t.Fatalf("expected expired ticket, got %+v", byID2)
	}
}
