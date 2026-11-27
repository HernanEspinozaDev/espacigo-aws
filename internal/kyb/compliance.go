package kyb

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ComplianceDocument struct {
	ID                  string       `json:"id"`
	OwnerID             string       `json:"owner_usuario_id"`
	SubmittedBy         string       `json:"submitted_by,omitempty"`
	OrganizationID      string       `json:"organization_id,omitempty"`
	SpaceID             string       `json:"space_id,omitempty"`
	Kind                DocumentKind `json:"document_kind"`
	Status              Status       `json:"status"`
	MIME                string       `json:"mime_type"`
	Size                int64        `json:"size_bytes"`
	SHA256              []byte       `json:"-"`
	StorageRef          string       `json:"-"`
	GiroCode            string       `json:"sii_giro_code,omitempty"`
	CategoryCode        string       `json:"category_code,omitempty"`
	SubmittedAt         time.Time    `json:"submitted_at"`
	RejectionReason     string       `json:"rejection_reason,omitempty"`
	ScopeOrganizationID string       `json:"-"`
}

type ComplianceProfile struct {
	OwnerID        string     `json:"owner_usuario_id"`
	OrganizationID string     `json:"organization_id,omitempty"`
	LegalName      string     `json:"legal_name,omitempty"`
	Status         string     `json:"compliance_status"`
	GiroCode       string     `json:"sii_giro_code,omitempty"`
	SIIStatus      Status     `json:"sii_status,omitempty"`
	GraceStartedAt *time.Time `json:"grace_started_at,omitempty"`
	GraceDueAt     *time.Time `json:"grace_due_at,omitempty"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	DisabledReason string     `json:"disabled_reason,omitempty"`
}

type SpaceEligibility struct {
	CanPublish                 bool       `json:"can_publish"`
	CanBook                    bool       `json:"can_book"`
	SpaceVerified              bool       `json:"space_verified"`
	ComplianceStatus           string     `json:"compliance_status"`
	VerificationStatus         Status     `json:"verification_status,omitempty"`
	GraceDueAt                 *time.Time `json:"grace_due_at,omitempty"`
	PropertyApproved           bool       `json:"property_approved"`
	PropertyStatus             Status     `json:"property_status,omitempty"`
	PropertyRejectionReason    string     `json:"property_rejection_reason,omitempty"`
	CompanyVerified            bool       `json:"company_verified"`
	RequiresCategorySII        bool       `json:"requires_category_sii"`
	CategorySIIApproved        bool       `json:"category_sii_approved"`
	CategorySIIStatus          Status     `json:"category_sii_status,omitempty"`
	CategorySIIRejectionReason string     `json:"category_sii_rejection_reason,omitempty"`
	CategoryCode               string     `json:"category_code,omitempty"`
	CategoryName               string     `json:"category_name,omitempty"`
}

// ComplianceRepository is implemented by the PostgreSQL KYB adapter. Keeping
// it optional preserves compatibility with callers and tests that only use KYB.
type ComplianceRepository interface {
	SubmitPersonalSII(context.Context, ComplianceDocument) (ComplianceDocument, error)
	PersonalCompliance(context.Context, string) (ComplianceProfile, error)
	SubmitSpaceProperty(context.Context, ComplianceDocument) (ComplianceDocument, error)
	SubmitSpaceCategorySII(context.Context, ComplianceDocument) (ComplianceDocument, error)
	SpaceStorageScope(context.Context, string, string) (ownerID, organizationID string, err error)
	SpaceEligibility(context.Context, string, string, time.Time) (SpaceEligibility, error)
	ListCompliance(context.Context, string) ([]ComplianceProfile, error)
	SetComplianceDisabled(context.Context, string, string, string, time.Time) error
	SetOrganizationDisabled(context.Context, string, string, string, time.Time) error
	ListPendingComplianceDocuments(context.Context) ([]ComplianceDocument, error)
	GetComplianceDocument(context.Context, string) (ComplianceDocument, error)
	GetComplianceDocumentByStorageRef(context.Context, string) (ComplianceDocument, error)
	StartComplianceReview(context.Context, string, string, time.Time) (ComplianceDocument, error)
	DecideComplianceDocument(context.Context, string, string, Status, string, time.Time) (ComplianceDocument, error)
	ExpireNoncompliant(context.Context, time.Time) (int, error)
}

type OrganizationVerificationReader interface {
	GetOwnForOrganization(context.Context, string, string) (Verification, error)
}

func (s *Service) OwnForOrganization(ctx context.Context, ownerID, organizationID string) (Verification, error) {
	repo, ok := s.repository.(OrganizationVerificationReader)
	if !ok || !canonicalUUID(ownerID) || !canonicalUUID(organizationID) {
		return Verification{}, ErrInvalid
	}
	return repo.GetOwnForOrganization(ctx, ownerID, organizationID)
}

func (s *Service) SubmitPersonalSII(ctx context.Context, ownerID, giro string, upload Upload) (ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) || !giroCodePattern.MatchString(giro) || upload.Kind != DocumentSIIStartActivities || !validUpload(upload) {
		return ComplianceDocument{}, ErrInvalid
	}
	id, err := s.ids.ID()
	if err != nil || !canonicalUUID(id) {
		return ComplianceDocument{}, ErrInvalid
	}
	ref, err := s.ids.ID()
	if err != nil || !canonicalUUID(ref) {
		return ComplianceDocument{}, ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	item := ComplianceDocument{ID: id, OwnerID: ownerID, Kind: upload.Kind, Status: StatusPending, MIME: upload.MIME, Size: int64(len(upload.Content)), SHA256: sha256Bytes(upload.Content), StorageRef: ref, GiroCode: giro, SubmittedAt: now}
	if err = s.documents.Put(ctx, ownerID, ownerID, id, ref, upload.Content); err != nil {
		return ComplianceDocument{}, err
	}
	created, err := repo.SubmitPersonalSII(ctx, item)
	if err != nil {
		_ = s.documents.Delete(ctx, ownerID, ownerID, id, ref)
		return ComplianceDocument{}, err
	}
	return created, nil
}

func (s *Service) PersonalCompliance(ctx context.Context, ownerID string) (ComplianceProfile, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) {
		return ComplianceProfile{}, ErrInvalid
	}
	return repo.PersonalCompliance(ctx, ownerID)
}

func (s *Service) SubmitSpaceProperty(ctx context.Context, ownerID, spaceID string, upload Upload) (ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) || !canonicalUUID(spaceID) || upload.Kind != DocumentPropertyOrSublease || !validUpload(upload) {
		return ComplianceDocument{}, ErrInvalid
	}
	id, err := s.ids.ID()
	if err != nil || !canonicalUUID(id) {
		return ComplianceDocument{}, ErrInvalid
	}
	ref, err := s.ids.ID()
	if err != nil || !canonicalUUID(ref) {
		return ComplianceDocument{}, ErrInvalid
	}
	spaceOwner, organizationID, err := repo.SpaceStorageScope(ctx, ownerID, spaceID)
	if err != nil {
		return ComplianceDocument{}, err
	}
	storageOrganizationID := organizationID
	if storageOrganizationID == "" {
		storageOrganizationID = spaceOwner
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	digest := sha256.Sum256(upload.Content)
	item := ComplianceDocument{ID: id, OwnerID: spaceOwner, SubmittedBy: ownerID, OrganizationID: organizationID, SpaceID: spaceID, Kind: upload.Kind, Status: StatusPending, MIME: upload.MIME, Size: int64(len(upload.Content)), SHA256: digest[:], StorageRef: ref, SubmittedAt: now}
	if err = s.documents.Put(ctx, spaceOwner, storageOrganizationID, id, ref, upload.Content); err != nil {
		return ComplianceDocument{}, err
	}
	created, err := repo.SubmitSpaceProperty(ctx, item)
	if err != nil {
		_ = s.documents.Delete(ctx, spaceOwner, storageOrganizationID, id, ref)
		return ComplianceDocument{}, err
	}
	return created, nil
}

func (s *Service) SubmitSpaceCategorySII(ctx context.Context, ownerID, spaceID, giro string, upload Upload) (ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) || !canonicalUUID(spaceID) || !giroCodePattern.MatchString(giro) || upload.Kind != DocumentSIIStartActivities || !validUpload(upload) {
		return ComplianceDocument{}, ErrInvalid
	}
	id, err := s.ids.ID()
	if err != nil || !canonicalUUID(id) {
		return ComplianceDocument{}, ErrInvalid
	}
	ref, err := s.ids.ID()
	if err != nil || !canonicalUUID(ref) {
		return ComplianceDocument{}, ErrInvalid
	}
	spaceOwner, organizationID, err := repo.SpaceStorageScope(ctx, ownerID, spaceID)
	if err != nil {
		return ComplianceDocument{}, err
	}
	if organizationID == "" {
		return ComplianceDocument{}, ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	digest := sha256.Sum256(upload.Content)
	item := ComplianceDocument{
		ID: id, OwnerID: spaceOwner, SubmittedBy: ownerID, OrganizationID: organizationID,
		SpaceID: spaceID, Kind: upload.Kind, Status: StatusPending, MIME: upload.MIME,
		Size: int64(len(upload.Content)), SHA256: digest[:], StorageRef: ref,
		GiroCode: giro, SubmittedAt: now,
	}
	if err = s.documents.Put(ctx, spaceOwner, organizationID, id, ref, upload.Content); err != nil {
		return ComplianceDocument{}, err
	}
	created, err := repo.SubmitSpaceCategorySII(ctx, item)
	if err != nil {
		_ = s.documents.Delete(ctx, spaceOwner, organizationID, id, ref)
		return ComplianceDocument{}, err
	}
	return created, nil
}

func (s *Service) SpaceEligibility(ctx context.Context, ownerID, spaceID string) (SpaceEligibility, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) || !canonicalUUID(spaceID) {
		return SpaceEligibility{}, ErrInvalid
	}
	return repo.SpaceEligibility(ctx, ownerID, spaceID, s.now().UTC())
}

func (s *Service) ComplianceProfiles(ctx context.Context, filter string) ([]ComplianceProfile, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok {
		return nil, ErrInvalid
	}
	filter = strings.TrimSpace(filter)
	if filter != "" && filter != "grace_active" && filter != "in_review" && filter != "compliant" && filter != "disabled_missing_docs" {
		return nil, ErrInvalid
	}
	return repo.ListCompliance(ctx, filter)
}

func (s *Service) SetComplianceDisabled(ctx context.Context, ownerID, actorID, reason string, disabled bool) error {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(ownerID) || !canonicalUUID(actorID) {
		return ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	if disabled && (len([]rune(reason)) < 5 || len([]rune(reason)) > 500) {
		return ErrInvalid
	}
	if !disabled {
		reason = ""
	}
	return repo.SetComplianceDisabled(ctx, ownerID, actorID, reason, s.now().UTC())
}

func (s *Service) SetOrganizationDisabled(ctx context.Context, organizationID, actorID, reason string, disabled bool) error {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(organizationID) || !canonicalUUID(actorID) {
		return ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	if disabled && (len([]rune(reason)) < 5 || len([]rune(reason)) > 500) {
		return ErrInvalid
	}
	if !disabled {
		reason = ""
	}
	return repo.SetOrganizationDisabled(ctx, organizationID, actorID, reason, s.now().UTC())
}

func (s *Service) PendingComplianceDocuments(ctx context.Context) ([]ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok {
		return nil, ErrInvalid
	}
	return repo.ListPendingComplianceDocuments(ctx)
}
func (s *Service) StartComplianceReview(ctx context.Context, id, reviewer string) (ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(id) || !canonicalUUID(reviewer) {
		return ComplianceDocument{}, ErrInvalid
	}
	return repo.StartComplianceReview(ctx, id, reviewer, s.now().UTC())
}
func (s *Service) DecideComplianceDocument(ctx context.Context, id, reviewer string, approved bool, reason string) (ComplianceDocument, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(id) || !canonicalUUID(reviewer) {
		return ComplianceDocument{}, ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	status := StatusApproved
	if !approved {
		status = StatusRejected
		if len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
			return ComplianceDocument{}, ErrInvalid
		}
	} else if reason != "" {
		return ComplianceDocument{}, ErrInvalid
	}
	return repo.DecideComplianceDocument(ctx, id, reviewer, status, reason, s.now().UTC())
}
func (s *Service) IssueComplianceReviewURL(ctx context.Context, id string, now time.Time) (string, time.Time, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || !canonicalUUID(id) || now.IsZero() {
		return "", time.Time{}, ErrInvalid
	}
	document, err := repo.GetComplianceDocument(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := now.UTC().Add(ReviewLinkTTL).Truncate(time.Second)
	version, signature, err := s.documents.SignReviewAccess(document.StorageRef, expires.Unix())
	if err != nil {
		return "", time.Time{}, err
	}
	return fmt.Sprintf("/api/v1/kyb/compliance-documents/%s?expires=%d&key_version=%s&signature=%s", document.StorageRef, expires.Unix(), version, signature), expires, nil
}
func (s *Service) ReadSignedComplianceDocument(ctx context.Context, ref, version, signature string, expires int64, now time.Time) ([]byte, string, error) {
	if !canonicalUUID(ref) || now.IsZero() || expires <= now.Unix() || expires > now.Add(ReviewLinkTTL).Unix() || !s.documents.VerifyReviewAccess(ref, expires, version, signature, now) {
		return nil, "", ErrForbidden
	}
	repo, ok := s.repository.(ComplianceRepository)
	if !ok {
		return nil, "", ErrInvalid
	}
	document, err := repo.GetComplianceDocumentByStorageRef(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	content, err := s.documents.Read(ctx, document.OwnerID, document.ScopeOrganizationID, document.ID, ref)
	if err != nil || int64(len(content)) != document.Size || !equalBytes(sha256Bytes(content), document.SHA256) {
		return nil, "", errors.New("kyb: private document unavailable")
	}
	return content, document.MIME, nil
}

func (s *Service) ExpireNoncompliant(ctx context.Context, now time.Time) (int, error) {
	repo, ok := s.repository.(ComplianceRepository)
	if !ok || now.IsZero() {
		return 0, ErrInvalid
	}
	return repo.ExpireNoncompliant(ctx, now.UTC())
}

func validUpload(upload Upload) bool {
	return len(upload.Content) > 0 && len(upload.Content) <= MaxDocumentBytes && validDocumentMIME(upload.MIME) && http.DetectContentType(upload.Content) == upload.MIME
}
