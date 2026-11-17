package migrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMigrationSourceManifestRejectsMissingInventory(t *testing.T) {
	root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
	_, err := VerifyMigrationSource(context.Background(), root, SourceManifest{Commit: commit, Entries: []SourceManifestEntry{entry}})
	if err == nil {
		t.Fatal("missing inventory fingerprint accepted")
	}
}
func TestMigrationSourceManifestAllowsCandidateNameMapping(t *testing.T) {
	root, commit, entry := sourceFixture(t, "internal/migrator/testdata/synthetic_candidate.sql", "V000001__synthetic_candidate.sql")
	manifest := validSourceManifest(commit, entry)
	got, err := VerifyMigrationSource(context.Background(), root, manifest)
	if err != nil || len(got) != 1 || got[0].Name != entry.Name {
		t.Fatalf("candidate mapping: %#v %v", got, err)
	}
}
func TestMigrationSourceManifestRejectsVersionGap(t *testing.T) {
	root, _, one := sourceFixture(t, "db/migrations/V000001__synthetic_one.sql", "V000001__synthetic_one.sql")
	two := writeSource(t, root, "db/migrations/V000003__synthetic_three.sql", "V000003__synthetic_three.sql")
	commit := commitSource(t, root)
	_, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, one, two))
	if err == nil {
		t.Fatal("version gap accepted")
	}
}
func TestMigrationSourceManifestRejectsOmittedCommittedMigration(t *testing.T) {
	root, _, one := sourceFixture(t, "db/migrations/V000001__synthetic_one.sql", "V000001__synthetic_one.sql")
	writeSource(t, root, "db/migrations/V000002__synthetic_two.sql", "V000002__synthetic_two.sql")
	commit := commitSource(t, root)
	_, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, one))
	if err == nil {
		t.Fatal("omitted committed migration accepted")
	}
}
func TestMigrationSourceManifestReturnsPinnedCapturedBytes(t *testing.T) {
	root, commit, e := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
	got, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, e))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got[0].SQL) != "SELECT 'synthetic';\n" || got[0].Checksum != e.Checksum {
		t.Fatalf("captured output: %#v", got)
	}
}
func TestMigrationSourceManifestRejectsSymlinkRepositoryRoot(t *testing.T) {
	root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMigrationSource(context.Background(), alias, validSourceManifest(commit, entry)); err == nil {
		t.Fatal("symlink repository root accepted")
	}
}

func TestMigrationSourceManifestRejectsExtraCheckoutWithoutCommittedMigrations(t *testing.T) {
	root, commit, entry := sourceFixture(t, "internal/migrator/testdata/synthetic_candidate.sql", "V000001__synthetic_candidate.sql")
	writeSource(t, root, "db/migrations/V000001__unexpected.sql", "V000001__unexpected.sql")
	if _, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, entry)); err == nil {
		t.Fatal("untracked applying migration accepted in candidate-only checkout")
	}
}

func TestMigrationSourceManifestRejectsCommittedSymlinkMode(t *testing.T) {
	root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
	blob := sourceGit(t, root, "rev-parse", commit+":"+entry.Path)
	sourceGit(t, root, "update-index", "--cacheinfo", "120000,"+blob+","+entry.Path)
	sourceGit(t, root, "commit", "-qm", "synthetic symlink mode")
	commit = sourceGit(t, root, "rev-parse", "HEAD")
	if _, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, entry)); err == nil {
		t.Fatal("committed symlink mode accepted as SQL file")
	}
}

