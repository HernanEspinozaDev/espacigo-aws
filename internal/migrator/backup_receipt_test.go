package migrator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backupFixture(t *testing.T, payload []byte) (MigrationBackupReceipt, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "backup.dump")
	if err := os.WriteFile(p, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	return MigrationBackupReceipt{Path: p, SHA256: fmt.Sprintf("%x", sum), Size: int64(len(payload)), Database: "fixture", DatabaseOID: 16384, SystemIdentifier: "1234567890123456789", ServerVersionNum: 180000, SourceCommit: strings.Repeat("a", 40)}, p
}

func TestValidateMigrationBackupFile(t *testing.T) {
	r, _ := backupFixture(t, append([]byte("PGDMP"), 1, 15, 0, 4))
	if err := ValidateMigrationBackupFile(context.Background(), r); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
}
func TestValidateMigrationBackupFileRejectsTampering(t *testing.T) {
	cases := []struct {
		name  string
		alter func(*MigrationBackupReceipt, string)
	}{
		{"sha", func(r *MigrationBackupReceipt, _ string) { r.SHA256 = strings.Repeat("0", 64) }},
		{"size", func(r *MigrationBackupReceipt, _ string) { r.Size++ }},
		{"source commit", func(r *MigrationBackupReceipt, _ string) { r.SourceCommit = strings.Repeat("A", 40) }},
		{"malformed header", func(r *MigrationBackupReceipt, p string) {
			_ = os.WriteFile(p, []byte("NOPE!"), 0600)
			b, _ := os.ReadFile(p)
			s := sha256.Sum256(b)
			r.SHA256 = fmt.Sprintf("%x", s)
			r.Size = int64(len(b))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, p := backupFixture(t, append([]byte("PGDMP"), 1, 15))
			tc.alter(&r, p)
			if err := ValidateMigrationBackupFile(context.Background(), r); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}
func TestValidateMigrationBackupFileRejectsSymlinkAndPermissions(t *testing.T) {
	r, p := backupFixture(t, append([]byte("PGDMP"), 1, 15))
	link := filepath.Join(t.TempDir(), "link.dump")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	r.Path = link
	if err := ValidateMigrationBackupFile(context.Background(), r); err == nil {
		t.Fatal("symlink accepted")
	}
	r.Path = p
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMigrationBackupFile(context.Background(), r); err == nil {
		t.Fatal("non-0600 accepted")
	}
}
func TestValidateMigrationBackupFileCancellation(t *testing.T) {
	r, _ := backupFixture(t, append([]byte("PGDMP"), 1, 15))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateMigrationBackupFile(ctx, r); err == nil {
		t.Fatal("canceled context accepted")
	}
}
func TestValidateMigrationBackupArchiveFakeExecutableStages(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		ok                 bool
	}{{"success", "echo list; [ \"$1\" = --list ] && echo parse; echo restore", "--exit-on-error", true}, {"list fail", "[ \"$1\" = --list ] && exit 7", "--list", false}, {"restore fail", "[ \"$1\" = --list ] && exit 0; exit 9", "--exit-on-error", false}} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := backupFixture(t, append([]byte("PGDMP"), 1, 15))
			log := filepath.Join(t.TempDir(), "stages")
			bin := filepath.Join(t.TempDir(), "pg_restore-fake")
			body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + fmt.Sprintf("%q", log) + "\n" + tc.script + "\n"
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			err := ValidateMigrationBackupArchive(context.Background(), r, bin)
			if (err == nil) != tc.ok {
				t.Fatalf("error=%v", err)
			}
			b, _ := os.ReadFile(log)
			if tc.want != "" && !strings.Contains(string(b), tc.want) {
				t.Fatalf("stages=%q", b)
			}
		})
	}
}
func TestValidateMigrationBackupArchiveCancellation(t *testing.T) {
	r, _ := backupFixture(t, append([]byte("PGDMP"), 1, 15))
	bin := filepath.Join(t.TempDir(), "restore")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateMigrationBackupArchive(ctx, r, bin); err == nil {
		t.Fatal("canceled context accepted")
	}
}

func TestValidateMigrationBackupDoesNotLeakDirectoryDescriptors(t *testing.T) {
	r, _ := backupFixture(t, []byte("PGDMPfixture"))
	if err := ValidateMigrationBackupFile(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := ValidateMigrationBackupFile(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before) {
		t.Fatalf("backup directory descriptors leaked: before=%d after=%d", len(before), len(after))
	}
}

func TestValidateMigrationBackupArchiveRejectsReplacementDuringParse(t *testing.T) {
	r, p := backupFixture(t, []byte("PGDMPfixture"))
	bin := filepath.Join(t.TempDir(), "restore")
	body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = --list ]; then mv %q %q; printf 'foreign sentinel' > %q; chmod 600 %q; fi\nexit 0\n", p, p+".moved", p, p)
	if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMigrationBackupArchive(context.Background(), r, bin); err == nil {
		t.Fatal("backup replaced during external parsing was accepted")
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "foreign sentinel" {
		t.Fatalf("replacement not preserved: %q %v", b, err)
	}
}
