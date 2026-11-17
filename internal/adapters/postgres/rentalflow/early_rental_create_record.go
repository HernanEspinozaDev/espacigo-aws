package rentalflowpg

import (
	"encoding/hex"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

type earlyRentalInitialRecord struct {
	Offer          earlyrental.Offer
	OccupancyID    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ContractDigest []byte
	InitialEvent   earlyrental.Event
}

func newEarlyRentalInitialRecord(offer earlyrental.Offer, occupancyID string, event earlyrental.Event, createdAt time.Time) (earlyRentalInitialRecord, error) {
	invalid := func() (earlyRentalInitialRecord, error) { return earlyRentalInitialRecord{}, earlyrental.ErrInvalid }
	if earlyrental.ValidateOffer(offer) != nil || offer.State != earlyrental.StatePendingIdentity || offer.Revision != 1 || len(offer.IdentityConfirmed) != 0 || len(offer.ContractAccepted) != 0 || offer.PaymentID != "" || offer.PaymentSentAt != nil || offer.PaymentReceivedAt != nil {
		return invalid()
	}
	id, err := uuid.Parse(occupancyID)
	if err != nil || id == uuid.Nil || id.String() != occupancyID {
		return invalid()
	}
	if createdAt.IsZero() || !createdAt.Before(offer.Snapshot.StartAt) {
		return invalid()
	}
	expiry := createdAt.Add(earlyrental.HoldTTL)
	if offer.Snapshot.StartAt.Before(expiry) {
		expiry = offer.Snapshot.StartAt
	}
	if !offer.ExpiresAt.Equal(expiry) {
		return invalid()
	}
	if event.Kind != earlyrental.EventOfferCreated || event.ActorID != offer.RenterID || event.Sequence != 1 || event.Revision != 1 || event.NextState != earlyrental.StatePendingIdentity || event.PreviousState != "" || !event.At.Equal(createdAt) || event.ContractVersion != 0 || event.ContractDigest != "" || event.ReasonCode != "" || event.PaymentID != nil {
		return invalid()
	}
	digest, err := hex.DecodeString(offer.ContractDigest)
	if err != nil || len(digest) != 32 || !earlyrental.ValidContractDigest(offer.ContractDigest) {
		return invalid()
	}
	offer.IdentityConfirmed = cloneInitialMap(offer.IdentityConfirmed)
	offer.ContractAccepted = cloneInitialMap(offer.ContractAccepted)
	offer.Snapshot.StartAt = offer.Snapshot.StartAt.UTC()
	offer.Snapshot.EndAt = offer.Snapshot.EndAt.UTC()
	offer.ExpiresAt = offer.ExpiresAt.UTC()
	event.At = event.At.UTC()
	return earlyRentalInitialRecord{Offer: offer, OccupancyID: occupancyID, CreatedAt: createdAt.UTC(), UpdatedAt: createdAt.UTC(), ContractDigest: append([]byte(nil), digest...), InitialEvent: event}, nil
}

func cloneInitialMap(in map[string]time.Time) map[string]time.Time {
	if in == nil {
		return nil
	}
	out := make(map[string]time.Time, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
