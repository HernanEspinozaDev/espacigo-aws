package rentalflowpg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type staticSellerTokens struct {
	token string
	err   error
}

func (s staticSellerTokens) AccessToken(context.Context, string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.token), nil
}

type fakeMercadoPagoServer struct {
	mu          sync.Mutex
	payment     map[string]any
	searchItems []map[string]any
	putStatus   int
	putCalls    int
	getCalls    int
	searchCalls int
	searchErr   int
	lastPutKey  string
	putHook     func()
}

func newFakeMercadoPagoServer(t *testing.T, handler *fakeMercadoPagoServer) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer seller-oauth-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		handler.mu.Lock()
		defer handler.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payments/search":
			handler.searchCalls++
			if handler.searchErr >= 400 {
				http.Error(w, `{"error":"internal_error"}`, handler.searchErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": handler.searchItems})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/payments/"):
			handler.getCalls++
			if handler.payment == nil {
				http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(handler.payment)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/payments/"):
			handler.putCalls++
			handler.lastPutKey = r.Header.Get("X-Idempotency-Key")
			if handler.putHook != nil {
				handler.putHook()
			}
			if handler.putStatus >= 400 {
				http.Error(w, `{"error":"internal_error"}`, handler.putStatus)
				return
			}
			if handler.payment != nil {
				handler.payment["status"] = "cancelled"
			}
			_ = json.NewEncoder(w).Encode(handler.payment)
		default:
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupPendingPaymentCancellationTestDB(t *testing.T) (*pgxpool.Pool, *Repository, *time.Time) {
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
		t.Skip("pending payment cancellation integration requires loopback PostgreSQL")
	}
	ctx := t.Context()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	dbName := "test_pending_cancel_" + uuid.NewString()[:8]
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

	ddl := `DO $$BEGIN
		IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='espacigo_runtime') THEN
			CREATE ROLE espacigo_runtime;
		END IF;
	END$$;
	CREATE TABLE public.usuario(id uuid PRIMARY KEY);
	CREATE TABLE public.espacio(id uuid PRIMARY KEY, propietario_id uuid NOT NULL, estado text NOT NULL);
	CREATE TABLE public.cotizacion_arriendo_comercial(
		id uuid PRIMARY KEY,
		espacio_id uuid NOT NULL,
		propietario_id uuid NOT NULL,
		arrendatario_id uuid NOT NULL,
		subtotal_clp bigint NOT NULL,
		moneda text NOT NULL,
		vence_en timestamptz NOT NULL
	);
	CREATE TABLE public.cotizacion_arriendo_tramo(
		id uuid PRIMARY KEY,
		cotizacion_id uuid NOT NULL,
		inicio timestamptz NOT NULL,
		termino timestamptz NOT NULL,
		unidades bigint NOT NULL,
		orden integer NOT NULL
	);
	CREATE TABLE public.mercadopago_seller_connection(
		anfitrion_id uuid PRIMARY KEY,
		seller_id text NOT NULL,
		connection_state text NOT NULL,
		live_mode boolean NOT NULL
	);
	CREATE TABLE public.checkout_preferencia_comercial(
		id uuid PRIMARY KEY,
		cotizacion_id uuid NOT NULL,
		espacio_id uuid NOT NULL,
		arrendatario_id uuid NOT NULL,
		host_id uuid NOT NULL,
		estado text NOT NULL,
		checkout_method text NOT NULL DEFAULT 'checkout_pro',
		base_clp bigint NOT NULL,
		total_charged_clp bigint NOT NULL,
		marketplace_fee_clp bigint NOT NULL DEFAULT 0,
		test_mode boolean NOT NULL DEFAULT false,
		vence_en timestamptz NOT NULL,
		creada_en timestamptz NOT NULL,
		actualizada_en timestamptz NOT NULL,
		provider_payment_id text,
		provider_payment_status text,
		provider_payment_updated_at timestamptz,
		payment_verified_at timestamptz,
		reconciliation_reason text,
		provider_settlement jsonb,
		provider_settlement_attempted_at timestamptz
	);
	CREATE TABLE public.reserva_arriendo_comercial(
		id uuid PRIMARY KEY,
		operacion_checkout_id uuid NOT NULL,
		cotizacion_id uuid NOT NULL,
		espacio_id uuid NOT NULL,
		anfitrion_id uuid NOT NULL,
		arrendatario_id uuid NOT NULL,
		estado text NOT NULL,
		inicio timestamptz NOT NULL,
		termino timestamptz NOT NULL,
		ocupacion_id uuid NOT NULL,
		pago_vence_en timestamptz NOT NULL,
		creada_en timestamptz NOT NULL,
		actualizada_en timestamptz NOT NULL
	);
	CREATE TABLE public.ocupacion(
		id uuid PRIMARY KEY,
		espacio_id uuid NOT NULL,
		reserva_arriendo_comercial_id uuid,
		intervalo tstzrange NOT NULL,
		tipo text NOT NULL,
		activo boolean NOT NULL DEFAULT true,
		expira_en timestamptz,
		desactivada_en timestamptz
	);
	CREATE TABLE public.reserva_cuota_mensual(
		reserva_arriendo_comercial_id uuid NOT NULL,
		numero_cuota integer NOT NULL DEFAULT 1,
		estado text NOT NULL,
		pagada_en timestamptz
	);
	CREATE TABLE public.checkout_payment_notice_outbox(
		id uuid PRIMARY KEY,
		reservation_id uuid NOT NULL,
		operation_id uuid NOT NULL,
		recipient_id uuid NOT NULL,
		recipient_role text NOT NULL,
		transition text NOT NULL,
		dedupe_key text NOT NULL UNIQUE,
		state text NOT NULL DEFAULT 'pending',
		available_at timestamptz NOT NULL,
		created_at timestamptz NOT NULL
	);
	CREATE TABLE public.notificacion_usuario(
		id uuid PRIMARY KEY,
		usuario_id uuid NOT NULL,
		tipo text NOT NULL,
		titulo text NOT NULL,
		mensaje text NOT NULL,
		href text NOT NULL,
		referencia_id uuid,
		creada_en timestamptz NOT NULL
	);
	CREATE UNIQUE INDEX notificacion_owner ON public.notificacion_usuario(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL;
	CREATE FUNCTION public.enqueue_checkout_payment_notices(p_operation_id uuid, p_transition text, p_at timestamptz) RETURNS void
	LANGUAGE plpgsql AS $$
	DECLARE reservation public.reserva_arriendo_comercial%ROWTYPE;
	BEGIN
		SELECT * INTO reservation FROM public.reserva_arriendo_comercial WHERE operacion_checkout_id=p_operation_id FOR UPDATE;
		IF p_transition='approved' THEN
			INSERT INTO public.checkout_payment_notice_outbox(id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
			VALUES
				(gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,'checkout-payment:'||p_operation_id::text||':approved:'||reservation.arrendatario_id::text,p_at,p_at),
				(gen_random_uuid(),reservation.id,p_operation_id,reservation.anfitrion_id,'anfitrion',p_transition,'checkout-payment:'||p_operation_id::text||':approved:'||reservation.anfitrion_id::text,p_at,p_at)
			ON CONFLICT (dedupe_key) DO NOTHING;
		ELSE
			INSERT INTO public.checkout_payment_notice_outbox(id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
			VALUES (gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,'checkout-payment:'||p_operation_id::text||':'||p_transition||':'||reservation.arrendatario_id::text,p_at,p_at)
			ON CONFLICT (dedupe_key) DO NOTHING;
		END IF;
	END $$;`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	v10SQL, err := os.ReadFile("../../../../db/legacy-migrations/v29/V000010__cancelacion_checkout_pendiente.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(v10SQL)); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2030, 1, 1, 10, 0, 0, 0, time.UTC)
	repo, err := NewRepository(pool, pool, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return pool, repo, &now
}

type fixtureIDs struct {
	host, renter, outsider, space, quote, op, hold string
}

func seedPendingCheckoutFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, now time.Time, expiresAt time.Time, paymentID string) fixtureIDs {
	t.Helper()
	ids := fixtureIDs{
		host:     uuid.NewString(),
		renter:   uuid.NewString(),
		outsider: uuid.NewString(),
		space:    uuid.NewString(),
		quote:    uuid.NewString(),
		op:       uuid.NewString(),
		hold:     uuid.NewString(),
	}
	startAt := now.Add(48 * time.Hour)
	endAt := startAt.Add(2 * time.Hour)

	var payIDArg, payStatusArg, payUpdatedArg any
	if paymentID != "" {
		payIDArg = paymentID
		payStatusArg = "pending"
		payUpdatedArg = now
	}

	b := &pgx.Batch{}
	b.Queue(`INSERT INTO public.usuario VALUES($1),($2),($3)`, ids.host, ids.renter, ids.outsider)
	b.Queue(`INSERT INTO public.espacio VALUES($1,$2,'activa')`, ids.space, ids.host)
	b.Queue(`INSERT INTO public.cotizacion_arriendo_comercial VALUES($1,$2,$3,$4,12000,'CLP',$5)`, ids.quote, ids.space, ids.host, ids.renter, expiresAt)
	b.Queue(`INSERT INTO public.cotizacion_arriendo_tramo VALUES($1,$2,$3,$4,2,1)`, uuid.NewString(), ids.quote, startAt, endAt)
	b.Queue(`INSERT INTO public.mercadopago_seller_connection VALUES($1,'91','active',true)`, ids.host)
	b.Queue(`INSERT INTO public.checkout_preferencia_comercial(id,cotizacion_id,espacio_id,arrendatario_id,host_id,estado,checkout_method,base_clp,total_charged_clp,marketplace_fee_clp,test_mode,vence_en,creada_en,actualizada_en,provider_payment_id,provider_payment_status,provider_payment_updated_at) VALUES($1,$2,$3,$4,$5,'pending','checkout_pro',12000,12000,600,false,$6,$7,$7,$8,$9,$10)`,
		ids.op, ids.quote, ids.space, ids.renter, ids.host, expiresAt, now, payIDArg, payStatusArg, payUpdatedArg)
	b.Queue(`INSERT INTO public.reserva_arriendo_comercial VALUES($1,$1,$2,$3,$4,$5,'pendiente_de_pago',$6,$7,$8,$9,$10,$10)`,
		ids.op, ids.quote, ids.space, ids.host, ids.renter, startAt, endAt, ids.hold, expiresAt, now)
	b.Queue(`INSERT INTO public.ocupacion VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'retencion_comercial',true,$6,NULL)`,
		ids.hold, ids.space, ids.op, startAt, endAt, expiresAt)
	b.Queue(`INSERT INTO public.reserva_cuota_mensual VALUES($1,1,'pendiente',NULL)`, ids.op)
	if err := pool.SendBatch(ctx, b).Close(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestPendingPaymentCancellationPostgres(t *testing.T) {
	pool, repo, clock := setupPendingPaymentCancellationTestDB(t)
	ctx := t.Context()

	t.Run("accepted cancellation, replay idempotency, and non-participant rejection", func(t *testing.T) {
		now := *clock
		ids := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(15*time.Minute), "10001")
		mp := &fakeMercadoPagoServer{
			payment: map[string]any{
				"id": 10001, "collector_id": 91, "external_reference": ids.op,
				"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
				"live_mode": true, "payment_type_id": "account_money",
				"metadata": map[string]any{"operation_id": ids.op}, "date_last_updated": now.Format(time.RFC3339),
			},
		}
		srv := newFakeMercadoPagoServer(t, mp)
		svc, _ := rentalflow.NewCommercialReservationService(repo)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srv.URL, Client: srv.Client()}, func() time.Time { return *clock })

		// Non-participant must receive ErrNotFound.
		if _, err := svc.CancelPendingPayment(ctx, ids.outsider, ids.op); !errors.Is(err, rentalflow.ErrNotFound) {
			t.Fatalf("outsider expected ErrNotFound, got %v", err)
		}

		res, err := svc.CancelPendingPayment(ctx, ids.renter, ids.op)
		if err != nil || res.PaymentStatus != "cancelled" {
			t.Fatalf("expected cancelled, got %+v err=%v", res, err)
		}
		if mp.putCalls != 1 || mp.getCalls != 2 {
			t.Fatalf("unexpected MP calls: put=%d get=%d", mp.putCalls, mp.getCalls)
		}

		// Replay by host hits persisted outcome without calling MP again.
		replay, err := svc.CancelPendingPayment(ctx, ids.host, ids.op)
		if err != nil || replay.PaymentStatus != "cancelled" || mp.putCalls != 1 || mp.getCalls != 2 {
			t.Fatalf("replay got=%+v err=%v put=%d get=%d", replay, err, mp.putCalls, mp.getCalls)
		}

		var resState, chkState, outcome string
		var activeHold bool
		var notices, appNotifs int
		if err := pool.QueryRow(ctx, `SELECT r.estado,c.estado,i.resultado,o.activo,
			(SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1),
			(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1 AND tipo='pago_pendiente_cancelado')
			FROM public.reserva_arriendo_comercial r
			JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
			JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=c.id
			JOIN public.ocupacion o ON o.id=r.ocupacion_id WHERE r.id=$1`, ids.op).
			Scan(&resState, &chkState, &outcome, &activeHold, &notices, &appNotifs); err != nil {
			t.Fatal(err)
		}
		if resState != "cancelada_por_pago" || chkState != "cancelled" || outcome != "cancelled" || activeHold || notices != 1 || appNotifs != 2 {
			t.Fatalf("unexpected DB state: res=%q chk=%q out=%q active=%t notices=%d appNotifs=%d", resState, chkState, outcome, activeHold, notices, appNotifs)
		}
	})

	t.Run("in-flight lease, timeout PUT recovery, and reconciler reuse of idempotency key", func(t *testing.T) {
		now := *clock
		ids := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(15*time.Minute), "10002")
		mp := &fakeMercadoPagoServer{
			putStatus: http.StatusGatewayTimeout,
			payment: map[string]any{
				"id": 10002, "collector_id": 91, "external_reference": ids.op,
				"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
				"live_mode": true, "payment_type_id": "account_money",
				"metadata": map[string]any{"operation_id": ids.op}, "date_last_updated": now.Format(time.RFC3339),
			},
		}
		srv := newFakeMercadoPagoServer(t, mp)
		svc, _ := rentalflow.NewCommercialReservationService(repo)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srv.URL, Client: srv.Client()}, func() time.Time { return *clock })

		// First attempt fails PUT and GET still returns pending -> 202 pending.
		res, err := svc.CancelPendingPayment(ctx, ids.renter, ids.op)
		if err != nil || res.PaymentStatus != "pending" {
			t.Fatalf("expected pending, got %+v err=%v", res, err)
		}
		firstKey := mp.lastPutKey

		// Immediate second call within 30s lease returns pending without calling MP.
		*clock = now.Add(10 * time.Second)
		res2, err := svc.CancelPendingPayment(ctx, ids.host, ids.op)
		if err != nil || res2.PaymentStatus != "pending" || mp.putCalls != 1 {
			t.Fatalf("lease was not respected: got=%+v err=%v putCalls=%d", res2, err, mp.putCalls)
		}

		// After lease expires, MP actually accepted cancellation on PUT despite timeout.
		*clock = now.Add(35 * time.Second)
		mp.mu.Lock()
		mp.putHook = func() {
			mp.payment["status"] = "cancelled"
		}
		mp.mu.Unlock()
		if err := svc.ReconcilePendingPaymentCancellations(ctx, 10); err != nil {
			t.Fatalf("reconcile failed: %v", err)
		}
		if mp.putCalls != 2 || mp.lastPutKey != firstKey {
			t.Fatalf("expected retry with original idempotency key %q, got %q (calls=%d)", firstKey, mp.lastPutKey, mp.putCalls)
		}
		res3, err := svc.CancelPendingPayment(ctx, ids.renter, ids.op)
		if err != nil || res3.PaymentStatus != "cancelled" {
			t.Fatalf("expected cancelled after recovery, got %+v err=%v", res3, err)
		}
	})

	t.Run("concurrent approval preserves paid reservation", func(t *testing.T) {
		now := *clock
		ids := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(15*time.Minute), "10003")
		mp := &fakeMercadoPagoServer{
			putStatus: http.StatusBadRequest,
			payment: map[string]any{
				"id": 10003, "collector_id": 91, "external_reference": ids.op,
				"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
				"live_mode": true, "payment_type_id": "account_money",
				"metadata": map[string]any{"operation_id": ids.op}, "date_last_updated": now.Format(time.RFC3339),
			},
		}
		mp.putHook = func() {
			mp.payment["status"] = "approved"
		}
		srv := newFakeMercadoPagoServer(t, mp)
		svc, _ := rentalflow.NewCommercialReservationService(repo)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srv.URL, Client: srv.Client()}, func() time.Time { return *clock })

		res, err := svc.CancelPendingPayment(ctx, ids.renter, ids.op)
		if err != nil || res.PaymentStatus != "approved" {
			t.Fatalf("expected approved, got %+v err=%v", res, err)
		}
		var resState, holdType, outcome string
		var activeHold bool
		if err := pool.QueryRow(ctx, `SELECT r.estado,o.tipo,o.activo,i.resultado FROM public.reserva_arriendo_comercial r JOIN public.ocupacion o ON o.id=r.ocupacion_id JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=r.operacion_checkout_id WHERE r.id=$1`, ids.op).
			Scan(&resState, &holdType, &activeHold, &outcome); err != nil {
			t.Fatal(err)
		}
		if resState != "pagada" || holdType != "reserva_comercial" || !activeHold || outcome != "approved" {
			t.Fatalf("paid reservation not preserved: state=%q holdType=%q active=%t outcome=%q", resState, holdType, activeHold, outcome)
		}
	})

	t.Run("missing payment ID with external_reference found and expired vs not expired", func(t *testing.T) {
		now := *clock

		// Subcase 1: Missing payment ID, not expired, MP returns empty -> stays pending and holds remain active.
		idsPending := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(15*time.Minute), "")
		mpEmpty := &fakeMercadoPagoServer{searchItems: []map[string]any{}}
		srvEmpty := newFakeMercadoPagoServer(t, mpEmpty)
		svc, _ := rentalflow.NewCommercialReservationService(repo)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srvEmpty.URL, Client: srvEmpty.Client()}, func() time.Time { return *clock })

		res, err := svc.CancelPendingPayment(ctx, idsPending.renter, idsPending.op)
		if err != nil || res.PaymentStatus != "pending" || mpEmpty.searchCalls != 1 {
			t.Fatalf("expected pending before expiry, got %+v err=%v searchCalls=%d", res, err, mpEmpty.searchCalls)
		}
		// Immediate retry respects lease even with NULL payment ID.
		resLease, err := svc.CancelPendingPayment(ctx, idsPending.renter, idsPending.op)
		if err != nil || resLease.PaymentStatus != "pending" || mpEmpty.searchCalls != 1 {
			t.Fatalf("expected lease to throttle search, got %+v err=%v searchCalls=%d", resLease, err, mpEmpty.searchCalls)
		}

		// Advance clock past lease AND past pago_vence_en -> expires locally, releases occupancy once, marks cancelled.
		*clock = now.Add(20 * time.Minute)
		resExpired, err := svc.CancelPendingPayment(ctx, idsPending.renter, idsPending.op)
		if err != nil || resExpired.PaymentStatus != "cancelled" {
			t.Fatalf("expected cancelled after expiry with empty MP search, got %+v err=%v", resExpired, err)
		}
		var resState, chkState, outcome string
		var activeHold bool
		var notices int
		if err := pool.QueryRow(ctx, `SELECT r.estado,c.estado,i.resultado,o.activo,(SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1)
			FROM public.reserva_arriendo_comercial r JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=c.id JOIN public.ocupacion o ON o.id=r.ocupacion_id WHERE r.id=$1`, idsPending.op).
			Scan(&resState, &chkState, &outcome, &activeHold, &notices); err != nil {
			t.Fatal(err)
		}
		if resState != "vencida_pago" || chkState != "expired" || outcome != "cancelled" || activeHold || notices != 1 {
			t.Fatalf("unexpected expired state: res=%q chk=%q outcome=%q active=%t notices=%d", resState, chkState, outcome, activeHold, notices)
		}

		// Subcase 2: Missing payment ID, MP search fails past expiry -> must NOT release occupancy.
		now = *clock
		idsUncertain := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(-time.Minute), "")
		mpFail := &fakeMercadoPagoServer{searchErr: http.StatusInternalServerError}
		srvFail := newFakeMercadoPagoServer(t, mpFail)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srvFail.URL, Client: srvFail.Client()}, func() time.Time { return *clock })
		resFail, err := svc.CancelPendingPayment(ctx, idsUncertain.renter, idsUncertain.op)
		if err != nil || resFail.PaymentStatus != "pending" {
			t.Fatalf("expected pending on search error, got %+v err=%v", resFail, err)
		}
		var stillActive bool
		if err := pool.QueryRow(ctx, `SELECT activo FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1`, idsUncertain.op).Scan(&stillActive); err != nil || !stillActive {
			t.Fatalf("occupancy was released despite uncertain provider search: active=%t err=%v", stillActive, err)
		}

		// Subcase 3: Missing payment ID, MP search finds pending payment -> cancels and binds payment ID.
		idsFound := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(15*time.Minute), "")
		mpFound := &fakeMercadoPagoServer{
			searchItems: []map[string]any{{
				"id": 10004, "collector_id": 91, "external_reference": idsFound.op,
				"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
				"live_mode": true, "payment_type_id": "account_money",
				"metadata": map[string]any{"operation_id": idsFound.op}, "date_last_updated": now.Format(time.RFC3339),
			}},
			payment: map[string]any{
				"id": 10004, "collector_id": 91, "external_reference": idsFound.op,
				"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
				"live_mode": true, "payment_type_id": "account_money",
				"metadata": map[string]any{"operation_id": idsFound.op}, "date_last_updated": now.Format(time.RFC3339),
			},
		}
		srvFound := newFakeMercadoPagoServer(t, mpFound)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srvFound.URL, Client: srvFound.Client()}, func() time.Time { return *clock })
		resFound, err := svc.CancelPendingPayment(ctx, idsFound.renter, idsFound.op)
		if err != nil || resFound.PaymentStatus != "cancelled" {
			t.Fatalf("expected cancelled via external_reference, got %+v err=%v", resFound, err)
		}
		var boundID string
		if err := pool.QueryRow(ctx, `SELECT provider_payment_id FROM public.checkout_cancelacion_pendiente WHERE operacion_id=$1`, idsFound.op).Scan(&boundID); err != nil || boundID != "10004" {
			t.Fatalf("expected bound payment ID 10004, got %q err=%v", boundID, err)
		}
	})

	t.Run("late cancellation on already expired checkout (vencida_pago)", func(t *testing.T) {
		now := *clock
		ids := seedPendingCheckoutFixture(t, ctx, pool, now, now.Add(5*time.Minute), "10005")
		mp := &fakeMercadoPagoServer{}
		srv := newFakeMercadoPagoServer(t, mp)
		// First call records intent while pending, but GET fails so intent remains open.
		svc, _ := rentalflow.NewCommercialReservationService(repo)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srv.URL, Client: srv.Client()}, func() time.Time { return *clock })
		res, err := svc.CancelPendingPayment(ctx, ids.renter, ids.op)
		if err != nil || res.PaymentStatus != "pending" {
			t.Fatalf("expected pending, got %+v err=%v", res, err)
		}

		// Checkout expires via PurgeExpiredCommercialQuotes while cancellation is pending.
		*clock = now.Add(10 * time.Minute)
		if _, err := repo.PurgeExpiredCommercialQuotes(ctx, *clock, 100); err != nil {
			t.Fatal(err)
		}

		// Reconciler resumes after lease on vencida_pago checkout and MP confirms cancellation.
		mp.mu.Lock()
		mp.payment = map[string]any{
			"id": 10005, "collector_id": 91, "external_reference": ids.op,
			"status": "pending", "transaction_amount": 12000, "currency_id": "CLP",
			"live_mode": true, "payment_type_id": "account_money",
			"metadata": map[string]any{"operation_id": ids.op}, "date_last_updated": (*clock).Format(time.RFC3339),
		}
		mp.mu.Unlock()
		if err := svc.ReconcilePendingPaymentCancellations(ctx, 10); err != nil {
			t.Fatalf("late reconcile on expired checkout failed: %v", err)
		}
		var resState, chkState, outcome string
		var notices int
		if err := pool.QueryRow(ctx, `SELECT r.estado,c.estado,i.resultado,(SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1)
			FROM public.reserva_arriendo_comercial r JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=c.id WHERE r.id=$1`, ids.op).
			Scan(&resState, &chkState, &outcome, &notices); err != nil {
			t.Fatal(err)
		}
		if resState != "vencida_pago" || chkState != "expired" || outcome != "cancelled" || notices != 1 {
			t.Fatalf("unexpected late expired state: res=%q chk=%q outcome=%q notices=%d", resState, chkState, outcome, notices)
		}

		// If MP approved payment before/around expiry, late reconcile recovers paid reservation.
		idsApproved := seedPendingCheckoutFixture(t, ctx, pool, *clock, (*clock).Add(5*time.Minute), "10006")
		mp2 := &fakeMercadoPagoServer{}
		srv2 := newFakeMercadoPagoServer(t, mp2)
		svc.SetPendingPaymentCancellation(repo, staticSellerTokens{token: "seller-oauth-token"}, &checkoutpro.HTTPGateway{BaseURL: srv2.URL, Client: srv2.Client()}, func() time.Time { return *clock })
		if res, err := svc.CancelPendingPayment(ctx, idsApproved.renter, idsApproved.op); err != nil || res.PaymentStatus != "pending" {
			t.Fatalf("expected pending, got %+v err=%v", res, err)
		}
		*clock = (*clock).Add(10 * time.Minute)
		if _, err := repo.PurgeExpiredCommercialQuotes(ctx, *clock, 100); err != nil {
			t.Fatal(err)
		}
		mp2.mu.Lock()
		mp2.payment = map[string]any{
			"id": 10006, "collector_id": 91, "external_reference": idsApproved.op,
			"status": "approved", "transaction_amount": 12000, "currency_id": "CLP",
			"live_mode": true, "payment_type_id": "account_money",
			"metadata": map[string]any{"operation_id": idsApproved.op}, "date_last_updated": (*clock).Format(time.RFC3339),
		}
		mp2.mu.Unlock()
		if err := svc.ReconcilePendingPaymentCancellations(ctx, 10); err != nil {
			t.Fatalf("late approved reconcile on expired checkout failed: %v", err)
		}
		var activeHold bool
		if err := pool.QueryRow(ctx, `SELECT r.estado,c.estado,i.resultado,o.activo
			FROM public.reserva_arriendo_comercial r JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id JOIN public.checkout_cancelacion_pendiente i ON i.operacion_id=c.id JOIN public.ocupacion o ON o.id=r.ocupacion_id WHERE r.id=$1`, idsApproved.op).
			Scan(&resState, &chkState, &outcome, &activeHold); err != nil {
			t.Fatal(err)
		}
		if resState != "pagada" || chkState != "approved" || outcome != "approved" || !activeHold {
			t.Fatalf("expired approval recovery failed: res=%q chk=%q outcome=%q active=%t", resState, chkState, outcome, activeHold)
		}
	})
}
