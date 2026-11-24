package commercialrentaldocument

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

type validatorFake struct {
	calls int
	err   error
}

func (v *validatorFake) Validate(context.Context, []byte) error { v.calls++; return v.err }

type storeFake struct {
	blobs    map[string][]byte
	metadata map[string]map[string]string
	puts     int
}

func newStoreFake() *storeFake {
	return &storeFake{blobs: map[string][]byte{}, metadata: map[string]map[string]string{}}
}
func (s *storeFake) PutWithMetadata(_ context.Context, id string, blob []byte, metadata map[string]string) error {
	s.puts++
	if _, exists := s.blobs[id]; exists {
		return errors.New("already exists")
	}
	s.blobs[id] = append([]byte(nil), blob...)
	s.metadata[id] = metadata
	return nil
}
func (s *storeFake) GetWithMetadata(_ context.Context, id string, _ int64) ([]byte, map[string]string, error) {
	blob, ok := s.blobs[id]
	if !ok {
		return nil, nil, errors.New("not found")
	}
	return append([]byte(nil), blob...), s.metadata[id], nil
}

type repoFake struct {
	host, renter string
	paid         bool
	docs         map[string]Document
}

func (r *repoFake) AuthorizeUpload(_ context.Context, actor, reservation string) error {
	if actor != r.host || !r.paid || reservation == "" {
		return ErrNotFound
	}
	return nil
}
func (r *repoFake) Create(_ context.Context, actor string, doc Document) error {
	if actor != r.host || !r.paid {
		return ErrNotFound
	}
	if _, ok := r.docs[doc.ReservationID]; ok {
		return ErrConflict
	}
	r.docs[doc.ReservationID] = doc
	return nil
}
func (r *repoFake) Get(_ context.Context, actor, reservation string) (Document, error) {
	doc, ok := r.docs[reservation]
	if !ok || actor != r.host && actor != r.renter || !r.paid {
		return Document{}, ErrNotFound
	}
	return doc, nil
}

func fixture(t *testing.T) (*Service, *repoFake, *storeFake, *validatorFake, string, string, string) {
	t.Helper()
	host, renter, reservation := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := &repoFake{host: host, renter: renter, paid: true, docs: map[string]Document{}}
	store := newStoreFake()
	validator := &validatorFake{}
	service, err := New(repo, store, validator, "private-rental", func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, store, validator, host, renter, reservation
}

func TestUploadAuthorizesOwnerBeforeInspectionAndStorage(t *testing.T) {
	service, _, store, validator, host, renter, reservation := fixture(t)
	if _, err := service.Upload(context.Background(), renter, reservation, []byte("%PDF-1.7 synthetic")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("buyer upload error=%v", err)
	}
	if validator.calls != 0 || store.puts != 0 {
		t.Fatalf("unauthorized upload touched inspector/store: inspections=%d writes=%d", validator.calls, store.puts)
	}
	if _, err := service.Upload(context.Background(), host, reservation, []byte("%PDF-1.7 synthetic")); err != nil {
		t.Fatal(err)
	}
	if validator.calls != 1 || store.puts != 1 {
		t.Fatalf("authorized upload calls: inspections=%d writes=%d", validator.calls, store.puts)
	}
}

func TestUploadReplayIsIdempotentAndDifferentFileConflicts(t *testing.T) {
	service, _, store, validator, host, _, reservation := fixture(t)
	raw := []byte("%PDF-1.7 same signed document")
	first, err := service.Upload(context.Background(), host, reservation, raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Upload(context.Background(), host, reservation, raw)
	if err != nil || first.SHA256 != second.SHA256 || store.puts != 1 || validator.calls != 1 {
		t.Fatalf("replay result=%+v err=%v writes=%d inspections=%d", second, err, store.puts, validator.calls)
	}
	if _, err := service.Upload(context.Background(), host, reservation, []byte("%PDF-1.7 changed")); !errors.Is(err, ErrConflict) {
		t.Fatalf("different upload error=%v, want conflict", err)
	}
}

func TestUploadFailsClosedAndDownloadChecksStoredDigest(t *testing.T) {
	service, repo, store, validator, host, renter, reservation := fixture(t)
	validator.err = fmt.Errorf("clamav offline")
	if _, err := service.Upload(context.Background(), host, reservation, []byte("%PDF-1.7 signed")); !errors.Is(err, ErrInspectionUnavailable) {
		t.Fatalf("inspection error=%v", err)
	}
	if store.puts != 0 || len(repo.docs) != 0 {
		t.Fatal("inspection failure persisted document")
	}
	validator.err = nil
	raw := []byte("%PDF-1.7 signed")
	if _, err := service.Upload(context.Background(), host, reservation, raw); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Download(context.Background(), renter, reservation); err != nil {
		t.Fatalf("renter download: %v", err)
	}
	store.blobs[reservation] = []byte("%PDF-1.7 tampered")
	if _, _, err := service.Download(context.Background(), renter, reservation); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("tampered download error=%v", err)
	}
}
