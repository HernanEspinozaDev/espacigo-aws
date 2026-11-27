package privacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type executionMemoryRepository struct {
	memoryRepository
	ownData      OwnData
	prefs        map[string]PurposePreference
	restrictions []TreatmentRestriction
	blocks       map[string]bool
}

func (m *executionMemoryRepository) ExportOwnData(_ context.Context, accountID string) (OwnData, error) {
	if m.ownData.Account.Email != "" {
		return m.ownData, nil
	}
	return m.memoryRepository.ExportOwnData(context.Background(), accountID)
}

func (m *executionMemoryRepository) ListOwnPurposePreferences(_ context.Context, _ string) ([]PurposePreference, error) {
	out := make([]PurposePreference, 0, len(m.prefs))
	for _, pref := range m.prefs {
		out = append(out, pref)
	}
	return out, nil
}

func (m *executionMemoryRepository) UpsertOwnPurposePreference(_ context.Context, _ string, in UpdatePurposePreferenceInput, now time.Time) (PurposePreference, error) {
	if m.prefs == nil {
		m.prefs = map[string]PurposePreference{}
	}
	var reason *string
	if in.Reason != "" {
		r := in.Reason
		reason = &r
	}
	ts := now.UTC()
	pref := PurposePreference{
		Purpose:   in.Purpose,
		State:     in.State,
		Reason:    reason,
		UpdatedAt: &ts,
	}
	m.prefs[in.Purpose] = pref
	return pref, nil
}

func (m *executionMemoryRepository) ListOwnTreatmentRestrictions(_ context.Context, _ string) ([]TreatmentRestriction, error) {
	return m.restrictions, nil
}

func (m *executionMemoryRepository) HasActiveTreatmentBlock(_ context.Context, accountID, scope string) (bool, error) {
	if m.blocks[accountID+":"+ScopeGeneralTreatment] {
		return true, nil
	}
	return m.blocks[accountID+":"+scope], nil
}

func TestPurposeAndScopeValidators(t *testing.T) {
	purposes := AllPurposes()
	if len(purposes) != 4 {
		t.Fatalf("AllPurposes len = %d, want 4", len(purposes))
	}
	// Verify returned slice is a defensive copy.
	purposes[0] = "mutated"
	if AllPurposes()[0] != PurposeCommercialComms {
		t.Fatal("AllPurposes returned shared backing array")
	}

	for _, p := range AllPurposes() {
		if !ValidPurpose(p) {
			t.Errorf("ValidPurpose(%q) = false, want true", p)
		}
	}
	if ValidPurpose("otra_finalidad") || ValidPurpose("") {
		t.Fatal("ValidPurpose accepted invalid purpose")
	}

	for _, s := range []string{PurposeStateAllowed, PurposeStateOpposed, PurposeStateWithdrawn} {
		if !ValidPurposeState(s) {
			t.Errorf("ValidPurposeState(%q) = false, want true", s)
		}
	}
	if ValidPurposeState("bloqueado") || ValidPurposeState("") {
		t.Fatal("ValidPurposeState accepted invalid state")
	}

	for _, sc := range []string{
		ScopeCommercialComms, ScopeAnalyticsProfile, ScopePublicationsVis,
		ScopeExportsThirdParty, ScopeGeneralTreatment,
	} {
		if !ValidTreatmentScope(sc) {
			t.Errorf("ValidTreatmentScope(%q) = false, want true", sc)
		}
	}
	if ValidTreatmentScope("desconocido") || ValidTreatmentScope("") {
		t.Fatal("ValidTreatmentScope accepted invalid scope")
	}
}

