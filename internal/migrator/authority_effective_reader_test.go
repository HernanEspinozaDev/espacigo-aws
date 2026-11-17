package migrator

import (
	"context"
	"testing"
)

func TestCaptureAuthorityPrivilegeProbesRejectsInvalidInputBeforeSQL(t *testing.T) {
	for name, probes := range map[string][]AuthorityPrivilegeProbe{
		"empty":           nil,
		"duplicate":       {{Role: "r", ObjectClass: "schema", ObjectIdentity: "public", Privilege: "USAGE"}, {Role: "r", ObjectClass: "schema", ObjectIdentity: "public", Privilege: "USAGE"}},
		"comma privilege": {{Role: "r", ObjectClass: "relation", ObjectIdentity: "public.t", Privilege: "SELECT,UPDATE"}},
		"unknown class":   {{Role: "r", ObjectClass: "nonsense", ObjectIdentity: "x", Privilege: "SELECT"}},
		"nul":             {{Role: "r\x00x", ObjectClass: "schema", ObjectIdentity: "public", Privilege: "USAGE"}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := CaptureAuthorityPrivilegeProbes(context.Background(), nil, probes)
			if err == nil || got != nil {
				t.Fatalf("got=%v err=%v", got, err)
			}
		})
	}
}

func TestCaptureAuthorityACLCatalogRequiresTransaction(t *testing.T) {
	got, err := CaptureAuthorityACLCatalog(context.Background(), nil)
	if err == nil || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
