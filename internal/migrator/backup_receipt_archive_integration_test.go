package migrator

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Opt-in archive parsing is read-only and does not connect to PostgreSQL.
// The receipt and archive remain private; only pass/fail is emitted.
func TestBackupReceiptRealArchive(t *testing.T) {
	path := os.Getenv("TEST_E1_BACKUP_RECEIPT")
	if path == "" {
		t.Skip("requires an explicitly selected private backup receipt")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read selected backup receipt failed")
	}
	var receipt MigrationBackupReceipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		t.Fatal("invalid selected backup receipt")
	}
	if err := ValidateMigrationBackupArchive(context.Background(), receipt, "/usr/lib/postgresql/18/bin/pg_restore"); err != nil {
		t.Fatal(err)
	}
}
