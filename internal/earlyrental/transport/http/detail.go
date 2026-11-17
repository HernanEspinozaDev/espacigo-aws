package earlyrentalhttp

import (
	"errors"
	"math"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

var errInvalidEarlyRentalDetail = errors.New("early rental detail cannot be projected")

const earlyRentalFlowType = "manual_early_v1"

type earlyRentalDetailEnvelope struct {
	Data earlyRentalDetailView `json:"data"`
}

type earlyRentalDetailView struct {
	ID                    string                    `json:"id"`
	FlowType              string                    `json:"flow_type"`
	ActorRole             string                    `json:"actor_role"`
	SpaceID               string                    `json:"space_id"`
	SpaceTitle            string                    `json:"space_title"`
	RateUnit              string                    `json:"rate_unit"`
	UnitPriceCLP          int64                     `json:"unit_price_clp"`
	Currency              string                    `json:"currency"`
	Units                 int64                     `json:"units"`
	SubtotalCLP           int64                     `json:"subtotal_clp"`
	StartAt               time.Time                 `json:"start_at"`
	EndAt                 time.Time                 `json:"end_at"`
	TimeZone              string                    `json:"time_zone"`
	State                 earlyrental.State         `json:"state"`
	Revision              int64                     `json:"revision"`
	LastEventSequence     int64                     `json:"last_event_sequence"`
	CreatedAt             time.Time                 `json:"created_at"`
	UpdatedAt             time.Time                 `json:"updated_at"`
	ExpiresAt             time.Time                 `json:"expires_at"`
	IdentityConfirmations earlyRentalTimestampPair  `json:"identity_confirmations"`
	Contract              earlyRentalContractView   `json:"contract"`
	Payment               *earlyRentalPaymentView   `json:"payment"`
	AllowedActions        []earlyrental.CommandKind `json:"allowed_actions"`
}

type earlyRentalTimestampPair struct {
	Renter *time.Time `json:"arrendatario"`
	Host   *time.Time `json:"arrendador"`
}

type earlyRentalContractView struct {
	Version     int64                    `json:"version"`
	Digest      string                   `json:"digest"`
	TermsText   string                   `json:"terms_text"`
	Acceptances earlyRentalTimestampPair `json:"acceptances"`
}

type earlyRentalPaymentView struct {
	ID         string     `json:"id"`
	AmountCLP  int64      `json:"amount_clp"`
	Currency   string     `json:"currency"`
	State      string     `json:"state"`
	SentAt     *time.Time `json:"sent_at"`
	ReceivedAt *time.Time `json:"received_at"`
}

// projectEarlyRentalDetail emits the participant-safe OpenAPI detail shape.
// createdAt and updatedAt must come from the same authoritative read as offer.
func projectEarlyRentalDetail(offer earlyrental.Offer, actorID string, createdAt, updatedAt, now time.Time) (earlyRentalDetailEnvelope, error) {
	if earlyrental.ValidateOffer(offer) != nil || actorID == "" || now.IsZero() || createdAt.IsZero() || updatedAt.IsZero() ||
		updatedAt.Before(createdAt) || offer.Revision > uint64(math.MaxInt64) {
		return earlyRentalDetailEnvelope{}, errInvalidEarlyRentalDetail
	}

	actorRole := ""
	switch actorID {
	case offer.RenterID:
		actorRole = earlyRentalRoleRenter
	case offer.HostID:
		actorRole = earlyRentalRoleHost
	default:
		return earlyRentalDetailEnvelope{}, errInvalidEarlyRentalDetail
	}

	actions := earlyrental.AllowedActions(offer, actorID, now)
	if actions == nil {
		actions = make([]earlyrental.CommandKind, 0)
	}
	snapshot := offer.Snapshot
	view := earlyRentalDetailView{
		ID: offer.ID, FlowType: earlyRentalFlowType, ActorRole: actorRole,
		SpaceID: snapshot.SpaceID.String(), SpaceTitle: snapshot.SpaceTitle,
		RateUnit: snapshot.RateUnit, UnitPriceCLP: snapshot.UnitPriceCLP, Currency: snapshot.Currency,
		Units: snapshot.Units, SubtotalCLP: snapshot.SubtotalCLP,
		StartAt: snapshot.StartAt.UTC(), EndAt: snapshot.EndAt.UTC(), TimeZone: snapshot.TimeZone,
		State: offer.State, Revision: int64(offer.Revision), LastEventSequence: int64(offer.Revision),
		CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC(), ExpiresAt: offer.ExpiresAt.UTC(),
		IdentityConfirmations: earlyRentalTimestampPair{Renter: participantTime(offer.IdentityConfirmed, offer.RenterID), Host: participantTime(offer.IdentityConfirmed, offer.HostID)},
		Contract: earlyRentalContractView{
			Version: offer.ContractVersion, Digest: offer.ContractDigest, TermsText: offer.ContractText,
			Acceptances: earlyRentalTimestampPair{Renter: participantTime(offer.ContractAccepted, offer.RenterID), Host: participantTime(offer.ContractAccepted, offer.HostID)},
		},
		AllowedActions: actions,
	}
	if paymentState, hasPayment := earlyRentalPaymentState(offer); hasPayment {
		view.Payment = &earlyRentalPaymentView{
			ID: offer.PaymentID, AmountCLP: snapshot.SubtotalCLP, Currency: snapshot.Currency, State: paymentState,
			SentAt: participantTimeValue(offer.PaymentSentAt), ReceivedAt: participantTimeValue(offer.PaymentReceivedAt),
		}
	}
	return earlyRentalDetailEnvelope{Data: view}, nil
}

func participantTime(values map[string]time.Time, actorID string) *time.Time {
	at, ok := values[actorID]
	if !ok {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func participantTimeValue(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}
