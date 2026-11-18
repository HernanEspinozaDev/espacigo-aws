package earlyrental

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalid            = errors.New("earlyrental: invalid offer or command")
	ErrForbidden          = errors.New("earlyrental: actor is not allowed to perform this command")
	ErrWrongState         = errors.New("earlyrental: command is not allowed in the current state")
	ErrContractMismatch   = errors.New("earlyrental: contract version or digest does not match")
	ErrPreconditionFailed = errors.New("earlyrental: offer revision does not match")
	ErrExpired            = errors.New("earlyrental: offer has expired")
)

type State string

const (
	StatePendingIdentity       State = "pendiente_identidad"
	StatePendingContract       State = "pendiente_contrato"
	StatePendingPaymentSend    State = "pendiente_envio_pago"
	StatePendingPaymentReceive State = "pendiente_recepcion_pago"
	StateConfirmed             State = "confirmado"
	StateRejected              State = "rechazado"
	StateCancelled             State = "cancelado"
	StateExpired               State = "vencido"
)

type CommandKind string

const (
	CommandConfirmIdentity CommandKind = "confirmar_identidad_simulada"
	CommandAcceptContract  CommandKind = "aceptar_contrato"
	CommandSendPayment     CommandKind = "marcar_pago_enviado"
	CommandReceivePayment  CommandKind = "marcar_pago_recibido"
	CommandReject          CommandKind = "rechazar"
	CommandCancel          CommandKind = "cancelar"
	CommandExpire          CommandKind = "oferta_vencida"
)

type EventKind string

const (
	EventOfferCreated          EventKind = "oferta_creada"
	EventIdentityConfirmed     EventKind = "identidad_simulada_confirmada"
	EventContractAccepted      EventKind = "contrato_aceptado"
	EventPaymentMarkedSent     EventKind = "pago_marcado_enviado"
	EventPaymentMarkedReceived EventKind = "pago_marcado_recibido"
	EventOfferRejected         EventKind = "oferta_rechazada"
	EventOfferCancelled        EventKind = "oferta_cancelada"
	EventOfferExpired          EventKind = "oferta_vencida"
)

const HoldTTL = 30 * time.Minute

const systemActorID = "system"

type Command struct {
	Kind             CommandKind
	ExpectedRevision uint64
	ContractVersion  int64
	ContractDigest   string
	ReasonCode       string
	PaymentID        string
}

type Result struct {
	Offer                Offer
	CreatedAt, UpdatedAt time.Time
}

type Offer struct {
	ID                string
	HostID            string
	RenterID          string
	Snapshot          OfferSnapshot
	State             State
	Revision          uint64
	ContractVersion   int64
	ContractText      string
	ContractDigest    string
	IdentityConfirmed map[string]time.Time
	ContractAccepted  map[string]time.Time
	PaymentID         string
	PaymentSentAt     *time.Time
	PaymentReceivedAt *time.Time
	ExpiresAt         time.Time
}

type NewOfferInput struct {
	ID              string
	HostID          string
	RenterID        string
	Snapshot        OfferSnapshotInput
	ContractVersion int64
	ContractText    string
}

// NewOffer creates the initial aggregate and caps its hold at the earlier of
// the configured TTL or the requested interval start.
func NewOffer(input NewOfferInput, now time.Time) (Offer, Event, error) {
	if now.IsZero() {
		return Offer{}, Event{}, ErrInvalid
	}
	snapshot, err := newOfferSnapshot(input.Snapshot)
	if err != nil || !snapshot.StartAt.After(now) {
		return Offer{}, Event{}, ErrInvalid
	}
	contractDigest, err := DigestContractText(input.ContractText)
	if err != nil {
		return Offer{}, Event{}, ErrInvalid
	}

	expiresAt := now.Add(HoldTTL)
	if snapshot.StartAt.Before(expiresAt) {
		expiresAt = snapshot.StartAt
	}
	offer := Offer{
		ID: input.ID, HostID: input.HostID, RenterID: input.RenterID, Snapshot: snapshot,
		State: StatePendingIdentity, Revision: 1,
		ContractVersion: input.ContractVersion, ContractText: input.ContractText, ContractDigest: contractDigest,
		IdentityConfirmed: make(map[string]time.Time), ContractAccepted: make(map[string]time.Time),
		ExpiresAt: expiresAt.UTC(),
	}
	if !validOffer(offer) {
		return Offer{}, Event{}, ErrInvalid
	}
	event := Event{
		Kind: EventOfferCreated, ActorID: input.RenterID, Sequence: 1, Revision: 1, At: now.UTC(),
		NextState: StatePendingIdentity,
	}
	return offer, event, nil
}

