package rentalflowpg

import (
	"bytes"
	"context"
	"errors"
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
	if _, err := admin.Exec(ctx, `GRANT UPDATE (tipo, expira_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}

	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
	record := earlyInsertRecord(t, ids, now)
	commitEarlyReadFixtureOffer(t, ctx, dsn, record)
	renter, host, id := uuid.MustParse(ids.renter), uuid.MustParse(ids.host), uuid.MustParse(ids.offer)
	conn := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(conn, conn, func() time.Time { return now.Add(5 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	command := earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity, ExpectedRevision: 1}
	if _, err := repo.ConfirmEarlyRental(ctx, host, id, command); !errors.Is(err, earlyrental.ErrWrongState) {
		t.Fatalf("host approval before renter verification: %v", err)
	}
	for index, actor := range []uuid.UUID{renter, host, renter, host} {
		command.ExpectedRevision = uint64(index + 1)
		if index >= 2 {
			command.Kind = earlyrental.CommandAcceptContract
			command.ContractVersion = record.Offer.ContractVersion
			command.ContractDigest = record.Offer.ContractDigest
		}
		result, err := repo.ConfirmEarlyRental(ctx, actor, id, command)
		if err != nil || result.Offer.Revision != command.ExpectedRevision+1 {
			t.Fatalf("persist confirmation %d: %v", index, err)
		}
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
	for _, actor := range []uuid.UUID{renter, host} {
		offers, more, err := repo.ListDemoEarlyRentals(ctx, actor, "", time.Time{}, uuid.Nil, 25)
		if err != nil || more || len(offers) != 1 || offers[0].Offer.State != earlyrental.StateConfirmed {
			t.Fatalf("paid reservation is missing from participant list: %v", err)
		}
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

func TestPostgresEarlyRentalDemoRejectReleasesInterval(t *testing.T) {
	ctx, url := earlyInsertPostgresFixture(t)
	dsn := url.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
	record := earlyInsertRecord(t, ids, now)
	commitEarlyReadFixtureOffer(t, ctx, dsn, record)
	admin := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := admin.Exec(ctx, `GRANT UPDATE (activo, desactivada_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	conn := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(conn, conn, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	renter, host, id := uuid.MustParse(ids.renter), uuid.MustParse(ids.host), uuid.MustParse(ids.offer)
	command := earlyrental.Command{Kind: earlyrental.CommandReject, ExpectedRevision: 1, ReasonCode: "condiciones_no_aceptadas"}
	if _, err := repo.ConfirmEarlyRental(ctx, renter, id, command); !errors.Is(err, earlyrental.ErrForbidden) {
		t.Fatalf("renter cannot reject as host: %v", err)
	}
	result, err := repo.ConfirmEarlyRental(ctx, host, id, command)
	if err != nil || result.Offer.State != earlyrental.StateRejected {
		t.Fatalf("host rejection failed: %v", err)
	}
	var active bool
	if err := admin.QueryRow(ctx, `SELECT activo FROM public.ocupacion WHERE arriendo_temprano_id=$1`, id).Scan(&active); err != nil || active {
		t.Fatalf("rejection did not release the slot: %v", err)
	}
}
