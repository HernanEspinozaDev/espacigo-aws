package rentalflowpg

import (
	"context"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func TestPostgresEarlyRentalApplyPaymentSend(t *testing.T) {
	for _, name := range []string{"SendCommit", "CallerRollback"} {
		scenario := name
		t.Run(scenario, func(t *testing.T) {
			ctx, dbURL := earlyInsertPostgresFixture(t)
			dsn := dbURL.String()
			grantEarlyOfferReadColumns(t, ctx, dsn)
			ids := earlyInsertIDs()
			seedEarlyInsertActors(t, ctx, dsn, ids)
			now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
			record := earlyInsertRecord(t, ids, now)
			commitEarlyReadFixtureOffer(t, ctx, dsn, record)
			seedEarlyRentalPaymentSendReady(t, ctx, dsn, ids, record, now)
			before := earlyOfferReadCounts(t, ctx, dsn)
			admin := connectEarlyReadAdmin(t, ctx, dsn)
			var occupancyType string
			var occupancyActive bool
			var occupancyExpiry time.Time
			if err := admin.QueryRow(ctx, `SELECT tipo,activo,expira_en FROM public.ocupacion WHERE id=$1`, uuid.MustParse(ids.occupancy)).Scan(&occupancyType, &occupancyActive, &occupancyExpiry); err != nil {
				t.Fatal(err)
			}
			if occupancyType != "retencion_temprana" || !occupancyActive || !occupancyExpiry.Equal(record.Offer.ExpiresAt) {
				t.Fatalf("pre-send occupancy=%s active=%v expiry=%v", occupancyType, occupancyActive, occupancyExpiry)
			}

			actorID := uuid.MustParse(ids.renter)
			offerID := uuid.MustParse(ids.offer)
			sentAt := now.Add(5 * time.Minute)
			_, tx := createClaimNativeTx(t, ctx, dsn)
			result, err := applyEarlyRentalPaymentSend(ctx, tx, actorID, offerID, earlyrental.Command{Kind: earlyrental.CommandSendPayment, ExpectedRevision: 5}, sentAt)
			if err != nil {
				t.Fatalf("apply renter payment send: %v", err)
			}
			paymentID, parseErr := uuid.Parse(result.Stored.Offer.PaymentID)
			if parseErr != nil || paymentID == uuid.Nil || result.EventID == uuid.Nil || result.Stored.Offer.State != earlyrental.StatePendingPaymentReceive || result.Stored.Offer.Revision != 6 || result.Stored.LastEventSequence != 6 || result.Stored.Offer.PaymentSentAt == nil || !result.Stored.Offer.PaymentSentAt.Equal(sentAt) || result.Stored.Offer.PaymentReceivedAt != nil {
				t.Fatalf("unexpected returned send result: payment=%q event=%s state=%s revision=%d sequence=%d sent=%v received=%v parse=%v", result.Stored.Offer.PaymentID, result.EventID, result.Stored.Offer.State, result.Stored.Offer.Revision, result.Stored.LastEventSequence, result.Stored.Offer.PaymentSentAt, result.Stored.Offer.PaymentReceivedAt, parseErr)
			}

			if scenario == "CallerRollback" {
				if err := tx.Rollback(ctx); err != nil {
					t.Fatalf("caller rollback: %v", err)
				}
				var state string
				var revision int64
				var payments, events int64
				var afterType string
				var afterActive bool
				var afterExpiry time.Time
				if err := admin.QueryRow(ctx, `SELECT o.estado,o.revision,(SELECT count(*) FROM public.arriendo_temprano_pago WHERE arriendo_id=o.id),(SELECT count(*) FROM public.arriendo_temprano_evento WHERE arriendo_id=o.id),oc.tipo,oc.activo,oc.expira_en FROM public.arriendo_temprano o JOIN public.ocupacion oc ON oc.id=o.ocupacion_id WHERE o.id=$1`, offerID).Scan(&state, &revision, &payments, &events, &afterType, &afterActive, &afterExpiry); err != nil {
					t.Fatal(err)
				}
				if state != string(earlyrental.StatePendingPaymentSend) || revision != 5 || payments != 0 || events != 5 || afterType != "retencion_temprana" || !afterActive || !afterExpiry.Equal(record.Offer.ExpiresAt) {
					t.Fatalf("rollback readback state=%s revision=%d payments=%d events=%d occupancy=%s active=%v expiry=%v", state, revision, payments, events, afterType, afterActive, afterExpiry)
				}
				if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
					t.Fatalf("caller rollback changed aggregate/occupancy/evidence counts: before=%+v after=%+v", before, after)
				}
				return
			}

			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("caller commit with deferred V99 guards: %v", err)
			}
			var state, paymentCurrency, paymentState, eventType, eventActor, previousState, nextState, occupancyTypeAfter string
			var revision, amount, sequence, eventRevision int64
			var updated, sent, occurred, expiry time.Time
			var sender, receiver, persistedPaymentID, eventID, eventPaymentID uuid.UUID
			var receivedIsNull, occupancyActiveAfter bool
			if err := admin.QueryRow(ctx, `SELECT o.estado,o.revision,o.actualizada_en,p.pago_id,p.emisor_id,p.receptor_id,p.monto_clp,p.moneda,p.estado,p.enviado_en,p.recibido_en IS NULL,e.id,e.secuencia,e.revision,e.tipo,e.actor_id,e.ocurrido_en,e.estado_anterior,e.estado_nuevo,e.pago_id,oc.tipo,oc.activo,oc.expira_en FROM public.arriendo_temprano o JOIN public.arriendo_temprano_pago p ON p.arriendo_id=o.id JOIN public.arriendo_temprano_evento e ON e.arriendo_id=o.id AND e.id=$2 JOIN public.ocupacion oc ON oc.id=o.ocupacion_id WHERE o.id=$1`, offerID, result.EventID).Scan(&state, &revision, &updated, &persistedPaymentID, &sender, &receiver, &amount, &paymentCurrency, &paymentState, &sent, &receivedIsNull, &eventID, &sequence, &eventRevision, &eventType, &eventActor, &occurred, &previousState, &nextState, &eventPaymentID, &occupancyTypeAfter, &occupancyActiveAfter, &expiry); err != nil {
				t.Fatal(err)
			}
			if state != string(earlyrental.StatePendingPaymentReceive) || revision != 6 || !updated.Equal(sentAt) || persistedPaymentID != uuid.MustParse(result.Stored.Offer.PaymentID) || sender != actorID || receiver != uuid.MustParse(ids.host) || amount != record.Offer.Snapshot.SubtotalCLP || paymentCurrency != "CLP" || paymentState != "enviado" || !sent.Equal(sentAt) || !receivedIsNull || eventID != result.EventID || sequence != 6 || eventRevision != 6 || eventType != string(earlyrental.EventPaymentMarkedSent) || eventActor != ids.renter || !occurred.Equal(sentAt) || previousState != string(earlyrental.StatePendingPaymentSend) || nextState != string(earlyrental.StatePendingPaymentReceive) || eventPaymentID != persistedPaymentID || occupancyTypeAfter != "retencion_temprana" || !occupancyActiveAfter || !expiry.Equal(record.Offer.ExpiresAt) {
				t.Fatalf("committed payment/event/occupancy readback mismatch: state=%s rev=%d sender=%s receiver=%s amount=%d currency=%s payment=%s sent=%v event=%s/%d/%d/%s actor=%s occupancy=%s active=%v expiry=%v", state, revision, sender, receiver, amount, paymentCurrency, paymentState, sent, eventID, sequence, eventRevision, eventType, eventActor, occupancyTypeAfter, occupancyActiveAfter, expiry)
			}
			if result.Stored.Offer.PaymentID != persistedPaymentID.String() {
				t.Fatalf("returned payment id=%s differs from persisted movement=%s", result.Stored.Offer.PaymentID, persistedPaymentID)
			}
			if after := earlyOfferReadCounts(t, ctx, dsn); after.offers != before.offers || after.occupancies != before.occupancies || after.events != before.events+1 || after.confirmations != before.confirmations || after.claims != before.claims {
				t.Fatalf("unexpected committed counts: before=%+v after=%+v", before, after)
			}
			for _, actor := range []uuid.UUID{uuid.MustParse(ids.renter), uuid.MustParse(ids.host)} {
				_, readTx := createEarlyOfferReadTx(t, ctx, dsn)
				readback, err := readEarlyRentalOffer(ctx, readTx, actor, offerID)
				if err != nil || readback.Offer.PaymentID != eventPaymentID.String() || readback.Offer.State != earlyrental.StatePendingPaymentReceive || readback.Offer.Revision != 6 || readback.Offer.PaymentSentAt == nil || !readback.Offer.PaymentSentAt.Equal(sentAt) {
					t.Fatalf("independent participant readback actor=%s offer=%+v err=%v", actor, readback.Offer, err)
				}
				if err := readTx.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func seedEarlyRentalPaymentSendReady(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, record earlyRentalInitialRecord, now time.Time) {
	t.Helper()
	steps := []struct {
		actor string
		kind  earlyrental.CommandKind
	}{
		{ids.renter, earlyrental.CommandConfirmIdentity},
		{ids.host, earlyrental.CommandConfirmIdentity},
		{ids.renter, earlyrental.CommandAcceptContract},
		{ids.host, earlyrental.CommandAcceptContract},
	}
	for i, step := range steps {
		_, tx := createClaimNativeTx(t, ctx, dsn)
		command := earlyrental.Command{Kind: step.kind, ExpectedRevision: uint64(i + 1)}
		if step.kind == earlyrental.CommandAcceptContract {
			command.ContractVersion = record.Offer.ContractVersion
			command.ContractDigest = record.Offer.ContractDigest
		}
		at := now.Add(time.Duration(i+1) * time.Minute)
		result, err := applyEarlyRentalConfirmation(ctx, tx, uuid.MustParse(step.actor), uuid.MustParse(ids.offer), command, at)
		if err != nil {
			t.Fatalf("seed payment-send phase step %d: %v", i+1, err)
		}
		if result.Stored.Offer.Revision != uint64(i+2) {
			t.Fatalf("seed payment-send phase step %d revision=%d", i+1, result.Stored.Offer.Revision)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit payment-send phase step %d: %v", i+1, err)
		}
	}
}
