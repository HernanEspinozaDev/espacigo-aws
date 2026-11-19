package postgres_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPostgresLocationPermission_Lifecycle(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)

	accountID := randomUUID()
	email := fmt.Sprintf("loc-%s@ejemplo.invalid", accountID[:8])
	account := testAccount(accountID, email)
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create account: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_permiso_ubicacion WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id = $1", accountID)
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Initially nil
	rec, err := repo.GetLocationPermission(ctx, accountID)
	if err != nil {
		t.Fatalf("GetLocationPermission initially: %v", err)
	}
	if rec != nil {
		t.Fatalf("expected nil location permission, got %+v", rec)
	}

	// 2. Upsert enabled with region
	region := "Región de Valparaíso"
	perm := identity.LocationPermissionRecord{
		AccountID:        accountID,
		Enabled:          true,
		ConsentUpdatedAt: now,
		LastRegionLabel:  &region,
	}
	if err := repo.UpsertLocationPermission(ctx, perm); err != nil {
		t.Fatalf("UpsertLocationPermission: %v", err)
	}

	// 3. Retrieve
	rec, err = repo.GetLocationPermission(ctx, accountID)
	if err != nil {
		t.Fatalf("GetLocationPermission after upsert: %v", err)
	}
	if rec == nil || !rec.Enabled || rec.LastRegionLabel == nil || *rec.LastRegionLabel != region {
		t.Fatalf("expected enabled with region %s, got %+v", region, rec)
	}

	// 4. Update (disable)
	disabledTime := now.Add(time.Hour)
	perm2 := identity.LocationPermissionRecord{
		AccountID:        accountID,
		Enabled:          false,
		ConsentUpdatedAt: disabledTime,
		LastRegionLabel:  nil,
	}
	if err := repo.UpsertLocationPermission(ctx, perm2); err != nil {
		t.Fatalf("UpsertLocationPermission disable: %v", err)
	}

	rec, err = repo.GetLocationPermission(ctx, accountID)
	if err != nil {
		t.Fatalf("GetLocationPermission after disable: %v", err)
	}
	if rec == nil || rec.Enabled || rec.LastRegionLabel != nil {
		t.Fatalf("expected disabled with nil region, got %+v", rec)
	}
}

func TestPostgresTrustedContacts_LifecycleAndIsolation(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)

	accountA := randomUUID()
	emailA := fmt.Sprintf("trusted-a-%s@ejemplo.invalid", accountA[:8])
	if err := repo.CreateWithTerms(ctx, testAccount(accountA, emailA), nil); err != nil {
		t.Fatalf("create account A: %v", err)
	}

	accountB := randomUUID()
	emailB := fmt.Sprintf("trusted-b-%s@ejemplo.invalid", accountB[:8])
	if err := repo.CreateWithTerms(ctx, testAccount(accountB, emailB), nil); err != nil {
		t.Fatalf("create account B: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_persona_confianza WHERE account_id IN ($1, $2)", accountA, accountB)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id IN ($1, $2)", accountA, accountB)
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Initial counts: 0
	cntA, err := repo.CountActiveTrustedContacts(ctx, accountA)
	if err != nil {
		t.Fatalf("CountActiveTrustedContacts A: %v", err)
	}
	if cntA != 0 {
		t.Fatalf("expected 0 contacts for A, got %d", cntA)
	}

	// 2. Insert contact 1 for Account A
	contactID1 := randomUUID()
	phone1 := "+56987654321"
	c1 := identity.TrustedContactRecord{
		ID:           contactID1,
		AccountID:    accountA,
		FullName:     "Persona Uno",
		Email:        "uno@ejemplo.invalid",
		PhoneE164:    &phone1,
		Relationship: "Hermano",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repo.InsertTrustedContact(ctx, c1); err != nil {
		t.Fatalf("InsertTrustedContact 1: %v", err)
	}

	// 3. Insert contact 2 for Account A (created 1 minute later)
	contactID2 := randomUUID()
	c2 := identity.TrustedContactRecord{
		ID:           contactID2,
		AccountID:    accountA,
		FullName:     "Persona Dos",
		Email:        "dos@ejemplo.invalid",
		Relationship: "Madre",
		Status:       "active",
		CreatedAt:    now.Add(time.Minute),
		UpdatedAt:    now.Add(time.Minute),
	}
	if err := repo.InsertTrustedContact(ctx, c2); err != nil {
		t.Fatalf("InsertTrustedContact 2: %v", err)
	}

	// 4. Verify counts & isolation
	cntA, err = repo.CountActiveTrustedContacts(ctx, accountA)
	if err != nil {
		t.Fatalf("CountActiveTrustedContacts A: %v", err)
	}
	if cntA != 2 {
		t.Fatalf("expected 2 active contacts for A, got %d", cntA)
	}

	cntB, err := repo.CountActiveTrustedContacts(ctx, accountB)
	if err != nil {
		t.Fatalf("CountActiveTrustedContacts B: %v", err)
	}
	if cntB != 0 {
		t.Fatalf("expected 0 contacts for B (isolation), got %d", cntB)
	}

	// 5. List order DESC by created_at: contact 2 must come first
	listA, err := repo.ListActiveTrustedContacts(ctx, accountA)
	if err != nil {
		t.Fatalf("ListActiveTrustedContacts A: %v", err)
	}
	if len(listA) != 2 {
		t.Fatalf("expected 2 contacts in list A, got %d", len(listA))
	}
	if listA[0].ID != contactID2 || listA[1].ID != contactID1 {
		t.Fatalf("expected order [contact2, contact1], got [%s, %s]", listA[0].ID, listA[1].ID)
	}

	// 6. List for B returns empty
	listB, err := repo.ListActiveTrustedContacts(ctx, accountB)
	if err != nil {
		t.Fatalf("ListActiveTrustedContacts B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("expected 0 contacts for B, got %d", len(listB))
	}

	// 7. GetTrustedContact: A can get c1, B gets nil
	got1, err := repo.GetTrustedContact(ctx, accountA, contactID1)
	if err != nil {
		t.Fatalf("GetTrustedContact A: %v", err)
	}
	if got1 == nil || got1.FullName != "Persona Uno" {
		t.Fatalf("expected Persona Uno, got %+v", got1)
	}

	gotB, err := repo.GetTrustedContact(ctx, accountB, contactID1)
	if err != nil {
		t.Fatalf("GetTrustedContact B: %v", err)
	}
	if gotB != nil {
		t.Fatalf("expected nil for cross-account GetTrustedContact, got %+v", gotB)
	}

	// 8. Revoke: B cannot revoke A's contact
	err = repo.RevokeTrustedContact(ctx, accountB, contactID1, now.Add(2*time.Minute))
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-account revoke, got %v", err)
	}

	// A revokes c1
	revokeTime := now.Add(2 * time.Minute)
	if err := repo.RevokeTrustedContact(ctx, accountA, contactID1, revokeTime); err != nil {
		t.Fatalf("RevokeTrustedContact A: %v", err)
	}

	// Verify c1 is no longer in active list
	listA, err = repo.ListActiveTrustedContacts(ctx, accountA)
	if err != nil {
		t.Fatalf("ListActiveTrustedContacts after revoke: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != contactID2 {
		t.Fatalf("expected only contact2 active, got %+v", listA)
	}

	// Double revoke fails
	err = repo.RevokeTrustedContact(ctx, accountA, contactID1, revokeTime)
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for double revoke, got %v", err)
	}
}
