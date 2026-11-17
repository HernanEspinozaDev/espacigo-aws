package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// AuthorityCatalogCoverage accounts for every ACL-bearing PostgreSQL catalog
// column. An exclusion must state why that class cannot authorize this delta.
// It is not a claim that every object in the cluster is least-privileged.
type AuthorityCatalogCoverage struct{ Catalog, Column, Disposition, Reason, BaselineSHA256 string }

type SingleOwnerAuthorityProfile struct {
	Expected      SingleOwnerAuthoritySnapshot
	ACLStates     []AuthorityACLState
	DefaultStates []AuthorityDefaultACLState
	Coverage      []AuthorityCatalogCoverage
}

type SingleOwnerAuthorityObservation struct {
	Snapshot      SingleOwnerAuthoritySnapshot
	ACLStates     []AuthorityACLState
	DefaultStates []AuthorityDefaultACLState
	Catalog       []AuthorityACLCatalogColumn
	ExcludedACLs  []AuthorityExcludedACLBaseline
}

// CaptureSingleOwnerAuthority is a read-only inspection of the independently
// supplied profile's exact scope. It never learns or approves expected values.
func CaptureSingleOwnerAuthority(ctx context.Context, tx pgx.Tx, profile SingleOwnerAuthorityProfile) (SingleOwnerAuthorityObservation, error) {
	return captureOperationalAuthority(ctx, tx, profile, true)
}

func validateOperationalAuthorityProfile(p SingleOwnerAuthorityProfile) error {
	if err := validateAuthoritySnapshot("profile", &p.Expected); err != nil {
		return err
	}
	if p.ACLStates == nil || p.DefaultStates == nil || len(p.Coverage) == 0 {
		return errors.New("profile omits ACL state or catalog coverage")
	}
	roleSet := map[string]bool{}
	for _, r := range p.Expected.Roles {
		roleSet[r.Name] = true
		if r.Superuser || r.CreateDB || r.CreateRole || r.Replication || r.BypassRLS {
			return errors.New("operational authority profile permits an elevated role")
		}
	}
	objSet := map[objectIdentity]bool{}
	for _, o := range p.Expected.Objects {
		objSet[objectIdentity{o.Class, o.Identity}] = true
	}
	for _, a := range p.Expected.ACLs {
		if !objSet[objectIdentity{a.Class, a.ObjectIdentity}] {
			return errors.New("ACL tuple is outside profile object scope")
		}
	}
	for _, a := range p.Expected.DefaultACLs {
		if !roleSet[a.Owner] {
			return errors.New("default ACL owner is outside profile role scope")
		}
	}
	for _, a := range p.Expected.EffectivePrivileges {
		if !roleSet[a.Role] || !objSet[objectIdentity{a.ObjectClass, a.ObjectIdentity}] {
			return errors.New("probe lies outside profile scope")
		}
	}
	stateSet := map[struct{ Class, Object, Column string }]bool{}
	for _, a := range p.ACLStates {
		key := struct{ Class, Object, Column string }{a.Class, a.ObjectIdentity, a.Column}
		if !objSet[objectIdentity{a.Class, a.ObjectIdentity}] || stateSet[key] || (a.State != "null" && a.State != "empty" && a.State != "explicit") {
			return errors.New("invalid or duplicate profile ACL state")
		}
		stateSet[key] = true
	}
	for _, o := range p.Expected.Objects {
		if !stateSet[struct{ Class, Object, Column string }{o.Class, o.Identity, ""}] {
			return errors.New("profile omits object ACL metadata")
		}
	}
	defaultSet := map[struct{ Owner, Schema, Class string }]bool{}
	for _, a := range p.DefaultStates {
		key := struct{ Owner, Schema, Class string }{a.Owner, a.Schema, a.Class}
		if !roleSet[a.Owner] || defaultSet[key] || (a.State != "null" && a.State != "empty" && a.State != "explicit") {
			return errors.New("invalid or duplicate profile default ACL state")
		}
		defaultSet[key] = true
	}
	coverage := map[AuthorityACLCatalogColumn]bool{}
	supportedCatalogs := map[AuthorityACLCatalogColumn]bool{{"pg_catalog.pg_attribute", "attacl"}: true, {"pg_catalog.pg_class", "relacl"}: true, {"pg_catalog.pg_database", "datacl"}: true, {"pg_catalog.pg_default_acl", "defaclacl"}: true, {"pg_catalog.pg_namespace", "nspacl"}: true, {"pg_catalog.pg_proc", "proacl"}: true}
	for _, c := range p.Coverage {
		k := AuthorityACLCatalogColumn{c.Catalog, c.Column}
		if !strings.HasPrefix(c.Catalog, "pg_catalog.") || c.Column == "" || strings.TrimSpace(c.Reason) == "" || coverage[k] || (c.Disposition != "scoped" && c.Disposition != "excluded") {
			return errors.New("invalid or duplicate catalog coverage disposition")
		}
		if (c.Disposition == "scoped") != supportedCatalogs[k] {
			return errors.New("coverage cannot claim an unimplemented ACL reader or exclude a required class")
		}
		if (c.Disposition == "excluded" && !validHex(c.BaselineSHA256, 64)) || (c.Disposition == "scoped" && c.BaselineSHA256 != "") {
			return errors.New("catalog exclusion requires an explicit preserved baseline")
		}
		coverage[k] = true
	}
	return nil
}

