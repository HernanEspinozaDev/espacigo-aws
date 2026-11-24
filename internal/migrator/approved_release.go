package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type releasePolicy struct {
	Format    string `json:"format"`
	Baseline  int    `json:"baseline"`
	Automatic map[string]struct {
		Checksum            string   `json:"checksum"`
		Strategy            string   `json:"strategy"`
		RequiresSchemaOwner []string `json:"requires_schema_owner,omitempty"`
	} `json:"automatic_migrations"`
	ReviewRequired map[string]json.RawMessage `json:"review_required_migrations"`
}

func (p releasePolicy) checkPending(plan PlanResult) error {
	if p.Format != "espacigo.initial-migration-policy.v1" || p.Baseline < 1 {
		return errors.New("invalid migration release policy")
	}
	if !plan.HistoryExists || len(plan.Applied) < p.Baseline {
		return errors.New("initialize and verify the production baseline before automatic migrations")
	}
	for _, pending := range plan.Pending {
		rule, approved := p.Automatic[pending.Name]
		_, reviewRequired := p.ReviewRequired[pending.Name]
		if reviewRequired || !approved || rule.Strategy != "expand" || rule.Checksum != pending.Checksum {
			return fmt.Errorf("migration %s requires explicit review before deployment", pending.Name)
		}
		for _, target := range rule.RequiresSchemaOwner {
			if !isSupportedOwnerTarget(target) {
				return fmt.Errorf("migration %s has invalid schema-owner preflight target %q", pending.Name, target)
			}
		}
	}
	return nil
}

var ownerTargetIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func isSupportedOwnerTarget(target string) bool {
	parts := strings.Split(target, ".")
	return len(parts) == 2 && parts[0] == "public" && ownerTargetIdentifier.MatchString(parts[1])
}

type schemaOwnerQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func pendingOwnerTargets(policy releasePolicy, pending []MigrationSummary) map[string][]string {
	targets := make(map[string][]string)
	for _, migration := range pending {
		rule := policy.Automatic[migration.Name]
		for _, target := range rule.RequiresSchemaOwner {
			targets[target] = append(targets[target], migration.Name)
		}
	}
	for target := range targets {
		sort.Strings(targets[target])
	}
	return targets
}

func checkPendingSchemaOwners(ctx context.Context, queryer schemaOwnerQueryer, policy releasePolicy, pending []MigrationSummary) error {
	targets := pendingOwnerTargets(policy, pending)
	orderedTargets := make([]string, 0, len(targets))
	for target := range targets {
		orderedTargets = append(orderedTargets, target)
	}
	sort.Strings(orderedTargets)
	for _, target := range orderedTargets {
		migrations := targets[target]
		parts := strings.SplitN(target, ".", 2)
		var owner string
		err := queryer.QueryRow(ctx, `
			SELECT pg_catalog.pg_get_userbyid(c.relowner)
			FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p', 'v', 'm', 'f')`, parts[0], parts[1]).Scan(&owner)
		if err != nil {
			return fmt.Errorf("schema-owner preflight for %s required by %s failed: relation missing or unreadable", target, strings.Join(migrations, ", "))
		}
		if owner != "espacigo_schema_owner" {
			return fmt.Errorf("schema-owner preflight blocked %s required by %s: current owner is %s; prepare a reviewed one-object transfer", target, strings.Join(migrations, ", "), owner)
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
		ownerTx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
		if err != nil {
			return Result{}, errors.New("could not start read-only migration ownership preflight")
		}
		if err := checkPendingSchemaOwners(ctx, ownerTx, policy, before.Pending); err != nil {
			_ = ownerTx.Rollback(context.Background())
			return Result{}, err
		}
		if err := ownerTx.Commit(ctx); err != nil {
			return Result{}, errors.New("could not complete read-only migration ownership preflight")
		}
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
