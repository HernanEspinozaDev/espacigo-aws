package rentalflowpg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type contractCheckoutTx struct {
	staleAuthorization *commercialContractAuthorization
	selectedRequestID  string
	pgx.Tx
	quote       checkoutpro.Quote
	auth        commercialContractAuthorization
	prior       *checkoutpro.Operation
	fingerprint []byte
	opBinding   bool
	commits     int
	mutations   []string
}

func (f *contractCheckoutTx) Begin(context.Context) (pgx.Tx, error) { return f, nil }
func (f *contractCheckoutTx) Commit(context.Context) error          { f.commits++; return nil }
func (f *contractCheckoutTx) Rollback(context.Context) error        { return nil }
func (f *contractCheckoutTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	f.mutations = append(f.mutations, q)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (f *contractCheckoutTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return agreementEmptyRows{}, nil
}
func (f *contractCheckoutTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "FROM public.cotizacion_arriendo_comercial WHERE id="):
		x := f.quote
		return &mockRow{vals: []any{x.ID, x.SpaceID, x.HostID, x.RenterID, x.Title, x.RateVersion, x.RateUnit, x.Currency, x.UnitPriceCLP, x.Units, x.SubtotalCLP, x.StartAt, x.EndAt, x.TimeZone, x.ExpiresAt}}
	case strings.HasPrefix(q, "SELECT estado FROM public.usuario"):
		return &mockRow{vals: []any{"activo"}}
	case strings.HasPrefix(q, "SELECT id::text FROM public.espacio"):
		return &mockRow{vals: []any{f.quote.SpaceID}}
	case strings.HasPrefix(q, "SELECT COALESCE(solicitud_comercial_id::text,'') FROM"):
		if f.prior == nil {
			return &mockRow{err: pgx.ErrNoRows}
		}
		return &mockRow{vals: []any{f.auth.RequestID}}
	case strings.HasPrefix(q, "SELECT s.id::text"):
		a := f.auth
		if len(args) > 2 {
			if request, ok := args[2].(string); ok {
				f.selectedRequestID = request
			} else if f.staleAuthorization != nil {
				a = *f.staleAuthorization
			}
		}
		return &mockRow{vals: []any{a.RequestID, a.State, a.Version, a.Digest, a.ExpiresAt, a.OccupancyID, a.OccupancyType, a.Active, a.OccupancyExpiry, a.Bound, a.CurrentLocation}}
	case strings.HasPrefix(q, "SELECT id::text,cotizacion_id::text,estado,"):
		if f.prior == nil {
			return &mockRow{err: pgx.ErrNoRows}
		}
		o := f.prior
		return &mockRow{vals: []any{o.ID, o.QuoteID, o.Status, o.PreferenceID, o.InitPoint, o.ExpiresAt, o.UpdatedAt, f.fingerprint, o.Amounts.BaseCLP, o.Amounts.CommissionCLP, o.Amounts.VATCLP, o.Amounts.MarketplaceFeeCLP, o.TestMode, o.CheckoutMethod, o.ProviderPaymentID, f.auth.RequestID, f.auth.Version, f.auth.Digest}}
	case strings.HasPrefix(q, "SELECT c.cotizacion_id::text,EXISTS"):
		return &mockRow{vals: []any{f.quote.ID, f.auth.RequestID, f.opBinding}}
	case strings.HasPrefix(q, "SELECT solicitud_comercial_id="):
		return &mockRow{vals: []any{f.opBinding}}
	default:
		panic("unexpected contract checkout query: " + q)
	}
}
func contractCheckoutFixture(now time.Time) *contractCheckoutTx {
	source := agreementInput(now).Quote
	q := checkoutpro.Quote{ID: source.ID, SpaceID: source.SpaceID, HostID: source.OwnerID, RenterID: source.RenterID, Title: "Sala", RateVersion: source.RateVersion, RateUnit: source.RateUnit, Currency: source.Currency, UnitPriceCLP: source.UnitPriceCLP, Units: source.Units, SubtotalCLP: source.SubtotalCLP, StartAt: source.StartAt, EndAt: source.EndAt, TimeZone: source.TimeZone, ExpiresAt: source.ExpiresAt}
	expires := now.Add(15 * time.Minute)
	return &contractCheckoutTx{quote: q, opBinding: true, auth: commercialContractAuthorization{RequestID: "60000000-0000-4000-8000-000000000006", State: "ready_for_payment", Version: 1, Digest: make([]byte, 32), OccupancyID: "70000000-0000-4000-8000-000000000007", OccupancyType: "retencion_contrato_comercial", Active: true, Bound: true, CurrentLocation: true, ExpiresAt: expires, OccupancyExpiry: expires}}
}
func TestContractsGateConvertsOneSignedHoldForEveryPaymentEntry(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	for _, entry := range []string{"checkout", "wallet", "payments_api"} {
		t.Run(entry, func(t *testing.T) {
			tx := contractCheckoutFixture(now)
			r, _ := NewRepository(tx, tx, func() time.Time { return now })
			r.RequireCommercialContracts(true)
			var q checkoutpro.Quote
			var op checkoutpro.Operation
			var created bool
			var err error
			if entry == "payments_api" {
				q, op, created, err = r.BeginPayment(context.Background(), tx.quote.RenterID, tx.quote.ID, "key", "80000000-0000-4000-8000-000000000008", make([]byte, 32), now)
			} else {
				q, op, created, err = r.Begin(context.Background(), tx.quote.RenterID, tx.quote.ID, "key", "80000000-0000-4000-8000-000000000008", make([]byte, 32), true, now)
			}
			if err != nil || !created || tx.commits != 1 || !q.ExpiresAt.Equal(tx.auth.ExpiresAt) || !op.ExpiresAt.Equal(tx.auth.ExpiresAt) {
				t.Fatalf("created=%v q=%+v op=%+v err=%v", created, q, op, err)
			}
			converted, insertedHolds := 0, 0
			for _, query := range tx.mutations {
				if strings.Contains(query, "SET tipo='retencion_comercial'") {
					converted++
				}
				if strings.HasPrefix(query, "INSERT INTO public.ocupacion") {
					insertedHolds++
				}
			}
			if converted != 1 || insertedHolds != 0 {
				t.Fatalf("converted=%d duplicate holds=%d", converted, insertedHolds)
			}
		})
	}
}
func TestContractsGateBlocksMissingSignaturesChangedLocationOrExpiredHold(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	for _, breakRule := range []func(*commercialContractAuthorization){func(a *commercialContractAuthorization) { a.Bound = false }, func(a *commercialContractAuthorization) { a.State = "pending_landlord" }, func(a *commercialContractAuthorization) { a.CurrentLocation = false }, func(a *commercialContractAuthorization) { a.ExpiresAt = now }, func(a *commercialContractAuthorization) { a.Active = false }} {
		tx := contractCheckoutFixture(now)
		breakRule(&tx.auth)
		r, _ := NewRepository(tx, tx, func() time.Time { return now })
		r.RequireCommercialContracts(true)
		_, _, created, err := r.BeginPayment(context.Background(), tx.quote.RenterID, tx.quote.ID, "key", "80000000-0000-4000-8000-000000000008", make([]byte, 32), now)
		if !errors.Is(err, checkoutpro.ErrConflict) || created || tx.commits != 0 {
			t.Fatalf("gate bypass: created=%v err=%v", created, err)
		}
		for _, query := range tx.mutations {
			if strings.HasPrefix(query, "INSERT") {
				t.Fatal("mutated before gate")
			}
		}
	}
}
func TestContractsGateTerminalReplayUsesDurableBindingWithoutCreatingPayment(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	tx := contractCheckoutFixture(now)
	tx.auth.Active = false
	tx.auth.State = "expired"
	tx.auth.OccupancyType = "retencion_comercial"
	tx.fingerprint = make([]byte, 32)
	tx.prior = &checkoutpro.Operation{ID: "80000000-0000-4000-8000-000000000008", QuoteID: tx.quote.ID, Status: "rejected", CheckoutMethod: "payments_api", TestMode: true, ExpiresAt: tx.auth.ExpiresAt}
	r, _ := NewRepository(tx, tx, func() time.Time { return now })
	r.RequireCommercialContracts(true)
	_, op, created, err := r.BeginPayment(context.Background(), tx.quote.RenterID, tx.quote.ID, "key", "90000000-0000-4000-8000-000000000009", make([]byte, 32), now)
	if err != nil || created || op.ID != tx.prior.ID || tx.commits != 1 {
		t.Fatalf("op=%+v created=%v err=%v", op, created, err)
	}
	for _, query := range tx.mutations {
		if strings.HasPrefix(query, "INSERT") || strings.HasPrefix(query, "UPDATE") {
			t.Fatal("terminal replay mutated payment")
		}
	}
}
func TestQuotePurgeReferencesContractsOnlyWhenGateEnabled(t *testing.T) {
	r := &Repository{}
	query := "SELECT q.id FROM quote q WHERE true ORDER BY vence_en,id LIMIT $2"
	if got := r.contractQuotePurgeSQL(query); got != query {
		t.Fatal("legacy schema gained contract dependency")
	}
	r.RequireCommercialContracts(true)
	if got := r.contractQuotePurgeSQL(query); !strings.Contains(got, "contrato_solicitud_comercial") {
		t.Fatal("active contract quote not preserved")
	}
	if !preservedCommercialQuoteRace(&pgconn.PgError{Code: "23503"}) || preservedCommercialQuoteRace(errors.New("network")) {
		t.Fatal("unsafe purge error classification")
	}
}

