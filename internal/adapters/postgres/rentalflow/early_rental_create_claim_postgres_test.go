package rentalflowpg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresEarlyRentalCreateClaim(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	dsn := dbURL.String()
	base := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, base)
	now := time.Date(2030, 6, 1, 12, 0, 0, 123456000, time.UTC)
	key := "create-claim-東京"
	fp := replayFingerprint(t, base.space, now)

	t.Run("CommitMetadataAndReplay", func(t *testing.T) {
		// The shared fixture now invokes the production claim writer and checks
		// every persisted claim field through a separate connection after commit.
		claimReplay(t, ctx, dbURL, base, now, key, fp)
		assertLookup(t, ctx, dbURL, base.renter, key, fp, uuid.MustParse(base.offer), true, nil)
	})

	t.Run("MissingOffer", func(t *testing.T) {
		before := replayTableCounts(t, ctx, dsn)
		_, tx := createClaimNativeTx(t, ctx, dsn)
		if err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse(base.renter), key+"-missing"); err != nil {
			t.Fatal(err)
		}
		id, err := insertEarlyRentalCreateClaim(ctx, tx, uuid.MustParse(base.renter), uuid.New(), key+"-missing", fp)
		if id != uuid.Nil || !errors.Is(err, earlyrental.ErrInvalid) {
			t.Fatalf("missing offer: id=%s err=%v", id, err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if got := replayTableCounts(t, ctx, dsn); got != before {
			t.Fatalf("missing offer changed rows: %v -> %v", before, got)
		}
	})

	t.Run("WrongActor", func(t *testing.T) {
		before := replayTableCounts(t, ctx, dsn)
		_, tx := createClaimNativeTx(t, ctx, dsn)
		if err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse(base.host), key+"-wrong-actor"); err != nil {
			t.Fatal(err)
		}
		id, err := insertEarlyRentalCreateClaim(ctx, tx, uuid.MustParse(base.host), uuid.MustParse(base.offer), key+"-wrong-actor", fp)
		if id != uuid.Nil || !errors.Is(err, earlyrental.ErrInvalid) {
			t.Fatalf("wrong actor: id=%s err=%v", id, err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if got := replayTableCounts(t, ctx, dsn); got != before {
			t.Fatalf("wrong actor changed rows: %v -> %v", before, got)
		}
	})

	t.Run("DuplicateKey", func(t *testing.T) {
		before := replayTableCounts(t, ctx, dsn)
		conn, tx := createClaimNativeTx(t, ctx, dsn)
		if err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse(base.renter), key); err != nil {
			t.Fatal(err)
		}
		id, err := insertEarlyRentalCreateClaim(ctx, tx, uuid.MustParse(base.renter), uuid.MustParse(base.offer), key, fp)
		var pg *pgconn.PgError
		if id != uuid.Nil || !errors.As(err, &pg) || pg.Code != "23505" {
			t.Fatalf("duplicate: id=%s err=%v", id, err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var expectedConstraint string
		if err := conn.QueryRow(ctx, `SELECT conname FROM pg_catalog.pg_constraint WHERE conrelid='public.arriendo_temprano_idempotencia'::regclass AND contype='u'`).Scan(&expectedConstraint); err != nil {
			t.Fatal(err)
		}
		if pg.ConstraintName != expectedConstraint {
			t.Fatalf("duplicate failed on %s, want %s", pg.ConstraintName, expectedConstraint)
		}
		if got := replayTableCounts(t, ctx, dsn); got != before {
			t.Fatalf("duplicate changed rows: %v -> %v", before, got)
		}
		assertLookup(t, ctx, dbURL, base.renter, key, fp, uuid.MustParse(base.offer), true, nil)
	})

	t.Run("CallerRollback", func(t *testing.T) {
		before := replayTableCounts(t, ctx, dsn)
		ids := freshReplayIDs(base)
		record := earlyInsertRecord(t, ids, now.Add(24*time.Hour))
		_, tx := createClaimNativeTx(t, ctx, dsn)
		if err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse(ids.renter), key+"-rollback"); err != nil {
			t.Fatal(err)
		}
		if _, found, err := lookupEarlyRentalCreateReplay(ctx, tx, uuid.MustParse(ids.renter), key+"-rollback", fp); err != nil || found {
			t.Fatalf("initial lookup: found=%v err=%v", found, err)
		}
		if err := insertEarlyRentalInitial(ctx, tx, record); err != nil {
			t.Fatal(err)
		}
		eventID, err := insertEarlyRentalCreateClaim(ctx, tx, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer), key+"-rollback", fp)
		if err != nil || eventID == uuid.Nil {
			t.Fatalf("claim before rollback: event=%s err=%v", eventID, err)
		}
		var actualEvent uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM public.arriendo_temprano_evento WHERE arriendo_id=$1 AND secuencia=1`, ids.offer).Scan(&actualEvent); err != nil || actualEvent != eventID {
			t.Fatalf("initial event mismatch: %v", err)
		}
		if got := replayTableCounts(t, ctx, dsn); got != before {
			t.Fatal("uncommitted transaction visible to independent reader")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if got := replayTableCounts(t, ctx, dsn); got != before {
			t.Fatalf("rollback left partial rows: %v -> %v", before, got)
		}
	})
}

func createClaimNativeTx(t *testing.T, ctx context.Context, dsn string) (*pgx.Conn, pgx.Tx) {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Close(cleanup); err != nil {
			t.Errorf("close native claim connection: %v", err)
		}
	})
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) && !conn.IsClosed() {
			t.Errorf("rollback native claim transaction: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	return conn, tx
}
