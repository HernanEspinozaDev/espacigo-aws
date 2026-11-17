package migrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Exercise the production contract/authority validator with an unchanged Plan
// collected from the marked PostgreSQL fixture, not hand-built authority data.
func assertPostgresAuthorityPythonWire(t *testing.T, ctx context.Context, migrationURL, database string, migrations []Migration, version int) {
	t.Helper()
	plan, err := Plan(ctx, migrationURL, prefixMigrationDirForCombined(t, migrations, version), "espacigo_schema_owner")
	if err != nil {
		t.Fatalf("collect PostgreSQL authority plan before V%d: %v", version, err)
	}
	if plan.Database != database || !strings.HasPrefix(plan.Database, "migrator_test_") || !plan.ReadOnly || !plan.HistoryExists || len(plan.Applied) != version-1 || len(plan.Pending) != 1 || plan.Pending[0].Version != int64(version) {
		t.Fatalf("unexpected PostgreSQL wire boundary before V%d", version)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if version >= 84 && version <= 88 {
		if _, ok := decoded["ddl_authority"]; !ok {
			t.Fatalf("PostgreSQL authority Go JSON missing ddl_authority before V%d", version)
		}
	} else if version == 95 || version == 96 {
		if _, ok := decoded["identity_ddl_authority"]; !ok {
			t.Fatalf("PostgreSQL authority Go JSON missing identity_ddl_authority before V%d", version)
		}
	}
}
