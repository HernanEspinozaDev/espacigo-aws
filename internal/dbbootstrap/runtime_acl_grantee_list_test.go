package dbbootstrap

import (
	"reflect"
	"strings"
	"testing"
)

func TestACLExplicitGranteeListsPreserveEachTarget(t *testing.T) {
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		for _, roles := range []string{"PUBLIC, espacigo_runtime", "espacigo_runtime, PUBLIC", "unrelated_role, PUBLIC, espacigo_runtime, another_role"} {
			t.Run(grantee+"/"+roles, func(t *testing.T) {
				source := `REVOKE ALL ON TABLE public.items, public.events FROM ` + roles + `;`
				want := []RuntimeACLEffect{
					{Action: "REVOKE", Grantee: grantee, ObjectKind: "TABLE", Object: "public.items", Privilege: "ALL PRIVILEGES"},
					{Action: "REVOKE", Grantee: grantee, ObjectKind: "TABLE", Object: "public.events", Privilege: "ALL PRIVILEGES"},
				}
				for _, procedural := range []bool{false, true} {
					input := source
					if procedural {
						input = "DO $$ BEGIN " + source + " END $$;"
					}
					got, err := ExtractMigrationACLForGrantee(99, []byte(input), grantee)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("procedural=%t effects=%+v want=%+v error=%v", procedural, got, want, err)
					}
				}
				got, err := ParseACLStatementsForGrantee(source, grantee)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("parser effects=%+v want=%+v error=%v", got, want, err)
				}
			})
		}
	}
}

func TestACLExplicitGranteeListsPreserveOptions(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   RuntimeACLEffect
	}{
		{`GRANT UPDATE (status) ON TABLE public.items TO PUBLIC, espacigo_runtime WITH GRANT OPTION;`, RuntimeACLEffect{Action: "GRANT", Grantee: "espacigo_runtime", ObjectKind: "TABLE", Object: "public.items", Privilege: "UPDATE", Column: "status", GrantOption: true}},
		{`REVOKE EXECUTE ON FUNCTION public.items(uuid, text) FROM PUBLIC, espacigo_runtime CASCADE;`, RuntimeACLEffect{Action: "REVOKE", Grantee: "espacigo_runtime", ObjectKind: "FUNCTION", Object: "public.items(uuid, text)", Privilege: "EXECUTE", RevokeBehavior: "CASCADE"}},
	} {
		got, err := ExtractMigrationACLForGrantee(99, []byte(tc.source), "espacigo_runtime")
		if err != nil || len(got) != 1 || got[0] != tc.want {
			t.Fatalf("effects=%+v want=%+v error=%v", got, tc.want, err)
		}
	}
}

func TestACLExplicitGranteeListsRejectUnsupportedRoles(t *testing.T) {
	for _, roles := range []string{
		`"quoted_role", espacigo_runtime`,
		`PUBLIC, "espacigo_runtime"`,
		`PUBLIC,, espacigo_runtime`,
		`PUBLIC, espacigo_runtime,`,
		`PUBLIC, espacigo_runtime, espacigo_runtime`,
		`CURRENT_USER, espacigo_runtime`,
		`PUBLIC, session_user, espacigo_runtime`,
	} {
		source := `GRANT SELECT ON TABLE public.items TO ` + roles + `;`
		if _, err := ParseACLStatementsForGrantee(source, "espacigo_runtime"); err == nil {
			t.Errorf("parser accepted unsupported roles %s", roles)
		}
		// The extractor must not silently drop a target after a quoted role.
		if strings.Contains(roles, `"`) {
			if _, err := ExtractMigrationACLForGrantee(99, []byte(source), "espacigo_runtime"); err == nil {
				t.Errorf("extractor accepted/ignored quoted role list %s", roles)
			}
		}
	}
}

func TestMigrationACLGuardBranchesInUnsupportedControlFlowFailClosed(t *testing.T) {
	for name, body := range map[string]string{
		"exception-handler": `BEGIN IF current_user <> 'owner' THEN RAISE EXCEPTION 'stop'; END IF; GRANT SELECT ON TABLE public.items TO espacigo_runtime; EXCEPTION WHEN OTHERS THEN NULL; END`,
		"loop":              `BEGIN WHILE false LOOP IF current_user <> 'owner' THEN RAISE EXCEPTION 'stop'; END IF; GRANT SELECT ON TABLE public.items TO espacigo_runtime; END LOOP; END`,
		"external-return":   `BEGIN IF current_user <> 'owner' THEN RAISE EXCEPTION 'stop'; END IF; RETURN; GRANT SELECT ON TABLE public.items TO espacigo_runtime; END`,
		"case":              `BEGIN CASE WHEN false THEN IF current_user <> 'owner' THEN RAISE EXCEPTION 'stop'; END IF; GRANT SELECT ON TABLE public.items TO espacigo_runtime; ELSE NULL; END CASE; END`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ExtractMigrationRuntimeACL(99, []byte("DO $$ "+body+" $$;")); err == nil {
				t.Fatal("normalizing assertion guards hid unsupported enclosing control flow")
			}
		})
	}
}

func TestMigrationACLGuardCannotSkipFollowingEffects(t *testing.T) {
	for _, body := range []string{
		`RETURN;`,
		`RAISE NOTICE 'skipped'; RETURN;`,
		`IF current_user = 'other_role' THEN RETURN; END IF;`,
		`RAISE EXCEPTION 'bad'; ELSE RETURN;`,
		`EXECUTE 'GRANT SELECT ON TABLE public.hidden_items TO espacigo_runtime';`,
	} {
		source := `DO $$ BEGIN IF current_user <> 'espacigo_schema_owner' THEN ` + body + ` END IF; GRANT SELECT ON TABLE public.items TO espacigo_runtime; END $$;`
		if _, err := ExtractMigrationRuntimeACL(99, []byte(source)); err == nil {
			t.Errorf("guard that can skip/perform ACL effects was ignored: %s", body)
		}
	}
}
