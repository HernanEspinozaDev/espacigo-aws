package main

import (
	"bytes"
	"errors"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"strings"
	"testing"
)

type failingResultWriter struct{}

func (failingResultWriter) Write([]byte) (int, error) { return 0, errors.New("closed stdout") }

func TestCLIOutputFailurePreservesCommittedReconciliationError(t *testing.T) {
	var stderr bytes.Buffer
	report := migrator.E1MigrationReport{Committed: true, CommitOutcome: "committed"}
	if code := writeResult(failingResultWriter{}, &stderr, report, errors.New("committed; postcheck failed; do not replay")); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(stderr.String(), "do not replay") || !strings.Contains(stderr.String(), "cannot encode E1 result") {
		t.Fatalf("lost reconciliation instruction: %s", stderr.String())
	}
}
