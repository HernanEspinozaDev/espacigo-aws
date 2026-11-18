package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// E1MigrationOperation is an independently reviewed input, pinned by the CLI's
// external SHA-256 argument. Observed catalogs must never populate these profiles
// automatically in an applying command.
type E1MigrationOperation struct {
	Source        SourceManifest
	Migration     SingleOwnerMigrationContract
	Before        SingleOwnerAuthorityProfile
	After         SingleOwnerAuthorityProfile
	Backup        MigrationBackupReceipt
	RestoreBinary string
	RDS           *E1RDSTransport `json:",omitempty"`
}

type E1MigrationReport struct {
	SourceCommit    string
	Version         int64
	Target          SingleOwnerMigrationTarget
	PreflightPassed bool
	Committed       bool
	Verified        bool
	CommitOutcome   string
}

// ObserveE1MigrationAuthority returns unapproved metadata only. It does not
// learn/approve a profile and cannot stage or apply SQL.
func ObserveE1MigrationAuthority(ctx context.Context, target SingleOwnerMigrationTarget, scope SingleOwnerAuthorityProfile) (SingleOwnerAuthorityObservation, error) {
	return ObserveE1OperationAuthority(ctx, E1MigrationOperation{
		Migration: SingleOwnerMigrationContract{Target: target}, Before: scope,
	})
}

// ObserveE1OperationAuthority reads the operation's scope over its pinned transport.
func ObserveE1OperationAuthority(ctx context.Context, op E1MigrationOperation) (SingleOwnerAuthorityObservation, error) {
	zero := SingleOwnerAuthorityObservation{}
	target, scope := op.Migration.Target, op.Before
	if target.SessionUser != "espacigo_migrator_login" || target.EffectiveRole != "espacigo_schema_owner" || target.OccupancyOwner != target.EffectiveRole || target.LedgerOwner != target.EffectiveRole {
		return zero, errors.New("E1 inspection requires the named existing migrator/schema owner")
	}
	if err := validateOperationalAuthorityProfile(scope); err != nil {
		return zero, err
	}
	cfg, err := e1ConnectionConfig(op)
	if err != nil {
		return zero, err
	}
	conn, err := connectE1(ctx, cfg, op.RDS != nil)
	if err != nil {
		return zero, errors.New("E1 inspection connection failed")
	}
	defer conn.Close(context.Background())
	tx, err := beginE1Inspection(ctx, conn, target)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback(context.Background())
	return CaptureSingleOwnerAuthority(ctx, tx, scope)
}

func validateE1Operation(op E1MigrationOperation) error {
	t := op.Migration.Target
	if !validDatabaseRoleName(t.Database) || t.SessionUser != "espacigo_migrator_login" || t.EffectiveRole != "espacigo_schema_owner" || t.OccupancyOwner != t.EffectiveRole || t.LedgerOwner != t.EffectiveRole {
		return errors.New("E1 requires the existing non-admin migrator and one unchanged schema owner")
	}
	if err := validateE1Transport(op); err != nil {
		return err
	}
	if op.Backup.SourceCommit != op.Source.Commit || op.Backup.Database != t.Database || op.Backup.DatabaseOID != t.DatabaseOID || op.Backup.SystemIdentifier != t.SystemIdentifier || op.Backup.ServerVersionNum != t.ServerVersionNum {
		return errors.New("backup provenance differs from the reviewed source/target")
	}
	if int64(len(op.Source.Entries)) == op.Migration.Version && op.Source.InventorySHA256 != op.Migration.InventorySHA256 {
		return errors.New("source and execution inventory differ")
	}
	for _, p := range []SingleOwnerAuthorityProfile{op.Before, op.After} {
		if err := validateOperationalAuthorityProfile(p); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range p.Expected.Roles {
			seen[r.Name] = true
		}
		if !seen[t.SessionUser] || !seen[t.EffectiveRole] || !seen["espacigo_runtime"] {
			return errors.New("E1 profile omits a protected principal")
		}
	}
	return nil
}

