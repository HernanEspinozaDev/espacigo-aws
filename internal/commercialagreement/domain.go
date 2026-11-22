// Package commercialagreement records bilateral decisions on immutable,
// identity-bound commercial proposals. Payment and document states are separate.
package commercialagreement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ConsentVersion = "commercial-agreement-v1"
const RequestLifetime = 24 * time.Hour
const (
	Pending   = "pending_landlord"
	Ready     = "ready_for_payment"
	Rejected  = "rejected"
	Withdrawn = "withdrawn"
	Expired   = "expired"
)

var (
	ErrInvalid      = errors.New("commercialagreement: invalid request")
	ErrNotFound     = errors.New("commercialagreement: not found")
	ErrConflict     = errors.New("commercialagreement: state conflict")
	ErrPrecondition = errors.New("commercialagreement: stale revision")
	ErrHash         = errors.New("commercialagreement: proposal hash mismatch")
	ErrIdempotency  = errors.New("commercialagreement: idempotency key conflict")
	ErrExpired      = errors.New("commercialagreement: request expired")
	ErrIdentity     = errors.New("commercialagreement: verified identities required")
)

type Actor struct{ ID, SessionID string }
type CreateInput struct {
	ProposalID     string `json:"proposal_id"`
	SHA256         string `json:"snapshot_sha256"`
	Consent        bool   `json:"consent"`
	ConsentVersion string `json:"consent_version"`
}
type SignInput struct {
	SHA256         string `json:"snapshot_sha256"`
	Consent        bool   `json:"consent"`
	ConsentVersion string `json:"consent_version"`
}
type DecisionInput struct {
	SHA256   string `json:"snapshot_sha256"`
	Decision string `json:"decision"`
}
type Signature struct {
	Role            string    `json:"role"`
	SignedAt        time.Time `json:"signed_at"`
	SHA256          string    `json:"snapshot_sha256"`
	ProposalVersion int64     `json:"proposal_version"`
	ConsentVersion  string    `json:"consent_version"`
}
type Agreement struct {
	ID               string      `json:"id"`
	ProposalID       string      `json:"proposal_id"`
	QuoteID          string      `json:"quote_id"`
	SpaceID          string      `json:"space_id"`
	OwnerID          string      `json:"-"`
	RenterID         string      `json:"-"`
	ProposalVersion  int64       `json:"proposal_version"`
	SHA256           string      `json:"snapshot_sha256"`
	State            string      `json:"state"`
	ExpiresAt        time.Time   `json:"expires_at"`
	PaymentExpiresAt *time.Time  `json:"payment_expires_at"`
	OccupancyID      *string     `json:"-"`
	Revision         int64       `json:"revision"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Signatures       []Signature `json:"signatures"`
	Replayed         bool        `json:"replayed,omitempty"`
}
type Repository interface {
	Create(context.Context, Actor, CreateInput, string) (Agreement, error)
	Accept(context.Context, Actor, string, int64, SignInput) (Agreement, error)
	Decide(context.Context, Actor, string, int64, DecisionInput) (Agreement, error)
	Get(context.Context, string, string) (Agreement, error)
	List(context.Context, string, string) ([]Agreement, error)
	ExpireDue(context.Context, int) (int, error)
}
type Service struct{ repo Repository }

func New(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, ErrInvalid
	}
	return &Service{repo: repo}, nil
}
func ValidID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func ValidSHA(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && strings.ToLower(s) == s
}
func validActor(a Actor) bool { return ValidID(a.ID) && ValidID(a.SessionID) }
func ValidKey(key string) bool {
	if len(key) < 1 || len(key) > 200 || strings.TrimSpace(key) != key {
		return false
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func Fingerprint(in CreateInput) []byte {
	blob, _ := json.Marshal(in)
	sum := sha256.Sum256(blob)
	return sum[:]
}
func (s *Service) Create(ctx context.Context, a Actor, in CreateInput, key string) (Agreement, error) {
	if !validActor(a) || !ValidID(in.ProposalID) || !ValidSHA(in.SHA256) || !in.Consent || in.ConsentVersion != ConsentVersion || !ValidKey(key) {
		return Agreement{}, ErrInvalid
	}
	return s.repo.Create(ctx, a, in, key)
}
func (s *Service) Accept(ctx context.Context, a Actor, id string, rev int64, in SignInput) (Agreement, error) {
	if !validActor(a) || !ValidID(id) || rev < 1 || !ValidSHA(in.SHA256) || !in.Consent || in.ConsentVersion != ConsentVersion {
		return Agreement{}, ErrInvalid
	}
	return s.repo.Accept(ctx, a, id, rev, in)
}
func (s *Service) Decide(ctx context.Context, a Actor, id string, rev int64, in DecisionInput) (Agreement, error) {
	if !validActor(a) || !ValidID(id) || rev < 1 || !ValidSHA(in.SHA256) || (in.Decision != "reject" && in.Decision != "withdraw") {
		return Agreement{}, ErrInvalid
	}
	return s.repo.Decide(ctx, a, id, rev, in)
}
func (s *Service) Get(ctx context.Context, a, id string) (Agreement, error) {
	if !ValidID(a) || !ValidID(id) {
		return Agreement{}, ErrNotFound
	}
	return s.repo.Get(ctx, a, id)
}
func (s *Service) List(ctx context.Context, a, role string) ([]Agreement, error) {
	if !ValidID(a) || (role != "" && role != "landlord" && role != "renter") {
		return nil, ErrInvalid
	}
	return s.repo.List(ctx, a, role)
}
func (s *Service) ExpireDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	return s.repo.ExpireDue(ctx, limit)
}
