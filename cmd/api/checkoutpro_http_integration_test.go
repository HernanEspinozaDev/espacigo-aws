package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	checkoutnoticepg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/checkoutnotice"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutnotice"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	rentalflowhttp "github.com/HernanEspinozaDev/espaciGo/internal/rentalflow/transport/http"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const checkoutWebhookTestSecret = "synthetic-local-checkout-webhook-key-only"

type checkoutIntegrationAuthenticator struct{ renterID string }

func (a checkoutIntegrationAuthenticator) Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error) {
	return identity.Principal{AccountID: a.renterID}, nil
}

type checkoutIntegrationTokens struct {
	hostID string
	token  []byte
}

func (s checkoutIntegrationTokens) AccessToken(_ context.Context, hostID string) ([]byte, error) {
	if hostID != s.hostID {
		return nil, checkoutpro.ErrHostNotConnected
	}
	return append([]byte(nil), s.token...), nil
}

type checkoutIntegrationGateway struct {
	mu       sync.Mutex
	creates  int
	request  checkoutpro.PreferenceRequest
	provider checkoutpro.Preference
}

func (g *checkoutIntegrationGateway) PaymentTypes(context.Context, []byte) ([]string, error) {
	return []string{"account_money", "credit_card"}, nil
}
func (g *checkoutIntegrationGateway) Create(_ context.Context, _ []byte, req checkoutpro.PreferenceRequest) (checkoutpro.Preference, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.creates++
	g.request = req
	return g.provider, nil
}
func (g *checkoutIntegrationGateway) FindByExternalReference(context.Context, []byte, string) (checkoutpro.Preference, error) {
	return checkoutpro.Preference{}, checkoutpro.ErrNotFound
}

type checkoutIntegrationProvider struct {
	mu      sync.Mutex
	result  checkoutpro.VerifiedPayment
	calls   int
	gotID   string
	tokenOK bool
}

func (p *checkoutIntegrationProvider) LookupPayment(_ context.Context, token []byte, paymentID string) (checkoutpro.VerifiedPayment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.gotID = paymentID
	p.tokenOK = string(token) == "synthetic-seller-token"
	return p.result, nil
}

type checkoutIntegrationMailer struct {
	mu         sync.Mutex
	recipients []string
}

func (m *checkoutIntegrationMailer) SendLocalBookingNotice(_ context.Context, recipient, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recipients = append(m.recipients, recipient)
	return nil
}

