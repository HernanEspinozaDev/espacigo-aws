package rentalflowpg

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

// earlyRentalEventRecord carries the event columns needed by the domain. For
// contract events the reader must join the confirmation by (id, arriendo_id)
// and take its version and digest, not the current listing or contract text.
// Empty strings represent NULL for actor, previous state, reason and payment.
// The SQL reader must reject non-finite timestamps before converting to time.Time.
type earlyRentalEventRecord struct {
	Sequence, Revision       int64
	Kind, ActorID            string
	PreviousState, NextState string
	OccurredAt               time.Time
	ReasonCode, PaymentID    string
	ContractVersion          int64
	ContractDigest           []byte
}

// mapEarlyRentalEventRecord validates the row in isolation. Participant roles,
// evidence associations and continuity across rows belong to the feed reader.
func mapEarlyRentalEventRecord(row earlyRentalEventRecord) (earlyrental.Event, bool) {
	if row.Sequence <= 0 || row.Revision != row.Sequence || row.OccurredAt.IsZero() {
		return earlyrental.Event{}, false
	}
	event := earlyrental.Event{
		Kind: earlyrental.EventKind(row.Kind), ActorID: row.ActorID,
		Sequence: uint64(row.Sequence), Revision: uint64(row.Revision), At: row.OccurredAt.UTC(),
		PreviousState: earlyrental.State(row.PreviousState), NextState: earlyrental.State(row.NextState),
		ReasonCode: row.ReasonCode,
	}
	if !validEarlyRentalStoredEventTransition(event) {
		return earlyrental.Event{}, false
	}
	if event.Kind == earlyrental.EventOfferExpired || (event.Kind == earlyrental.EventPaymentMarkedReceived && row.ActorID == "") {
		if row.ActorID != "" {
			return earlyrental.Event{}, false
		}
		event.ActorID = "system"
	} else if !validEarlyRentalEventUUID(row.ActorID) {
		return earlyrental.Event{}, false
	}
	if event.Kind == earlyrental.EventOfferRejected || event.Kind == earlyrental.EventOfferCancelled {
		if !earlyrental.ValidReasonCode(row.ReasonCode) {
			return earlyrental.Event{}, false
		}
	} else if row.ReasonCode != "" {
		return earlyrental.Event{}, false
	}
	if event.Kind == earlyrental.EventContractAccepted {
		if row.ContractVersion <= 0 || len(row.ContractDigest) != sha256.Size {
			return earlyrental.Event{}, false
		}
		event.ContractVersion = row.ContractVersion
		event.ContractDigest = hex.EncodeToString(row.ContractDigest)
	} else if row.ContractVersion != 0 || row.ContractDigest != nil {
		return earlyrental.Event{}, false
	}
	switch event.Kind {
	case earlyrental.EventPaymentMarkedSent, earlyrental.EventPaymentMarkedReceived:
		if !validEarlyRentalEventUUID(row.PaymentID) {
			return earlyrental.Event{}, false
		}
		paymentID := row.PaymentID
		event.PaymentID = &paymentID
	case earlyrental.EventOfferCancelled, earlyrental.EventOfferExpired:
		// V99 permits a reference for terminal persistence evidence. The domain
		// and HTTP feed put PaymentID only on send/receive events.
		if row.PaymentID != "" && !validEarlyRentalEventUUID(row.PaymentID) {
			return earlyrental.Event{}, false
		}
	default:
		if row.PaymentID != "" {
			return earlyrental.Event{}, false
		}
	}
	return event, true
}

func validEarlyRentalEventUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

func validEarlyRentalStoredEventTransition(e earlyrental.Event) bool {
	from, to := e.PreviousState, e.NextState
	if e.Sequence == 1 {
		return e.Kind == earlyrental.EventOfferCreated && from == "" && to == earlyrental.StatePendingIdentity
	}
	switch e.Kind {
	case earlyrental.EventIdentityConfirmed:
		return from == earlyrental.StatePendingIdentity && (to == from || to == earlyrental.StatePendingContract)
	case earlyrental.EventContractAccepted:
		return from == earlyrental.StatePendingContract && (to == from || to == earlyrental.StatePendingPaymentSend)
	case earlyrental.EventPaymentMarkedSent:
		return from == earlyrental.StatePendingPaymentSend && to == earlyrental.StatePendingPaymentReceive
	case earlyrental.EventPaymentMarkedReceived:
		return from == earlyrental.StatePendingPaymentReceive && to == earlyrental.StateConfirmed
	case earlyrental.EventOfferRejected:
		return (from == earlyrental.StatePendingIdentity || from == earlyrental.StatePendingContract) && to == earlyrental.StateRejected
	case earlyrental.EventOfferCancelled, earlyrental.EventOfferExpired:
		if from != earlyrental.StatePendingIdentity && from != earlyrental.StatePendingContract &&
			from != earlyrental.StatePendingPaymentSend && from != earlyrental.StatePendingPaymentReceive {
			return false
		}
		return (e.Kind == earlyrental.EventOfferCancelled && to == earlyrental.StateCancelled) ||
			(e.Kind == earlyrental.EventOfferExpired && to == earlyrental.StateExpired)
	default:
		return false
	}
}
