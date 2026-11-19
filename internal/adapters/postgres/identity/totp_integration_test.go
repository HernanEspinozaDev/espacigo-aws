package postgres_test

import (
	"errors"
	"fmt"
	"testing"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPostgresTOTP_Flow(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)

	accountID := randomUUID()
	email := fmt.Sprintf("totp-%s@ejemplo.invalid", accountID[:8])
	account := testAccount(accountID, email)
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create account: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_totp_backup_code WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_totp WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id = $1", accountID)
	}()

	// 1. Initial state: no record, 0 remaining backup codes
	rec, remaining, err := repo.GetTOTP(ctx, accountID)
	if err != nil {
		t.Fatalf("GetTOTP initially: %v", err)
	}
	if rec != nil || remaining != 0 {
		t.Fatalf("expected nil record and 0 codes, got rec=%+v, remaining=%d", rec, remaining)
	}

	// 2. Save pending secret
	pendingCipher := []byte("pending-encrypted-secret-v1")
	keyVersion := "v1"
	if err := repo.SavePendingTOTPSecret(ctx, accountID, pendingCipher, keyVersion); err != nil {
		t.Fatalf("SavePendingTOTPSecret 1: %v", err)
	}

	// Verify pending saved
	rec, remaining, err = repo.GetTOTP(ctx, accountID)
	if err != nil {
		t.Fatalf("GetTOTP after save pending: %v", err)
	}
	if rec == nil || string(rec.PendingSecretCiphertext) != string(pendingCipher) || rec.PendingKeyVersion == nil || *rec.PendingKeyVersion != keyVersion {
		t.Fatalf("pending secret mismatch: %+v", rec)
	}
	if rec.EnabledAt != nil || len(rec.SecretCiphertext) > 0 {
		t.Fatalf("expected TOTP not enabled yet, got %+v", rec)
	}

	// Overwrite pending secret
	pendingCipher2 := []byte("pending-encrypted-secret-v2")
	if err := repo.SavePendingTOTPSecret(ctx, accountID, pendingCipher2, keyVersion); err != nil {
		t.Fatalf("SavePendingTOTPSecret 2: %v", err)
	}
	rec, _, _ = repo.GetTOTP(ctx, accountID)
	if string(rec.PendingSecretCiphertext) != string(pendingCipher2) {
		t.Fatalf("expected updated pending secret, got %s", string(rec.PendingSecretCiphertext))
	}

	// 3. Activate TOTP with backup codes
	_, backupHashes, err := identity.GenerateBackupCodes(8)
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	verifiedStep := int64(57000000)

	if err := repo.ActivateTOTP(ctx, accountID, verifiedStep, backupHashes); err != nil {
		t.Fatalf("ActivateTOTP: %v", err)
	}

	// Verify activation
	rec, remaining, err = repo.GetTOTP(ctx, accountID)
	if err != nil {
		t.Fatalf("GetTOTP after activate: %v", err)
	}
	if rec == nil || rec.EnabledAt == nil {
		t.Fatal("expected enabled TOTP")
	}
	if string(rec.SecretCiphertext) != string(pendingCipher2) {
		t.Fatalf("active secret mismatch: %s", string(rec.SecretCiphertext))
	}
	if len(rec.PendingSecretCiphertext) > 0 || rec.PendingKeyVersion != nil {
		t.Fatal("expected pending secret cleared")
	}
	if rec.LastVerifiedStep == nil || *rec.LastVerifiedStep != verifiedStep {
		t.Fatalf("expected last verified step %d, got %v", verifiedStep, rec.LastVerifiedStep)
	}
	if remaining != 8 {
		t.Fatalf("expected 8 remaining backup codes, got %d", remaining)
	}

	// Activating again without pending secret returns ErrTOTPNotPending
	if err := repo.ActivateTOTP(ctx, accountID, verifiedStep+1, backupHashes); !errors.Is(err, identity.ErrTOTPNotPending) {
		t.Fatalf("expected ErrTOTPNotPending on second activate, got %v", err)
	}

	// 4. Update last verified step
	newStep := verifiedStep + 1
	if err := repo.UpdateTOTPLastVerifiedStep(ctx, accountID, newStep); err != nil {
		t.Fatalf("UpdateTOTPLastVerifiedStep: %v", err)
	}
	rec, _, _ = repo.GetTOTP(ctx, accountID)
	if rec.LastVerifiedStep == nil || *rec.LastVerifiedStep != newStep {
		t.Fatalf("expected step %d, got %v", newStep, rec.LastVerifiedStep)
	}

	// 5. Consume backup code
	firstHash := backupHashes[0]
	consumed, err := repo.ConsumeBackupCode(ctx, accountID, firstHash)
	if err != nil {
		t.Fatalf("ConsumeBackupCode: %v", err)
	}
	if !consumed {
		t.Fatal("expected backup code to be successfully consumed")
	}

	// Re-consuming same code returns false
	consumed, err = repo.ConsumeBackupCode(ctx, accountID, firstHash)
	if err != nil || consumed {
		t.Fatalf("expected already-consumed code to return false, got %v, err=%v", consumed, err)
	}

	// Remaining codes should now be 7
	_, remaining, err = repo.GetTOTP(ctx, accountID)
	if err != nil || remaining != 7 {
		t.Fatalf("expected 7 remaining codes, got %d", remaining)
	}

	// Non-existent hash returns false
	consumed, err = repo.ConsumeBackupCode(ctx, accountID, "nonexistent-hash")
	if err != nil || consumed {
		t.Fatalf("expected false for nonexistent hash, got %v, err=%v", consumed, err)
	}

	// 6. Disable TOTP
	if err := repo.DisableTOTP(ctx, accountID); err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}

	rec, remaining, err = repo.GetTOTP(ctx, accountID)
	if err != nil {
		t.Fatalf("GetTOTP after disable: %v", err)
	}
	if rec != nil && rec.EnabledAt != nil {
		t.Fatal("expected TOTP disabled")
	}
	if remaining != 0 {
		t.Fatalf("expected 0 remaining backup codes after disable, got %d", remaining)
	}
}
