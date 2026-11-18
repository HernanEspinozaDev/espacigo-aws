package migrator

import (
	"encoding/json"
	"testing"
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
