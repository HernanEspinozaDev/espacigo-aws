package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type AuthorityDefaultACLState struct{ Owner, Schema, Class, State string }
type AuthorityDefaultACLObservation struct {
	ACLs   []AuthorityDefaultACL
	States []AuthorityDefaultACLState
}

func CaptureScopedAuthorityDefaultACLs(ctx context.Context, tx pgx.Tx, owners []string) (AuthorityDefaultACLObservation, error) {
	return captureAuthorityDefaultACLTransaction(ctx, tx, owners, true)
}

func captureAuthorityDefaultACLTransaction(ctx context.Context, tx pgx.Tx, owners []string, requireReadOnly bool) (AuthorityDefaultACLObservation, error) {
	zero := AuthorityDefaultACLObservation{}
	if ctx == nil || tx == nil {
		return zero, errors.New("default ACL capture requires context and caller-owned transaction")
	}
	if len(owners) == 0 {
		return zero, errors.New("default ACL capture requires explicit owners")
	}
	seen := map[string]bool{}
	for _, owner := range owners {
		if !utf8.ValidString(owner) || strings.ContainsRune(owner, 0) || strings.TrimSpace(owner) == "" || seen[owner] {
			return zero, errors.New("default ACL owner scope is invalid or duplicated")
		}
		seen[owner] = true
	}
	_, err := captureAuthorityIdentityTransaction(ctx, tx, AuthorityIdentityScope{Roles: owners}, requireReadOnly)
	if err != nil {
		return zero, err
	}
	out := AuthorityDefaultACLObservation{ACLs: []AuthorityDefaultACL{}, States: []AuthorityDefaultACLState{}}
	rows, err := tx.Query(ctx, `SELECT owner.rolname, COALESCE(ns.nspname,''),
 CASE d.defaclobjtype WHEN 'r' THEN 'relation' WHEN 'S' THEN 'sequence' WHEN 'f' THEN 'function' WHEN 'T' THEN 'type' WHEN 'n' THEN 'schema' END,
 COALESCE(pg_catalog.pg_get_userbyid(x.grantor),''),
 CASE WHEN x.grantee OPERATOR(pg_catalog.=) 0::pg_catalog.oid THEN 'PUBLIC' ELSE COALESCE(pg_catalog.pg_get_userbyid(x.grantee),'') END,
 COALESCE(x.privilege_type,''),COALESCE(x.is_grantable,false),
 CASE WHEN d.defaclacl IS NULL THEN 'null' WHEN pg_catalog.cardinality(d.defaclacl)=0 THEN 'empty' ELSE 'explicit' END
 FROM pg_catalog.pg_default_acl d JOIN pg_catalog.pg_roles owner ON owner.oid=d.defaclrole
 LEFT JOIN pg_catalog.pg_namespace ns ON ns.oid=d.defaclnamespace
 LEFT JOIN LATERAL pg_catalog.aclexplode(CASE WHEN d.defaclacl IS NOT NULL AND pg_catalog.cardinality(d.defaclacl)>0 THEN d.defaclacl ELSE NULL::pg_catalog.aclitem[] END) x ON true
 WHERE owner.rolname=ANY($1::text[])
 ORDER BY owner.rolname,COALESCE(ns.nspname,''),d.defaclobjtype,x.grantor,x.grantee,x.privilege_type`, owners)
	if err != nil {
		return zero, fmt.Errorf("query default ACL catalog: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			rows.Close()
		}
	}()
	seenRows := map[AuthorityDefaultACLState]bool{}
	for rows.Next() {
		var owner, schema, class, grantor, grantee, privilege, state string
		var grantOption bool
		if err := rows.Scan(&owner, &schema, &class, &grantor, &grantee, &privilege, &grantOption, &state); err != nil {
			return zero, fmt.Errorf("scan default ACL catalog: %w", err)
		}
		if (state != "null" && state != "empty" && state != "explicit") || class == "" {
			return zero, errors.New("default ACL catalog returned invalid metadata")
		}
		key := AuthorityDefaultACLState{Owner: owner, Schema: schema, Class: class, State: state}
		if !seenRows[key] {
			out.States = append(out.States, key)
			seenRows[key] = true
		}
		if privilege == "" {
			if grantor != "" || grantee != "" || grantOption {
				return zero, errors.New("default ACL catalog returned partial tuple")
			}
			continue
		}
		if grantor == "" || grantee == "" || state != "explicit" {
			return zero, errors.New("default ACL catalog returned contradictory tuple")
		}
		out.ACLs = append(out.ACLs, AuthorityDefaultACL{Owner: owner, Grantor: grantor, Schema: schema, Class: class, Grantee: grantee, Privilege: privilege, GrantOption: grantOption})
	}
	if err := rows.Err(); err != nil {
		return zero, fmt.Errorf("iterate default ACL catalog: %w", err)
	}
	rows.Close()
	closed = true
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return out, nil
}
