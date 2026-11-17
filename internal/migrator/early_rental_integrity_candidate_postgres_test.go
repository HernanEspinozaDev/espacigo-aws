package migrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Exercises only candidate-schema invariants; identity, payment and event
// contracts are intentionally outside this schema-integrity test.
func TestRunnerEarlyRentalIntegrityCandidatePostgres(t *testing.T) {
	withEarlyRentalCandidate(t, func(t *testing.T, ctx context.Context, admin, reader *pgx.Conn, databaseURL string, inventory []Migration) {
		f := newEarlyRentalFixture(t, ctx, admin)
		assertEarlyRentalImmutableRejections(t, ctx, admin, reader, f)
		assertEarlyRentalStateTransitions(t, ctx, admin, reader, f)
	})
}

func assertEarlyRentalImmutableRejections(t *testing.T, ctx context.Context, db, reader *pgx.Conn, f *earlyRentalFixture) {
	t.Helper()
	insertEarlyRentalPair(t, ctx, db, f, "retencion_temprana", f.deadline, true)
	cases := []struct {
		name, sql string
		args      []any
	}{
		{"snapshot", `UPDATE public.arriendo_temprano SET titulo_snapshot='changed' WHERE id=$1`, []any{f.offerID}},
		{"parts", `UPDATE public.arriendo_temprano SET unidades=2, subtotal_clp=20000 WHERE id=$1`, []any{f.offerID}},
		{"participants", `UPDATE public.arriendo_temprano SET anfitrion_id=arrendatario_id WHERE id=$1`, []any{f.offerID}},
		{"contract-version", `UPDATE public.arriendo_temprano SET contrato_version=2 WHERE id=$1`, []any{f.offerID}},
		{"contract-text", `UPDATE public.arriendo_temprano SET contrato_texto='changed' WHERE id=$1`, []any{f.offerID}},
		{"contract-digest", `UPDATE public.arriendo_temprano SET contrato_digest=pg_catalog.sha256(convert_to('changed','UTF8')) WHERE id=$1`, []any{f.offerID}},
		{"interval", `UPDATE public.arriendo_temprano SET inicio=inicio+interval '1 hour', termino=termino+interval '1 hour' WHERE id=$1`, []any{f.offerID}},
		{"deadline", `UPDATE public.arriendo_temprano SET vence_en=vence_en+interval '1 minute' WHERE id=$1`, []any{f.offerID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := earlyRentalCounts(t, ctx, reader)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, tc.sql, tc.args...)
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			assertPgError(t, err, "23514", "arriendo_temprano_inmutable_ck")
			if after := earlyRentalCounts(t, ctx, reader); after != before {
				t.Fatalf("counts changed: %v -> %v", before, after)
			}
		})
	}
}

func assertEarlyRentalStateTransitions(t *testing.T, ctx context.Context, db, reader *pgx.Conn, f *earlyRentalFixture) {
	t.Helper()
	// Inconsistent occupancy is rejected at COMMIT; a consistent pair may be
	// updated in either order because both constraint triggers are deferred.
	for _, tc := range []struct {
		name       string
		occupancy  bool
		state, typ string
		expiry     any
		active     bool
		wantErr    bool
	}{
		{"pending-confirmed-with-hold", false, "confirmado", "retencion_temprana", f.deadline, true, true},
		{"pending-confirmed-consistent", true, "confirmado", "reserva_temprana", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f = newEarlyRentalFixture(t, ctx, db)
			before := earlyRentalCounts(t, ctx, reader)
			f.offerID, f.occupancyID = newID(), newID()
			f.deadline = time.Now().UTC().Add(20 * time.Minute).Truncate(time.Microsecond)
			f.start = time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
				t.Fatal(err)
			}
			if err = f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, "retencion_temprana", nil, f.deadline, true); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado='confirmado',revision=revision+1,actualizada_en=actualizada_en+interval '1 second' WHERE id=$1`, f.offerID); err != nil {
				t.Fatal(err)
			}
			if tc.occupancy {
				if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET tipo=$2,expira_en=$3,activo=$4 WHERE id=$1`, f.occupancyID, tc.typ, tc.expiry, tc.active); err != nil {
					t.Fatal(err)
				}
			}
			err = tx.Commit(ctx)
			if tc.wantErr {
				assertPgError(t, err, "23514", "arriendo_temprano_ocupacion_estado_ck")
			} else if err != nil {
				t.Fatal(err)
			}
			var state string
			readErr := reader.QueryRow(ctx, `SELECT estado FROM public.arriendo_temprano WHERE id=$1`, f.offerID).Scan(&state)
			if tc.wantErr {
				if !errors.Is(readErr, pgx.ErrNoRows) {
					t.Fatalf("failed commit persisted row: %q %v", state, readErr)
				}
				if after := earlyRentalCounts(t, ctx, reader); after != before {
					t.Fatalf("counts changed after failed commit: %v -> %v", before, after)
				}
			} else if readErr != nil || state != "confirmado" {
				t.Fatalf("reader state=%q err=%v", state, readErr)
			}
		})
	}
	for _, tc := range []struct {
		name, state, typ string
		expiry           any
		active           bool
		release          bool
	}{
		{"terminal-release", "rechazado", "retencion_temprana", nil, false, true},
		{"terminal-without-release", "cancelado", "retencion_temprana", f.deadline, true, false},
		{"terminal-expiry-mismatch", "vencido", "retencion_temprana", f.deadline.Add(time.Second), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f = newEarlyRentalFixture(t, ctx, db)
			f.offerID, f.occupancyID = newID(), newID()
			f.deadline = time.Now().UTC().Add(20 * time.Minute).Truncate(time.Microsecond)
			f.start = time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
				t.Fatal(err)
			}
			if err = f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, "retencion_temprana", nil, f.deadline, true); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$2,revision=revision+1,actualizada_en=actualizada_en+interval '1 second' WHERE id=$1`, f.offerID, tc.state); err != nil {
				t.Fatal(err)
			}
			if tc.release {
				if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=now() WHERE id=$1`, f.occupancyID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "terminal-expiry-mismatch" {
				if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET expira_en=$2,activo=false,desactivada_en=now() WHERE id=$1`, f.occupancyID, f.deadline.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			err = tx.Commit(ctx)
			if tc.release {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				assertPgError(t, err, "23514", "arriendo_temprano_ocupacion_estado_ck")
			}
			var n int
			if err := reader.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano WHERE id=$1 AND estado=$2`, f.offerID, tc.state).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if (tc.release && n != 1) || (!tc.release && n != 0) {
				t.Fatalf("committed terminal rows=%d", n)
			}
		})
	}
}

func insertEarlyRentalPair(t *testing.T, ctx context.Context, db *pgx.Conn, f *earlyRentalFixture, typ string, expiry any, active bool) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
		t.Fatal(err)
	}
	if err = f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, typ, nil, expiry, active); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertPgError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf("want SQLSTATE/constraint %s/%s, got %v", code, constraint, err)
	}
}

func newID() string { return uuid.NewString() }
