package migrator

import (
	"reflect"
	"testing"
)

func operationalProfileFixture() SingleOwnerAuthorityProfile {
	return SingleOwnerAuthorityProfile{
		Expected: SingleOwnerAuthoritySnapshot{
			Roles: []AuthorityRole{{Name: "actor"}}, Objects: []AuthorityObject{{Class: "schema", Identity: "public", Owner: "owner"}}, Memberships: []AuthorityMembership{}, ACLs: []AuthorityACL{}, DefaultACLs: []AuthorityDefaultACL{}, EffectivePrivileges: []AuthorityPrivilegeProbe{{Role: "actor", ObjectClass: "schema", ObjectIdentity: "public", Privilege: "USAGE"}, {Role: "actor", ObjectClass: "schema", ObjectIdentity: "public", Privilege: "CREATE"}}},
		ACLStates: []AuthorityACLState{{Class: "schema", ObjectIdentity: "public", State: "null"}}, DefaultStates: []AuthorityDefaultACLState{}, Coverage: []AuthorityCatalogCoverage{{Catalog: "pg_catalog.pg_namespace", Column: "nspacl", Disposition: "scoped", Reason: "explicit schema ACL capture"}},
	}
}

func TestOperationalAuthorityProfileComparesRawStateAndCoverage(t *testing.T) {
	p := operationalProfileFixture()
	actual := SingleOwnerAuthorityObservation{Snapshot: p.Expected, ACLStates: append([]AuthorityACLState(nil), p.ACLStates...), DefaultStates: []AuthorityDefaultACLState{}, Catalog: []AuthorityACLCatalogColumn{{Catalog: "pg_catalog.pg_namespace", Column: "nspacl"}}, ExcludedACLs: []AuthorityExcludedACLBaseline{}}
	if err := CompareSingleOwnerAuthorityProfile(p, actual); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"raw-state", "extra-catalog", "missing-catalog", "missing-states", "effective-drift"} {
		t.Run(kind, func(t *testing.T) {
			got := actual
			got.ACLStates = append([]AuthorityACLState(nil), actual.ACLStates...)
			got.Catalog = append([]AuthorityACLCatalogColumn(nil), actual.Catalog...)
			got.Snapshot.EffectivePrivileges = append([]AuthorityPrivilegeProbe(nil), actual.Snapshot.EffectivePrivileges...)
			switch kind {
			case "raw-state":
				got.ACLStates[0].State = "empty"
			case "extra-catalog":
				got.Catalog = append(got.Catalog, AuthorityACLCatalogColumn{"pg_catalog.pg_type", "typacl"})
			case "missing-catalog":
				got.Catalog = []AuthorityACLCatalogColumn{}
			case "missing-states":
				got.ACLStates = nil
			case "effective-drift":
				got.Snapshot.EffectivePrivileges[0].Granted = true
			}
			if err := CompareSingleOwnerAuthorityProfile(p, got); err == nil {
				t.Fatal("authority drift accepted")
			}
		})
	}
	if !reflect.DeepEqual(p, operationalProfileFixture()) {
		t.Fatal("comparison mutated reviewed profile")
	}
}

func TestOperationalAuthorityProfileRejectsUnreviewedScope(t *testing.T) {
	for _, kind := range []string{"elevated-role", "missing-object-state", "duplicate-state", "missing-reason", "duplicate-coverage", "foreign-probe", "foreign-default-owner"} {
		t.Run(kind, func(t *testing.T) {
			p := operationalProfileFixture()
			switch kind {
			case "elevated-role":
				p.Expected.Roles[0].CreateRole = true
			case "missing-object-state":
				p.ACLStates = []AuthorityACLState{}
			case "duplicate-state":
				p.ACLStates = append(p.ACLStates, p.ACLStates[0])
			case "missing-reason":
				p.Coverage[0].Reason = ""
			case "duplicate-coverage":
				p.Coverage = append(p.Coverage, p.Coverage[0])
			case "foreign-probe":
				p.Expected.EffectivePrivileges[0].Role = "foreign"
			case "foreign-default-owner":
				p.Expected.DefaultACLs = append(p.Expected.DefaultACLs, AuthorityDefaultACL{Owner: "foreign", Grantor: "foreign", Class: "relation", Grantee: "PUBLIC", Privilege: "SELECT"})
			}
			if err := validateOperationalAuthorityProfile(p); err == nil {
				t.Fatal("incomplete profile accepted")
			}
		})
	}
}
