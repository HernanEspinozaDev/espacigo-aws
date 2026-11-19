package postgres_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestPostgresSecurityAlerts_Flow(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	accountID := randomUUID()
	email := fmt.Sprintf("alerts-%s@ejemplo.invalid", accountID[:8])
	account := testAccount(accountID, email)
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create account: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_alerta_evento WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_alerta_preferencia WHERE account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id = $1", accountID)
	}()

	// 1. Initial preferences should be nil (not found yet)
	pref, err := repo.GetSecurityAlertPreferences(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityAlertPreferences before upsert: %v", err)
	}
	if pref != nil {
		t.Fatalf("expected nil preferences before first write, got %+v", pref)
	}

	// 2. Initial events should be empty
	events, err := repo.ListSecurityAlertEvents(ctx, accountID, 20)
	if err != nil {
		t.Fatalf("ListSecurityAlertEvents before records: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}

	// 3. Upsert preferences
	extraEmail := "secundario@ejemplo.invalid"
	verifiedAt := now.Add(-1 * time.Hour)
	record := identity.SecurityAlertPreferencesRecord{
		AccountID:               accountID,
		AppNotificationsEnabled: true,
		EmailAlertsEnabled:      false,
		WhatsAppAlertsEnabled:   true,
		ExtraEmail:              &extraEmail,
		ExtraEmailVerifiedAt:    &verifiedAt,
		UpdatedAt:               now,
	}
	if err := repo.UpsertSecurityAlertPreferences(ctx, record); err != nil {
		t.Fatalf("UpsertSecurityAlertPreferences: %v", err)
	}

	// 4. Retrieve preferences
	pref, err = repo.GetSecurityAlertPreferences(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityAlertPreferences after upsert: %v", err)
	}
	if pref == nil {
		t.Fatal("expected non-nil preferences after upsert")
	}
	if pref.AccountID != accountID {
		t.Errorf("expected account_id %s, got %s", accountID, pref.AccountID)
	}
	if !pref.AppNotificationsEnabled {
		t.Error("expected AppNotificationsEnabled to be true")
	}
	if pref.EmailAlertsEnabled {
		t.Error("expected EmailAlertsEnabled to be false")
	}
	if !pref.WhatsAppAlertsEnabled {
		t.Error("expected WhatsAppAlertsEnabled to be true")
	}
	if pref.ExtraEmail == nil || *pref.ExtraEmail != extraEmail {
		t.Errorf("expected extra_email %q, got %v", extraEmail, pref.ExtraEmail)
	}
	if pref.ExtraEmailVerifiedAt == nil || !pref.ExtraEmailVerifiedAt.Equal(verifiedAt) {
		t.Errorf("expected extra_email_verified_at %v, got %v", verifiedAt, pref.ExtraEmailVerifiedAt)
	}

	// 5. Update preferences again (ON CONFLICT DO UPDATE)
	updatedNow := now.Add(10 * time.Minute)
	record.EmailAlertsEnabled = true
	record.ExtraEmail = nil
	record.ExtraEmailVerifiedAt = nil
	record.UpdatedAt = updatedNow
	if err := repo.UpsertSecurityAlertPreferences(ctx, record); err != nil {
		t.Fatalf("UpsertSecurityAlertPreferences update: %v", err)
	}

	pref, err = repo.GetSecurityAlertPreferences(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSecurityAlertPreferences after second upsert: %v", err)
	}
	if !pref.EmailAlertsEnabled {
		t.Error("expected EmailAlertsEnabled to be true after update")
	}
	if pref.ExtraEmail != nil {
		t.Errorf("expected extra_email nil after clearing, got %v", *pref.ExtraEmail)
	}

	// 6. Record security alert events
	event1ID := randomUUID()
	event1 := identity.SecurityAlertEventRecord{
		ID:          event1ID,
		AccountID:   accountID,
		EventCode:   "alert_channel_updated",
		Title:       "Canales de alerta actualizados",
		DeviceLabel: "Chrome en Linux",
		IPMasked:    "190.162.•••.•••",
		CreatedAt:   now.Add(-5 * time.Minute),
	}
	if err := repo.RecordSecurityAlertEvent(ctx, event1); err != nil {
		t.Fatalf("RecordSecurityAlertEvent 1: %v", err)
	}

	event2ID := randomUUID()
	event2 := identity.SecurityAlertEventRecord{
		ID:          event2ID,
		AccountID:   accountID,
		EventCode:   "session_revoked",
		Title:       "Sesión de dispositivo revocada",
		DeviceLabel: "Safari en iPhone",
		IPMasked:    "200.54.•••.•••",
		CreatedAt:   now.Add(-1 * time.Minute),
	}
	if err := repo.RecordSecurityAlertEvent(ctx, event2); err != nil {
		t.Fatalf("RecordSecurityAlertEvent 2: %v", err)
	}

	// 7. List events: must be sorted by created_at DESC (event2 then event1)
	events, err = repo.ListSecurityAlertEvents(ctx, accountID, 10)
	if err != nil {
		t.Fatalf("ListSecurityAlertEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != event2ID {
		t.Errorf("expected first event to be event2 (newer), got %s", events[0].ID)
	}
	if events[1].ID != event1ID {
		t.Errorf("expected second event to be event1 (older), got %s", events[1].ID)
	}
	if events[0].EventCode != "session_revoked" {
		t.Errorf("unexpected event_code: %s", events[0].EventCode)
	}
	if !strings.Contains(events[0].IPMasked, "••") {
		t.Errorf("expected masked ip, got %s", events[0].IPMasked)
	}

	// 8. Test limit
	limitedEvents, err := repo.ListSecurityAlertEvents(ctx, accountID, 1)
	if err != nil {
		t.Fatalf("ListSecurityAlertEvents limit 1: %v", err)
	}
	if len(limitedEvents) != 1 {
		t.Fatalf("expected 1 event with limit 1, got %d", len(limitedEvents))
	}
	if limitedEvents[0].ID != event2ID {
		t.Errorf("expected event2 with limit 1, got %s", limitedEvents[0].ID)
	}
}
