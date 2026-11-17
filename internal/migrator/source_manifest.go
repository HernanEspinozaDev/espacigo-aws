package migrator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SourceManifest pins an externally reviewed Git commit and its exact SQL inventory.
type SourceManifest struct {
	Commit          string
	InventorySHA256 string
	Entries         []SourceManifestEntry
}

// SourceManifestEntry binds one conventional migration filename to SQL bytes.
type SourceManifestEntry struct {
	Path     string
	Name     string
	Version  int64
	Checksum string
}

var fullGitCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func VerifyMigrationSource(ctx context.Context, repoDir string, manifest SourceManifest) ([]Migration, error) {
	fail := func(err error) ([]Migration, error) { return nil, err }
	if ctx == nil || ctx.Err() != nil {
		return fail(fmt.Errorf("source verification context canceled"))
	}
	if !fullGitCommit.MatchString(manifest.Commit) {
		return fail(fmt.Errorf("commit must be a full lowercase Git object ID"))
	}
	if len(manifest.Entries) == 0 {
		return fail(fmt.Errorf("source manifest is empty"))
	}
	if !validHex(manifest.InventorySHA256, 64) {
		return fail(fmt.Errorf("manifest inventory must be a lowercase SHA-256 fingerprint"))
	}
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"} {
		if os.Getenv(k) != "" {
			return fail(fmt.Errorf("unsafe Git environment %s", k))
		}
	}
	for _, k := range []string{"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"} {
		if os.Getenv(k) != "" {
			return fail(fmt.Errorf("unsafe Git environment %s", k))
		}
	}
	absoluteRoot, err := filepath.Abs(repoDir)
	if err != nil {
		return fail(fmt.Errorf("invalid source repository root: %w", err))
	}
	repoDir = absoluteRoot
	for ancestor := repoDir; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return fail(fmt.Errorf("repository root has an unavailable or symlink ancestor"))
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	git := func(args ...string) ([]byte, error) {
		c := exec.CommandContext(ctx, "git", append([]string{"--no-replace-objects", "-C", repoDir}, args...)...)
		c.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0"}
		return c.Output()
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSuffix(string(top), "\n") != repoDir {
		return fail(fmt.Errorf("source directory must be the exact Git working-tree root"))
	}
	typ, err := git("cat-file", "-t", manifest.Commit)
	if err != nil || string(typ) != "commit\n" {
		return fail(fmt.Errorf("pinned object is not an available commit"))
	}
	out := make([]Migration, 0, len(manifest.Entries))
	seen := map[string]bool{}
	for i, e := range manifest.Entries {
		if ctx.Err() != nil {
			return fail(fmt.Errorf("source verification context canceled"))
		}
		if e.Path == "" || strings.HasPrefix(e.Path, "/") || filepath.IsAbs(e.Path) || strings.Contains(e.Path, "\\") {
			return fail(fmt.Errorf("invalid source path %q", e.Path))
		}
		for _, part := range strings.Split(e.Path, "/") {
			if part == ".." || part == "." || part == ".git" || part == ".local" || part == "secrets" {
				return fail(fmt.Errorf("forbidden source path %q", e.Path))
			}
		}
		allowedRoot := strings.HasPrefix(e.Path, "db/migrations/") && strings.Count(e.Path, "/") == 2 || strings.HasPrefix(e.Path, "internal/migrator/testdata/") && strings.Count(e.Path, "/") == 3
		if !allowedRoot || !strings.HasSuffix(e.Path, ".sql") {
			return fail(fmt.Errorf("source path outside approved SQL roots"))
		}
		if seen[e.Path] {
			return fail(fmt.Errorf("duplicate source path"))
		}
		seen[e.Path] = true
		m := migrationName.FindStringSubmatch(e.Name)
		if m == nil || (strings.HasPrefix(e.Path, "db/migrations/") && e.Name != filepath.Base(e.Path)) {
			return fail(fmt.Errorf("invalid migration name"))
		}
		v, er := strconv.ParseInt(m[1], 10, 64)
		if er != nil || v != e.Version || v < 1 {
			return fail(fmt.Errorf("migration name/version mismatch"))
		}
		if e.Version != int64(i+1) {
			return fail(fmt.Errorf("manifest versions must be contiguous from one"))
		}
		path := filepath.Join(repoDir, filepath.FromSlash(e.Path))
		cur := repoDir
		for _, part := range strings.Split(e.Path, "/") {
			cur = filepath.Join(cur, part)
			fi, er := os.Lstat(cur)
			if er != nil {
				return fail(fmt.Errorf("checkout source unavailable: %w", er))
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return fail(fmt.Errorf("symlink in source path"))
			}
			if cur == path && !fi.Mode().IsRegular() {
				return fail(fmt.Errorf("source SQL is not a regular file"))
			}
		}
		mode, er := git("ls-tree", "-z", manifest.Commit, "--", e.Path)
		if er != nil || !(bytes.HasPrefix(mode, []byte("100644 blob ")) || bytes.HasPrefix(mode, []byte("100755 blob "))) {
			return fail(fmt.Errorf("committed SQL is not a regular file"))
		}
		current, er := os.ReadFile(path)
		if er != nil {
			return fail(er)
		}
		committed, er := git("cat-file", "blob", manifest.Commit+":"+e.Path)
		if er != nil {
			return fail(fmt.Errorf("committed source unavailable: %w", er))
		}
		if !bytes.Equal(current, committed) {
			return fail(fmt.Errorf("checkout differs from pinned commit"))
		}
		sum := sha256Hex(current)
		if len(e.Checksum) != 64 || sum != e.Checksum {
			return fail(fmt.Errorf("source checksum mismatch"))
		}
		out = append(out, Migration{Version: e.Version, Name: e.Name, Path: path, SQL: append([]byte(nil), current...), Checksum: sum})
	}
	if inventoryFingerprint(out) != manifest.InventorySHA256 {
		return fail(fmt.Errorf("source inventory mismatch"))
	}
	committedNames, err := git("ls-tree", "-r", "--name-only", manifest.Commit, "db/migrations")
	if err != nil {
		return fail(fmt.Errorf("list committed migrations: %w", err))
	}
	want := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSpace(string(committedNames)), "\n") {
		if strings.HasPrefix(name, "db/migrations/") && strings.HasSuffix(name, ".sql") {
			want[name] = true
		}
	}
	checkoutDir := filepath.Join(repoDir, "db", "migrations")
	for _, relative := range []string{"db", "db/migrations"} {
		info, err := os.Lstat(filepath.Join(repoDir, filepath.FromSlash(relative)))
		if os.IsNotExist(err) && len(want) == 0 {
			break
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fail(fmt.Errorf("migration inventory directory is unavailable or unsafe"))
		}
	}
	files, err := os.ReadDir(checkoutDir)
	if err != nil && !(os.IsNotExist(err) && len(want) == 0) {
		return fail(fmt.Errorf("read migration checkout inventory: %w", err))
	}
	gotNames := map[string]bool{}
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
			gotNames["db/migrations/"+f.Name()] = true
		}
	}
	if len(want) != len(gotNames) {
		return fail(fmt.Errorf("committed and checkout migration inventory differs"))
	}
	for name := range want {
		if !gotNames[name] || !seen[name] {
			return fail(fmt.Errorf("committed migration omitted or checkout differs: %s", name))
		}
	}
	for _, e := range manifest.Entries {
		if strings.HasPrefix(e.Path, "db/migrations/") && !want[e.Path] {
			return fail(fmt.Errorf("manifest names uncommitted migration"))
		}
	}
	return out, nil
}
