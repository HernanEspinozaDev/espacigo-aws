package migrator

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The parent runner supplies the isolated networkless PostgreSQL target; this test never
// creates or connects to a database outside that disposable fixture.
func TestRunnerAuthorityEffectiveReaderPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires networkless disposable PostgreSQL harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() != "postgres" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires disposable postgres Unix-socket target")
	}
	if _, ok := u.User.Password(); ok {
		t.Fatal("password forbidden in disposable target URL")
	}
	marker, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	withTestDatabase(t, func(ctx context.Context, dsn string) {
		conn := connectTestDB(t, ctx, dsn)
		defer conn.Close(context.Background())
		if _, err := conn.Exec(ctx, `CREATE ROLE effective_probe NOLOGIN; CREATE SCHEMA effective_probe; CREATE TABLE effective_probe.t(id integer, hidden text); CREATE SEQUENCE effective_probe.s; CREATE FUNCTION effective_probe.f(integer) RETURNS integer LANGUAGE sql AS 'SELECT $1'; GRANT USAGE ON SCHEMA effective_probe TO effective_probe; GRANT SELECT(id) ON effective_probe.t TO effective_probe;`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, cleanupErr := conn.Exec(context.Background(), `DROP SCHEMA IF EXISTS effective_probe CASCADE; DROP ROLE IF EXISTS effective_probe`); cleanupErr != nil {
				t.Errorf("cleanup disposable authority fixture: %v", cleanupErr)
			}
		}()
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		probes := []AuthorityPrivilegeProbe{{Role: "effective_probe", ObjectClass: "schema", ObjectIdentity: "effective_probe", Privilege: "USAGE", Granted: false}, {Role: "effective_probe", ObjectClass: "relation", ObjectIdentity: "effective_probe.t", Column: "id", Privilege: "SELECT"}, {Role: "effective_probe", ObjectClass: "relation", ObjectIdentity: "effective_probe.t", Column: "hidden", Privilege: "SELECT", Granted: true}, {Role: "effective_probe", ObjectClass: "sequence", ObjectIdentity: "effective_probe.s", Privilege: "USAGE"}, {Role: "effective_probe", ObjectClass: "function", ObjectIdentity: "effective_probe.f(integer)", Privilege: "EXECUTE"}}
		got, err := CaptureAuthorityPrivilegeProbes(ctx, tx, probes)
		if err != nil {
			t.Fatal(err)
		}
		want := []bool{true, true, false, false, true}
		for i := range want {
			if got[i].Granted != want[i] {
				t.Fatalf("probe %d got %v want %v", i, got[i].Granted, want[i])
			}
		}
		if probes[0].Granted {
			t.Fatal("input slice was mutated")
		}
	})
}

func TestRunnerAuthorityACLCatalogPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires networkless disposable PostgreSQL harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() != "postgres" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires disposable postgres Unix-socket target")
	}
	if _, ok := u.User.Password(); ok {
		t.Fatal("password forbidden in disposable target URL")
	}
	marker, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	withTestDatabase(t, func(ctx context.Context, dsn string) {
		conn := connectTestDB(t, ctx, dsn)
		defer conn.Close(context.Background())
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		got, err := CaptureAuthorityACLCatalog(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range got {
			if c.Catalog == "pg_catalog.pg_class" && c.Column == "relacl" {
				found = true
			}
		}
		if !found {
			t.Fatalf("catalog discovery omitted pg_class.relacl: %+v", got)
		}
	})
}
