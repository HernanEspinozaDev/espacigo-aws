package migrator

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Only the existing networkless disposable harness may enable this test.
func TestRunnerAuthorityDefaultACLReaderPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the networkless disposable PostgreSQL harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() != "postgres" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires disposable postgres Unix-socket target")
	}
	if _, password := u.User.Password(); password {
		t.Fatal("disposable harness must not supply a password")
	}
	marker, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		_, err := conn.Exec(ctx, `CREATE SCHEMA defaults_probe;
ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO PUBLIC;
ALTER DEFAULT PRIVILEGES GRANT UPDATE ON TABLES TO pg_read_all_stats WITH GRANT OPTION;
ALTER DEFAULT PRIVILEGES IN SCHEMA defaults_probe GRANT UPDATE ON TABLES TO pg_read_all_stats;
ALTER DEFAULT PRIVILEGES GRANT USAGE ON SEQUENCES TO pg_read_all_stats;
ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO pg_read_all_stats;
ALTER DEFAULT PRIVILEGES GRANT USAGE ON TYPES TO pg_read_all_stats;
ALTER DEFAULT PRIVILEGES GRANT USAGE ON SCHEMAS TO pg_read_all_stats;`)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("captures-explicit-all-object-classes-and-global-scope", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureScopedAuthorityDefaultACLs(ctx, tx, []string{"postgres"})
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range got.States {
				if state.Owner != "postgres" {
					t.Fatalf("unexpected owner: %+v", state)
				}
			}
			if len(got.States) != 6 {
				t.Fatalf("expected five global classes plus schema-scoped relation: %+v", got.States)
			}
			found, grantOptionFound := false, false
			for _, a := range got.ACLs {
				if a.Owner == "postgres" && a.Grantor == "postgres" && a.Schema == "" && a.Class == "relation" && a.Grantee == "PUBLIC" && a.Privilege == "SELECT" && !a.GrantOption {
					found = true
				}
				if a.Owner == "postgres" && a.Grantor == "postgres" && a.Schema == "" && a.Class == "relation" && a.Grantee == "pg_read_all_stats" && a.Privilege == "UPDATE" && a.GrantOption {
					grantOptionFound = true
				}
			}
			if !found || !grantOptionFound {
				t.Fatalf("global PUBLIC or role grant option missing: %+v", got)
			}
		})
		t.Run("absent-owner-defaults-explicit-empty-observation", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureScopedAuthorityDefaultACLs(ctx, tx, []string{"pg_read_all_data"})
			if err != nil || got.ACLs == nil || got.States == nil || len(got.ACLs) != 0 || len(got.States) != 0 {
				t.Fatalf("invented defaults: %+v %v", got, err)
			}
		})
		t.Run("explicit-empty-function-default", func(t *testing.T) {
			if _, err := conn.Exec(ctx, `ALTER DEFAULT PRIVILEGES REVOKE ALL ON FUNCTIONS FROM postgres, PUBLIC, pg_read_all_stats;`); err != nil {
				t.Fatal(err)
			}
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureScopedAuthorityDefaultACLs(ctx, tx, []string{"postgres"})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, s := range got.States {
				if s.Class == "function" && s.Schema == "" && s.State == "empty" {
					found = true
				}
			}
			if !found {
				t.Fatalf("empty raw state lost: %+v", got)
			}
			for _, a := range got.ACLs {
				if a.Class == "function" {
					t.Fatalf("empty ACL invented tuple: %+v", a)
				}
			}
		})
		t.Run("rejects-missing-owner-with-zero-observation", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureScopedAuthorityDefaultACLs(ctx, tx, []string{"missing_defaults_probe"})
			if err == nil || got.ACLs != nil || got.States != nil {
				t.Fatalf("missing owner accepted: %+v %v", got, err)
			}
		})
	})
}
