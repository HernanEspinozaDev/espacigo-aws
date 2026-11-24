package dbbootstrap

import (
	"strings"
	"testing"
)

func TestCombinedHistoryACLParsingRegressions(t *testing.T) {
	t.Run("unconditional revoke and conditional grant", func(t *testing.T) {
		source := []byte(`REVOKE ALL ON TABLE public.identity_declaration_revision FROM espacigo_runtime;
DO $$
BEGIN
	IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
		GRANT SELECT, INSERT ON TABLE public.identity_declaration_revision TO espacigo_runtime;
	END IF;
END $$;`)
		effects, err := ExtractMigrationRuntimeACL(85, source)
		if err != nil {
			t.Fatal(err)
		}
		var unconditionalRevoke, conditionalGrant bool
		for _, effect := range effects {
			if effect.Action == "REVOKE" && effect.Condition == "" {
				unconditionalRevoke = true
			}
			if effect.Action == "GRANT" && effect.Condition == "role-exists:espacigo_runtime" {
				conditionalGrant = true
			}
		}
		if !unconditionalRevoke || !conditionalGrant {
			t.Fatalf("lost action-specific condition: revoke=%v grant=%v effects=%+v", unconditionalRevoke, conditionalGrant, effects)
		}
	})
	t.Run("procedural role mention without ACL", func(t *testing.T) {
		source := []byte(`DO $$
BEGIN
	PERFORM 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime';
END $$;`)
		if _, err := ExtractMigrationRuntimeACL(95, source); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("supported text function signatures", func(t *testing.T) {
		for _, source := range []string{
			"GRANT EXECUTE ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) TO espacigo_runtime;",
			"REVOKE ALL ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) FROM PUBLIC;",
		} {
			grantee := "espacigo_runtime"
			if strings.Contains(source, " FROM PUBLIC") {
				grantee = "PUBLIC"
			}
			effects, err := ExtractMigrationACLForGrantee(90, []byte(source), grantee)
			if err != nil {
				t.Fatal(err)
			}
			if len(effects) != 1 || effects[0].Object != "public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz)" {
				t.Fatalf("unexpected parsed function ACL: %+v", effects)
			}
		}
	})
}
