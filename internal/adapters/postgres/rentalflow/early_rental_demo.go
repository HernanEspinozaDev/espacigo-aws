package rentalflowpg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/booking"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (r *Repository) CreateEarlyRental(ctx context.Context, actor, space uuid.UUID, starts, ends time.Time, key string) (earlyrental.Result, error) {
	return r.earlyRentalTransaction(ctx, func(tx pgx.Tx) (earlyRentalStoredOffer, error) {
		fingerprint, err := earlyRentalCreateFingerprint(space, starts, ends)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if err := lockEarlyRentalCreateKey(ctx, tx, actor, key); err != nil {
			return earlyRentalStoredOffer{}, err
		}
		id, replay, err := lookupEarlyRentalCreateReplay(ctx, tx, actor, key, fingerprint)
		if errors.Is(err, errEarlyRentalCreateKeyConflict) {
			return earlyRentalStoredOffer{}, rentalflow.ErrConflict
		}
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if replay {
			return readEarlyRentalOffer(ctx, tx, actor, id)
		}
		snapshot, err := r.ResolveEarlyRentalOfferSnapshot(ctx, tx, actor.String(), space.String(), rentalflow.Interval{StartAt: starts, EndAt: ends})
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		now := r.now().UTC().Truncate(time.Microsecond)
		id = uuid.New()
		terms := fmt.Sprintf("Contrato de prueba para %s. Arriendo por %d hora(s), del %s al %s. Total: $%d CLP. Las firmas y el pago son de prueba.",
			snapshot.Input.SpaceTitle, snapshot.Input.Units, starts.Format(time.RFC3339), ends.Format(time.RFC3339), snapshot.Input.Units*snapshot.Input.UnitPriceCLP)
		offer, event, err := earlyrental.NewOffer(earlyrental.NewOfferInput{
			ID: id.String(), HostID: snapshot.HostID, RenterID: actor.String(), Snapshot: snapshot.Input,
			ContractVersion: 1, ContractText: terms,
		}, now)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		record, err := newEarlyRentalInitialRecord(offer, uuid.NewString(), event, now)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if err := insertEarlyRentalInitial(ctx, tx, record); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
				return earlyRentalStoredOffer{}, rentalflow.ErrConflict
			}
			return earlyRentalStoredOffer{}, err
		}
		if _, err := insertEarlyRentalCreateClaim(ctx, tx, actor, id, key, fingerprint); err != nil {
			return earlyRentalStoredOffer{}, err
		}
		return readEarlyRentalOffer(ctx, tx, actor, id)
	})
}

func (r *Repository) GetEarlyRental(ctx context.Context, actor, id uuid.UUID) (earlyrental.Result, error) {
	return r.earlyRentalTransaction(ctx, func(tx pgx.Tx) (earlyRentalStoredOffer, error) {
		return readEarlyRentalOffer(ctx, tx, actor, id)
	})
}

func (r *Repository) ConfirmEarlyRental(ctx context.Context, actor, id uuid.UUID, command earlyrental.Command) (earlyrental.Result, error) {
	return r.earlyRentalTransaction(ctx, func(tx pgx.Tx) (earlyRentalStoredOffer, error) {
		stored, err := readEarlyRentalOffer(ctx, tx, actor, id)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		offer := stored.Offer
		switch command.Kind {
		case earlyrental.CommandConfirmIdentity:
			if actor.String() == offer.HostID && offer.IdentityConfirmed[offer.RenterID].IsZero() {
				return earlyRentalStoredOffer{}, earlyrental.ErrWrongState
			}
			if _, done := offer.IdentityConfirmed[actor.String()]; done {
				return stored, nil
			}
		case earlyrental.CommandReject:
			if actor.String() != offer.HostID {
				return earlyRentalStoredOffer{}, earlyrental.ErrForbidden
			}
			if offer.State == earlyrental.StateRejected {
				return stored, nil
			}
			if offer.State != earlyrental.StatePendingIdentity || !offer.IdentityConfirmed[offer.HostID].IsZero() {
				return earlyRentalStoredOffer{}, earlyrental.ErrWrongState
			}
			now := r.now().UTC().Truncate(time.Microsecond)
			next, event, err := earlyrental.Apply(offer, actor.String(), command, now)
			if err != nil {
				return earlyRentalStoredOffer{}, err
			}
			batch := &pgx.Batch{}
			batch.Queue(`UPDATE public.arriendo_temprano SET estado=$2,revision=$3,actualizada_en=$4
				WHERE id=$1 AND revision=$3-1`, id, next.State, int64(next.Revision), now)
			batch.Queue(`UPDATE public.ocupacion SET activo=false,desactivada_en=$2
				WHERE arriendo_temprano_id=$1 AND tipo='retencion_temprana' AND activo`, id, now)
			batch.Queue(`INSERT INTO public.arriendo_temprano_evento
				(arriendo_id,secuencia,revision,tipo,actor_id,ocurrido_en,estado_anterior,estado_nuevo,motivo_codigo)
				VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8)`,
				id, int64(next.Revision), event.Kind, actor, now, event.PreviousState, event.NextState, event.ReasonCode)
			if err := executeEarlyRentalBatch(ctx, tx, batch); err != nil {
				return earlyRentalStoredOffer{}, err
			}
			return readEarlyRentalOffer(ctx, tx, actor, id)
		case earlyrental.CommandAcceptContract:
			if command.ContractVersion != offer.ContractVersion || command.ContractDigest != offer.ContractDigest {
				return earlyRentalStoredOffer{}, earlyrental.ErrContractMismatch
			}
			if _, done := offer.ContractAccepted[actor.String()]; done {
				return stored, nil
			}
		default:
			return earlyRentalStoredOffer{}, earlyrental.ErrInvalid
		}
		result, err := applyEarlyRentalConfirmation(ctx, tx, actor, id, command, r.now().UTC().Truncate(time.Microsecond))
		return result.Stored, err
	})
}

