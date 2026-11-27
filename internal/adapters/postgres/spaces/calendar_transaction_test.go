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

const calendarOwnerID = "00000000-0000-4000-8000-000000000088"

type calendarTestPool struct {
	availabilityPool
	tx *calendarTestTx
}

func (p calendarTestPool) Begin(context.Context) (pgx.Tx, error) { return p.tx, nil }

type calendarTestTx struct {
	pgx.Tx
	current, next                   time.Time
	writes, commits, bumps          int
	ruleErr, streamErr, errorAtBump error
	disabled                        bool
	notFound                        bool
}

func (tx *calendarTestTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	return monthReadRow(func(d ...any) error {
		switch {
		case strings.Contains(q, "FROM public.usuario"):
			*d[0].(*string) = "activo"
		case strings.Contains(q, "bloqueo_cuenta_administrativo_local"):
			*d[0].(*bool) = false
		case strings.Contains(q, "kyb_space_eligibility"):
			*d[0].(*bool) = !tx.disabled
		case strings.Contains(q, "UPDATE public.espacio"):
			tx.bumps++
			tx.writes++
			if !strings.Contains(q, "GREATEST") || !strings.Contains(q, "RETURNING actualizado_en") {
				return errors.New("revision must be renewed monotonically")
			}
			if tx.errorAtBump != nil {
				return tx.errorAtBump
			}
			*d[0].(*time.Time) = tx.next
		case strings.Contains(q, "FROM public.espacio_regla_disponibilidad"):
			if tx.ruleErr != nil {
				return tx.ruleErr
			}
			return pgx.ErrNoRows
		case strings.Contains(q, "FROM public.espacio_regla_mensual"):
			return pgx.ErrNoRows
		case strings.Contains(q, "FROM public.espacio"):
			if len(args) != 2 || args[0] != calendarOwnerID || args[1] != monthReadSpaceID {
				return errors.New("owner scope missing")
			}
			if !strings.Contains(q, "FOR UPDATE") && !strings.Contains(q, "FOR SHARE") {
				return errors.New("space lock missing")
			}
			if tx.notFound {
				return pgx.ErrNoRows
			}
			zone := "America/Santiago"
			*d[0].(**string) = &zone
			if len(d) == 4 {
				*d[1].(*string) = "activa"
				*d[2].(*int64) = 35000
				*d[3].(*time.Time) = tx.current
			} else {
				*d[1].(*int) = 35000
				*d[2].(*time.Time) = tx.current
			}
		default:
			return errors.New("unexpected calendar query")
		}
		return nil
	})
}
func (tx *calendarTestTx) Exec(_ context.Context, q string, _ ...any) (pgconn.CommandTag, error) {
	if !strings.Contains(q, "pg_advisory_xact_lock") {
		tx.writes++
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *calendarTestTx) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	if !strings.Contains(q, "FROM public.espacio_calendario_override") || len(args) != 1 || args[0] != monthReadSpaceID {
		return nil, errors.New("unscoped overrides")
	}
	rate := 45000
	return &monthReadRows{streamErr: tx.streamErr, rows: []monthReadRow{func(d ...any) error {
		*d[0].(*string) = "2027-01-02"
		*d[1].(*string) = "disponible"
		*d[2].(**int) = &rate
		*d[3].(*string) = "private owner note"
		return nil
	}}}, nil
}
func (tx *calendarTestTx) Commit(context.Context) error   { tx.commits++; return nil }
func (tx *calendarTestTx) Rollback(context.Context) error { return nil }
func calendarTransactionFixture() (*Repository, *calendarTestTx) {
	tx := &calendarTestTx{current: time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC)}
	tx.next = tx.current.Add(time.Microsecond)
	return &Repository{pool: calendarTestPool{tx: tx}}, tx
}
func TestCalendarUpdateRejectsStaleBeforeAnyMutation(t *testing.T) {
	for _, expected := range []time.Time{{}, time.Date(2026, 11, 15, 17, 0, 0, 0, time.UTC)} {
		r, tx := calendarTransactionFixture()
		_, _, err := r.UpdateAvailability(context.Background(), calendarOwnerID, monthReadSpaceID, spaces.AvailabilityUpdateInput{ExpectedUpdatedAt: expected})
		if !errors.Is(err, spaces.ErrPreconditionFailed) || tx.writes != 0 || tx.commits != 0 {
			t.Fatalf("stale accepted: err=%v writes=%d commits=%d", err, tx.writes, tx.commits)
		}
	}
}
func TestCalendarUpdateBlocksDisabledCommercialProfile(t *testing.T) {
	r, tx := calendarTransactionFixture()
	tx.disabled = true
	_, _, err := r.UpdateAvailability(context.Background(), calendarOwnerID, monthReadSpaceID, spaces.AvailabilityUpdateInput{ExpectedUpdatedAt: tx.current})
	if !errors.Is(err, spaces.ErrCommercialProfileDisabled) || tx.writes != 0 || tx.commits != 0 {
		t.Fatalf("disabled profile wrote calendar: err=%v writes=%d commits=%d", err, tx.writes, tx.commits)
	}
}
func TestCalendarOverrideOnlyRenewsSharedRevision(t *testing.T) {
	r, tx := calendarTransactionFixture()
	rule, _, err := r.UpdateAvailability(context.Background(), calendarOwnerID, monthReadSpaceID, spaces.AvailabilityUpdateInput{ExpectedUpdatedAt: tx.current, Overrides: []spaces.CalendarOverrideInput{{Fecha: "2027-01-02", Estado: "bloqueado"}}})
	if err != nil || !rule.ActualizadoEn.Equal(tx.next) || tx.bumps != 1 || tx.commits != 1 {
		t.Fatalf("shared revision not renewed: err=%v revision=%s bumps=%d commits=%d", err, rule.ActualizadoEn, tx.bumps, tx.commits)
	}
}
func TestCalendarUpdateReadErrorRollsBack(t *testing.T) {
	r, tx := calendarTransactionFixture()
	tx.ruleErr = errors.New("rule read failed")
	_, _, err := r.UpdateAvailability(context.Background(), calendarOwnerID, monthReadSpaceID, spaces.AvailabilityUpdateInput{ExpectedUpdatedAt: tx.current})
	if !errors.Is(err, tx.ruleErr) || tx.commits != 0 {
		t.Fatalf("read failure committed: err=%v commits=%d", err, tx.commits)
	}
}
func TestCalendarUpdateBumpErrorRollsBack(t *testing.T) {
	r, tx := calendarTransactionFixture()
	tx.errorAtBump = errors.New("revision write failed")
	_, _, err := r.UpdateAvailability(context.Background(), calendarOwnerID, monthReadSpaceID, spaces.AvailabilityUpdateInput{ExpectedUpdatedAt: tx.current})
	if !errors.Is(err, tx.errorAtBump) || tx.commits != 0 {
		t.Fatalf("failed revision committed: err=%v commits=%d", err, tx.commits)
	}
}
func TestCalendarConfigurationReadsDefaultsAndPrivateOverride(t *testing.T) {
	r, tx := calendarTransactionFixture()
	out, err := r.GetCalendarConfiguration(context.Background(), calendarOwnerID, monthReadSpaceID)
	if err != nil || out.Rule.TarifaBaseDia != 35000 || len(out.Overrides) != 1 || *out.Overrides[0].TarifaPersonalizadaCLP != 45000 || !out.UpdatedAt.Equal(tx.current) || tx.writes != 0 || tx.commits != 1 {
		t.Fatalf("bad configuration: %+v err=%v writes=%d commits=%d", out, err, tx.writes, tx.commits)
	}
}
func TestCalendarConfigurationFailureNeverReturnsPartial(t *testing.T) {
	for _, scenario := range []string{"missing", "rule", "stream"} {
		t.Run(scenario, func(t *testing.T) {
			r, tx := calendarTransactionFixture()
			switch scenario {
			case "missing":
				tx.notFound = true
			case "rule":
				tx.ruleErr = errors.New("rule failure")
			case "stream":
				tx.streamErr = errors.New("stream failure")
			}
			out, err := r.GetCalendarConfiguration(context.Background(), calendarOwnerID, monthReadSpaceID)
			if err == nil || out.SpaceID != "" || tx.commits != 0 || tx.writes != 0 {
				t.Fatalf("partial/error accepted: %+v err=%v", out, err)
			}
		})
	}
}
