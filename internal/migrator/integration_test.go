package migrator

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRunnerAppliesFromEmptyAndRepeatIsNoop(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__probe.sql", "CREATE TABLE probe (id integer PRIMARY KEY); INSERT INTO probe VALUES (1);\n")

		first, err := Run(ctx, databaseURL, dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Applied) != 1 || first.Applied[0] != 1 {
			t.Fatalf("first run applied versions %v", first.Applied)
		}
		second, err := Run(ctx, databaseURL, dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(second.Applied) != 0 {
			t.Fatalf("second run reapplied versions %v", second.Applied)
		}

		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var rows, historyRows int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM probe").Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&historyRows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 || historyRows != 1 {
			t.Fatalf("repeat changed state: probe rows=%d history rows=%d", rows, historyRows)
		}
	})
}

func TestRunnerRefusesToInitializeLedgerBesideExistingApplicationObjects(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		conn := connectTestDB(t, ctx, databaseURL)
		if _, err := conn.Exec(ctx, `CREATE TABLE public.existing_application_probe (id integer PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(ctx); err != nil {
			t.Fatal(err)
		}

		dir := t.TempDir()
		writeMigration(t, dir, "V000001__probe.sql", "CREATE TABLE public.new_probe (id integer PRIMARY KEY);\n")
		if _, err := Run(ctx, databaseURL, dir); err == nil || !strings.Contains(err.Error(), "without public.schema_migrations") {
			t.Fatalf("expected missing-ledger guard, got %v", err)
		}

		check := connectTestDB(t, ctx, databaseURL)
		defer check.Close(context.Background())
		var historyExists bool
		if err := check.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&historyExists); err != nil {
			t.Fatal(err)
		}
		if historyExists {
			t.Fatal("runner initialized migration history beside existing application objects")
		}
	})
}

func TestPlanOnlyDoesNotInitializeLedgerOrExecutePendingMigrations(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := filepath.Join("..", "..", "db", "migrations")
		migrations, err := DiscoverMigrations(dir)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := Plan(ctx, databaseURL, dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if !plan.ReadOnly || plan.SessionUser == "" || plan.SessionUser != plan.CurrentUser || plan.HistoryExists || len(plan.Applied) != 0 {
			t.Fatalf("unexpected read-only plan: %#v", plan)
		}
		if len(plan.Pending) != len(migrations) || plan.DDLAuthority != nil || plan.IdentityDDLAuthority != nil {
			t.Fatalf("unexpected read-only plan: %#v", plan)
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var historyExists, migrationObjectExists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL, to_regclass('public.espacio') IS NOT NULL`).Scan(&historyExists, &migrationObjectExists); err != nil {
			t.Fatal(err)
		}
		if historyExists || migrationObjectExists {
			t.Fatalf("plan-only changed database state: history=%t migration_object=%t", historyExists, migrationObjectExists)
		}
	})
}

func TestRunnerAppliesDDLUnderRestrictedSetRoleMembership(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		group := "mig_group_" + suffix
		login := "mig_login_" + suffix
		adminURL := os.Getenv("TEST_DATABASE_URL")
		admin := connectTestDB(t, ctx, adminURL)
		defer admin.Close(context.Background())
		dbURL, err := url.Parse(databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		databaseName := strings.TrimPrefix(dbURL.Path, "/")
		if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{group}.Sanitize()+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{login}.Sanitize()+" LOGIN PASSWORD 'local-integration-only' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cleanupAdmin, err := pgx.Connect(cleanupCtx, adminURL)
			if err != nil {
				t.Errorf("connect disposable PostgreSQL for role cleanup: %v", err)
				return
			}
			defer cleanupAdmin.Close(context.Background())
			if _, err := cleanupAdmin.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{login}.Sanitize()+", "+pgx.Identifier{group}.Sanitize()); err != nil {
				t.Errorf("drop disposable restricted roles: %v", err)
			}
		})
		if _, err := admin.Exec(ctx, "GRANT CONNECT ON DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" TO "+pgx.Identifier{login}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		databaseAdmin := connectTestDB(t, ctx, databaseURL)
		if _, err := databaseAdmin.Exec(ctx, "GRANT USAGE, CREATE ON SCHEMA public TO "+pgx.Identifier{group}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		if err := databaseAdmin.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, "GRANT "+pgx.Identifier{group}.Sanitize()+" TO "+pgx.Identifier{login}.Sanitize()+" WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"); err != nil {
			t.Fatal(err)
		}
		loginURL := *dbURL
		loginURL.User = url.UserPassword(login, "local-integration-only")
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__owned_probe.sql", "CREATE TABLE set_role_probe (id integer PRIMARY KEY);\n")
		result, err := RunWithRole(ctx, loginURL.String(), dir, group)
		if err != nil || len(result.Applied) != 1 {
			t.Fatalf("restricted migration applied=%v err=%v", result.Applied, err)
		}
		inspector := connectTestDB(t, ctx, databaseURL)
		var owner string
		if err := inspector.QueryRow(ctx, `SELECT tableowner FROM pg_tables WHERE schemaname='public' AND tablename='set_role_probe'`).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		if err := inspector.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if owner != group {
			t.Fatalf("migration object owner=%q, want SET ROLE group %q", owner, group)
		}
	})
}

