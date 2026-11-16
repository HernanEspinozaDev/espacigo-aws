package spacespg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
)

type monthReadRow func(...any) error

func (f monthReadRow) Scan(dest ...any) error { return f(dest...) }

type monthReadRows struct {
	pgx.Rows
	rows               []monthReadRow
	index              int
	scanErr, streamErr error
	closed             bool
}

func (f *monthReadRows) Next() bool { f.index++; return f.index <= len(f.rows) }
func (f *monthReadRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	return f.rows[f.index-1](dest...)
}
func (f *monthReadRows) Err() error { return f.streamErr }
func (f *monthReadRows) Close()     { f.closed = true }

type monthReadPool struct {
	state, zone                                  string
	ruleErr, overrideQueryErr, occupancyQueryErr error
	overrides, occupancies                       monthReadRows
	reads, begins                                int
	occupancyStart                               time.Time
}

func (f *monthReadPool) Begin(context.Context) (pgx.Tx, error) {
	f.begins++
	return nil, errors.New("monthly read must not start a transaction")
}
func (f *monthReadPool) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.reads++
	if len(args) != 1 || args[0] != monthReadSpaceID {
		panic("wrong monthly QueryRow resource")
	}
	if strings.Contains(query, "FROM public.espacio_regla_disponibilidad") {
		return monthReadRow(func(...any) error { return f.ruleErr })
	}
	if strings.Contains(query, "FROM public.espacio\n") {
		return monthReadRow(func(dest ...any) error {
			if len(dest) != 4 {
				panic("unexpected listing scan")
			}
			*dest[0].(*string) = f.state
			*dest[1].(**string) = &f.zone
			*dest[2].(*string) = "dia"
			*dest[3].(*int64) = 35000
			return nil
		})
	}
	panic("unexpected monthly QueryRow")
}
func (f *monthReadPool) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	f.reads++
	if len(args) != 3 || args[0] != monthReadSpaceID {
		panic("wrong monthly Query resource")
	}
	if strings.Contains(query, "FROM public.espacio_calendario_override") {
		if f.overrideQueryErr != nil {
			return nil, f.overrideQueryErr
		}
		return &f.overrides, nil
	}
	if strings.Contains(query, "FROM public.ocupacion") {
		if f.occupancyQueryErr != nil {
			return nil, f.occupancyQueryErr
		}
		f.occupancyStart = args[1].(time.Time)
		return &f.occupancies, nil
	}
	panic("unexpected monthly Query")
}

const monthReadSpaceID = "00000000-0000-4000-8000-000000000099"

func newMonthReadPool() *monthReadPool {
	return &monthReadPool{state: "activa", zone: "America/Santiago", ruleErr: pgx.ErrNoRows}
}
func monthReadFuture() string { return time.Now().UTC().AddDate(0, 2, 0).Format("2006-01") }

func TestMonthlyReaderDefaultAndEffectiveOverrideStayPublic(t *testing.T) {
	pool := newMonthReadPool()
	month := monthReadFuture()
	price := 48000
	pool.overrides.rows = []monthReadRow{func(dest ...any) error {
		*dest[0].(*string) = month + "-17"
		*dest[1].(*string) = "disponible"
		*dest[2].(**int) = &price
		*dest[3].(*string) = "private override reason"
		return nil
	}}
	repo := &Repository{pool: pool}
	got, err := repo.GetMonthlyAvailability(context.Background(), monthReadSpaceID, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Days) < 28 || got.Days[16].PriceCLP != price || got.Days[16].Status != "AVAILABLE" {
		t.Fatalf("default or override did not reach the real monthly projection: %+v", got)
	}
	if pool.occupancyStart.Location().String() != pool.zone || pool.occupancyStart.Day() != 1 || pool.occupancyStart.Hour() != 0 {
		t.Fatalf("occupancy month must start at civil midnight in listing zone: %v", pool.occupancyStart)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private override reason", "propietario_id", "motivo", monthReadSpaceID} {
		if strings.Contains(string(body), private) {
			t.Fatalf("private field reached monthly JSON: %s", private)
		}
	}
	if pool.begins != 0 || !pool.overrides.closed || !pool.occupancies.closed {
		t.Fatal("reader did not remain read-only or close its streams")
	}
}

func TestMonthlyReaderRejectsUnpublishedAndInvalidZone(t *testing.T) {
	for _, state := range []string{"borrador", "oculta", "eliminada"} {
		t.Run(state, func(t *testing.T) {
			pool := newMonthReadPool()
			pool.state = state
			got, err := (&Repository{pool: pool}).GetMonthlyAvailability(context.Background(), monthReadSpaceID, monthReadFuture())
			if !errors.Is(err, spaces.ErrNotFound) || got.Days != nil || pool.reads != 1 {
				t.Fatalf("unpublished read = %+v, %v, reads=%d", got, err, pool.reads)
			}
		})
	}
	pool := newMonthReadPool()
	pool.zone = "Invalid/Zone"
	got, err := (&Repository{pool: pool}).GetMonthlyAvailability(context.Background(), monthReadSpaceID, monthReadFuture())
	if err == nil || got.Days != nil || pool.reads != 1 {
		t.Fatal("invalid IANA zone must not fall back to a fixed offset")
	}
}

func TestMonthlyReaderWithholdsPartialResultsOnDependencyErrors(t *testing.T) {
	sentinel := errors.New("injected monthly read error")
	for _, tc := range []struct {
		name   string
		inject func(*monthReadPool)
	}{
		{"rule", func(p *monthReadPool) { p.ruleErr = sentinel }},
		{"override query", func(p *monthReadPool) { p.overrideQueryErr = sentinel }},
		{"override scan", func(p *monthReadPool) { p.overrides.rows = []monthReadRow{nil}; p.overrides.scanErr = sentinel }},
		{"override stream", func(p *monthReadPool) { p.overrides.streamErr = sentinel }},
		{"occupancy query", func(p *monthReadPool) { p.occupancyQueryErr = sentinel }},
		{"occupancy scan", func(p *monthReadPool) { p.occupancies.rows = []monthReadRow{nil}; p.occupancies.scanErr = sentinel }},
		{"occupancy stream", func(p *monthReadPool) { p.occupancies.streamErr = sentinel }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := newMonthReadPool()
			tc.inject(pool)
			got, err := (&Repository{pool: pool}).GetMonthlyAvailability(context.Background(), monthReadSpaceID, monthReadFuture())
			if !errors.Is(err, sentinel) || got.Days != nil || pool.begins != 0 {
				t.Fatalf("partial availability escaped dependency error: days=%d err=%v begins=%d", len(got.Days), err, pool.begins)
			}
		})
	}
}
