package rentalflowpg

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestPostgresEarlyRentalReadCanceledSentPayment exercises the canonical V99
// lifecycle and reader against a canceled offer whose payment was sent first.
func TestPostgresEarlyRentalReadCanceledSentPayment(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	dsn := dbURL.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	acl, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := acl.Close(cleanup); err != nil {
			t.Errorf("close payment fixture ACL connection: %v", err)
		}
	})
	if _, err := acl.Exec(ctx, `GRANT UPDATE (activo,desactivada_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	var activeGrant, deactivatedGrant bool
	if err := acl.QueryRow(ctx, `SELECT has_column_privilege('espacigo_runtime','public.ocupacion','activo','UPDATE'),has_column_privilege('espacigo_runtime','public.ocupacion','desactivada_en','UPDATE')`).Scan(&activeGrant, &deactivatedGrant); err != nil || !activeGrant || !deactivatedGrant {
		t.Fatalf("minimal occupancy release grants: %v %v %v", activeGrant, deactivatedGrant, err)
	}
	_ = acl.Close(ctx)
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
	record := earlyInsertRecord(t, ids, now)
	commitEarlyReadFixtureOffer(t, ctx, dsn, record)

	want := record.Offer
	steps := []struct {
		actor string
		cmd   earlyrental.Command
		at    time.Time
	}{
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(time.Minute)},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(2 * time.Minute)},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: want.ContractVersion, ContractDigest: want.ContractDigest}, now.Add(3 * time.Minute)},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: want.ContractVersion, ContractDigest: want.ContractDigest}, now.Add(4 * time.Minute)},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: uuid.NewString()}, now.Add(5 * time.Minute)},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}, now.Add(6 * time.Minute)},
	}
	for _, step := range steps {
		step.cmd.ExpectedRevision = want.Revision
		next, event, err := earlyrental.Apply(want, step.actor, step.cmd, step.at)
		if err != nil {
			t.Fatalf("derive lifecycle %s at revision %d: %v", step.cmd.Kind, want.Revision, err)
		}
		applyCanceledSentPaymentStep(t, ctx, dsn, ids, want, next, event, step.at)
		want = next
	}
	if err := earlyrental.ValidateOffer(want); err != nil {
		t.Fatalf("expected canceled aggregate: %v", err)
	}
	if want.State != earlyrental.StateCancelled || want.PaymentID == "" || want.PaymentSentAt == nil || want.PaymentReceivedAt != nil {
		t.Fatalf("fixture did not preserve sent-only payment: %+v", want)
	}

	before := earlyOfferReadCounts(t, ctx, dsn)
	paymentReader := connectEarlyReadAdmin(t, ctx, dsn)
	var paymentBefore int64
	if err := paymentReader.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_pago`).Scan(&paymentBefore); err != nil {
		t.Fatal(err)
	}
	var paymentState string
	var storedPaymentID uuid.UUID
	var sent time.Time
	var received *time.Time
	if err := paymentReader.QueryRow(ctx, `SELECT pago_id,estado,enviado_en,recibido_en FROM public.arriendo_temprano_pago WHERE arriendo_id=$1`, ids.offer).Scan(&storedPaymentID, &paymentState, &sent, &received); err != nil {
		t.Fatal(err)
	}
	if paymentBefore != 1 || storedPaymentID.String() != want.PaymentID || paymentState != "anulado" || !sent.Equal(*want.PaymentSentAt) || received != nil {
		t.Fatal("payment lifecycle fixture did not persist sent-to-annulled evidence")
	}
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := readEarlyRentalOffer(ctx, tx, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer))
	if err != nil {
		t.Fatalf("read canceled sent-payment offer: %v", err)
	}
	if !reflect.DeepEqual(got.Offer, want) || !got.CreatedAt.Equal(record.CreatedAt.UTC()) || !got.UpdatedAt.Equal(steps[len(steps)-1].at.UTC()) || got.LastEventSequence != want.Revision {
		t.Fatalf("read differs from Apply aggregate/snapshot: got=%+v updated=%s seq=%d want=%+v", got.Offer, got.UpdatedAt, got.LastEventSequence, want)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
		t.Fatalf("reader changed counts: %v -> %v", before, after)
	}
	var paymentAfter int64
	if err := paymentReader.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_pago`).Scan(&paymentAfter); err != nil || paymentAfter != paymentBefore {
		t.Fatalf("reader changed payment rows: %d -> %d (%v)", paymentBefore, paymentAfter, err)
	}
}

func applyCanceledSentPaymentStep(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, prior, next earlyrental.Offer, event earlyrental.Event, at time.Time) {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	var priorState string
	if err := tx.QueryRow(ctx, `SELECT estado FROM public.arriendo_temprano WHERE id=$1 FOR UPDATE`, ids.offer).Scan(&priorState); err != nil {
		t.Fatal(err)
	}
	if priorState != string(prior.State) {
		t.Fatalf("persisted prior state=%s, Apply prior=%s", priorState, prior.State)
	}
	var confirmationID, paymentID any
	switch event.Kind {
	case earlyrental.EventIdentityConfirmed:
		var id uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,confirmada_en) VALUES($1,$2,'identidad',$3) RETURNING id`, ids.offer, event.ActorID, at).Scan(&id)
		confirmationID = id
	case earlyrental.EventContractAccepted:
		var id uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest,confirmada_en) SELECT id,$2,'contrato',contrato_version,contrato_digest,$3 FROM public.arriendo_temprano WHERE id=$1 RETURNING id`, ids.offer, event.ActorID, at).Scan(&id)
		confirmationID = id
	case earlyrental.EventPaymentMarkedSent:
		var id uuid.UUID
		if event.PaymentID == nil || *event.PaymentID != next.PaymentID {
			t.Fatal("Apply payment event missing its payment identity")
		}
		err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_pago(arriendo_id,pago_id,emisor_id,receptor_id,monto_clp,moneda,estado,enviado_en) SELECT id,$2,arrendatario_id,anfitrion_id,subtotal_clp,moneda,'pendiente',NULL FROM public.arriendo_temprano WHERE id=$1 RETURNING pago_id`, ids.offer, next.PaymentID).Scan(&id)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano_pago SET estado='enviado',enviado_en=$2 WHERE arriendo_id=$1`, ids.offer, at)
		}
		paymentID = id
	case earlyrental.EventOfferCancelled:
		// The sent payment is retained as evidence but terminalized as anulled;
		// V99 permits this transition while preserving enviado_en.
		if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano_pago SET estado='anulado' WHERE arriendo_id=$1 AND estado='enviado'`, ids.offer); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$2 WHERE id=$1`, ids.occupancy, at); err != nil {
			t.Fatal(err)
		}
		paymentID = next.PaymentID
	}
	if err != nil {
		t.Fatalf("insert lifecycle evidence %s: %v", event.Kind, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$2,revision=$3,actualizada_en=$4 WHERE id=$1`, ids.offer, string(next.State), next.Revision, at); err != nil {
		t.Fatalf("update aggregate %s: %v", event.Kind, err)
	}
	var reason any
	if event.ReasonCode != "" {
		reason = event.ReasonCode
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,ocurrido_en,estado_anterior,estado_nuevo,confirmacion_id,pago_id,motivo_codigo) VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.offer, event.Sequence, string(event.Kind), event.ActorID, event.At, string(event.PreviousState), string(event.NextState), confirmationID, paymentID, reason); err != nil {
		t.Fatalf("append lifecycle event %s: %v", event.Kind, err)
	}
	if event.Kind == earlyrental.EventOfferCancelled {
		var canUpdate bool
		if err := tx.QueryRow(ctx, `SELECT has_column_privilege('espacigo_runtime','public.ocupacion','activo','UPDATE') AND has_column_privilege('espacigo_runtime','public.ocupacion','desactivada_en','UPDATE')`).Scan(&canUpdate); err != nil || !canUpdate {
			t.Fatalf("runtime occupancy release UPDATE columns missing: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit canonical V99 lifecycle step %s: %v", event.Kind, err)
	}
}
