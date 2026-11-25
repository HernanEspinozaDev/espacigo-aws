package rentalflowpg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommercialCancellationPostgres(t *testing.T) {
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
		t.Fatal("cancellation integration requires loopback PostgreSQL")
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
	CREATE TABLE public.checkout_preferencia_comercial(id uuid PRIMARY KEY, estado text, provider_payment_id text, total_charged_clp bigint, payment_verified_at timestamptz, provider_payment_status text);
	CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY, politica_cancelacion_version text, politica_cancelacion_texto text);
	CREATE TABLE public.reserva_arriendo_comercial(id uuid PRIMARY KEY, estado text, pago_vence_en timestamptz, operacion_checkout_id uuid, cotizacion_id uuid, ocupacion_id uuid, inicio timestamptz, anfitrion_id uuid, arrendatario_id uuid, actualizada_en timestamptz);
	CREATE TABLE public.ocupacion(id uuid PRIMARY KEY, activo boolean, desactivada_en timestamptz, reserva_arriendo_comercial_id uuid);
	CREATE TABLE public.reserva_cuota_mensual(reserva_arriendo_comercial_id uuid, estado text);
	CREATE TABLE public.notificacion_usuario(id uuid PRIMARY KEY, usuario_id uuid, tipo text, titulo text, mensaje text, href text, referencia_id uuid, creada_en timestamptz);
	CREATE UNIQUE INDEX notificacion_owner ON public.notificacion_usuario(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../../db/migrations/V000002__reservas_comerciales_postventa.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	host, renter, reservationID, quote, firstHold, secondHold := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO public.usuario VALUES($1),($2)`, host, renter)
	batch.Queue(`INSERT INTO public.checkout_preferencia_comercial VALUES($1,'approved','10001',12000,$2,'approved')`, reservationID, now)
	batch.Queue(`INSERT INTO public.cotizacion_arriendo_comercial VALUES($1,'facil_v1','Devolución completa antes del inicio.')`, quote)
	batch.Queue(`INSERT INTO public.reserva_arriendo_comercial VALUES($1,'pagada',$2,$1,$3,$4,$5,$6,$7,$2)`, reservationID, now, quote, firstHold, now.Add(48*time.Hour), host, renter)
	batch.Queue(`INSERT INTO public.ocupacion VALUES($1,true,NULL,$3),($2,true,NULL,$3)`, firstHold, secondHold, reservationID)
	batch.Queue(`INSERT INTO public.reserva_cuota_mensual VALUES($1,'pagada'),($1,'pendiente')`, reservationID)
	if err := pool.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(pool, pool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewCommercialCancellation(ctx, renter, reservationID, now)
	if err != nil || !preview.CanCancel || preview.RefundAmountCLP != 12000 {
		t.Fatalf("bad preview: %+v, %v", preview, err)
	}
	if _, err = repo.BeginCommercialCancellation(ctx, renter, reservationID, "Cambio de planes", 0, now, uuid.NewString()); !errors.Is(err, rentalflow.ErrConflict) {
		t.Fatalf("wrong expected refund should be rejected: %v", err)
	}
	operation, err := repo.BeginCommercialCancellation(ctx, renter, reservationID, "Cambio de planes", 12000, now, uuid.NewString())
	if err != nil || operation.RefundStatus != "pendiente" || operation.IdempotencyKey == "" {
		t.Fatalf("expected durable intent: %+v, %v", operation, err)
	}
	for range 2 {
		result, err := repo.FinishCommercialCancellation(ctx, reservationID, "20001", "approved", now)
		if err != nil || result.Status != "reembolsada" || result.RefundStatus != "aprobado" {
			t.Fatalf("expected approved result: %+v, %v", result, err)
		}
	}
	var active, pending, notifications int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.ocupacion WHERE reserva_arriendo_comercial_id=$1 AND activo),
		(SELECT count(*) FROM public.reserva_cuota_mensual WHERE reserva_arriendo_comercial_id=$1 AND estado='pendiente'),
		(SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1)`, reservationID).Scan(&active, &pending, &notifications); err != nil {
		t.Fatal(err)
	}
	if active != 0 || pending != 0 || notifications != 2 {
		t.Fatalf("expected released holds, canceled pending installments and both notifications, got %d/%d/%d", active, pending, notifications)
	}
}