func e1ConnectionConfig(op E1MigrationOperation) (*pgx.ConnConfig, error) {
	if err := validateE1Transport(op); err != nil {
		return nil, err
	}
	t := op.Migration.Target
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(k, "PG") {
			return nil, fmt.Errorf("PostgreSQL environment override %s is forbidden for E1", k)
		}
	}
	cfg, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, errors.New("construct E1 connection configuration")
	}
	cfg.Host = t.Host
	cfg.Port = t.Port
	cfg.Database = t.Database
	cfg.User = t.SessionUser
	cfg.Password = ""
	cfg.TLSConfig = nil
	cfg.Fallbacks = nil
	cfg.ConnectTimeout = 5 * time.Second
	cfg.RuntimeParams = map[string]string{"application_name": "espacigo-e1-migrator", "search_path": "public"}
	if op.RDS != nil {
		if err := configureE1RDS(cfg, *op.RDS); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func connectE1(ctx context.Context, cfg *pgx.ConnConfig, rds bool) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err == nil && rds {
		err = verifyE1RDSTLS(conn, cfg.Host)
		if err != nil {
			_ = conn.Close(context.Background())
			return nil, err
		}
	}
	return conn, err
}

func beginE1Inspection(ctx context.Context, conn *pgx.Conn, target SingleOwnerMigrationTarget) (pgx.Tx, error) {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO public"); err == nil {
		_, err = tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{target.EffectiveRole}.Sanitize())
	}
	if err == nil {
		err = guardSingleOwnerTarget(ctx, tx, target)
	}
	if err != nil {
		_ = tx.Rollback(context.Background())
		return nil, err
	}
	return tx, nil
}

func inspectE1State(ctx context.Context, conn *pgx.Conn, op E1MigrationOperation, inventory []Migration, after bool) error {
	tx, err := beginE1Inspection(ctx, conn, op.Migration.Target)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	history, err := readHistory(ctx, tx)
	if err != nil {
		return err
	}
	count := len(inventory) - 1
	profile := op.Before
	if after {
		count = len(inventory)
		profile = op.After
	}
	if len(history) != count {
		return errors.New("E1 ledger differs from the exact inspection boundary; reconcile without replay")
	}
	if err := verifyHistory(inventory, history); err != nil {
		return err
	}
	if err := guardAppliedSchemaObjects(ctx, tx, history); err != nil {
		return err
	}
	if after {
		if err := guardE1SchemaCatalog(ctx, tx); err != nil {
			return err
		}
	}
	actual, err := CaptureSingleOwnerAuthority(ctx, tx, profile)
	if err != nil {
		return err
	}
	return CompareSingleOwnerAuthorityProfile(profile, actual)
}

// verifyE1OperationSource verifies every migration in the pinned commit, then
// isolates the explicitly authorized prefix. A newer SQL file in main cannot
// be omitted from provenance checks or included in this single-version operation.
func verifyE1OperationSource(ctx context.Context, repoDir string, op E1MigrationOperation) ([]Migration, error) {
	full, err := VerifyMigrationSource(ctx, repoDir, op.Source)
	if err != nil {
		return nil, err
	}
	if op.Migration.Version < 1 || op.Migration.Version > int64(len(full)) {
		return nil, errors.New("E1 version is outside the verified source inventory")
	}
	prefix := full[:op.Migration.Version]
	if err := validateSingleOwnerMigrationContractForTransport(op.Migration, prefix, op.RDS != nil); err != nil {
		return nil, err
	}
	return prefix, nil
}