func TestMigratorSerializesIndependentProcesses(t *testing.T) {
	if os.Getenv("MIGRATOR_PROCESS_HELPER") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := Run(ctx, os.Getenv("TEST_DATABASE_URL"), os.Getenv("MIGRATOR_PROCESS_MIGRATIONS")); err != nil {
			t.Fatalf("subprocess migrator failed: %v", err)
		}
		return
	}
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		migrationDir := t.TempDir()
		writeMigration(t, migrationDir, "V000001__process_lock.sql", `CREATE TABLE public.process_migration_once (id integer PRIMARY KEY); INSERT INTO public.process_migration_once VALUES (1);`)
		env := make([]string, 0, len(os.Environ())+3)
		for _, item := range os.Environ() {
			if strings.HasPrefix(item, "MIGRATOR_PROCESS_HELPER=") || strings.HasPrefix(item, "TEST_DATABASE_URL=") || strings.HasPrefix(item, "MIGRATOR_PROCESS_MIGRATIONS=") {
				continue
			}
			env = append(env, item)
		}
		commands := make([]*exec.Cmd, 2)
		var outputs [2]bytes.Buffer
		for index := range commands {
			commands[index] = exec.Command(os.Args[0], "-test.run=^TestMigratorSerializesIndependentProcesses$")
			commands[index].Env = append(env, "MIGRATOR_PROCESS_HELPER=1", "TEST_DATABASE_URL="+databaseURL, "MIGRATOR_PROCESS_MIGRATIONS="+migrationDir)
			commands[index].Stdout = &outputs[index]
			commands[index].Stderr = &outputs[index]
			if err := commands[index].Start(); err != nil {
				t.Fatalf("start migrator subprocess %d: %v", index, err)
			}
		}
		for index, command := range commands {
			if err := command.Wait(); err != nil {
				t.Fatalf("migrator subprocess %d: %v: %s", index, err, outputs[index].String())
			}
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(ctx)
		var historyRows, ddlRows int
		if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.schema_migrations),(SELECT count(*) FROM public.process_migration_once)`).Scan(&historyRows, &ddlRows); err != nil {
			t.Fatal(err)
		}
		if historyRows != 1 || ddlRows != 1 {
			t.Fatalf("concurrent process migration duplicated work: history=%d ddl=%d", historyRows, ddlRows)
		}
	})
}

func TestRunnerStoresExactMigrationAndRunnerChecksums(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		const migrationSQL = "CREATE TABLE checksum_probe (id integer PRIMARY KEY);\n"
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__checksum_probe.sql", migrationSQL)
		if _, err := Run(ctx, databaseURL, dir); err != nil {
			t.Fatal(err)
		}

		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var gotMigrationChecksum, gotRunnerChecksum string
		var appliedAt time.Time
		if err := conn.QueryRow(ctx, `SELECT checksum, runner_checksum, applied_at
			FROM public.schema_migrations WHERE version = 1`).Scan(
			&gotMigrationChecksum, &gotRunnerChecksum, &appliedAt); err != nil {
			t.Fatal(err)
		}
		if want := sha256Hex([]byte(migrationSQL)); gotMigrationChecksum != want {
			t.Fatalf("stored migration checksum=%s, want %s", gotMigrationChecksum, want)
		}
		wantRunnerChecksum, err := executableChecksum()
		if err != nil {
			t.Fatal(err)
		}
		if gotRunnerChecksum != wantRunnerChecksum {
			t.Fatalf("stored runner checksum=%s, want %s", gotRunnerChecksum, wantRunnerChecksum)
		}
		if appliedAt.IsZero() {
			t.Fatal("applied_at was not recorded")
		}
	})
}

func TestRunnerRejectsChecksumDriftBeforeDDL(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		path := writeMigration(t, dir, "V000001__probe.sql", "CREATE TABLE probe (id integer PRIMARY KEY);\n")
		if _, err := Run(ctx, databaseURL, dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("CREATE TABLE drift_must_not_run (id integer);\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Run(ctx, databaseURL, dir); err == nil {
			t.Fatal("expected checksum drift to fail")
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var relation *string
		if err := conn.QueryRow(ctx, "SELECT to_regclass('public.drift_must_not_run')::text").Scan(&relation); err != nil {
			t.Fatal(err)
		}
		if relation != nil {
			t.Fatalf("DDL ran despite checksum drift: %v", *relation)
		}
	})
}

func TestRunnerRollsBackMigrationAndHistoryTogether(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__failing_probe.sql", "CREATE TABLE rolled_back_probe (id integer); SELECT 1 / 0;\n")
		if _, err := Run(ctx, databaseURL, dir); err == nil {
			t.Fatal("expected the invalid migration to fail")
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var relation *string
		var historyRows int
		if err := conn.QueryRow(ctx, "SELECT to_regclass('public.rolled_back_probe')::text").Scan(&relation); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&historyRows); err != nil {
			t.Fatal(err)
		}
		if relation != nil || historyRows != 0 {
			t.Fatalf("failed migration was not atomic: relation=%v history=%d", relation, historyRows)
		}
	})
}

func TestRunnerRecoversAfterFailureBetweenMigrationCommits(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__recovery_first.sql", "CREATE TABLE recovery_first_probe (id integer PRIMARY KEY); INSERT INTO recovery_first_probe VALUES (1);\n")
		secondPath := filepath.Join(dir, "V000002__recovery_second.sql")
		writeMigration(t, dir, filepath.Base(secondPath), "CREATE TABLE recovery_second_probe (id integer PRIMARY KEY); INSERT INTO recovery_second_probe VALUES (1); SELECT 1 / 0;\n")

		firstRun, err := Run(ctx, databaseURL, dir)
		if err == nil || !strings.Contains(err.Error(), "execute migration V000002") || len(firstRun.Applied) != 1 || firstRun.Applied[0] != 1 {
			t.Fatalf("expected V000001 to commit before V000002 fails; applied=%v err=%v", firstRun.Applied, err)
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var firstExists, secondExists bool
		var historyCount int
		var latestVersion int64
		if err := conn.QueryRow(ctx, `SELECT to_regclass('public.recovery_first_probe') IS NOT NULL, to_regclass('public.recovery_second_probe') IS NOT NULL`).Scan(&firstExists, &secondExists); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, `SELECT count(*),max(version) FROM public.schema_migrations`).Scan(&historyCount, &latestVersion); err != nil {
			t.Fatal(err)
		}
		if !firstExists || secondExists || historyCount != 1 || latestVersion != 1 {
			t.Fatalf("failure boundary not preserved: first=%t second=%t history=%d latest=%d", firstExists, secondExists, historyCount, latestVersion)
		}
		if err := os.WriteFile(secondPath, []byte("CREATE TABLE recovery_second_probe (id integer PRIMARY KEY); INSERT INTO recovery_second_probe VALUES (2);\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		recovered, err := Run(ctx, databaseURL, dir)
		if err != nil || len(recovered.Applied) != 1 || recovered.Applied[0] != 2 {
			t.Fatalf("recovery applied=%v err=%v", recovered.Applied, err)
		}
		var firstRows, secondRows int
		if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM recovery_first_probe), (SELECT count(*) FROM recovery_second_probe)`).Scan(&firstRows, &secondRows); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&historyCount); err != nil {
			t.Fatal(err)
		}
		if firstRows != 1 || secondRows != 1 || historyCount != 2 {
			t.Fatalf("recovery state mismatch: first_rows=%d second_rows=%d history=%d", firstRows, secondRows, historyCount)
		}
	})
}

