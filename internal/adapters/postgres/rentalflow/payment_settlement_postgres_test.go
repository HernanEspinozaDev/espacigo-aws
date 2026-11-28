package rentalflowpg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPaymentSettlementPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	switch cfg.ConnConfig.Host {
	case "127.0.0.1", "localhost", "::1":
	default:
		t.Skip("requires loopback PostgreSQL")
	}
	ctx := t.Context()
	admin, err := pgx.ConnectConfig(ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := "test_payment_settlement_" + uuid.NewString()[:8]
	db := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+db); err != nil {
			t.Error(err)
		}
	})
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `
CREATE TABLE public.checkout_preferencia_comercial (
  id uuid PRIMARY KEY, host_id uuid, provider_payment_id text, estado text, checkout_method text,
  test_mode boolean, total_charged_clp bigint, marketplace_fee_clp bigint
);
CREATE TABLE public.mercadopago_seller_connection (
  anfitrion_id uuid PRIMARY KEY, seller_id text, connection_state text, live_mode boolean
);`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../../db/legacy-migrations/v29/V000006__conciliacion_financiera_mercado_pago.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	id, host := uuid.NewString(), uuid.NewString()
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO public.checkout_preferencia_comercial VALUES($1,$2,'42','approved','checkout_pro',false,1036,72)`, id, host)
	batch.Queue(`INSERT INTO public.mercadopago_seller_connection VALUES($1,'91','active',true)`, host)
	if err = pool.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(pool, pool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := repo.ClaimSettlementCandidates(ctx)
	if err != nil || len(candidates) != 1 || candidates[0].OperationID != id {
		t.Fatalf("pending candidates: %v, %v", candidates, err)
	}
	if claimed, err := repo.ClaimSettlementCandidates(ctx); err != nil || len(claimed) != 0 {
		t.Fatalf("already claimed: %v, %v", claimed, err)
	}
	now := time.Now().UTC()
	net, fee, marketplace := int64(925), int64(39), int64(72)
	p := checkoutpro.VerifiedPayment{
		OperationID: id, ExternalReference: id, ProviderPaymentID: "42", SellerID: "90", Status: "approved",
		TransactionAmountCLP: 1036, Currency: "CLP", LiveMode: true, PaymentUpdatedAt: now,
		Settlement: &checkoutpro.Settlement{NetCLP: &net, ProcessingFeeCLP: &fee, MarketplaceCLP: &marketplace, ProviderUpdated: now, ReleaseStatus: "released"},
	}
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wrong seller should fail the binding: %v", err)
	}
	p.SellerID = "91"
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); err != nil {
		t.Fatal(err)
	}
	p.PaymentUpdatedAt = now.Add(-time.Minute)
	p.Settlement.ProviderUpdated = p.PaymentUpdatedAt
	stale := int64(900)
	p.Settlement.NetCLP = &stale
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); err != nil {
		t.Fatal(err)
	}
	var reported *checkoutpro.Settlement
	if err = pool.QueryRow(ctx, `SELECT provider_settlement FROM public.checkout_preferencia_comercial WHERE id=$1`, id).Scan(&reported); err != nil {
		t.Fatal(err)
	}
	if reported == nil || reported.NetCLP == nil || *reported.NetCLP != 925 || reported.Reconciliation != "reconciled" || reported.ReleaseStatus != "released" {
		t.Fatalf("unexpected persisted settlement: %+v", reported)
	}
	if reported.SyncedAt == nil || reported.PaymentStatus != "approved" {
		t.Fatalf("expected source readback and payment status: %+v", reported)
	}

	p.PaymentUpdatedAt = now.Add(time.Minute)
	p.Settlement = &checkoutpro.Settlement{
		ProviderUpdated: p.PaymentUpdatedAt,
		ReleaseStatus:   "pending",
	}
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT provider_settlement FROM public.checkout_preferencia_comercial WHERE id=$1`, id).Scan(&reported); err != nil {
		t.Fatal(err)
	}
	if reported.ReleaseStatus != "pending" || reported.Reconciliation != "reconciled" || reported.NetCLP == nil || *reported.NetCLP != 925 {
		t.Fatalf("expected updated release status and preserved figures: %+v", reported)
	}

	p.PaymentUpdatedAt = now.Add(2 * time.Minute)
	refund := int64(500)
	p.Settlement = &checkoutpro.Settlement{
		ProviderUpdated: p.PaymentUpdatedAt,
		RefundedCLP:     &refund,
	}
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT provider_settlement FROM public.checkout_preferencia_comercial WHERE id=$1`, id).Scan(&reported); err != nil {
		t.Fatal(err)
	}
	if reported.Reconciliation != "discrepancy" || reported.RefundedCLP == nil || *reported.RefundedCLP != 500 ||
		reported.NetCLP == nil || *reported.NetCLP != 925 {
		t.Fatalf("expected explicit partial refund and last reported net: %+v", reported)
	}

	p.Status = "charged_back"
	p.PaymentUpdatedAt = now.Add(3 * time.Minute)
	p.Settlement = &checkoutpro.Settlement{ProviderUpdated: p.PaymentUpdatedAt}
	if err = repo.SyncPaymentSettlement(ctx, candidates[0], p); err != nil {
		t.Fatal(err)
	}
	var checkoutStatus string
	if err = pool.QueryRow(ctx, `SELECT provider_settlement,estado FROM public.checkout_preferencia_comercial WHERE id=$1`, id).Scan(&reported, &checkoutStatus); err != nil {
		t.Fatal(err)
	}
	if reported.PaymentStatus != "charged_back" || reported.Reconciliation != "discrepancy" || checkoutStatus != "approved" {
		t.Fatalf("unexpected chargeback projection or checkout mutation: %+v, %s", reported, checkoutStatus)
	}
}
