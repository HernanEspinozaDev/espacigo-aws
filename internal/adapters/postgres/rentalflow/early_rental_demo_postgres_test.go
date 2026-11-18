package rentalflowpg

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/fakebooking"
	"github.com/HernanEspinozaDev/espaciGo/internal/booking"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type rejectedEarlyRentalProcessor struct {
	booking.LocalPaymentAdapter
}

func (rejectedEarlyRentalProcessor) StartPayment(context.Context, string, string) (*booking.PaymentEvent, error) {
	return &booking.PaymentEvent{Outcome: "rechazo_simulado"}, nil
}

func TestPostgresEarlyRentalLocalProcessorPayment(t *testing.T) {
	ctx, url := earlyInsertPostgresFixture(t)
	dsn := url.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	admin := connectEarlyReadAdmin(t, ctx, dsn)
	migration, err := os.ReadFile("../../../../db/migrations/V000100__early_rental_local_payment.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(migration)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `RESET ROLE`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `GRANT UPDATE (tipo, expira_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}

	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
	record := earlyInsertRecord(t, ids, now)
	commitEarlyReadFixtureOffer(t, ctx, dsn, record)
	seedEarlyRentalPaymentSendReady(t, ctx, dsn, ids, record, now)
	renter, host, id := uuid.MustParse(ids.renter), uuid.MustParse(ids.host), uuid.MustParse(ids.offer)
	conn := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(conn, conn, func() time.Time { return now.Add(5 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	processor, err := fakebooking.NewWithClock(bytes.Repeat([]byte{1}, 32), repo.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PayEarlyRental(ctx, host, id, 5, processor); !errors.Is(err, earlyrental.ErrForbidden) {
		t.Fatalf("host payment should be refused, got %v", err)
	}
	if _, err := repo.PayEarlyRental(ctx, renter, id, 5, rejectedEarlyRentalProcessor{}); err == nil {
		t.Fatal("expected the processor result to be rejected")
	}
	failed, err := repo.GetEarlyRental(ctx, renter, id)
	if err != nil || failed.Offer.Revision != 5 || failed.Offer.PaymentID != "" {
		t.Fatalf("processor rejection did not roll back payment and revision: %v", err)
	}

	paid, err := repo.PayEarlyRental(ctx, renter, id, 5, processor)
	if err != nil || paid.Offer.State != earlyrental.StateConfirmed || paid.Offer.PaymentReceivedAt == nil {
		t.Fatalf("processor approval did not confirm rental: %v", err)
	}
	replay, err := repo.PayEarlyRental(ctx, renter, id, 5, processor)
	if err != nil || paid.Offer.PaymentID != replay.Offer.PaymentID || paid.Offer.Revision != replay.Offer.Revision {
		t.Fatalf("retry changed approved payment: %v", err)
	}
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		events, err := readEarlyRentalEvents(ctx, tx, host, id, 5)
		if err != nil {
			return err
		}
		if len(events) != 2 || events[1].ActorID != "system" || events[1].Kind != earlyrental.EventPaymentMarkedReceived {
			t.Fatalf("missing processor approval event: %+v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var occupancy, payment, reference string
	if err := admin.QueryRow(ctx, `SELECT o.tipo,p.estado,e.procesador_referencia
		FROM public.ocupacion o JOIN public.arriendo_temprano_pago p ON p.arriendo_id=o.arriendo_temprano_id
		JOIN public.arriendo_temprano_evento e ON e.arriendo_id=p.arriendo_id AND e.tipo='pago_marcado_recibido'
		WHERE p.arriendo_id=$1`, id).Scan(&occupancy, &payment, &reference); err != nil {
		t.Fatal(err)
	}
	if occupancy != "reserva_temprana" || payment != "recibido" || reference != "localfake:"+paid.Offer.PaymentID+":exito_simulado" {
		t.Fatal("persisted payment, processor reference and occupancy do not agree")
	}
}
