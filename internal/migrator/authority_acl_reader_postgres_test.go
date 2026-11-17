package migrator

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Only the existing networkless disposable harness may enable this test.
func TestRunnerAuthorityACLReaderPostgres(t *testing.T) {
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
		_, err := conn.Exec(ctx, `
CREATE SCHEMA acl_probe;
CREATE TABLE acl_probe.unset(id bigint, extra text, dropped bigint);
ALTER TABLE acl_probe.unset DROP COLUMN dropped;
CREATE SEQUENCE acl_probe.seq;
CREATE FUNCTION acl_probe.fn(integer) RETURNS integer LANGUAGE sql AS 'SELECT $1';
CREATE FUNCTION acl_probe.fn(text) RETURNS text LANGUAGE sql AS 'SELECT $1';
CREATE TABLE acl_probe.explicit(visible bigint, hidden text);
GRANT SELECT ON TABLE acl_probe.explicit TO pg_read_all_stats WITH GRANT OPTION;
GRANT SELECT (visible) ON TABLE acl_probe.explicit TO PUBLIC;
GRANT UPDATE (hidden) ON TABLE acl_probe.explicit TO pg_read_all_stats WITH GRANT OPTION;
CREATE TABLE acl_probe.empty(id bigint);
REVOKE ALL ON TABLE acl_probe.empty FROM postgres;
CREATE SCHEMA "ACL Odd";
CREATE TABLE "ACL Odd"."T""able"(id bigint);
CREATE FUNCTION acl_probe.current_setting(text) RETURNS text LANGUAGE sql AS $$ SELECT CASE $1 WHEN 'transaction_read_only' THEN 'on' WHEN 'transaction_isolation' THEN 'repeatable read' ELSE '180000' END $$;
CREATE FUNCTION acl_probe.current_database() RETURNS name LANGUAGE sql AS $$ SELECT 'postgres'::name $$;
CREATE FUNCTION acl_probe.cardinality(pg_catalog.aclitem[]) RETURNS integer LANGUAGE sql AS 'SELECT 0';
CREATE FUNCTION acl_probe.oid_equals_integer(pg_catalog.oid, pg_catalog.int4) RETURNS boolean LANGUAGE sql AS 'SELECT true';
CREATE OPERATOR acl_probe.= (LEFTARG=pg_catalog.oid, RIGHTARG=pg_catalog.int4, FUNCTION=acl_probe.oid_equals_integer);
`)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("reject-shadowed-transaction-guard", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(ctx, `SET LOCAL search_path TO acl_probe, pg_catalog`); err != nil {
				t.Fatal(err)
			}
			got, err := CaptureScopedAuthorityACLs(ctx, tx, []AuthorityObjectRef{{Class: "schema", Identity: "acl_probe"}})
			if err == nil || !strings.Contains(err.Error(), "read-only repeatable-read") || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
				t.Fatalf("shadowed transaction settings bypassed real read-only guard: %+v %v", got, err)
			}
		})
		t.Run("reject-untrusted-search-path", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(ctx, `SET LOCAL search_path TO acl_probe, pg_catalog`); err != nil {
				t.Fatal(err)
			}
			got, err := CaptureScopedAuthorityACLs(ctx, tx, []AuthorityObjectRef{{Class: "schema", Identity: "acl_probe"}})
			if err == nil || !strings.Contains(err.Error(), "pg_catalog first") || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
				t.Fatalf("untrusted lookup path accepted: %+v %v", got, err)
			}
		})
		t.Run("identity-reject-shadowed-current-database", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(ctx, `SET LOCAL search_path TO acl_probe, pg_catalog`); err != nil {
				t.Fatal(err)
			}
			got, err := CaptureSingleOwnerAuthorityIdentity(ctx, tx, AuthorityIdentityScope{Objects: []AuthorityObjectRef{{Class: "database", Identity: "postgres"}}})
			if err == nil || !reflect.DeepEqual(got, AuthorityIdentitySnapshot{}) {
				t.Fatalf("shadowed database function escaped target scope: %+v %v", got, err)
			}
		})
		for _, testName := range []string{"ignore-shadowed-cardinality", "ignore-shadowed-oid-operator"} {
			t.Run(testName, func(t *testing.T) {
				tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				if _, err := tx.Exec(ctx, `SET LOCAL search_path TO pg_catalog, acl_probe`); err != nil {
					t.Fatal(err)
				}
				got, err := CaptureScopedAuthorityACLs(ctx, tx, []AuthorityObjectRef{{Class: "relation", Identity: "acl_probe.explicit"}})
				if err != nil {
					t.Fatal(err)
				}
				assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.explicit", State: "explicit"})
				assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: "acl_probe.explicit", Grantor: "postgres", Grantee: "pg_read_all_stats", Privilege: "SELECT", GrantOption: true})
			})
		}
		base := []AuthorityObjectRef{
			{Class: "database", Identity: conn.Config().Database},
			{Class: "schema", Identity: "acl_probe"},
			{Class: "relation", Identity: "acl_probe.unset"},
			{Class: "sequence", Identity: "acl_probe.seq"},
			{Class: "function", Identity: "acl_probe.fn(integer)"},
			{Class: "function", Identity: "acl_probe.fn(text)"},
		}
		capture := func(t *testing.T, objects []AuthorityObjectRef) AuthorityACLObservation {
			t.Helper()
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureScopedAuthorityACLs(ctx, tx, objects)
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		t.Run("null-acl-builtins-and-columns", func(t *testing.T) {
			got := capture(t, base)
			if len(got.Objects) != len(base) || got.ACLs == nil || got.ACLStates == nil {
				t.Fatalf("incomplete observation: %+v", got)
			}
			for _, ref := range base {
				assertAuthorityACLState(t, got, AuthorityACLState{Class: ref.Class, ObjectIdentity: ref.Identity, State: "null"})
			}
			for _, column := range []string{"id", "extra"} {
				assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.unset", Column: column, State: "null"})
			}
			for _, acl := range got.ACLs {
				if acl.ObjectIdentity == "acl_probe.unset" && acl.Column != "" {
					t.Fatalf("null column ACL invented a direct grant: %+v", acl)
				}
			}
			for _, state := range got.ACLStates {
				if state.Column == "dropped" {
					t.Fatal("dropped column included")
				}
			}
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "database", ObjectIdentity: conn.Config().Database, Grantor: "postgres", Grantee: "postgres", Privilege: "CREATE"})
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "schema", ObjectIdentity: "acl_probe", Grantor: "postgres", Grantee: "postgres", Privilege: "CREATE"})
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: "acl_probe.unset", Grantor: "postgres", Grantee: "postgres", Privilege: "SELECT"})
			for _, privilege := range []string{"SELECT", "UPDATE", "USAGE"} {
				assertAuthorityACLTuple(t, got, AuthorityACL{Class: "sequence", ObjectIdentity: "acl_probe.seq", Grantor: "postgres", Grantee: "postgres", Privilege: privilege})
			}
			for _, signature := range []string{"integer", "text"} {
				assertAuthorityACLTuple(t, got, AuthorityACL{Class: "function", ObjectIdentity: "acl_probe.fn(" + signature + ")", Grantor: "postgres", Grantee: "PUBLIC", Privilege: "EXECUTE"})
			}
		})
		t.Run("explicit-public-column-and-grant-option", func(t *testing.T) {
			got := capture(t, []AuthorityObjectRef{{Class: "relation", Identity: "acl_probe.explicit"}})
			assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.explicit", State: "explicit"})
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: "acl_probe.explicit", Grantor: "postgres", Grantee: "pg_read_all_stats", Privilege: "SELECT", GrantOption: true})
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: "acl_probe.explicit", Column: "visible", Grantor: "postgres", Grantee: "PUBLIC", Privilege: "SELECT"})
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: "acl_probe.explicit", Column: "hidden", Grantor: "postgres", Grantee: "pg_read_all_stats", Privilege: "UPDATE", GrantOption: true})
			for _, column := range []string{"visible", "hidden"} {
				assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.explicit", Column: column, State: "explicit"})
			}
		})
		t.Run("empty-acl-distinct-from-null", func(t *testing.T) {
			got := capture(t, []AuthorityObjectRef{{Class: "relation", Identity: "acl_probe.empty"}})
			assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.empty", State: "empty"})
			assertAuthorityACLState(t, got, AuthorityACLState{Class: "relation", ObjectIdentity: "acl_probe.empty", Column: "id", State: "null"})
			if len(got.ACLs) != 0 {
				t.Fatalf("explicit empty ACL incorrectly restored owner defaults: %+v", got.ACLs)
			}
		})
		t.Run("quoted-canonical-identity", func(t *testing.T) {
			identity := `"ACL Odd"."T""able"`
			got := capture(t, []AuthorityObjectRef{{Class: "relation", Identity: identity}})
			if !reflect.DeepEqual(got.Objects, []AuthorityObject{{Class: "relation", Identity: identity, Owner: "postgres"}}) {
				t.Fatalf("quoted identity changed: %+v", got.Objects)
			}
			assertAuthorityACLTuple(t, got, AuthorityACL{Class: "relation", ObjectIdentity: identity, Grantor: "postgres", Grantee: "postgres", Privilege: "SELECT"})
		})
		for _, name := range []string{"reject-read-write", "reject-read-committed", "missing-object-no-partial", "foreign-database-no-partial", "invalid-scope-no-partial", "canceled-no-partial"} {
			t.Run(name, func(t *testing.T) {
				options := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
				if name == "reject-read-write" {
					options.AccessMode = pgx.ReadWrite
				}
				if name == "reject-read-committed" {
					options.IsoLevel = pgx.ReadCommitted
				}
				tx, err := conn.BeginTx(ctx, options)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				objects := append([]AuthorityObjectRef(nil), base...)
				switch name {
				case "missing-object-no-partial":
					objects = append(objects, AuthorityObjectRef{Class: "relation", Identity: "acl_probe.absent"})
				case "foreign-database-no-partial":
					objects = append(objects, AuthorityObjectRef{Class: "database", Identity: "postgres"})
				case "invalid-scope-no-partial":
					objects = append(objects, base[0])
				}
				readCtx := ctx
				if name == "canceled-no-partial" {
					var cancel context.CancelFunc
					readCtx, cancel = context.WithCancel(ctx)
					cancel()
				}
				got, err := CaptureScopedAuthorityACLs(readCtx, tx, objects)
				if err == nil || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
					t.Fatalf("expected rejection without partial ACLs: %+v %v", got, err)
				}
			})
		}
	})
}

func assertAuthorityACLState(t *testing.T, got AuthorityACLObservation, want AuthorityACLState) {
	t.Helper()
	for _, state := range got.ACLStates {
		if state == want {
			return
		}
	}
	t.Fatalf("missing raw ACL state %+v in %+v", want, got.ACLStates)
}

func assertAuthorityACLTuple(t *testing.T, got AuthorityACLObservation, want AuthorityACL) {
	t.Helper()
	for _, acl := range got.ACLs {
		if acl == want {
			return
		}
	}
	t.Fatalf("missing ACL tuple %+v in %+v", want, got.ACLs)
}
