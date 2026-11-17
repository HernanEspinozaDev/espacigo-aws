package migrator

import (
	"context"
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
