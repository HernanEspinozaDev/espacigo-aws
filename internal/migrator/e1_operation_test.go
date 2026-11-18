package migrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestE1ReconciliationFailureRemainsUnknown(t *testing.T) {
	report, err := VerifyE1MigrationOperation(context.Background(), "/unavailable", E1MigrationOperation{})
	if err == nil || report.CommitOutcome != "unknown" || report.Committed || report.Verified {
		t.Fatalf("inconclusive readback misclassified: %+v %v", report, err)
	}
}

func TestE1RejectedOperationNeverStartsWriter(t *testing.T) {
	report, err := RunE1MigrationOperation(context.Background(), "/unavailable", "/unavailable", E1MigrationOperation{}, true)
	if err == nil || report.CommitOutcome != "not_started" || report.Committed || report.Verified {
		t.Fatalf("invalid invocation outcome: %+v %v", report, err)
	}
}

func TestE1VerifiesFullCommitBeforeSelectingVersion(t *testing.T) {
	root, _, first := sourceFixture(t, "db/migrations/V000001__first.sql", "V000001__first.sql")
	second := writeSource(t, root, "db/migrations/V000002__candidate.sql", "V000002__candidate.sql")
	sql := []byte("DO $migration$\nBEGIN NULL; END;\n$migration$;\n")
	if err := os.WriteFile(filepath.Join(root, second.Path), sql, 0644); err != nil {
		t.Fatal(err)
	}
	second.Checksum = sha256Hex(sql)
	third := writeSource(t, root, "db/migrations/V000003__later.sql", "V000003__later.sql")
	commit := commitSource(t, root)
	prefix := validSourceManifest(commit, first, second)
	op := E1MigrationOperation{
		Source: validSourceManifest(commit, first, second, third),
		Migration: SingleOwnerMigrationContract{
			InventorySHA256: prefix.InventorySHA256, Version: 2, Name: second.Name, Checksum: second.Checksum,
			Target: SingleOwnerMigrationTarget{
				Host: "127.0.0.1", Port: 5432, Database: "test", DatabaseOID: 1,
				SystemIdentifier: "42", ServerVersionNum: 180006,
				SessionUser: "migrator", EffectiveRole: "owner", OccupancyOwner: "owner", LedgerOwner: "owner",
			},
		},
	}
	got, err := verifyE1OperationSource(context.Background(), root, op)
	if err != nil || len(got) != 2 || got[1].Name != second.Name {
		t.Fatalf("verified prefix is incorrect: %+v %v", got, err)
	}

	op.Source = prefix
	if _, err := verifyE1OperationSource(context.Background(), root, op); err == nil {
		t.Fatal("accepted a source manifest omitting a committed migration")
	}
	op.Source = validSourceManifest(commit, first, second, third)
	op.Migration.InventorySHA256 = op.Source.InventorySHA256
	if _, err := verifyE1OperationSource(context.Background(), root, op); err == nil {
		t.Fatal("accepted the full commit digest in place of the authorized execution prefix")
	}
}
