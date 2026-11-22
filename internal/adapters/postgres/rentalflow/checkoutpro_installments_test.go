package rentalflowpg

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type installmentRow struct {
	value string
	err   error
}

func (r installmentRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if s, ok := dest[0].(*string); ok {
			*s = r.value
		}
	}
	return nil
}

type installmentExecCall struct {
	sql  string
	args []any
}

type testInstallmentTx struct {
	pgx.Tx
	modeValue string
	modeErr   error
	execCalls []installmentExecCall
}

func (tx *testInstallmentTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "FROM public.cotizacion_modalidad_cobro") {
		return installmentRow{value: tx.modeValue, err: tx.modeErr}
	}
	return installmentRow{err: pgx.ErrNoRows}
}

func (tx *testInstallmentTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execCalls = append(tx.execCalls, installmentExecCall{sql: sql, args: append([]any(nil), args...)})
	return pgconn.CommandTag{}, nil
}

func TestSyncMonthlyInstallmentsIgnoresNonMonthly(t *testing.T) {
	tx := &testInstallmentTx{modeValue: "monthly_installments"}
	repo := &Repository{}
	q := checkoutpro.Quote{RateUnit: "dia", UnitPriceCLP: 50000}
	loc, _ := time.LoadLocation("America/Santiago")

	err := repo.syncMonthlyInstallments(context.Background(), tx, "res-1", "quote-1", q, nil, loc, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(tx.execCalls) != 0 {
		t.Fatalf("execCalls = %d, want 0", len(tx.execCalls))
	}
}

func TestSyncMonthlyInstallmentsIgnoresFullBillingMode(t *testing.T) {
	tx := &testInstallmentTx{modeValue: "full"}
	repo := &Repository{}
	q := checkoutpro.Quote{RateUnit: "mes", UnitPriceCLP: 150000}
	loc, _ := time.LoadLocation("America/Santiago")

	err := repo.syncMonthlyInstallments(context.Background(), tx, "res-1", "quote-1", q, nil, loc, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(tx.execCalls) != 0 {
		t.Fatalf("execCalls = %d, want 0", len(tx.execCalls))
	}
}

func TestSyncMonthlyInstallmentsGeneratesAndPersistsSchedule(t *testing.T) {
	tx := &testInstallmentTx{modeValue: "monthly_installments"}
	idCounter := 0
	repo := &Repository{
		newID: func() string {
			idCounter++
			return "cuota-uuid"
		},
	}
	loc, _ := time.LoadLocation("America/Santiago")
	now := time.Date(2030, time.June, 1, 10, 0, 0, 0, time.UTC)
	start := time.Date(2030, time.July, 1, 0, 0, 0, 0, loc)
	end := time.Date(2030, time.October, 1, 0, 0, 0, 0, loc)

	q := checkoutpro.Quote{
		ID:           "quote-uuid",
		RateUnit:     "mes",
		UnitPriceCLP: 200000,
		Units:        3,
		StartAt:      start.UTC(),
		EndAt:        end.UTC(),
	}
	intervals := []rentalflow.QuoteInterval{
		{StartAt: start.UTC(), EndAt: end.UTC(), Units: 3},
	}

	reservaID := "reserva-uuid-123"
	err := repo.syncMonthlyInstallments(context.Background(), tx, reservaID, q.ID, q, intervals, loc, now)
	if err != nil {
		t.Fatalf("syncMonthlyInstallments failed: %v", err)
	}

	// 1 DELETE + 3 INSERTs = 4 calls
	if len(tx.execCalls) != 4 {
		t.Fatalf("execCalls = %d, want 4 (1 delete + 3 inserts)", len(tx.execCalls))
	}

	// Call 0: DELETE
	if !strings.Contains(tx.execCalls[0].sql, "DELETE FROM public.reserva_cuota_mensual") {
		t.Fatalf("call 0 sql = %q, want DELETE", tx.execCalls[0].sql)
	}
	if tx.execCalls[0].args[0] != reservaID {
		t.Fatalf("delete arg = %v, want %q", tx.execCalls[0].args[0], reservaID)
	}

	// Calls 1..3: INSERT cuotas 1..3
	for i := 1; i <= 3; i++ {
		call := tx.execCalls[i]
		if !strings.Contains(call.sql, "INSERT INTO public.reserva_cuota_mensual") {
			t.Fatalf("call %d sql = %q, want INSERT", i, call.sql)
		}
		// args: cuotaID, reservaID, cuotaNum, pStart, pEnd, UnitPriceCLP, now
		if call.args[1] != reservaID {
			t.Fatalf("call %d reservaID = %v, want %q", i, call.args[1], reservaID)
		}
		if call.args[2] != i {
			t.Fatalf("call %d cuotaNum = %v, want %d", i, call.args[2], i)
		}
		if call.args[5] != int64(200000) {
			t.Fatalf("call %d monto = %v, want 200000", i, call.args[5])
		}
		pStart := call.args[3].(time.Time)
		pEnd := call.args[4].(time.Time)
		if !pEnd.After(pStart) {
			t.Fatalf("call %d pEnd (%v) must be after pStart (%v)", i, pEnd, pStart)
		}
	}
}
