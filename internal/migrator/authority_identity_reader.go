package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type AuthorityObjectRef struct{ Class, Identity string }
type AuthorityIdentityScope struct {
	Roles   []string
	Objects []AuthorityObjectRef
}
type AuthorityIdentitySnapshot struct {
	Roles       []AuthorityRole
	Objects     []AuthorityObject
	Memberships []AuthorityMembership
}

type authorityIdentityQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CaptureSingleOwnerAuthorityIdentity reads a deliberately partial identity inventory.
// It does not inspect ACLs or default privileges and must not be treated as a full authority snapshot.
func CaptureSingleOwnerAuthorityIdentity(ctx context.Context, tx pgx.Tx, scope AuthorityIdentityScope) (AuthorityIdentitySnapshot, error) {
	return captureSingleOwnerAuthorityIdentity(ctx, tx, scope)
}

func captureSingleOwnerAuthorityIdentity(ctx context.Context, q authorityIdentityQueryer, scope AuthorityIdentityScope) (AuthorityIdentitySnapshot, error) {
	zero := AuthorityIdentitySnapshot{}
	if ctx == nil || q == nil {
		return zero, errors.New("identity capture requires context and caller-owned transaction")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if len(scope.Roles) == 0 && len(scope.Objects) == 0 {
		return zero, errors.New("authority identity scope is empty")
	}
	rolesSeen := map[string]bool{}
	for _, r := range scope.Roles {
		if strings.TrimSpace(r) == "" || rolesSeen[r] {
			return zero, errors.New("authority role scope is incomplete or duplicated")
		}
		rolesSeen[r] = true
	}
	objectsSeen := map[AuthorityObjectRef]bool{}
	allowed := map[string]bool{"database": true, "schema": true, "relation": true, "sequence": true, "function": true}
	for _, o := range scope.Objects {
		if !allowed[o.Class] || strings.TrimSpace(o.Identity) == "" || objectsSeen[o] {
			return zero, errors.New("authority object scope is unknown, incomplete, or duplicated")
		}
		objectsSeen[o] = true
	}
	var readOnly bool
	var isolation string
	var version int
	var firstSchema string
	if err := q.QueryRow(ctx, `SELECT pg_catalog.current_setting('transaction_read_only') OPERATOR(pg_catalog.=) 'on', pg_catalog.current_setting('transaction_isolation'), pg_catalog.current_setting('server_version_num')::pg_catalog.int4, (pg_catalog.current_schemas(true))[1]::pg_catalog.text`).Scan(&readOnly, &isolation, &version, &firstSchema); err != nil {
		return zero, fmt.Errorf("check transaction and server version: %w", err)
	}
	if !readOnly || (isolation != "repeatable read" && isolation != "serializable") {
		return zero, errors.New("identity capture requires a read-only repeatable-read or serializable transaction")
	}
	if version < 180000 || version >= 190000 {
		return zero, fmt.Errorf("identity capture requires PostgreSQL 18, got version number %d", version)
	}
	if firstSchema != "pg_catalog" {
		return zero, errors.New("identity capture requires pg_catalog first in the effective search path")
	}
	out := AuthorityIdentitySnapshot{Roles: []AuthorityRole{}, Objects: []AuthorityObject{}, Memberships: []AuthorityMembership{}}
	rows, err := q.Query(ctx, `SELECT r.rolname, r.rolsuper, r.rolcreatedb, r.rolcreaterole, r.rolreplication, r.rolbypassrls, r.rolcanlogin, r.rolinherit FROM pg_catalog.pg_roles r WHERE r.rolname = ANY($1::text[]) ORDER BY r.rolname`, scope.Roles)
	if err != nil {
		return zero, fmt.Errorf("query scoped roles: %w", err)
	}
	for rows.Next() {
		var x AuthorityRole
		if err = rows.Scan(&x.Name, &x.Superuser, &x.CreateDB, &x.CreateRole, &x.Replication, &x.BypassRLS, &x.Login, &x.Inherit); err != nil {
			rows.Close()
			return zero, fmt.Errorf("scan scoped role: %w", err)
		}
		out.Roles = append(out.Roles, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return zero, fmt.Errorf("iterate scoped roles: %w", err)
	}
	if len(out.Roles) != len(scope.Roles) {
		return zero, errors.New("one or more scoped roles do not exist")
	}
	for _, ref := range scope.Objects {
		var sql string
		switch ref.Class {
		case "database":
			sql = `SELECT d.datname, pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname=$1 AND d.datname=pg_catalog.current_database()`
		case "schema":
			sql = `SELECT n.nspname, pg_catalog.pg_get_userbyid(n.nspowner) FROM pg_catalog.pg_namespace n WHERE n.nspname=$1`
		case "relation":
			sql = `SELECT pg_catalog.format('%I.%I',n.nspname,c.relname), pg_catalog.pg_get_userbyid(c.relowner) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','v','m','f') AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$1`
		case "sequence":
			sql = `SELECT pg_catalog.format('%I.%I',n.nspname,c.relname), pg_catalog.pg_get_userbyid(c.relowner) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='S' AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$1`
		case "function":
			sql = `SELECT pg_catalog.format('%I.%I(%s)',n.nspname,p.proname,pg_catalog.pg_get_function_identity_arguments(p.oid)), pg_catalog.pg_get_userbyid(p.proowner) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE p.prokind='f' AND pg_catalog.format('%I.%I(%s)',n.nspname,p.proname,pg_catalog.pg_get_function_identity_arguments(p.oid))=$1`
		}
		var identity, owner string
		if err := q.QueryRow(ctx, sql, ref.Identity).Scan(&identity, &owner); err != nil {
			return zero, fmt.Errorf("resolve scoped %s %q: %w", ref.Class, ref.Identity, err)
		}
		out.Objects = append(out.Objects, AuthorityObject{Class: ref.Class, Identity: identity, Owner: owner})
	}
	if len(scope.Roles) > 0 {
		rows, err = q.Query(ctx, `SELECT granted.rolname, member.rolname, m.admin_option, m.inherit_option, m.set_option FROM pg_catalog.pg_auth_members m JOIN pg_catalog.pg_roles granted ON granted.oid=m.roleid JOIN pg_catalog.pg_roles member ON member.oid=m.member WHERE granted.rolname=ANY($1::text[]) OR member.rolname=ANY($1::text[]) ORDER BY granted.rolname,member.rolname`, scope.Roles)
		if err != nil {
			return zero, fmt.Errorf("query scoped memberships: %w", err)
		}
		for rows.Next() {
			var x AuthorityMembership
			if err = rows.Scan(&x.Role, &x.Member, &x.AdminOption, &x.InheritOption, &x.SetOption); err != nil {
				rows.Close()
				return zero, fmt.Errorf("scan scoped membership: %w", err)
			}
			out.Memberships = append(out.Memberships, x)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return zero, fmt.Errorf("iterate scoped memberships: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return out, nil
}
