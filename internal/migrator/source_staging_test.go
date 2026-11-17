package migrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func stagingFixture(t *testing.T) (string, SourceManifest, []byte) {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "Test")
	rel := "internal/migrator/testdata/V000001__fixture.sql"
	data := []byte("CREATE TABLE staging_fixture(id integer);\n")
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	git("add", rel)
	git("commit", "-qm", "fixture")
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	migrations := []Migration{{Version: 1, Name: filepath.Base(rel), Checksum: checksum}}
	return root, SourceManifest{Commit: stringTrim(out), InventorySHA256: inventoryFingerprint(migrations), Entries: []SourceManifestEntry{{Path: rel, Name: filepath.Base(rel), Version: 1, Checksum: checksum}}}, data
}
func stringTrim(b []byte) string { return string(b[:len(b)-1]) }

func TestStageVerifiedMigrationSource(t *testing.T) {
	root, manifest, want := stagingFixture(t)
	dest := filepath.Join(t.TempDir(), "stage")
	got, err := StageVerifiedMigrationSource(context.Background(), root, manifest, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != filepath.Join(dest, manifest.Entries[0].Name) || string(got[0].SQL) != string(want) || got[0].Checksum != manifest.Entries[0].Checksum {
		t.Fatalf("unexpected staged migration: %#v", got)
	}
	fromDisk, readErr := DiscoverMigrations(dest)
	if readErr != nil || len(fromDisk) != len(got) || string(fromDisk[0].SQL) != string(want) || inventoryFingerprint(fromDisk) != manifest.InventorySHA256 {
		t.Fatalf("staged disk inventory differs from reviewed bytes: %+v %v", fromDisk, readErr)
	}
	info, err := os.Stat(got[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dinfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dinfo.Mode().Perm() != 0700 {
		t.Fatalf("directory mode = %o", dinfo.Mode().Perm())
	}
}
func TestStageInvalidManifestProducesZeroFSSideEffects(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	manifest.Entries[0].Checksum = "0000000000000000000000000000000000000000000000000000000000000000"
	dest := filepath.Join(t.TempDir(), "not-created")
	if _, err := StageVerifiedMigrationSource(context.Background(), root, manifest, dest); err == nil {
		t.Fatal("expected invalid manifest error")
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination side effect: %v", err)
	}
}
func TestStageExistingDestinationSymlinkAndCanceledContext(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	parent := t.TempDir()
	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(existing, "keep")
	if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := StageVerifiedMigrationSource(context.Background(), root, manifest, existing); err == nil {
		t.Fatal("expected existing destination rejection")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "safe" {
		t.Fatalf("existing sentinel changed: %q %v", b, err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(existing, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := StageVerifiedMigrationSource(context.Background(), root, manifest, filepath.Join(link, "child")); err == nil {
		t.Fatal("expected symlink ancestor rejection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StageVerifiedMigrationSource(ctx, root, manifest, filepath.Join(parent, "canceled")); err == nil {
		t.Fatal("expected pre-canceled context")
	}
}

type stagedCancelContext struct {
	context.Context
	mu          sync.Mutex
	destination string
	canceled    chan struct{}
	didCancel   bool
	onCancel    func()
}

func (c *stagedCancelContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.didCancel {
		return context.Canceled
	}
	if _, err := os.Lstat(filepath.Join(c.destination, "V000001__fixture.sql")); err == nil {
		if c.onCancel != nil {
			c.onCancel()
			c.onCancel = nil
		}
		c.didCancel = true
		close(c.canceled)
		return context.Canceled
	}
	return nil
}
func (c *stagedCancelContext) Done() <-chan struct{} { return c.canceled }
func TestStageCancellationRollsBackOwnedDirectory(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "stage")
	ctx := &stagedCancelContext{Context: context.Background(), destination: dest, canceled: make(chan struct{})}
	if got, err := StageVerifiedMigrationSource(ctx, root, manifest, dest); err == nil || got != nil {
		t.Fatalf("expected nil result/error, got %#v, %v", got, err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("owned destination not rolled back: %v", err)
	}
}
func TestStageCancellationPreservesReplacementDirectory(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "stage")
	moved := filepath.Join(parent, "moved")
	sentinel := []byte("replacement sentinel")
	ctx := &stagedCancelContext{Context: context.Background(), destination: dest, canceled: make(chan struct{})}
	ctx.onCancel = func() {
		if err := os.Rename(dest, moved); err != nil {
			panic(err)
		}
		if err := os.Mkdir(dest, 0700); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "sentinel"), sentinel, 0600); err != nil {
			panic(err)
		}
	}
	got, err := StageVerifiedMigrationSource(ctx, root, manifest, dest)
	if err == nil || got != nil {
		t.Fatalf("expected cancellation/error, got %#v, %v", got, err)
	}
	b, readErr := os.ReadFile(filepath.Join(dest, "sentinel"))
	if readErr != nil || string(b) != string(sentinel) {
		t.Fatalf("replacement damaged: %q, %v", b, readErr)
	}
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "cleanup staging destination refused") {
		t.Fatalf("missing cancellation or cleanup identity cause: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(moved, manifest.Entries[0].Name)); statErr != nil {
		t.Fatalf("original staged directory was not preserved for diagnosis: %v", statErr)
	}
}

type stagedRenameContext struct {
	context.Context
	once               sync.Once
	destination, moved string
}

func (c *stagedRenameContext) Err() error {
	if info, err := os.Lstat(c.destination); err == nil && info.IsDir() {
		c.once.Do(func() {
			if err := os.Rename(c.destination, c.moved); err != nil {
				panic(err)
			}
			if err := os.Mkdir(c.destination, 0700); err != nil {
				panic(err)
			}
			if err := os.WriteFile(filepath.Join(c.destination, "sentinel"), []byte("keep"), 0600); err != nil {
				panic(err)
			}
		})
	}
	return nil
}
func TestStageCancellationPreservesUnownedFileInsideStaging(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	dest := filepath.Join(t.TempDir(), "stage")
	ctx := &stagedCancelContext{Context: context.Background(), destination: dest, canceled: make(chan struct{})}
	ctx.onCancel = func() {
		if err := os.WriteFile(filepath.Join(dest, "external-sentinel"), []byte("keep"), 0600); err != nil {
			panic(err)
		}
	}
	if got, err := StageVerifiedMigrationSource(ctx, root, manifest, dest); err == nil || got != nil {
		t.Fatalf("expected cancellation: %+v %v", got, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "external-sentinel")); err != nil || string(b) != "keep" {
		t.Fatalf("unowned staging file removed: %q %v", b, err)
	}
}

func TestStageCleanupBeforeOpenRootDoesNotDereferenceNilRoot(t *testing.T) {
	for _, scenario := range []string{"empty-owned", "unknown-file", "replaced-directory"} {
		t.Run(scenario, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "stage")
			if err := os.Mkdir(dest, 0700); err != nil {
				t.Fatal(err)
			}
			owned, err := os.Lstat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "replaced-directory" {
				if err := os.Rename(dest, filepath.Join(parent, "moved")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "empty-owned" {
				if err := os.WriteFile(filepath.Join(dest, "sentinel"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Exact arguments used when os.OpenRoot fails: no file has been created.
			err = removeOwnedDirectory(dest, owned, nil, nil)
			if scenario == "empty-owned" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(dest); !os.IsNotExist(err) {
					t.Fatalf("owned empty directory retained: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected cleanup refusal")
				}
				if b, err := os.ReadFile(filepath.Join(dest, "sentinel")); err != nil || string(b) != "keep" {
					t.Fatalf("early cleanup damaged sentinel: %q %v", b, err)
				}
			}
		})
	}
}

func TestStageAnchorsWritesBeforeDestinationReplacement(t *testing.T) {
	root, manifest, _ := stagingFixture(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "stage")
	moved := filepath.Join(parent, "moved")
	ctx := &stagedRenameContext{Context: context.Background(), destination: dest, moved: moved}
	got, err := StageVerifiedMigrationSource(ctx, root, manifest, dest)
	if err == nil || got != nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replaced destination accepted: %+v %v", got, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "sentinel")); err != nil || string(b) != "keep" {
		t.Fatalf("replacement changed: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dest, manifest.Entries[0].Name)); !os.IsNotExist(err) {
		t.Fatalf("SQL written into replacement: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, manifest.Entries[0].Name)); err != nil {
		t.Fatalf("original root did not receive anchored SQL: %v", err)
	}
}
