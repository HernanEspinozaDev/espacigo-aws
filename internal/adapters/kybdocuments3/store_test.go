package kybdocuments3

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

type memoryObjectStore struct {
	objects  map[string][]byte
	metadata map[string]map[string]string
}

func (s *memoryObjectStore) PutWithMetadata(_ context.Context, id string, value []byte, metadata map[string]string) error {
	if s.objects[id] != nil {
		return errors.New("exists")
	}
	s.objects[id] = append([]byte(nil), value...)
	s.metadata[id] = metadata
	return nil
}
func (s *memoryObjectStore) GetWithMetadata(_ context.Context, id string, _ int64) ([]byte, map[string]string, error) {
	value := s.objects[id]
	if value == nil {
		return nil, nil, errors.New("missing")
	}
	return append([]byte(nil), value...), s.metadata[id], nil
}
func (s *memoryObjectStore) Delete(_ context.Context, id string) error {
	delete(s.objects, id)
	delete(s.metadata, id)
	return nil
}

func TestKYBDocumentsAreEncryptedAndScopeBound(t *testing.T) {
	backend := &memoryObjectStore{objects: map[string][]byte{}, metadata: map[string]map[string]string{}}
	store, err := New(backend, map[string][]byte{"v1": bytes.Repeat([]byte{7}, 32)}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	ownerID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	organizationID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	verificationID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	storageRef := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	content := []byte("synthetic kyb document content")
	if err := store.Put(context.Background(), ownerID, organizationID, verificationID, storageRef, content); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backend.objects[storageRef], content) {
		t.Fatal("S3 envelope contains plaintext document bytes")
	}
	opened, err := store.Read(context.Background(), ownerID, organizationID, verificationID, storageRef)
	if err != nil || !bytes.Equal(opened, content) {
		t.Fatalf("read document=%q err=%v", opened, err)
	}
	if _, err := store.Read(context.Background(), "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", organizationID, verificationID, storageRef); err == nil {
		t.Fatal("document opened under another owner scope")
	}
}

func TestReviewCapabilityBindsDocumentAndExpiry(t *testing.T) {
	backend := &memoryObjectStore{objects: map[string][]byte{}, metadata: map[string]map[string]string{}}
	store, err := New(backend, map[string][]byte{"v1": bytes.Repeat([]byte{9}, 32)}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	storageRef := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	expires := time.Now().Add(time.Minute).Unix()
	version, signature, err := store.SignReviewAccess(storageRef, expires)
	if err != nil {
		t.Fatal(err)
	}
	if !store.VerifyReviewAccess(storageRef, expires, version, signature, time.Now()) {
		t.Fatal("valid review capability was rejected")
	}
	if store.VerifyReviewAccess("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", expires, version, signature, time.Now()) {
		t.Fatal("review capability was accepted for another document")
	}
	if store.VerifyReviewAccess(storageRef, expires+1, version, signature, time.Now()) {
		t.Fatal("review capability was accepted with a changed expiry")
	}
	if store.VerifyReviewAccess(storageRef, expires, version, signature, time.Unix(expires, 0)) {
		t.Fatal("expired review capability was accepted")
	}
}
