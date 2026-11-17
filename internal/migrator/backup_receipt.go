package migrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type MigrationBackupReceipt struct {
	Path             string
	SHA256           string
	Size             int64
	Database         string
	DatabaseOID      uint32
	SystemIdentifier string
	ServerVersionNum int
	SourceCommit     string
}

var lowerHexSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)
var lowerHexCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var decimalIdentifier = regexp.MustCompile(`^[0-9]+$`)

const linuxTMPFS = 0x01021994

func ValidateMigrationBackupFile(ctx context.Context, r MigrationBackupReceipt) error {
	if ctx == nil {
		return errors.New("backup validation requires context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup validation canceled: %w", err)
	}
	if !filepath.IsAbs(r.Path) || filepath.Clean(r.Path) != r.Path || r.Path == "/" {
		return errors.New("backup path must be an absolute clean file path")
	}
	if !lowerHexSHA.MatchString(r.SHA256) || !lowerHexCommit.MatchString(r.SourceCommit) {
		return errors.New("backup receipt digest or source commit is invalid")
	}
	cluster, clusterErr := strconv.ParseUint(r.SystemIdentifier, 10, 64)
	if r.Size <= 0 || strings.TrimSpace(r.Database) == "" || r.DatabaseOID == 0 || !decimalIdentifier.MatchString(r.SystemIdentifier) || clusterErr != nil || cluster == 0 || r.ServerVersionNum < 180000 || r.ServerVersionNum >= 190000 {
		return errors.New("backup receipt PostgreSQL identity or size is invalid")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(filepath.Dir(r.Path), &fs); err != nil {
		return fmt.Errorf("backup filesystem check: %w", err)
	}
	if uint64(fs.Type) == linuxTMPFS {
		return errors.New("backup filesystem must not be tmpfs")
	}
	// Resolve each parent component without following symlinks.
	parts := strings.Split(strings.TrimPrefix(r.Path, "/"), "/")
	dir, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("backup root open: %w", err)
	}
	defer func() { _ = syscall.Close(dir) }()
	for _, part := range parts[:len(parts)-1] {
		next, e := syscall.Openat(dir, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if e != nil {
			return fmt.Errorf("backup path ancestor validation: %w", e)
		}
		_ = syscall.Close(dir)
		dir = next
	}
	name := parts[len(parts)-1]
	before, err := syscall.Openat(dir, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("backup file open: %w", err)
	}
	f := os.NewFile(uintptr(before), r.Path)
	defer f.Close()
	var openedFS syscall.Statfs_t
	if err := syscall.Fstatfs(before, &openedFS); err != nil {
		return fmt.Errorf("opened backup filesystem: %w", err)
	}
	if uint64(openedFS.Type) == linuxTMPFS || uint64(openedFS.Type) == 0x858458f6 {
		return errors.New("backup file must not reside on volatile memory filesystem")
	}
	var st unix.Stat_t
	if err := unix.Fstat(before, &st); err != nil {
		return fmt.Errorf("backup file stat: %w", err)
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != uint32(os.Getuid()) || st.Mode&07777 != 0600 || st.Size != r.Size {
		return errors.New("backup file type, owner, mode, or size does not match receipt")
	}
	var l unix.Stat_t
	if err := unix.Fstatat(dir, name, &l, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("backup path recheck: %w", err)
	}
	if !sameInode(&st, &l) {
		return errors.New("backup file changed during validation")
	}
	h := sha256.New()
	header := make([]byte, 5)
	n, e := io.ReadFull(f, header)
	if e != nil || n != 5 || string(header) != "PGDMP" {
		return errors.New("backup archive header is invalid")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("backup seek: %w", err)
	}
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("backup hash: %w", err)
	}
	var after unix.Stat_t
	if err := unix.Fstat(before, &after); err != nil {
		return fmt.Errorf("backup post-read stat: %w", err)
	}
	if !sameInode(&st, &after) || after.Size != st.Size || after.Mtim != st.Mtim || after.Ctim != st.Ctim {
		return errors.New("backup file changed while hashing")
	}
	if err := unix.Fstatat(dir, name, &l, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameInode(&after, &l) {
		return errors.New("backup path replaced while hashing")
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), r.SHA256) {
		return errors.New("backup SHA-256 does not match receipt")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup validation canceled: %w", err)
	}
	return nil
}

func sameInode(a, b *unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode&syscall.S_IFMT == b.Mode&syscall.S_IFMT
}

func ValidateMigrationBackupArchive(ctx context.Context, r MigrationBackupReceipt, restoreBinary string) error {
	if err := ValidateMigrationBackupFile(ctx, r); err != nil {
		return fmt.Errorf("backup file validation: %w", err)
	}
	if !filepath.IsAbs(restoreBinary) || filepath.Clean(restoreBinary) != restoreBinary {
		return errors.New("restore binary path must be absolute and clean")
	}
	st, err := os.Lstat(restoreBinary)
	if err != nil {
		return fmt.Errorf("restore binary validation: %w", err)
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0111 == 0 {
		return errors.New("restore binary must be a regular executable, not a symlink")
	}
	f, err := os.Open(r.Path)
	if err != nil {
		return fmt.Errorf("open backup for parsing: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	onPath, err := os.Lstat(r.Path)
	if err != nil || !os.SameFile(info, onPath) || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != r.Size {
		return errors.New("backup changed before external parsing")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return errors.New("backup bytes changed before external parsing")
	}
	for _, args := range [][]string{{"--list", "/proc/self/fd/3"}, {"--exit-on-error", "--file=/dev/null", "/proc/self/fd/3"}} {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, restoreBinary, args...)
		cmd.ExtraFiles = []*os.File{f}
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
		cmd.Stdin = nil
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("restore validation canceled: %w", ctx.Err())
			}
			if args[0] == "--list" {
				return errors.New("pg_restore list validation failed")
			}
			return errors.New("pg_restore parse validation failed")
		}
	}
	if err := ValidateMigrationBackupFile(ctx, r); err != nil {
		return fmt.Errorf("backup post-parse validation: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restore validation canceled: %w", err)
	}
	return nil
}
