package kybdocuments3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/documentcodec"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/documents3"
	"github.com/HernanEspinozaDev/espaciGo/internal/documentverification"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
)

const keyVersionMetadata = "key_version"
const maxEnvelopeBytes = documentcodec.MaxDocumentBytes + 64

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type Store struct {
	backend documents3.ObjectStore
	keys    map[string][]byte
	current string
}

func New(backend documents3.ObjectStore, keys map[string][]byte, current string) (*Store, error) {
	if backend == nil || len(keys) == 0 || !versionPattern.MatchString(current) {
		return nil, kyb.ErrInvalid
	}
	if len(keys[current]) != 32 {
		return nil, kyb.ErrInvalid
	}
	cloned := make(map[string][]byte, len(keys))
	for version, key := range keys {
		if !versionPattern.MatchString(version) || len(key) != 32 {
			return nil, kyb.ErrInvalid
		}
		cloned[version] = append([]byte(nil), key...)
	}
	return &Store{backend: backend, keys: cloned, current: current}, nil
}

func (s *Store) Close() error {
	if closer, ok := s.backend.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (s *Store) Put(ctx context.Context, ownerID, organizationID, verificationID, storageRef string, content []byte) error {
	if s == nil || len(content) == 0 || len(content) > documentcodec.MaxDocumentBytes || documentverification.ValidateID(storageRef) != nil {
		return kyb.ErrInvalid
	}
	scope, err := documentcodec.KYBScope(ownerID, organizationID, verificationID, storageRef)
	if err != nil {
		return kyb.ErrInvalid
	}
	envelope, err := documentcodec.Seal(s.keys[s.current], scope, content)
	if err != nil {
		return err
	}
	if err = s.backend.PutWithMetadata(ctx, storageRef, envelope, map[string]string{keyVersionMetadata: s.current}); err != nil {
		return errors.New("kyb: private document write unavailable")
	}
	return nil
}

func (s *Store) Read(ctx context.Context, ownerID, organizationID, verificationID, storageRef string) ([]byte, error) {
	if s == nil || documentverification.ValidateID(storageRef) != nil {
		return nil, kyb.ErrInvalid
	}
	scope, err := documentcodec.KYBScope(ownerID, organizationID, verificationID, storageRef)
	if err != nil {
		return nil, kyb.ErrInvalid
	}
	envelope, metadata, err := s.backend.GetWithMetadata(ctx, storageRef, maxEnvelopeBytes)
	if err != nil {
		return nil, errors.New("kyb: private document read unavailable")
	}
	version := metadata[keyVersionMetadata]
	key, ok := s.keys[version]
	if !ok || !versionPattern.MatchString(version) {
		return nil, errors.New("kyb: private document key unavailable")
	}
	content, err := documentcodec.Open(key, scope, envelope)
	if err != nil {
		return nil, errors.New("kyb: private document integrity check failed")
	}
	return content, nil
}

func (s *Store) Delete(ctx context.Context, ownerID, organizationID, verificationID, storageRef string) error {
	if s == nil || documentverification.ValidateID(storageRef) != nil {
		return kyb.ErrInvalid
	}
	if _, err := documentcodec.KYBScope(ownerID, organizationID, verificationID, storageRef); err != nil {
		return kyb.ErrInvalid
	}
	if err := s.backend.Delete(ctx, storageRef); err != nil {
		return errors.New("kyb: private document delete unavailable")
	}
	return nil
}

func (s *Store) SignReviewAccess(storageRef string, expires int64) (string, string, error) {
	if s == nil || documentverification.ValidateID(storageRef) != nil || expires <= time.Now().Unix() {
		return "", "", kyb.ErrInvalid
	}
	signature := reviewMAC(s.keys[s.current], storageRef, expires)
	return s.current, hex.EncodeToString(signature), nil
}

func (s *Store) VerifyReviewAccess(storageRef string, expires int64, version, signature string, now time.Time) bool {
	if s == nil || documentverification.ValidateID(storageRef) != nil || now.IsZero() || expires <= now.Unix() {
		return false
	}
	key, ok := s.keys[version]
	if !ok || !versionPattern.MatchString(version) {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	return hmac.Equal(provided, reviewMAC(key, storageRef, expires))
}

func reviewMAC(encryptionKey []byte, storageRef string, expires int64) []byte {
	derive := hmac.New(sha256.New, encryptionKey)
	_, _ = derive.Write([]byte("espacigo:kyb-review-url-signing-key:v1"))
	signingKey := derive.Sum(nil)
	message := []byte("espacigo:kyb-review-url:v1\n" + storageRef + "\n" + time.Unix(expires, 0).UTC().Format(time.RFC3339))
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write(message)
	return mac.Sum(nil)
}
