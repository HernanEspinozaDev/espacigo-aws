package rentalflowpg

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	writerHostID      = "10000000-0000-4000-8000-000000000001"
	writerRenterID    = "20000000-0000-4000-8000-000000000002"
	writerOfferID     = "30000000-0000-4000-8000-000000000003"
	writerSpaceID     = "50000000-0000-4000-8000-000000000005"
	writerOccupancyID = "4a000000-0000-0000-8000-000000000004"
	writerContract    = "Early rental contract — exact UTF-8 bytes\n"
)

type writerExec struct {
	sql  string
	args []any
}
type writerFakeTx struct {
	pgx.Tx
	t        *testing.T
	calls    []writerExec
	failAt   int
	failErr  error
	affected []int
	cancel   context.CancelFunc
	cancelAt int
}

func (f *writerFakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, writerExec{sql: sql, args: append([]any(nil), args...)})
	n := len(f.calls)
	if f.cancel != nil && f.cancelAt == n {
		f.cancel()
	}
	if f.failAt == n {
		return pgconn.CommandTag{}, f.failErr
	}
	if n > len(f.affected) {
		f.t.Fatalf("unexpected extra Exec #%d: %q", n, sql)
	}
	return pgconn.NewCommandTag(fmt.Sprintf("INSERT 0 %d", f.affected[n-1])), nil
}
func (f *writerFakeTx) Begin(context.Context) (pgx.Tx, error) {
	f.t.Fatal("Begin called")
	return nil, nil
}
func (f *writerFakeTx) Commit(context.Context) error   { f.t.Fatal("Commit called"); return nil }
func (f *writerFakeTx) Rollback(context.Context) error { f.t.Fatal("Rollback called"); return nil }

func writerFixture(t *testing.T) earlyRentalInitialRecord {
	t.Helper()
	created := time.Date(2032, 1, 2, 15, 0, 0, 0, time.UTC)
	start := created.Add(2 * time.Hour)
	offer, event, err := earlyrental.NewOffer(earlyrental.NewOfferInput{
		ID: writerOfferID, HostID: writerHostID, RenterID: writerRenterID,
		Snapshot:        earlyrental.OfferSnapshotInput{SpaceID: uuid.MustParse(writerSpaceID), SpaceTitle: "Habitación", RateUnit: earlyrental.RateUnitHour, UnitPriceCLP: 6000, Units: 1, StartAt: start, EndAt: start.Add(time.Hour), TimeZone: "America/Santiago", CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent},
		ContractVersion: 7, ContractText: writerContract,
	}, created)
	if err != nil {
		t.Fatal(err)
	}
	r, err := newEarlyRentalInitialRecord(offer, writerOccupancyID, event, created)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// This oracle intentionally duplicates the SQL contract instead of consulting production statement definitions.
func writerWant(r earlyRentalInitialRecord) []writerExec {
	o, s, e := r.Offer, r.Offer.Snapshot, r.InitialEvent
	return []writerExec{
		{`INSERT INTO public.arriendo_temprano (id, espacio_id, anfitrion_id, arrendatario_id, titulo_snapshot, modalidad, precio_unitario_clp, moneda, unidades, subtotal_clp, inicio, termino, zona_horaria, contrato_version, contrato_texto, contrato_digest, estado, revision, creada_en, actualizada_en, vence_en, ocupacion_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,1,$18,$18,$19,$20)`, []any{o.ID, s.SpaceID, o.HostID, o.RenterID, s.SpaceTitle, s.RateUnit, s.UnitPriceCLP, s.Currency, s.Units, s.SubtotalCLP, s.StartAt, s.EndAt, s.TimeZone, o.ContractVersion, o.ContractText, append([]byte(nil), r.ContractDigest...), o.State, r.CreatedAt, o.ExpiresAt, r.OccupancyID}},
		{`INSERT INTO public.ocupacion (id, espacio_id, reserva_id, intervalo, tipo, activo, expira_en, motivo, creada_en, desactivada_en, arriendo_temprano_id) VALUES ($1,$2,NULL,tstzrange($3,$4,'[)'),'retencion_temprana',true,$5,NULL,$6,NULL,$7)`, []any{r.OccupancyID, s.SpaceID, s.StartAt, s.EndAt, o.ExpiresAt, r.CreatedAt, o.ID}},
		{`INSERT INTO public.arriendo_temprano_evento (arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,ocurrido_en) VALUES ($1,1,1,$2,$3,NULL,$4,$5)`, []any{o.ID, e.Kind, e.ActorID, e.NextState, e.At}},
	}
}
func compareWriterCalls(t *testing.T, got, want []writerExec) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Exec count=%d want %d", len(got), len(want))
	}
	for i := range want {
		if strings.Join(strings.Fields(got[i].sql), " ") != strings.Join(strings.Fields(want[i].sql), " ") {
			t.Errorf("Exec %d SQL mismatch\ngot:  %s\nwant: %s", i+1, got[i].sql, want[i].sql)
		}
		if !reflect.DeepEqual(got[i].args, want[i].args) {
			t.Errorf("Exec %d args mismatch\ngot:  %#v\nwant: %#v", i+1, got[i].args, want[i].args)
		}
	}
}

