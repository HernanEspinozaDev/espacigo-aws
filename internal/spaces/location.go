package spaces

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
)

var ErrLocationBoundaryUnavailable = errors.New("spaces: location boundary data unavailable")

// LocationInput replaces the complete location. Confirmation covers the exact
// address, administrative codes and pin submitted in this request.
type LocationInput struct {
	Address            string   `json:"address"`
	RegionCode         string   `json:"region_code"`
	CommuneCode        string   `json:"comuna_code"`
	Latitude           *float64 `json:"latitude"`
	Longitude          *float64 `json:"longitude"`
	Source             string   `json:"source,omitempty"`
	Precision          string   `json:"precision,omitempty"`
	LocationVisibility string   `json:"location_visibility,omitempty"`
	PublicConfirmed    bool     `json:"public_confirmed"`
}

var (
	chileanPostalCodeToken = regexp.MustCompile(`(^|[\s,])\d{7}($|[\s,])`)
	emptyCommaSegments     = regexp.MustCompile(`\s*,\s*(?:,\s*)+`)
	spaceBeforeComma       = regexp.MustCompile(`\s+,`)
	multiWhitespace        = regexp.MustCompile(`\s+`)
)

// StripPostalCodeFromAddress removes standalone Chilean 7-digit postal codes
// from geocoded or stored addresses without altering 1-5 digit street numbers.
func StripPostalCodeFromAddress(address string) string {
	s := strings.TrimSpace(address)
	if s == "" {
		return ""
	}
	for {
		next := chileanPostalCodeToken.ReplaceAllString(s, "$1$2")
		if next == s {
			break
		}
		s = next
	}
	s = emptyCommaSegments.ReplaceAllString(s, ", ")
	s = spaceBeforeComma.ReplaceAllString(s, ",")
	s = multiWhitespace.ReplaceAllString(s, " ")
	return strings.Trim(s, " \t\r\n,")
}

