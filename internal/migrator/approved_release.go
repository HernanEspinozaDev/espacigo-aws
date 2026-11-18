package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

type releasePolicy struct {
	Format    string `json:"format"`
	Baseline  int    `json:"baseline"`
	Automatic map[string]struct {
		Checksum string `json:"checksum"`
		Strategy string `json:"strategy"`
	} `json:"automatic_migrations"`
	ReviewRequired map[string]json.RawMessage `json:"review_required_migrations"`
}

func (p releasePolicy) checkPending(plan PlanResult) error {
	if p.Format != "espacigo.initial-migration-policy.v1" || p.Baseline < 1 {
		return errors.New("invalid migration release policy")
	}
	if !plan.HistoryExists || len(plan.Applied) < max(p.Baseline, 100) {
		return errors.New("initialize and verify the production baseline before automatic migrations")
	}
	for _, pending := range plan.Pending {
		rule, approved := p.Automatic[pending.Name]
		_, reviewRequired := p.ReviewRequired[pending.Name]
		if reviewRequired || !approved || rule.Strategy != "expand" || rule.Checksum != pending.Checksum {
			return fmt.Errorf("migration %s requires explicit review before deployment", pending.Name)
		}
	}
	return nil
}

// RunApprovedRelease applies only compatible expansion migrations approved by
// name and checksum. Readback, policy validation, and execution share a lock.
func RunApprovedRelease(ctx context.Context, config *pgx.ConnConfig, password, migrationDir string) (Result, error) {
	policyBytes, err := os.ReadFile(filepath.Join(migrationDir, "..", "release-policy.json"))
	if err != nil {
		return Result{}, errors.New("could not read packaged migration policy")
	}
	var policy releasePolicy
	if err := json.Unmarshal(policyBytes, &policy); err != nil {
		return Result{}, errors.New("invalid packaged migration policy")
	}
	migrations, err := DiscoverMigrations(migrationDir)
	if err != nil {
		return Result{}, err
	}
	config = config.Copy()
	config.Password = password
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return Result{}, errors.New("connect to production migration database")
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "SET ROLE espacigo_schema_owner"); err != nil {
		return Result{}, errors.New("could not assume production migration role")
	}
	unlock, err := AcquireMigrationAdvisoryLock(ctx, conn)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = unlock(context.Background()) }()

	before, err := planWithConn(ctx, conn, migrations, "")
	if err != nil {
		return Result{}, err
	}
	if before.Database != config.Database || before.SessionUser != "espacigo_migrator_login" || before.CurrentUser != "espacigo_schema_owner" {
		return Result{}, errors.New("production migration identity does not match the task")
	}
	if err := policy.checkPending(before); err != nil {
		return Result{}, err
	}
	if len(before.Pending) != 0 {
		var ownerCanCreate bool
		if err := conn.QueryRow(ctx, `SELECT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE')`).Scan(&ownerCanCreate); err != nil {
			return Result{}, err
		}
		if !ownerCanCreate {
			return Result{}, errors.New("enable routine DDL for the dedicated schema owner before applying new migrations")
		}
	}
	result, err := runDiscoveredWithConnOptions(ctx, conn, migrations, nil)
	if err != nil {
		return result, err
	}
	after, err := planWithConn(ctx, conn, migrations, "")
	if err != nil {
		return result, err
	}
	if !after.HistoryExists || len(after.Applied) != len(migrations) || len(after.Pending) != 0 {
		return result, errors.New("production migration postcheck is incomplete")
	}
	return result, nil
}
