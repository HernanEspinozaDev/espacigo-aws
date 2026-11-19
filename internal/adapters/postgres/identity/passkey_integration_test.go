package postgres_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPostgresPasskey_Flow(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	accountA := randomUUID()
	emailA := fmt.Sprintf("passkey-a-%s@ejemplo.invalid", accountA[:8])
	accA := testAccount(accountA, emailA)
	if err := repo.CreateWithTerms(ctx, accA, nil); err != nil {
		t.Fatalf("create account A: %v", err)
	}

	accountB := randomUUID()
	emailB := fmt.Sprintf("passkey-b-%s@ejemplo.invalid", accountB[:8])
	accB := testAccount(accountB, emailB)
	if err := repo.CreateWithTerms(ctx, accB, nil); err != nil {
		t.Fatalf("create account B: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_passkey WHERE account_id IN ($1, $2)", accountA, accountB)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_webauthn_challenge WHERE account_id IN ($1, $2)", accountA, accountB)
		_, _ = pool.Exec(ctx, "DELETE FROM public.perfil_usuario WHERE usuario_id IN ($1, $2)", accountA, accountB)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id IN ($1, $2)", accountA, accountB)
	}()

	// 1. Initial state: 0 passkeys
	passkeysA, err := repo.ListPasskeys(ctx, accountA)
	if err != nil {
		t.Fatalf("ListPasskeys initial: %v", err)
	}
	if len(passkeysA) != 0 {
		t.Fatalf("expected 0 passkeys initially, got %d", len(passkeysA))
	}

	// 2. Save challenge and retrieve it
	challengeID1 := randomUUID()
	challengeRec := identity.WebAuthnChallengeRecord{
		ID:        challengeID1,
		AccountID: accountA,
		Challenge: "random-challenge-base64url-1",
		Purpose:   "register",
		ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now,
	}
	if err := repo.SaveWebAuthnChallenge(ctx, challengeRec); err != nil {
		t.Fatalf("SaveWebAuthnChallenge: %v", err)
	}

	gotCh, err := repo.GetWebAuthnChallenge(ctx, challengeID1)
	if err != nil {
		t.Fatalf("GetWebAuthnChallenge: %v", err)
	}
	if gotCh.Challenge != challengeRec.Challenge || gotCh.Purpose != "register" || gotCh.ConsumedAt != nil {
		t.Fatalf("unexpected challenge record: %+v", gotCh)
	}

	// 3. Save passkey and consume challenge atomically
	passkeyID1 := randomUUID()
	credentialID1 := "cred-raw-id-1"
	pkRec1 := identity.PasskeyRecord{
		ID:           passkeyID1,
		AccountID:    accountA,
		CredentialID: credentialID1,
		PublicKey:    []byte("test-public-key-bytes-1"),
		Algorithm:    -7,
		SignCount:    0,
		Label:        "MacBook Touch ID",
		Transports:   []string{"internal", "hybrid"},
		CreatedAt:    now,
	}
	consumeTime := now.Add(10 * time.Second)
	if err := repo.SavePasskeyAndConsumeChallenge(ctx, pkRec1, challengeID1, consumeTime); err != nil {
		t.Fatalf("SavePasskeyAndConsumeChallenge: %v", err)
	}

	// Challenge must be consumed in DB
	gotChConsumed, err := repo.GetWebAuthnChallenge(ctx, challengeID1)
	if err != nil {
		t.Fatalf("GetWebAuthnChallenge after consume: %v", err)
	}
	if gotChConsumed.ConsumedAt == nil {
		t.Fatalf("challenge was not marked consumed")
	}

	// 4. Challenge replay: consuming already consumed challenge must return ErrPasskeyChallengeConsumed
	pkRecReplay := pkRec1
	pkRecReplay.ID = randomUUID()
	pkRecReplay.CredentialID = "cred-raw-id-replay"
	err = repo.SavePasskeyAndConsumeChallenge(ctx, pkRecReplay, challengeID1, consumeTime)
	if !errors.Is(err, identity.ErrPasskeyChallengeConsumed) {
		t.Fatalf("expected ErrPasskeyChallengeConsumed, got %v", err)
	}

	// 5. Duplicate Credential ID: attempting to register with same credential_id must return ErrPasskeyCredentialConflict
	challengeID2 := randomUUID()
	_ = repo.SaveWebAuthnChallenge(ctx, identity.WebAuthnChallengeRecord{
		ID:        challengeID2,
		AccountID: accountB,
		Challenge: "random-challenge-base64url-2",
		Purpose:   "register",
		ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now,
	})
	pkRecConflict := identity.PasskeyRecord{
		ID:           randomUUID(),
		AccountID:    accountB,
		CredentialID: credentialID1, // same as accountA
		PublicKey:    []byte("different-key-bytes"),
		Algorithm:    -7,
		Label:        "Conflict key",
		Transports:   []string{},
		CreatedAt:    now,
	}
	err = repo.SavePasskeyAndConsumeChallenge(ctx, pkRecConflict, challengeID2, consumeTime)
	if !errors.Is(err, identity.ErrPasskeyCredentialConflict) {
		t.Fatalf("expected ErrPasskeyCredentialConflict for duplicate credential_id, got %v", err)
	}

	// 6. User Isolation in ListPasskeys: Account A has 1 passkey, Account B has 0
	passkeysA, err = repo.ListPasskeys(ctx, accountA)
	if err != nil {
		t.Fatalf("ListPasskeys A: %v", err)
	}
	if len(passkeysA) != 1 || passkeysA[0].ID != passkeyID1 {
		t.Fatalf("expected 1 passkey for Account A, got %+v", passkeysA)
	}

	passkeysB, err := repo.ListPasskeys(ctx, accountB)
	if err != nil {
		t.Fatalf("ListPasskeys B: %v", err)
	}
	if len(passkeysB) != 0 {
		t.Fatalf("expected 0 passkeys for Account B, got %d", len(passkeysB))
	}

	// 7. GetAccountDisplayName
	name, err := repo.GetAccountDisplayName(ctx, accountA)
	if err != nil {
		t.Fatalf("GetAccountDisplayName initially: %v", err)
	}
	if name != "" {
		t.Fatalf("expected empty displayName initially, got %q", name)
	}

	// Insert profile and re-check
	_, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario (usuario_id, nombre_visible) VALUES ($1, $2)`, accountA, "Valeria González")
	if err != nil {
		t.Fatalf("insert perfil_usuario: %v", err)
	}
	name, err = repo.GetAccountDisplayName(ctx, accountA)
	if err != nil {
		t.Fatalf("GetAccountDisplayName after profile: %v", err)
	}
	if name != "Valeria González" {
		t.Fatalf("expected 'Valeria González', got %q", name)
	}

	// 8. DeletePasskey: Account B cannot delete Account A's passkey
	err = repo.DeletePasskey(ctx, accountB, passkeyID1)
	if !errors.Is(err, identity.ErrPasskeyNotFound) {
		t.Fatalf("expected ErrPasskeyNotFound when deleting another user's passkey, got %v", err)
	}

	// Account A deletes own passkey
	err = repo.DeletePasskey(ctx, accountA, passkeyID1)
	if err != nil {
		t.Fatalf("DeletePasskey A: %v", err)
	}

	// Already deleted passkey returns ErrPasskeyNotFound
	err = repo.DeletePasskey(ctx, accountA, passkeyID1)
	if !errors.Is(err, identity.ErrPasskeyNotFound) {
		t.Fatalf("repeat DeletePasskey: expected ErrPasskeyNotFound, got %v", err)
	}

	// Passkeys list is empty now
	passkeysA, _ = repo.ListPasskeys(ctx, accountA)
	if len(passkeysA) != 0 {
		t.Fatalf("expected 0 passkeys after delete, got %d", len(passkeysA))
	}
}
