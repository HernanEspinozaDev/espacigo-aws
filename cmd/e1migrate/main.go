package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
)

func readOperation(path, pin string) (migrator.E1MigrationOperation, error) {
	var op migrator.E1MigrationOperation
	decoded, err := hex.DecodeString(pin)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != pin {
		return op, errors.New("operation SHA-256 must be explicit lowercase hex")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return op, errors.New("operation path must be absolute and clean")
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > 4<<20 {
		return op, errors.New("operation must be a regular mode-0600 file of at most 4 MiB")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return op, errors.New("cannot open operation")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(st, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 {
		return op, errors.New("operation identity changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return op, errors.New("cannot read bounded operation")
	}
	final, err := os.Lstat(path)
	if err != nil || !final.Mode().IsRegular() || final.Mode().Perm() != 0600 || !os.SameFile(opened, final) || opened.Size() != final.Size() || !opened.ModTime().Equal(final.ModTime()) {
		return op, errors.New("operation changed while reading")
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != pin {
		return op, errors.New("operation differs from reviewed SHA-256")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&op); err != nil {
		return op, errors.New("invalid or unknown operation fields")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return op, errors.New("operation has trailing JSON")
	}
	return op, nil
}

func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("e1migrate", flag.ContinueOnError)
	flags.SetOutput(errOut)
	mode := flags.String("mode", "", "explicit inspect, preflight, apply or postcheck")
	operation := flags.String("operation", "", "absolute private reviewed operation JSON")
	pin := flags.String("operation-sha256", "", "independent reviewed operation SHA-256")
	repo := flags.String("repo", "", "exact absolute source Git root")
	stage := flags.String("stage", "", "new absolute private staging directory (preflight/apply)")
	confirm := flags.String("confirm", "", "apply only: exact database:system-identifier:version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(errOut, err); return 2 }
	if flags.NArg() != 0 || (*mode != "inspect" && *mode != "preflight" && *mode != "apply" && *mode != "postcheck") {
		return fail(errors.New("one explicit E1 mode is required; positional arguments are forbidden"))
	}
	if !filepath.IsAbs(*repo) {
		return fail(errors.New("an absolute source repository is required"))
	}
	if (*mode == "preflight" || *mode == "apply") && !filepath.IsAbs(*stage) {
		return fail(errors.New("preflight/apply require an absolute new staging directory"))
	}
	if (*mode == "postcheck" || *mode == "inspect") && *stage != "" {
		return fail(errors.New("inspection/postcheck never stages applying SQL"))
	}
	op, err := readOperation(*operation, *pin)
	if err != nil {
		return fail(err)
	}
	expected := fmt.Sprintf("%s:%s:%d", op.Migration.Target.Database, op.Migration.Target.SystemIdentifier, op.Migration.Version)
	if (*mode == "apply" && *confirm != expected) || (*mode != "apply" && *confirm != "") {
		return fail(errors.New("apply requires its exact database/cluster/version confirmation; other modes reject confirmation"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if *mode == "inspect" {
		observed, err := migrator.ObserveE1MigrationAuthority(ctx, op.Migration.Target, op.Before)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		if err := json.NewEncoder(out).Encode(observed); err != nil {
			fmt.Fprintln(errOut, "cannot encode unapproved inspection")
			return 1
		}
		return 0
	}
	var report migrator.E1MigrationReport
	if *mode == "postcheck" {
		report, err = migrator.VerifyE1MigrationOperation(ctx, *repo, op)
	} else {
		report, err = migrator.RunE1MigrationOperation(ctx, *repo, *stage, op, *mode == "apply")
	}
	return writeResult(out, errOut, report, err)
}

func writeResult(out, errOut io.Writer, report migrator.E1MigrationReport, executionErr error) int {
	if executionErr != nil {
		fmt.Fprintln(errOut, executionErr)
	}
	if encodeErr := json.NewEncoder(out).Encode(report); encodeErr != nil {
		fmt.Fprintln(errOut, "cannot encode E1 result")
		return 1
	}
	if executionErr != nil {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
