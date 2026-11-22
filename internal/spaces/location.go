package spaces

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
)

// LocationInput replaces the complete location. Confirmation covers the exact
// address, administrative codes and pin submitted in this request.
type LocationInput struct {
	Address         string   `json:"address"`
	RegionCode      string   `json:"region_code"`
	CommuneCode     string   `json:"comuna_code"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	PublicConfirmed bool     `json:"public_confirmed"`
}

type CommercialLocation struct {
	LocationInput
	SpaceID     string     `json:"space_id"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	Revision    int64      `json:"revision"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type LocationRepository interface {
	GetOwnLocation(context.Context, string, string) (CommercialLocation, error)
	SetOwnLocation(context.Context, string, string, time.Time, LocationInput) (CommercialLocation, error)
}

var commercialAddressContact = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}|(?:https?://|www\.|mailto:|tel:|wa\.me/|t\.me/)|(?:\+?56[ .-]?)?\b[2-9][ .-]?\d{4}[ .-]?\d{4}\b|\b\d{1,2}(?:\.\d{3}){2}-[\dk]\b|\b\d{7,8}-[\dk]\b`)

func (i LocationInput) FieldErrors() []FieldError {
	var fields []FieldError
	if i.PublicConfirmed && commercialAddressContact.MatchString(i.Address) {
		fields = append(fields, FieldError{"address", "contact_not_allowed"})
	}
	if strings.TrimSpace(i.Address) == "" {
		fields = append(fields, FieldError{"address", "required"})
	} else if len([]rune(i.Address)) > 500 {
		fields = append(fields, FieldError{"address", "max_length"})
	}
	if (i.RegionCode == "") != (i.CommuneCode == "") {
		fields = append(fields, FieldError{"region_code", "location_pair"}, FieldError{"comuna_code", "location_pair"})
	} else if i.RegionCode != "" && !validPublicLocationCodes(i.RegionCode, i.CommuneCode) {
		fields = append(fields, FieldError{"comuna_code", "invalid_value"})
	}
	if (i.Latitude == nil) != (i.Longitude == nil) {
		fields = append(fields, FieldError{"latitude", "coordinate_pair"}, FieldError{"longitude", "coordinate_pair"})
	}
	for _, v := range []struct {
		name  string
		value *float64
		limit float64
	}{{"latitude", i.Latitude, 90}, {"longitude", i.Longitude, 180}} {
		if v.value != nil && (math.IsNaN(*v.value) || math.IsInf(*v.value, 0) || math.Abs(*v.value) > v.limit) {
			fields = append(fields, FieldError{v.name, "invalid_value"})
		}
	}
	if i.PublicConfirmed && (i.Latitude == nil || i.Longitude == nil || i.RegionCode == "" || i.CommuneCode == "") {
		fields = append(fields, FieldError{"public_confirmed", "location_required"})
	}
	return fields
}

func (s *Service) GetOwnLocation(ctx context.Context, owner, id string) (CommercialLocation, error) {
	if !validOwner(owner) || !validID(id) {
		return CommercialLocation{}, ErrNotFound
	}
	r, ok := s.repo.(LocationRepository)
	if !ok {
		return CommercialLocation{}, errors.New("spaces: location repository required")
	}
	return r.GetOwnLocation(ctx, owner, id)
}
func (s *Service) SetOwnLocation(ctx context.Context, owner, id string, expected time.Time, in LocationInput) (CommercialLocation, error) {
	if !validOwner(owner) || !validID(id) {
		return CommercialLocation{}, ErrNotFound
	}
	if expected.IsZero() {
		return CommercialLocation{}, ErrInvalid
	}
	if fields := in.FieldErrors(); len(fields) != 0 {
		return CommercialLocation{}, NewValidationError(fields)
	}
	r, ok := s.repo.(LocationRepository)
	if !ok {
		return CommercialLocation{}, errors.New("spaces: location repository required")
	}
	in.Address = strings.TrimSpace(in.Address)
	return r.SetOwnLocation(ctx, owner, id, expected.UTC(), in)
}
