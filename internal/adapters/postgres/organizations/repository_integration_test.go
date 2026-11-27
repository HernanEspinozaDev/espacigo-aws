package organizationspg

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupOrganizationsTestDB(t *testing.T) (context.Context, *pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is required for disposable PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	var createdRoles []string
	for _, role := range []struct {
		name  string
		login bool
	}{
		{"espacigo_runtime", false},
		{"espacigo_schema_owner", false},
		{"espacigo_migrator_login", true},
		{"espacigo_privacy_cleanup_worker", false},
		{"espacigo_gadm_bootstrap", false},
		{"espacigo_document_verification_fn", false},
	} {
		var exists bool
		if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role.name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			loginClause := "NOLOGIN"
			if role.login {
				loginClause = "LOGIN"
			}
			if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role.name}.Sanitize()+" "+loginClause+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
				t.Fatal(err)
			}
			createdRoles = append(createdRoles, role.name)
		}
	}

	dbName := fmt.Sprintf("org_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanCancel()
		_, _ = admin.Exec(cleanCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)")
		for _, role := range createdRoles {
			_, _ = admin.Exec(cleanCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		}
	})

	parsedURL, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	parsedURL.Path = "/" + dbName
	dbURL := parsedURL.String()

	setupConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer setupConn.Close(context.Background())

	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		if _, err := setupConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
			t.Fatalf("create extension %s: %v", ext, err)
		}
	}
	if _, err := setupConn.Exec(ctx, `GRANT CONNECT ON DATABASE `+pgx.Identifier{dbName}.Sanitize()+` TO espacigo_migrator_login;
		GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner;
		GRANT espacigo_schema_owner TO espacigo_migrator_login WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;`); err != nil {
		t.Fatal(err)
	}

	migrations, err := migrator.DiscoverMigrations("../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	migConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer migConn.Close(context.Background())

	if _, err := migConn.Exec(ctx, `SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner;`); err != nil {
		t.Fatal(err)
	}
	if _, err := migConn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version bigint PRIMARY KEY CHECK (version > 0),
		name text NOT NULL UNIQUE,
		checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
		runner_checksum text NOT NULL CHECK (runner_checksum ~ '^[0-9a-f]{64}$'),
		applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
	)`); err != nil {
		t.Fatal(err)
	}
	runnerDigest := strings.Repeat("0", 64)
	for _, m := range migrations {
		tx, err := migConn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Conn().PgConn().Exec(ctx, string(m.SQL)).ReadAll(); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("execute migration %s: %v", m.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations(version,name,checksum,runner_checksum) VALUES($1,$2,$3,$4)`, m.Version, m.Name, m.Checksum, runnerDigest); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("record migration %s: %v", m.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %s: %v", m.Name, err)
		}
	}

	if err := dbbootstrap.GrantRuntimePermissions(ctx, setupConn); err != nil {
		t.Fatal(err)
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	runtimeCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`)
		return err
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimePool.Close)

	return ctx, adminPool, runtimePool
}

func TestUserCanCreateMultipleOrganizationsAndUpdatePublicSummaryPostgres(t *testing.T) {
	ctx, adminPool, runtimePool := setupOrganizationsTestDB(t)
	repo := New(runtimePool)

	userID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	revID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,'multi-org@example.test','multi-org@example.test','x','activo')`, []any{userID}},
		{`INSERT INTO public.account_identity_profile_revision(id,account_id,key_version,encrypted_data,masked_rut,masked_phone,saved_at) VALUES($1,$2,'v1',decode(repeat('ab',40),'hex'),'•••78-5','•••1234',now())`, []any{revID, userID}},
		{`INSERT INTO public.account_identity_profile(account_id,username,current_revision) VALUES($1,'multi_org_host',$2)`, []any{userID, revID}},
		{`INSERT INTO public.account_identity_verification(id,account_id,profile_revision,state,started_at,updated_at,verified_at,verified_until) VALUES(gen_random_uuid(),$1,$2,'verified',now(),now(),now(),now()+interval '1 day')`, []any{userID, revID}},
	} {
		if _, err := adminPool.Exec(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}

	fp1 := "v1:" + strings.Repeat("1", 64)
	fp2 := "v1:" + strings.Repeat("2", 64)
	org1, err := repo.Create(ctx, userID, "Arriendos Sur SpA", "Arriendos Sur", fp1, []string{fp1})
	if err != nil {
		t.Fatalf("create org1: %v", err)
	}
	org2, err := repo.Create(ctx, userID, "Bodegas Norte SpA", "Bodegas Norte", fp2, []string{fp2})
	if err != nil {
		t.Fatalf("create org2: %v", err)
	}

	mine, err := repo.ListMine(ctx, userID)
	if err != nil || len(mine) != 2 {
		t.Fatalf("ListMine=%+v err=%v, want 2 organizations", mine, err)
	}

	updated, err := repo.Update(ctx, userID, org1.ID, "Arriendos Sur SpA", "Arriendos Sur Pro", "Espacios corporativos en Santiago")
	if err != nil {
		t.Fatalf("Update public_summary: %v", err)
	}
	if updated.TradeName != "Arriendos Sur Pro" || updated.PublicSummary != "Espacios corporativos en Santiago" {
		t.Fatalf("unexpected updated org: %+v", updated)
	}

	fetched, err := repo.Get(ctx, userID, org1.ID)
	if err != nil || fetched.PublicSummary != "Espacios corporativos en Santiago" {
		t.Fatalf("Get after update=%+v err=%v", fetched, err)
	}

	cleared, err := repo.Update(ctx, userID, org1.ID, "Arriendos Sur SpA", "Arriendos Sur Pro", "")
	if err != nil || cleared.PublicSummary != "" {
		t.Fatalf("clear public_summary=%+v err=%v", cleared, err)
	}
	if org2.ID == org1.ID {
		t.Fatal("expected distinct organization IDs")
	}
}
