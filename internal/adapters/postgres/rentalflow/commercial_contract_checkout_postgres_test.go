package rentalflowpg

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cooperative opt-in: the isolated migration harness supplies one synthetic
// ready_for_payment agreement with both signatures and a live unconsumed hold.
// No provider is contacted; this verifies only the committed local transition.
func TestCommercialContractCheckoutPostgresConvertsSignedHold(t *testing.T) {
	if os.Getenv("TEST_COMMERCIAL_CHECKOUT_POSTGRES") != "1" {
		t.Skip("requires disposable local PostgreSQL, V120 and a synthetic signed ready agreement")
	}
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || (!strings.Contains(u.Path, "contrat") && !strings.Contains(u.Path, "test_")) {
		t.Fatal("isolated contratos/test database on loopback required")
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`)
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var requestID, quoteID, renterID, holdID string
	var version int64
	var digest []byte
	err = pool.QueryRow(ctx, `SELECT s.id::text,s.cotizacion_id::text,s.arrendatario_id::text,s.ocupacion_id::text,s.propuesta_version,s.documento_sha256 FROM public.contrato_solicitud_comercial s JOIN public.ocupacion o ON o.id=s.ocupacion_id WHERE s.estado='ready_for_payment' AND s.pago_vence_en>now() AND o.tipo='retencion_contrato_comercial' AND o.activo ORDER BY s.creada_en LIMIT 1`).Scan(&requestID, &quoteID, &renterID, &holdID, &version, &digest)
	if err != nil {
		t.Fatalf("synthetic ready agreement fixture required: %v", err)
	}
	r, _ := NewRepository(pool, pool, time.Now)
	r.RequireCommercialContracts(true)
	key, id := uuid.NewString(), uuid.NewString()
	fingerprint := bytes.Repeat([]byte{7}, 32)
	q, op, created, err := r.BeginPayment(ctx, renterID, quoteID, key, id, fingerprint, time.Now().UTC())
	if err != nil || !created || op.ID != id {
		t.Fatalf("begin created=%v err=%v", created, err)
	}
	var linkedRequest, linkedHold, holdType, reservationState string
	var storedVersion int64
	var storedDigest []byte
	var active bool
	err = pool.QueryRow(ctx, `SELECT c.solicitud_comercial_id::text,c.contrato_version,c.contrato_sha256,r.ocupacion_id::text,r.estado,o.tipo,o.activo FROM public.checkout_preferencia_comercial c JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id JOIN public.ocupacion o ON o.id=r.ocupacion_id WHERE c.id=$1`, op.ID).Scan(&linkedRequest, &storedVersion, &storedDigest, &linkedHold, &reservationState, &holdType, &active)
	if err != nil || linkedRequest != requestID || storedVersion != version || !bytes.Equal(storedDigest, digest) || linkedHold != holdID || holdType != "retencion_comercial" || !active || reservationState != "pendiente_de_pago" {
		t.Fatalf("atomic binding/hold conversion failed: %v", err)
	}
	q2, replay, created, err := r.BeginPayment(ctx, renterID, quoteID, key, uuid.NewString(), fingerprint, time.Now().UTC())
	if err != nil || created || replay.ID != op.ID || !q2.ExpiresAt.Equal(q.ExpiresAt) {
		t.Fatalf("idempotent checkout failed created=%v err=%v", created, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.ocupacion WHERE solicitud_comercial_id=$1`, requestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate hold count=%d err=%v", count, err)
	}
	if os.Getenv("TEST_COMMERCIAL_DOCUMENTS_POSTGRES") == "1" {
		payment := checkoutpro.VerifiedPayment{
			OperationID: op.ID, ExternalReference: op.ID, ProviderPaymentID: "120121", SellerID: "31001",
			Status: "approved", TransactionAmountCLP: q.SubtotalCLP, Currency: "CLP", PaymentType: "credit_card",
			PaymentUpdatedAt: time.Now().UTC(),
		}
		if duplicate, err := r.ApplyVerifiedPayment(ctx, payment); err != nil || duplicate {
			t.Fatalf("first approval duplicate=%t err=%v", duplicate, err)
		}
		var objectID, state string
		if err := pool.QueryRow(ctx, `SELECT object_id::text, estado FROM public.contrato_documento_comercial WHERE operacion_id=$1`, op.ID).Scan(&objectID, &state); err != nil || state != "pending" {
			t.Fatalf("document was not queued with payment: state=%q err=%v", state, err)
		}
		if duplicate, err := r.ApplyVerifiedPayment(ctx, payment); err != nil || !duplicate {
			t.Fatalf("repeated approval duplicate=%t err=%v", duplicate, err)
		}
		var replayObject string
		if err := pool.QueryRow(ctx, `SELECT object_id::text FROM public.contrato_documento_comercial WHERE operacion_id=$1`, op.ID).Scan(&replayObject); err != nil || replayObject != objectID {
			t.Fatalf("document identity changed on receipt replay: %v", err)
		}
	}
}