func TestInsertEarlyRentalInitialUsesExpectedSQLAndArguments(t *testing.T) {
	r := writerFixture(t)
	before := r
	before.ContractDigest = append([]byte(nil), r.ContractDigest...)
	before.Offer.IdentityConfirmed = make(map[string]time.Time, len(r.Offer.IdentityConfirmed))
	for actor, at := range r.Offer.IdentityConfirmed {
		before.Offer.IdentityConfirmed[actor] = at
	}
	before.Offer.ContractAccepted = make(map[string]time.Time, len(r.Offer.ContractAccepted))
	for actor, at := range r.Offer.ContractAccepted {
		before.Offer.ContractAccepted[actor] = at
	}
	want := writerWant(before)
	f := &writerFakeTx{t: t, affected: []int{1, 1, 1}}
	if err := insertEarlyRentalInitial(context.Background(), f, r); err != nil {
		t.Fatal(err)
	}
	compareWriterCalls(t, f.calls, want)
	if !reflect.DeepEqual(r, before) {
		t.Fatal("writer mutated input record")
	}
}
func TestInsertEarlyRentalInitialPreservesFailuresAndStops(t *testing.T) {
	for failAt := 1; failAt <= 3; failAt++ {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			sentinel := fmt.Errorf("exec %d sentinel", failAt)
			f := &writerFakeTx{t: t, failAt: failAt, failErr: sentinel, affected: []int{1, 1, 1}}
			err := insertEarlyRentalInitial(context.Background(), f, writerFixture(t))
			if !errors.Is(err, sentinel) {
				t.Fatalf("error=%v does not preserve sentinel", err)
			}
			if len(f.calls) != failAt {
				t.Fatalf("calls=%d want %d", len(f.calls), failAt)
			}
		})
	}
}
func TestInsertEarlyRentalInitialRejectsAffectedRowsForEveryStatement(t *testing.T) {
	for stmt := 1; stmt <= 3; stmt++ {
		for _, rows := range []int{0, 2} {
			t.Run(fmt.Sprintf("statement_%d_rows_%d", stmt, rows), func(t *testing.T) {
				affected := []int{1, 1, 1}
				affected[stmt-1] = rows
				f := &writerFakeTx{t: t, affected: affected}
				if err := insertEarlyRentalInitial(context.Background(), f, writerFixture(t)); err == nil {
					t.Fatal("expected affected-row rejection")
				}
				if len(f.calls) != stmt {
					t.Fatalf("calls=%d want %d", len(f.calls), stmt)
				}
			})
		}
	}
}
func TestInsertEarlyRentalInitialCancellationBoundaries(t *testing.T) {
	for cancelAt := 0; cancelAt <= 3; cancelAt++ {
		t.Run(fmt.Sprintf("after_%d", cancelAt), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &writerFakeTx{t: t, affected: []int{1, 1, 1}, cancel: cancel, cancelAt: cancelAt}
			if cancelAt == 0 {
				cancel()
			}
			err := insertEarlyRentalInitial(ctx, f, writerFixture(t))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v want context.Canceled", err)
			}
			if len(f.calls) != cancelAt {
				t.Fatalf("calls=%d want %d", len(f.calls), cancelAt)
			}
		})
	}
}
func TestInsertEarlyRentalInitialRejectsInvalidInputsBeforeExec(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		f := &writerFakeTx{t: t}
		if !errors.Is(insertEarlyRentalInitial(nil, f, writerFixture(t)), earlyrental.ErrInvalid) || len(f.calls) != 0 {
			t.Fatalf("calls=%d", len(f.calls))
		}
	})
	t.Run("nil tx", func(t *testing.T) {
		if !errors.Is(insertEarlyRentalInitial(context.Background(), nil, writerFixture(t)), earlyrental.ErrInvalid) {
			t.Fatal("want ErrInvalid")
		}
	})
	for _, field := range []string{"created", "updated", "digest", "offer_digest", "event"} {
		t.Run(field, func(t *testing.T) {
			r := writerFixture(t)
			switch field {
			case "created":
				r.CreatedAt = r.CreatedAt.Add(time.Second)
			case "updated":
				r.UpdatedAt = r.UpdatedAt.Add(time.Second)
			case "digest":
				r.ContractDigest[0] ^= 1
			case "offer_digest":
				r.Offer.ContractDigest = "0000000000000000000000000000000000000000000000000000000000000000"
			case "event":
				r.InitialEvent.ActorID = writerHostID
			}
			f := &writerFakeTx{t: t}
			if !errors.Is(insertEarlyRentalInitial(context.Background(), f, r), earlyrental.ErrInvalid) || len(f.calls) != 0 {
				t.Fatalf("calls=%d", len(f.calls))
			}
		})
	}
}
