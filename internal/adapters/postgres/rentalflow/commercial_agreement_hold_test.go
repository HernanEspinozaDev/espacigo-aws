package rentalflowpg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type agreementHoldTx struct {
	pgx.Tx
	input              CommercialAgreementHoldInput
	current, available bool
	prior              *CommercialAgreementHold
	priorActive        bool
	inserts            int
	calls              []string
	insertedExpiry     time.Time
}

func (f *agreementHoldTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, q)
	if strings.HasPrefix(q, "INSERT INTO public.ocupacion") {
		f.inserts++
		f.insertedExpiry = args[5].(time.Time)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (f *agreementHoldTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	f.calls = append(f.calls, q)
	switch {
	case strings.Contains(q, "FROM public.ocupacion WHERE solicitud_comercial_id"):
		if f.prior == nil {
			return &mockRow{err: pgx.ErrNoRows}
		}
		return &mockRow{vals: []any{f.prior.OccupancyID, f.input.Quote.SpaceID, f.input.Quote.StartAt, f.input.Quote.EndAt, f.priorActive, f.prior.ExpiresAt}}
	case strings.Contains(q, "SELECT EXISTS(SELECT 1 FROM public.espacio"):
		return &mockRow{vals: []any{f.current}}
	case strings.Contains(q, "FOR UPDATE"):
		return &mockRow{vals: []any{f.input.Quote.SpaceID}}
	case strings.Contains(q, "SELECT COALESCE((SELECT jsonb_build_object"):
		return &mockRow{vals: []any{[]byte(`{"enabled":false,"days":[]}`)}}
	case strings.Contains(q, "WITH candidates AS"):
		starts, ends := []time.Time{}, []time.Time{}
		if f.available {
			starts = append(starts, f.input.Quote.StartAt)
			ends = append(ends, f.input.Quote.EndAt)
		}
		return &mockRow{vals: []any{starts, ends}}
	case strings.Contains(q, "SELECT dias_operacion"):
		return &mockRow{err: pgx.ErrNoRows}
	default:
		panic("unexpected agreement hold QueryRow: " + q)
	}
}
func (f *agreementHoldTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return agreementEmptyRows{}, nil
}

type agreementEmptyRows struct{ pgx.Rows }

func (agreementEmptyRows) Next() bool                     { return false }
func (agreementEmptyRows) Close()                         {}
func (agreementEmptyRows) Err() error                     { return nil }
func (f *agreementHoldTx) Commit(context.Context) error   { panic("helper must not commit") }
func (f *agreementHoldTx) Rollback(context.Context) error { panic("helper must not rollback") }

func agreementInput(now time.Time) CommercialAgreementHoldInput {
	start := time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, time.UTC)
	return CommercialAgreementHoldInput{RequestID: "10000000-0000-4000-8000-000000000001", LocationRevision: 1, Quote: rentalflow.Quote{ID: "20000000-0000-4000-8000-000000000002", SpaceID: "30000000-0000-4000-8000-000000000003", OwnerID: "40000000-0000-4000-8000-000000000004", RenterID: "50000000-0000-4000-8000-000000000005", RateVersion: 1, RateUnit: "hora", UnitPriceCLP: 1000, Units: 1, SubtotalCLP: 1000, Currency: "CLP", TimeZone: "UTC", StartAt: start, EndAt: start.Add(time.Hour), ExpiresAt: now.Add(-time.Hour)}}
}
func TestAgreementHoldRevalidatesAndCapsPaymentWindow(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{now, now.Add(115 * time.Minute)} {
		input := agreementInput(at)
		tx := &agreementHoldTx{input: input, current: true, available: true}
		result, err := RetainCommercialAgreementInterval(context.Background(), tx, input, at)
		want := at.Add(15 * time.Minute)
		if input.Quote.StartAt.Before(want) {
			want = input.Quote.StartAt
		}
		if err != nil || result.OccupancyID == "" || !result.ExpiresAt.Equal(want) || tx.inserts != 1 || result.Replayed {
			t.Fatalf("hold=%+v err=%v inserts=%d", result, err, tx.inserts)
		}
		if !strings.Contains(tx.calls[0], "checkout-quote:") || !strings.Contains(tx.calls[1], "hashtextextended($1, 0)") || !strings.Contains(tx.calls[2], "FOR UPDATE") {
			t.Fatal("incorrect common lock order")
		}
	}
}
func TestAgreementHoldReplayCannotExtendOrResurrectInterval(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	input := agreementInput(now)
	prior := &CommercialAgreementHold{OccupancyID: "60000000-0000-4000-8000-000000000006", ExpiresAt: now.Add(3 * time.Minute)}
	tx := &agreementHoldTx{input: input, prior: prior, priorActive: true}
	result, err := RetainCommercialAgreementInterval(context.Background(), tx, input, now)
	if err != nil || !result.Replayed || result.ExpiresAt != prior.ExpiresAt || tx.inserts != 0 {
		t.Fatalf("replay=%+v err=%v", result, err)
	}
	for _, at := range []time.Time{now, prior.ExpiresAt} {
		tx.priorActive = false
		if _, err := RetainCommercialAgreementInterval(context.Background(), tx, input, at); !errors.Is(err, rentalflow.ErrConflict) {
			t.Fatalf("resurrected: %v", err)
		}
	}
}
func TestAgreementHoldRejectsChangedPublicationOrLostAvailability(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	input := agreementInput(now)
	for _, tx := range []*agreementHoldTx{{input: input, current: false, available: true}, {input: input, current: true, available: false}} {
		if _, err := RetainCommercialAgreementInterval(context.Background(), tx, input, now); !errors.Is(err, rentalflow.ErrConflict) || tx.inserts != 0 {
			t.Fatalf("accepted stale input: err=%v inserts=%d", err, tx.inserts)
		}
	}
}

func TestAgreementHoldExpiryOnlyReleasesDueRequestHoldWithoutDeleting(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	input := agreementInput(now)
	tx := &agreementHoldTx{input: input}
	if err := ExpireCommercialAgreementInterval(context.Background(), tx, input.RequestID, input.Quote.SpaceID, now); err != nil {
		t.Fatal(err)
	}
	if len(tx.calls) != 1 || !strings.Contains(tx.calls[0], "activo=false,desactivada_en=$3") || !strings.Contains(tx.calls[0], "solicitud_comercial_id=$1 AND espacio_id=$2") || !strings.Contains(tx.calls[0], "tipo='retencion_contrato_comercial'") || !strings.Contains(tx.calls[0], "expira_en<=$3") || strings.Contains(tx.calls[0], "DELETE") {
		t.Fatalf("unsafe release: %v", tx.calls)
	}
}
