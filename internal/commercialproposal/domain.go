package commercialproposal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
)

var (
	ErrInvalid             = errors.New("commercialproposal: invalid")
	ErrNotFound            = errors.New("commercialproposal: not found")
	ErrConflict            = errors.New("commercialproposal: changed terms")
	ErrLocationRequired    = errors.New("commercialproposal: confirmed location required")
	ErrTemplateRequired    = errors.New("commercialproposal: published template required")
	ErrTermsUnavailable    = errors.New("commercialproposal: cancellation policy unavailable")
	ErrEvidenceUnavailable = errors.New("commercialproposal: private evidence unavailable")
)

type CancellationPolicy struct {
	Version string `json:"version"`
	Text    string `json:"text"`
}

func (p CancellationPolicy) Valid() bool {
	if len(p.Version) < 1 || len(p.Version) > 100 || p.Version != strings.TrimSpace(p.Version) {
		return false
	}
	_, _, err := contracttemplate.CanonicalSections([]contracttemplate.Section{{Key: "cancellation_policy", Title: "Política de cancelación", Body: p.Text}})
	return err == nil
}

type Location struct {
	Address     string  `json:"address"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Revision    int64   `json:"revision"`
	RegionCode  string  `json:"region_code"`
	CommuneCode string  `json:"commune_code"`
}

// Terms deliberately excludes all user identifiers and legal identities.
type Terms struct {
	QuoteID            string                     `json:"quote_id"`
	SpaceID            string                     `json:"space_id"`
	SpaceTitle         string                     `json:"space_title"`
	TemplateID         string                     `json:"template_id"`
	TemplateVersion    int64                      `json:"template_version"`
	Location           Location                   `json:"location"`
	Sections           []contracttemplate.Section `json:"sections"`
	RateUnit           string                     `json:"rate_unit"`
	RateVersion        int64                      `json:"rate_version"`
	UnitPriceCLP       int64                      `json:"unit_price_clp"`
	Units              int64                      `json:"units"`
	SubtotalCLP        int64                      `json:"subtotal_clp"`
	Currency           string                     `json:"currency"`
	StartAt            time.Time                  `json:"start_at"`
	EndAt              time.Time                  `json:"end_at"`
	TimeZone           string                     `json:"time_zone"`
	CancellationPolicy CancellationPolicy         `json:"cancellation_policy"`
}
type EvidenceParty struct {
	Role      string `json:"role"`
	SubjectID string `json:"subject_id"`
	LegalName string `json:"legal_name"`
	RUT       string `json:"rut"`
}
type Record struct {
	ID         string
	Version    int64
	OwnerID    string
	RenterID   string
	Terms      Terms
	SHA256     string
	KeyVersion string
	Ciphertext []byte
	CreatedAt  time.Time
}
type View struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
	Terms
	Parties       []rentalflow.ContractParty `json:"parties"`
	ContactNotice string                     `json:"contact_notice"`
	GeneratedAt   time.Time                  `json:"generated_at"`
}
type Repository interface {
	CurrentTerms(context.Context, string, rentalflow.Quote, CancellationPolicy) (Terms, error)
	Create(context.Context, string, rentalflow.Quote, Record, func() error) (Record, error)
	Get(context.Context, string, string) (Record, error)
}
type Cipher interface {
	Seal([]byte, []byte) (string, []byte, error)
	Open(string, []byte, []byte) ([]byte, error)
}
type Previewer interface {
	Preview(context.Context, string, string) (rentalflow.ContractPreview, error)
}
type Service struct {
	repo       Repository
	quotes     rentalflow.QuoteRepository
	preview    Previewer
	gate       rentalflow.IdentityGate
	identities rentalflow.ContractIdentityReader
	cipher     Cipher
	policy     CancellationPolicy
	now        func() time.Time
}

func New(repo Repository, quotes rentalflow.QuoteRepository, preview Previewer, gate rentalflow.IdentityGate, identities rentalflow.ContractIdentityReader, cipher Cipher, policy CancellationPolicy, now func() time.Time) *Service {
	return &Service{repo: repo, quotes: quotes, preview: preview, gate: gate, identities: identities, cipher: cipher, policy: policy, now: now}
}
func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func AAD(r Record) []byte {
	blob, _ := json.Marshal(r.Terms)
	return []byte("espacigo:commercial-contract-proposal:v1:" + r.ID + ":" + r.Terms.QuoteID + ":" + Digest(blob))
}
func document(terms Terms, parties []EvidenceParty) []byte {
	blob, _ := json.Marshal(struct {
		FormatVersion int             `json:"format_version"`
		Terms         Terms           `json:"terms"`
		Parties       []EvidenceParty `json:"parties"`
	}{1, terms, parties})
	return blob
}

func (s *Service) Create(ctx context.Context, actor, quoteID string) (View, error) {
	if !contracttemplate.ValidIDs(actor, quoteID) {
		return View{}, ErrInvalid
	}
	actor = uuid.MustParse(actor).String()
	quoteID = uuid.MustParse(quoteID).String()
	// Even an unconfigured proposal service does not disclose a private quote.
	if _, err := s.preview.Preview(ctx, actor, quoteID); err != nil {
		return View{}, err
	}
	if !s.policy.Valid() {
		return View{}, ErrTermsUnavailable
	}
	if s.cipher == nil {
		return View{}, ErrEvidenceUnavailable
	}
	quote, err := s.quotes.GetForParticipant(ctx, actor, quoteID)
	if err != nil {
		return View{}, err
	}
	terms, err := s.repo.CurrentTerms(ctx, actor, quote, s.policy)
	if err != nil {
		return View{}, err
	}
	parties := make([]EvidenceParty, 0, 2)
	asOf := s.now().UTC()
	for i, id := range []string{quote.OwnerID, quote.RenterID} {
		p, e := s.identities.CurrentForRentalContract(ctx, actor, quoteID, id, asOf)
		if e != nil {
			return View{}, rentalflow.ErrIdentityUnavailable
		}
		p, valid := rentalflow.NormalizeContractIdentity(p, id)
		if !valid {
			return View{}, rentalflow.ErrIdentityUnavailable
		}
		role := "arrendador"
		if i == 1 {
			role = "arrendatario"
		}
		parties = append(parties, EvidenceParty{Role: role, SubjectID: id, LegalName: p.LegalName, RUT: p.RUT})
	}
	r := Record{ID: uuid.NewString(), OwnerID: quote.OwnerID, RenterID: quote.RenterID, Terms: terms, SHA256: Digest(document(terms, parties))}
	private, _ := json.Marshal(parties)
	r.KeyVersion, r.Ciphertext, err = s.cipher.Seal(AAD(r), private)
	if err != nil {
		return View{}, ErrEvidenceUnavailable
	}
	r, err = s.repo.Create(ctx, actor, quote, r, func() error { return s.gate.RequireForBooking(ctx, quote.OwnerID, quote.RenterID, s.now().UTC()) })
	if err != nil {
		return View{}, err
	}
	return s.project(actor, r)
}
func (s *Service) Get(ctx context.Context, actor, id string) (View, error) {
	if !contracttemplate.ValidIDs(actor, id) {
		return View{}, ErrInvalid
	}
	actor = uuid.MustParse(actor).String()
	r, err := s.repo.Get(ctx, actor, id)
	if err != nil {
		return View{}, err
	}
	return s.project(actor, r)
}
func (s *Service) project(actor string, r Record) (View, error) {
	if actor != r.OwnerID && actor != r.RenterID {
		return View{}, ErrNotFound
	}
	parties, err := OpenEvidence(r, s.cipher)
	if err != nil {
		return View{}, ErrEvidenceUnavailable
	}
	owner := rentalflow.ContractIdentity{SubjectID: parties[0].SubjectID, LegalName: parties[0].LegalName, RUT: parties[0].RUT}
	renter := rentalflow.ContractIdentity{SubjectID: parties[1].SubjectID, LegalName: parties[1].LegalName, RUT: parties[1].RUT}
	return View{ID: r.ID, Version: r.Version, SHA256: r.SHA256, Terms: r.Terms, Parties: rentalflow.RedactedContractParties(actor, owner, renter), ContactNotice: rentalflow.ContractContactNotice, GeneratedAt: r.CreatedAt}, nil
}

// OpenEvidence is an internal custody primitive, never an HTTP projection. The
// caller must first authorize the record (e.g. against a verified paid reserve).
// It binds private evidence to this proposal's ID and complete public terms,
// verifies the canonical document digest, and validates both legal identities.
func OpenEvidence(r Record, cipher Cipher) ([]EvidenceParty, error) {
	if cipher == nil {
		return nil, ErrEvidenceUnavailable
	}
	private, err := cipher.Open(r.KeyVersion, AAD(r), r.Ciphertext)
	if err != nil {
		return nil, ErrEvidenceUnavailable
	}
	var parties []EvidenceParty
	if json.Unmarshal(private, &parties) != nil || len(parties) != 2 || Digest(document(r.Terms, parties)) != r.SHA256 || parties[0].Role != "arrendador" || parties[1].Role != "arrendatario" || parties[0].SubjectID != r.OwnerID || parties[1].SubjectID != r.RenterID {
		return nil, ErrEvidenceUnavailable
	}
	for _, p := range parties {
		if _, valid := rentalflow.NormalizeContractIdentity(rentalflow.ContractIdentity{SubjectID: p.SubjectID, LegalName: p.LegalName, RUT: p.RUT}, p.SubjectID); !valid {
			return nil, ErrEvidenceUnavailable
		}
	}
	return parties, nil
}
