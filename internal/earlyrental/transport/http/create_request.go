package earlyrentalhttp

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMalformedCreateOfferBody = errors.New("malformed early-rental create body")
	ErrInvalidCreateOfferValue  = errors.New("invalid early-rental create value")
)

// CreateOfferRequest contains only the client-selectable resource and interval.
type CreateOfferRequest struct {
	SpaceID uuid.UUID
	StartAt time.Time
	EndAt   time.Time
}

// DecodeCreateOfferRequest validates the exact create DTO without deriving any
// actor, rate, unit, price, contract, or reservation state from client input.
func DecodeCreateOfferRequest(body []byte) (CreateOfferRequest, error) {
	var payload struct {
		SpaceID *string `json:"space_id"`
		StartAt *string `json:"start_at"`
		EndAt   *string `json:"end_at"`
	}
	if err := decodeCommandJSON(body, &payload, "space_id", "start_at", "end_at"); err != nil {
		return CreateOfferRequest{}, ErrMalformedCreateOfferBody
	}
	if payload.SpaceID == nil || payload.StartAt == nil || payload.EndAt == nil {
		return CreateOfferRequest{}, ErrMalformedCreateOfferBody
	}

	spaceID, err := uuid.Parse(*payload.SpaceID)
	if err != nil || strings.ToLower(*payload.SpaceID) != spaceID.String() {
		return CreateOfferRequest{}, ErrInvalidCreateOfferValue
	}
	startAt, err := time.Parse(time.RFC3339Nano, *payload.StartAt)
	if err != nil {
		return CreateOfferRequest{}, ErrInvalidCreateOfferValue
	}
	endAt, err := time.Parse(time.RFC3339Nano, *payload.EndAt)
	if err != nil || !endAt.After(startAt) {
		return CreateOfferRequest{}, ErrInvalidCreateOfferValue
	}
	return CreateOfferRequest{SpaceID: spaceID, StartAt: startAt.UTC(), EndAt: endAt.UTC()}, nil
}
