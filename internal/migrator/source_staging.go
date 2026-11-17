package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StageVerifiedMigrationSource verifies a pinned source and materializes the exact
// verified SQL bytes into a new private directory. This is offline staging only;
// it does not authorize or execute migrations.
func StageVerifiedMigrationSource(ctx context.Context, repoDir string, manifest SourceManifest, destination string) ([]Migration, error) {
	verified, err := VerifyMigrationSource(ctx, repoDir, manifest)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("staging context canceled: %w", err)
	}
	if destination == "" {
		return nil, fmt.Errorf("destination is required")
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("invalid staging destination: %w", err)
	}
	if filepath.Clean(dest) != dest || filepath.Dir(dest) == dest {
		return nil, fmt.Errorf("invalid staging destination")
	}
	parent := filepath.Dir(dest)
	for p := parent; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("destination parent unavailable or has symlink ancestor")
		}
		if p == parent && !info.IsDir() {
			return nil, fmt.Errorf("destination parent is not a directory")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return nil, fmt.Errorf("create staging destination: %w", err)
	}
	ownedInfo, err := os.Lstat(dest)
	if err != nil {
		return nil, fmt.Errorf("identify staging destination: %w", err)
	}
	if !ownedInfo.IsDir() || ownedInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("new staging destination is not a real directory")
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		cleanupErr := removeOwnedDirectory(dest, ownedInfo, nil, nil)
		return nil, errors.Join(fmt.Errorf("open staging destination: %w", err), cleanupErr)
	}
	defer func() { _ = root.Close() }()
	ownedFiles := make([]stagedFileIdentity, 0, len(verified))
	fail := func(cause error) ([]Migration, error) {
		cleanupErr := removeOwnedDirectory(dest, ownedInfo, root, ownedFiles)
		return nil, errors.Join(cause, cleanupErr)
	}
	if err := root.Chmod(".", 0700); err != nil {
		return fail(fmt.Errorf("secure staging destination: %w", err))
	}
	staged := make([]Migration, len(verified))
	for i, m := range verified {
		if err := ctx.Err(); err != nil {
			return fail(fmt.Errorf("staging context canceled: %w", err))
		}
		f, err := root.OpenFile(m.Name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fail(fmt.Errorf("create staged migration: %w", err))
		}
		fileInfo, statErr := f.Stat()
		if statErr != nil {
			_ = f.Close()
			return fail(fmt.Errorf("identify staged migration: %w", statErr))
		}
		ownedFiles = append(ownedFiles, stagedFileIdentity{name: m.Name, info: fileInfo})
		if err := f.Chmod(0600); err != nil {
			_ = f.Close()
			return fail(fmt.Errorf("secure staged migration: %w", err))
		}
		_, writeErr := f.Write(m.SQL)
		closeErr := f.Close()
		if writeErr != nil {
			return fail(fmt.Errorf("write staged migration: %w", writeErr))
		}
		if closeErr != nil {
			return fail(fmt.Errorf("close staged migration: %w", closeErr))
		}
		staged[i] = Migration{Version: m.Version, Name: m.Name, Path: filepath.Join(dest, m.Name), SQL: append([]byte(nil), m.SQL...), Checksum: m.Checksum}
	}
	if err := ctx.Err(); err != nil {
		return fail(fmt.Errorf("staging context canceled: %w", err))
	}
	currentInfo, err := os.Lstat(dest)
	if err != nil || !os.SameFile(ownedInfo, currentInfo) {
		if err == nil {
			err = fmt.Errorf("staging destination identity changed")
		}
		return fail(fmt.Errorf("verify staging destination identity: %w", err))
	}
	return staged, nil
}

type stagedFileIdentity struct {
	name string
	info os.FileInfo
}

func removeOwnedDirectory(path string, owned os.FileInfo, root *os.Root, files []stagedFileIdentity) error {
	current, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cleanup staging destination: owned directory no longer present")
		}
		return fmt.Errorf("cleanup staging destination identity check: %w", err)
	}
	if !os.SameFile(owned, current) {
		return fmt.Errorf("cleanup staging destination refused: directory identity changed")
	}
	// Never recursively remove a name: unknown or replaced entries must survive.
	for _, file := range files {
		info, err := root.Lstat(file.name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !os.SameFile(file.info, info) {
			return fmt.Errorf("cleanup staged file refused: identity unavailable or changed")
		}
		if err := root.Remove(file.name); err != nil {
			return fmt.Errorf("cleanup staged file: %w", err)
		}
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cleanup staging destination: %w", err)
	}
	return nil
}
