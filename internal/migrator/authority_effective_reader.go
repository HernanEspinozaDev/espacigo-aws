package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// AuthorityACLCatalogColumn describes an ACL-bearing catalog column. This inventory is
// discovered from PostgreSQL's catalogs at runtime; it is not a claim of complete authority.
type AuthorityACLCatalogColumn struct{ Catalog, Column string }

// CaptureAuthorityPrivilegeProbes resolves explicitly scoped identities in tx, then returns
// effective privilege values. Granted in each input is ignored. Caller must supply a stable
// READ ONLY REPEATABLE READ or SERIALIZABLE PostgreSQL 18 transaction.
func CaptureAuthorityPrivilegeProbes(ctx context.Context, tx pgx.Tx, probes []AuthorityPrivilegeProbe) ([]AuthorityPrivilegeProbe, error) {
	return captureAuthorityPrivilegeTransaction(ctx, tx, probes, true)
}

func captureAuthorityPrivilegeTransaction(ctx context.Context, tx pgx.Tx, probes []AuthorityPrivilegeProbe, requireReadOnly bool) ([]AuthorityPrivilegeProbe, error) {
	var zero []AuthorityPrivilegeProbe
	if ctx == nil || tx == nil {
		return zero, errors.New("effective privilege capture requires context and transaction")
	}
	if len(probes) == 0 {
		return zero, errors.New("effective privilege probes are empty")
	}
	refs := make([]AuthorityObjectRef, 0, len(probes))
	roles := make([]string, 0, len(probes))
	seen := map[probeIdentity]bool{}
	for _, p := range probes {
		for _, s := range []string{p.Role, p.ObjectClass, p.ObjectIdentity, p.Column, p.Privilege} {
			if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
				return zero, errors.New("probe contains invalid UTF-8 or NUL")
			}
		}
		if strings.TrimSpace(p.Role) == "" || strings.TrimSpace(p.ObjectIdentity) == "" {
			return zero, errors.New("probe identity is incomplete")
		}
		id := probeIdentity{p.Role, p.ObjectClass, p.ObjectIdentity, p.Column, p.Privilege}
		if seen[id] {
			return zero, errors.New("duplicate effective privilege probe")
		}
		seen[id] = true
		if !validProbePrivilege(p) {
			return zero, fmt.Errorf("unsupported privilege probe class/column/privilege: %q/%q", p.ObjectClass, p.Privilege)
		}
		roles = append(roles, p.Role)
		refs = append(refs, AuthorityObjectRef{Class: p.ObjectClass, Identity: p.ObjectIdentity})
	}
	if _, err := captureAuthorityIdentityTransaction(ctx, tx, AuthorityIdentityScope{Roles: dedupeStrings(roles), Objects: dedupeObjectRefs(refs)}, requireReadOnly); err != nil {
		return zero, err
	}
	out := append([]AuthorityPrivilegeProbe(nil), probes...)
	for i, p := range probes {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		var q string
		var args []any
		switch p.ObjectClass {
		case "database":
			q = `SELECT pg_catalog.has_database_privilege(r.oid,o.oid,$3) FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.pg_database o WHERE r.rolname=$1 AND o.datname=$2 AND o.datname=pg_catalog.current_database()`
			args = []any{p.Role, p.ObjectIdentity, p.Privilege}
		case "schema":
			q = `SELECT pg_catalog.has_schema_privilege(r.oid,o.oid,$3) FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.pg_namespace o WHERE r.rolname=$1 AND o.nspname=$2`
			args = []any{p.Role, p.ObjectIdentity, p.Privilege}
		case "relation", "sequence":
			kind := "c.relkind IN ('r','p','v','m','f')"
			fn := "has_table_privilege"
			if p.ObjectClass == "sequence" {
				kind = "c.relkind='S'"
				fn = "has_sequence_privilege"
			}
			q = `SELECT pg_catalog.` + fn + `(r.oid,c.oid,$3) FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE r.rolname=$1 AND ` + kind + ` AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$2`
			args = []any{p.Role, p.ObjectIdentity, p.Privilege}
			if p.Column != "" {
				q = `SELECT pg_catalog.has_column_privilege(r.oid,c.oid,a.attnum,$4) FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_attribute a ON a.attrelid=c.oid AND a.attname=$3 AND a.attnum>0 AND NOT a.attisdropped WHERE r.rolname=$1 AND c.relkind IN ('r','p','v','m','f') AND pg_catalog.format('%I.%I',n.nspname,c.relname)=$2`
				args = []any{p.Role, p.ObjectIdentity, p.Column, p.Privilege}
			}
		case "function":
			q = `SELECT pg_catalog.has_function_privilege(r.oid,p.oid,$3) FROM pg_catalog.pg_roles r CROSS JOIN pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE r.rolname=$1 AND p.prokind='f' AND pg_catalog.format('%I.%I(%s)',n.nspname,p.proname,pg_catalog.pg_get_function_identity_arguments(p.oid))=$2`
			args = []any{p.Role, p.ObjectIdentity, p.Privilege}
		}
		var granted bool
		if err := tx.QueryRow(ctx, q, args...).Scan(&granted); err != nil {
			return zero, fmt.Errorf("read effective privilege probe %d: %w", i, err)
		}
		out[i].Granted = granted
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return out, nil
}