type Event struct {
	Kind            EventKind
	ActorID         string
	Sequence        uint64
	Revision        uint64
	At              time.Time
	PreviousState   State
	NextState       State
	ContractVersion int64
	ContractDigest  string
	ReasonCode      string
	PaymentID       *string
}

// Apply validates one actor command and returns a new aggregate plus its event.
// It never mutates the supplied offer; persistence must commit both atomically.
func Apply(offer Offer, actorID string, command Command, now time.Time) (Offer, Event, error) {
	return apply(offer, actorID, command, now, false)
}

// ApproveProcessedPayment applies an already verified processor approval.
func ApproveProcessedPayment(offer Offer, now time.Time) (Offer, Event, error) {
	return apply(offer, systemActorID, Command{Kind: CommandReceivePayment, ExpectedRevision: offer.Revision}, now, true)
}

func apply(offer Offer, actorID string, command Command, now time.Time, processorApproval bool) (Offer, Event, error) {
	if !validOffer(offer) || actorID == "" || now.IsZero() {
		return Offer{}, Event{}, ErrInvalid
	}
	if !processorApproval && actorID != offer.HostID && actorID != offer.RenterID {
		return Offer{}, Event{}, ErrForbidden
	}
	if command.ExpectedRevision != offer.Revision {
		return Offer{}, Event{}, ErrPreconditionFailed
	}
	if !now.Before(offer.ExpiresAt) {
		return Offer{}, Event{}, ErrExpired
	}
	if offer.Revision == math.MaxUint64 {
		return Offer{}, Event{}, ErrInvalid
	}

	next := cloneOffer(offer)
	at := now.UTC()
	event := Event{ActorID: actorID, At: at, PreviousState: offer.State}
	switch command.Kind {
	case CommandConfirmIdentity:
		if offer.State != StatePendingIdentity {
			return Offer{}, Event{}, ErrWrongState
		}
		if _, exists := offer.IdentityConfirmed[actorID]; exists {
			return Offer{}, Event{}, ErrWrongState
		}
		next.IdentityConfirmed[actorID] = at
		if confirmedByBoth(next.IdentityConfirmed, offer.HostID, offer.RenterID) {
			next.State = StatePendingContract
		}
	case CommandAcceptContract:
		if offer.State != StatePendingContract {
			return Offer{}, Event{}, ErrWrongState
		}
		if command.ContractVersion != offer.ContractVersion || command.ContractDigest != offer.ContractDigest {
			return Offer{}, Event{}, ErrContractMismatch
		}
		if _, exists := offer.ContractAccepted[actorID]; exists {
			return Offer{}, Event{}, ErrWrongState
		}
		next.ContractAccepted[actorID] = at
		if confirmedByBoth(next.ContractAccepted, offer.HostID, offer.RenterID) {
			next.State = StatePendingPaymentSend
		}
		event.ContractVersion = command.ContractVersion
		event.ContractDigest = command.ContractDigest
	case CommandSendPayment:
		if actorID != offer.RenterID {
			return Offer{}, Event{}, ErrForbidden
		}
		if offer.State != StatePendingPaymentSend || offer.PaymentSentAt != nil {
			return Offer{}, Event{}, ErrWrongState
		}
		if !validPaymentID(command.PaymentID) {
			return Offer{}, Event{}, ErrInvalid
		}
		next.PaymentSentAt = &at
		next.PaymentID = command.PaymentID
		next.State = StatePendingPaymentReceive
		paymentID := command.PaymentID
		event.PaymentID = &paymentID
	case CommandReceivePayment:
		if !processorApproval && actorID != offer.HostID {
			return Offer{}, Event{}, ErrForbidden
		}
		if command.PaymentID != "" {
			return Offer{}, Event{}, ErrInvalid
		}
		if offer.State != StatePendingPaymentReceive || offer.PaymentSentAt == nil || offer.PaymentReceivedAt != nil {
			return Offer{}, Event{}, ErrWrongState
		}
		next.PaymentReceivedAt = &at
		next.State = StateConfirmed
		paymentID := offer.PaymentID
		event.PaymentID = &paymentID
	case CommandReject:
		if !rejectableState(offer.State) {
			return Offer{}, Event{}, ErrWrongState
		}
		if !ValidReasonCode(command.ReasonCode) {
			return Offer{}, Event{}, ErrInvalid
		}
		next.State = StateRejected
		event.ReasonCode = command.ReasonCode
	case CommandCancel:
		if !cancellableState(offer.State) {
			return Offer{}, Event{}, ErrWrongState
		}
		if !ValidReasonCode(command.ReasonCode) {
			return Offer{}, Event{}, ErrInvalid
		}
		next.State = StateCancelled
		event.ReasonCode = command.ReasonCode
	default:
		return Offer{}, Event{}, ErrInvalid
	}
	event.NextState = next.State
	event.Kind = eventKindForCommand(command.Kind)
	if event.Kind == "" {
		return Offer{}, Event{}, ErrInvalid
	}
	next.Revision++
	event.Sequence = next.Revision
	event.Revision = next.Revision
	return next, event, nil
}

