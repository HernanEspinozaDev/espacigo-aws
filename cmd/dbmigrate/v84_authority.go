package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
)

const (
	v84AuthorityDatabase = "espacigo_dms_stage_20261020"
	v84AuthorityPassFile = "/var/secrets/v84-admin/password"
)

var v84SHA40 = regexp.MustCompile(`^[a-f0-9]{40}$`)
var v84SHA64 = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validateV84AuthorityFlags(mode, baseline, confirmation, sourceSHA string) error {
	if mode != "inspect" && mode != "prepare" && mode != "finalize" {
		return errors.New("-v84-authority-mode must be inspect, prepare, or finalize")
	}
	if confirmation != v84AuthorityDatabase {
		return errors.New("V84 authority mode requires exact target confirmation")
	}
	if !v84SHA40.MatchString(sourceSHA) {
		return errors.New("V84 authority mode requires an embedded 40-character release source SHA")
	}
	if mode == "finalize" {
		if !v84SHA64.MatchString(baseline) {
			return errors.New("finalize requires a 64-character baseline SHA-256")
		}
	} else if baseline != "" {
		return errors.New("baseline SHA-256 is valid only in finalize mode")
	}
	return nil
}

func validateV84AuthorityInvocation(mode, baseline, confirmation string, settings connectionSettings, sourceSHA string, getenv func(string) string) error {
	if err := validateV84AuthorityFlags(mode, baseline, confirmation, sourceSHA); err != nil {
		return err
	}
	if settings.mode != "direct" || settings.setRole != "" || settings.passwordFile != v84AuthorityPassFile {
		return errors.New("V84 authority mode requires the pinned direct admin session")
	}
	if err := validateDirectConnectionURL(settings.databaseURL); err != nil {
		return err
	}
	u, err := url.Parse(settings.databaseURL)
	if err != nil || strings.TrimPrefix(u.Path, "/") != v84AuthorityDatabase || u.User == nil || u.User.Username() != "postgres" {
		return errors.New("V84 authority mode requires the pinned database and postgres login")
	}
	return validatePGEnvironment(getenv)
}

func runV84Authority(ctx context.Context, out io.Writer, settings connectionSettings, dir, mode, baseline string, getenv func(string) string) error {
	if err := validateV84AuthorityInvocation(mode, baseline, v84AuthorityDatabase, settings, releaseSourceSHA, getenv); err != nil {
		return err
	}
	raw, password, err := settings.databaseURLAndPasswordFile()
	if err != nil {
		return err
	}
	report, err := migrator.ManageV84Authority(ctx, raw, password, dir, mode, baseline)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}

func printV84AuthorityError(out io.Writer, err error) {
	_, _ = fmt.Fprintf(out, "V84 authority failed: %v\n", err)
}
