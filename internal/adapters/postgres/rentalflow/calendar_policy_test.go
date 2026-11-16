package rentalflowpg

import (
	"context"
	"errors"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
	"time"
)

type policyGuardRow func(...any) error

func (r policyGuardRow) Scan(v ...any) error { return r(v...) }

type policyGuardRows struct {
	pgx.Rows
	date, state string
	used        bool
	streamErr   error
}

func (r *policyGuardRows) Next() bool {
	if r.used || r.date == "" {
		return false
	}
	r.used = true
	return true
}
func (r *policyGuardRows) Scan(v ...any) error {
	*v[0].(*string) = r.date
	*v[1].(*string) = r.state
	return nil
}
func (r *policyGuardRows) Err() error { return r.streamErr }
func (r *policyGuardRows) Close()     {}

type policyGuardDB struct {
	mockDB
	days                             []int16
	open, close, overrideDate, state string
	queryErr, streamErr, ruleErr     error
	writes                           int
}

func (d *policyGuardDB) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	if strings.Contains(q, "FROM public.espacio_regla_disponibilidad") {
		return policyGuardRow(func(v ...any) error {
			if d.ruleErr != nil {
				return d.ruleErr
			}
			*v[0].(*[]int16) = d.days
			*v[1].(*string) = d.open
			*v[2].(*string) = d.close
			*v[3].(*int) = 1
			*v[4].(*int) = 30
			return nil
		})
	}
	if strings.Contains(q, "WITH candidates AS") {
		return &mockRow{vals: []any{args[1], args[2]}}
	}
	if strings.Contains(q, "espacio_horario_semanal") {
		return &mockRow{vals: []any{[]byte(`{"enabled":false,"days":[]}`)}}
	}
	return &mockRow{vals: []any{true}}
}
func (d *policyGuardDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	return &policyGuardRows{date: d.overrideDate, state: d.state, streamErr: d.streamErr}, nil
}
func (d *policyGuardDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	d.writes++
	return pgconn.CommandTag{}, errors.New("reader attempted a write")
}
func policyGuardFixture(t *testing.T, unit string) (*Repository, *policyGuardDB, rentalflow.Listing) {
	t.Helper()
	db := &policyGuardDB{days: []int16{1, 2, 3, 4, 5, 6, 7}, open: "00:00", close: "24:00"}
	r, e := NewRepository(db, nil, func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) })
	if e != nil {
		t.Fatal(e)
	}
	return r, db, rentalflow.Listing{ID: "00000000-0000-4000-8000-000000000099", TimeZone: "UTC", RateUnit: unit}
}
func TestIntervalAvailableRejectsCalendarBlockInsideRange(t *testing.T) {
	r, db, l := policyGuardFixture(t, "dia")
	db.overrideDate = "2027-01-03"
	db.state = "bloqueado"
	got, e := r.IntervalAvailable(context.Background(), l, rentalflow.Interval{StartAt: time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)})
	if e != nil || got || db.writes != 0 {
		t.Fatalf("blocked range available=%v err=%v writes=%d", got, e, db.writes)
	}
}
func TestIntervalAvailableRejectsHourOutsideBaseRule(t *testing.T) {
	r, db, l := policyGuardFixture(t, "hora")
	db.open = "10:00"
	db.close = "12:00"
	got, e := r.IntervalAvailable(context.Background(), l, rentalflow.Interval{StartAt: time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC), EndAt: time.Date(2027, 1, 2, 10, 0, 0, 0, time.UTC)})
	if e != nil || got {
		t.Fatalf("outside operating time available=%v err=%v", got, e)
	}
}
func TestIntervalAvailablePropagatesCalendarReadErrors(t *testing.T) {
	for _, stream := range []bool{false, true} {
		r, db, l := policyGuardFixture(t, "dia")
		sentinel := errors.New("calendar dependency failed")
		if stream {
			db.streamErr = sentinel
		} else {
			db.queryErr = sentinel
		}
		got, e := r.IntervalAvailable(context.Background(), l, rentalflow.Interval{StartAt: time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2027, 1, 3, 0, 0, 0, 0, time.UTC)})
		if got || !errors.Is(e, sentinel) {
			t.Fatalf("partial calendar accepted: available=%v err=%v", got, e)
		}
	}
}
func TestPublicIntervalsApplyCalendarOverride(t *testing.T) {
	r, db, l := policyGuardFixture(t, "dia")
	db.overrideDate = "2027-01-02"
	db.state = "mantenimiento"
	got, e := r.PublicIntervals(context.Background(), l, "2027-01-02", 1)
	if e != nil || len(got) != 0 || db.writes != 0 {
		t.Fatalf("blocked public intervals=%v err=%v", got, e)
	}
}
func TestAvailableOverrideOpensClosedOperatingDay(t *testing.T) {
	r, db, l := policyGuardFixture(t, "dia")
	db.days = []int16{1}
	db.overrideDate = "2027-01-03"
	db.state = "disponible"
	got, e := r.IntervalAvailable(context.Background(), l, rentalflow.Interval{StartAt: time.Date(2027, 1, 3, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)})
	if e != nil || !got || db.writes != 0 {
		t.Fatalf("explicit available override rejected=%v err=%v", got, e)
	}
}
