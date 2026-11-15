package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func validDatabaseRoleName(value string) bool {
	if len(value) == 0 || len(value) > 63 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' ||
			(i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func validateMigrationExecutionIdentity(databaseURL, setRole string) error {
	if setRole != "espacigo_schema_owner" {
		return errors.New("DATABASE_SET_ROLE must be explicitly set to espacigo_schema_owner for migration execution")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("DATABASE_URL must identify the dedicated migration login")
	}
	if config.User != "espacigo_migrator_login" {
		return errors.New("DATABASE_URL user must be espacigo_migrator_login for migration execution")
	}
	return nil
}

func validateF5AdminMigrationIdentity(databaseURL, mode, setRole, confirmation string) error {
	if confirmation != runtimeACLConfirmation {
		return errors.New("F5 administrative migration requires exact target confirmation")
	}
	return validateRuntimeACLAdminIdentity(databaseURL, mode, setRole)
}

func withF5AdminMigrationMarker(databaseURL, confirmation string) (string, error) {
	if confirmation != runtimeACLConfirmation {
		return "", errors.New("F5 administrative migration requires exact target confirmation")
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") {
		return "", errors.New("F5 administrative migration requires a PostgreSQL URL")
	}
	query := parsedURL.Query()
	query.Set("application_name", f5AdminMigrationApplicationName)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func validateRuntimeACLAdminIdentity(databaseURL, mode, setRole string) error {
	if mode != "direct" || setRole != "" {
		return errors.New("runtime ACL phase requires direct mode without DATABASE_SET_ROLE")
	}
	passfile := os.Getenv("PGPASSFILE")
	if !filepath.IsAbs(passfile) || filepath.Clean(passfile) == "/dev/null" {
		return errors.New("runtime ACL phase requires an absolute private PGPASSFILE")
	}
	passfileDirectory, err := os.Lstat(filepath.Dir(passfile))
	if err != nil || !passfileDirectory.IsDir() || passfileDirectory.Mode().Perm() != 0o700 {
		return errors.New("runtime ACL phase requires a private PGPASSFILE directory with mode 0700")
	}
	info, err := os.Lstat(passfile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("runtime ACL phase requires a regular PGPASSFILE with mode 0600")
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") {
		return errors.New("runtime ACL phase requires a PostgreSQL URL without embedded credentials")
	}
	if parsedURL.User != nil {
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			return errors.New("runtime ACL phase requires credentials from the protected PGPASSFILE")
		}
	}
	if os.Getenv("PGPASSWORD") != "" || os.Getenv("PGSERVICE") != "" ||
		os.Getenv("PGSERVICEFILE") != "" || os.Getenv("PGOPTIONS") != "" {
		return errors.New("runtime ACL phase requires the protected PGPASSFILE without credential, service, or session-option overrides")
	}
	config, err := pgx.ParseConfigWithOptions(databaseURL, pgx.ParseConfigOptions{
		ParseConfigOptions: pgconn.ParseConfigOptions{
			ConnStringAllowedKeys: []string{"host", "port", "user", "database", "sslmode", "application_name"},
		},
	})
	if err != nil || config.Host != "127.0.0.1" || config.Port != 15434 ||
		config.Database != "espacigo_f5_gate_20261101" || config.User != "postgres" {
		return errors.New("runtime ACL phase requires the exact local F5 gate target and administrator identity")
	}
	return nil
}

type connectionSettings struct {
	databaseURL  string
	passwordFile string
	mode         string
	setRole      string
	timeout      time.Duration
}

func loadConnectionSettings(getenv func(string) string) (connectionSettings, error) {
	settings := connectionSettings{
		databaseURL:  getenv("DATABASE_URL"),
		passwordFile: strings.TrimSpace(getenv("DATABASE_PASSWORD_FILE")),
		mode:         strings.TrimSpace(getenv("DATABASE_CONNECTION_MODE")),
		setRole:      strings.TrimSpace(getenv("DATABASE_SET_ROLE")),
		timeout:      10 * time.Minute,
	}
	if settings.setRole != "" && !validDatabaseRoleName(settings.setRole) {
		return connectionSettings{}, errors.New("DATABASE_SET_ROLE must be a simple PostgreSQL role name")
	}
	if strings.TrimSpace(settings.databaseURL) == "" {
		return connectionSettings{}, errors.New("DATABASE_URL is required")
	}
	if settings.mode == "" {
		settings.mode = "direct"
	}
	if timeoutValue := strings.TrimSpace(getenv("DBMIGRATE_TIMEOUT")); timeoutValue != "" {
		timeout, err := time.ParseDuration(timeoutValue)
		if err != nil || timeout <= 0 || timeout > 4*time.Hour {
			return connectionSettings{}, errors.New("DBMIGRATE_TIMEOUT must be a positive duration no greater than 4h")
		}
		settings.timeout = timeout
	}
	if settings.mode != "direct" {
		return connectionSettings{}, errors.New("DATABASE_CONNECTION_MODE must be direct")
	}
	return settings, nil
}

func validateDirectConnectionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return errors.New("database URL must use postgres or postgresql scheme")
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return errors.New("database URL must not contain an embedded password")
		}
	}
	for key, values := range parsed.Query() {
		if key != "sslmode" || len(values) != 1 || values[0] != "disable" {
			return errors.New("database URL contains unapproved query parameters")
		}
	}
	if _, err := pgx.ParseConfig(rawURL); err != nil {
		return errors.New("invalid database connection configuration")
	}
	return nil
}

func validatePGEnvironment(getenv func(string) string) error {
	for _, key := range []string{
		"PGHOST", "PGHOSTADDR", "PGPORT", "PGDATABASE", "PGUSER",
		"PGPASSWORD", "PGPASSFILE", "PGSERVICE", "PGSERVICEFILE",
		"PGOPTIONS", "PGSSLMODE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY",
	} {
		if strings.TrimSpace(getenv(key)) != "" {
			return errors.New("PostgreSQL environment overrides are not permitted")
		}
	}
	return nil
}

func (settings connectionSettings) databaseURLAndPasswordFile() (string, string, error) {
	password, err := os.ReadFile(settings.passwordFile)
	if err != nil {
		return "", "", errors.New("could not read database password file")
	}
	passwordValue := strings.TrimSpace(string(password))
	if passwordValue == "" {
		return "", "", errors.New("database password file is empty")
	}
	return settings.databaseURL, passwordValue, nil
}
