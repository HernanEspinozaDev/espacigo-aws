package kyb

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
)

var (
	ErrInvalid   = errors.New("kyb: invalid request")
	ErrNotFound  = errors.New("kyb: verification not found")
	ErrConflict  = errors.New("kyb: verification conflict")
	ErrForbidden = errors.New("kyb: forbidden")
)

type Status string

const (
	StatusPending  Status = "PENDING"
	StatusInReview Status = "IN_REVIEW"
	StatusApproved Status = "APPROVED"
	StatusRejected Status = "REJECTED"
)

type DocumentKind string

const (
	DocumentERUT               DocumentKind = "erut"
	DocumentPowerCertificate   DocumentKind = "power_certificate"
	DocumentPropertyOrSublease DocumentKind = "property_or_sublease_authorization"
)

const MaxDocumentBytes = 10 << 20
const ReviewLinkTTL = 5 * time.Minute

var giroCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

type IDGenerator interface {
	ID() (string, error)
}

type DeclarationKeyring interface {
	Seal([]byte, []byte) (string, []byte, error)
	Open(string, []byte, []byte) ([]byte, error)
}

type RutFingerprinter interface {
	Fingerprints(string) (verification.RutFingerprintSet, error)
}

type DocumentStore interface {
	Put(context.Context, string, string, string, string, []byte) error
	Read(context.Context, string, string, string, string) ([]byte, error)
	Delete(context.Context, string, string, string, string) error
	SignReviewAccess(string, int64) (string, string, error)
	VerifyReviewAccess(string, int64, string, string, time.Time) bool
}

type Repository interface {
	GetOrCreateOrganization(context.Context, string, string) (string, error)
	Create(context.Context, Verification, []Document) (Verification, bool, error)
	GetOwn(context.Context, string) (Verification, error)
	ListPending(context.Context) ([]Verification, error)
	GetForReview(context.Context, string) (Verification, error)
	GetDocument(context.Context, string, string) (Document, Verification, error)
	GetDocumentByStorageRef(context.Context, string) (Document, Verification, error)
	StartReview(context.Context, string, string, time.Time) (Verification, error)
	Decide(context.Context, string, string, Status, string, time.Time) (Verification, error)
}

type Upload struct {
	Kind    DocumentKind
	MIME    string
	Content []byte
}

type SubmissionInput struct {
	LegalName          string
	CompanyRUT         string
	RepresentativeName string
	RepresentativeRUT  string
	SIIGiroCode        string
	IdempotencyKey     string
	Documents          []Upload
}

type Document struct {
	ID             string       `json:"id"`
	VerificationID string       `json:"verification_id,omitempty"`
	Kind           DocumentKind `json:"kind"`
	MIME           string       `json:"mime_type"`
	Size           int64        `json:"size_bytes"`
	SHA256         []byte       `json:"-"`
	StorageRef     string       `json:"-"`
}

