package earlyrentalhttp

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

var errInvalidEarlyRentalTransactionProjection = errors.New("early-rental transactions cannot be projected")

type earlyRentalTransactionsEnvelope struct {
	Data earlyRentalTransactionsView `json:"data"`
}

type earlyRentalTransactionsView struct {
	Items      []earlyRentalTransactionView `json:"items"`
	NextCursor *string                      `json:"next_cursor"`
}

type earlyRentalTransactionView struct {
	ID            string     `json:"id"`
	EarlyRentalID string     `json:"early_rental_id"`
	ActorRole     string     `json:"actor_role"`
	Perspective   string     `json:"perspective"`
	SpaceTitle    string     `json:"space_title"`
	AmountCLP     int64      `json:"amount_clp"`
	Currency      string     `json:"currency"`
	State         string     `json:"state"`
	SentAt        *time.Time `json:"sent_at"`
	ReceivedAt    *time.Time `json:"received_at"`
}

// projectEarlyRentalTransactions projects the one canonical movement from each
// source offer into the authenticated participant's perspective.
func projectEarlyRentalTransactions(offers []earlyrental.Offer, actorID string, nextCursor *string) (earlyRentalTransactionsEnvelope, error) {
	if actorID == "" || (nextCursor != nil && (!utf8.ValidString(*nextCursor) || utf8.RuneCountInString(*nextCursor) < 1 || utf8.RuneCountInString(*nextCursor) > maxEarlyRentalCursorRunes)) {
		return earlyRentalTransactionsEnvelope{}, errInvalidEarlyRentalTransactionProjection
	}
	projected := earlyRentalTransactionsEnvelope{Data: earlyRentalTransactionsView{Items: make([]earlyRentalTransactionView, 0, len(offers))}}
	if nextCursor != nil {
		cursor := *nextCursor
		projected.Data.NextCursor = &cursor
	}
	seenPaymentIDs := make(map[string]struct{}, len(offers))
	for _, offer := range offers {
		if earlyrental.ValidateOffer(offer) != nil {
			return earlyRentalTransactionsEnvelope{}, errInvalidEarlyRentalTransactionProjection
		}
		paymentState, hasMovement := earlyRentalPaymentState(offer)
		if !hasMovement {
			return earlyRentalTransactionsEnvelope{}, errInvalidEarlyRentalTransactionProjection
		}
		if _, duplicate := seenPaymentIDs[offer.PaymentID]; duplicate {
			return earlyRentalTransactionsEnvelope{}, errInvalidEarlyRentalTransactionProjection
		}
		seenPaymentIDs[offer.PaymentID] = struct{}{}

		actorRole, perspective := "", ""
		switch actorID {
		case offer.RenterID:
			actorRole, perspective = earlyRentalRoleRenter, "egreso"
		case offer.HostID:
			actorRole, perspective = earlyRentalRoleHost, "ingreso"
		default:
			return earlyRentalTransactionsEnvelope{}, errInvalidEarlyRentalTransactionProjection
		}
		projected.Data.Items = append(projected.Data.Items, earlyRentalTransactionView{
			ID: offer.PaymentID, EarlyRentalID: offer.ID, ActorRole: actorRole, Perspective: perspective,
			SpaceTitle: offer.Snapshot.SpaceTitle, AmountCLP: offer.Snapshot.SubtotalCLP, Currency: offer.Snapshot.Currency,
			State: paymentState, SentAt: participantTimeValue(offer.PaymentSentAt), ReceivedAt: participantTimeValue(offer.PaymentReceivedAt),
		})
	}
	return projected, nil
}

func earlyRentalPaymentState(offer earlyrental.Offer) (string, bool) {
	if offer.PaymentSentAt == nil {
		return "", false
	}
	switch {
	case offer.PaymentReceivedAt != nil:
		return "recibido", true
	case offer.State == earlyrental.StateCancelled:
		return "anulado", true
	case offer.State == earlyrental.StateExpired:
		return "vencido", true
	default:
		return "enviado", true
	}
}
