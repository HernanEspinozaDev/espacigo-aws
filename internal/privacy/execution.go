package privacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	PurposeCommercialComms        = "comunicaciones_comerciales"
	PurposeProductAnalytics       = "analitica_producto"
	PurposeRecommendationPersonal = "personalizacion_recomendaciones"
	PurposeOptionalProcessors     = "comparticion_encargados_opcionales"

	PurposeStateAllowed   = "permitido"
	PurposeStateOpposed   = "opuesto"
	PurposeStateWithdrawn = "retirado"

	ScopeCommercialComms   = "comunicaciones_comerciales"
	ScopeAnalyticsProfile  = "analitica_y_perfilado"
	ScopePublicationsVis   = "publicaciones_y_visibilidad"
	ScopeExportsThirdParty = "exportaciones_y_terceros"
	ScopeGeneralTreatment  = "tratamiento_general"

	PortabilitySchemaVersion = "portabilidad_ley21719_v1"
)

// AllPurposes returns the canonical ordered list of optional processing
// purposes exposed to data subjects under Law 21.719.
func AllPurposes() []string {
	return []string{
		PurposeCommercialComms,
		PurposeProductAnalytics,
		PurposeRecommendationPersonal,
		PurposeOptionalProcessors,
	}
}

// ValidPurpose reports whether purpose is one of the canonical processing purposes.
func ValidPurpose(purpose string) bool {
	switch purpose {
	case PurposeCommercialComms, PurposeProductAnalytics, PurposeRecommendationPersonal, PurposeOptionalProcessors:
		return true
	default:
		return false
	}
}

// ValidPurposeState reports whether state is an admitted purpose preference state.
func ValidPurposeState(state string) bool {
	switch state {
	case PurposeStateAllowed, PurposeStateOpposed, PurposeStateWithdrawn:
		return true
	default:
		return false
	}
}

// ValidTreatmentScope reports whether scope is an admitted treatment restriction scope.
func ValidTreatmentScope(scope string) bool {
	switch scope {
	case ScopeCommercialComms, ScopeAnalyticsProfile, ScopePublicationsVis, ScopeExportsThirdParty, ScopeGeneralTreatment:
		return true
	default:
		return false
	}
}

// ValidateProfileRectification validates and normalizes optional profile fields
// for rectification. At least one pointer must be non-nil. When phone is non-nil
// and trims to "", cleanPhone points to "" so callers can clear it to SQL NULL.
func ValidateProfileRectification(displayName, phone *string) (cleanName *string, cleanPhone *string, ok bool) {
	if displayName == nil && phone == nil {
		return nil, nil, false
	}
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if !utf8.ValidString(trimmed) {
			return nil, nil, false
		}
		n := utf8.RuneCountInString(trimmed)
		if n < 1 || n > 120 {
			return nil, nil, false
		}
		for _, r := range trimmed {
			if !(unicode.IsLetter(r) || r == ' ') {
				return nil, nil, false
			}
		}
		cleanName = &trimmed
	}
	if phone != nil {
		trimmed := strings.TrimSpace(*phone)
		if !utf8.ValidString(trimmed) {
			return nil, nil, false
		}
		if trimmed != "" && !nineDigits.MatchString(trimmed) {
			return nil, nil, false
		}
		cleanPhone = &trimmed
	}
	return cleanName, cleanPhone, true
}

type PurposePreference struct {
	Purpose   string     `json:"purpose"`
	State     string     `json:"state"`
	Reason    *string    `json:"reason,omitempty"`
	RequestID *string    `json:"request_id,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type UpdatePurposePreferenceInput struct {
	Purpose string `json:"purpose"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
}

type TreatmentRestriction struct {
	ID         string     `json:"id"`
	RequestID  string     `json:"request_id"`
	Scope      string     `json:"scope"`
	State      string     `json:"state"`
	Reason     string     `json:"reason"`
	AppliedAt  time.Time  `json:"applied_at"`
	ReviewAt   *time.Time `json:"review_at,omitempty"`
	LiftedAt   *time.Time `json:"lifted_at,omitempty"`
	LiftReason *string    `json:"lift_reason,omitempty"`
}

// PortabilityPackage is the structured, interoperable owner export defined for
// Law 21.719 portability. Slices are always serialized as JSON arrays ([]).
type PortabilityPackage struct {
	SchemaVersion         string                 `json:"schema_version"`
	GeneratedAt           time.Time              `json:"generated_at"`
	Account               ExportAccount          `json:"account"`
	Profile               *ExportProfile         `json:"profile,omitempty"`
	Roles                 []string               `json:"roles"`
	Acceptances           []TermsAcceptance      `json:"terms_acceptances"`
	PurposePreferences    []PurposePreference    `json:"purpose_preferences"`
	TreatmentRestrictions []TreatmentRestriction `json:"treatment_restrictions"`
	Requests              []RightsRequest        `json:"rights_requests"`
}

// RightsExecutionRepository persists and reads subject-level purpose preferences
// and treatment restrictions for ARCO+ execution.
type RightsExecutionRepository interface {
	ListOwnPurposePreferences(ctx context.Context, accountID string) ([]PurposePreference, error)
	UpsertOwnPurposePreference(ctx context.Context, accountID string, in UpdatePurposePreferenceInput, now time.Time) (PurposePreference, error)
	ListOwnTreatmentRestrictions(ctx context.Context, accountID string) ([]TreatmentRestriction, error)
	HasActiveTreatmentBlock(ctx context.Context, accountID, scope string) (bool, error)
}

