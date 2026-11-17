package dbbootstrap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMigrationACLRoleRestoreDoesNotHideStaticEffects(t *testing.T) {
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		t.Run(grantee, func(t *testing.T) {
			source := `DO $operation$ DECLARE original_role text := current_user; BEGIN
EXECUTE pg_catalog.format('SET LOCAL ROLE %I', original_role);
GRANT SELECT ON TABLE public.items TO ` + grantee + `;
END $operation$;`
			got, err := ExtractMigrationACLForGrantee(99, []byte(source), grantee)
			if err != nil {
				t.Fatal(err)
			}
			want := []RuntimeACLEffect{{Action: "GRANT", Grantee: grantee, ObjectKind: "TABLE", Object: "public.items", Privilege: "SELECT"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("effects = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMigrationACLGuardOnlyBranchesDoNotConditionFollowingEffects(t *testing.T) {
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		t.Run(grantee, func(t *testing.T) {
			source := `DO $operation$ DECLARE original_role text := current_user; BEGIN
IF session_user <> current_user THEN
    IF current_user <> 'espacigo_schema_owner' THEN
        RAISE EXCEPTION 'unexpected role';
    END IF;
END IF;
GRANT SELECT ON TABLE public.items TO ` + grantee + `;
IF current_user <> original_role THEN
    EXECUTE pg_catalog.format('SET LOCAL ROLE %I', original_role);
END IF;
END $operation$;`
			got, err := ExtractMigrationACLForGrantee(99, []byte(source), grantee)
			if err != nil {
				t.Fatal(err)
			}
			want := []RuntimeACLEffect{{Action: "GRANT", Grantee: grantee, ObjectKind: "TABLE", Object: "public.items", Privilege: "SELECT"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("guard changed ACL conditions: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestMigrationACLRoleRestoreStillRejectsUnknownDynamicSQL(t *testing.T) {
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		for name, expression := range map[string]string{
			"dynamic-template":     `pg_catalog.format('%s', acl_sql)`,
			"dynamic-grantee":      `pg_catalog.format('GRANT SELECT ON TABLE public.items TO %I', target_role)`,
			"concatenated-role":    `pg_catalog.format('SET LOCAL ROLE %I', original_role || acl_sql)`,
			"concatenated-command": `pg_catalog.format('SET LOCAL ROLE %I', original_role) || acl_sql`,
			"literal-decoy":        `acl_sql`,
		} {
			t.Run(grantee+"/"+name, func(t *testing.T) {
				source := `DO $operation$ DECLARE
acl_sql text := 'GRANT SELECT ON TABLE public.items TO ` + grantee + `';
original_role text := current_user;
target_role text := '` + grantee + `';
BEGIN
PERFORM 'EXECUTE pg_catalog.format(''SET LOCAL ROLE %I'', original_role);';
EXECUTE ` + expression + `;
GRANT SELECT ON TABLE public.other_items TO ` + grantee + `;
END $operation$;`
				if _, err := ExtractMigrationACLForGrantee(99, []byte(source), grantee); err == nil || !strings.Contains(err.Error(), "unsupported dynamic ACL") {
					t.Fatalf("unknown dynamic SQL was not rejected: %v", err)
				}
			})
		}
	}
}

func TestMigrationACLGuardRemovalDoesNotHideConditionalEffects(t *testing.T) {
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		for _, action := range []string{
			`GRANT SELECT ON TABLE public.items TO ` + grantee,
			`EXECUTE 'GRANT SELECT ON TABLE public.items TO ` + grantee + `'`,
		} {
			source := `DO $operation$ BEGIN
IF current_user <> 'espacigo_schema_owner' THEN RAISE EXCEPTION 'bad actor'; END IF;
IF current_setting('application_name') = 'custom' THEN
` + action + `;
END IF;
END $operation$;`
			if _, err := ExtractMigrationACLForGrantee(99, []byte(source), grantee); err == nil || !strings.Contains(err.Error(), "condition") {
				t.Fatalf("unknown ACL branch must fail closed: grantee=%s action=%s error=%v", grantee, action, err)
			}
		}
	}
}

func TestCheckedInV99ACLMatchesStaticSource(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "V000099__early_rental_manual_contract.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, grantee := range []string{"espacigo_runtime", "PUBLIC"} {
		got, err := ExtractMigrationACLForGrantee(99, source, grantee)
		if err != nil {
			t.Fatal(err)
		}
		var want []RuntimeACLEffect
		addTables := func(action string, names []string, privileges []string) {
			for _, name := range names {
				for _, privilege := range privileges {
					want = append(want, RuntimeACLEffect{Action: action, Grantee: grantee, ObjectKind: "TABLE", Object: "public." + name, Privilege: privilege})
				}
			}
		}
		tables := []string{"arriendo_temprano", "arriendo_temprano_confirmacion", "arriendo_temprano_pago", "arriendo_temprano_evento", "arriendo_temprano_idempotencia"}
		if grantee == "PUBLIC" {
			addTables("REVOKE", tables[1:], []string{"ALL PRIVILEGES"})
		} else {
			// The immutable SQL grants once during creation, then revokes/regrants
			// explicitly to override broad default ACLs. Preserve both occurrences.
			for repeat := 0; repeat < 2; repeat++ {
				for _, name := range tables {
					privileges := []string{"SELECT", "INSERT"}
					if name == "arriendo_temprano" || name == "arriendo_temprano_pago" {
						privileges = append(privileges, "UPDATE")
					}
					addTables("GRANT", []string{name}, privileges)
				}
			}
		}
		addTables("REVOKE", tables, []string{"ALL PRIVILEGES"})
		repeats := 1
		if grantee == "PUBLIC" {
			repeats = 2
		}
		for repeat := 0; repeat < repeats; repeat++ {
			for _, name := range []string{"immutable_update", "occupancy_consistency", "confirmacion_guard", "pago_guard", "evento_guard", "evento_consistency", "idempotencia_guard"} {
				want = append(want, RuntimeACLEffect{Action: "REVOKE", Grantee: grantee, ObjectKind: "FUNCTION", Object: "public.arriendo_temprano_" + name + "()", Privilege: "ALL PRIVILEGES"})
			}
		}
		counts := func(effects []RuntimeACLEffect) map[RuntimeACLEffect]int {
			out := make(map[RuntimeACLEffect]int)
			for _, effect := range effects {
				out[effect]++
			}
			return out
		}
		if !reflect.DeepEqual(counts(got), counts(want)) {
			t.Errorf("%s effects = %+v, want %+v", grantee, counts(got), counts(want))
		}
	}
}
