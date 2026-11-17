package migrator

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"unicode/utf8"
)

// AuthorityACLState preserves the stored ACL shape separately from normalized privileges.
type AuthorityACLState struct{ Class, ObjectIdentity, Column, State string }

// AuthorityACLObservation is scoped and partial, not a complete operational authority snapshot.
type AuthorityACLObservation struct {
	Objects   []AuthorityObject
	ACLs      []AuthorityACL
	ACLStates []AuthorityACLState
}

func CaptureScopedAuthorityACLs(ctx context.Context, tx pgx.Tx, objects []AuthorityObjectRef) (AuthorityACLObservation, error) {
	return captureScopedAuthorityACLs(ctx, tx, objects)
}
func captureScopedAuthorityACLs(ctx context.Context, q authorityIdentityQueryer, objects []AuthorityObjectRef) (AuthorityACLObservation, error) {
	zero := AuthorityACLObservation{}
	if len(objects) == 0 {
		return zero, errors.New("ACL capture requires an explicit nonempty object scope")
	}
	for _, ref := range objects {
		if !utf8.ValidString(ref.Class) || !utf8.ValidString(ref.Identity) || strings.ContainsRune(ref.Class, 0) || strings.ContainsRune(ref.Identity, 0) {
			return zero, errors.New("ACL scope contains invalid UTF-8 or NUL")
		}
	}
	identity, err := captureSingleOwnerAuthorityIdentity(ctx, q, AuthorityIdentityScope{Objects: objects})
	if err != nil {
		return zero, err
	}
	out := AuthorityACLObservation{Objects: identity.Objects, ACLs: []AuthorityACL{}, ACLStates: []AuthorityACLState{}}
	for _, object := range identity.Objects {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		query, err := scopedAuthorityACLQuery(object.Class)
		if err != nil {
			return zero, err
		}
		rows, err := q.Query(ctx, query, object.Identity)
		if err != nil {
			return zero, fmt.Errorf("query scoped ACL: %w", err)
		}
		if err := appendScopedAuthorityACLRows(rows, object, &out); err != nil {
			return zero, err
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return out, nil
}

func appendScopedAuthorityACLRows(rows pgx.Rows, object AuthorityObject, out *AuthorityACLObservation) error {
	defer rows.Close()
	states := map[string]string{}
	tuples := map[aclIdentity]bool{}
	for rows.Next() {
		var column, state, grantor, grantee, privilege string
		var grantOption bool
		if err := rows.Scan(&column, &state, &grantor, &grantee, &privilege, &grantOption); err != nil {
			return fmt.Errorf("scan scoped ACL: %w", err)
		}
		if (state != "null" && state != "empty" && state != "explicit") || (column != "" && object.Class != "relation") {
			return errors.New("scoped ACL has an invalid raw state or column")
		}
		if prior, seen := states[column]; seen {
			if prior != state {
				return errors.New("scoped ACL has conflicting raw states")
			}
		} else {
			states[column] = state
			out.ACLStates = append(out.ACLStates, AuthorityACLState{Class: object.Class, ObjectIdentity: object.Identity, Column: column, State: state})
		}
		if privilege == "" {
			if grantor != "" || grantee != "" || grantOption {
				return errors.New("scoped ACL contains a partial tuple")
			}
			continue
		}
		if grantor == "" || grantee == "" || state == "empty" || (column != "" && state == "null") {
			return errors.New("scoped ACL contains an incomplete or contradictory tuple")
		}
		id := aclIdentity{object.Class, object.Identity, column, grantor, grantee, privilege}
		if tuples[id] {
			return errors.New("scoped ACL contains a duplicate tuple")
		}
		tuples[id] = true
		out.ACLs = append(out.ACLs, AuthorityACL{Class: object.Class, ObjectIdentity: object.Identity, Column: column, Grantor: grantor, Grantee: grantee, Privilege: privilege, GrantOption: grantOption})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate scoped ACL: %w", err)
	}
	if _, found := states[""]; !found {
		return errors.New("scoped ACL omits object metadata")
	}
	return nil
}

func scopedAuthorityACLQuery(class string) (string, error) {
	var selected, selector string
	switch class {
	case "database":
		selected = `SELECT d.oid,d.datdba AS owner,d.datacl AS raw_acl FROM pg_catalog.pg_database d WHERE d.datname=$1 AND d.datname=pg_catalog.current_database()`
		selector = "d"
	case "schema":
		selected = `SELECT n.oid,n.nspowner AS owner,n.nspacl AS raw_acl FROM pg_catalog.pg_namespace n WHERE n.nspname=$1`
		selector = "n"
	case "relation":
		selected = `SELECT c.oid,c.relowner AS owner,c.relacl AS raw_acl FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','v','m','f') AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$1`
		selector = "r"
	case "sequence":
		selected = `SELECT c.oid,c.relowner AS owner,c.relacl AS raw_acl FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='S' AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$1`
		// acldefault uses lowercase s; uppercase S means FOREIGN SERVER.
		selector = "s"
	case "function":
		selected = `SELECT p.oid,p.proowner AS owner,p.proacl AS raw_acl FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE p.prokind='f' AND pg_catalog.format('%I.%I(%s)',n.nspname,p.proname,pg_catalog.pg_get_function_identity_arguments(p.oid))=$1`
		selector = "f"
	default:
		return "", fmt.Errorf("ACL capture does not support class %q", class)
	}
	// Only fixed fragments selected above are composed; all identities remain parameters.
	entries := `SELECT ''::text AS col,o.raw_acl,pg_catalog.acldefault('` + selector + `'::pg_catalog."char",o.owner) AS fallback FROM selected o`
	if class == "relation" {
		entries += ` UNION ALL SELECT a.attname::text,a.attacl,NULL::pg_catalog.aclitem[] FROM selected o JOIN pg_catalog.pg_attribute a ON a.attrelid=o.oid WHERE a.attnum>0 AND NOT a.attisdropped`
	}
	return `WITH selected AS (` + selected + `), entries AS (` + entries + `)
SELECT e.col, CASE WHEN e.raw_acl IS NULL THEN 'null' WHEN pg_catalog.cardinality(e.raw_acl)=0 THEN 'empty' ELSE 'explicit' END,
COALESCE(pg_catalog.pg_get_userbyid(x.grantor),''),
CASE WHEN x.grantee OPERATOR(pg_catalog.=) 0::pg_catalog.oid THEN 'PUBLIC' ELSE COALESCE(pg_catalog.pg_get_userbyid(x.grantee),'') END,
COALESCE(x.privilege_type,''), COALESCE(x.is_grantable,false)
FROM entries e LEFT JOIN LATERAL pg_catalog.aclexplode(
CASE WHEN e.raw_acl IS NULL THEN e.fallback WHEN pg_catalog.cardinality(e.raw_acl)>0 THEN e.raw_acl ELSE NULL::pg_catalog.aclitem[] END
) x ON true ORDER BY e.col,x.grantor,x.grantee,x.privilege_type`, nil
}