func captureOperationalAuthority(ctx context.Context, tx pgx.Tx, p SingleOwnerAuthorityProfile, requireReadOnly bool) (SingleOwnerAuthorityObservation, error) {
	zero := SingleOwnerAuthorityObservation{}
	if err := validateOperationalAuthorityProfile(p); err != nil {
		return zero, err
	}
	roles := []string{}
	refs := []AuthorityObjectRef{}
	for _, r := range p.Expected.Roles {
		roles = append(roles, r.Name)
	}
	for _, o := range p.Expected.Objects {
		refs = append(refs, AuthorityObjectRef{o.Class, o.Identity})
	}
	identity, err := captureAuthorityIdentityTransaction(ctx, tx, AuthorityIdentityScope{Roles: roles, Objects: refs}, requireReadOnly)
	if err != nil {
		return zero, err
	}
	acl, err := captureScopedAuthorityACLTransaction(ctx, tx, refs, requireReadOnly)
	if err != nil {
		return zero, err
	}
	defaults, err := captureAuthorityDefaultACLTransaction(ctx, tx, roles, requireReadOnly)
	if err != nil {
		return zero, err
	}
	probes, err := captureAuthorityPrivilegeTransaction(ctx, tx, p.Expected.EffectivePrivileges, requireReadOnly)
	if err != nil {
		return zero, err
	}
	catalog, err := captureAuthorityACLCatalogTransaction(ctx, tx, requireReadOnly)
	if err != nil {
		return zero, err
	}
	excluded, err := captureExcludedACLBaselines(ctx, tx, p.Coverage)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return SingleOwnerAuthorityObservation{Snapshot: SingleOwnerAuthoritySnapshot{Roles: identity.Roles, Objects: identity.Objects, Memberships: identity.Memberships, ACLs: acl.ACLs, DefaultACLs: defaults.ACLs, EffectivePrivileges: probes}, ACLStates: acl.ACLStates, DefaultStates: defaults.States, Catalog: catalog, ExcludedACLs: excluded}, nil
}

func CompareSingleOwnerAuthorityProfile(p SingleOwnerAuthorityProfile, actual SingleOwnerAuthorityObservation) error {
	if err := validateOperationalAuthorityProfile(p); err != nil {
		return err
	}
	if err := CompareSingleOwnerAuthorityContract(&p.Expected, &actual.Snapshot); err != nil {
		return err
	}
	if actual.ACLStates == nil || actual.DefaultStates == nil || actual.Catalog == nil || actual.ExcludedACLs == nil {
		return errors.New("authority observation omits catalog/state inventories")
	}
	if err := compareAuthoritySet("raw ACL states", p.ACLStates, actual.ACLStates); err != nil {
		return err
	}
	if err := compareAuthoritySet("raw default ACL states", p.DefaultStates, actual.DefaultStates); err != nil {
		return err
	}
	expectedCatalog := make([]AuthorityACLCatalogColumn, 0, len(p.Coverage))
	expectedExcluded := []AuthorityExcludedACLBaseline{}
	for _, c := range p.Coverage {
		expectedCatalog = append(expectedCatalog, AuthorityACLCatalogColumn{c.Catalog, c.Column})
		if c.Disposition == "excluded" {
			expectedExcluded = append(expectedExcluded, AuthorityExcludedACLBaseline{c.Catalog, c.Column, c.BaselineSHA256})
		}
	}
	if err := compareAuthoritySet("ACL catalog coverage", expectedCatalog, actual.Catalog); err != nil {
		return fmt.Errorf("unaccounted ACL-bearing catalog column: %w", err)
	}
	if err := compareAuthoritySet("preserved excluded ACLs", expectedExcluded, actual.ExcludedACLs); err != nil {
		return err
	}
	return nil
}
