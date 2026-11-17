package migrator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"testing"
	"time"
)

func TestRunnerSingleOwnerMigrationPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the networkless disposable PostgreSQL harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || u.User == nil || u.User.Username() != "postgres" {
		t.Fatal("requires disposable postgres socket target")
	}
	if _, ok := u.User.Password(); ok || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires passwordless disposable Unix socket")
	}
	marker, err := os.Stat(filepath.Join(u.Query().Get("host"), ".espacigo-migration-ci"))
	if err != nil || marker.Mode().Perm() != 0o600 {
		t.Fatal("missing isolated-target marker")
	}
	for _, scenario := range []string{"success", "wrong-cluster", "wrong-database-oid", "wrong-engine", "wrong-owner", "wrong-prefix", "ddl-rollback", "ledger-rollback", "multi-statement", "transaction-escape", "role-not-restored"} {
		t.Run(scenario, func(t *testing.T) {
			withTestDatabase(t, func(ctx context.Context, databaseURL string) {
				conn := connectTestDB(t, ctx, databaseURL)
				defer conn.Close(context.Background())
				role := fmt.Sprintf("single_owner_%d", time.Now().UnixNano())
				if _, err := conn.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE; GRANT CREATE ON SCHEMA public TO "+role); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					admin := connectTestDB(t, cleanupCtx, os.Getenv("TEST_DATABASE_URL"))
					defer admin.Close(context.Background())
					if _, err := admin.Exec(cleanupCtx, "DROP ROLE "+role); err != nil {
						t.Errorf("remove own synthetic role: %v", err)
					}
				})
				dir := t.TempDir()
				writeMigration(t, dir, "V000001__owner_base.sql", "CREATE TABLE public.ocupacion(id bigint PRIMARY KEY);\n")
				if _, err := Run(ctx, databaseURL, dir); err != nil {
					t.Fatal(err)
				}
				body := "EXECUTE 'SET LOCAL ROLE " + role + "'; CREATE TABLE public.single_owned(id bigint); RESET ROLE; ALTER TABLE public.ocupacion ADD COLUMN early_probe boolean;"
				switch scenario {
				case "ddl-rollback":
					body += " PERFORM 1/0;"
				case "ledger-rollback":
					if _, err := conn.Exec(ctx, `CREATE FUNCTION public.fail_new_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'deliberate ledger failure'; END $$; CREATE TRIGGER fail_new_ledger BEFORE INSERT ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.fail_new_ledger()`); err != nil {
						t.Fatal(err)
					}
				case "transaction-escape":
					body += " COMMIT;"
				case "role-not-restored":
					body += " EXECUTE 'SET LOCAL ROLE " + role + "';"
				}
				sql := "DO $migration$\nBEGIN\n" + body + "\nEND\n$migration$;"
				if scenario == "multi-statement" {
					sql += "\nDO $migration$\nBEGIN PERFORM 1; END\n$migration$;"
				}
				writeMigration(t, dir, "V000002__owner_delta.sql", sql)
				migrations, err := DiscoverMigrations(dir)
				if err != nil {
					t.Fatal(err)
				}
				target := SingleOwnerMigrationTarget{Host: conn.Config().Host, Port: conn.Config().Port}
				if err := conn.QueryRow(ctx, `SELECT current_database(),oid,(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),current_setting('server_version_num')::int,session_user::text FROM pg_catalog.pg_database WHERE datname=current_database()`).Scan(&target.Database, &target.DatabaseOID, &target.SystemIdentifier, &target.ServerVersionNum, &target.SessionUser); err != nil {
					t.Fatal(err)
				}
				target.OccupancyOwner, target.LedgerOwner = "postgres", "postgres"
				contract := SingleOwnerMigrationContract{
					InventorySHA256: inventoryFingerprint(migrations),
					Version:         2, Name: migrations[1].Name, Checksum: migrations[1].Checksum, Target: target,
				}
				switch scenario {
				case "wrong-cluster":
					contract.Target.SystemIdentifier = "1"
				case "wrong-database-oid":
					contract.Target.DatabaseOID++
				case "wrong-engine":
					contract.Target.ServerVersionNum++
				case "wrong-owner":
					contract.Target.OccupancyOwner = role
				case "wrong-prefix":
					if _, err := conn.Exec(ctx, "UPDATE public.schema_migrations SET checksum=repeat('0',64)"); err != nil {
						t.Fatal(err)
					}
				}
				result, err := RunSingleOwnerMigration(ctx, conn, dir, contract)
				wantSuccess := scenario == "success"
				if (err == nil) != wantSuccess || result.Committed != wantSuccess || result.Verified != wantSuccess {
					t.Fatalf("scenario %s committed=%t verified=%t err=%v", scenario, result.Committed, result.Verified, err)
				}
				// Independent session readback proves commit/rollback and preserves
				// both legacy owners; it does not reuse the writer's transaction.
				probe := connectTestDB(t, ctx, databaseURL)
				defer probe.Close(context.Background())
				var count int
				var column, table bool
				var occupancyOwner, ledgerOwner string
				if err := probe.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.schema_migrations), EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='ocupacion' AND column_name='early_probe'),to_regclass('public.single_owned') IS NOT NULL`).Scan(&count, &column, &table); err != nil {
					t.Fatal(err)
				}
				if err := probe.QueryRow(ctx, `SELECT (SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.ocupacion'::regclass),(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.schema_migrations'::regclass)`).Scan(&occupancyOwner, &ledgerOwner); err != nil {
					t.Fatal(err)
				}
				wantCount := 1
				if wantSuccess {
					wantCount = 2
					var newOwner string
					if err := probe.QueryRow(ctx, "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.single_owned'::regclass").Scan(&newOwner); err != nil || newOwner != role {
						t.Fatalf("new object owner=%s err=%v", newOwner, err)
					}
					if _, err := RunSingleOwnerMigration(ctx, conn, dir, contract); err == nil {
						t.Fatal("blind replay accepted")
					}
				}
				if count != wantCount || column != wantSuccess || table != wantSuccess || occupancyOwner != "postgres" || ledgerOwner != "postgres" {
					t.Fatalf("state ledger=%d column=%t table=%t owners=%s/%s", count, column, table, occupancyOwner, ledgerOwner)
				}
				var locks bool
				if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=pg_backend_pid())").Scan(&locks); err != nil || locks {
					t.Fatalf("advisory lock leaked: %t %v", locks, err)
				}
			})
		})
	}
}