func TestRunnerSerializesConcurrentExecutions(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__concurrent_probe.sql", "SELECT pg_sleep(0.3); CREATE TABLE concurrent_probe (id integer PRIMARY KEY);\n")

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := Run(ctx, databaseURL, dir)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var historyRows int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&historyRows); err != nil {
			t.Fatal(err)
		}
		if historyRows != 1 {
			t.Fatalf("concurrent runs applied migration %d times", historyRows)
		}
	})
}

func TestRunnerRejectsVersionGapBeforeDDL(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__first.sql", "CREATE TABLE first_probe (id integer);\n")
		writeMigration(t, dir, "V000003__third.sql", "CREATE TABLE third_probe (id integer);\n")
		if _, err := Run(ctx, databaseURL, dir); err == nil {
			t.Fatal("expected version gap to fail")
		}
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var historyExists bool
		if err := conn.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&historyExists); err != nil {
			t.Fatal(err)
		}
		if historyExists {
			var historyRows int
			if err := conn.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&historyRows); err != nil {
				t.Fatal(err)
			}
			if historyRows != 0 {
				t.Fatalf("invalid sequence changed history: %d rows", historyRows)
			}
		}
	})
}

func TestBaselinePostgresIntegration(t *testing.T) {
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		adminURL := os.Getenv("TEST_DATABASE_URL")
		admin := connectTestDB(t, ctx, adminURL)
		defer admin.Close(context.Background())

		baselineRoles := []string{
			"espacigo_runtime",
			"espacigo_schema_owner",
			"espacigo_privacy_cleanup_worker",
			"espacigo_gadm_bootstrap",
		}
		var createdRoles []string
		for _, role := range baselineRoles {
			var exists bool
			if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" NOLOGIN"); err != nil {
					t.Fatal(err)
				}
				createdRoles = append(createdRoles, role)
			}
		}
		if len(createdRoles) > 0 {
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cleanupAdmin, err := pgx.Connect(cleanupCtx, adminURL)
				if err != nil {
					t.Errorf("connect for baseline role cleanup: %v", err)
					return
				}
				defer cleanupAdmin.Close(context.Background())
				for _, role := range createdRoles {
					if _, err := cleanupAdmin.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize()); err != nil {
						t.Errorf("drop disposable baseline role %s: %v", role, err)
					}
				}
			})
		}

		baselineSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "V000001__baseline.sql"))
		if err != nil {
			t.Fatal(err)
		}
		extConn := connectTestDB(t, ctx, databaseURL)
		for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
			if _, err := extConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
				_ = extConn.Close(context.Background())
				t.Fatalf("create extension %s: %v", ext, err)
			}
		}
		if err := extConn.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		writeMigration(t, dir, "V000001__baseline.sql", string(baselineSQL))

		first, err := Run(ctx, databaseURL, dir)
		if err != nil {
			t.Fatalf("apply baseline: %v", err)
		}
		if len(first.Applied) != 1 || first.Applied[0] != 1 {
			t.Fatalf("baseline applied=%v, want [1]", first.Applied)
		}
		repeat, err := Run(ctx, databaseURL, dir)
		if err != nil {
			t.Fatalf("repeat baseline: %v", err)
		}
		if len(repeat.Applied) != 0 {
			t.Fatalf("repeat baseline applied=%v, want []", repeat.Applied)
		}

		writeMigration(t, dir, "V000002__baseline_expand_probe.sql", "CREATE TABLE public.baseline_expand_probe (id integer PRIMARY KEY);\n")
		expand, err := Run(ctx, databaseURL, dir)
		if err != nil {
			t.Fatalf("apply V000002 expand: %v", err)
		}
		if len(expand.Applied) != 1 || expand.Applied[0] != 2 {
			t.Fatalf("expand applied=%v, want [2]", expand.Applied)
		}

		writeMigration(t, dir, "V000003__baseline_rollback_probe.sql", "CREATE TABLE public.baseline_rollback_probe (id integer PRIMARY KEY); SELECT 1 / 0;\n")
		if _, err := Run(ctx, databaseURL, dir); err == nil {
			t.Fatal("expected V000003 rollback probe to fail")
		}

		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		var expandExists, rollbackExists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass('public.baseline_expand_probe') IS NOT NULL, to_regclass('public.baseline_rollback_probe') IS NOT NULL`).Scan(&expandExists, &rollbackExists); err != nil {
			t.Fatal(err)
		}
		if !expandExists || rollbackExists {
			t.Fatalf("unexpected probe tables after expand+rollback: expand=%t rollback=%t", expandExists, rollbackExists)
		}
		var historyCount, maxVersion int
		if err := conn.QueryRow(ctx, `SELECT count(*), max(version) FROM public.schema_migrations`).Scan(&historyCount, &maxVersion); err != nil {
			t.Fatal(err)
		}
		if historyCount != 2 || maxVersion != 2 {
			t.Fatalf("unexpected ledger after expand+rollback: count=%d max=%d", historyCount, maxVersion)
		}
	})
}

const rentalCombinedFixtureMarker = ".espacigo-migration-ci"

func withTestDatabase(t *testing.T, test func(context.Context, string)) {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; requires disposable PostgreSQL 18")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to disposable PostgreSQL: %v", err)
	}
	defer admin.Close(context.Background())
	databaseName := fmt.Sprintf("migrator_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		t.Fatalf("create disposable test database: %v", err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
			t.Errorf("drop disposable test database: %v", err)
		}
	}()
	databaseURLParsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	databaseURLParsed.Path = "/" + databaseName
	testURL := databaseURLParsed.String()
	testConn := connectTestDB(t, ctx, testURL)
	if err := testConn.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	test(ctx, testURL)
}

func connectTestDB(t *testing.T, ctx context.Context, databaseURL string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeMigration(t *testing.T, dir, name, sql string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
