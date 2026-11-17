package migrator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRunnerSingleOwnerEffectiveRolePostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires networkless disposable harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || u.User == nil || u.User.Username() != "postgres" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires disposable socket")
	}
	marker, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0600 {
		t.Fatal("requires private disposable marker")
	}
	for _, scenario := range []string{"success", "missing-membership", "wrong-owner", "role-not-restored", "ddl-rollback", "ledger-rollback", "before-authority-rollback", "after-authority-rollback", "committed-postcheck-failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			control := connectTestDB(t, ctx, os.Getenv("TEST_DATABASE_URL"))
			defer control.Close(context.Background())
			suffix := fmt.Sprint(time.Now().UnixNano())
			owner := "e1_owner_" + suffix
			login := "e1_login_" + suffix
			if _, err := control.Exec(ctx, "CREATE ROLE "+owner+" NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; CREATE ROLE "+login+" LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := control.Exec(context.Background(), "DROP ROLE "+login+"; DROP ROLE "+owner); err != nil {
					t.Errorf("own disposable roles cleanup: %v", err)
				}
			}()
			if scenario != "missing-membership" {
				if _, err := control.Exec(ctx, "GRANT "+owner+" TO "+login+" WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"); err != nil {
					t.Fatal(err)
				}
			}
			withTestDatabase(t, func(ctx context.Context, dsn string) {
				admin := connectTestDB(t, ctx, dsn)
				defer admin.Close(context.Background())
				if _, err := admin.Exec(ctx, "GRANT CREATE ON SCHEMA public TO "+owner); err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				writeMigration(t, dir, "V000001__base.sql", "CREATE TABLE public.ocupacion(id bigint PRIMARY KEY);")
				if _, err := RunWithRole(ctx, dsn, dir, owner); err != nil {
					t.Fatal(err)
				}
				body := "CREATE TABLE public.e1_effective_probe(id bigint); ALTER TABLE public.ocupacion ADD COLUMN probe boolean;"
				if scenario == "ddl-rollback" {
					body += " PERFORM 1/0;"
				}
				if scenario == "role-not-restored" {
					body += " RESET ROLE;"
				}
				if scenario == "ledger-rollback" {
					if _, err := admin.Exec(ctx, `CREATE FUNCTION public.e1_fail_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture ledger rejection'; END $$; CREATE TRIGGER e1_fail_ledger BEFORE INSERT ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.e1_fail_ledger()`); err != nil {
						t.Fatal(err)
					}
				}
				writeMigration(t, dir, "V000002__effective.sql", "DO $migration$\nBEGIN\n"+body+"\nEND\n$migration$;\n")
				inventory, err := DiscoverMigrations(dir)
				if err != nil {
					t.Fatal(err)
				}
				selected, _ := url.Parse(dsn)
				selected.User = url.User(login)
				writer := connectTestDB(t, ctx, selected.String())
				defer writer.Close(context.Background())
				target := SingleOwnerMigrationTarget{Host: writer.Config().Host, Port: writer.Config().Port, Database: writer.Config().Database, SessionUser: login, OccupancyOwner: owner, LedgerOwner: owner, EffectiveRole: owner}
				if err := admin.QueryRow(ctx, `SELECT oid,(SELECT system_identifier::text FROM pg_control_system()),current_setting('server_version_num')::int FROM pg_database WHERE datname=current_database()`).Scan(&target.DatabaseOID, &target.SystemIdentifier, &target.ServerVersionNum); err != nil {
					t.Fatal(err)
				}
				if scenario == "wrong-owner" {
					target.OccupancyOwner = login
				}
				contract := SingleOwnerMigrationContract{InventorySHA256: inventoryFingerprint(inventory), Version: 2, Name: inventory[1].Name, Checksum: inventory[1].Checksum, Target: target}
				postCalls := 0
				gate := func(ctx context.Context, tx pgx.Tx, after bool) error {
					var current, iso string
					if err := tx.QueryRow(ctx, "SELECT current_user,current_setting('transaction_isolation')").Scan(&current, &iso); err != nil {
						return err
					}
					if current != owner || (!after && iso != "serializable") {
						return errors.New("incorrect gate role or writer isolation")
					}
					if !after && scenario == "before-authority-rollback" {
						return errors.New("fixture before gate rejection")
					}
					if after {
						postCalls++
						if scenario == "after-authority-rollback" || (scenario == "committed-postcheck-failure" && postCalls == 2) {
							return errors.New("fixture after gate rejection")
						}
					}
					return nil
				}
				result, err := RunSingleOwnerMigrationWithAuthority(ctx, writer, dir, contract, gate)
				wantCommit := scenario == "success" || scenario == "committed-postcheck-failure"
				if (err == nil) != (scenario == "success") || result.Committed != wantCommit || result.Verified != (scenario == "success") {
					t.Fatalf("result=%+v error=%v", result, err)
				}
				var count int
				var table, column bool
				var legacyOwner, current string
				var locks bool
				if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.schema_migrations),to_regclass('public.e1_effective_probe') IS NOT NULL,EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='public.ocupacion'::regclass AND attname='probe'),(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.ocupacion'::regclass)`).Scan(&count, &table, &column, &legacyOwner); err != nil {
					t.Fatal(err)
				}
				expectedCount := 1
				if wantCommit {
					expectedCount = 2
				}
				if count != expectedCount || table != wantCommit || column != wantCommit || legacyOwner != owner {
					t.Fatalf("independent state count=%d table=%t column=%t owner=%s", count, table, column, legacyOwner)
				}
				if err := writer.QueryRow(ctx, `SELECT current_user,EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=pg_backend_pid())`).Scan(&current, &locks); err != nil || current != login || locks {
					t.Fatalf("role/lock not restored: %s %t %v", current, locks, err)
				}
				if wantCommit {
					if _, err := RunSingleOwnerMigrationWithAuthority(ctx, writer, dir, contract, gate); err == nil {
						t.Fatal("blind replay accepted")
					}
				}
			})
		})
	}
}
