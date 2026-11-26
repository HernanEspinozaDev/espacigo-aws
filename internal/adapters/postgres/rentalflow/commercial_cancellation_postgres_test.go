package rentalflowpg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupCancellationTestDB(t *testing.T) (*pgxpool.Pool, *Repository) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	switch adminConfig.Host {
	case "127.0.0.1", "localhost", "::1":
	default:
		t.Skip("cancellation integration requires loopback PostgreSQL")
	}

	ctx := t.Context()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	dbName := "test_commercial_cancellation_" + uuid.NewString()[:8]
	identifier := pgx.Identifier{dbName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = dbName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DO $$BEGIN
		IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='espacigo_runtime') THEN
			CREATE ROLE espacigo_runtime;
		END IF;
	END$$;
	CREATE TABLE public.usuario(id uuid PRIMARY KEY);
	CREATE TABLE public.espacio(politica_cancelacion_version text);
	CREATE TABLE public.checkout_preferencia_comercial(
		id uuid PRIMARY KEY,
		cotizacion_id uuid,
		espacio_id uuid,
		arrendatario_id uuid,
		host_id uuid NOT NULL,
		estado text,
		checkout_method text NOT NULL DEFAULT 'checkout_pro',
		base_clp bigint NOT NULL DEFAULT 0,
		total_charged_clp bigint NOT NULL DEFAULT 0,
		marketplace_fee_clp bigint NOT NULL DEFAULT 0,
		test_mode boolean NOT NULL DEFAULT false,
		vence_en timestamptz,
		creada_en timestamptz,
		actualizada_en timestamptz,
		provider_payment_id text,
		provider_payment_status text,
		provider_payment_updated_at timestamptz,
		reconciliation_reason text,
		payment_verified_at timestamptz
	);
	CREATE TABLE public.mercadopago_seller_connection(anfitrion_id uuid PRIMARY KEY,seller_id text,connection_state text,live_mode boolean);
	CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY, subtotal_clp bigint, moneda text, politica_cancelacion_version text, politica_cancelacion_texto text);
	CREATE TABLE public.reserva_arriendo_comercial(id uuid PRIMARY KEY, estado text, pago_vence_en timestamptz, operacion_checkout_id uuid, cotizacion_id uuid, ocupacion_id uuid, inicio timestamptz, anfitrion_id uuid, arrendatario_id uuid, actualizada_en timestamptz);
	CREATE TABLE public.ocupacion(id uuid PRIMARY KEY, activo boolean, desactivada_en timestamptz, reserva_arriendo_comercial_id uuid);
	CREATE TABLE public.reserva_cuota_mensual(reserva_arriendo_comercial_id uuid, estado text);
	CREATE TABLE public.notificacion_usuario(id uuid PRIMARY KEY, usuario_id uuid, tipo text, titulo text, mensaje text, href text, referencia_id uuid, creada_en timestamptz);
	CREATE UNIQUE INDEX notificacion_owner ON public.notificacion_usuario(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	for _, migrationPath := range []string{
		"../../../../db/migrations/V000002__reservas_comerciales_postventa.sql",
		"../../../../db/migrations/V000006__conciliacion_financiera_mercado_pago.sql",
	} {
		sqlBytes, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := NewRepository(pool, pool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return pool, repo
}

type cancellationFixture struct {
	host, renter, space, reservationID, quote, firstHold, secondHold string
}

func seedCancellationReservation(t *testing.T, pool *pgxpool.Pool, now time.Time, totalCLP int64, policyVersion, policyText string, paymentID string, settlementJSON *string) cancellationFixture {
	t.Helper()
	f := cancellationFixture{
		host:          uuid.NewString(),
		renter:        uuid.NewString(),
		space:         uuid.NewString(),
		reservationID: uuid.NewString(),
		quote:         uuid.NewString(),
		firstHold:     uuid.NewString(),
		secondHold:    uuid.NewString(),
	}
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO public.usuario VALUES($1),($2)`, f.host, f.renter)
	batch.Queue(`INSERT INTO public.checkout_preferencia_comercial
		(id,cotizacion_id,espacio_id,arrendatario_id,host_id,estado,checkout_method,base_clp,total_charged_clp,marketplace_fee_clp,test_mode,vence_en,creada_en,actualizada_en,provider_payment_id,provider_payment_status,provider_payment_updated_at,payment_verified_at,provider_settlement)
		VALUES($1,$2,$3,$4,$5,'approved','checkout_pro',$6,$6,0,false,$7,$7,$7,$8,'approved',$7,$7,$9::jsonb)`,
		f.reservationID, f.quote, f.space, f.renter, f.host, totalCLP, now, paymentID, settlementJSON)
	batch.Queue(`INSERT INTO public.mercadopago_seller_connection VALUES($1,'91','active',true)`, f.host)
	batch.Queue(`INSERT INTO public.cotizacion_arriendo_comercial VALUES($1,$2,'CLP',$3,$4)`, f.quote, totalCLP, policyVersion, policyText)
	batch.Queue(`INSERT INTO public.reserva_arriendo_comercial VALUES($1,'pagada',$2,$1,$3,$4,$5,$6,$7,$2)`, f.reservationID, now, f.quote, f.firstHold, now.Add(72*time.Hour), f.host, f.renter)
	batch.Queue(`INSERT INTO public.ocupacion VALUES($1,true,NULL,$3),($2,true,NULL,$3)`, f.firstHold, f.secondHold, f.reservationID)
	batch.Queue(`INSERT INTO public.reserva_cuota_mensual VALUES($1,'pagada'),($1,'pendiente')`, f.reservationID)
	if err := pool.SendBatch(t.Context(), batch).Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCommercialCancellationPostgres(t *testing.T) {
	pool, repo := setupCancellationTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := seedCancellationReservation(t, pool, now, 12000, "facil_v1", "Devolución completa antes del inicio.", "10001", nil)

	preview, err := repo.PreviewCommercialCancellation(ctx, f.renter, f.reservationID, now)
	if err != nil || !preview.CanCancel || preview.RefundAmountCLP != 12000 || preview.RefundedAmountCLP != 0 || preview.RefundableBalanceCLP != 12000 {
		t.Fatalf("bad preview: %+v, %v", preview, err)
	}
	if _, err = repo.BeginCommercialCancellation(ctx, f.renter, f.reservationID, "Cambio de planes", 0, now, uuid.NewString()); !errors.Is(err, rentalflow.ErrConflict) {
		t.Fatalf("wrong expected refund should be rejected: %v", err)
	}
	operation, err := repo.BeginCommercialCancellation(ctx, f.renter, f.reservationID, "Cambio de planes", 12000, now, uuid.NewString())
	if err != nil || operation.RefundStatus != "pendiente" || operation.IdempotencyKey == "" || operation.SellerID != "91" {
		t.Fatalf("expected durable intent: %+v, %v", operation, err)
	}
	staleKey := uuid.NewString()
	if err := repo.FailCommercialCancellation(ctx, f.reservationID, staleKey, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FinishCommercialCancellation(ctx, f.reservationID, staleKey, "20001", rentalflow.CommercialRefundApproved, now); !errors.Is(err, rentalflow.ErrConflict) {
		t.Fatalf("stale attempt was accepted: %v", err)
	}
	pendingResult, err := repo.FinishCommercialCancellation(ctx, f.reservationID, operation.IdempotencyKey, "20001", rentalflow.CommercialRefundPending, now)
	if err != nil || pendingResult.Status != "pagada" || pendingResult.ProviderRefundID != "20001" {
		t.Fatalf("expected pending result bound to refund: %+v, %v", pendingResult, err)
	}
	if err := repo.FailCommercialCancellation(ctx, f.reservationID, operation.IdempotencyKey, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FinishCommercialCancellation(ctx, f.reservationID, operation.IdempotencyKey, "20002", rentalflow.CommercialRefundApproved, now); !errors.Is(err, rentalflow.ErrConflict) {
		t.Fatalf("different refund ID was accepted: %v", err)
	}
	for range 2 {
		result, err := repo.FinishCommercialCancellation(ctx, f.reservationID, operation.IdempotencyKey, "20001", rentalflow.CommercialRefundApproved, now)
		if err != nil || result.Status != "reembolsada" || result.RefundStatus != "aprobado" {
			t.Fatalf("expected approved result: %+v, %v", result, err)
		}
	}
	var active, pending, notifications int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1 AND activo),
		(SELECT count(*) FROM public.reserva_cuota_mensual WHERE reserva_arriendo_comercial_id=$1 AND estado='pendiente'),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1)`, f.reservationID).Scan(&active, &pending, &notifications); err != nil {
		t.Fatal(err)
	}
	if active != 0 || pending != 0 || notifications != 2 {
		t.Fatalf("expected released holds, canceled pending installments and both notifications, got %d/%d/%d", active, pending, notifications)
	}
}

func TestCommercialCancellationPartialRemainderAndZeroBalancePostgres(t *testing.T) {
	pool, repo := setupCancellationTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Partial refund (50% via moderada_v1) transitions reservation to 'cancelada'.
	fPartial := seedCancellationReservation(t, pool, now, 10000, "moderada_v1", "50% hasta 48h antes.", "10002", nil)
	partialPreview, err := repo.PreviewCommercialCancellation(ctx, fPartial.renter, fPartial.reservationID, now)
	if err != nil || !partialPreview.CanCancel || partialPreview.RefundPercent != 50 || partialPreview.RefundAmountCLP != 5000 || partialPreview.RefundableBalanceCLP != 10000 {
		t.Fatalf("unexpected partial preview: %+v err=%v", partialPreview, err)
	}
	partialOp, err := repo.BeginCommercialCancellation(ctx, fPartial.renter, fPartial.reservationID, "Cambio", 5000, now, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	partialDone, err := repo.FinishCommercialCancellation(ctx, fPartial.reservationID, partialOp.IdempotencyKey, "30001", rentalflow.CommercialRefundApproved, now)
	if err != nil || partialDone.Status != "cancelada" || partialDone.RefundStatus != rentalflow.CommercialRefundApproved {
		t.Fatalf("unexpected partial finish: %+v err=%v", partialDone, err)
	}

	// 2. Prior refunded_clp=2000 caps 100% policy refund to remainder 8000.
	prior2000 := `{"refunded_clp": 2000, "reconciliation": "discrepancy"}`
	fRem := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10003", &prior2000)
	remPreview, err := repo.PreviewCommercialCancellation(ctx, fRem.renter, fRem.reservationID, now)
	if err != nil || !remPreview.CanCancel || remPreview.RefundedAmountCLP != 2000 || remPreview.RefundableBalanceCLP != 8000 || remPreview.RefundAmountCLP != 8000 {
		t.Fatalf("unexpected remainder preview: %+v err=%v", remPreview, err)
	}
	remOp, err := repo.BeginCommercialCancellation(ctx, fRem.renter, fRem.reservationID, "Reembolsar remanente", 8000, now, uuid.NewString())
	if err != nil || remOp.RefundedAmountCLP != 2000 || remOp.RefundableBalanceCLP != 8000 || remOp.RefundAmountCLP != 8000 {
		t.Fatalf("unexpected remainder begin: %+v err=%v", remOp, err)
	}
	remDone, err := repo.FinishCommercialCancellation(ctx, fRem.reservationID, remOp.IdempotencyKey, "30002", rentalflow.CommercialRefundApproved, now)
	if err != nil || remDone.RefundedAmountCLP != 2000 || remDone.RefundableBalanceCLP != 8000 {
		t.Fatalf("unexpected remainder finish: %+v err=%v", remDone, err)
	}

	// 3. Zero refundable balance on uncancelled reservation blocks cancellation and requires reconciliation.
	fullPrior := `{"refunded_clp": 10000, "reconciliation": "discrepancy"}`
	fZero := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10004", &fullPrior)
	zeroPreview, err := repo.PreviewCommercialCancellation(ctx, fZero.renter, fZero.reservationID, now)
	if err != nil || zeroPreview.CanCancel || zeroPreview.Reason != "refund_requires_reconciliation" || zeroPreview.RefundableBalanceCLP != 0 || zeroPreview.RefundAmountCLP != 0 {
		t.Fatalf("expected blocked zero-balance preview: %+v err=%v", zeroPreview, err)
	}
	if _, err := repo.BeginCommercialCancellation(ctx, fZero.renter, fZero.reservationID, "Intentar", 0, now, uuid.NewString()); !errors.Is(err, rentalflow.ErrConflict) {
		t.Fatalf("expected ErrConflict for zero refundable balance with percent>0, got %v", err)
	}
}

func TestCommercialCancellationRetryKeysAndWebhookAndExternalReconciliationPostgres(t *testing.T) {
	pool, repo := setupCancellationTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Insufficient funds / deterministic fail keeps reservation pagada & occupancy active, then retry creates new key.
	fFail := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10005", nil)
	op1, err := repo.BeginCommercialCancellation(ctx, fFail.renter, fFail.reservationID, "Cancelar", 10000, now, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	// Uncertain retry while pendiente reuses the same idempotency key.
	opSame, err := repo.BeginCommercialCancellation(ctx, fFail.renter, fFail.reservationID, "Cancelar", 10000, now.Add(time.Second), uuid.NewString())
	if err != nil || opSame.IdempotencyKey != op1.IdempotencyKey {
		t.Fatalf("uncertain retry must reuse idempotency key: op1=%s opSame=%s err=%v", op1.IdempotencyKey, opSame.IdempotencyKey, err)
	}
	pendingList, err := repo.ListPendingCommercialCancellations(ctx, 10)
	if err != nil || len(pendingList) != 1 || pendingList[0].ReservationID != fFail.reservationID || pendingList[0].RefundableBalanceCLP != 10000 {
		t.Fatalf("unexpected pending list: %+v err=%v", pendingList, err)
	}
	if err := repo.FailCommercialCancellation(ctx, fFail.reservationID, op1.IdempotencyKey, "", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	var resStatus string
	var activeHolds, failedNotifs int
	if err := pool.QueryRow(ctx, `SELECT r.estado,
		(SELECT count(*) FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1 AND activo),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1 AND tipo='reembolso_fallido')
		FROM public.reserva_arriendo_comercial r WHERE r.id=$1`, fFail.reservationID).Scan(&resStatus, &activeHolds, &failedNotifs); err != nil {
		t.Fatal(err)
	}
	if resStatus != "pagada" || activeHolds != 2 || failedNotifs != 2 {
		t.Fatalf("failed refund must preserve active reservation/holds and notify both parties: status=%s holds=%d notifs=%d", resStatus, activeHolds, failedNotifs)
	}
	retryReqID := uuid.NewString()
	op2, err := repo.BeginCommercialCancellation(ctx, fFail.renter, fFail.reservationID, "Cancelar", 10000, now.Add(3*time.Second), retryReqID)
	if err != nil || op2.IdempotencyKey != retryReqID || op2.IdempotencyKey == op1.IdempotencyKey {
		t.Fatalf("retry after deterministic fail must use new key: op1=%s op2=%s err=%v", op1.IdempotencyKey, op2.IdempotencyKey, err)
	}

	// 2. Duplicate/replayed webhook with status='refunded' closes pending cancellation once.
	fHook := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10006", nil)
	if _, err := repo.BeginCommercialCancellation(ctx, fHook.renter, fHook.reservationID, "Cancelar", 10000, now, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	hookPayment := checkoutpro.VerifiedPayment{
		OperationID:          fHook.reservationID,
		ExternalReference:    fHook.reservationID,
		ProviderPaymentID:    "10006",
		SellerID:             "91",
		Status:               "refunded",
		TransactionAmountCLP: 10000,
		Currency:             "CLP",
		PaymentType:          "account_money",
		LiveMode:             true,
		PaymentUpdatedAt:     now.Add(time.Minute),
	}
	for range 2 {
		applied, err := repo.ApplyVerifiedPayment(ctx, hookPayment)
		if err != nil || !applied {
			t.Fatalf("webhook replay failed: applied=%t err=%v", applied, err)
		}
	}
	var hookState string
	var hookActive, hookNotifs int
	if err := pool.QueryRow(ctx, `SELECT r.estado,
		(SELECT count(*) FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1 AND activo),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1)
		FROM public.reserva_arriendo_comercial r WHERE r.id=$1`, fHook.reservationID).Scan(&hookState, &hookActive, &hookNotifs); err != nil {
		t.Fatal(err)
	}
	if hookState != "reembolsada" || hookActive != 0 || hookNotifs != 2 {
		t.Fatalf("unexpected webhook state/holds/notifications: %s/%d/%d", hookState, hookActive, hookNotifs)
	}

	// 3. External full refund reconciliation in Postgres closes reservation once and is idempotent.
	fExt := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10007", nil)
	refundedTotal := int64(10000)
	extPayment := checkoutpro.VerifiedPayment{
		OperationID:          fExt.reservationID,
		ExternalReference:    fExt.reservationID,
		ProviderPaymentID:    "10007",
		SellerID:             "91",
		Status:               "refunded",
		TransactionAmountCLP: 10000,
		Currency:             "CLP",
		PaymentType:          "account_money",
		LiveMode:             true,
		PaymentUpdatedAt:     now.Add(2 * time.Minute),
		Settlement:           &checkoutpro.Settlement{PaymentStatus: "refunded", RefundedCLP: &refundedTotal, ProviderUpdated: now.Add(2 * time.Minute)},
	}
	extRefund := checkoutpro.RefundResult{ID: "9007", Status: "approved", Amount: 10000}
	for range 2 {
		got, err := repo.RecordExternalFullCommercialRefund(ctx, fExt.host, fExt.reservationID, extPayment, extRefund, now.Add(3*time.Minute))
		if err != nil || got.Status != "reembolsada" || got.RefundStatus != rentalflow.CommercialRefundApproved || got.RefundedAmountCLP != 10000 || got.RefundableBalanceCLP != 0 {
			t.Fatalf("external reconciliation failed: %+v err=%v", got, err)
		}
	}
	var extActive, extPending, extNotifs int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1 AND activo),
		(SELECT count(*) FROM public.reserva_cuota_mensual WHERE reserva_arriendo_comercial_id=$1 AND estado='pendiente'),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1)`, fExt.reservationID).Scan(&extActive, &extPending, &extNotifs); err != nil {
		t.Fatal(err)
	}
	if extActive != 0 || extPending != 0 || extNotifs != 2 {
		t.Fatalf("unexpected external refund cleanup: %d/%d/%d", extActive, extPending, extNotifs)
	}

	// 4. Chargeback webhook keeps checkout approved, records settlement discrepancy, and notifies host once.
	fChargeback := seedCancellationReservation(t, pool, now, 10000, "facil_v1", "Total.", "10008", nil)
	cbPayment := checkoutpro.VerifiedPayment{
		OperationID:          fChargeback.reservationID,
		ExternalReference:    fChargeback.reservationID,
		ProviderPaymentID:    "10008",
		SellerID:             "91",
		Status:               "charged_back",
		TransactionAmountCLP: 10000,
		Currency:             "CLP",
		PaymentType:          "credit_card",
		LiveMode:             true,
		PaymentUpdatedAt:     now.Add(4 * time.Minute),
		Settlement:           &checkoutpro.Settlement{PaymentStatus: "charged_back", ProviderUpdated: now.Add(4 * time.Minute)},
	}
	for range 2 {
		applied, err := repo.ApplyVerifiedPayment(ctx, cbPayment)
		if err != nil || !applied {
			t.Fatalf("chargeback webhook failed: applied=%t err=%v", applied, err)
		}
	}
	var cbCheckoutState, cbReservationState, cbPaymentStatus, cbHref string
	var cbNotifs int
	if err := pool.QueryRow(ctx, `SELECT c.estado,r.estado,COALESCE(c.provider_payment_status,''),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1 AND tipo='contracargo_reserva'),
		COALESCE((SELECT href FROM public.notificacion_usuario WHERE referencia_id=$1 AND tipo='contracargo_reserva' LIMIT 1),'')
		FROM public.checkout_preferencia_comercial c
		JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id
		WHERE r.id=$1`, fChargeback.reservationID).Scan(&cbCheckoutState, &cbReservationState, &cbPaymentStatus, &cbNotifs, &cbHref); err != nil {
		t.Fatal(err)
	}
	if cbCheckoutState != "approved" || cbReservationState != "pagada" || cbPaymentStatus != "charged_back" || cbNotifs != 1 || cbHref != "/anfitrion/reservas/"+fChargeback.reservationID {
		t.Fatalf("unexpected chargeback persistence: chk=%s res=%s pay=%s notifs=%d href=%s", cbCheckoutState, cbReservationState, cbPaymentStatus, cbNotifs, cbHref)
	}
}

func TestCommercialCancellationRetryFailedRefundAfterStartPostgres(t *testing.T) {
	pool, repo := setupCancellationTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	f := seedCancellationReservation(t, pool, now, 1036, "flexible_v1", "Flexible: reembolso total hasta 24 horas antes del inicio del arriendo.", "190475305601", nil)

	op1, err := repo.BeginCommercialCancellation(ctx, f.renter, f.reservationID, "Cambio de planes", 1036, now, uuid.NewString())
	if err != nil || op1.RefundStatus != "pendiente" {
		t.Fatalf("expected initial pending refund: %+v err=%v", op1, err)
	}
	if err := repo.FailCommercialCancellation(ctx, f.reservationID, op1.IdempotencyKey, "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	// Simulate time moving past the reservation start, still within 180 days of payment verification.
	var startsAt time.Time
	if err := pool.QueryRow(ctx, `SELECT inicio FROM public.reserva_arriendo_comercial WHERE id=$1`, f.reservationID).Scan(&startsAt); err != nil {
		t.Fatal(err)
	}
	afterStart := startsAt.Add(24 * time.Hour)
	previewHost, err := repo.PreviewCommercialCancellation(ctx, f.host, f.reservationID, afterStart)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if !previewHost.CanCancel || previewHost.Reason != "refund_failed" || previewHost.RefundAmountCLP != 1036 {
		t.Fatalf("expected retryable refund_failed after start within 180 days: %+v", previewHost)
	}

	// Beyond 180 days from payment verification, retry must report refund_window_expired.
	expiredPreview, err := repo.PreviewCommercialCancellation(ctx, f.host, f.reservationID, now.Add(181*24*time.Hour))
	if err != nil {
		t.Fatalf("expired preview failed: %v", err)
	}
	if expiredPreview.CanCancel || expiredPreview.Reason != "refund_window_expired" {
		t.Fatalf("expected refund_window_expired after 180 days: %+v", expiredPreview)
	}

	// Host retries the renter-initiated failed refund with a new reason and finishes it.
	retryOp, err := repo.BeginCommercialCancellation(ctx, f.host, f.reservationID, "Reintento tras reponer saldo", 1036, afterStart, uuid.NewString())
	if err != nil || retryOp.RefundStatus != "pendiente" || retryOp.IdempotencyKey == op1.IdempotencyKey {
		t.Fatalf("expected host retry with fresh idempotency key: %+v err=%v", retryOp, err)
	}
	finished, err := repo.FinishCommercialCancellation(ctx, f.reservationID, retryOp.IdempotencyKey, "99001", rentalflow.CommercialRefundApproved, afterStart.Add(time.Second))
	if err != nil || finished.Status != "reembolsada" || finished.RefundStatus != rentalflow.CommercialRefundApproved {
		t.Fatalf("expected host retry finish to refund reservation: %+v err=%v", finished, err)
	}
}
