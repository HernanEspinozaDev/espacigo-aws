package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type mockLocationTrustedRepo struct {
	mockAlertsRepo
	locationRecords map[string]*identity.LocationPermissionRecord
	trustedContacts map[string][]identity.TrustedContactRecord
	events          []identity.SecurityAlertEventRecord
	mockEmail       string
	phoneRecord     *identity.AccountPhoneVerification
	totpRecord      *identity.AccountTOTPRecord
	passkeys        []identity.PasskeyRecord
	qrDevice        *identity.AccountQRDeviceRecord
}

func newMockLocationTrustedRepo() *mockLocationTrustedRepo {
	return &mockLocationTrustedRepo{
		locationRecords: make(map[string]*identity.LocationPermissionRecord),
		trustedContacts: make(map[string][]identity.TrustedContactRecord),
	}
}

func (m *mockLocationTrustedRepo) GetLocationPermission(_ context.Context, accountID string) (*identity.LocationPermissionRecord, error) {
	if rec, ok := m.locationRecords[accountID]; ok {
		return rec, nil
	}
	return nil, nil
}

func (m *mockLocationTrustedRepo) UpsertLocationPermission(_ context.Context, record identity.LocationPermissionRecord) error {
	rec := record
	m.locationRecords[record.AccountID] = &rec
	return nil
}

func (m *mockLocationTrustedRepo) ListActiveTrustedContacts(_ context.Context, accountID string) ([]identity.TrustedContactRecord, error) {
	contacts := m.trustedContacts[accountID]
	active := make([]identity.TrustedContactRecord, 0, len(contacts))
	for _, c := range contacts {
		if c.Status == "active" {
			active = append(active, c)
		}
	}
	return active, nil
}

func (m *mockLocationTrustedRepo) CountActiveTrustedContacts(_ context.Context, accountID string) (int, error) {
	contacts := m.trustedContacts[accountID]
	count := 0
	for _, c := range contacts {
		if c.Status == "active" {
			count++
		}
	}
	return count, nil
}

func (m *mockLocationTrustedRepo) GetTrustedContact(_ context.Context, accountID, contactID string) (*identity.TrustedContactRecord, error) {
	for _, c := range m.trustedContacts[accountID] {
		if c.ID == contactID {
			rec := c
			return &rec, nil
		}
	}
	return nil, nil
}

func (m *mockLocationTrustedRepo) InsertTrustedContact(_ context.Context, contact identity.TrustedContactRecord) error {
	m.trustedContacts[contact.AccountID] = append(m.trustedContacts[contact.AccountID], contact)
	return nil
}

func (m *mockLocationTrustedRepo) RevokeTrustedContact(_ context.Context, accountID, contactID string, updatedAt time.Time) error {
	contacts := m.trustedContacts[accountID]
	for i, c := range contacts {
		if c.ID == contactID && c.Status == "active" {
			contacts[i].Status = "revoked"
			contacts[i].UpdatedAt = updatedAt
			return nil
		}
	}
	return identity.ErrNotFound
}

func (m *mockLocationTrustedRepo) RecordSecurityAlertEvent(_ context.Context, event identity.SecurityAlertEventRecord) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockLocationTrustedRepo) ActiveAccountEmail(_ context.Context, _ string) (string, error) {
	return m.mockEmail, nil
}

func (m *mockLocationTrustedRepo) GetPhoneVerification(_ context.Context, _ string) (*identity.AccountPhoneVerification, error) {
	return m.phoneRecord, nil
}

func (m *mockLocationTrustedRepo) GetTOTP(_ context.Context, _ string) (*identity.AccountTOTPRecord, int, error) {
	return m.totpRecord, 1, nil
}

func (m *mockLocationTrustedRepo) ListPasskeys(_ context.Context, _ string) ([]identity.PasskeyRecord, error) {
	return m.passkeys, nil
}

func (m *mockLocationTrustedRepo) GetQRDevice(_ context.Context, _ string) (*identity.AccountQRDeviceRecord, error) {
	return m.qrDevice, nil
}

