package spaces

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Service struct {
	repo               Repository
	locationBoundaries LocationBoundaryChecker
	locationCatalog    LocationCatalog
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("spaces: repository required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) Categories(ctx context.Context) ([]Category, error) { return s.repo.Categories(ctx) }
func (s *Service) Profile(ctx context.Context, category string, version int) (Profile, error) {
	return s.repo.Profile(ctx, category, version)
}
func (s *Service) Create(ctx context.Context, owner string, in Input) (Draft, error) {
	if !validOwner(owner) {
		return Draft{}, ErrInvalid
	}
	if fields := in.FieldErrors(); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	profile, err := s.repo.Profile(ctx, in.CategoryCode, 0)
	if errors.Is(err, ErrNotFound) {
		return Draft{}, NewValidationError([]FieldError{{Field: "category_code", Code: "invalid_value"}})
	}
	if err != nil {
		return Draft{}, err
	}
	if in.AttributeSchemaVersion != 0 && profile.SchemaVersion != in.AttributeSchemaVersion {
		return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
	}
	if fields := profile.AttributeFieldErrorsForArea(in.Attributes, in.AreaM2); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	in.AttributeSchemaVersion = profile.SchemaVersion
	if in.Attributes == nil {
		in.Attributes = map[string]any{}
	}
	return s.repo.Create(ctx, owner, in)
}

// CreateWithLocation validates a canonical map pin before the repository writes
// the draft and its location in one database transaction.
func (s *Service) CreateWithLocation(ctx context.Context, owner string, in CreateInput) (Draft, error) {
	if !validOwner(owner) {
		return Draft{}, ErrInvalid
	}
	loc := in.Location
	if !in.LocationPublicConfirmedProvided {
		return Draft{}, NewValidationError([]FieldError{{Field: "location.public_confirmed", Code: "required"}})
	}
	if fields := loc.FieldErrors(); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	if loc.Latitude == nil || loc.Longitude == nil || loc.RegionCode == "" || loc.CommuneCode == "" {
		return Draft{}, NewValidationError([]FieldError{{Field: "location.latitude", Code: "required"}, {Field: "location.longitude", Code: "required"}, {Field: "location.region_code", Code: "required"}, {Field: "location.comuna_code", Code: "required"}})
	}
	if err := s.ResolvePin(ctx, *loc.Latitude, *loc.Longitude, loc.RegionCode, loc.CommuneCode); err != nil {
		return Draft{}, err
	}
	base := Input{CategoryCode: in.CategoryCode, Title: in.Title, Description: in.Description, AreaM2: in.AreaM2, Capacity: in.Capacity, UsageRules: in.UsageRules, CancellationPolicyVersion: in.CancellationPolicyVersion, RateUnit: in.RateUnit, BasePriceCLP: in.BasePriceCLP, Address: loc.Address, RegionCode: loc.RegionCode, CommuneCode: loc.CommuneCode, IsB2B: in.IsB2B, AttributeSchemaVersion: in.AttributeSchemaVersion, Attributes: in.Attributes}
	if fields := base.FieldErrors(); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	profile, err := s.repo.Profile(ctx, base.CategoryCode, 0)
	if errors.Is(err, ErrNotFound) {
		return Draft{}, NewValidationError([]FieldError{{Field: "category_code", Code: "invalid_value"}})
	}
	if err != nil {
		return Draft{}, err
	}
	if base.AttributeSchemaVersion != 0 && profile.SchemaVersion != base.AttributeSchemaVersion {
		return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
	}
	if fields := profile.AttributeFieldErrorsForArea(base.Attributes, base.AreaM2); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	base.AttributeSchemaVersion = profile.SchemaVersion
	if base.Attributes == nil {
		base.Attributes = map[string]any{}
	}
	type creator interface {
		CreateWithLocation(context.Context, string, Input, LocationInput) (Draft, error)
	}
	r, ok := s.repo.(creator)
	if !ok {
		return Draft{}, errors.New("spaces: atomic location creation repository required")
	}
	return r.CreateWithLocation(ctx, owner, base, loc)
}
func (s *Service) ListOwn(ctx context.Context, owner string) ([]Draft, error) {
	if !validOwner(owner) {
		return nil, ErrInvalid
	}
	return s.repo.ListOwn(ctx, owner)
}
func (s *Service) GetOwn(ctx context.Context, owner, id string) (Draft, error) {
	if !validOwner(owner) || !validID(id) {
		return Draft{}, ErrNotFound
	}
	return s.repo.GetOwn(ctx, owner, id)
}
func (s *Service) UpdateOwnIfRevision(ctx context.Context, owner, id string, expectedUpdatedAt time.Time, in Input) (Draft, error) {
	if !validOwner(owner) || !validID(id) {
		return Draft{}, ErrNotFound
	}
	existing, err := s.repo.GetOwn(ctx, owner, id)
	if err != nil {
		return Draft{}, err
	}
	if !in.AddressProvided && in.Address == "" {
		in.Address = existing.Address
	}
	if !in.RegionCodeProvided && in.RegionCode == "" && existing.RegionCode != nil {
		in.RegionCode = *existing.RegionCode
	}
	if !in.CommuneCodeProvided && in.CommuneCode == "" && existing.CommuneCode != nil {
		in.CommuneCode = *existing.CommuneCode
	}
	if fields := in.FieldErrors(); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	version := in.AttributeSchemaVersion
	if version == 0 {
		if in.CategoryCode != existing.CategoryCode {
			return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
		}
		version = existing.AttributeSchemaVersion
	}
	profile, err := s.repo.Profile(ctx, in.CategoryCode, version)
	if errors.Is(err, ErrNotFound) {
		return Draft{}, NewValidationError([]FieldError{{Field: "category_code", Code: "invalid_value"}})
	}
	if err != nil {
		return Draft{}, err
	}
	if profile.SchemaVersion != version {
		return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
	}
	if fields := profile.AttributeFieldErrorsForArea(in.Attributes, in.AreaM2); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	in.AttributeSchemaVersion = version
	if in.Attributes == nil {
		in.Attributes = map[string]any{}
	}
	repository, ok := s.repo.(RevisionAwareDraftRepository)
	if !ok {
		return Draft{}, errors.New("spaces: revision-aware draft repository required")
	}
	return repository.UpdateOwnIfRevision(ctx, owner, id, expectedUpdatedAt, in)
}

func (s *Service) UpdateOwn(ctx context.Context, owner, id string, in Input) (Draft, error) {
	if !validOwner(owner) || !validID(id) {
		return Draft{}, ErrNotFound
	}
	// Existing drafts stay on their saved schema. A client may explicitly
	// select a different category/profile, but we never silently upgrade it.
	existing, err := s.repo.GetOwn(ctx, owner, id)
	if err != nil {
		return Draft{}, err
	}
	if !in.AddressProvided && in.Address == "" {
		in.Address = existing.Address
	}
	if !in.RegionCodeProvided && in.RegionCode == "" && existing.RegionCode != nil {
		in.RegionCode = *existing.RegionCode
	}
	if !in.CommuneCodeProvided && in.CommuneCode == "" && existing.CommuneCode != nil {
		in.CommuneCode = *existing.CommuneCode
	}
	if fields := in.FieldErrors(); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	version := in.AttributeSchemaVersion
	if version == 0 {
		if in.CategoryCode != existing.CategoryCode {
			return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
		}
		version = existing.AttributeSchemaVersion
	}
	profile, err := s.repo.Profile(ctx, in.CategoryCode, version)
	if errors.Is(err, ErrNotFound) {
		return Draft{}, NewValidationError([]FieldError{{Field: "category_code", Code: "invalid_value"}})
	}
	if err != nil {
		return Draft{}, err
	}
	if profile.SchemaVersion != version {
		return Draft{}, NewValidationError([]FieldError{{Field: "attribute_schema_version", Code: "invalid_value"}})
	}
	if fields := profile.AttributeFieldErrorsForArea(in.Attributes, in.AreaM2); len(fields) != 0 {
		return Draft{}, NewValidationError(fields)
	}
	in.AttributeSchemaVersion = version
	if in.Attributes == nil {
		in.Attributes = map[string]any{}
	}
	return s.repo.UpdateOwn(ctx, owner, id, in)
}

// SetPublicationState changes only the owner's local listing visibility. The
// repository serializes the transition with account suppression and fixture
// state inside one transaction; identity eligibility belongs to reservations.
func (s *Service) SetPublicationState(ctx context.Context, owner, id, state, correlationID string) (Draft, error) {
	if !validOwner(owner) || !validID(id) || (state != "activa" && state != "oculta") || strings.TrimSpace(correlationID) == "" || len(correlationID) > 120 {
		return Draft{}, ErrInvalid
	}
	return s.repo.SetPublicationState(ctx, owner, id, state, correlationID)
}

// SetPublicationStateIfRevision never falls back to an unversioned writer.
func (s *Service) SetPublicationStateIfRevision(ctx context.Context, owner, id, state, correlationID string, expected time.Time) (Draft, error) {
	if !validOwner(owner) || !validID(id) || (state != "activa" && state != "oculta") || strings.TrimSpace(correlationID) == "" || len(correlationID) > 120 || expected.IsZero() {
		return Draft{}, ErrInvalid
	}
	r, ok := s.repo.(RevisionAwarePublicationRepository)
	if !ok {
		return Draft{}, errors.New("spaces: revision-aware publication repository required")
	}
	return r.SetPublicationStateIfRevision(ctx, owner, id, state, correlationID, expected.UTC())
}

func (s *Service) UpdatePublishedOwnIfRevision(ctx context.Context, owner, id string, expected time.Time, input PublishedContentInput) (Draft, error) {
	if expected.IsZero() {
		return Draft{}, ErrInvalid
	}
	if !validOwner(owner) || !validID(id) {
		return Draft{}, ErrNotFound
	}
	if input.Validate() != nil {
		return Draft{}, NewValidationError(input.FieldErrors())
	}
	r, ok := s.repo.(RevisionAwarePublishedContentRepository)
	if !ok {
		return Draft{}, errors.New("spaces: revision-aware published content repository required")
	}
	return r.UpdatePublishedOwnIfRevision(ctx, owner, id, expected.UTC(), input)
}

// UpdatePublishedOwn edits supported content fields of an existing active/hidden
// publication. It does not change publication state or grant eligibility; new
// reservations continue to apply their own KYC gate.
func (s *Service) UpdatePublishedOwn(ctx context.Context, owner, id string, input PublishedContentInput) (Draft, error) {
	if !validOwner(owner) || !validID(id) {
		return Draft{}, ErrNotFound
	}
	if input.Validate() != nil {
		return Draft{}, NewValidationError(input.FieldErrors())
	}
	return s.repo.UpdatePublishedOwn(ctx, owner, id, input)
}

func (s *Service) DeleteOwn(ctx context.Context, owner, id, correlationID string) (Deletion, error) {
	if !validOwner(owner) || !validID(id) || strings.TrimSpace(correlationID) == "" || len(correlationID) > 120 {
		return Deletion{}, ErrInvalid
	}
	return s.repo.DeleteOwn(ctx, owner, id, correlationID)
}

// DeleteAsAdministrator delegates to the same terminal-retirement operation
// while preserving the real administrator actor and target owner separately.
// The repository rechecks global membership and applies all obligation,
// snapshot, gallery-cleanup, and history rules inside its transaction.
func (s *Service) DeleteAsAdministrator(ctx context.Context, actor, owner, id, correlationID string, expectedUpdatedAt time.Time, reason string) (Deletion, error) {
	if !validOwner(actor) || !validOwner(owner) || !validID(id) || expectedUpdatedAt.IsZero() || strings.TrimSpace(correlationID) == "" || len(correlationID) > 120 || strings.TrimSpace(reason) == "" || len(reason) > 60 {
		return Deletion{}, ErrInvalid
	}
	repository, ok := s.repo.(AdministrativeDeletionRepository)
	if !ok {
		return Deletion{}, ErrNotFound
	}
	return repository.DeleteAsAdministrator(ctx, actor, owner, id, correlationID, expectedUpdatedAt.UTC(), reason)
}

func (s *Service) MonthlyAvailability(ctx context.Context, spaceID, month string) (MonthlyAvailability, error) {
	if !validID(spaceID) {
		return MonthlyAvailability{}, ErrNotFound
	}
	r, ok := s.repo.(AvailabilityRepository)
	if !ok {
		return MonthlyAvailability{}, errors.New("spaces: availability repository required")
	}
	return r.GetMonthlyAvailability(ctx, spaceID, month)
}

func (s *Service) UpdateAvailability(ctx context.Context, owner, spaceID string, in AvailabilityUpdateInput) (AvailabilityRule, string, error) {
	if !validOwner(owner) || !validID(spaceID) {
		return AvailabilityRule{}, "", ErrNotFound
	}
	r, ok := s.repo.(AvailabilityRepository)
	if !ok {
		return AvailabilityRule{}, "", errors.New("spaces: availability repository required")
	}
	return r.UpdateAvailability(ctx, owner, spaceID, in)
}

func validateAttributes(profile Profile, input Input) error {
	return profile.ValidateAttributesForArea(input.Attributes, input.AreaM2)
}

var uuidPattern = func() *regexp.Regexp {
	return regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
}()

func ValidID(v string) bool    { return uuidPattern.MatchString(v) }
func validID(v string) bool    { return ValidID(v) }
func validOwner(v string) bool { return ValidID(strings.TrimSpace(v)) }
