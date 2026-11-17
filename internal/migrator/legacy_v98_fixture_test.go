package migrator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// discoverLegacyV98Migrations keeps historical fixtures pinned to V1..V98
// while the canonical migration inventory continues to grow.
func discoverLegacyV98Migrations(dir string) ([]Migration, error) {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return nil, err
	}
	if len(migrations) < 98 {
		return nil, fmt.Errorf("legacy V1..V98 fixture requires at least 98 migrations, got %d", len(migrations))
	}
	for i := 0; i < 98; i++ {
		if migrations[i].Version != int64(i+1) {
			return nil, fmt.Errorf("legacy V1..V98 fixture is not contiguous at index %d: version %d", i, migrations[i].Version)
		}
	}
	return migrations[:98], nil
}

func TestDiscoverLegacyV98Migrations(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 99; i++ {
		name := fmt.Sprintf("V%06d__fixture.sql", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	migrations, err := discoverLegacyV98Migrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 98 || migrations[0].Version != 1 || migrations[97].Version != 98 {
		t.Fatalf("legacy fixture inventory boundaries: len=%d first=%d last=%d", len(migrations), migrations[0].Version, migrations[len(migrations)-1].Version)
	}
}

func TestDiscoverLegacyV98MigrationsRejectsShortOrNoncontiguousInventory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		versions []int
	}{
		{name: "short", versions: makeVersions(97)},
		{name: "gap", versions: append(makeVersions(98)[:97], 99)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, version := range tc.versions {
				name := fmt.Sprintf("V%06d__fixture.sql", version)
				if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := discoverLegacyV98Migrations(dir); err == nil {
				t.Fatal("expected invalid legacy inventory to be rejected")
			}
		})
	}
}

func makeVersions(n int) []int {
	versions := make([]int, n)
	for i := range versions {
		versions[i] = i + 1
	}
	return versions
}