// This test intentionally needs the opt-in, retained, networkless V106 fixture.
// It exercises the HTTP preference route, the mounted signed webhook receiver,
// real PostgreSQL claim/approval/outbox transactions, and a fake provider/mail
// boundary. It never connects to Mercado Pago or sends external mail.
func TestCheckoutPreferenceSignedWebhookAndNoticesPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_CHECKOUTPRO_DATABASE_URL"))
	if dsn == "" {
		t.Skip("requires the opt-in retained test-only PostgreSQL preview")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" || u.User == nil || u.User.Username() != "espacigo_api_login" || u.Path != "/espacigo_prod" || u.Query().Get("sslmode") != "disable" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires the exact local preview DSN for espacigo_api_login@espacigo_prod over its Unix socket")
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		t.Fatal("preview DSN must not embed a password")
	}
	socketDir := u.Query().Get("host")
	marker, err := os.Lstat(filepath.Join(socketDir, ".espacigo-migration-ci"))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0o600 {
		t.Fatal("requires the private marker from the isolated migration preview runner")
	}
	adminURL := *u
	adminURL.User = url.User("espacigo_admin")
	adminConfig, err := pgxpool.ParseConfig(adminURL.String())
	if err != nil {
		t.Fatal("invalid test-only admin connection")
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var latest int
	var latestName, latestChecksum string
	if err := admin.QueryRow(ctx, `SELECT version,name,checksum FROM public.schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest, &latestName, &latestChecksum); err != nil || latest != 106 || latestName != "V000106__checkout_payment_notice_outbox.sql" || latestChecksum != "7f3ba3c503001b6b7521e261b4e995f8c132f82004f7a557d67ed683d9ea45b8" {
		t.Fatalf("requires exact V106 preview ledger, latest=%d %q %q err=%v", latest, latestName, latestChecksum, err)
	}
	runtimeConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE espacigo_runtime")
		return err
	}
	runtime, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var sessionUser, effectiveRole string
	if err := runtime.QueryRow(ctx, "SELECT session_user,current_user").Scan(&sessionUser, &effectiveRole); err != nil || sessionUser != "espacigo_api_login" || effectiveRole != "espacigo_runtime" {
		t.Fatalf("unexpected runtime identity %q/%q: %v", sessionUser, effectiveRole, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	ownerID, renterID, spaceID, quoteID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	operationID := uuid.NewString()
	sellerID := "123456789"
	// A provider payment ID is globally unique in the disposable fixture too;
	// choose a fresh numeric ID so retained evidence from prior runs cannot
	// make this run look like a cross-operation payment replay.
	paymentID := strconv.FormatInt(time.Now().UnixNano(), 10)
	requestID := "checkout-preview-" + uuid.NewString()
	startAt := now.Add(48 * time.Hour).Truncate(30 * time.Minute)
	endAt := startAt.Add(time.Hour)
	expiresAt := now.Add(15 * time.Minute)

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanupTx, err := admin.Begin(cleanupCtx)
		if err != nil {
			t.Errorf("begin Checkout Pro fixture cleanup: %v", err)
			return
		}
		defer func() { _ = cleanupTx.Rollback(context.Background()) }()
		if _, err = cleanupTx.Exec(cleanupCtx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
			t.Errorf("defer Checkout Pro fixture cleanup constraints: %v", err)
			return
		}
		for _, step := range []struct {
			name string
			sql  string
			args []any
		}{
			{"notice outbox", `DELETE FROM public.checkout_payment_notice_outbox WHERE operation_id=$1::uuid`, []any{operationID}},
			{"webhook receipt", `DELETE FROM public.mercadopago_payment_webhook_receipt WHERE provider_request_id=$1`, []any{requestID}},
		} {
			if _, err = cleanupTx.Exec(cleanupCtx, step.sql, step.args...); err != nil {
				t.Errorf("delete Checkout Pro fixture %s: %v", step.name, err)
				return
			}
		}
		var occupancyID string
		occupancyErr := cleanupTx.QueryRow(cleanupCtx, `SELECT ocupacion_id::text FROM public.reserva_arriendo_comercial WHERE operacion_checkout_id=$1::uuid`, operationID).Scan(&occupancyID)
		if occupancyErr != nil && occupancyErr != pgx.ErrNoRows {
			t.Errorf("find Checkout Pro fixture occupancy: %v", occupancyErr)
			return
		}
		if occupancyErr == nil {
			if _, err = cleanupTx.Exec(cleanupCtx, `UPDATE public.ocupacion SET tipo='bloqueo_manual',reserva_arriendo_comercial_id=NULL,expira_en=NULL,motivo='checkout fixture cleanup' WHERE id=$1::uuid`, occupancyID); err != nil {
				t.Errorf("detach Checkout Pro fixture occupancy: %v", err)
				return
			}
		}
		for _, step := range []struct {
			name string
			sql  string
			args []any
		}{
			{"commercial reservation", `DELETE FROM public.reserva_arriendo_comercial WHERE operacion_checkout_id=$1::uuid`, []any{operationID}},
			{"occupancy", `DELETE FROM public.ocupacion WHERE id=NULLIF($1,'')::uuid`, []any{occupancyID}},
			{"checkout operation", `DELETE FROM public.checkout_preferencia_comercial WHERE id=$1::uuid`, []any{operationID}},
			{"seller connection", `DELETE FROM public.mercadopago_seller_connection WHERE anfitrion_id=$1::uuid`, []any{ownerID}},
			{"commercial quote", `DELETE FROM public.cotizacion_arriendo_comercial WHERE id=$1::uuid`, []any{quoteID}},
			{"space rate", `DELETE FROM public.tarifa_espacio WHERE espacio_id=$1::uuid`, []any{spaceID}},
			{"space", `DELETE FROM public.espacio WHERE id=$1::uuid`, []any{spaceID}},
			{"users", `DELETE FROM public.usuario WHERE id IN ($1::uuid,$2::uuid)`, []any{ownerID, renterID}},
		} {
			if _, err = cleanupTx.Exec(cleanupCtx, step.sql, step.args...); err != nil {
				t.Errorf("delete Checkout Pro fixture %s: %v", step.name, err)
				return
			}
		}
		if err = cleanupTx.Commit(cleanupCtx); err != nil {
			t.Errorf("commit Checkout Pro fixture cleanup: %v", err)
		}
	}()

	for id, email := range map[string]string{ownerID: ownerID + "@checkout-preview.invalid", renterID: renterID + "@checkout-preview.invalid"} {
		if _, err := admin.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'synthetic-only','activo')`, id, email); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado,zona_horaria)