// Expire applies the deadline transition once. Persistence must commit the
// returned offer and event atomically with releasing its occupancy hold.
func Expire(offer Offer, now time.Time) (Offer, Event, bool, error) {
	if !validOffer(offer) || now.IsZero() {
		return Offer{}, Event{}, false, ErrInvalid
	}
	if terminalState(offer.State) || now.Before(offer.ExpiresAt) {
		return cloneOffer(offer), Event{}, false, nil
	}
	if offer.Revision == math.MaxUint64 {
		return Offer{}, Event{}, false, ErrInvalid
	}

	next := cloneOffer(offer)
	next.State = StateExpired
	next.Revision++
	event := Event{
		Kind:          EventOfferExpired,
		ActorID:       systemActorID,
		Sequence:      next.Revision,
		Revision:      next.Revision,
		At:            now.UTC(),
		PreviousState: offer.State,
		NextState:     StateExpired,
	}
	return next, event, true, nil
}

func eventKindForCommand(kind CommandKind) EventKind {
	switch kind {
	case CommandConfirmIdentity:
		return EventIdentityConfirmed
	case CommandAcceptContract:
		return EventContractAccepted
	case CommandSendPayment:
		return EventPaymentMarkedSent
	case CommandReceivePayment:
		return EventPaymentMarkedReceived
	case CommandReject:
		return EventOfferRejected
	case CommandCancel:
		return EventOfferCancelled
	case CommandExpire:
		return EventOfferExpired
	default:
		return ""
	}
}

// AllowedActions returns a presentation hint for one participant. Apply remains
// authoritative and revalidates every command against the current aggregate.
func AllowedActions(offer Offer, actorID string, now time.Time) []CommandKind {
	if !validOffer(offer) || actorID == "" || now.IsZero() || !now.Before(offer.ExpiresAt) ||
		(actorID != offer.HostID && actorID != offer.RenterID) || offer.Revision == math.MaxUint64 {
		return nil
	}

	actions := make([]CommandKind, 0, 3)
	switch offer.State {
	case StatePendingIdentity:
		if _, confirmed := offer.IdentityConfirmed[actorID]; !confirmed {
			actions = append(actions, CommandConfirmIdentity)
		}
		actions = append(actions, CommandReject, CommandCancel)
	case StatePendingContract:
		if _, accepted := offer.ContractAccepted[actorID]; !accepted {
			actions = append(actions, CommandAcceptContract)
		}
		actions = append(actions, CommandReject, CommandCancel)
	case StatePendingPaymentSend:
		if actorID == offer.RenterID {
			actions = append(actions, CommandSendPayment)
		}
		actions = append(actions, CommandCancel)
	case StatePendingPaymentReceive:
		if actorID == offer.HostID {
			actions = append(actions, CommandReceivePayment)
		}
		actions = append(actions, CommandCancel)
	}
	if len(actions) == 0 {
		return nil
	}
	return actions
}

func terminalState(state State) bool {
	switch state {
	case StateConfirmed, StateRejected, StateCancelled, StateExpired:
		return true
	default:
		return false
	}
}

func rejectableState(state State) bool {
	return state == StatePendingIdentity || state == StatePendingContract
}

func cancellableState(state State) bool {
	switch state {
	case StatePendingIdentity, StatePendingContract, StatePendingPaymentSend, StatePendingPaymentReceive:
		return true
	default:
		return false
	}
}

// ValidReasonCode reports whether code is part of the closed cancellation catalog.
func ValidReasonCode(code string) bool {
	switch code {
	case "condiciones_no_aceptadas", "cambio_de_planes", "error_en_la_oferta", "otro":
		return true
	default:
		return false
	}
}