func setupLocationTrustedService(repo *mockLocationTrustedRepo) *identity.AuthenticationService {
	svc, err := identity.NewAuthenticationService(
		repo,
		alertsTestHasher{},
		&alertsTestMailer{},
		&alertsTestLimiter{},
		&alertsTestCredGen{},
		func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestLocationPermission_Lifecycle(t *testing.T) {
	repo := newMockLocationTrustedRepo()
	svc := setupLocationTrustedService(repo)
	ctx := context.Background()
	accountID := "acc-loc-01"

	// 1. Initial state: enabled = false
	res, err := svc.GetLocationPermission(ctx, accountID)
	if err != nil {
		t.Fatalf("GetLocationPermission initially failed: %v", err)
	}
	if res.Enabled {
		t.Fatalf("expected Enabled: false, got true")
	}
	if res.ConsentUpdatedAt != nil {
		t.Fatalf("expected nil ConsentUpdatedAt, got %v", res.ConsentUpdatedAt)
	}

	// 2. Reject GPS coordinates
	gpsCoords := "-33.4372, -70.6506"
	_, err = svc.UpdateLocationPermission(ctx, accountID, true, &gpsCoords, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrCoordinatesNotAllowed) {
		t.Fatalf("expected ErrCoordinatesNotAllowed for GPS coordinates, got %v", err)
	}

	// 3. Reject GPS keywords
	gpsKeyword := "lat: -33.4372"
	_, err = svc.UpdateLocationPermission(ctx, accountID, true, &gpsKeyword, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrCoordinatesNotAllowed) {
		t.Fatalf("expected ErrCoordinatesNotAllowed for GPS keyword, got %v", err)
	}

	// 4. Reject label > 80 runes
	longLabel := strings.Repeat("A", 81)
	_, err = svc.UpdateLocationPermission(ctx, accountID, true, &longLabel, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrInvalidRegionLabel) {
		t.Fatalf("expected ErrInvalidRegionLabel for long label, got %v", err)
	}

	// 5. Update with valid label containing CRLF that should be stripped
	rawLabel := "Región Metropolitana,\r\n Chile"
	updated, err := svc.UpdateLocationPermission(ctx, accountID, true, &rawLabel, "Chrome en Linux", "190.162.1.1")
	if err != nil {
		t.Fatalf("UpdateLocationPermission failed: %v", err)
	}
	if !updated.Enabled {
		t.Fatalf("expected Enabled: true")
	}
	if updated.LastRegionLabel == nil || *updated.LastRegionLabel != "Región Metropolitana, Chile" {
		t.Fatalf("expected sanitized 'Región Metropolitana, Chile', got %v", updated.LastRegionLabel)
	}

	// Verify security audit event
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(repo.events))
	}
	if repo.events[0].EventCode != "location_permission_updated" {
		t.Fatalf("expected event code 'location_permission_updated', got %q", repo.events[0].EventCode)
	}

	// 6. Get location returns updated state
	res, err = svc.GetLocationPermission(ctx, accountID)
	if err != nil {
		t.Fatalf("GetLocationPermission failed: %v", err)
	}
	if !res.Enabled || res.LastRegionLabel == nil || *res.LastRegionLabel != "Región Metropolitana, Chile" {
		t.Fatalf("expected Enabled: true and Region, got %+v", res)
	}

	// 7. Disable permission
	disabled, err := svc.UpdateLocationPermission(ctx, accountID, false, nil, "Chrome en Linux", "190.162.1.1")
	if err != nil {
		t.Fatalf("UpdateLocationPermission(false) failed: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected Enabled: false")
	}
}

