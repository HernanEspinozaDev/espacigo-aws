package migrator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SingleOwnerMigrationTarget must be supplied from a separately reviewed local
// preflight, not learned from the execution connection and silently accepted.
// Cluster identifier and database OID distinguish same-name replacement targets.
type SingleOwnerMigrationTarget struct {
	Host             string
	Port             uint16
	Database         string
	DatabaseOID      uint32
	SystemIdentifier string
	ServerVersionNum int
	SessionUser      string
	OccupancyOwner   string
	LedgerOwner      string
}

// SingleOwnerMigrationContract binds SQL bytes, not a Git commit. An operational
// caller must separately prove that this reviewed inventory belongs to the
// authorized source. A caller-supplied SHA string would not prove provenance.
type SingleOwnerMigrationContract struct {
	InventorySHA256 string
	Version         int64
	Name            string
	Checksum        string
	Target          SingleOwnerMigrationTarget
}

type SingleOwnerMigrationResult struct {
	Contract  SingleOwnerMigrationContract
	Committed bool
	Verified  bool
}

func validateSingleOwnerMigrationContract(c SingleOwnerMigrationContract, migrations []Migration) error {
	if !validHex(c.InventorySHA256, 64) || !validHex(c.Checksum, 64) || c.Version < 2 {
		return errors.New("single-owner migration requires exact inventory, version and SQL checksum")
	}
	if err := verifyRangeInventory(migrations, migrationRange{c.Version - 1, c.Version}); err != nil {
		return err
	}
	for _, m := range migrations {
		if m.Checksum != sha256Hex(m.SQL) {
			return errors.New("single-owner migration SQL differs from its checksum")
		}
	}
	last := migrations[len(migrations)-1]
	if inventoryFingerprint(migrations) != c.InventorySHA256 || last.Name != c.Name || last.Version != c.Version || last.Checksum != c.Checksum {
		return errors.New("single-owner migration differs from reviewed inventory")
	}
	// Execute through the extended protocol below: PostgreSQL rejects multiple
	// top-level statements. A DO block cannot commit/rollback inside our explicit
	// transaction, unlike a raw simple-protocol migration script.
	if !strings.HasPrefix(string(last.SQL), "DO $migration$\n") || !strings.HasSuffix(strings.TrimSpace(string(last.SQL)), "\n$migration$;") {
		return errors.New("single-owner migration requires one reviewed DO $migration$ block")
	}
	t := c.Target
	ip := net.ParseIP(t.Host)
	localHost := filepath.IsAbs(t.Host) || (ip != nil && ip.IsLoopback())
	cluster, err := strconv.ParseUint(t.SystemIdentifier, 10, 64)
	if !localHost || t.Port == 0 || t.Database == "" || t.DatabaseOID == 0 || err != nil || cluster == 0 || t.ServerVersionNum < 180000 || t.ServerVersionNum >= 190000 || !validDatabaseRoleName(t.SessionUser) || !validDatabaseRoleName(t.OccupancyOwner) || !validDatabaseRoleName(t.LedgerOwner) {
		return errors.New("single-owner migration requires an exact local PostgreSQL 18 target and owners")
	}
	return nil
}