func validProbePrivilege(p AuthorityPrivilegeProbe) bool {
	allowed := map[string]map[string]bool{"database": {"CREATE": true, "CONNECT": true, "TEMPORARY": true}, "schema": {"CREATE": true, "USAGE": true}, "relation": {"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "TRUNCATE": true, "REFERENCES": true, "TRIGGER": true, "MAINTAIN": true}, "sequence": {"USAGE": true, "SELECT": true, "UPDATE": true}, "function": {"EXECUTE": true}}
	if !allowed[p.ObjectClass][p.Privilege] || strings.Contains(p.Privilege, ",") {
		return false
	}
	if p.ObjectClass == "relation" {
		return p.Column == "" || (p.Privilege == "SELECT" || p.Privilege == "INSERT" || p.Privilege == "UPDATE" || p.Privilege == "REFERENCES")
	}
	return p.Column == ""
}
func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
func dedupeObjectRefs(in []AuthorityObjectRef) []AuthorityObjectRef {
	seen := map[AuthorityObjectRef]bool{}
	out := []AuthorityObjectRef{}
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// CaptureAuthorityACLCatalog discovers catalog columns declared aclitem[] under the same
// caller-owned transaction guard. The result is current PostgreSQL catalog metadata only.
func CaptureAuthorityACLCatalog(ctx context.Context, tx pgx.Tx) ([]AuthorityACLCatalogColumn, error) {
	return captureAuthorityACLCatalogTransaction(ctx, tx, true)
}

func captureAuthorityACLCatalogTransaction(ctx context.Context, tx pgx.Tx, requireReadOnly bool) ([]AuthorityACLCatalogColumn, error) {
	zero := []AuthorityACLCatalogColumn{}
	if ctx == nil || tx == nil {
		return nil, errors.New("ACL catalog capture requires context and transaction")
	}
	var ro bool
	var iso, first string
	var ver int
	if err := tx.QueryRow(ctx, `SELECT pg_catalog.current_setting('transaction_read_only') OPERATOR(pg_catalog.=) 'on',pg_catalog.current_setting('transaction_isolation'),pg_catalog.current_setting('server_version_num')::pg_catalog.int4,(pg_catalog.current_schemas(true))[1]::pg_catalog.text`).Scan(&ro, &iso, &ver, &first); err != nil {
		return nil, fmt.Errorf("check catalog transaction: %w", err)
	}
	if (requireReadOnly && !ro) || (iso != "repeatable read" && iso != "serializable") || (!ro && iso != "serializable") {
		return nil, errors.New("ACL catalog capture requires read-only repeatable-read or serializable transaction")
	}
	if ver < 180000 || ver >= 190000 {
		return nil, fmt.Errorf("ACL catalog capture requires PostgreSQL 18, got %d", ver)
	}
	if first != "pg_catalog" {
		return nil, errors.New("ACL catalog capture requires pg_catalog first in effective search path")
	}
	rows, err := tx.Query(ctx, `SELECT pg_catalog.format('%I.%I',n.nspname,c.relname)::text,a.attname::text FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE a.attnum>0 AND NOT a.attisdropped AND n.nspname='pg_catalog' AND c.relkind='r' AND a.atttypid='pg_catalog.aclitem[]'::pg_catalog.regtype ORDER BY n.nspname,c.relname,a.attnum`)
	if err != nil {
		return nil, fmt.Errorf("discover ACL catalog columns: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			rows.Close()
		}
	}()
	for rows.Next() {
		var x AuthorityACLCatalogColumn
		if err := rows.Scan(&x.Catalog, &x.Column); err != nil {
			return nil, fmt.Errorf("scan ACL catalog column: %w", err)
		}
		zero = append(zero, x)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ACL catalog columns: %w", err)
	}
	rows.Close()
	closed = true
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return zero, nil
}
