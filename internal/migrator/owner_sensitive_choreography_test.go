package migrator

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This proves only PostgreSQL transaction/role choreography on the explicitly
// isolated 18.7 harness. It does not authorize or exercise a persistent target.
func TestOwnerSensitiveMigrationChoreographyPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the networkless disposable PostgreSQL 18.7 harness")
	}
	adminURL := os.Getenv("TEST_DATABASE_URL")
	parsed, err := url.Parse(adminURL)
	if err != nil || parsed.User == nil || parsed.User.Username() != "postgres" {
		t.Fatal("owner choreography requires the disposable postgres socket DSN")
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		t.Fatal("owner choreography harness must not use a password")
	}
	socketDir := parsed.Query().Get("host")
	if !filepath.IsAbs(socketDir) {
		t.Fatal("owner choreography harness must use an absolute Unix-socket directory")
	}
	marker, err := os.Stat(filepath.Join(socketDir, ".espacigo-migration-ci"))
	if err != nil || marker.Mode().Perm() != 0o600 {
		t.Fatalf("isolated-target marker missing or unsafe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	probe := connectTestDB(t, ctx, adminURL)
	var serverVersion, sessionUser string
	if err := probe.QueryRow(ctx, `SELECT current_setting('server_version_num'),session_user`).Scan(&serverVersion, &sessionUser); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(serverVersion, "180007") || sessionUser != "postgres" {
		t.Fatalf("isolated executor profile=%s/%s, want PostgreSQL 18.7/postgres", serverVersion, sessionUser)
	}
	if err := probe.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, failAfterOccupancyAlter := range []bool{true, false} {
		name := "commits-role-separated-objects"
		if failAfterOccupancyAlter {
			name = "failure-rolls-back-both-owners-and-ledger"
		}
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanupCancel()
				admin, err := pgx.Connect(cleanupCtx, adminURL)
				if err != nil {
					t.Errorf("connect to isolated cluster for owned role cleanup: %v", err)
					return
				}
				defer admin.Close(context.Background())
				if _, err := admin.Exec(cleanupCtx, `DROP ROLE IF EXISTS espacigo_schema_owner,espacigo_privacy_cleanup_worker`); err != nil {
					t.Errorf("drop isolated fixture roles after database cleanup: %v", err)
				}
			})
			withTestDatabase(t, func(ctx context.Context, databaseURL string) {
				conn := connectTestDB(t, ctx, databaseURL)
				defer conn.Close(context.Background())
				if _, err := conn.Exec(ctx, `
CREATE ROLE espacigo_schema_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE espacigo_privacy_cleanup_worker NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
CREATE TABLE public.schema_migrations (
 version bigint PRIMARY KEY CHECK(version>0), name text NOT NULL UNIQUE,
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),
 runner_checksum text NOT NULL CHECK(runner_checksum ~ '^[0-9a-f]{64}$'),
 applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE public.ocupacion (id bigint PRIMARY KEY);
CREATE TABLE public.space_media (id bigint PRIMARY KEY);
CREATE TABLE public.media_asset (id bigint PRIMARY KEY);
CREATE TABLE public.media_variant (id bigint PRIMARY KEY);
CREATE TABLE public.rol_usuario_evento (id bigint PRIMARY KEY);
CREATE FUNCTION public.claim_private_media_cleanup(integer,timestamptz) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;
CREATE FUNCTION public.complete_private_media_cleanup(uuid,bigint,timestamptz) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;
CREATE FUNCTION public.fail_private_media_cleanup(uuid,bigint,timestamptz) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;
CREATE FUNCTION public.finalize_pending_suppression_cleanup(uuid,timestamptz) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;
`); err != nil {
					t.Fatalf("prepare disposable owner/ledger fixture: %v", err)
				}

				source, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
				if err != nil || len(source) != 98 {
					t.Fatalf("discover verified V1..V98 source: count=%d err=%v", len(source), err)
				}
				for _, migration := range source {
					if _, err := conn.Exec(ctx, `INSERT INTO public.schema_migrations(version,name,checksum,runner_checksum) VALUES($1,$2,$3,$4)`, migration.Version, migration.Name, migration.Checksum, strings.Repeat("0", 64)); err != nil {
						t.Fatalf("seed disposable V%d history: %v", migration.Version, err)
					}
				}
				stage := t.TempDir()
				for _, migration := range source {
					if err := os.WriteFile(filepath.Join(stage, migration.Name), migration.SQL, 0o600); err != nil {
						t.Fatalf("stage V%d source: %v", migration.Version, err)
					}
				}
				migrationSQL := `SET LOCAL search_path TO public;
SET ROLE espacigo_schema_owner;
CREATE TABLE public.early_rental_owner_probe(id bigint PRIMARY KEY);
RESET ROLE;
ALTER TABLE public.ocupacion ADD COLUMN early_rental_owner_probe bigint;
`
				if failAfterOccupancyAlter {
					migrationSQL += "ALTER TABLE public.ocupacion ADD CONSTRAINT early_rental_expected_failure CHECK (missing_column IS NOT NULL);\n"
				}
				if err := os.WriteFile(filepath.Join(stage, "V000099__synthetic_owner_probe.sql"), []byte(migrationSQL), 0o600); err != nil {
					t.Fatal(err)
				}
				config, err := pgx.ParseConfig(databaseURL)
				if err != nil {
					t.Fatal("parse disposable migration target")
				}
				result, runErr := runWithConfigOptions(ctx, config, stage, "", &migrationRange{minApplied: 98, maxVersion: 99})
				if failAfterOccupancyAlter {
					if runErr == nil || !strings.Contains(runErr.Error(), "execute migration V000099") {
						t.Fatalf("expected controlled V99 failure, result=%+v err=%v", result, runErr)
					}
					var columnExists, tableExists, ledgerExists bool
					if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='ocupacion' AND column_name='early_rental_owner_probe'),to_regclass('public.early_rental_owner_probe') IS NOT NULL,EXISTS(SELECT 1 FROM public.schema_migrations WHERE version=99)`).Scan(&columnExists, &tableExists, &ledgerExists); err != nil {
						t.Fatal(err)
					}
					if columnExists || tableExists || ledgerExists {
						t.Fatalf("failed transaction left partial owner/occupancy/ledger state: column=%v table=%v ledger=%v", columnExists, tableExists, ledgerExists)
					}
					return
				}
				if runErr != nil || len(result.Applied) != 1 || result.Applied[0] != 99 {
					t.Fatalf("owner-sensitive synthetic migration result=%+v err=%v", result, runErr)
				}
				var occupancyOwner, newTableOwner, currentUser string
				var columnExists, ledgerExists bool
				if err := conn.QueryRow(ctx, `
SELECT pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.ocupacion'::regclass)),
 pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.early_rental_owner_probe'::regclass)),
 current_user,
 EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='ocupacion' AND column_name='early_rental_owner_probe'),
 EXISTS(SELECT 1 FROM public.schema_migrations WHERE version=99 AND name='V000099__synthetic_owner_probe.sql')`).Scan(&occupancyOwner, &newTableOwner, &currentUser, &columnExists, &ledgerExists); err != nil {
					t.Fatal(err)
				}
				if occupancyOwner != "postgres" || newTableOwner != "espacigo_schema_owner" || currentUser != "postgres" || !columnExists || !ledgerExists {
					t.Fatalf("postcheck owner/ledger profile occupancy=%q new=%q current=%q column=%v ledger=%v", occupancyOwner, newTableOwner, currentUser, columnExists, ledgerExists)
				}
			})
		})
	}
}
