package earlyrentalhttp

import (
	"errors"
	"math"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

var errInvalidEarlyRentalEvents = errors.New("early rental events cannot be projected")

const (
	earlyRentalRoleRenter = "arrendatario"
	earlyRentalRoleHost   = "arrendador"
	earlyRentalRoleSystem = "sistema"
	earlyRentalSystemID   = "system"
)

type earlyRentalEventsEnvelope struct {
	Data earlyRentalEventsData `json:"data"`
}

type earlyRentalEventsData struct {
	Items []earlyRentalEventView `json:"items"`
}

type earlyRentalEventView struct {
	Sequence        int64                 `json:"sequence"`
	Kind            earlyrental.EventKind `json:"kind"`
	ActorRole       string                `json:"actor_role"`
	Revision        int64                 `json:"revision"`
	OccurredAt      time.Time             `json:"occurred_at"`
	ContractVersion *int64                `json:"contract_version"`
	PaymentID       *string               `json:"payment_id"`
}

// projectEarlyRentalEvents maps private actor IDs to public roles and emits only
// the fields defined by the early-rental event contract. Call it only after the
// request has been authorized for a participant of the offer.
func projectEarlyRentalEvents(offer earlyrental.Offer, events []earlyrental.Event, after uint64) (earlyRentalEventsEnvelope, error) {
	if offer.HostID == "" || offer.RenterID == "" || offer.HostID == offer.RenterID ||
		offer.Revision == 0 || offer.Revision > uint64(math.MaxInt64) || after > uint64(math.MaxInt64) {
		return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
	}

	projected := earlyRentalEventsEnvelope{Data: earlyRentalEventsData{Items: make([]earlyRentalEventView, 0, len(events))}}
	if after >= offer.Revision {
		if len(events) != 0 {
			return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
		}
		return projected, nil
	}
	if len(events) == 0 || uint64(len(events)) != offer.Revision-after {
		return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
	}

	var lastSequence uint64
	var previousNextState earlyrental.State
	var movementID string
	for index, event := range events {
		if event.Sequence == 0 || event.Revision != event.Sequence || event.Sequence > uint64(math.MaxInt64) || event.At.IsZero() ||
			(index == 0 && event.Sequence != after+1) || (index > 0 && event.Sequence != lastSequence+1) || !validEarlyRentalEventTransition(event) ||
			(index > 0 && event.PreviousState != previousNextState) {
			return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
		}

		actorRole, ok := earlyRentalEventActorRole(offer, event)
		if !ok {
			return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
		}
		view := earlyRentalEventView{
			Sequence: int64(event.Sequence), Kind: event.Kind, ActorRole: actorRole,
			Revision: int64(event.Revision), OccurredAt: event.At.UTC(),
		}
		if event.Kind == earlyrental.EventContractAccepted {
			if event.ContractVersion <= 0 {
				return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
			}
			version := event.ContractVersion
			view.ContractVersion = &version
		} else if event.ContractVersion != 0 {
			return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
		}

		isPaymentEvent := event.Kind == earlyrental.EventPaymentMarkedSent || event.Kind == earlyrental.EventPaymentMarkedReceived
		if isPaymentEvent {
			if event.PaymentID == nil {
				return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
			}
			parsedID, err := uuid.Parse(*event.PaymentID)
			if err != nil || parsedID == uuid.Nil || parsedID.String() != *event.PaymentID || (movementID != "" && movementID != *event.PaymentID) {
				return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
			}
			movementID = *event.PaymentID
			paymentID := *event.PaymentID
			view.PaymentID = &paymentID
		} else if event.PaymentID != nil {
			return earlyRentalEventsEnvelope{}, errInvalidEarlyRentalEvents
		}

		projected.Data.Items = append(projected.Data.Items, view)
		lastSequence = event.Sequence
		previousNextState = event.NextState
	}
	return projected, nil
}

func validEarlyRentalEventTransition(event earlyrental.Event) bool {
	if event.Kind == earlyrental.EventOfferCreated {
		return event.Sequence == 1 && event.Revision == 1 && event.PreviousState == "" && event.NextState == earlyrental.StatePendingIdentity
	}
	if event.Sequence == 1 || !validEarlyRentalEventState(event.PreviousState) || !validEarlyRentalEventState(event.NextState) {
		return false
	}
	switch event.Kind {
	case earlyrental.EventIdentityConfirmed:
		return event.PreviousState == earlyrental.StatePendingIdentity &&
			(event.NextState == earlyrental.StatePendingIdentity || event.NextState == earlyrental.StatePendingContract)
	case earlyrental.EventContractAccepted:
		return event.PreviousState == earlyrental.StatePendingContract &&
			(event.NextState == earlyrental.StatePendingContract || event.NextState == earlyrental.StatePendingPaymentSend)
	case earlyrental.EventPaymentMarkedSent:
		return event.PreviousState == earlyrental.StatePendingPaymentSend && event.NextState == earlyrental.StatePendingPaymentReceive
	case earlyrental.EventPaymentMarkedReceived:
		return event.PreviousState == earlyrental.StatePendingPaymentReceive && event.NextState == earlyrental.StateConfirmed
	case earlyrental.EventOfferRejected:
		return (event.PreviousState == earlyrental.StatePendingIdentity || event.PreviousState == earlyrental.StatePendingContract) && event.NextState == earlyrental.StateRejected
	case earlyrental.EventOfferCancelled:
		return earlyRentalCancellableState(event.PreviousState) && event.NextState == earlyrental.StateCancelled
	case earlyrental.EventOfferExpired:
		return earlyRentalCancellableState(event.PreviousState) && event.NextState == earlyrental.StateExpired
	default:
		return false
	}
}

func validEarlyRentalEventState(state earlyrental.State) bool {
	switch state {
	case earlyrental.StatePendingIdentity, earlyrental.StatePendingContract,
		earlyrental.StatePendingPaymentSend, earlyrental.StatePendingPaymentReceive,
		earlyrental.StateConfirmed, earlyrental.StateRejected, earlyrental.StateCancelled,
		earlyrental.StateExpired:
		return true
	default:
		return false
	}
}

func earlyRentalCancellableState(state earlyrental.State) bool {
	switch state {
	case earlyrental.StatePendingIdentity, earlyrental.StatePendingContract,
		earlyrental.StatePendingPaymentSend, earlyrental.StatePendingPaymentReceive:
		return true
	default:
		return false
	}
}

func earlyRentalEventActorRole(offer earlyrental.Offer, event earlyrental.Event) (string, bool) {
	switch event.Kind {
	case earlyrental.EventOfferCreated, earlyrental.EventPaymentMarkedSent:
		return earlyRentalRoleRenter, event.ActorID == offer.RenterID
	case earlyrental.EventPaymentMarkedReceived:
		if event.ActorID == earlyRentalSystemID {
			return earlyRentalRoleSystem, true
		}
		return earlyRentalRoleHost, event.ActorID == offer.HostID
	case earlyrental.EventOfferExpired:
		return earlyRentalRoleSystem, event.ActorID == earlyRentalSystemID
	case earlyrental.EventIdentityConfirmed, earlyrental.EventContractAccepted,
		earlyrental.EventOfferRejected, earlyrental.EventOfferCancelled:
		switch event.ActorID {
		case offer.RenterID:
			return earlyRentalRoleRenter, true
		case offer.HostID:
			return earlyRentalRoleHost, true
		default:
			return "", false
		}
	default:
		return "", false
	}
}