func TestValidateProfileRectification(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	if _, _, ok := ValidateProfileRectification(nil, nil); ok {
		t.Fatal("expected false when both displayName and phone are nil")
	}

	name, phone, ok := ValidateProfileRectification(strPtr("  María José  "), strPtr(" 912345678 "))
	if !ok || name == nil || *name != "María José" || phone == nil || *phone != "912345678" {
		t.Fatalf("unexpected valid rectification: name=%v phone=%v ok=%t", name, phone, ok)
	}

	// Clearing phone to empty string is allowed (maps to SQL NULL in repository).
	name, phone, ok = ValidateProfileRectification(nil, strPtr("   "))
	if !ok || name != nil || phone == nil || *phone != "" {
		t.Fatalf("expected clean empty phone for clearing, got name=%v phone=%v ok=%t", name, phone, ok)
	}

	// Name only, max 120 runes.
	longValid := strings.Repeat("á", 120)
	name, phone, ok = ValidateProfileRectification(&longValid, nil)
	if !ok || name == nil || phone != nil {
		t.Fatalf("expected 120-rune valid name, got ok=%t", ok)
	}

	invalidCases := []struct {
		name  *string
		phone *string
	}{
		{name: strPtr("   "), phone: nil},
		{name: strPtr("Ada 42"), phone: nil},
		{name: strPtr("Ada-Lovelace"), phone: nil},
		{name: strPtr(strings.Repeat("a", 121)), phone: nil},
		{name: strPtr("\xff"), phone: nil},
		{name: nil, phone: strPtr("12345678")},
		{name: nil, phone: strPtr("1234567890")},
		{name: nil, phone: strPtr("91234567a")},
		{name: nil, phone: strPtr("+56912345678")},
	}
	for i, tc := range invalidCases {
		if _, _, ok := ValidateProfileRectification(tc.name, tc.phone); ok {
			t.Errorf("case %d unexpectedly succeeded", i)
		}
	}
}

func TestOwnPurposePreferencesAndUpdateValidation(t *testing.T) {
	ctx := context.Background()
	fixedNow := time.Date(2026, time.November, 27, 18, 0, 0, 123456789, time.UTC)

	legacySvc, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacySvc.OwnPurposePreferences(ctx, "acc-1"); err == nil {
		t.Fatal("expected error when repo does not implement RightsExecutionRepository")
	}
	if _, err := legacySvc.UpdateOwnPurposePreference(ctx, "acc-1", UpdatePurposePreferenceInput{
		Purpose: PurposeCommercialComms,
		State:   PurposeStateOpposed,
	}); err == nil {
		t.Fatal("expected error on update when repo does not implement RightsExecutionRepository")
	}

	repo := &executionMemoryRepository{
		prefs: map[string]PurposePreference{
			PurposeProductAnalytics: {Purpose: PurposeProductAnalytics, State: PurposeStateWithdrawn},
		},
	}
	svc, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return fixedNow }

	if _, err := svc.OwnPurposePreferences(ctx, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty accountID err = %v, want ErrInvalid", err)
	}

	prefs, err := svc.OwnPurposePreferences(ctx, "acc-1")
	if err != nil {
		t.Fatalf("OwnPurposePreferences err = %v", err)
	}
	if len(prefs) != 4 {
		t.Fatalf("prefs len = %d, want 4", len(prefs))
	}
	if prefs[0].Purpose != PurposeCommercialComms || prefs[0].State != PurposeStateAllowed {
		t.Fatalf("expected default permitido for comunicaciones_comerciales, got %+v", prefs[0])
	}
	if prefs[1].Purpose != PurposeProductAnalytics || prefs[1].State != PurposeStateWithdrawn {
		t.Fatalf("expected persisted retirado for analitica_producto, got %+v", prefs[1])
	}

	// Invalid updates.
	for _, bad := range []UpdatePurposePreferenceInput{
		{Purpose: "invalid", State: PurposeStateOpposed},
		{Purpose: PurposeCommercialComms, State: "invalid"},
		{Purpose: PurposeCommercialComms, State: PurposeStateOpposed, Reason: "abcd"},
		{Purpose: PurposeCommercialComms, State: PurposeStateOpposed, Reason: strings.Repeat("x", 501)},
		{Purpose: PurposeCommercialComms, State: PurposeStateOpposed, Reason: "motivo\x00nulo"},
	} {
		if _, err := svc.UpdateOwnPurposePreference(ctx, "acc-1", bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("bad input %+v err = %v, want ErrInvalid", bad, err)
		}
	}

	updated, err := svc.UpdateOwnPurposePreference(ctx, "acc-1", UpdatePurposePreferenceInput{
		Purpose: PurposeCommercialComms,
		State:   PurposeStateOpposed,
		Reason:  "  No deseo recibir correos promocionales  ",
	})
	if err != nil {
		t.Fatalf("UpdateOwnPurposePreference err = %v", err)
	}
	if updated.State != PurposeStateOpposed || updated.Reason == nil || *updated.Reason != "No deseo recibir correos promocionales" {
		t.Fatalf("unexpected updated pref: %+v", updated)
	}
	if updated.UpdatedAt == nil || !updated.UpdatedAt.Equal(fixedNow.Truncate(time.Microsecond)) {
		t.Fatalf("unexpected updated timestamp: %v", updated.UpdatedAt)
	}

	// Empty reason is allowed.
	updatedEmptyReason, err := svc.UpdateOwnPurposePreference(ctx, "acc-1", UpdatePurposePreferenceInput{
		Purpose: PurposeCommercialComms,
		State:   PurposeStateAllowed,
	})
	if err != nil || updatedEmptyReason.Reason != nil {
		t.Fatalf("expected empty reason update to succeed with nil Reason, got %+v err=%v", updatedEmptyReason, err)
	}
}

