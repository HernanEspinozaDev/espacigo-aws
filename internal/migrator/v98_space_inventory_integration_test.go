package migrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// assertV98SpaceInventoryMigration applies the current expand migration only
// inside the marked, disposable PostgreSQL fixture, using a bounded DDL window.
func assertV98SpaceInventoryMigration(t *testing.T, ctx context.Context, admin *pgx.Conn, migrationURL string, migrations []Migration) {
	t.Helper()
	if len(migrations) != 98 || migrations[97].Version != 98 || migrations[97].Name != "V000098__space_inventory_and_calendar_rules.sql" {
		t.Fatalf("V98 expansion requires the exact contiguous V1..V98 inventory; got %d migrations", len(migrations))
	}
	if admin == nil {
		t.Fatal("V98 expansion requires the disposable fixture administrator")
	}
	migrationDir := t.TempDir()
	for _, migration := range migrations {
		if err := os.WriteFile(filepath.Join(migrationDir, migration.Name), migration.SQL, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
GRANT REFERENCES (id) ON TABLE public.espacio TO espacigo_schema_owner`); err != nil {
		t.Fatalf("prepare exact disposable V98 DDL authority: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
REVOKE REFERENCES (id) ON TABLE public.espacio FROM espacigo_schema_owner`); err != nil {
			t.Errorf("revoke disposable V98 DDL authority: %v", err)
			return
		}
		var create, references bool
		if err := admin.QueryRow(context.Background(), `SELECT
pg_catalog.has_schema_privilege('espacigo_schema_owner','public','CREATE'),
pg_catalog.has_column_privilege('espacigo_schema_owner','public.espacio','id','REFERENCES')`).Scan(&create, &references); err != nil || create || references {
			t.Errorf("V98 temporary authority remains open: create=%t references=%t err=%v", create, references, err)
		}
	})
	if result, err := RunWithRole(ctx, migrationURL, migrationDir, "espacigo_schema_owner"); err != nil || len(result.Applied) != 1 || result.Applied[0] != 98 {
		t.Fatalf("apply exact V98 expansion: applied=%v err=%v", result.Applied, err)
	}
	if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
REVOKE REFERENCES (id) ON TABLE public.espacio FROM espacigo_schema_owner`); err != nil {
		t.Fatalf("close exact disposable V98 DDL authority: %v", err)
	}
	var create, references, tablesOwned, runtimeACL, publicDenied bool
	const verify = `SELECT
pg_catalog.has_schema_privilege('espacigo_schema_owner','public','CREATE'),
pg_catalog.has_column_privilege('espacigo_schema_owner','public.espacio','id','REFERENCES'),
(SELECT count(*)=2 AND bool_and(pg_catalog.pg_get_userbyid(c.relowner)='espacigo_schema_owner')
 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname IN ('espacio_regla_disponibilidad','espacio_calendario_override')),
pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_regla_disponibilidad','SELECT')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_regla_disponibilidad','INSERT')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_regla_disponibilidad','UPDATE')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_regla_disponibilidad','DELETE')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_calendario_override','SELECT')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_calendario_override','INSERT')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_calendario_override','UPDATE')
AND pg_catalog.has_table_privilege('espacigo_runtime','public.espacio_calendario_override','DELETE'),
NOT EXISTS (SELECT 1 FROM pg_catalog.pg_class c CROSS JOIN LATERAL pg_catalog.aclexplode(COALESCE(c.relacl,pg_catalog.acldefault('r'::"char",c.relowner))) acl WHERE c.oid IN ('public.espacio_regla_disponibilidad'::regclass,'public.espacio_calendario_override'::regclass) AND acl.grantee=0 AND acl.privilege_type='SELECT')`
	if err := admin.QueryRow(ctx, verify).Scan(&create, &references, &tablesOwned, &runtimeACL, &publicDenied); err != nil {
		t.Fatalf("read back V98 schema and ACL contract: %v", err)
	}
	if create || references || !tablesOwned || !runtimeACL || !publicDenied {
		t.Fatalf("V98 schema/ACL contract: create=%t references=%t tablesOwned=%t runtimeACL=%t publicDenied=%t", create, references, tablesOwned, runtimeACL, publicDenied)
	}
	var constraints bool
	const inspectConstraints = `SELECT
(SELECT count(*)=1 AND bool_and(contype='p') FROM pg_catalog.pg_constraint WHERE conrelid='public.espacio_regla_disponibilidad'::regclass AND contype='p')
AND (SELECT count(*)=2 AND bool_and(contype IN ('p','u')) FROM pg_catalog.pg_constraint WHERE conrelid='public.espacio_calendario_override'::regclass AND contype IN ('p','u'))
AND EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.espacio_regla_disponibilidad'::regclass AND contype='f' AND confrelid='public.espacio'::regclass)
AND EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.espacio_calendario_override'::regclass AND contype='f' AND confrelid='public.espacio'::regclass)
AND EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.espacio_calendario_override'::regclass AND conname='espacio_calendario_override_uk' AND contype='u')`
	if err := admin.QueryRow(ctx, inspectConstraints).Scan(&constraints); err != nil || !constraints {
		t.Fatalf("V98 constraints do not match the reviewed expand schema: valid=%t err=%v", constraints, err)
	}
}
