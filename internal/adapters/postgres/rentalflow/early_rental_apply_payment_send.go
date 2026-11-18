package rentalflowpg

import (
	"context"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type earlyRentalPaymentSendResult struct {
	Stored  earlyRentalStoredOffer
	EventID uuid.UUID
}

// applyEarlyRentalPaymentSend records the renter's sent-payment transition in
// the caller-owned READ COMMITTED transaction. The occupancy remains a hold.
func applyEarlyRentalPaymentSend(ctx context.Context, tx pgx.Tx, actorID, offerID uuid.UUID, command earlyrental.Command, now time.Time) (earlyRentalPaymentSendResult, error) {
	invalid := func() (earlyRentalPaymentSendResult, error) {
		return earlyRentalPaymentSendResult{}, earlyrental.ErrInvalid
	}
	if ctx == nil || tx == nil || actorID == uuid.Nil || offerID == uuid.Nil || now.IsZero() ||
		command.Kind != earlyrental.CommandSendPayment || command.PaymentID != "" {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}

	stored, err := readEarlyRentalOffer(ctx, tx, actorID, offerID)
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if stored.Offer.PaymentID != "" || stored.Offer.PaymentSentAt != nil || stored.Offer.PaymentReceivedAt != nil ||
		stored.LastEventSequence != stored.Offer.Revision || stored.Offer.Revision >= maxEarlyRentalRevision {
		return invalid()
	}

	paymentID := uuid.New()
	if paymentID == uuid.Nil || paymentID.String() == "" {
		return invalid()
	}
	domainCommand := command
	domainCommand.PaymentID = paymentID.String()
	updated, event, err := earlyrental.Apply(stored.Offer, actorID.String(), domainCommand, now)
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if updated.Revision != stored.Offer.Revision+1 || updated.Revision > maxEarlyRentalRevision ||
		updated.State != earlyrental.StatePendingPaymentReceive || updated.PaymentID != paymentID.String() ||
		updated.PaymentSentAt == nil || !updated.PaymentSentAt.Equal(now.UTC()) || updated.PaymentReceivedAt != nil ||
		event.Kind != earlyrental.EventPaymentMarkedSent || event.ActorID != actorID.String() ||
		event.Sequence != updated.Revision || event.Revision != updated.Revision ||
		event.PreviousState != earlyrental.StatePendingPaymentSend || event.NextState != earlyrental.StatePendingPaymentReceive ||
		event.PaymentID == nil || *event.PaymentID != paymentID.String() || !event.At.Equal(now.UTC()) {
		return invalid()
	}

	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$1,revision=$2,actualizada_en=$3 WHERE id=$4 AND revision=$5 AND estado=$6`, string(updated.State), int64(updated.Revision), event.At.UTC(), offerID, int64(stored.Offer.Revision), string(stored.Offer.State))
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return earlyRentalPaymentSendResult{}, earlyrental.ErrPreconditionFailed
	}
	if tag.RowsAffected() != 1 {
		return invalid()
	}

	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	var insertedPaymentID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_pago(arriendo_id,pago_id,emisor_id,receptor_id,monto_clp,moneda,estado,enviado_en,recibido_en,creado_en) VALUES($1,$2,$3,$4,$5,'CLP','pendiente',NULL,NULL,$6) RETURNING pago_id`, offerID, paymentID, uuid.MustParse(stored.Offer.RenterID), uuid.MustParse(stored.Offer.HostID), stored.Offer.Snapshot.SubtotalCLP, event.At.UTC()).Scan(&insertedPaymentID)
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if insertedPaymentID != paymentID {
		return invalid()
	}

	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	tag, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano_pago SET estado='enviado',enviado_en=$3 WHERE arriendo_id=$1 AND pago_id=$2 AND estado='pendiente' AND enviado_en IS NULL AND recibido_en IS NULL`, offerID, paymentID, event.At.UTC())
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return invalid()
	}

	eventID := uuid.New()
	if eventID == uuid.Nil {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	tag, err = tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(id,arriendo_id,secuencia,revision,tipo,actor_id,ocurrido_en,estado_anterior,estado_nuevo,pago_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, eventID, offerID, int64(event.Sequence), int64(event.Revision), string(event.Kind), actorID, event.At.UTC(), string(event.PreviousState), string(event.NextState), paymentID)
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return invalid()
	}

	authoritative, err := readEarlyRentalOffer(ctx, tx, actorID, offerID)
	if err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalPaymentSendResult{}, err
	}
	if authoritative.Offer.Revision != updated.Revision || authoritative.Offer.State != updated.State ||
		authoritative.LastEventSequence != event.Sequence || !authoritative.UpdatedAt.Equal(event.At) ||
		authoritative.Offer.PaymentID != paymentID.String() || authoritative.Offer.PaymentSentAt == nil ||
		!authoritative.Offer.PaymentSentAt.Equal(event.At) || authoritative.Offer.PaymentReceivedAt != nil {
		return invalid()
	}
	return earlyRentalPaymentSendResult{Stored: authoritative, EventID: eventID}, nil
}
