package migrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestE1CanonicalMigrationIsManualAndMatchesReviewedSQL(t *testing.T) {
	all, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 99 || all[98].Version != 99 || all[98].Name != "V000099__early_rental_manual_contract.sql" {
		t.Fatal("E1 canonical inventory must be contiguous through V99")
	}
	candidate, err := os.ReadFile(filepath.Join("testdata", "early_rental_operational_candidate.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(candidate) != all[98].Checksum || all[98].Checksum != "ff466a678929b32d413128f1a885b9bfce3177c5176be1d7ab3ea2d2eab1fa29" {
		t.Fatal("canonical V99 differs from the SQL actually exercised")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "db", "release-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Automatic map[string]json.RawMessage `json:"automatic_migrations"`
		Reviewed  map[string]struct {
			Checksum  string `json:"checksum"`
			Automatic bool   `json:"automatic"`
			Strategy  string `json:"strategy"`
		} `json:"review_required_migrations"`
	}
	if err := json.Unmarshal(b, &policy); err != nil {
		t.Fatal(err)
	}
	reviewed, ok := policy.Reviewed[all[98].Name]
	if !ok || reviewed.Automatic || reviewed.Checksum != all[98].Checksum || reviewed.Strategy != "security-tightening" {
		t.Fatal("V99 requires exact reviewed manual registration")
	}
	if _, ok := policy.Automatic[all[98].Name]; ok {
		t.Fatal("E1 must never become an automatic generic release")
	}
}