func TestTrustedContacts_LifecycleAndLimits(t *testing.T) {
	repo := newMockLocationTrustedRepo()
	svc := setupLocationTrustedService(repo)
	ctx := context.Background()
	accountID := "acc-trusted-01"

	// 1. Initial list empty
	list, err := svc.ListTrustedContacts(ctx, accountID)
	if err != nil {
		t.Fatalf("ListTrustedContacts: %v", err)
	}
	if len(list.Contacts) != 0 {
		t.Fatalf("expected 0 contacts, got %d", len(list.Contacts))
	}

	// 2. Validation failures
	// Short name
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "A",
		Email:        "contacto1@example.com",
		Relationship: "Hermano",
	}, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrInvalidTrustedContactName) {
		t.Fatalf("expected ErrInvalidTrustedContactName, got %v", err)
	}

	// Invalid email
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Juan Pérez",
		Email:        "invalid-email-address",
		Relationship: "Hermano",
	}, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrInvalidEmail) {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}

	// Invalid relationship
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Juan Pérez",
		Email:        "contacto1@example.com",
		Relationship: "X",
	}, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrInvalidRelationship) {
		t.Fatalf("expected ErrInvalidRelationship, got %v", err)
	}

	// Invalid phone
	badPhone := "12345"
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Juan Pérez",
		Email:        "contacto1@example.com",
		Phone:        &badPhone,
		Relationship: "Hermano",
	}, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrInvalidPhone) {
		t.Fatalf("expected ErrInvalidPhone, got %v", err)
	}

	// 3. Add Contact 1 with Chilean phone normalization
	rawPhone1 := "+56 9 8765 4321"
	c1, err := svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "María González",
		Email:        "maria.gonzalez@example.com",
		Phone:        &rawPhone1,
		Relationship: "Hermana",
	}, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("AddTrustedContact 1: %v", err)
	}
	if c1.MaskedEmail != "m••••m" {
		t.Fatalf("expected masked email 'm••••m', got %q", c1.MaskedEmail)
	}
	if c1.MaskedPhone != "+56 ••••4321" {
		t.Fatalf("expected masked phone '+56 ••••4321', got %q", c1.MaskedPhone)
	}

	// 4. Add Contact 2 without phone (optional)
	c2, err := svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Carlos Muñoz",
		Email:        "carlos@example.com",
		Relationship: "Padre",
	}, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("AddTrustedContact 2: %v", err)
	}
	if c2.MaskedPhone != "" {
		t.Fatalf("expected empty masked phone, got %q", c2.MaskedPhone)
	}

	// 5. Add Contact 3
	c3, err := svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Lucía Soto",
		Email:        "lucia@example.com",
		Relationship: "Amiga",
	}, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("AddTrustedContact 3: %v", err)
	}
	_ = c3

	// 6. 4th contact must fail with ErrTrustedContactLimit
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Pedro Silva",
		Email:        "pedro@example.com",
		Relationship: "Abogado",
	}, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrTrustedContactLimit) {
		t.Fatalf("expected ErrTrustedContactLimit, got %v", err)
	}

	// Verify list returns 3 contacts masked
	list, err = svc.ListTrustedContacts(ctx, accountID)
	if err != nil {
		t.Fatalf("ListTrustedContacts: %v", err)
	}
	if len(list.Contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(list.Contacts))
	}
	for _, c := range list.Contacts {
		if strings.Contains(c.MaskedEmail, "@") {
			t.Fatalf("masked email leaked plain address: %s", c.MaskedEmail)
		}
	}

	// 7. Remove Contact 2
	revResp, err := svc.RemoveTrustedContact(ctx, accountID, c2.ID, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("RemoveTrustedContact: %v", err)
	}
	if revResp.Status != "revoked" || revResp.ContactID != c2.ID {
		t.Fatalf("expected revoked status for %s, got %+v", c2.ID, revResp)
	}

	// Verify list now has 2 contacts
	list, err = svc.ListTrustedContacts(ctx, accountID)
	if err != nil {
		t.Fatalf("ListTrustedContacts after removal: %v", err)
	}
	if len(list.Contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(list.Contacts))
	}

	// 8. Try removing non-existent or already revoked contact
	_, err = svc.RemoveTrustedContact(ctx, accountID, c2.ID, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound removing already revoked contact, got %v", err)
	}

	// 9. Now adding a 3rd contact should succeed since active count is 2
	_, err = svc.AddTrustedContact(ctx, accountID, identity.AddTrustedContactInput{
		FullName:     "Pedro Silva",
		Email:        "pedro@example.com",
		Relationship: "Abogado",
	}, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("AddTrustedContact after slot freed failed: %v", err)
	}
}