// DeterministicApproximatePoint computes a deterministic ~250m offset from exact
// coordinates using the space ID so the public projection is stable across requests.
func DeterministicApproximatePoint(spaceID string, lat, lng float64) (float64, float64) {
	sum := sha256.Sum256([]byte("espacigo.approx.v1:" + spaceID))
	angle := (float64(binary.BigEndian.Uint32(sum[:4])) / float64(math.MaxUint32)) * 2 * math.Pi
	const radiusMeters = 250.0
	dLat := (radiusMeters * math.Cos(angle)) / 111320.0
	cosLat := math.Cos(lat * math.Pi / 180.0)
	if math.Abs(cosLat) < 0.15 {
		cosLat = 0.15
	}
	dLng := (radiusMeters * math.Sin(angle)) / (111320.0 * cosLat)
	return lat + dLat, lng + dLng
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

// LocationBoundaryChecker checks a WGS84 point against a selected CUT commune.
// The second result reports whether the supplied dataset covers that code.
type LocationBoundaryChecker interface {
	Contains(code string, latitude, longitude float64) (inside, known bool)
	Bounds(code string) (west, south, east, north float64, known bool)
}

type LocationCatalog interface {
	ResolveLocation(context.Context, string, string) (region, commune string, found bool, err error)
}

// SetLocationBoundaryChecker configures authoritative pin validation before
// serving requests. Mutations fail closed when either geometry or catalog is absent.
func (s *Service) SetLocationBoundaryChecker(checker LocationBoundaryChecker) {
	s.locationBoundaries = checker
}

func (s *Service) SetLocationCatalog(catalog LocationCatalog) { s.locationCatalog = catalog }

func (s *Service) HasLocationBoundaryChecker() bool { return s != nil && s.locationBoundaries != nil }

// CommuneBounds returns a map extent only for a recognized CUT catalogue code
// covered by the configured geometry. Unknown and uncovered codes are alike.
func (s *Service) CommuneBounds(code string) (west, south, east, north float64, known bool) {
	if s.locationBoundaries == nil || len(code) != 5 || !validPublicLocationCodes(code[:2], code) {
		return 0, 0, 0, 0, false
	}
	return s.locationBoundaries.Bounds(code)
}

// FindCommune locates a pin in the configured official commune boundaries.
func (s *Service) FindCommune(latitude, longitude float64) (string, bool) {
	if s.locationBoundaries == nil {
		return "", false
	}
	locator, ok := s.locationBoundaries.(interface {
		Find(float64, float64) (string, bool)
	})
	if !ok {
		return "", false
	}
	return locator.Find(latitude, longitude)
}

// ResolvePin confirms both supplied CUT codes against the same authoritative
// polygon used by reverse geocoding. Missing geometry is distinguishable from
// a pin that simply has no matching commune.
func (s *Service) ResolvePin(ctx context.Context, latitude, longitude float64, region, commune string) error {
	resolvedRegion, resolvedCommune, err := s.ResolvePinCoordinates(ctx, latitude, longitude)
	if err != nil {
		return err
	}
	if resolvedCommune != commune || resolvedRegion != region {
		return NewValidationError([]FieldError{{Field: "location.comuna_code", Code: "outside_selected_commune"}})
	}
	return nil
}

func (s *Service) ResolvePinCoordinates(ctx context.Context, latitude, longitude float64) (string, string, error) {
	if s.locationBoundaries == nil || s.locationCatalog == nil {
		return "", "", ErrLocationBoundaryUnavailable
	}
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 || math.IsNaN(latitude) || math.IsNaN(longitude) || math.IsInf(latitude, 0) || math.IsInf(longitude, 0) {
		return "", "", NewValidationError([]FieldError{{Field: "location.latitude", Code: "invalid_value"}, {Field: "location.longitude", Code: "invalid_value"}})
	}
	code, found := s.FindCommune(latitude, longitude)
	if !found {
		return "", "", NewValidationError([]FieldError{{Field: "location.latitude", Code: "no_commune_match"}})
	}
	region, commune := code[:2], code
	_, _, found, err := s.locationCatalog.ResolveLocation(ctx, region, commune)
	if err != nil {
		return "", "", ErrLocationBoundaryUnavailable
	}
	if !found {
		return "", "", NewValidationError([]FieldError{{Field: "location.comuna_code", Code: "invalid_value"}})
	}
	return region, commune, nil
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
	if i.Source != "" {
		if i.Source != "manual" && i.Source != "geocoded" {
			fields = append(fields, FieldError{"source", "invalid_value"})
		} else if i.Latitude == nil || i.Longitude == nil {
			fields = append(fields, FieldError{"source", "coordinate_pair"})
		}
	}
	if i.Source == "geocoded" {
		if i.Precision != "address" && i.Precision != "approximate" {
			fields = append(fields, FieldError{"precision", "invalid_value"})
		}
	} else if i.Precision != "" {
		fields = append(fields, FieldError{"precision", "invalid_value"})
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
	current, err := r.GetOwnLocation(ctx, owner, id)
	if err != nil {
		return CommercialLocation{}, err
	}
	if !expected.UTC().Equal(current.UpdatedAt.UTC()) {
		return CommercialLocation{}, ErrPreconditionFailed
	}
	if in.Latitude == nil || in.Longitude == nil || in.RegionCode == "" || in.CommuneCode == "" {
		return CommercialLocation{}, NewValidationError([]FieldError{{Field: "latitude", Code: "location_required"}, {Field: "longitude", Code: "location_required"}, {Field: "region_code", Code: "location_required"}, {Field: "comuna_code", Code: "location_required"}})
	}
	if err := s.ResolvePin(ctx, *in.Latitude, *in.Longitude, in.RegionCode, in.CommuneCode); err != nil {
		return CommercialLocation{}, err
	}
	in.Address = strings.TrimSpace(in.Address)
	return r.SetOwnLocation(ctx, owner, id, expected.UTC(), in)
}
