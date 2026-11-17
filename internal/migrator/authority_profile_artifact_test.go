package migrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationalReviewedAuthorityProfileArtifact(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "planning", "evidence", "e1_authority_profile_20261117.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct{ Before, After SingleOwnerAuthorityProfile }
	if err := json.Unmarshal(b, &artifact); err != nil {
		t.Fatal(err)
	}
	for _, p := range []SingleOwnerAuthorityProfile{artifact.Before, artifact.After} {
		if err := validateOperationalAuthorityProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := compareAuthoritySet("preserved role attributes", artifact.Before.Expected.Roles, artifact.After.Expected.Roles); err != nil {
		t.Fatal(err)
	}
	if err := compareAuthoritySet("preserved memberships", artifact.Before.Expected.Memberships, artifact.After.Expected.Memberships); err != nil {
		t.Fatal(err)
	}
	if err := compareAuthoritySet("preserved default ACLs", artifact.Before.Expected.DefaultACLs, artifact.After.Expected.DefaultACLs); err != nil {
		t.Fatal(err)
	}
	if err := compareAuthoritySet("preserved catalog baselines", artifact.Before.Coverage, artifact.After.Coverage); err != nil {
		t.Fatal(err)
	}
	actualLegacy := []AuthorityACL{}
	newACL := []AuthorityACL{}
	for _, a := range artifact.After.Expected.ACLs {
		if strings.HasPrefix(a.ObjectIdentity, "public.arriendo_temprano") {
			newACL = append(newACL, a)
		} else {
			actualLegacy = append(actualLegacy, a)
		}
	}
	if err := compareAuthoritySet("legacy ACL unchanged", artifact.Before.Expected.ACLs, actualLegacy); err != nil {
		t.Fatal(err)
	}
	for _, a := range newACL {
		if a.Column != "" || a.GrantOption || a.Grantor != "espacigo_schema_owner" {
			t.Fatalf("unsafe new ACL: %+v", a)
		}
		if a.Grantee == "espacigo_schema_owner" {
			continue
		}
		if a.Grantee != "espacigo_runtime" || a.Class != "relation" {
			t.Fatalf("unexpected new grant: %+v", a)
		}
		if a.Privilege != "SELECT" && a.Privilege != "INSERT" && !(a.Privilege == "UPDATE" && (a.ObjectIdentity == "public.arriendo_temprano" || a.ObjectIdentity == "public.arriendo_temprano_pago")) {
			t.Fatalf("excessive new runtime grant: %+v", a)
		}
	}
	if len(artifact.After.Expected.Objects)-len(artifact.Before.Expected.Objects) != 12 {
		t.Fatal("expected exactly five new relations and seven trigger functions")
	}
	for _, probe := range artifact.After.Expected.EffectivePrivileges {
		if strings.HasPrefix(probe.ObjectIdentity, "public.arriendo_temprano") && probe.Role != "espacigo_schema_owner" && probe.Role != "espacigo_runtime" && probe.Granted {
			t.Fatalf("new object accessible to an unrelated protected principal: %+v", probe)
		}
	}
}
