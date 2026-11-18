package rentalflowpg

import (
	"context"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type earlyRentalConfirmationResult struct {
	Stored  earlyRentalStoredOffer
	EventID uuid.UUID
}

const maxEarlyRentalRevision = uint64(1<<63 - 1)

// applyEarlyRentalConfirmation persists an identity or contract confirmation
// in the caller's READ COMMITTED transaction. The caller owns commit/rollback.
func applyEarlyRentalConfirmation(ctx context.Context, tx pgx.Tx, actorID, offerID uuid.UUID, command earlyrental.Command, now time.Time) (earlyRentalConfirmationResult, error) {
	invalid := func() (earlyRentalConfirmationResult, error) {
		return earlyRentalConfirmationResult{}, earlyrental.ErrInvalid
	}
	if ctx == nil || tx == nil || actorID == uuid.Nil || offerID == uuid.Nil || now.IsZero() {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalConfirmationResult{}, err
	}

	var class string
	var eventKind earlyrental.EventKind
	switch command.Kind {
	case earlyrental.CommandConfirmIdentity:
		class = "identidad"
		eventKind = earlyrental.EventIdentityConfirmed
	case earlyrental.CommandAcceptContract:
		class = "contrato"
		eventKind = earlyrental.EventContractAccepted
	default:
		return invalid()
	}

	stored, err := readEarlyRentalOffer(ctx, tx, actorID, offerID)
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	updated, event, err := earlyrental.Apply(stored.Offer, actorID.String(), command, now)
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	if stored.LastEventSequence != stored.Offer.Revision || stored.Offer.Revision >= maxEarlyRentalRevision ||
		updated.Revision != stored.Offer.Revision+1 || updated.Revision > maxEarlyRentalRevision ||
		event.Kind != eventKind || event.ActorID != actorID.String() || event.Sequence != updated.Revision ||
		event.Revision != updated.Revision || event.PreviousState != stored.Offer.State ||
		event.NextState != updated.State || !event.At.Equal(now.UTC()) {
		return invalid()
	}

	eventID := uuid.New()
	if eventID == uuid.Nil {
		return invalid()
	}
	tag, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$1,revision=$2,actualizada_en=$3 WHERE id=$4 AND revision=$5 AND estado=$6`, string(updated.State), int64(updated.Revision), event.At.UTC(), offerID, int64(stored.Offer.Revision), string(stored.Offer.State))
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return earlyRentalConfirmationResult{}, earlyrental.ErrPreconditionFailed
	}
	if tag.RowsAffected() != 1 {
		return invalid()
	}

	var confirmationID uuid.UUID
	if class == "identidad" {
		err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,confirmada_en) VALUES($1,$2,'identidad',$3) RETURNING id`, offerID, actorID, event.At.UTC()).Scan(&confirmationID)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest,confirmada_en) VALUES($1,$2,'contrato',$3,decode($4,'hex'),$5) RETURNING id`, offerID, actorID, updated.ContractVersion, updated.ContractDigest, event.At.UTC()).Scan(&confirmationID)
	}
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	if confirmationID == uuid.Nil {
		return invalid()
	}

	tag, err = tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(id,arriendo_id,secuencia,revision,tipo,actor_id,ocurrido_en,estado_anterior,estado_nuevo,confirmacion_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, eventID, offerID, int64(event.Sequence), int64(event.Revision), string(event.Kind), actorID, event.At.UTC(), string(event.PreviousState), string(event.NextState), confirmationID)
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return invalid()
	}

	authoritative, err := readEarlyRentalOffer(ctx, tx, actorID, offerID)
	if err != nil {
		return earlyRentalConfirmationResult{}, err
	}
	if authoritative.Offer.Revision != updated.Revision || authoritative.Offer.State != updated.State ||
		authoritative.LastEventSequence != event.Sequence || !authoritative.UpdatedAt.Equal(event.At) {
		return invalid()
	}
	if class == "identidad" {
		if at, ok := authoritative.Offer.IdentityConfirmed[actorID.String()]; !ok || !at.Equal(event.At) {
			return invalid()
		}
	} else if at, ok := authoritative.Offer.ContractAccepted[actorID.String()]; !ok || !at.Equal(event.At) {
		return invalid()
	}
	return earlyRentalConfirmationResult{Stored: authoritative, EventID: eventID}, nil
}
