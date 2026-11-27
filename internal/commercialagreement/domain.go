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
	ErrOTPRequired  = errors.New("commercialagreement: otp verification required")
	ErrOTPInvalid   = errors.New("commercialagreement: invalid otp code")
	ErrOTPExpired   = errors.New("commercialagreement: otp challenge expired")
)

// SigningProvider abstracts the signature modality (internal FES today,
// external FEA / FirmaVirtual notarial in future deliveries).
type SigningProvider interface {
	Mode() string
}

const (
	ModeFESInternal          = "fes_interna"
	ModeFirmaVirtualNotarial = "firmavirtual_notarial"
	ModeFEAExternal          = "fea_externa"
)

type Actor struct {
	ID            string
	SessionID     string
	IPMasked      string
	UserAgentHash []byte
}
type OTPChallengeInput struct {
	ProposalID  string `json:"proposal_id"`
	SHA256      string `json:"snapshot_sha256"`
	Role        string `json:"role"`
	AgreementID string `json:"agreement_id,omitempty"`
}
type OTPChallengeResult struct {
	ChallengeID string    `json:"challenge_id"`
	Channel     string    `json:"channel"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaskedEmail string    `json:"masked_email"`
}
type CreateInput struct {
	ProposalID     string `json:"proposal_id"`
	SHA256         string `json:"snapshot_sha256"`
	Consent        bool   `json:"consent"`
	ConsentVersion string `json:"consent_version"`
	OTPChallengeID string `json:"otp_challenge_id,omitempty"`
	OTPCode        string `json:"otp_code,omitempty"`
	SignatureMode  string `json:"signature_mode,omitempty"`
}
type SignInput struct {
	SHA256         string `json:"snapshot_sha256"`
	Consent        bool   `json:"consent"`
	ConsentVersion string `json:"consent_version"`
	OTPChallengeID string `json:"otp_challenge_id,omitempty"`
	OTPCode        string `json:"otp_code,omitempty"`
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
	SignatureType   string    `json:"signature_type,omitempty"`
	AuthMethod      string    `json:"auth_method,omitempty"`
	EvidenceSHA256  string    `json:"evidence_sha256,omitempty"`
}
type CompanyRepresentation struct {
	OrganizationID            string `json:"organization_id"`
	LegalName                 string `json:"legal_name"`
	MaskedRUT                 string `json:"rut_masked"`
	RUTVerified               bool   `json:"rut_verified"`
	RepresentativeUserID      string `json:"representative_user_id"`
	RepresentativeName        string `json:"representative_name"`
	RepresentativeRUT         string `json:"representative_rut_masked"`
	RepresentativeRUTVerified bool   `json:"representative_rut_verified"`
}
type Agreement struct {
	ID                    string                 `json:"id"`
	ProposalID            string                 `json:"proposal_id"`
	QuoteID               string                 `json:"quote_id"`
	SpaceID               string                 `json:"space_id"`
	OwnerID               string                 `json:"-"`
	RenterID              string                 `json:"-"`
	ProposalVersion       int64                  `json:"proposal_version"`
	SHA256                string                 `json:"snapshot_sha256"`
	State                 string                 `json:"state"`
	ExpiresAt             time.Time              `json:"expires_at"`
	PaymentExpiresAt      *time.Time             `json:"payment_expires_at"`
	OccupancyID           *string                `json:"-"`
	SignatureMode         string                 `json:"signature_mode,omitempty"`
	CompanyRepresentation *CompanyRepresentation `json:"company_representation,omitempty"`
	Revision              int64                  `json:"revision"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
	Signatures            []Signature            `json:"signatures"`
	Replayed              bool                   `json:"replayed,omitempty"`
}
type Repository interface {
	Create(context.Context, Actor, CreateInput, string) (Agreement, error)
	Accept(context.Context, Actor, string, int64, SignInput) (Agreement, error)
	Decide(context.Context, Actor, string, int64, DecisionInput) (Agreement, error)
	Get(context.Context, string, string) (Agreement, error)
	List(context.Context, string, string) ([]Agreement, error)
	ExpireDue(context.Context, int) (int, error)
}
type OTPRepository interface {
	CreateOTPChallenge(context.Context, Actor, OTPChallengeInput) (OTPChallengeResult, string, string, error)
}
type Service struct {
	repo       Repository
	otpRepo    OTPRepository
	otpSender  Sender
	requireOTP bool
}

func New(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, ErrInvalid
	}
	return &Service{repo: repo}, nil
}
func (s *Service) ConfigureOTP(otpRepo OTPRepository, sender Sender, require bool) {
	s.otpRepo = otpRepo
	s.otpSender = sender
	s.requireOTP = require
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
func validOTPCode(code string) bool {
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
func (s *Service) RequestOTPChallenge(ctx context.Context, a Actor, in OTPChallengeInput) (OTPChallengeResult, error) {
	if s.otpRepo == nil || s.otpSender == nil {
		return OTPChallengeResult{}, ErrInvalid
	}
	if !validActor(a) || !ValidID(in.ProposalID) || !ValidSHA(in.SHA256) || (in.Role != "arrendatario" && in.Role != "arrendador") {
		return OTPChallengeResult{}, ErrInvalid
	}
	if in.AgreementID != "" && !ValidID(in.AgreementID) {
		return OTPChallengeResult{}, ErrInvalid
	}
	res, email, code, err := s.otpRepo.CreateOTPChallenge(ctx, a, in)
	if err != nil {
		return OTPChallengeResult{}, err
	}
	subject := "Código de firma electrónica EspaciGo"
	body := "Tu código de verificación para firmar el contrato es: " + code + ". Expira en 5 minutos. No lo compartas con nadie."
	if err := s.otpSender.SendLocalBookingNotice(ctx, email, subject, body); err != nil {
		return OTPChallengeResult{}, err
	}
	return res, nil
}
func (s *Service) Create(ctx context.Context, a Actor, in CreateInput, key string) (Agreement, error) {
	if !validActor(a) || !ValidID(in.ProposalID) || !ValidSHA(in.SHA256) || !in.Consent || in.ConsentVersion != ConsentVersion || !ValidKey(key) {
		return Agreement{}, ErrInvalid
	}
	if s.requireOTP && (!ValidID(in.OTPChallengeID) || !validOTPCode(in.OTPCode)) {
		return Agreement{}, ErrOTPRequired
	}
	return s.repo.Create(ctx, a, in, key)
}
func (s *Service) Accept(ctx context.Context, a Actor, id string, rev int64, in SignInput) (Agreement, error) {
	if !validActor(a) || !ValidID(id) || rev < 1 || !ValidSHA(in.SHA256) || !in.Consent || in.ConsentVersion != ConsentVersion {
		return Agreement{}, ErrInvalid
	}
	if s.requireOTP && (!ValidID(in.OTPChallengeID) || !validOTPCode(in.OTPCode)) {
		return Agreement{}, ErrOTPRequired
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