// RunE1MigrationOperation stages reviewed source and validates a real archive
// before opening a database connection. apply=false is read-only preflight.
// No catch/retry replays a writer. A commit/postcheck error must be reconciled.
func RunE1MigrationOperation(ctx context.Context, repoDir, stageDir string, op E1MigrationOperation, apply bool) (E1MigrationReport, error) {
	report := E1MigrationReport{SourceCommit: op.Source.Commit, Version: op.Migration.Version, Target: op.Migration.Target, CommitOutcome: "not_started"}
	if err := validateE1Operation(op); err != nil {
		return report, err
	}
	verified, err := verifyE1OperationSource(ctx, repoDir, op)
	if err != nil {
		return report, err
	}
	inventory, err := stageVerifiedMigrationInventory(ctx, verified, stageDir)
	if err != nil {
		return report, err
	}
	if err := ValidateMigrationBackupArchive(ctx, op.Backup, op.RestoreBinary); err != nil {
		return report, err
	}
	cfg, err := e1ConnectionConfig(op)
	if err != nil {
		return report, err
	}
	conn, err := connectE1(ctx, cfg, op.RDS != nil)
	if err != nil {
		return report, errors.New("connect to the reviewed E1 transport failed")
	}
	defer conn.Close(context.Background())
	if err := inspectE1State(ctx, conn, op, inventory, false); err != nil {
		return report, fmt.Errorf("E1 preflight: %w", err)
	}
	report.PreflightPassed = true
	if !apply {
		return report, nil
	}
	guard := func(ctx context.Context, tx pgx.Tx, after bool) error {
		if !after {
			if err := ValidateMigrationBackupFile(ctx, op.Backup); err != nil {
				return err
			}
		}
		profile := op.Before
		if after {
			profile = op.After
		}
		if after {
			if err := guardE1SchemaCatalog(ctx, tx); err != nil {
				return err
			}
		}
		actual, err := captureOperationalAuthority(ctx, tx, profile, false)
		if err != nil {
			return err
		}
		return CompareSingleOwnerAuthorityProfile(profile, actual)
	}
	applied, err := runSingleOwnerMigration(ctx, conn, stageDir, op.Migration, guard, op.RDS != nil)
	report.Committed = applied.Committed
	report.CommitOutcome = "not_committed"
	if applied.CommitUncertain {
		report.CommitOutcome = "unknown"
	}
	if applied.Committed {
		report.CommitOutcome = "committed"
	}
	if err != nil {
		return report, err
	}
	// Fresh connection, not only a new transaction on the writer.
	reader, err := connectE1(ctx, cfg, op.RDS != nil)
	if err != nil {
		return report, errors.New("E1 committed; independent postcheck connection failed; do not replay")
	}
	defer reader.Close(context.Background())
	if err := inspectE1State(ctx, reader, op, inventory, true); err != nil {
		return report, fmt.Errorf("E1 committed; independent postcheck failed; do not replay: %w", err)
	}
	report.Verified = true
	return report, nil
}

// VerifyE1MigrationOperation reconciles a completed/ambiguous operation by
// readback only. It cannot execute DDL or append a migration ledger row.
func VerifyE1MigrationOperation(ctx context.Context, repoDir string, op E1MigrationOperation) (E1MigrationReport, error) {
	report := E1MigrationReport{SourceCommit: op.Source.Commit, Version: op.Migration.Version, Target: op.Migration.Target, CommitOutcome: "unknown"}
	if err := validateE1Operation(op); err != nil {
		return report, err
	}
	inventory, err := verifyE1OperationSource(ctx, repoDir, op)
	if err != nil {
		return report, err
	}
	cfg, err := e1ConnectionConfig(op)
	if err != nil {
		return report, err
	}
	conn, err := connectE1(ctx, cfg, op.RDS != nil)
	if err != nil {
		return report, errors.New("E1 readback connection failed")
	}
	defer conn.Close(context.Background())
	if err := inspectE1State(ctx, conn, op, inventory, true); err != nil {
		return report, err
	}
	report.Committed = true
	report.Verified = true
	report.CommitOutcome = "committed"
	return report, nil
}
