package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const earlyRentalLockPostgresOptIn = "TEST_RENTALFLOW_POSTGRES_ISOLATED"
const earlyRentalLockRuntimeRole = "espacigo_runtime"

func TestPostgresEarlyRentalCreateKeyLock(t *testing.T) {
	if os.Getenv(earlyRentalLockPostgresOptIn) != "1" {
		t.Skip(earlyRentalLockPostgresOptIn + "=1 is required for isolated PostgreSQL lock integration")
	}
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("TEST_DATABASE_URL is required when PostgreSQL lock integration is opted in")
	}
	target, err := validateDisposablePostgresTarget(adminURL, os.Getenv("TEST_RENTALFLOW_POSTGRES_ISOLATION_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Role bootstrap is the only shared-cluster change; connect only after the
	// exact disposable Unix-socket marker and token have been validated.
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	roleCreated := false
	var connections []*pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, c := range connections {
			if e := c.Close(cleanupCtx); e != nil {
				t.Errorf("close lock-test connection: %v", e)
			}
		}
		if roleCreated {
			if _, e := admin.Exec(cleanupCtx, `DROP ROLE `+pgx.Identifier{earlyRentalLockRuntimeRole}.Sanitize()); e != nil {
				t.Errorf("drop owned runtime role: %v", e)
			} else {
				var remains bool
				if e := admin.QueryRow(cleanupCtx, `SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1)`, earlyRentalLockRuntimeRole).Scan(&remains); e != nil || remains {
					t.Errorf("owned role absence: remains=%v err=%v", remains, e)
				}
			}
		}
		if e := admin.Close(cleanupCtx); e != nil {
			t.Errorf("close lock admin: %v", e)
		}
	})
	var roleTx pgx.Tx
	roleTx, err = admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = roleTx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1)`, earlyRentalLockRuntimeRole).Scan(&exists); err == nil && !exists {
		_, err = roleTx.Exec(ctx, `CREATE ROLE `+pgx.Identifier{earlyRentalLockRuntimeRole}.Sanitize()+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`)
	}
	if err == nil {
		var login, super, createdb, createrole, inherit bool
		err = roleTx.QueryRow(ctx, `SELECT rolcanlogin, rolsuper, rolcreatedb, rolcreaterole, rolinherit FROM pg_catalog.pg_roles WHERE rolname=$1`, earlyRentalLockRuntimeRole).Scan(&login, &super, &createdb, &createrole, &inherit)
		if err == nil && (login || super || createdb || createrole || inherit) {
			err = fmt.Errorf("existing runtime role has unsafe flags: login=%v super=%v createdb=%v createrole=%v inherit=%v", login, super, createdb, createrole, inherit)
		}
	}
	if err != nil {
		_ = roleTx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = roleTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	roleCreated = !exists

	// Every connection below addresses the validated harness database; no
	// application schema or business rows are needed for advisory locks.
	dbURL := *target
	dbURL.Path = "/postgres"
	connect := func() *pgx.Conn {
		c, e := pgx.Connect(ctx, dbURL.String())
		if e != nil {
			t.Fatal(e)
		}
		connections = append(connections, c)
		return c
	}
	holder, waiter, observer := connect(), connect(), connect()

	actor := uuid.MustParse("8c8c47c1-9dab-4f0d-97d5-08a8f2744f81")
	otherActor := uuid.MustParse("90d71fe2-8c03-4c67-b7d5-0ed8d9ce8c39")
	key := "opaque-key-α-東京"
	otherKey := "different-key-β-大阪"

	t.Run("SameActorKeyCommitRelease", func(t *testing.T) { runLockWaitCase(t, ctx, holder, waiter, observer, actor, key, false) })
	t.Run("SameActorKeyRollbackRelease", func(t *testing.T) { runLockWaitCase(t, ctx, holder, waiter, observer, actor, key, true) })
	t.Run("DifferentActorAndKeyNoBlocking", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			actor uuid.UUID
			key   string
		}{{"DifferentKey", actor, otherKey}, {"DifferentActor", otherActor, key}} {
			t.Run(tc.name, func(t *testing.T) {
				object, err := lockObject(actor, key)
				if err != nil {
					t.Fatal(err)
				}
				other, err := lockObject(tc.actor, tc.key)
				if err != nil {
					t.Fatal(err)
				}
				if object == other {
					t.Fatal("literal fixture lock objects collide")
				}
				checkNoBlockCase(t, ctx, holder, waiter, observer, actor, key, tc.actor, tc.key)
			})
		}
	})
	t.Run("CanceledWaiterReleasesLocks", func(t *testing.T) { runCanceledWaiterCase(t, ctx, holder, waiter, observer, actor, key) })
}

