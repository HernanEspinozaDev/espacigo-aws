package migrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RunAWSBootstrap initializes bootstrap roles and applies the migration inventory.
func RunAWSBootstrap(ctx context.Context, config *pgx.ConnConfig, password, dir string) (Result, error) {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return Result{}, err
	}
	if len(migrations) == 0 {
		return Result{}, errors.New("AWS bootstrap requires at least one migration")
	}
	cfg := config.Copy()
	cfg.Password = password
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return Result{}, errors.New("connect to RDS bootstrap database")
	}
	defer conn.Close(context.Background())
	var identityOK bool
	if err := conn.QueryRow(ctx, `SELECT current_database() = 'espacigo_prod'
		AND session_user = 'espacigo_admin' AND current_user = session_user
		AND current_setting('server_version_num')::int / 10000 = 18
		AND EXISTS (SELECT 1 FROM pg_catalog.pg_stat_ssl
			WHERE pid = pg_backend_pid() AND ssl)`).Scan(&identityOK); err != nil || !identityOK {
		return Result{}, errors.New("unexpected RDS bootstrap identity or TLS status")
	}
	release, err := AcquireMigrationAdvisoryLock(ctx, conn)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = release(context.Background()) }()
	if err := guardHistoryInitialization(ctx, conn); err != nil {
		return Result{}, err
	}
	var ledger bool
	if err := conn.QueryRow(ctx, `SELECT pg_catalog.to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&ledger); err != nil {
		return Result{}, err
	}
	if ledger {
		history, err := readHistory(ctx, conn)
		if err != nil {
			return Result{}, err
		}
		if err := verifyHistory(migrations, history); err != nil {
			return Result{}, err
		}
	}
	if err := ensureAWSBootstrapRoles(ctx, conn); err != nil {
		return Result{}, err
	}
	result, err := runDiscoveredWithConnOptions(ctx, conn, migrations, nil)
	if err != nil {
		return result, err
	}
	if err := VerifyAllApplied(ctx, conn, dir, len(migrations)); err != nil {
		return result, err
	}
	return result, nil
}

func ensureAWSBootstrapRoles(ctx context.Context, conn *pgx.Conn) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		// No role receives a password in bootstrap. Runtime credentials are provisioned separately.
		for _, role := range []struct {
			name  string
			login bool
		}{
			{"espacigo_runtime", false},
			{"espacigo_api_login", true},
			{"espacigo_migrator_login", true},
			{"espacigo_schema_owner", false},
			{"espacigo_document_verification_fn", false},
			{"espacigo_privacy_cleanup_worker", false},
			{"espacigo_privacy_worker_login", true},
		} {
			var exists, valid bool
			if err := tx.QueryRow(ctx, `SELECT count(*) = 1, coalesce(bool_and(
				rolcanlogin = $2 AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR
				rolinherit OR rolreplication OR rolbypassrls)), false)
				FROM pg_catalog.pg_roles WHERE rolname = $1`, role.name, role.login).Scan(&exists, &valid); err != nil {
				return err
			}
			if exists {
				if !valid {
					return fmt.Errorf("incompatible bootstrap role %s", role.name)
				}
				continue
			}
			login := " NOLOGIN"
			if role.login {
				login = " LOGIN"
			}
			if _, err := tx.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role.name}.Sanitize()+login+
				" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS"); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `GRANT espacigo_runtime TO espacigo_api_login
			WITH ADMIN FALSE, INHERIT FALSE, SET TRUE`)
		return err
	})
}

// ApplyAWSV83RuntimeACL installs the canonical application ACL once at the
// reviewed baseline.
func ApplyAWSV83RuntimeACL(ctx context.Context, config *pgx.ConnConfig, password, dir string, apply func(context.Context, pgx.Tx) error) error {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return errors.New("AWS runtime ACL requires at least one migration")
	}
	config = config.Copy()
	config.Password = password
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return errors.New("connect to RDS runtime ACL database")
	}
	defer conn.Close(context.Background())
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var bound bool
		if err := tx.QueryRow(ctx, `SELECT current_database() = 'espacigo_prod'
			AND session_user = 'espacigo_admin' AND current_user = session_user
			AND EXISTS (SELECT 1 FROM pg_catalog.pg_stat_ssl WHERE pid = pg_backend_pid() AND ssl)`).Scan(&bound); err != nil || !bound {
			return errors.New("unexpected runtime ACL target identity")
		}
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.pg_advisory_xact_lock($1, $2)`, advisoryLockClass, advisoryLockObject); err != nil {
			return err
		}
		history, err := readHistory(ctx, tx)
		if err != nil {
			return err
		}
		if err := verifyCompleteHistory(migrations, history, len(migrations)); err != nil {
			return err
		}
		return apply(ctx, tx)
	})
}