VALUES($1,$2,'oficina','Checkout webhook fixture',repeat('Descripción sintética para pruebas. ',4),10,1,'Solo pruebas','hora',10000,'Ubicación sintética','activa','UTC')`, spaceID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) VALUES($1,1,'hora',10000)`, spaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial(id,espacio_id,propietario_id,arrendatario_id,titulo_espacio,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,creada_en,vence_en)
VALUES($1,$2,$3,$4,'Checkout webhook fixture',1,'hora',10000,'CLP',1,10000,$5,$6,'UTC',$7,$8)`, quoteID, spaceID, ownerID, renterID, startAt, endAt, now, expiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.mercadopago_seller_connection(anfitrion_id,seller_id,encrypted_access_token,encrypted_refresh_token,token_expires_at,scopes,live_mode,connection_state,connected_at,updated_at)
VALUES($1,$2,decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),$3,'{}',false,'active',$4,$4)`, ownerID, sellerID, now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}

	repo, err := rentalflowpg.NewRepository(runtime, runtime, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	tokens := checkoutIntegrationTokens{hostID: ownerID, token: []byte("synthetic-seller-token")}
	initPoint := "https://www.mercadopago.com/checkout/test-preference"
	preferenceGateway := &checkoutIntegrationGateway{provider: checkoutpro.Preference{ID: "synthetic-pref-" + uuid.NewString(), InitPoint: initPoint}}
	checkoutService, err := checkoutpro.New(repo, tokens, preferenceGateway, checkoutpro.Config{FrontendOrigin: "https://checkout.example.invalid", TestMode: true}, func() time.Time { return now }, func() string { return operationID })
	if err != nil {
		t.Fatal(err)
	}
	checkoutHandler := rentalflowhttp.New(checkoutIntegrationAuthenticator{renterID: renterID}, nil, nil, nil, nil, checkoutService)
	provider := &checkoutIntegrationProvider{result: checkoutpro.VerifiedPayment{
		OperationID: operationID, ProviderPaymentID: paymentID, SellerID: sellerID, ExternalReference: operationID,
		Status: "approved", TransactionAmountCLP: 10000, Currency: "CLP", PaymentType: "account_money",
		LiveMode: false, ObservedAt: now, PaymentUpdatedAt: now.Add(time.Second),
	}}
	secretPath := filepath.Join(t.TempDir(), "webhook-secret")
	if err := os.WriteFile(secretPath, []byte(checkoutWebhookTestSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "MERCADOPAGO_WEBHOOK_SECRET_FILE" {
			return secretPath
		}
		return ""
	}
	webhookHandler, worker, err := newCheckoutWebhookWithProvider(repo, runtime, tokens, getenv, provider)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", checkoutHandler)
	if err := mountCheckoutWebhook(mux, webhookHandler); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	httpClient := httpServer.Client()

	startRequest := func() checkoutpro.StartResult {
		req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/rental-quotes/"+quoteID+"/checkout", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer synthetic-auth")
		req.Header.Set("Idempotency-Key", "checkout-preview-idempotency")
		res, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			var operationState string
			queryErr := admin.QueryRow(ctx, `SELECT estado FROM public.checkout_preferencia_comercial WHERE id=$1::uuid`, operationID).Scan(&operationState)
			preferenceGateway.mu.Lock()
			createCount := preferenceGateway.creates
			preferenceGateway.mu.Unlock()
			_, directErr := checkoutService.Start(ctx, renterID, quoteID, "diagnostic-idempotency")
			t.Fatalf("preference HTTP returned %d: %s; operation_state=%q query_err=%v gateway_create_calls=%d direct_start_err=%v", res.StatusCode, body, operationState, queryErr, createCount, directErr)
		}
		var envelope struct {
			Data checkoutpro.StartResult `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}
	started := startRequest()
	if started.OperationID != operationID || started.ReservationID != operationID || started.Status != "pending" || started.InitPoint != initPoint {
		t.Fatalf("unexpected preference response: %+v", started)
	}
	if got := startRequest(); got.OperationID != started.OperationID {
		t.Fatalf("idempotent preference changed operation: first=%+v second=%+v", started, got)
	}
	preferenceGateway.mu.Lock()
	createCount := preferenceGateway.creates
	providerReference := preferenceGateway.request.ExternalReference
	providerFee := preferenceGateway.request.MarketplaceFee
	preferenceGateway.mu.Unlock()
	if createCount != 1 || providerReference != operationID || providerFee != 357 {
		t.Fatalf("preference request wasn't idempotent or quote-derived: calls=%d reference=%q fee=%v", createCount, providerReference, providerFee)
	}

	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(workerCtx) }()
	defer func() {
		stopWorker()
		select {
		case <-workerDone:
		case <-time.After(3 * time.Second):
			t.Error("payment worker did not stop")
		}
	}()

	ts := fmt.Sprint(time.Now().Unix())
	requestBody := fmt.Sprintf(`{"type":"payment","action":"payment.updated","user_id":%s,"data":{"id":%s}}`, sellerID, paymentID)
	webhookReq, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/webhooks/mercadopago", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	webhookReq.Header.Set("x-request-id", requestID)
	webhookReq.Header.Set("x-signature", signCheckoutPreviewWebhook([]byte(checkoutWebhookTestSecret), paymentID, requestID, ts))
	webhookRes, err := httpClient.Do(webhookReq)
	if err != nil {
		t.Fatal(err)
	}
	webhookBody, _ := io.ReadAll(webhookRes.Body)
	webhookRes.Body.Close()
	if webhookRes.StatusCode != http.StatusOK {
		t.Fatalf("signed webhook HTTP returned %d: %s", webhookRes.StatusCode, webhookBody)
	}

	waitForCheckoutState(t, ctx, admin, operationID, "approved")
	var reservationState, occupancyType string
	var occupancyActive bool
	if err := admin.QueryRow(ctx, `SELECT r.estado,o.tipo,o.activo FROM public.reserva_arriendo_comercial r JOIN public.ocupacion o ON o.reserva_arriendo_comercial_id=r.id WHERE r.operacion_checkout_id=$1::uuid`, operationID).Scan(&reservationState, &occupancyType, &occupancyActive); err != nil {
		t.Fatal(err)
	}
	if reservationState != "pagada" || occupancyType != "reserva_comercial" || !occupancyActive {
		t.Fatalf("payment did not atomically confirm held reservation: reservation=%q occupancy=%q active=%t", reservationState, occupancyType, occupancyActive)
	}
	var receiptState string
	if err := admin.QueryRow(ctx, `SELECT estado FROM public.mercadopago_payment_webhook_receipt WHERE provider_request_id=$1`, requestID).Scan(&receiptState); err != nil || receiptState != "processed" {
		t.Fatalf("webhook receipt not durably processed: state=%q err=%v", receiptState, err)
	}
	var notices int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1::uuid AND transition='approved'`, operationID).Scan(&notices); err != nil || notices != 2 {
		t.Fatalf("approval should enqueue exactly two notices, got %d err=%v", notices, err)
	}

	// A duplicate delivery with the same signed request ID must not create new
	// payment state or additional email intents.
	dupReq, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/webhooks/mercadopago", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	dupReq.Header.Set("x-request-id", requestID)
	dupReq.Header.Set("x-signature", signCheckoutPreviewWebhook([]byte(checkoutWebhookTestSecret), paymentID, requestID, ts))
	dupRes, err := httpClient.Do(dupReq)
	if err != nil {
		t.Fatal(err)
	}
	dupBody, _ := io.ReadAll(dupRes.Body)
	dupRes.Body.Close()
	if dupRes.StatusCode != http.StatusOK {
		t.Fatalf("duplicate webhook HTTP returned %d: %s", dupRes.StatusCode, dupBody)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1::uuid AND transition='approved'`, operationID).Scan(&notices); err != nil || notices != 2 {
		t.Fatalf("duplicate delivery created extra notices: count=%d err=%v", notices, err)
	}
	provider.mu.Lock()
	providerCalls, lookedUpID, tokenOK := provider.calls, provider.gotID, provider.tokenOK
	provider.mu.Unlock()
	if providerCalls != 1 || lookedUpID != paymentID || !tokenOK {
		t.Fatalf("worker provider lookup mismatch: calls=%d id=%q token_ok=%t", providerCalls, lookedUpID, tokenOK)
	}

	mailer := &checkoutIntegrationMailer{}
	noticeService, err := checkoutnotice.New(checkoutnoticepg.New(runtime), mailer, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		dispatched, err := noticeService.DispatchOne(ctx)
		if err != nil || !dispatched {
			t.Fatalf("notice dispatch %d failed: dispatched=%t err=%v", i+1, dispatched, err)
		}
	}
	if dispatched, err := noticeService.DispatchOne(ctx); err != nil || dispatched {
		t.Fatalf("notice outbox was not drained: dispatched=%t err=%v", dispatched, err)
	}
	mailer.mu.Lock()
	gotRecipients := append([]string(nil), mailer.recipients...)
	mailer.mu.Unlock()
	if len(gotRecipients) != 2 {
		t.Fatalf("SMTP fake received %d notices, want renter and host: %v", len(gotRecipients), gotRecipients)
	}
	var delivered int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM public.checkout_payment_notice_outbox WHERE operation_id=$1::uuid AND state='delivered'`, operationID).Scan(&delivered); err != nil || delivered != 2 {
		t.Fatalf("outbox delivered readback failed: count=%d err=%v", delivered, err)
	}
}