func lockObject(actor uuid.UUID, key string) (int32, error) {
	if actor == uuid.Nil || len([]rune(key)) < 8 {
		return 0, errors.New("invalid lock fixture")
	}
	// Independent literal vector preimage; do not call production hashing code.
	preimage := "espacigo.early-rental.create-idempotency.v1\x00" + actor.String() + "\x00" + key
	digest := sha256.Sum256([]byte(preimage))
	return int32(binary.BigEndian.Uint32(digest[:4])), nil
}

func lockTx(t *testing.T, ctx context.Context, c *pgx.Conn, actor uuid.UUID, key string) pgx.Tx {
	t.Helper()
	tx, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{earlyRentalLockRuntimeRole}.Sanitize()); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = lockEarlyRentalCreateKey(ctx, tx, actor, key); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	return tx
}

func assertWaitObserved(t *testing.T, ctx context.Context, observer *pgx.Conn, waiterPID, holderPID int32, object int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var blocked, matching bool
		err := observer.QueryRow(ctx, `SELECT $1 = ANY(pg_catalog.pg_blocking_pids($2)), EXISTS (SELECT 1 FROM pg_catalog.pg_locks WHERE pid=$2 AND locktype='advisory' AND NOT granted AND classid=1163019057::oid AND objid=$3::oid AND objsubid=2)`, holderPID, waiterPID, uint32(object)).Scan(&blocked, &matching)
		if err == nil && blocked && matching {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended waiting for advisory contention: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("waiter %d was not observed blocked by holder %d on expected advisory object %d", waiterPID, holderPID, object)
}

func backendPID(t *testing.T, ctx context.Context, c *pgx.Conn) int32 {
	t.Helper()
	var pid int32
	if err := c.QueryRow(ctx, `SELECT pg_catalog.pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}

func runLockWaitCase(t *testing.T, ctx context.Context, holder, waiter, observer *pgx.Conn, actor uuid.UUID, key string, rollbackHolder bool) {
	t.Helper()
	object, err := lockObject(actor, key)
	if err != nil {
		t.Fatal(err)
	}
	htx := lockTx(t, ctx, holder, actor, key)
	wtx, err := waiter.Begin(ctx)
	if err != nil {
		_ = htx.Rollback(context.Background())
		t.Fatal(err)
	}
	if _, err = wtx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{earlyRentalLockRuntimeRole}.Sanitize()); err != nil {
		_ = wtx.Rollback(context.Background())
		_ = htx.Rollback(context.Background())
		t.Fatal(err)
	}
	hpid, wpid := backendPID(t, ctx, holder), backendPID(t, ctx, waiter)
	if hpid == wpid {
		_ = wtx.Rollback(context.Background())
		_ = htx.Rollback(context.Background())
		t.Fatal("holder and waiter share a backend")
	}
	result := make(chan error, 1)
	wctx, wcancel := context.WithTimeout(ctx, 8*time.Second)
	joined := false
	defer func() {
		wcancel()
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = htx.Rollback(cleanup)
		if !joined {
			<-result
			joined = true
		}
		// Join the canceled Exec before reusing its pgx transaction.
		_ = wtx.Rollback(cleanup)
	}()
	go func() { result <- lockEarlyRentalCreateKey(wctx, wtx, actor, key) }()
	assertWaitObserved(t, ctx, observer, wpid, hpid, object)
	if rollbackHolder {
		err = htx.Rollback(ctx)
	} else {
		err = htx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		joined = true
		if err != nil {
			t.Fatalf("waiter lock after holder release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertNoOwnedAdvisoryLocks(t, ctx, observer, hpid, wpid, object)
}

func checkNoBlockCase(t *testing.T, ctx context.Context, holder, waiter, observer *pgx.Conn, actor uuid.UUID, key string, otherActor uuid.UUID, otherKey string) {
	t.Helper()
	object, _ := lockObject(actor, key)
	htx := lockTx(t, ctx, holder, actor, key)
	defer htx.Rollback(context.Background())
	wtx := lockTx(t, ctx, waiter, otherActor, otherKey)
	defer wtx.Rollback(context.Background())
	var blocked bool
	if err := observer.QueryRow(ctx, `SELECT cardinality(pg_catalog.pg_blocking_pids($1)) > 0`, backendPID(t, ctx, waiter)).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked {
		t.Fatal("different actor/key unexpectedly blocked")
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := htx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertNoOwnedAdvisoryLocks(t, ctx, observer, backendPID(t, ctx, holder), backendPID(t, ctx, waiter), object)
}

func runCanceledWaiterCase(t *testing.T, ctx context.Context, holder, waiter, observer *pgx.Conn, actor uuid.UUID, key string) {
	t.Helper()
	object, _ := lockObject(actor, key)
	htx := lockTx(t, ctx, holder, actor, key)
	wtx, err := waiter.Begin(ctx)
	if err != nil {
		_ = htx.Rollback(context.Background())
		t.Fatal(err)
	}
	if _, err = wtx.Exec(ctx, `SET LOCAL ROLE `+pgx.Identifier{earlyRentalLockRuntimeRole}.Sanitize()); err != nil {
		_ = wtx.Rollback(context.Background())
		_ = htx.Rollback(context.Background())
		t.Fatal(err)
	}
	hpid, wpid := backendPID(t, ctx, holder), backendPID(t, ctx, waiter)
	wctx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	joined := false
	defer func() {
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = htx.Rollback(cleanupCtx)
		if !joined {
			<-result
		}
		_ = wtx.Rollback(cleanupCtx)
	}()
	go func() { result <- lockEarlyRentalCreateKey(wctx, wtx, actor, key) }()
	assertWaitObserved(t, ctx, observer, wpid, hpid, object)
	cancel()
	select {
	case err = <-result:
		joined = true
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("canceled waiter error=%v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCleanup()
	if err = wtx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) && !waiter.IsClosed() {
		t.Fatalf("rollback canceled waiter: %v", err)
	}
	if err = htx.Rollback(cleanup); err != nil {
		t.Fatal(err)
	}
	if err := waiter.Close(cleanup); err != nil {
		t.Fatal(err)
	}
	// Cancellation can close the client before PostgreSQL has reaped the
	// backend. Observe eventual server cleanup, not an assumed instant fence.
	for {
		var remains bool
		if err := observer.QueryRow(cleanup, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE pid=$1)`, wpid).Scan(&remains); err != nil {
			t.Fatal(err)
		}
		if !remains {
			break
		}
		select {
		case <-cleanup.Done():
			t.Fatal("canceled waiter backend remains", cleanup.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	assertNoOwnedAdvisoryLocks(t, cleanup, observer, hpid, wpid, object)
}

func assertNoOwnedAdvisoryLocks(t *testing.T, ctx context.Context, observer *pgx.Conn, holderPID, waiterPID int32, object int32) {
	t.Helper()
	var count int
	err := observer.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_locks WHERE pid = ANY($1::int[]) AND locktype='advisory'`, []int32{holderPID, waiterPID}).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("owned advisory locks remain: %d", count)
	}
}
