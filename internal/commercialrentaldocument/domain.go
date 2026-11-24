// Package commercialrentaldocument stores private rental tax documents. It
// records an uploaded file without issuing, classifying, or validating a DTE.
package commercialrentaldocument

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

const MaxPDFBytes = 10 << 20

var (
	ErrInvalid               = errors.New("commercial rental document: invalid input")
	ErrNotFound              = errors.New("commercial rental document: not found")
	ErrConflict              = errors.New("commercial rental document: already attached")
	ErrInspectionUnavailable = errors.New("commercial rental document: inspection unavailable")
	ErrUnavailable           = errors.New("commercial rental document: unavailable")
)

type Validator interface {
	Validate(context.Context, []byte) error
}

type PrivateStore interface {
	PutWithMetadata(context.Context, string, []byte, map[string]string) error
	GetWithMetadata(context.Context, string, int64) ([]byte, map[string]string, error)
}

type Document struct {
	ID, ReservationID, HostID, ObjectID string
	Bucket, SHA256                      string
	Size                                int64
	InspectedAt, UploadedAt             time.Time
}

type Repository interface {
	AuthorizeUpload(context.Context, string, string) error
	Create(context.Context, string, Document) error
	Get(context.Context, string, string) (Document, error)
}

type Service struct {
	repo      Repository
	store     PrivateStore
	validator Validator
	bucket    string
	now       func() time.Time
}

func New(repo Repository, store PrivateStore, validator Validator, bucket string, now func() time.Time) (*Service, error) {
	if repo == nil || (store != nil && bucket == "") {
		return nil, ErrInvalid
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, store: store, validator: validator, bucket: bucket, now: now}, nil
}

func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

func (s *Service) AuthorizeUpload(ctx context.Context, actor, reservationID string) error {
	if s == nil || !validID(actor) || !validID(reservationID) {
		return ErrNotFound
	}
	return s.repo.AuthorizeUpload(ctx, actor, reservationID)
}

func (s *Service) Upload(ctx context.Context, actor, reservationID string, raw []byte) (Document, error) {
	if s == nil || !validID(actor) || !validID(reservationID) || len(raw) == 0 || len(raw) > MaxPDFBytes || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		return Document{}, ErrInvalid
	}
	if err := s.AuthorizeUpload(ctx, actor, reservationID); err != nil {
		return Document{}, err
	}
	if s.validator == nil || s.store == nil {
		return Document{}, ErrInspectionUnavailable
	}
	digest := sha256.Sum256(raw)
	digestHex := hex.EncodeToString(digest[:])
	if existing, err := s.repo.Get(ctx, actor, reservationID); err == nil {
		if existing.SHA256 != digestHex {
			return Document{}, ErrConflict
		}
		blob, verified, downloadErr := s.Download(ctx, actor, reservationID)
		if downloadErr != nil || !bytes.Equal(blob, raw) {
			return Document{}, ErrUnavailable
		}
		return verified, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Document{}, err
	}
	if err := s.validator.Validate(ctx, raw); err != nil {
		if errors.Is(err, ErrInvalid) {
			return Document{}, ErrInvalid
		}
		return Document{}, ErrInspectionUnavailable
	}
	doc := Document{
		ID: reservationID, ReservationID: reservationID, HostID: actor,
		ObjectID: reservationID, Bucket: s.bucket, SHA256: digestHex,
		Size: int64(len(raw)), InspectedAt: s.now().UTC(), UploadedAt: s.now().UTC(),
	}
	metadata := map[string]string{
		"sha256":         doc.SHA256,
		"reservation_id": reservationID,
		"document_id":    doc.ID,
	}
	if err := s.store.PutWithMetadata(ctx, doc.ObjectID, raw, metadata); err != nil {
		// A retry can recover an object written before a process interruption.
		stored, storedMetadata, getErr := s.store.GetWithMetadata(ctx, doc.ObjectID, MaxPDFBytes)
		if getErr != nil || !bytes.Equal(stored, raw) || storedMetadata["sha256"] != digestHex || storedMetadata["reservation_id"] != reservationID || storedMetadata["document_id"] != doc.ID {
			if existing, readErr := s.repo.Get(ctx, actor, reservationID); readErr == nil && existing.SHA256 == digestHex {
				if blob, verified, verifyErr := s.Download(ctx, actor, reservationID); verifyErr == nil && bytes.Equal(blob, raw) {
					return verified, nil
				}
			}
			if getErr == nil {
				return Document{}, ErrConflict
			}
			return Document{}, ErrUnavailable
		}
	}
	if err := s.repo.Create(ctx, actor, doc); err != nil {
		if existing, readErr := s.repo.Get(ctx, actor, reservationID); readErr == nil {
			if existing.SHA256 == digestHex {
				if blob, verified, verifyErr := s.Download(ctx, actor, reservationID); verifyErr == nil && bytes.Equal(blob, raw) {
					return verified, nil
				}
			}
			return Document{}, ErrConflict
		}
		return Document{}, err
	}
	return doc, nil
}

func (s *Service) Download(ctx context.Context, actor, reservationID string) ([]byte, Document, error) {
	if s == nil || !validID(actor) || !validID(reservationID) {
		return nil, Document{}, ErrNotFound
	}
	doc, err := s.repo.Get(ctx, actor, reservationID)
	if err != nil {
		return nil, Document{}, err
	}
	if s.store == nil || doc.Bucket != s.bucket || !validID(doc.ObjectID) || doc.Size < 1 || doc.Size > MaxPDFBytes || doc.SHA256 == "" || doc.InspectedAt.IsZero() {
		return nil, Document{}, ErrUnavailable
	}
	blob, metadata, err := s.store.GetWithMetadata(ctx, doc.ObjectID, MaxPDFBytes)
	if err != nil {
		return nil, Document{}, ErrUnavailable
	}
	digest := sha256.Sum256(blob)
	if int64(len(blob)) != doc.Size || hex.EncodeToString(digest[:]) != doc.SHA256 || metadata["sha256"] != doc.SHA256 || metadata["reservation_id"] != reservationID || metadata["document_id"] != doc.ID || !bytes.HasPrefix(blob, []byte("%PDF-")) {
		return nil, Document{}, ErrUnavailable
	}
	return blob, doc, nil
}
