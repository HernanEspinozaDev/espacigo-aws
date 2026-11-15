package s3storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/media"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var mediaUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89a-fA-F][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var mediaGenerationSeq atomic.Uint64

// MediaStore confines uploads to generated names inside the versioned
// media/v1 and users/.../images/v2 namespaces and always uses create-only
// writes and exact generations.
type MediaStore struct {
	client     *s3.Client
	bucketName string
}

func NewMediaStore(ctx context.Context, bucketName string) (*MediaStore, error) {
	if strings.TrimSpace(bucketName) == "" {
		return nil, errors.New("media bucket is required")
	}
	client, err := NewClient(ctx)
	if err != nil {
		return nil, errors.New("media storage client initialization failed")
	}
	return &MediaStore{client: client, bucketName: bucketName}, nil
}

func newMediaStoreWithClient(client *s3.Client, bucketName string) *MediaStore {
	return &MediaStore{client: client, bucketName: bucketName}
}

func (s *MediaStore) Close() error {
	return nil
}

func (s *MediaStore) Put(ctx context.Context, key string, b []byte, mime string) (media.StoredObject, error) {
	if err := validateMediaKey(key); err != nil || mime != "image/webp" || len(b) == 0 {
		return media.StoredObject{}, media.ErrInvalid
	}
	if s == nil || s.client == nil {
		return media.StoredObject{}, media.ErrUnavailable
	}
	sum := hashMedia(b)
	gen := computeMediaGeneration(key, b)
	out, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.bucketName),
		Key:          aws.String(key),
		Body:         bytes.NewReader(b),
		ContentType:  aws.String(mime),
		CacheControl: aws.String("private, no-store"),
		IfNoneMatch:  aws.String("*"),
		Metadata: map[string]string{
			"sha256":     sum,
			"generation": strconv.FormatInt(gen, 10),
		},
	})
	if err != nil {
		return media.StoredObject{}, err
	}
	if gen <= 0 && out != nil && out.ETag != nil {
		gen = generationFromETag(*out.ETag)
	}
	if gen <= 0 {
		return media.StoredObject{}, errors.New("media object generation unavailable")
	}
	return media.StoredObject{
		ObjectKey:  key,
		Provider:   "s3",
		Bucket:     s.bucketName,
		Generation: gen,
		Size:       int64(len(b)),
		SHA256:     sum,
	}, nil
}

func (s *MediaStore) Get(ctx context.Context, key string, generation int64) ([]byte, error) {
	if err := validateMediaKey(key); err != nil || generation <= 0 {
		return nil, media.ErrInvalid
	}
	if s == nil || s.client == nil {
		return nil, media.ErrUnavailable
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, media.ErrNotFound
		}
		return nil, err
	}
	actualGen := extractMediaGeneration(out.Metadata, out.ETag)
	if actualGen != generation {
		_ = out.Body.Close()
		return nil, media.ErrNotFound
	}
	b, readErr := io.ReadAll(io.LimitReader(out.Body, 10<<20+1))
	closeErr := out.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(b) > 10<<20 {
		return nil, media.ErrInvalid
	}
	return b, nil
}

func (s *MediaStore) Generation(ctx context.Context, key string) (int64, error) {
	if err := validateMediaKey(key); err != nil {
		return 0, media.ErrInvalid
	}
	if s == nil || s.client == nil {
		return 0, media.ErrUnavailable
	}
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if isNotFoundError(err) {
		return 0, media.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	gen := extractMediaGeneration(head.Metadata, head.ETag)
	if gen <= 0 {
		return 0, errors.New("media object generation unavailable")
	}
	return gen, nil
}

func (s *MediaStore) Delete(ctx context.Context, key string, generation int64) error {
	if err := validateMediaKey(key); err != nil || generation <= 0 {
		return media.ErrInvalid
	}
	if s == nil || s.client == nil {
		return media.ErrUnavailable
	}
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if isNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	actualGen := extractMediaGeneration(head.Metadata, head.ETag)
	if actualGen != generation {
		return errors.New("media object generation mismatch")
	}
	delInput := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	}
	if head.ETag != nil && *head.ETag != "" {
		delInput.IfMatch = head.ETag
	}
	_, err = s.client.DeleteObject(ctx, delInput)
	if isNotFoundError(err) {
		return nil
	}
	return err
}

func computeMediaGeneration(key string, b []byte) int64 {
	seq := mediaGenerationSeq.Add(1)
	h := sha256.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(b)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seq)
	_, _ = h.Write(buf[:])
	sum := h.Sum(nil)
	v := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	if v <= 0 {
		v = 1
	}
	return v
}

func extractMediaGeneration(metadata map[string]string, etag *string) int64 {
	if metadata != nil {
		if raw, ok := metadata["generation"]; ok {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	if etag != nil {
		return generationFromETag(*etag)
	}
	return 0
}

func generationFromETag(etag string) int64 {
	cleaned := strings.TrimSpace(strings.Trim(etag, `"`))
	if cleaned == "" {
		return 0
	}
	if parsed, err := strconv.ParseInt(cleaned, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	sum := sha256.Sum256([]byte(cleaned))
	v := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	if v <= 0 {
		v = 1
	}
	return v
}

func validateMediaKey(key string) error {
	parts := strings.Split(key, "/")
	if len(parts) == 7 && parts[0] == "media" && parts[1] == "v1" {
		if parts[2] == "spaces" && mediaUUID.MatchString(parts[3]) && mediaUUID.MatchString(parts[4]) && validToken(parts[5]) && (parts[6] == "display.webp" || parts[6] == "thumbnail.webp") {
			return nil
		}
		if parts[2] == "profiles" && mediaUUID.MatchString(parts[3]) && mediaUUID.MatchString(parts[4]) && validToken(parts[5]) && parts[6] == "avatar.webp" {
			return nil
		}
	}
	if len(parts) == 8 && parts[0] == "users" && mediaUUID.MatchString(parts[1]) && parts[2] == "spaces" && mediaUUID.MatchString(parts[3]) && parts[4] == "images" && parts[5] == "v2" && mediaUUID.MatchString(parts[6]) {
		bits := strings.Split(parts[7], "_")
		if len(bits) == 3 && validMediaTimestamp(bits[0]) && validToken(bits[1]) && (bits[2] == "display.webp" || bits[2] == "thumbnail.webp") {
			return nil
		}
	}
	return media.ErrInvalid
}

func validMediaTimestamp(v string) bool {
	const layout = "20060102T150405.000000000Z"
	parsed, err := time.Parse(layout, v)
	return err == nil && parsed.UTC().Format(layout) == v
}

func validToken(v string) bool {
	if v == "" || v[0] == '0' {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hashMedia(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
