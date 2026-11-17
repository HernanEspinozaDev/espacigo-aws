package migrator

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type AuthorityExcludedACLBaseline struct{ Catalog, Column, SHA256 string }

// Excluded classes are not modified by the E1 delta. Their complete ACL-bearing
// rows are nevertheless pinned before/inside/after the operation. These hashes
// are intentionally target/cluster-specific, not portable restore semantics.
func captureExcludedACLBaselines(ctx context.Context, tx pgx.Tx, coverage []AuthorityCatalogCoverage) ([]AuthorityExcludedACLBaseline, error) {
	out := []AuthorityExcludedACLBaseline{}
	for _, c := range coverage {
		if c.Disposition != "excluded" {
			continue
		}
		var selectRows string
		switch (AuthorityACLCatalogColumn{c.Catalog, c.Column}) {
		case AuthorityACLCatalogColumn{"pg_catalog.pg_foreign_data_wrapper", "fdwacl"}:
			selectRows = `SELECT oid,fdwowner,fdwacl FROM pg_catalog.pg_foreign_data_wrapper ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_foreign_server", "srvacl"}:
			selectRows = `SELECT oid,srvowner,srvacl FROM pg_catalog.pg_foreign_server ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_init_privs", "initprivs"}:
			selectRows = `SELECT objoid,classoid,objsubid,privtype,initprivs FROM pg_catalog.pg_init_privs ORDER BY classoid,objoid,objsubid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_language", "lanacl"}:
			selectRows = `SELECT oid,lanname,lanowner,lanacl FROM pg_catalog.pg_language ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_largeobject_metadata", "lomacl"}:
			selectRows = `SELECT oid,lomowner,lomacl FROM pg_catalog.pg_largeobject_metadata ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_parameter_acl", "paracl"}:
			selectRows = `SELECT oid,parname,paracl FROM pg_catalog.pg_parameter_acl ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_tablespace", "spcacl"}:
			selectRows = `SELECT oid,spcname,spcowner,spcacl FROM pg_catalog.pg_tablespace ORDER BY oid`
		case AuthorityACLCatalogColumn{"pg_catalog.pg_type", "typacl"}:
			selectRows = `SELECT oid,typnamespace,typname,typowner,typacl FROM pg_catalog.pg_type WHERE typacl IS NOT NULL ORDER BY oid`
		default:
			return nil, errors.New("unimplemented ACL exclusion baseline")
		}
		query := `SELECT pg_catalog.encode(pg_catalog.sha256(pg_catalog.convert_to(COALESCE(pg_catalog.jsonb_agg(pg_catalog.to_jsonb(rows))::pg_catalog.text,'[]'),'UTF8')),'hex') FROM (` + selectRows + `) rows`
		var digest string
		if err := tx.QueryRow(ctx, query).Scan(&digest); err != nil {
			return nil, err
		}
		if !validHex(digest, 64) {
			return nil, errors.New("invalid excluded ACL baseline digest")
		}
		out = append(out, AuthorityExcludedACLBaseline{c.Catalog, c.Column, digest})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
