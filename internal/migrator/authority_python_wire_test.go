package migrator

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
)

// This exercises real Go JSON serialization and contract validation with synthetic authority.
func TestMigrationAuthorityGoJSONPythonContract(t *testing.T) {
	all, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	validatePlanWire := func(raw []byte, version int) error {
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		planObj, ok := decoded["plan"].(map[string]any)
		if !ok {
			return errInvalidPlanContract
		}
		if version == 84 {
			if _, ok := planObj["ddl_authority"].(map[string]any); !ok {
				return errInvalidPlanContract
			}
		} else {
			if _, ok := planObj["identity_ddl_authority"].(map[string]any); !ok {
				return errInvalidPlanContract
			}
		}
		return nil
	}
	for _, version := range []int{84, 95, 96} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			plan := PlanResult{Database: "espacigo_dms_stage_20261020", SessionUser: "espacigo_migrator_login", CurrentUser: "espacigo_schema_owner", ServerVersion: "18.7", ReadOnly: true, HistoryExists: true}
			var rows []MigrationSummary
			for i, migration := range all[:version] {
				row := MigrationSummary{Version: migration.Version, Name: migration.Name, Checksum: migration.Checksum}
				rows = append(rows, row)
				if i < version-1 {
					plan.Applied = append(plan.Applied, HistoryEntry{Version: row.Version, Name: row.Name, Checksum: row.Checksum})
				} else {
					plan.Pending = append(plan.Pending, row)
				}
			}
			if version == 84 {
				authority := validV84DDLAuthority()
				plan.DDLAuthority = &authority
			} else {
				authority := validIdentityTailDDLAuthority()
				plan.IdentityDDLAuthority = &authority
			}
			run := func(plan PlanResult) ([]byte, error) {
				payload, err := json.Marshal(struct {
					Plan       PlanResult         `json:"plan"`
					Migrations []MigrationSummary `json:"migrations"`
				}{plan, rows})
				if err != nil {
					t.Fatal(err)
				}
				if err := validatePlanWire(payload, version); err != nil {
					return []byte(err.Error()), err
				}
				return payload, nil
			}
			if output, err := run(plan); err != nil {
				t.Fatalf("Go authority JSON rejected: %v %s", err, output)
			}
			plan.DDLAuthority, plan.IdentityDDLAuthority = nil, nil
			if output, err := run(plan); err == nil || !bytes.Contains(output, []byte("invalid migration plan contract")) {
				t.Fatalf("expected strict authority-omission rejection: err=%v output=%s", err, output)
			}
		})
	}
}

var errInvalidPlanContract = &planContractError{}

type planContractError struct{}

func (*planContractError) Error() string { return "invalid migration plan contract" }