// RunSingleOwnerMigration applies exactly one reviewed DO-block migration on a
// dedicated caller-owned connection. It never opens a connection, initializes a
// ledger, grants privileges or transfers ownership. There is deliberately no CLI
// integration yet: backup, target provenance and human authorization are separate
// operational prerequisites. The caller must close this dedicated connection.
// A commit error is ambiguous and must be reconciled by a fresh read, not replay.
func RunSingleOwnerMigration(ctx context.Context, conn *pgx.Conn, migrationDir string, contract SingleOwnerMigrationContract) (SingleOwnerMigrationResult, error) {
	result := SingleOwnerMigrationResult{Contract: contract}
	if ctx == nil || conn == nil || conn.IsClosed() || conn.PgConn().TxStatus() != 'I' {
		return result, errors.New("single-owner migration requires an idle dedicated connection")
	}
	migrations, err := DiscoverMigrations(migrationDir)
	if err != nil {
		return result, err
	}
	if err := validateSingleOwnerMigrationContract(contract, migrations); err != nil {
		return result, err
	}
	cfg := conn.Config()
	if cfg.Host != contract.Target.Host || cfg.Port != contract.Target.Port || cfg.Database != contract.Target.Database || cfg.User != contract.Target.SessionUser {
		return result, errors.New("single-owner migration connection differs from reviewed target")
	}
	release, err := AcquireMigrationAdvisoryLock(ctx, conn)
	if err != nil {
		return result, err
	}
	defer func() { _ = release(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := guardSingleOwnerTarget(ctx, tx, contract.Target); err != nil {
		return result, err
	}
	// Stabilize the legacy relation's ownership and the exact ledger while both
	// role-separated DDL and ledger insertion remain in this one transaction.
	if _, err := tx.Exec(ctx, "LOCK TABLE public.ocupacion, public.schema_migrations IN ACCESS EXCLUSIVE MODE"); err != nil {
		return result, fmt.Errorf("lock single-owner migration relations: %w", err)
	}
	if err := guardSingleOwnerTarget(ctx, tx, contract.Target); err != nil {
		return result, err
	}
	if err := guardHistoryInitialization(ctx, tx); err != nil {
		return result, err
	}
	history, err := readHistory(ctx, tx)
	if err != nil {
		return result, err
	}
	if len(history) != int(contract.Version-1) {
		return result, errors.New("single-owner migration requires the exact predecessor ledger; reconcile before retry")
	}
	if err := verifyHistory(migrations, history); err != nil {
		return result, err
	}
	if err := guardAppliedSchemaObjects(ctx, tx, history); err != nil {
		return result, err
	}
	runnerDigest, err := executableChecksum()
	if err != nil {
		return result, err
	}
	migration := migrations[len(migrations)-1]
	// pgx.Exec falls back to simple protocol for zero arguments even with an
	// explicit QueryExecModeExec. Use PgConn.ExecParams to keep the server-side
	// single-statement boundary for a parameterless DO block.
	if execution := tx.Conn().PgConn().ExecParams(ctx, string(migration.SQL), nil, nil, nil, nil).Read(); execution.Err != nil {
		return result, fmt.Errorf("execute single-owner migration: %w", execution.Err)
	}
	if err := guardSingleOwnerTarget(ctx, tx, contract.Target); err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations (version,name,checksum,runner_checksum) VALUES ($1,$2,$3,$4)`, migration.Version, migration.Name, migration.Checksum, runnerDigest); err != nil {
		return result, fmt.Errorf("record single-owner migration: %w", err)
	}
	if err := verifySingleOwnerLedger(ctx, tx, migrations); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("single-owner migration commit outcome requires reconciliation: %w", err)
	}
	result.Committed = true
	// A separate read-only transaction checks committed state, not merely values
	// visible to the writer before commit. Failure preserves Committed=true.
	post, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, fmt.Errorf("committed migration postcheck: %w", err)
	}
	defer func() { _ = post.Rollback(context.Background()) }()
	if err := guardSingleOwnerTarget(ctx, post, contract.Target); err != nil {
		return result, err
	}
	if err := verifySingleOwnerLedger(ctx, post, migrations); err != nil {
		return result, err
	}
	result.Verified = true
	return result, nil
}

func guardSingleOwnerTarget(ctx context.Context, q queryRower, expected SingleOwnerMigrationTarget) error {
	actual := SingleOwnerMigrationTarget{Host: expected.Host, Port: expected.Port}
	var currentUser string
	err := q.QueryRow(ctx, `SELECT current_database(), d.oid,
		(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),
		current_setting('server_version_num')::int, session_user::text, current_user::text,
		(SELECT pg_catalog.pg_get_userbyid(c.relowner) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='ocupacion' AND c.relkind='r'),
		(SELECT pg_catalog.pg_get_userbyid(c.relowner) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='schema_migrations' AND c.relkind='r')
		FROM pg_catalog.pg_database d WHERE d.datname=current_database()`).Scan(&actual.Database, &actual.DatabaseOID, &actual.SystemIdentifier, &actual.ServerVersionNum, &actual.SessionUser, &currentUser, &actual.OccupancyOwner, &actual.LedgerOwner)
	if err != nil {
		return fmt.Errorf("read single-owner migration target: %w", err)
	}
	if actual != expected || currentUser != expected.SessionUser {
		return errors.New("single-owner migration target, effective role or owners differ from reviewed contract")
	}
	return nil
}

func verifySingleOwnerLedger(ctx context.Context, q migrationHistoryQueryer, migrations []Migration) error {
	history, err := readHistory(ctx, q)
	if err != nil {
		return err
	}
	if len(history) != len(migrations) {
		return errors.New("single-owner migration ledger postcheck is incomplete")
	}
	return verifyHistory(migrations, history)
}
