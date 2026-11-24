// Package commercialauthorization records a landlord's bounded, revocable
// authorization for a published contract version. It never represents a
// second human signature or legal consent.
package commercialauthorization

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/google/uuid"
)

var (
	ErrInvalid     = errors.New("commercialauthorization: invalid")
	ErrNotFound    = errors.New("commercialauthorization: not found")
	ErrConflict    = errors.New("commercialauthorization: conflict")
	ErrOTPRequired = errors.New("commercialauthorization: otp required")
	ErrOTPInvalid  = errors.New("commercialauthorization: otp invalid")
	ErrOTPExpired  = errors.New("commercialauthorization: otp expired")
)

const (
	MaxAmountCLP int64 = 100_000_000
	MaxValidity        = 90 * 24 * time.Hour
)

type Grant struct {
	ID               string     `json:"id"`
	SpaceID          string     `json:"space_id"`
	OwnerID          string     `json:"owner_id"`
	TemplateVersion  int64      `json:"template_version"`
	MaxAmountCLP     int64      `json:"max_amount_clp"`
	ValidFrom        time.Time  `json:"valid_from"`
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	RevokedBy        string     `json:"revoked_by"`
	RevocationReason string     `json:"revocation_reason"`
}

type Event struct {
	GrantID    string    `json:"grant_id"`
	SpaceID    string    `json:"space_id"`
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

type Activation struct {
	TemplateVersion int64
	MaxAmountCLP    int64
	ValidFrom       time.Time
	ExpiresAt       time.Time
	Consent         bool
	OTPChallengeID  string
	OTPCode         string
}

type OTPChallengeInput struct {
	TemplateVersion int64 `json:"template_version"`
}

type OTPChallengeResult struct {
	ChallengeID string    `json:"challenge_id"`
	Channel     string    `json:"channel"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaskedEmail string    `json:"masked_email"`
}

type Repository interface {
	Current(context.Context, string, string, time.Time) (Grant, error)
	History(context.Context, string, string) ([]Event, error)
	CreateOTPChallenge(context.Context, string, string, OTPChallengeInput, time.Time) (OTPChallengeResult, string, string, error)
	Activate(context.Context, string, string, Activation, time.Time) (Grant, error)
	Revoke(context.Context, string, string, string, string, time.Time) (Grant, error)
}

type Decision struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason"`
}

func ValidateActivation(a Activation, now time.Time) bool {
	if now.IsZero() || a.TemplateVersion < 1 || a.MaxAmountCLP < 1 || a.MaxAmountCLP > MaxAmountCLP || a.ValidFrom.IsZero() || a.ExpiresAt.IsZero() {
		return false
	}
	from, until, current := a.ValidFrom.UTC(), a.ExpiresAt.UTC(), now.UTC()
	return !from.Before(current.Add(-time.Minute)) && until.After(from) && until.Sub(from) <= MaxValidity
}

// ValidOTPCode verifies that code is a 6-digit numeric OTP string.
func ValidOTPCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Evaluate determines whether a bounded pre-authorization mandate applies to
// the proposal terms at now. When Eligible is true, the request qualifies for
// automatic confirmation within the landlord's configured limits.
// When grant.MaxAmountCLP >= MaxAmountCLP, no upper CLP cap is enforced.
func Evaluate(grant Grant, terms commercialproposal.Terms, now time.Time) Decision {
	switch {
	case now.IsZero() || grant.ID == "" || grant.RevokedAt != nil:
		return Decision{Reason: "authorization_unavailable"}
	case now.Before(grant.ValidFrom) || !now.Before(grant.ExpiresAt):
		return Decision{Reason: "authorization_expired_or_not_yet_valid"}
	case grant.TemplateVersion < 1 || terms.SpaceID != grant.SpaceID || terms.TemplateVersion != grant.TemplateVersion:
		return Decision{Reason: "template_version_mismatch"}
	case terms.Currency != "CLP" || terms.SubtotalCLP < 1 || (grant.MaxAmountCLP < MaxAmountCLP && terms.SubtotalCLP > grant.MaxAmountCLP):
		return Decision{Reason: "amount_out_of_bounds"}
	default:
		return Decision{Eligible: true, Reason: "within_authorized_limits"}
	}
}

func ValidIDs(ids ...string) bool {
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return false
		}
	}
	return true
}