func (r *Repository) PayEarlyRental(ctx context.Context, actor, id uuid.UUID, revision uint64, processor booking.LocalPaymentAdapter) (earlyrental.Result, error) {
	return r.earlyRentalTransaction(ctx, func(tx pgx.Tx) (earlyRentalStoredOffer, error) {
		stored, err := readEarlyRentalOffer(ctx, tx, actor, id)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if actor.String() != stored.Offer.RenterID {
			return earlyRentalStoredOffer{}, earlyrental.ErrForbidden
		}
		if stored.Offer.State == earlyrental.StateConfirmed {
			return stored, nil
		}

		now := r.now().UTC().Truncate(time.Microsecond)
		sent, err := applyEarlyRentalPaymentSend(ctx, tx, actor, id, earlyrental.Command{
			Kind: earlyrental.CommandSendPayment, ExpectedRevision: revision,
		}, now)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		paymentID := sent.Stored.Offer.PaymentID
		approval, err := processor.StartPayment(ctx, paymentID, "exito")
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if approval == nil || approval.OperationID != paymentID || approval.Outcome != "exito_simulado" || !processor.VerifyPaymentEvent(*approval) {
			return earlyRentalStoredOffer{}, errors.New("invalid local payment processor approval")
		}
		confirmed, event, err := earlyrental.ApproveProcessedPayment(sent.Stored.Offer, now)
		if err != nil {
			return earlyRentalStoredOffer{}, err
		}
		if err := persistEarlyRentalApproval(ctx, tx, id, confirmed, event, approval.EventID); err != nil {
			return earlyRentalStoredOffer{}, err
		}
		return readEarlyRentalOffer(ctx, tx, actor, id)
	})
}

func persistEarlyRentalApproval(ctx context.Context, tx pgx.Tx, id uuid.UUID, offer earlyrental.Offer, event earlyrental.Event, reference string) error {
	batch := &pgx.Batch{}
	batch.Queue(`UPDATE public.arriendo_temprano_pago SET estado='recibido',recibido_en=$3
		WHERE arriendo_id=$1 AND pago_id=$2 AND estado='enviado'`, id, offer.PaymentID, event.At)
	batch.Queue(`UPDATE public.arriendo_temprano SET estado=$2,revision=$3,actualizada_en=$4
		WHERE id=$1 AND revision=$3-1 AND estado=$5`, id, offer.State, int64(offer.Revision), event.At, event.PreviousState)
	batch.Queue(`UPDATE public.ocupacion SET tipo='reserva_temprana',expira_en=NULL
		WHERE arriendo_temprano_id=$1 AND tipo='retencion_temprana' AND activo`, id)
	batch.Queue(`INSERT INTO public.arriendo_temprano_evento
		(arriendo_id,secuencia,revision,tipo,ocurrido_en,estado_anterior,estado_nuevo,pago_id,procesador_referencia)
		VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8)`, id, int64(event.Revision), event.Kind, event.At, event.PreviousState, event.NextState, offer.PaymentID, reference)
	return executeEarlyRentalBatch(ctx, tx, batch)
}

func executeEarlyRentalBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch) error {
	results := tx.SendBatch(ctx, batch)
	defer results.Close()
	for range batch.Len() {
		tag, err := results.Exec()
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return earlyrental.ErrPreconditionFailed
		}
	}
	return results.Close()
}

func (r *Repository) earlyRentalTransaction(ctx context.Context, run func(pgx.Tx) (earlyRentalStoredOffer, error)) (earlyrental.Result, error) {
	var result earlyrental.Result
	err := pgx.BeginFunc(ctx, r.beginner, func(tx pgx.Tx) error {
		stored, err := run(tx)
		if err == nil {
			result = earlyrental.Result{Offer: stored.Offer, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt}
		}
		return err
	})
	if err != nil {
		return earlyrental.Result{}, err
	}
	return result, nil
}

func (r *Repository) ListDemoEarlyRentals(ctx context.Context, actor uuid.UUID, role string, afterAt time.Time, afterID uuid.UUID, limit int) ([]earlyrental.Result, bool, error) {
	input := EarlyRentalOfferListInput{ActorID: actor, Role: role, Limit: limit}
	if afterID != uuid.Nil {
		input.After = &EarlyRentalOfferListPosition{CreatedAt: afterAt, ID: afterID}
	}
	for range 3 {
		var page EarlyRentalOfferListResult
		err := pgx.BeginFunc(ctx, r.beginner, func(tx pgx.Tx) error {
			var err error
			page, err = listEarlyRentalOffers(ctx, tx, input)
			return err
		})
		if errors.Is(err, ErrEarlyRentalOfferListRetry) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		results := make([]earlyrental.Result, 0, len(page.Rows))
		for _, row := range page.Rows {
			results = append(results, earlyrental.Result{Offer: row.Offer, CreatedAt: row.CreatedAt})
		}
		return results, page.Next != nil, nil
	}
	return nil, false, rentalflow.ErrConflict
}