func TestBuildPortabilityJSONAndExportOwnPortability(t *testing.T) {
	ctx := context.Background()
	fixedNow := time.Date(2026, time.November, 27, 19, 30, 0, 0, time.UTC)

	repo := &executionMemoryRepository{
		ownData: OwnData{
			Account: ExportAccount{
				Email:     "titular@ejemplo.cl",
				State:     "activo",
				CreatedAt: fixedNow.Add(-24 * time.Hour),
			},
			Roles:       nil,
			Acceptances: nil,
			Requests:    nil,
			Scope:       "identidad_local_v1",
		},
	}
	svc, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return fixedNow }

	raw1, digest1, err := svc.ExportOwnPortability(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ExportOwnPortability err = %v", err)
	}
	raw2, digest2, err := svc.ExportOwnPortability(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ExportOwnPortability second call err = %v", err)
	}
	if string(raw1) != string(raw2) || digest1 != digest2 {
		t.Fatal("portability JSON output or digest is not deterministic")
	}
	if len(digest1) != 64 {
		t.Fatalf("digest len = %d, want 64", len(digest1))
	}
	sum := sha256.Sum256(raw1)
	if hex.EncodeToString(sum[:]) != digest1 {
		t.Fatalf("digest mismatch: got %s want %s", digest1, hex.EncodeToString(sum[:]))
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw1, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema_version"] != PortabilitySchemaVersion {
		t.Fatalf("schema_version = %v, want %s", decoded["schema_version"], PortabilitySchemaVersion)
	}
	for _, sliceKey := range []string{"roles", "terms_acceptances", "purpose_preferences", "treatment_restrictions", "rights_requests"} {
		val, exists := decoded[sliceKey]
		if !exists || val == nil {
			t.Fatalf("field %q missing or null in portability JSON: %s", sliceKey, raw1)
		}
		if _, isSlice := val.([]any); !isSlice {
			t.Fatalf("field %q is not a JSON array: %T", sliceKey, val)
		}
	}
	for _, forbidden := range []string{"password_hash", "nota_interna", "secret", "token"} {
		if strings.Contains(string(raw1), forbidden) {
			t.Fatalf("portability JSON leaked forbidden string %q", forbidden)
		}
	}

	// Verify OwnTreatmentRestrictions normalizes nil slice to empty slice.
	restrictions, err := svc.OwnTreatmentRestrictions(ctx, "acc-1")
	if err != nil || restrictions == nil || len(restrictions) != 0 {
		t.Fatalf("expected non-nil empty restrictions, got %#v err=%v", restrictions, err)
	}
}