func signCheckoutPreviewWebhook(secret []byte, paymentID, requestID, timestamp string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(fmt.Sprintf("id:%s;request-id:%s;ts:%s;", paymentID, requestID, timestamp)))
	return "ts=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func waitForCheckoutState(t *testing.T, ctx context.Context, db *pgxpool.Pool, operationID, want string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		err := db.QueryRow(ctx, `SELECT estado FROM public.checkout_preferencia_comercial WHERE id=$1::uuid`, operationID).Scan(&state)
		if err == nil && state == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	var state, reason, providerStatus, reservationState, occupancyType string
	err := db.QueryRow(ctx, `SELECT c.estado,COALESCE(c.reconciliation_reason,''),COALESCE(c.provider_payment_status,''),COALESCE(r.estado,''),COALESCE(o.tipo,'')
FROM public.checkout_preferencia_comercial c LEFT JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id
LEFT JOIN public.ocupacion o ON o.reserva_arriendo_comercial_id=r.id WHERE c.id=$1::uuid`, operationID).Scan(&state, &reason, &providerStatus, &reservationState, &occupancyType)
	t.Fatalf("checkout operation did not reach %q, got state=%q reason=%q provider_status=%q reservation=%q occupancy=%q: %v", want, state, reason, providerStatus, reservationState, occupancyType, err)
}
