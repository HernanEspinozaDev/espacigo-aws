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
		{name: "initial ledger incomplete", history: true, applied: 82, wantErr: true},
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

func TestBaselineReleasePolicyMatchesDiscoveredBaselineAndAllowsV2Expand(t *testing.T) {
	bytes, err := os.ReadFile(filepath.Join("..", "..", "db", "release-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy releasePolicy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.Format != "espacigo.initial-migration-policy.v1" || policy.Baseline != 1 {
		t.Fatalf("unexpected release policy header: %+v", policy)
	}
	if len(policy.Automatic) != 0 {
		t.Fatalf("expected empty automatic_migrations at baseline 1, got %d", len(policy.Automatic))
	}
	migrations, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "V000001__baseline.sql" {
		t.Fatalf("unexpected baseline migration discovery: %+v", migrations)
	}
	rawReview, ok := policy.ReviewRequired["V000001__baseline.sql"]
	if !ok {
		t.Fatal("V000001__baseline.sql missing from review_required_migrations")
	}
	var reviewRule struct {
		Checksum  string `json:"checksum"`
		Strategy  string `json:"strategy"`
		Automatic bool   `json:"automatic"`
	}
	if err := json.Unmarshal(rawReview, &reviewRule); err != nil {
		t.Fatal(err)
	}
	if reviewRule.Checksum != migrations[0].Checksum || reviewRule.Strategy != "expand" || reviewRule.Automatic {
		t.Fatalf("unexpected V000001 review rule: %+v (file checksum %s)", reviewRule, migrations[0].Checksum)
	}

	// Verify V000002+ expand migration is allowed once V000001 is applied.
	policy.Automatic["V000002__expand_example.sql"] = struct {
		Checksum            string   `json:"checksum"`
		Strategy            string   `json:"strategy"`
		RequiresSchemaOwner []string `json:"requires_schema_owner,omitempty"`
	}{Checksum: "abc123", Strategy: "expand"}
	plan := PlanResult{
		HistoryExists: true,
		Applied:       []HistoryEntry{{Version: 1, Name: migrations[0].Name, Checksum: migrations[0].Checksum}},
		Pending:       []MigrationSummary{{Version: 2, Name: "V000002__expand_example.sql", Checksum: "abc123"}},
	}
	if err := policy.checkPending(plan); err != nil {
		t.Fatalf("expected V2 expand migration to pass after baseline 1: %v", err)
	}
}
