package rentalflowpg

import (
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func TestPostgresEarlyRentalApplyConfirmation(t *testing.T) {
	for _, name := range []string{"IdentityCommit", "ContractCommit", "CallerRollback"} {
		t.Run(name, func(t *testing.T) {
			ctx, dbURL := earlyInsertPostgresFixture(t)
			dsn := dbURL.String()
			grantEarlyOfferReadColumns(t, ctx, dsn)
			ids := earlyInsertIDs()
			seedEarlyInsertActors(t, ctx, dsn, ids)
			now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
			record := earlyInsertRecord(t, ids, now)
			commitEarlyReadFixtureOffer(t, ctx, dsn, record)
			if name == "ContractCommit" {
				for i, actor := range []string{ids.renter, ids.host} {
					_, identityTx := createClaimNativeTx(t, ctx, dsn)
					identityAt := now.Add(time.Duration(i+1) * time.Minute)
					identity, err := applyEarlyRentalConfirmation(ctx, identityTx, uuid.MustParse(actor), uuid.MustParse(ids.offer), earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity, ExpectedRevision: uint64(i + 1)}, identityAt)
					if err != nil {
						t.Fatalf("seed identity confirmation %d: %v", i+1, err)
					}
					if err := identityTx.Commit(ctx); err != nil {
						t.Fatalf("commit seed identity confirmation %d: %v", i+1, err)
					}
					if identity.Stored.Offer.Revision != uint64(i+2) {
						t.Fatalf("seed identity revision=%d, want %d", identity.Stored.Offer.Revision, i+2)
					}
				}
			}
			before := earlyOfferReadCounts(t, ctx, dsn)

			actorID := uuid.MustParse(ids.renter)
			offerID := uuid.MustParse(ids.offer)
			at := now.Add(time.Minute)
			command := earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity, ExpectedRevision: 1}
			class := "identidad"
			eventType := string(earlyrental.EventIdentityConfirmed)
			previousState := earlyrental.StatePendingIdentity
			nextState := earlyrental.StatePendingIdentity
			var contractVersion any
			var contractDigest any
			if name == "ContractCommit" {
				command = earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ExpectedRevision: 3, ContractVersion: record.Offer.ContractVersion, ContractDigest: record.Offer.ContractDigest}
				at = now.Add(3 * time.Minute)
				class = "contrato"
				eventType = string(earlyrental.EventContractAccepted)
				previousState = earlyrental.StatePendingContract
				nextState = earlyrental.StatePendingContract
				contractVersion = record.Offer.ContractVersion
				contractDigest = record.Offer.ContractDigest
			}
			_, tx := createClaimNativeTx(t, ctx, dsn)
			result, err := applyEarlyRentalConfirmation(ctx, tx, actorID, offerID, command, at)
			if err != nil {
				t.Fatalf("apply identity confirmation: %v", err)
			}
			wantRevision := uint64(2)
			if name == "ContractCommit" {
				wantRevision = 4
			}
			if result.EventID == uuid.Nil || result.Stored.Offer.Revision != wantRevision || result.Stored.Offer.State != nextState || result.Stored.LastEventSequence != wantRevision {
				t.Fatalf("unexpected returned confirmation: event=%s revision=%d state=%s sequence=%d", result.EventID, result.Stored.Offer.Revision, result.Stored.Offer.State, result.Stored.LastEventSequence)
			}
			if name == "ContractCommit" {
				if got := result.Stored.Offer.ContractAccepted[ids.renter]; !got.Equal(at) {
					t.Fatalf("returned contract timestamp=%v, want %v", got, at)
				}
			} else if got := result.Stored.Offer.IdentityConfirmed[ids.renter]; !got.Equal(at) {
				t.Fatalf("returned identity timestamp=%v, want %v", got, at)
			}

			if name == "CallerRollback" {
				if err := tx.Rollback(ctx); err != nil {
					t.Fatalf("caller rollback: %v", err)
				}
				if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
					t.Fatalf("rollback changed table counts: before=%+v after=%+v", before, after)
				}
				admin := connectEarlyReadAdmin(t, ctx, dsn)
				var state string
				var revision int64
				var confirmations int64
				if err := admin.QueryRow(ctx, `SELECT estado,revision,(SELECT count(*) FROM public.arriendo_temprano_confirmacion WHERE arriendo_id=$1) FROM public.arriendo_temprano WHERE id=$1`, offerID).Scan(&state, &revision, &confirmations); err != nil {
					t.Fatal(err)
				}
				if state != string(earlyrental.StatePendingIdentity) || revision != 1 || confirmations != 0 {
					t.Fatalf("rollback readback state=%s revision=%d confirmations=%d", state, revision, confirmations)
				}
				return
			}

			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("caller commit with deferred V99 guards: %v", err)
			}
			admin := connectEarlyReadAdmin(t, ctx, dsn)
			var state string
			var revision int64
			var updated time.Time
			var confirmationCount, eventCount int64
			if err := admin.QueryRow(ctx, `SELECT o.estado,o.revision,o.actualizada_en,
				(SELECT count(*) FROM public.arriendo_temprano_confirmacion c WHERE c.arriendo_id=o.id AND c.actor_id=$2 AND c.clase=$5 AND c.confirmada_en=$3 AND c.contrato_version IS NOT DISTINCT FROM $6::bigint AND c.contrato_digest IS NOT DISTINCT FROM CASE WHEN $7::text IS NULL THEN NULL::bytea ELSE decode($7,'hex') END),
				(SELECT count(*) FROM public.arriendo_temprano_evento e WHERE e.arriendo_id=o.id AND e.id=$4 AND e.secuencia=$8 AND e.revision=$8 AND e.tipo=$9 AND e.actor_id=$2 AND e.ocurrido_en=$3 AND e.estado_anterior=$10 AND e.estado_nuevo=$11 AND e.confirmacion_id=(SELECT c.id FROM public.arriendo_temprano_confirmacion c WHERE c.arriendo_id=o.id AND c.actor_id=$2 AND c.clase=$5))
				FROM public.arriendo_temprano o WHERE o.id=$1`, offerID, actorID, at, result.EventID, class, contractVersion, contractDigest, int64(wantRevision), eventType, string(previousState), string(nextState)).Scan(&state, &revision, &updated, &confirmationCount, &eventCount); err != nil {
				t.Fatal(err)
			}
			if state != string(nextState) || revision != int64(wantRevision) || !updated.Equal(at) || confirmationCount != 1 || eventCount != 1 {
				t.Fatalf("committed readback state=%s revision=%d updated=%v confirmation=%d event=%d", state, revision, updated, confirmationCount, eventCount)
			}
			if after := earlyOfferReadCounts(t, ctx, dsn); after.offers != before.offers || after.occupancies != before.occupancies || after.events != before.events+1 || after.confirmations != before.confirmations+1 || after.claims != before.claims {
				t.Fatalf("unexpected committed counts: before=%+v after=%+v", before, after)
			}
			_, readTx := createEarlyOfferReadTx(t, ctx, dsn)
			readback, err := readEarlyRentalOffer(ctx, readTx, actorID, offerID)
			if err != nil || readback.Offer.Revision != wantRevision || readback.LastEventSequence != wantRevision {
				t.Fatalf("independent runtime readback=%+v err=%v", readback, err)
			}
			if (name == "ContractCommit" && !readback.Offer.ContractAccepted[ids.renter].Equal(at)) || (name != "ContractCommit" && !readback.Offer.IdentityConfirmed[ids.renter].Equal(at)) {
				t.Fatalf("independent runtime confirmation timestamp mismatch: %+v", readback.Offer)
			}
		})
	}
}
