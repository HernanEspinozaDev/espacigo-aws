package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsInvalidInvocationBeforeConnection(t *testing.T) {
	for _, args := range [][]string{nil, {"--mode", "apply"}, {"--mode", "unknown"}, {"--mode", "postcheck", "--repo", "/x", "--stage", "/x"}, {"--mode", "inspect", "--repo", "/x", "--operation", "/missing", "--operation-sha256", strings.Repeat("0", 64)}} {
		var out, err bytes.Buffer
		if code := run(args, &out, &err); code != 2 || out.Len() != 0 {
			t.Fatalf("unsafe invocation accepted: %q code=%d out=%s err=%s", args, code, out.String(), err.String())
		}
	}
}

func TestCLIParsesOnlyPinnedPrivateOperation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "op.json")
	payload := []byte(`{"Source":{},"Migration":{},"Before":{},"After":{},"Backup":{},"RestoreBinary":"/usr/bin/pg_restore"}`)
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	pin := hex.EncodeToString(digest[:])
	if _, err := readOperation(file, pin); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperation(file, strings.Repeat("0", 64)); err == nil {
		t.Fatal("unreviewed operation accepted")
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperation(file, pin); err == nil {
		t.Fatal("public operation accepted")
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "op-link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperation(link, pin); err == nil {
		t.Fatal("operation symlink accepted")
	}
	for _, text := range []string{`{"UnknownField":true}`, `{} {}`} {
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(text))
		if _, err := readOperation(file, hex.EncodeToString(sum[:])); err == nil {
			t.Fatal("unknown or trailing operation accepted")
		}
	}
}