func TestMigrationSourceManifestRejectsTamperingAndInvalidPins(t *testing.T) {
	cases := map[string]func(*testing.T, string, *SourceManifest){
		"unknown commit":   func(_ *testing.T, _ string, m *SourceManifest) { m.Commit = strings.Repeat("0", 40) },
		"symbolic commit":  func(_ *testing.T, _ string, m *SourceManifest) { m.Commit = "HEAD" },
		"uppercase commit": func(_ *testing.T, _ string, m *SourceManifest) { m.Commit = strings.Repeat("A", 40) },
		"wrong inventory":  func(_ *testing.T, _ string, m *SourceManifest) { m.InventorySHA256 = strings.Repeat("0", 64) },
		"wrong checksum": func(_ *testing.T, _ string, m *SourceManifest) {
			m.Entries[0].Checksum = strings.Repeat("0", 64)
			*m = validSourceManifest(m.Commit, m.Entries...)
		},
		"name version mismatch": func(_ *testing.T, _ string, m *SourceManifest) {
			m.Entries[0].Version = 2
			*m = validSourceManifest(m.Commit, m.Entries...)
		},
		"duplicate source": func(_ *testing.T, _ string, m *SourceManifest) {
			m.Entries = append(m.Entries, m.Entries[0])
			*m = validSourceManifest(m.Commit, m.Entries...)
		},
		"absolute path": func(_ *testing.T, root string, m *SourceManifest) {
			m.Entries[0].Path = filepath.Join(root, m.Entries[0].Path)
		},
		"traversal": func(_ *testing.T, _ string, m *SourceManifest) {
			m.Entries[0].Path = "db/migrations/../V000001__synthetic_probe.sql"
		},
		"private root": func(_ *testing.T, _ string, m *SourceManifest) { m.Entries[0].Path = ".local/secrets/never_read.sql" },
		"missing checkout": func(t *testing.T, root string, m *SourceManifest) {
			if err := os.Remove(filepath.Join(root, m.Entries[0].Path)); err != nil {
				t.Fatal(err)
			}
		},
		"changed bytes": func(t *testing.T, root string, m *SourceManifest) {
			if err := os.WriteFile(filepath.Join(root, m.Entries[0].Path), []byte("SELECT 'changed';"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"extra checkout migration": func(t *testing.T, root string, _ *SourceManifest) {
			writeSource(t, root, "db/migrations/V000002__unexpected.sql", "V000002__unexpected.sql")
		},
		"file symlink": func(t *testing.T, root string, m *SourceManifest) {
			p := filepath.Join(root, m.Entries[0].Path)
			target := filepath.Join(t.TempDir(), "synthetic.sql")
			if err := os.Rename(p, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
		},
		"directory symlink": func(t *testing.T, root string, _ *SourceManifest) {
			p := filepath.Join(root, "db", "migrations")
			target := filepath.Join(t.TempDir(), "migrations")
			if err := os.Rename(p, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
			m := validSourceManifest(commit, entry)
			mutate(t, root, &m)
			if got, err := VerifyMigrationSource(context.Background(), root, m); err == nil || got != nil {
				t.Fatalf("invalid source yielded partial/success output: %#v %v", got, err)
			}
		})
	}
}

func TestMigrationSourceManifestRejectsGitOverrides(t *testing.T) {
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"} {
		t.Run(key, func(t *testing.T) {
			root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
			t.Setenv(key, "synthetic-override")
			if _, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, entry)); err == nil || !strings.Contains(err.Error(), "unsafe Git environment") {
				t.Fatalf("Git override not rejected at environment guard: %v", err)
			}
		})
	}
}

func TestMigrationSourceManifestRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyMigrationSource(ctx, t.TempDir(), SourceManifest{}); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("canceled source verification accepted: %v", err)
	}
}

func TestMigrationSourceManifestRetainsCapturedBytes(t *testing.T) {
	root, commit, entry := sourceFixture(t, "db/migrations/V000001__synthetic_probe.sql", "V000001__synthetic_probe.sql")
	got, err := VerifyMigrationSource(context.Background(), root, validSourceManifest(commit, entry))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, entry.Path), []byte("SELECT 'later';"), 0600); err != nil {
		t.Fatal(err)
	}
	if string(got[0].SQL) != "SELECT 'synthetic';\n" || got[0].Checksum != entry.Checksum {
		t.Fatal("verified bytes changed after checkout mutation")
	}
}

func sourceGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = root
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func sourceFixture(t *testing.T, path, name string) (string, string, SourceManifestEntry) {
	t.Helper()
	root := t.TempDir()
	sourceGit(t, root, "init", "-q")
	sourceGit(t, root, "config", "user.email", "test@example.invalid")
	sourceGit(t, root, "config", "user.name", "Test")
	e := writeSource(t, root, path, name)
	return root, commitSource(t, root), e
}
func writeSource(t *testing.T, root, path, name string) SourceManifestEntry {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(path))
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	sql := []byte("SELECT 'synthetic';\n")
	if e := os.WriteFile(p, sql, 0644); e != nil {
		t.Fatal(e)
	}
	m := migrationName.FindStringSubmatch(name)
	v, _ := strconv.ParseInt(m[1], 10, 64)
	return SourceManifestEntry{Path: path, Name: name, Version: v, Checksum: sha256Hex(sql)}
}
func commitSource(t *testing.T, root string) string {
	t.Helper()
	sourceGit(t, root, "add", ".")
	sourceGit(t, root, "commit", "-qm", "fixture")
	return sourceGit(t, root, "rev-parse", "HEAD")
}
func validSourceManifest(commit string, entries ...SourceManifestEntry) SourceManifest {
	ms := make([]Migration, len(entries))
	for i, e := range entries {
		ms[i] = Migration{Version: e.Version, Name: e.Name, Checksum: e.Checksum}
	}
	return SourceManifest{Commit: commit, Entries: entries, InventorySHA256: inventoryFingerprint(ms)}
}
