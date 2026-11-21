package rentalflow

import (
	"context"
	"errors"
)

var ErrReservationUnavailable = errors.New("rental flow: commercial reservations unavailable")

type CommercialReservation struct {
	ID               string `json:"id"`
	QuoteID          string `json:"quote_id"`
	SpaceID          string `json:"space_id"`
	SpaceTitle       string `json:"space_title"`
	ParticipantRole  string `json:"participant_role"`
	Status           string `json:"status"`
	CheckoutStatus   string `json:"checkout_status"`
	RateUnit         string `json:"rate_unit"`
	UnitPriceCLP     int64  `json:"unit_price_clp"`
	Units            int64  `json:"units"`
	SubtotalCLP      int64  `json:"subtotal_clp"`
	Currency         string `json:"currency"`
	StartAt          string `json:"start_at"`
	EndAt            string `json:"end_at"`
	TimeZone         string `json:"time_zone"`
	PaymentExpiresAt string `json:"payment_expires_at"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type CommercialReservationPage struct {
	Items      []CommercialReservation `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type CommercialReservationReader interface {
	ListCommercialReservations(context.Context, string, int, string) (CommercialReservationPage, error)
	GetCommercialReservation(context.Context, string, string) (CommercialReservation, error)
}

type CommercialReservationService struct{ reader CommercialReservationReader }

func NewCommercialReservationService(reader CommercialReservationReader) (*CommercialReservationService, error) {
	if reader == nil {
		return nil, ErrInvalid
	}
	return &CommercialReservationService{reader: reader}, nil
}

func (s *CommercialReservationService) List(ctx context.Context, participantID string, pageSize int, cursor string) (CommercialReservationPage, error) {
	if s == nil || ctx == nil || !validUUID(participantID) || pageSize < 1 || pageSize > 100 || len(cursor) > 4096 {
		return CommercialReservationPage{}, ErrInvalid
	}
	page, err := s.reader.ListCommercialReservations(ctx, canonicalUUID(participantID), pageSize, cursor)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return CommercialReservationPage{}, ErrInvalid
		}
		if errors.Is(err, ErrNotFound) {
			return CommercialReservationPage{}, ErrNotFound
		}
		return CommercialReservationPage{}, ErrReservationUnavailable
	}
	if page.Items == nil {
		page.Items = []CommercialReservation{}
	}
	return page, nil
}

func (s *CommercialReservationService) Get(ctx context.Context, participantID, reservationID string) (CommercialReservation, error) {
	if s == nil || ctx == nil || !validUUID(participantID) || !validUUID(reservationID) {
		return CommercialReservation{}, ErrInvalid
	}
	item, err := s.reader.GetCommercialReservation(ctx, canonicalUUID(participantID), canonicalUUID(reservationID))
	if errors.Is(err, ErrNotFound) {
		return CommercialReservation{}, ErrNotFound
	}
	if errors.Is(err, ErrInvalid) {
		return CommercialReservation{}, ErrInvalid
	}
	if err != nil {
		return CommercialReservation{}, ErrReservationUnavailable
	}
	return item, nil
}