func TestSecurityMethodsSummary(t *testing.T) {
	repo := newMockLocationTrustedRepo()
	svc := setupLocationTrustedService(repo)
	ctx := context.Background()
	accountID := "acc-summary-01"

	repo.mockEmail = "usuario@espacigo.com"

	// Baseline: password (1) + email (1) = 2 active -> protected: false
	sum, err := svc.GetSecurityMethodsSummary(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityMethodsSummary: %v", err)
	}
	if sum.TotalCount != 6 {
		t.Fatalf("expected TotalCount 6, got %d", sum.TotalCount)
	}
	if sum.ActiveCount != 2 {
		t.Fatalf("expected ActiveCount 2, got %d", sum.ActiveCount)
	}
	if sum.Protected {
		t.Fatalf("expected Protected: false with only password+email")
	}
	if sum.Methods.FacialKYC.Enabled || sum.Methods.FacialKYC.Status != "pending_kyc_module" {
		t.Fatalf("expected FacialKYC disabled with pending_kyc_module, got %+v", sum.Methods.FacialKYC)
	}

	// Activate phone verification: password + email + phone = 3 -> protected: true
	verifiedAt := time.Now().UTC()
	repo.phoneRecord = &identity.AccountPhoneVerification{
		AccountID:  accountID,
		PhoneE164:  "+56987654321",
		VerifiedAt: &verifiedAt,
	}

	sum, err = svc.GetSecurityMethodsSummary(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityMethodsSummary: %v", err)
	}
	if sum.ActiveCount != 3 {
		t.Fatalf("expected ActiveCount 3, got %d", sum.ActiveCount)
	}
	if !sum.Protected {
		t.Fatalf("expected Protected: true with phone verified")
	}
	if !sum.Methods.Phone.Enabled || sum.Methods.Phone.MaskedPhone != "+56 ••••4321" {
		t.Fatalf("expected phone enabled with masked phone, got %+v", sum.Methods.Phone)
	}

	// Activate TOTP as well -> active: 4, protected: true
	totpEnabledAt := time.Now().UTC()
	repo.totpRecord = &identity.AccountTOTPRecord{
		AccountID: accountID,
		EnabledAt: &totpEnabledAt,
	}
	sum, err = svc.GetSecurityMethodsSummary(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityMethodsSummary: %v", err)
	}
	if sum.ActiveCount != 4 {
		t.Fatalf("expected ActiveCount 4, got %d", sum.ActiveCount)
	}
	if !sum.Methods.TOTP.Enabled {
		t.Fatalf("expected TOTP enabled")
	}

	// Add 2 passkeys -> active: 5
	repo.passkeys = []identity.PasskeyRecord{
		{ID: "pk-1", AccountID: accountID},
		{ID: "pk-2", AccountID: accountID},
	}
	sum, err = svc.GetSecurityMethodsSummary(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityMethodsSummary: %v", err)
	}
	if sum.ActiveCount != 5 {
		t.Fatalf("expected ActiveCount 5, got %d", sum.ActiveCount)
	}
	if !sum.Methods.Passkeys.Enabled || sum.Methods.Passkeys.Count != 2 {
		t.Fatalf("expected Passkeys enabled with count 2, got %+v", sum.Methods.Passkeys)
	}

	// Add QR paired device -> active: 6
	qrEnabledAt := time.Now().UTC()
	repo.qrDevice = &identity.AccountQRDeviceRecord{
		AccountID: accountID,
		EnabledAt: &qrEnabledAt,
	}
	sum, err = svc.GetSecurityMethodsSummary(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityMethodsSummary: %v", err)
	}
	if sum.ActiveCount != 6 {
		t.Fatalf("expected ActiveCount 6, got %d", sum.ActiveCount)
	}
	if !sum.Methods.QR.Enabled {
		t.Fatalf("expected QR enabled")
	}
}

func TestTrustedContacts_AccountIsolation(t *testing.T) {
	repo := newMockLocationTrustedRepo()
	svc := setupLocationTrustedService(repo)
	ctx := context.Background()

	accountA := "acc-tenant-a"
	accountB := "acc-tenant-b"

	c, err := svc.AddTrustedContact(ctx, accountA, identity.AddTrustedContactInput{
		FullName:     "Contacto de A",
		Email:        "contacto.a@example.com",
		Relationship: "Colega",
	}, "Chrome", "190.162.1.1")
	if err != nil {
		t.Fatalf("AddTrustedContact: %v", err)
	}

	// Account B must not see Account A's contact
	listB, err := svc.ListTrustedContacts(ctx, accountB)
	if err != nil {
		t.Fatalf("ListTrustedContacts B: %v", err)
	}
	if len(listB.Contacts) != 0 {
		t.Fatalf("account B saw account A's contacts: %+v", listB.Contacts)
	}

	// Account B must not be able to delete Account A's contact
	_, err = svc.RemoveTrustedContact(ctx, accountB, c.ID, "Chrome", "190.162.1.1")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-account removal, got %v", err)
	}
}
