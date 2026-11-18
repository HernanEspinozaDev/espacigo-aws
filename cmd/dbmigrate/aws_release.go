package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/jackc/pgx/v5"
)

func runAWSMigration(output io.Writer, migrationDir, mode string, getenv func(string) string) error {
	login, role := "espacigo_migrator_login", "espacigo_schema_owner"
	admin := mode != "aws-release"
	if admin {
		login, role = "espacigo_admin", ""
	}
	settings, err := loadConnectionSettings(getenv)
	if err != nil {
		return err
	}
	if err := validatePGEnvironment(getenv); err != nil {
		return err
	}
	raw, err := url.Parse(settings.databaseURL)
	if err != nil || raw.Scheme != "postgresql" && raw.Scheme != "postgres" || raw.User == nil ||
		raw.User.Username() != login || settings.setRole != role ||
		raw.Path != "/espacigo_prod" || raw.Hostname() != getenv("AWS_RDS_HOST") ||
		!strings.HasSuffix(raw.Hostname(), ".rds.amazonaws.com") || raw.Fragment != "" {
		return errors.New("AWS migration requires the configured production RDS target and dedicated roles")
	}
	if _, embedded := raw.User.Password(); embedded {
		return errors.New("AWS migration forbids credentials embedded in the database URL")
	}
	query, err := url.ParseQuery(raw.RawQuery)
	if err != nil || len(query) != 2 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "verify-full" ||
		len(query["sslrootcert"]) != 1 || !filepath.IsAbs(query.Get("sslrootcert")) {
		return errors.New("AWS migration requires PostgreSQL hostname verification and an absolute CA path")
	}
	config, err := pgx.ParseConfig(settings.databaseURL)
	if err != nil || config.TLSConfig == nil || config.TLSConfig.InsecureSkipVerify ||
		config.TLSConfig.ServerName != raw.Hostname() || len(config.Fallbacks) != 0 {
		return errors.New("invalid AWS migration TLS configuration")
	}
	password, err := awsMigrationPassword(settings, admin, getenv)
	if err != nil {
		return err
	}
	if admin && (config.Port != 5432 || !strings.HasSuffix(raw.Hostname(), ".us-east-2.rds.amazonaws.com")) {
		return errors.New("AWS bootstrap requires the production RDS Region and port")
	}
	ctx, cancel := context.WithTimeout(context.Background(), settings.timeout)
	defer cancel()
	if mode == "aws-runtime-v83" {
		return migrator.ApplyAWSV83RuntimeACL(ctx, config, password, migrationDir, func(ctx context.Context, tx pgx.Tx) error {
			return dbbootstrap.GrantRuntimePermissions(ctx, tx)
		})
	}
	if mode == "aws-bootstrap-v80" {
		result, err := migrator.RunAWSBootstrap(ctx, config, password, migrationDir)
		evidence := struct {
			Verified bool    `json:"verified"`
			Applied  []int64 `json:"applied"`
		}{Verified: err == nil, Applied: result.Applied}
		if encodeErr := json.NewEncoder(output).Encode(evidence); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	result, err := migrator.RunApprovedRelease(ctx, config, password, migrationDir)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

// The ECS execution role injects only the password field for the dedicated
// release task. Privileged bootstrap modes still require a private file.
func awsMigrationPassword(settings connectionSettings, admin bool, getenv func(string) string) (string, error) {
	injected := getenv("DATABASE_ECS_PASSWORD")
	if injected == "" {
		_, password, err := settings.databaseURLAndPasswordFile()
		return password, err
	}
	if admin || settings.passwordFile != "" {
		return "", errors.New("ECS password injection requires the dedicated release mode and one credential source")
	}
	if strings.TrimSpace(injected) == "" || strings.ContainsAny(injected, "\r\n\x00") {
		return "", errors.New("invalid injected ECS database password")
	}
	return injected, nil
}