// BuildPortabilityJSON deterministically serializes pkg with non-nil slices and
// returns the UTF-8 JSON bytes alongside their lowercase SHA-256 hex digest.
func BuildPortabilityJSON(pkg PortabilityPackage) ([]byte, string, error) {
	if pkg.SchemaVersion == "" {
		pkg.SchemaVersion = PortabilitySchemaVersion
	}
	pkg.GeneratedAt = pkg.GeneratedAt.UTC()
	if pkg.Roles == nil {
		pkg.Roles = []string{}
	}
	if pkg.Acceptances == nil {
		pkg.Acceptances = []TermsAcceptance{}
	}
	if pkg.PurposePreferences == nil {
		pkg.PurposePreferences = []PurposePreference{}
	}
	if pkg.TreatmentRestrictions == nil {
		pkg.TreatmentRestrictions = []TreatmentRestriction{}
	}
	if pkg.Requests == nil {
		pkg.Requests = []RightsRequest{}
	}
	encoded, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(sum[:]), nil
}

func (s *Service) OwnPurposePreferences(ctx context.Context, accountID string) ([]PurposePreference, error) {
	if s == nil || s.repo == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return nil, ErrInvalid
	}
	repo, ok := s.repo.(RightsExecutionRepository)
	if !ok {
		return nil, errors.New("privacy: rights execution repository unavailable")
	}
	rows, err := repo.ListOwnPurposePreferences(ctx, accountID)
	if err != nil {
		return nil, err
	}
	byPurpose := make(map[string]PurposePreference, len(rows))
	for _, row := range rows {
		byPurpose[row.Purpose] = row
	}
	purposes := AllPurposes()
	out := make([]PurposePreference, 0, len(purposes))
	for _, purpose := range purposes {
		if pref, exists := byPurpose[purpose]; exists {
			out = append(out, pref)
			continue
		}
		out = append(out, PurposePreference{
			Purpose: purpose,
			State:   PurposeStateAllowed,
		})
	}
	return out, nil
}

func (s *Service) UpdateOwnPurposePreference(ctx context.Context, accountID string, in UpdatePurposePreferenceInput) (PurposePreference, error) {
	if s == nil || s.repo == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return PurposePreference{}, ErrInvalid
	}
	if !ValidPurpose(in.Purpose) || !ValidPurposeState(in.State) {
		return PurposePreference{}, ErrInvalid
	}
	reason := strings.TrimSpace(in.Reason)
	if !utf8.ValidString(reason) || strings.ContainsRune(reason, 0) {
		return PurposePreference{}, ErrInvalid
	}
	if reason != "" {
		n := utf8.RuneCountInString(reason)
		if n < 5 || n > 500 {
			return PurposePreference{}, ErrInvalid
		}
	}
	repo, ok := s.repo.(RightsExecutionRepository)
	if !ok {
		return PurposePreference{}, errors.New("privacy: rights execution repository unavailable")
	}
	now := s.clockNow().UTC().Truncate(time.Microsecond)
	return repo.UpsertOwnPurposePreference(ctx, accountID, UpdatePurposePreferenceInput{
		Purpose: in.Purpose,
		State:   in.State,
		Reason:  reason,
	}, now)
}

func (s *Service) OwnTreatmentRestrictions(ctx context.Context, accountID string) ([]TreatmentRestriction, error) {
	if s == nil || s.repo == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return nil, ErrInvalid
	}
	repo, ok := s.repo.(RightsExecutionRepository)
	if !ok {
		return nil, errors.New("privacy: rights execution repository unavailable")
	}
	items, err := repo.ListOwnTreatmentRestrictions(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []TreatmentRestriction{}, nil
	}
	return items, nil
}

func (s *Service) HasActiveTreatmentBlock(ctx context.Context, accountID, scope string) (bool, error) {
	if s == nil || s.repo == nil || ctx == nil || strings.TrimSpace(accountID) == "" || !ValidTreatmentScope(scope) {
		return false, ErrInvalid
	}
	repo, ok := s.repo.(RightsExecutionRepository)
	if !ok {
		return false, errors.New("privacy: rights execution repository unavailable")
	}
	return repo.HasActiveTreatmentBlock(ctx, accountID, scope)
}

func (s *Service) ExportOwnPortability(ctx context.Context, accountID string) ([]byte, string, error) {
	if s == nil || s.repo == nil || ctx == nil || strings.TrimSpace(accountID) == "" {
		return nil, "", ErrInvalid
	}
	data, err := s.ExportOwnData(ctx, accountID)
	if err != nil {
		return nil, "", err
	}
	prefs, err := s.OwnPurposePreferences(ctx, accountID)
	if err != nil {
		return nil, "", err
	}
	restrictions, err := s.OwnTreatmentRestrictions(ctx, accountID)
	if err != nil {
		return nil, "", err
	}
	pkg := PortabilityPackage{
		SchemaVersion:         PortabilitySchemaVersion,
		GeneratedAt:           s.clockNow().UTC(),
		Account:               data.Account,
		Profile:               data.Profile,
		Roles:                 data.Roles,
		Acceptances:           data.Acceptances,
		PurposePreferences:    prefs,
		TreatmentRestrictions: restrictions,
		Requests:              data.Requests,
	}
	return BuildPortabilityJSON(pkg)
}
