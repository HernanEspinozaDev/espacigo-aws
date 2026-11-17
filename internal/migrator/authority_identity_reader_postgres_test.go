package migrator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This test accepts only the harness-owned socket target, never a retained DB.
func TestRunnerAuthorityIdentityReaderPostgres(t *testing.T) {
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
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0o600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		conn := connectTestDB(t, ctx, databaseURL)
		defer conn.Close(context.Background())
		prefix := fmt.Sprintf("authority_identity_%d", time.Now().UnixNano())
		roles := []string{prefix + "_a", prefix + "_b", prefix + "_outside"}
		owned := map[string]uint32{}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			control := connectTestDB(t, cleanupCtx, os.Getenv("TEST_DATABASE_URL"))
			defer control.Close(context.Background())
			for _, name := range roles {
				oid, created := owned[name]
				if !created {
					continue
				}
				var current uint32
				if err := control.QueryRow(cleanupCtx, `SELECT oid FROM pg_catalog.pg_roles WHERE rolname=$1`, name).Scan(&current); err != nil || current != oid {
					t.Errorf("refuse cleanup of replaced own role %s: %v", name, err)
					continue
				}
				if _, err := control.Exec(cleanupCtx, "DROP ROLE "+pgx.Identifier{name}.Sanitize()); err != nil {
					t.Errorf("drop own role: %v", err)
				}
			}
		})
		for _, name := range roles {
			if _, err := conn.Exec(ctx, "CREATE ROLE "+pgx.Identifier{name}.Sanitize()+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT"); err != nil {
				t.Fatal(err)
			}
			var oid uint32
			if err := conn.QueryRow(ctx, `SELECT oid FROM pg_catalog.pg_roles WHERE rolname=$1`, name).Scan(&oid); err != nil {
				t.Fatal(err)
			}
			owned[name] = oid
		}
		for _, edge := range [][2]string{{roles[0], roles[1]}, {roles[2], roles[0]}} {
			q := "GRANT " + pgx.Identifier{edge[0]}.Sanitize() + " TO " + pgx.Identifier{edge[1]}.Sanitize() + " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
			if _, err := conn.Exec(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `CREATE TABLE public.authority_identity_probe(id bigint); CREATE SEQUENCE public.authority_identity_probe_seq; CREATE FUNCTION public.authority_identity_probe(integer) RETURNS integer LANGUAGE sql AS 'SELECT $1'; CREATE FUNCTION public.authority_identity_probe(text) RETURNS text LANGUAGE sql AS 'SELECT $1'`); err != nil {
			t.Fatal(err)
		}
		scope := AuthorityIdentityScope{
			Roles:   append([]string(nil), roles[:2]...),
			Objects: []AuthorityObjectRef{{Class: "database", Identity: conn.Config().Database}, {Class: "schema", Identity: "public"}, {Class: "relation", Identity: "public.authority_identity_probe"}, {Class: "sequence", Identity: "public.authority_identity_probe_seq"}, {Class: "function", Identity: "public.authority_identity_probe(integer)"}, {Class: "function", Identity: "public.authority_identity_probe(text)"}},
		}
		t.Run("read-only-repeatable-identity", func(t *testing.T) {
			tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			got, err := CaptureSingleOwnerAuthorityIdentity(ctx, tx, scope)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Roles) != len(scope.Roles) || len(got.Objects) != len(scope.Objects) {
				t.Fatalf("incomplete identity snapshot: %+v", got)
			}
			for _, name := range scope.Roles {
				found := false
				for _, role := range got.Roles {
					if role.Name == name {
						found = true
						if role != (AuthorityRole{Name: name}) {
							t.Fatalf("role attributes not captured exactly: %+v", role)
						}
					}
				}
				if !found {
					t.Fatalf("missing scoped role %s", name)
				}
			}
			for _, ref := range scope.Objects {
				found := false
				for _, object := range got.Objects {
					if object.Class == ref.Class && object.Identity == ref.Identity {
						found = true
						wantOwner := "postgres"
						if ref.Class == "schema" {
							wantOwner = "pg_database_owner"
						}
						if object.Owner != wantOwner {
							t.Fatalf("owner for %+v: %+v", ref, object)
						}
					}
				}
				if !found {
					t.Fatalf("missing canonical object %+v", ref)
				}
			}
			for _, want := range []AuthorityMembership{{Role: roles[0], Member: roles[1], SetOption: true}, {Role: roles[2], Member: roles[0], SetOption: true}} {
				found := false
				for _, edge := range got.Memberships {
					if edge == want {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing inbound/outbound scoped membership %+v", want)
				}
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_auth_members am JOIN pg_catalog.pg_roles r ON r.oid=am.roleid JOIN pg_catalog.pg_roles m ON m.oid=am.member WHERE r.rolname=ANY($1::text[]) OR m.rolname=ANY($1::text[])`, scope.Roles).Scan(&count); err != nil || count != len(got.Memberships) {
				t.Fatalf("membership inventory incomplete: %d %v", count, err)
			}
		})
		for _, name := range []string{"reject-read-write", "reject-read-committed", "missing-role-no-partial", "missing-object-no-partial", "foreign-database-no-partial"} {
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
				bad := AuthorityIdentityScope{Roles: append([]string(nil), scope.Roles...), Objects: append([]AuthorityObjectRef(nil), scope.Objects...)}
				if name == "missing-role-no-partial" {
					bad.Roles = append(bad.Roles, prefix+"_missing")
				}
				if name == "missing-object-no-partial" {
					bad.Objects = append(bad.Objects, AuthorityObjectRef{Class: "relation", Identity: "public.absent_authority_probe"})
				}
				if name == "foreign-database-no-partial" {
					bad.Objects = append(bad.Objects, AuthorityObjectRef{Class: "database", Identity: "postgres"})
				}
				got, err := CaptureSingleOwnerAuthorityIdentity(ctx, tx, bad)
				if err == nil || !reflect.DeepEqual(got, AuthorityIdentitySnapshot{}) {
					t.Fatalf("expected rejection with zero partial result: %+v %v", got, err)
				}
			})
		}
	})
}
