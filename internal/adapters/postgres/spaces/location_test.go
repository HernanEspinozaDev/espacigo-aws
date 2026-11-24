package spacespg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type commercialLocationPool struct {
	availabilityPool
	tx *commercialLocationTx
}

func (p commercialLocationPool) Begin(context.Context) (pgx.Tx, error) { return p.tx, nil }

type commercialLocationTx struct {
	pgx.Tx
	now                       time.Time
	blocked, fixture, missing bool
	updateErr, provenanceErr  error
	writes, commits, bumps    int
	provenanceWrites          int
	source, precision         string
}

func (tx *commercialLocationTx) Rollback(context.Context) error { return nil }
func (tx *commercialLocationTx) Commit(context.Context) error   { tx.commits++; return nil }
func (tx *commercialLocationTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	return monthReadRow(func(d ...any) error {
		switch {
		case strings.Contains(q, "FROM public.usuario"):
			*d[0].(*string) = "activo"
		case strings.Contains(q, "bloqueo_cuenta_administrativo_local"):
			*d[0].(*bool) = tx.blocked
		case strings.Contains(q, "reserva_ensayo_local_fixture"):
			*d[0].(*bool) = tx.fixture
		case strings.Contains(q, "RETURNING actualizado_en"):
			tx.bumps++
			*d[0].(*time.Time) = tx.now.Add(time.Microsecond)
		case strings.Contains(q, "FROM public.espacio"):
			if tx.missing {
				return pgx.ErrNoRows
			}
			if strings.Contains(q, "FOR UPDATE") && (len(args) != 2 || args[0] != calendarOwnerID || args[1] != monthReadSpaceID) {
				return errors.New("missing ownership scope")
			}
			*d[0].(*string) = monthReadSpaceID
			*d[1].(*string) = "Prueba 123"
			*d[2].(*string) = "13"
			*d[3].(*string) = "13101"
			*d[6].(*string) = tx.source
			*d[7].(*string) = tx.precision
			*d[9].(*int64) = int64(tx.writes)
			*d[10].(*time.Time) = tx.now
			if tx.writes > 0 {
				*d[10].(*time.Time) = tx.now.Add(time.Microsecond)
			}
		default:
			return errors.New("unexpected query")
		}
		return nil
	})
}
func (tx *commercialLocationTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "SET ubicacion_pin_origen") {
		if len(args) != 3 || args[0] != monthReadSpaceID || tx.writes != 1 {
			return pgconn.CommandTag{}, errors.New("metadata must follow the guarded location write")
		}
		tx.provenanceWrites++
		tx.source = args[1].(string)
		tx.precision = args[2].(string)
		return pgconn.NewCommandTag("UPDATE 1"), tx.provenanceErr
	}
	if !strings.Contains(q, "pg_advisory_xact_lock") {
		tx.writes++
		return pgconn.NewCommandTag("UPDATE 1"), tx.updateErr
	}
	return pgconn.CommandTag{}, nil
}
func TestCommercialLocationTransactionGuards(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*commercialLocationTx)
		stale bool
		want  error
	}{
		{name: "stale version", stale: true, want: spaces.ErrPreconditionFailed},
		{name: "foreign owner or deleted", setup: func(tx *commercialLocationTx) { tx.missing = true }, want: spaces.ErrNotFound},
		{name: "blocked owner", setup: func(tx *commercialLocationTx) { tx.blocked = true }, want: spaces.ErrPublicationConflict},
		{name: "enabled demo", setup: func(tx *commercialLocationTx) { tx.fixture = true }, want: spaces.ErrEnabledFixture},
		{name: "invalid administrative FK", setup: func(tx *commercialLocationTx) { tx.updateErr = &pgconn.PgError{Code: "23503"} }, want: spaces.ErrInvalid},
		{name: "metadata failure", setup: func(tx *commercialLocationTx) { tx.provenanceErr = &pgconn.PgError{Code: "23514"} }, want: spaces.ErrInvalid},
		{name: "successful replacement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &commercialLocationTx{now: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)}
			if tc.setup != nil {
				tc.setup(tx)
			}
			repo := &Repository{pool: commercialLocationPool{tx: tx}}
			expected := tx.now
			if tc.stale {
				expected = expected.Add(-time.Second)
			}
			lat, lon := -33.45, -70.66
			saved, err := repo.SetOwnLocation(context.Background(), calendarOwnerID, monthReadSpaceID, expected, spaces.LocationInput{
				Address: "Prueba 123", Latitude: &lat, Longitude: &lon, Source: "geocoded", Precision: "approximate",
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if tc.want != nil {
				if tx.commits != 0 || tx.bumps != 0 {
					t.Fatalf("failed transaction committed/bumped: %+v", tx)
				}
			} else if tx.commits != 1 || tx.bumps != 1 || tx.writes != 1 || tx.provenanceWrites != 1 || saved.Source != "geocoded" || saved.Precision != "approximate" {
				t.Fatalf("non-atomic location replacement: %+v", tx)
			}
		})
	}
}

type firstLocationTx struct {
	pgx.Tx
	eligible bool
	reads    int
	err      error
}

func (tx *firstLocationTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	tx.reads++
	return monthReadRow(func(dest ...any) error {
		if !strings.Contains(q, "ubicacion_confirmada_en IS NOT NULL") || !strings.Contains(q, "estado_nuevo='activa'") || len(args) != 1 || args[0] != monthReadSpaceID {
			return errors.New("first publication must check confirmation and historical publication")
		}
		*dest[0].(*bool) = tx.eligible
		return tx.err
	})
}
func TestFirstCommercialPublicationRequiresConfirmedLocation(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		eligible    bool
		want        error
		reads       int
	}{
		{"new unconfirmed draft", "borrador", false, spaces.ErrLocationRequired, 1},
		{"confirmed draft or historically published", "borrador", true, nil, 1},
		{"legacy hidden publication", "oculta", false, nil, 0},
		{"already active", "activa", false, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &firstLocationTx{eligible: tc.eligible}
			err := requireConfirmedLocationForFirstPublication(context.Background(), tx, monthReadSpaceID, tc.state)
			if !errors.Is(err, tc.want) || tx.reads != tc.reads {
				t.Fatalf("err=%v reads=%d want %v/%d", err, tx.reads, tc.want, tc.reads)
			}
		})
	}
}