func TestContractReplayLoadsItsExactRequestInsteadOfHistoricalQuoteRequest(t *testing.T) {
	now := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	tx := contractCheckoutFixture(now)
	stale := tx.auth
	stale.RequestID = "90000000-0000-4000-8000-000000000009"
	stale.State = "expired"
	stale.Active = false
	tx.staleAuthorization = &stale
	tx.auth.OccupancyType = "retencion_comercial"
	tx.fingerprint = make([]byte, 32)
	tx.prior = &checkoutpro.Operation{ID: "80000000-0000-4000-8000-000000000008", QuoteID: tx.quote.ID, Status: "pending", CheckoutMethod: "payments_api", TestMode: true, ExpiresAt: tx.auth.ExpiresAt}
	r, _ := NewRepository(tx, tx, func() time.Time { return now })
	r.RequireCommercialContracts(true)
	_, op, created, err := r.BeginPayment(context.Background(), tx.quote.RenterID, tx.quote.ID, "key", "90000000-0000-4000-8000-000000000009", make([]byte, 32), now)
	if err != nil || created || op.ID != tx.prior.ID || tx.selectedRequestID != tx.auth.RequestID {
		t.Fatalf("historical request confused replay: request=%s created=%v err=%v", tx.selectedRequestID, created, err)
	}
}