type Verification struct {
	ID                       string     `json:"id"`
	OrganizationID           string     `json:"organization_id"`
	OwnerID                  string     `json:"-"`
	SubmittedBy              string     `json:"-"`
	LegalName                string     `json:"legal_name"`
	CompanyRUT               string     `json:"company_rut,omitempty"`
	CompanyRUTFingerprint    string     `json:"-"`
	CompanyRUTFingerprints   []string   `json:"-"`
	CompanyRUTKeyVersion     string     `json:"-"`
	CompanyRUTCiphertext     []byte     `json:"-"`
	RepresentativeName       string     `json:"representative_name"`
	RepresentativeRUT        string     `json:"representative_rut,omitempty"`
	RepresentativeRUTFprint  string     `json:"-"`
	RepresentativeRUTFprints []string   `json:"-"`
	RepresentativeRUTKey     string     `json:"-"`
	RepresentativeRUTCipher  []byte     `json:"-"`
	SIIGiroCode              string     `json:"sii_giro_code"`
	Status                   Status     `json:"status"`
	RejectionReason          string     `json:"rejection_reason,omitempty"`
	SubmittedAt              time.Time  `json:"submitted_at"`
	ReviewedAt               *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID               string     `json:"-"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	IdempotencyKey           string     `json:"-"`
	Documents                []Document `json:"documents,omitempty"`
}

type SignedDocument struct {
	Document Document
	OwnerID  string
	OrgID    string
	CaseID   string
}

type Service struct {
	repository Repository
	documents  DocumentStore
	keys       DeclarationKeyring
	rutKeys    RutFingerprinter
	ids        IDGenerator
	now        func() time.Time
}

func NewService(repository Repository, documents DocumentStore, keys DeclarationKeyring, rutKeys RutFingerprinter, ids IDGenerator, now func() time.Time) (*Service, error) {
	if repository == nil || documents == nil || keys == nil || rutKeys == nil || ids == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Service{repository: repository, documents: documents, keys: keys, rutKeys: rutKeys, ids: ids, now: now}, nil
}

func (s *Service) Submit(ctx context.Context, ownerID string, input SubmissionInput) (Verification, error) {
	if s == nil || s.repository == nil || ctx == nil || !canonicalUUID(ownerID) || validateSubmission(input) != nil {
		return Verification{}, ErrInvalid
	}
	companyRUT, err := NormalizeChileanRUT(input.CompanyRUT)
	if err != nil {
		return Verification{}, ErrInvalid
	}
	representativeRUT, err := NormalizeChileanRUT(input.RepresentativeRUT)
	if err != nil {
		return Verification{}, ErrInvalid
	}
	companyFingerprints, err := s.rutKeys.Fingerprints(companyRUT)
	if err != nil || verification.ValidateRutFingerprintSet(companyFingerprints) != nil {
		return Verification{}, ErrInvalid
	}
	representativeFingerprints, err := s.rutKeys.Fingerprints(representativeRUT)
	if err != nil || verification.ValidateRutFingerprintSet(representativeFingerprints) != nil {
		return Verification{}, ErrInvalid
	}
	organizationCandidate, err := s.ids.ID()
	if err != nil || !canonicalUUID(organizationCandidate) {
		return Verification{}, ErrInvalid
	}
	organizationID, err := s.repository.GetOrCreateOrganization(ctx, ownerID, organizationCandidate)
	if err != nil || !canonicalUUID(organizationID) {
		return Verification{}, errOrInvalid(err)
	}
	verificationID, err := s.ids.ID()
	if err != nil || !canonicalUUID(verificationID) {
		return Verification{}, ErrInvalid
	}
	companyKeyVersion, companyCiphertext, err := s.sealRUT(ownerID, organizationID, verificationID, "company", companyRUT)
	if err != nil {
		return Verification{}, err
	}
	representativeKeyVersion, representativeCiphertext, err := s.sealRUT(ownerID, organizationID, verificationID, "representative", representativeRUT)
	if err != nil {
		return Verification{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return Verification{}, ErrInvalid
	}
	item := Verification{
		ID: verificationID, OrganizationID: organizationID, OwnerID: ownerID, SubmittedBy: ownerID,
		LegalName: strings.TrimSpace(input.LegalName), CompanyRUTFingerprint: companyFingerprints.Active,
		CompanyRUTFingerprints: append([]string(nil), companyFingerprints.Candidates...),
		CompanyRUTKeyVersion:   companyKeyVersion, CompanyRUTCiphertext: companyCiphertext,
		RepresentativeName:       strings.TrimSpace(input.RepresentativeName),
		RepresentativeRUTFprint:  representativeFingerprints.Active,
		RepresentativeRUTFprints: append([]string(nil), representativeFingerprints.Candidates...),
		RepresentativeRUTKey:     representativeKeyVersion, RepresentativeRUTCipher: representativeCiphertext,
		SIIGiroCode: input.SIIGiroCode, Status: StatusPending, SubmittedAt: now,
		CreatedAt: now, UpdatedAt: now, IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		Documents: make([]Document, 0, len(input.Documents)),
	}
	storedRefs := make([]Document, 0, len(input.Documents))
	for _, upload := range input.Documents {
		documentID, idErr := s.ids.ID()
		storageRef, refErr := s.ids.ID()
		if idErr != nil || refErr != nil || !canonicalUUID(documentID) || !canonicalUUID(storageRef) {
			s.deleteDocuments(ctx, ownerID, organizationID, verificationID, storedRefs)
			return Verification{}, ErrInvalid
		}
		if err = s.documents.Put(ctx, ownerID, organizationID, verificationID, storageRef, upload.Content); err != nil {
			s.deleteDocuments(ctx, ownerID, organizationID, verificationID, storedRefs)
			return Verification{}, err
		}
		document := Document{
			ID: documentID, VerificationID: verificationID, Kind: upload.Kind, MIME: upload.MIME,
			Size: int64(len(upload.Content)), SHA256: sha256Bytes(upload.Content), StorageRef: storageRef,
		}
		storedRefs = append(storedRefs, document)
		item.Documents = append(item.Documents, document)
	}
	created, reused, err := s.repository.Create(ctx, item, item.Documents)
	if err != nil || reused {
		s.deleteDocuments(ctx, ownerID, organizationID, verificationID, storedRefs)
	}
	if err != nil {
		return Verification{}, err
	}
	if reused {
		return created, nil
	}
	return created, nil
}

func (s *Service) Own(ctx context.Context, ownerID string) (Verification, error) {
	if !canonicalUUID(ownerID) {
		return Verification{}, ErrNotFound
	}
	return s.repository.GetOwn(ctx, ownerID)
}

func (s *Service) Pending(ctx context.Context) ([]Verification, error) {
	return s.repository.ListPending(ctx)
}

func (s *Service) ForReview(ctx context.Context, id string) (Verification, error) {
	if !canonicalUUID(id) {
		return Verification{}, ErrNotFound
	}
	item, err := s.repository.GetForReview(ctx, id)
	if err != nil {
		return Verification{}, err
	}
	companyRUT, err := s.openRUT(item, "company", item.CompanyRUTKeyVersion, item.CompanyRUTCiphertext)
	if err != nil {
		return Verification{}, errors.New("kyb: encrypted review data unavailable")
	}
	representativeRUT, err := s.openRUT(item, "representative", item.RepresentativeRUTKey, item.RepresentativeRUTCipher)
	if err != nil {
		return Verification{}, errors.New("kyb: encrypted review data unavailable")
	}
	item.CompanyRUT = companyRUT
	item.RepresentativeRUT = representativeRUT
	return item, nil
}

func (s *Service) StartReview(ctx context.Context, id, reviewer string) (Verification, error) {
	if !canonicalUUID(id) || !canonicalUUID(reviewer) {
		return Verification{}, ErrInvalid
	}
	return s.repository.StartReview(ctx, id, reviewer, s.now().UTC())
}

func (s *Service) Decide(ctx context.Context, id, reviewer string, approved bool, reason string) (Verification, error) {
	if !canonicalUUID(id) || !canonicalUUID(reviewer) {
		return Verification{}, ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	status := StatusApproved
	if !approved {
		status = StatusRejected
		if len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
			return Verification{}, ErrInvalid
		}
	} else if reason != "" {
		return Verification{}, ErrInvalid
	}
	return s.repository.Decide(ctx, id, reviewer, status, reason, s.now().UTC())
}

func (s *Service) IssueReviewURL(ctx context.Context, verificationID, documentID string, now time.Time) (string, time.Time, error) {
	if !canonicalUUID(verificationID) || !canonicalUUID(documentID) || now.IsZero() {
		return "", time.Time{}, ErrInvalid
	}
	document, _, err := s.repository.GetDocument(ctx, verificationID, documentID)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.UTC().Add(ReviewLinkTTL).Truncate(time.Second)
	keyVersion, signature, err := s.documents.SignReviewAccess(document.StorageRef, expiresAt.Unix())
	if err != nil {
		return "", time.Time{}, err
	}
	url := fmt.Sprintf("/api/v1/kyb/documents/%s?expires=%d&key_version=%s&signature=%s", document.StorageRef, expiresAt.Unix(), keyVersion, signature)
	return url, expiresAt, nil
}

func (s *Service) ReadSignedDocument(ctx context.Context, storageRef, keyVersion, signature string, expires int64, now time.Time) ([]byte, string, error) {
	if !canonicalUUID(storageRef) || now.IsZero() || expires <= now.Unix() || expires > now.Add(ReviewLinkTTL).Unix() ||
		!s.documents.VerifyReviewAccess(storageRef, expires, keyVersion, signature, now) {
		return nil, "", ErrForbidden
	}
	document, verification, err := s.repository.GetDocumentByStorageRef(ctx, storageRef)
	if err != nil {
		return nil, "", err
	}
	content, err := s.documents.Read(ctx, verification.OwnerID, verification.OrganizationID, verification.ID, storageRef)
	if err != nil || int64(len(content)) != document.Size || !equalBytes(sha256Bytes(content), document.SHA256) {
		return nil, "", errors.New("kyb: private document unavailable")
	}
	return content, document.MIME, nil
}

func (s *Service) sealRUT(ownerID, organizationID, verificationID, subject, rut string) (string, []byte, error) {
	const purpose = "espacigo:kyb-rut:v1"
	aad := []byte(strings.Join([]string{purpose, ownerID, organizationID, verificationID, subject}, ":"))
	return s.keys.Seal(aad, []byte(rut))
}

func (s *Service) openRUT(item Verification, subject, version string, ciphertext []byte) (string, error) {
	const purpose = "espacigo:kyb-rut:v1"
	aad := []byte(strings.Join([]string{purpose, item.OwnerID, item.OrganizationID, item.ID, subject}, ":"))
	plaintext, err := s.keys.Open(version, aad, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func validateSubmission(input SubmissionInput) error {
	if len([]rune(strings.TrimSpace(input.LegalName))) < 2 || len([]rune(input.LegalName)) > 200 ||
		len([]rune(strings.TrimSpace(input.RepresentativeName))) < 2 || len([]rune(input.RepresentativeName)) > 200 ||
		!giroCodePattern.MatchString(input.SIIGiroCode) || strings.TrimSpace(input.IdempotencyKey) == "" ||
		len(input.IdempotencyKey) > 120 || len(input.Documents) != 3 {
		return ErrInvalid
	}
	seen := make(map[DocumentKind]bool, len(input.Documents))
	for _, document := range input.Documents {
		if seen[document.Kind] || !validDocumentMIME(document.MIME) || len(document.Content) == 0 || len(document.Content) > MaxDocumentBytes {
			return ErrInvalid
		}
		if http.DetectContentType(document.Content) != document.MIME {
			return ErrInvalid
		}
		seen[document.Kind] = true
	}
	if !seen[DocumentERUT] || !seen[DocumentPowerCertificate] || !seen[DocumentPropertyOrSublease] {
		return ErrInvalid
	}
	return nil
}

func validDocumentMIME(value string) bool {
	return value == "application/pdf" || value == "image/jpeg" || value == "image/png"
}

func canonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
		} else if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func ValidUUID(value string) bool { return canonicalUUID(value) }

func errOrInvalid(err error) error {
	if err == nil {
		return ErrInvalid
	}
	return err
}

func sha256Bytes(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func (s *Service) deleteDocuments(ctx context.Context, ownerID, organizationID, verificationID string, documents []Document) {
	for _, document := range documents {
		_ = s.documents.Delete(ctx, ownerID, organizationID, verificationID, document.StorageRef)
	}
}
