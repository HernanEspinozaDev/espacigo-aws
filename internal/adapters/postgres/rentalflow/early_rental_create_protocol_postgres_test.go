package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestPostgresEarlyRentalCreateProtocol proves manual composition of the
// persistence primitives under READ COMMITTED; it is not a service/HTTP test.
func TestPostgresEarlyRentalCreateProtocol(t *testing.T) {
	for _, tc := range []struct {
		name     string
		conflict bool
	}{
		{name: "SameKeySamePayload"},
		{name: "SameKeyDifferentPayload", conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The exact readback helper requires an otherwise empty offer table.
			// Give each scenario its own owned database, not just distinct actors.
			ctx, dbURL := earlyInsertPostgresFixture(t)
			caseCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			ids := earlyInsertIDs()
			seedEarlyInsertActors(t, caseCtx, dbURL.String(), ids)
			key := "create-protocol-" + uuid.NewString()
			record := earlyInsertRecord(t, ids, time.Date(2030, 7, 8, 12, 0, 0, 0, time.UTC))
			actor := uuid.MustParse(ids.renter)
			fingerprint, err := earlyRentalCreateFingerprint(uuid.MustParse(ids.space), record.Offer.Snapshot.StartAt, record.Offer.Snapshot.EndAt)
			if err != nil {
				t.Fatal(err)
			}
			waiterFingerprint := fingerprint
			if tc.conflict {
				waiterFingerprint[0] ^= 0xff
			}
			lockDigest := sha256.Sum256([]byte("espacigo.early-rental.create-idempotency.v1\x00" + actor.String() + "\x00" + key))
			lockObject := int32(binary.BigEndian.Uint32(lockDigest[:4]))

			dial := func() *pgx.Conn {
				c, e := pgx.Connect(caseCtx, dbURL.String())
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := c.Close(cleanup); err != nil {
						t.Errorf("close protocol connection: %v", err)
					}
				})
				return c
			}
			holder, waiter, observer := dial(), dial(), dial()
			var holderTx, waiterTx pgx.Tx
			var waitCancel context.CancelFunc
			joined := true
			waitResult := make(chan error, 1)
			cleanup := func() {
				cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cleanCancel()
				if waitCancel != nil {
					waitCancel()
				}
				if holderTx != nil {
					_ = holderTx.Rollback(cleanCtx)
				}
				if !joined {
					<-waitResult
					joined = true
				}
				if waiterTx != nil {
					_ = waiterTx.Rollback(cleanCtx)
				}

			}
			t.Cleanup(cleanup)

			var before replayCounts
			before = replayTableCounts(t, caseCtx, dbURL.String())
			beginRuntime := func(c *pgx.Conn) pgx.Tx {
				tx, e := c.BeginTx(caseCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
				if e != nil {
					t.Fatal(e)
				}
				if _, e = tx.Exec(caseCtx, `SET LOCAL ROLE espacigo_runtime`); e != nil {
					_ = tx.Rollback(context.Background())
					t.Fatal(e)
				}
				return tx
			}
			holderTx = beginRuntime(holder)
			if err = lockEarlyRentalCreateKey(caseCtx, holderTx, actor, key); err != nil {
				t.Fatal(err)
			}
			if id, found, e := lookupEarlyRentalCreateReplay(caseCtx, holderTx, actor, key, fingerprint); e != nil || found || id != uuid.Nil {
				t.Fatalf("holder initial lookup=(%s,%v,%v)", id, found, e)
			}
			if err = insertEarlyRentalInitial(caseCtx, holderTx, record); err != nil {
				t.Fatalf("insert initial offer/hold/event: %v", err)
			}
			eventID, err := insertEarlyRentalCreateClaim(caseCtx, holderTx, actor, uuid.MustParse(ids.offer), key, fingerprint)
			if err != nil || eventID == uuid.Nil {
				t.Fatalf("insert create claim event=%s err=%v", eventID, err)
			}
			if got := replayTableCounts(t, caseCtx, dbURL.String()); got != before {
				t.Fatalf("uncommitted holder rows visible: before=%+v got=%+v", before, got)
			}

			waiterTx = beginRuntime(waiter)
			var holderPID, waiterPID int32
			if err = holder.QueryRow(caseCtx, `SELECT pg_catalog.pg_backend_pid()`).Scan(&holderPID); err != nil {
				t.Fatal(err)
			}
			if err = waiter.QueryRow(caseCtx, `SELECT pg_catalog.pg_backend_pid()`).Scan(&waiterPID); err != nil {
				t.Fatal(err)
			}
			if holderPID == waiterPID {
				t.Fatal("holder and waiter must use distinct PostgreSQL backends")
			}
			waitCtx, cancelWait := context.WithTimeout(caseCtx, 20*time.Second)
			waitCancel, joined = cancelWait, false
			go func() { waitResult <- lockEarlyRentalCreateKey(waitCtx, waiterTx, actor, key) }()
			deadline := time.Now().Add(8 * time.Second)
			for {
				var blocked, pending bool
				e := observer.QueryRow(caseCtx, `SELECT $1 = ANY(pg_catalog.pg_blocking_pids($2)), EXISTS(SELECT 1 FROM pg_catalog.pg_locks WHERE pid=$2 AND locktype='advisory' AND NOT granted AND classid=1163019057::oid AND objid=$3::oid AND objsubid=2)`, holderPID, waiterPID, uint32(lockObject)).Scan(&blocked, &pending)
				if e == nil && blocked && pending {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("waiter %d not observed blocked by holder %d on advisory object %d (query error %v)", waiterPID, holderPID, lockObject, e)
				}
				select {
				case <-caseCtx.Done():
					t.Fatal(caseCtx.Err())
				case <-time.After(15 * time.Millisecond):
				}
			}
			if err = holderTx.Commit(caseCtx); err != nil {
				t.Fatalf("commit holder: %v", err)
			}
			holderTx = nil
			select {
			case err = <-waitResult:
				joined = true
				if err != nil {
					t.Fatalf("waiter acquire key lock: %v", err)
				}
			case <-caseCtx.Done():
				t.Fatalf("waiter did not acquire lock: %v", caseCtx.Err())
			}
			// This is a fresh READ COMMITTED statement snapshot acquired only after
			// the waiter's advisory lock succeeded.
			gotID, found, lookupErr := lookupEarlyRentalCreateReplay(caseCtx, waiterTx, actor, key, waiterFingerprint)
			if tc.conflict {
				if !errors.Is(lookupErr, errEarlyRentalCreateKeyConflict) || gotID != uuid.Nil || found {
					t.Fatalf("different payload lookup=(%s,%v,%v), want key conflict with zero/false", gotID, found, lookupErr)
				}
			} else if lookupErr != nil || !found || gotID != uuid.MustParse(ids.offer) {
				t.Fatalf("same payload lookup=(%s,%v,%v), want original offer", gotID, found, lookupErr)
			}
			if err = waiterTx.Commit(caseCtx); err != nil {
				t.Fatalf("commit read-only replay/conflict transaction: %v", err)
			}
			waiterTx = nil

			after := replayTableCounts(t, caseCtx, dbURL.String())
			if after.offers != before.offers+1 || after.occupancies != before.occupancies+1 || after.events != before.events+1 || after.claims != before.claims+1 {
				t.Fatalf("expected exactly one initial offer/occupancy/event/claim: before=%+v after=%+v", before, after)
			}
			assertEarlyInsertReadback(t, caseCtx, dbURL.String(), ids, record)
			var claimCount int
			if err = observer.QueryRow(caseCtx, `SELECT count(*) FROM public.arriendo_temprano_idempotencia WHERE actor_id=$1 AND comando='crear_oferta' AND clave=$2 AND arriendo_id=$3 AND huella=$4 AND evento_id=$5`, actor, key, uuid.MustParse(ids.offer), fingerprint[:], eventID).Scan(&claimCount); err != nil {
				t.Fatal(err)
			}
			if claimCount != 1 {
				t.Fatalf("original claim metadata rows=%d, want 1", claimCount)
			}
		})
	}
}
