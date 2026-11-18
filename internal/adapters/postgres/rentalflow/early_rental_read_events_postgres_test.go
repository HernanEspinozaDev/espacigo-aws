package rentalflowpg

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
)

func TestPostgresEarlyRentalReadEvents(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	dsn := dbURL.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	admin := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := admin.Exec(ctx, `GRANT UPDATE (activo,desactivada_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
	record := earlyInsertRecord(t, ids, now)
	commitEarlyReadFixtureOffer(t, ctx, dsn, record)

	offer := record.Offer
	expected := []earlyrental.Event{record.InitialEvent}
	for i, step := range []struct {
		actor string
		cmd   earlyrental.Command
	}{
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: uuid.NewString()}},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}},
	} {
		step.cmd.ExpectedRevision = offer.Revision
		at := now.Add(time.Duration(i+1) * time.Minute)
		next, event, err := earlyrental.Apply(offer, step.actor, step.cmd, at)
		if err != nil {
			t.Fatal(err)
		}
		applyCanceledSentPaymentStep(t, ctx, dsn, ids, offer, next, event, at)
		expected = append(expected, event)
		offer = next
	}
	if offer.Revision != 7 || len(expected) != 7 || expected[5].Kind != earlyrental.EventPaymentMarkedSent || expected[6].Kind != earlyrental.EventOfferCancelled {
		t.Fatal("invalid committed lifecycle fixture")
	}
	outsider := uuid.New()
	insertEarlyReadActor(t, ctx, dsn, outsider, "events-outsider")
	before := earlyOfferReadCounts(t, ctx, dsn)
	paymentCount := func(t *testing.T) int64 {
		t.Helper()
		var count int64
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_pago WHERE arriendo_id=$1`, ids.offer).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if paymentCount(t) != 1 {
		t.Fatal("fixture did not persist exactly one payment")
	}
	read := func(t *testing.T, actor uuid.UUID, after uint64) ([]earlyrental.Event, error) {
		t.Helper()
		_, tx := createEarlyOfferReadTx(t, ctx, dsn)
		got, err := readEarlyRentalEvents(ctx, tx, actor, uuid.MustParse(ids.offer), after)
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); rollbackErr != nil {
			t.Fatalf("end runtime reader transaction: %v", rollbackErr)
		}
		return got, err
	}
	t.Run("FullHistoryBothParticipants", func(t *testing.T) {
		for _, actor := range []string{ids.host, ids.renter} {
			got, err := read(t, uuid.MustParse(actor), 0)
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatalf("history mismatch: got=%+v want=%+v error=%v", got, expected, err)
			}
		}
	})
	t.Run("CompleteSuffixAndEmptyCursor", func(t *testing.T) {
		actor := uuid.MustParse(ids.renter)
		got, err := read(t, actor, 3)
		if err != nil || !reflect.DeepEqual(got, expected[3:]) {
			t.Fatalf("suffix mismatch: got=%+v want=%+v error=%v", got, expected[3:], err)
		}
		for _, after := range []uint64{7, math.MaxInt64} {
			got, err := read(t, actor, after)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("after %d: got=%#v error=%v, want a non-nil empty result", after, got, err)
			}
		}
	})
	t.Run("NonParticipantHiddenAtHighCursor", func(t *testing.T) {
		got, err := read(t, outsider, math.MaxInt64)
		if !errors.Is(err, rentalflow.ErrNotFound) || got != nil {
			t.Fatalf("nonparticipant read: got=%#v error=%v", got, err)
		}
	})
	t.Run("BlockedParticipantHiddenAtHighCursor", func(t *testing.T) {
		if _, err := admin.Exec(ctx, `INSERT INTO public.bloqueo_cuenta_administrativo_local(cuenta_id,bloqueada_por,motivo_codigo,bloqueada_en,correlacion_id) VALUES($1,$2,'revision_administrativa',$3,$4)`, ids.renter, ids.host, now, "events-test-"+uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		got, err := read(t, uuid.MustParse(ids.renter), math.MaxInt64)
		if !errors.Is(err, rentalflow.ErrNotFound) || got != nil {
			t.Fatalf("blocked participant read: got=%#v error=%v", got, err)
		}
	})
	if after := earlyOfferReadCounts(t, ctx, dsn); after != before || paymentCount(t) != 1 {
		t.Fatalf("reader changed counts: %v -> %v", before, after)
	}
}