// ValidContractDigest reports whether digest has canonical lowercase SHA-256 hex form.
func ValidContractDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, char := range digest {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func validPaymentID(paymentID string) bool {
	return validCanonicalUUID(paymentID)
}

func validCanonicalUUID(value string) bool {
	parsedID, err := uuid.Parse(value)
	return err == nil && parsedID != uuid.Nil && parsedID.String() == value
}

// ValidateOffer checks a persisted or externally reconstructed aggregate before
// exposing it through a transport projection.
func ValidateOffer(offer Offer) error {
	if !validOffer(offer) {
		return ErrInvalid
	}
	return nil
}

func validOffer(offer Offer) bool {
	if !validCanonicalUUID(offer.ID) || !validCanonicalUUID(offer.HostID) || !validCanonicalUUID(offer.RenterID) ||
		offer.HostID == offer.RenterID ||
		offer.Revision == 0 || offer.ContractVersion <= 0 ||
		offer.ExpiresAt.IsZero() || offer.ExpiresAt.After(offer.Snapshot.StartAt) ||
		!validOfferSnapshot(offer.Snapshot) || !validState(offer.State) ||
		!validParticipantTimes(offer.IdentityConfirmed, offer.HostID, offer.RenterID) ||
		!validParticipantTimes(offer.ContractAccepted, offer.HostID, offer.RenterID) {
		return false
	}
	contractDigest, err := DigestContractText(offer.ContractText)
	if err != nil || contractDigest != offer.ContractDigest {
		return false
	}

	identityComplete := confirmedByBoth(offer.IdentityConfirmed, offer.HostID, offer.RenterID)
	contractComplete := confirmedByBoth(offer.ContractAccepted, offer.HostID, offer.RenterID)
	for _, confirmedAt := range offer.IdentityConfirmed {
		if !confirmedAt.Before(offer.ExpiresAt) {
			return false
		}
	}
	if len(offer.ContractAccepted) > 0 && !identityComplete {
		return false
	}
	identityLatest := latestTimestamp(offer.IdentityConfirmed)
	for _, acceptedAt := range offer.ContractAccepted {
		if acceptedAt.Before(identityLatest) || !acceptedAt.Before(offer.ExpiresAt) {
			return false
		}
	}
	if offer.PaymentSentAt == nil {
		if offer.PaymentID != "" {
			return false
		}
	} else if !validPaymentID(offer.PaymentID) {
		return false
	}
	if offer.PaymentSentAt != nil {
		if !contractComplete || offer.PaymentSentAt.IsZero() || offer.PaymentSentAt.Before(latestTimestamp(offer.ContractAccepted)) || !offer.PaymentSentAt.Before(offer.ExpiresAt) {
			return false
		}
	}
	if offer.PaymentReceivedAt != nil {
		if offer.PaymentSentAt == nil || offer.PaymentReceivedAt.IsZero() || offer.PaymentReceivedAt.Before(*offer.PaymentSentAt) || !offer.PaymentReceivedAt.Before(offer.ExpiresAt) {
			return false
		}
	}

	switch offer.State {
	case StatePendingIdentity:
		return !identityComplete && len(offer.ContractAccepted) == 0 && offer.PaymentSentAt == nil && offer.PaymentReceivedAt == nil
	case StatePendingContract:
		return identityComplete && !contractComplete && offer.PaymentSentAt == nil && offer.PaymentReceivedAt == nil
	case StatePendingPaymentSend:
		return identityComplete && contractComplete && offer.PaymentSentAt == nil && offer.PaymentReceivedAt == nil
	case StatePendingPaymentReceive:
		return contractComplete && offer.PaymentSentAt != nil && offer.PaymentReceivedAt == nil
	case StateConfirmed:
		return contractComplete && offer.PaymentSentAt != nil && offer.PaymentReceivedAt != nil
	case StateRejected:
		return len(offer.ContractAccepted) < 2 && offer.PaymentSentAt == nil && offer.PaymentReceivedAt == nil
	case StateCancelled, StateExpired:
		return offer.PaymentReceivedAt == nil
	default:
		return false
	}
}

func validParticipantTimes(values map[string]time.Time, hostID, renterID string) bool {
	for actorID, at := range values {
		if (actorID != hostID && actorID != renterID) || at.IsZero() {
			return false
		}
	}
	return true
}

func latestTimestamp(values map[string]time.Time) time.Time {
	var latest time.Time
	for _, at := range values {
		if at.After(latest) {
			latest = at
		}
	}
	return latest
}

func validState(state State) bool {
	switch state {
	case StatePendingIdentity, StatePendingContract, StatePendingPaymentSend, StatePendingPaymentReceive,
		StateConfirmed, StateRejected, StateCancelled, StateExpired:
		return true
	default:
		return false
	}
}

func cloneOffer(offer Offer) Offer {
	next := offer
	next.IdentityConfirmed = cloneTimes(offer.IdentityConfirmed)
	next.ContractAccepted = cloneTimes(offer.ContractAccepted)
	if offer.PaymentSentAt != nil {
		at := *offer.PaymentSentAt
		next.PaymentSentAt = &at
	}
	if offer.PaymentReceivedAt != nil {
		at := *offer.PaymentReceivedAt
		next.PaymentReceivedAt = &at
	}
	return next
}

func cloneTimes(values map[string]time.Time) map[string]time.Time {
	copy := make(map[string]time.Time, len(values)+1)
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func confirmedByBoth(values map[string]time.Time, hostID, renterID string) bool {
	_, hostConfirmed := values[hostID]
	_, renterConfirmed := values[renterID]
	return hostConfirmed && renterConfirmed
}
