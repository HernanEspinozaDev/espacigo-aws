// Package commercialreservationdetail exposes private post-payment documents,
// a verified processor receipt and participant contacts. It never exposes fees
// or makes a tributary document out of the internal processor receipt.
package commercialreservationdetail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const MaxPDFBytes = 16 << 20

var (
	ErrInvalid         = errors.New("reservation detail: invalid request")
	ErrNotFound        = errors.New("reservation detail: not found")
	ErrPaymentRequired = errors.New("reservation detail: paid reservation required")
	ErrUnverified      = errors.New("reservation detail: verified payment required")
	ErrDocumentPending = errors.New("reservation detail: document pending")
	ErrUnavailable     = errors.New("reservation detail: document unavailable")
)

type Contract struct {
	State        string     `json:"state"`
	Version      *int64     `json:"version,omitempty"`
	SHA256       *string    `json:"sha256,omitempty"`
	SizeBytes    *int64     `json:"size_bytes,omitempty"`
	AvailableAt  *time.Time `json:"available_at,omitempty"`
	DownloadPath *string    `json:"download_path,omitempty"`
}
type Receipt struct {
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	AmountCLP         int64     `json:"amount_clp"`
	Currency          string    `json:"currency"`
	Provider          string    `json:"provider"`
	ProviderReference string    `json:"provider_reference"`
	VerifiedAt        time.Time `json:"verified_at"`
}
type Contact struct {
	Role        string  `json:"role"`
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
}
type TaxDocument struct {
	State string `json:"state"`
	Notice string `json:"notice"`
}
type Documents struct {
	ReservationID      string      `json:"reservation_id"`
	ParticipantRole    string      `json:"participant_role"`
	Contract           Contract    `json:"contract"`
	Receipt            Receipt     `json:"receipt"`
	Counterpart        *Contact    `json:"counterpart_contact"`
	TaxDocument        TaxDocument `json:"rental_tax_document"`
	AccessInstructions *string     `json:"access_instructions"`
}

// Record is server-private. Object IDs, buckets, source hashes and references
// are never serialized as part of the user-facing Documents projection.
type Record struct {
	Documents                                                  Documents
	ObjectID, Bucket, PDFSHA256, DocumentSHA256, ObjectVersion string
	Version, Size                                              int64
}
type Repository interface {
	Get(context.Context, string, string) (Record, error)
}
type PrivateStore interface {
	GetWithMetadata(context.Context, string, int64) ([]byte, map[string]string, error)
}
type VersionedPrivateStore interface {
	GetVersionedWithMetadata(context.Context, string, int64, string) ([]byte, map[string]string, string, error)
}
type Service struct {
	repo   Repository
	store  PrivateStore
	bucket string
}

func New(repo Repository, store PrivateStore, bucket string) (*Service, error) {
	if repo == nil || (store != nil && bucket == "") {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, store: store, bucket: bucket}, nil
}
func validID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func (s *Service) Get(ctx context.Context, actor, id string) (Documents, error) {
	if !validID(actor) || !validID(id) {
		return Documents{}, ErrNotFound
	}
	record, err := s.repo.Get(ctx, actor, id)
	if err != nil {
		return Documents{}, err
	}
	view := record.Documents
	if view.ReservationID != id {
		return Documents{}, ErrUnavailable
	}
	if view.Contract.State == "ready" && s.store != nil && record.Bucket == s.bucket {
		path := "/api/v1/commercial-reservations/" + id + "/contract-document"
		view.Contract.DownloadPath = &path
	}
	return view, nil
}
func (s *Service) Download(ctx context.Context, actor, id string) ([]byte, string, error) {
	if !validID(actor) || !validID(id) {
		return nil, "", ErrNotFound
	}
	record, err := s.repo.Get(ctx, actor, id)
	if err != nil {
		return nil, "", err
	}
	if record.Documents.ReservationID != id {
		return nil, "", ErrUnavailable
	}
	if record.Documents.Contract.State != "ready" {
		if record.Documents.Contract.State == "terminal" || record.Documents.Contract.State == "unavailable" {
			return nil, "", ErrUnavailable
		}
		return nil, "", ErrDocumentPending
	}
	if s.store == nil || record.Bucket != s.bucket || !validID(record.ObjectID) || record.Size < 1 || record.Size > MaxPDFBytes {
		return nil, "", ErrUnavailable
	}
	var blob []byte
	var metadata map[string]string
	if record.ObjectVersion != "" {
		versioned, ok := s.store.(VersionedPrivateStore)
		if !ok {
			return nil, "", ErrUnavailable
		}
		var version string
		blob, metadata, version, err = versioned.GetVersionedWithMetadata(ctx, record.ObjectID, MaxPDFBytes, record.ObjectVersion)
		if err == nil && version != record.ObjectVersion {
			return nil, "", ErrUnavailable
		}
	} else {
		blob, metadata, err = s.store.GetWithMetadata(ctx, record.ObjectID, MaxPDFBytes)
	}
	if err != nil {
		return nil, "", ErrUnavailable
	}
	sum := sha256.Sum256(blob)
	digest := hex.EncodeToString(sum[:])
	if int64(len(blob)) != record.Size || digest != record.PDFSHA256 || metadata["sha256"] != digest || metadata["document_sha256"] != record.DocumentSHA256 || metadata["proposal_version"] != strconv.FormatInt(record.Version, 10) || !bytes.HasPrefix(blob, []byte("%PDF-")) {
		return nil, "", ErrUnavailable
	}
	return blob, digest, nil
}
