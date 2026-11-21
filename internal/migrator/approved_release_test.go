package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestApprovedReleasePolicyChecksAllPendingMigrations(t *testing.T) {
	var policy releasePolicy
	err := json.Unmarshal([]byte(`{
		"format":"espacigo.initial-migration-policy.v1",
		"baseline":83,
		"automatic_migrations":{"V000101__expand.sql":{"checksum":"approved","strategy":"expand"}},
		"review_required_migrations":{"V000102__review.sql":{"automatic":false}}
	}`), &policy)
	if err != nil {
		t.Fatal(err)
	}
	approved := MigrationSummary{Name: "V000101__expand.sql", Checksum: "approved"}
	for _, tc := range []struct {
		name    string
		history bool
		applied int
		pending []MigrationSummary
		wantErr bool
	}{
		{name: "approved", history: true, applied: 100, pending: []MigrationSummary{approved}},
		{name: "noop", history: true, applied: 101},
		{name: "empty database", applied: 0, wantErr: true},
		{name: "initial ledger incomplete", history: true, applied: 83, wantErr: true},
		{name: "changed SQL", history: true, applied: 100, pending: []MigrationSummary{{Name: approved.Name, Checksum: "different"}}, wantErr: true},
		{name: "later review blocks the entire batch", history: true, applied: 100, pending: []MigrationSummary{approved, {Name: "V000102__review.sql"}}, wantErr: true},
		{name: "unclassified", history: true, applied: 100, pending: []MigrationSummary{{Name: "V000103__unclassified.sql"}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := PlanResult{HistoryExists: tc.history, Applied: make([]HistoryEntry, tc.applied), Pending: tc.pending}
			if err := policy.checkPending(plan); (err != nil) != tc.wantErr {
				t.Fatalf("unexpected policy result: %v", err)
			}
		})
	}
}

type ownerRow struct {
	owner string
	err   error
}

func (r ownerRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*string)) = r.owner
	return nil
}

type ownerQueryer struct{ owner string }

func (q ownerQueryer) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	if len(args) != 2 || args[0] != "public" || args[1] != "sesion" {
		return ownerRow{err: errors.New("unexpected target")}
	}
	return ownerRow{owner: q.owner}
}

func TestPendingSchemaOwnerPreflight(t *testing.T) {
	policy := releasePolicy{Automatic: map[string]struct {
		Checksum            string   `json:"checksum"`
		Strategy            string   `json:"strategy"`
		RequiresSchemaOwner []string `json:"requires_schema_owner,omitempty"`
	}{"V000104__session_device_metadata.sql": {RequiresSchemaOwner: []string{"public.sesion"}}}}
	pending := []MigrationSummary{{Name: "V000104__session_device_metadata.sql"}}
	if err := checkPendingSchemaOwners(context.Background(), ownerQueryer{owner: "espacigo_schema_owner"}, policy, pending); err != nil {
		t.Fatalf("expected approved owner: %v", err)
	}
	if err := checkPendingSchemaOwners(context.Background(), ownerQueryer{owner: "espacigo_admin"}, policy, pending); err == nil {
		t.Fatal("expected historical owner to block pending migration")
	}
	if got := pendingOwnerTargets(policy, nil); len(got) != 0 {
		t.Fatalf("applied migration should not trigger owner preflight: %#v", got)
	}
}

func TestReleasePolicyRejectsUnsafeOwnerTarget(t *testing.T) {
	for _, target := range []string{"sesion", "other.sesion", "public.sesion; DROP TABLE public.foo", "public.bad-name"} {
		policy := releasePolicy{
			Format:   "espacigo.initial-migration-policy.v1",
			Baseline: 100,
			Automatic: map[string]struct {
				Checksum            string   `json:"checksum"`
				Strategy            string   `json:"strategy"`
				RequiresSchemaOwner []string `json:"requires_schema_owner,omitempty"`
			}{"V000104__session_device_metadata.sql": {Checksum: "ok", Strategy: "expand", RequiresSchemaOwner: []string{target}}},
		}
		plan := PlanResult{HistoryExists: true, Applied: make([]HistoryEntry, 100), Pending: []MigrationSummary{{Name: "V000104__session_device_metadata.sql", Checksum: "ok"}}}
		if err := policy.checkPending(plan); err == nil {
			t.Errorf("unsafe target %q should fail policy validation", target)
		}
	}
}

func TestV104DeclaresOnlyHistoricalSessionOwnerDependency(t *testing.T) {
	bytes, err := os.ReadFile(filepath.Join("..", "..", "db", "release-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy releasePolicy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		t.Fatal(err)
	}
	rule, ok := policy.Automatic["V000104__session_device_metadata.sql"]
	if !ok || rule.Checksum != "f23d99dea23ddeebb5491e1f569f49b2da0812f5880fef5306fd688cf69b68c3" {
		t.Fatalf("V104 release rule/checksum changed unexpectedly: %#v", rule)
	}
	if len(rule.RequiresSchemaOwner) != 1 || rule.RequiresSchemaOwner[0] != "public.sesion" {
		t.Fatalf("V104 owner dependency must be limited to public.sesion: %#v", rule.RequiresSchemaOwner)
	}
}
