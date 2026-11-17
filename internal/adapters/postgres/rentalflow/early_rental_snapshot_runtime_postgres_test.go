package rentalflowpg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Characterizes the existing resolver under the database's effective runtime role.
func TestPostgresEarlyRentalSnapshotRuntime(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	migrations, err := migrator.DiscoverMigrations("../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	var v98 *migrator.Migration
	for i := range migrations {
		if migrations[i].Version == 98 {
			v98 = &migrations[i]
			break
		}
	}
	if v98 == nil {
		t.Fatal("canonical V98 migration is unavailable")
	}
	ddl, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ddl.Close(context.Background()); err != nil {
			t.Errorf("close V98 connection: %v", err)
		}
	}()
	txDDL, err := ddl.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txDDL.Exec(ctx, string(v98.SQL)); err != nil {
		_ = txDDL.Rollback(ctx)
		t.Fatalf("apply exact V98 bytes: %v", err)
	}
	if err := txDDL.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// One isolated listing is used for the positive and negative calendar reads.
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dbURL.String(), ids)
	seed, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := seed.Close(context.Background()); err != nil {
			t.Errorf("close snapshot fixture connection: %v", err)
		}
	}()
	if _, err := seed.Exec(ctx, `UPDATE public.espacio SET titulo='Snapshot runtime', precio_base_clp=9000 WHERE id=$1`, ids.space); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) VALUES($1,1,'hora',9000)`, ids.space); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(ctx, `INSERT INTO public.espacio_regla_disponibilidad(espacio_id,hora_apertura,hora_cierre,tarifa_base_dia) VALUES($1,'00:00','24:00',9000)`, ids.space); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2030, 4, 1, 0, 0, 0, 0, time.UTC)
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if _, err := seed.Exec(ctx, `INSERT INTO public.espacio_horario_semanal(espacio_id,activo) VALUES($1,true)`, ids.space); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(ctx, `INSERT INTO public.espacio_horario_semanal_tramo(espacio_id,dia_iso,orden,apertura_minuto,cierre_minuto) VALUES($1,$2,1,780,900)`, ids.space, weekday); err != nil {
		t.Fatal(err)
	}

	repo, err := NewRepository(seed, nil, func() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	interval := rentalflow.Interval{StartAt: date.Add(13 * time.Hour), EndAt: date.Add(14 * time.Hour)}
	resolve := func(t *testing.T, label string) (EarlyRentalOfferSnapshot, error) {
		t.Helper()
		conn, err := pgx.Connect(ctx, dbURL.String())
		if err != nil {
			t.Fatal(err)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			_ = conn.Close(ctx)
			t.Fatal(err)
		}
		closed := false
		t.Cleanup(func() {
			if !closed {
				if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rollback %s transaction: %v", label, err)
				}
				if err := conn.Close(context.Background()); err != nil {
					t.Errorf("close %s connection: %v", label, err)
				}
			}
		})
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
			_ = tx.Rollback(ctx)
			_ = conn.Close(ctx)
			closed = true
			t.Fatalf("set effective runtime role: %v", err)
		}
		var current string
		if err := tx.QueryRow(ctx, `SELECT current_user`).Scan(&current); err != nil {
			_ = tx.Rollback(ctx)
			_ = conn.Close(ctx)
			closed = true
			t.Fatal(err)
		}
		if current != "espacigo_runtime" {
			_ = tx.Rollback(ctx)
			_ = conn.Close(ctx)
			closed = true
			t.Fatalf("effective role=%q, want espacigo_runtime", current)
		}
		got, err := repo.ResolveEarlyRentalOfferSnapshot(ctx, tx, ids.renter, ids.space, interval)
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			t.Errorf("rollback %s transaction: %v", label, rbErr)
		}
		if err := conn.Close(ctx); err != nil {
			t.Errorf("close %s connection: %v", label, err)
		}
		closed = true
		return got, err
	}

	t.Run("MigrationOnlyDeniesListingRead", func(t *testing.T) {
		got, err := resolve(t, "migration-only privileges")
		var pgErr *pgconn.PgError
		if got != (EarlyRentalOfferSnapshot{}) || !errors.As(err, &pgErr) || pgErr.Code != "42501" || pgErr.Message != "permission denied for table espacio" {
			t.Fatalf("migration-only snapshot=%#v error=%v, want zero and espacio permission denial", got, err)
		}
	})

	// Project only the privileges consumed here from dbbootstrap's runtimeACLSQL.
	// V98 installs the calendar/override ACL itself. No persistent ACL is changed.
	if _, err := seed.Exec(ctx, `
		GRANT SELECT ON public.espacio, public.tarifa_espacio, public.usuario,
			public.bloqueo_cuenta_administrativo_local, public.espacio_horario_semanal,
			public.espacio_horario_semanal_tramo, public.ocupacion TO espacigo_runtime;
		GRANT UPDATE (titulo) ON public.espacio TO espacigo_runtime;
	`); err != nil {
		t.Fatalf("install fixture-only subset of runtime privileges: %v", err)
	}
	t.Run("SnapshotAvailable", func(t *testing.T) {
		got, err := resolve(t, "available")
		if err != nil {
			t.Fatalf("resolve available snapshot as runtime: %v", err)
		}
		if got.HostID != ids.host || got.Input.SpaceID != uuid.MustParse(ids.space) || got.Input.SpaceTitle != "Snapshot runtime" ||
			got.Input.RateUnit != earlyrental.RateUnitHour || got.Input.UnitPriceCLP != 9000 || got.Input.Units != 1 ||
			!got.Input.StartAt.Equal(interval.StartAt) || !got.Input.EndAt.Equal(interval.EndAt) || got.Input.TimeZone != "UTC" ||
			got.Input.CalendarRateOverride != earlyrental.CalendarRateOverrideAbsent {
			t.Fatalf("snapshot fields mismatch: %#v", got)
		}
	})
	t.Run("CustomTariffConflict", func(t *testing.T) {
		var customPrice int64
		if err := seed.QueryRow(ctx, `INSERT INTO public.espacio_calendario_override(espacio_id,fecha,estado,tarifa_personalizada_clp) VALUES($1,$2,'disponible',15000) RETURNING tarifa_personalizada_clp`, ids.space, date.Format("2006-01-02")).Scan(&customPrice); err != nil || customPrice != 15000 {
			t.Fatalf("seed custom tariff: price=%d error=%v", customPrice, err)
		}
		got, err := resolve(t, "custom tariff")
		if !errors.Is(err, rentalflow.ErrConflict) || got != (EarlyRentalOfferSnapshot{}) {
			t.Fatalf("custom tariff snapshot=%#v error=%v, want zero and conflict", got, err)
		}
	})
}
