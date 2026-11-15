package s3storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const replayRegistryMaxBytes = 16 << 20

// ReplayRegistry stores one small append-only manifest object in a private
// S3 bucket. Every update is guarded by the previously observed revision/ETag.
type ReplayRegistry struct {
	client *s3.Client
	bucket string
	key    string
}

func NewReplayRegistry(ctx context.Context, bucketName, prefix string) (*ReplayRegistry, error) {
	if bucketName == "" {
		return nil, errors.New("privacy replay bucket is required")
	}
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	client, err := NewClient(ctx)
	if err != nil {
		return nil, privacy.ErrReplayStoreUnavailable
	}
	return newReplayRegistryWithClient(client, bucketName, prefix), nil
}

func newReplayRegistryWithClient(client *s3.Client, bucketName, prefix string) *ReplayRegistry {
	name := "privacy-replay-v1.json"
	if prefix != "" {
		name = prefix + "/" + name
	}
	return &ReplayRegistry{client: client, bucket: bucketName, key: name}
}

func (r *ReplayRegistry) Close() error {
	return nil
}

func (r *ReplayRegistry) Load(ctx context.Context) (privacy.SuppressionReplaySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return privacy.SuppressionReplaySnapshot{}, err
	}
	if r == nil || r.client == nil {
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreUnavailable
	}
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.key),
	})
	if isNotFoundError(err) {
		return emptyReplaySnapshot(), nil
	}
	if err != nil {
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreUnavailable
	}
	if out.ContentLength != nil && (*out.ContentLength < 0 || *out.ContentLength > replayRegistryMaxBytes) {
		_ = out.Body.Close()
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreConflict
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, replayRegistryMaxBytes+1))
	closeErr := out.Body.Close()
	if err != nil || closeErr != nil || len(data) > replayRegistryMaxBytes {
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreUnavailable
	}
	manifest, err := decodeReplayManifest(data)
	if err != nil {
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreConflict
	}
	rev := extractReplayRevision(out.Metadata, out.ETag)
	if rev == "" {
		return privacy.SuppressionReplaySnapshot{}, privacy.ErrReplayStoreUnavailable
	}
	return privacy.SuppressionReplaySnapshot{Revision: rev, Manifest: manifest}, nil
}

func (r *ReplayRegistry) CompareAndSwap(ctx context.Context, expectedRevision string, manifest privacy.SuppressionReplayManifest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if r == nil || r.client == nil {
		return "", privacy.ErrReplayStoreUnavailable
	}
	normalized, err := privacy.NormalizeSuppressionReplayManifest(manifest)
	if err != nil || len(normalized.Entries) != len(manifest.Entries) {
		return "", privacy.ErrReplayStoreConflict
	}
	data, err := json.Marshal(normalized)
	if err != nil || len(data) > replayRegistryMaxBytes {
		return "", privacy.ErrReplayStoreConflict
	}

	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(r.key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	}

	nextRevNum := int64(1)
	if expectedRevision == "" {
		putInput.IfNoneMatch = aws.String("*")
	} else {
		head, headErr := r.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(r.bucket),
			Key:    aws.String(r.key),
		})
		if isNotFoundError(headErr) {
			return "", privacy.ErrReplayStoreConflict
		}
		if headErr != nil {
			return "", privacy.ErrReplayStoreUnavailable
		}
		currentRev := extractReplayRevision(head.Metadata, head.ETag)
		if currentRev == "" || currentRev != expectedRevision {
			return "", privacy.ErrReplayStoreConflict
		}
		if parsed, parseErr := strconv.ParseInt(currentRev, 10, 64); parseErr == nil && parsed > 0 {
			nextRevNum = parsed + 1
		}
		if head.ETag != nil && *head.ETag != "" {
			putInput.IfMatch = head.ETag
		}
	}
	putInput.Metadata = map[string]string{
		"revision": strconv.FormatInt(nextRevNum, 10),
	}

	if _, err := r.client.PutObject(ctx, putInput); err != nil {
		if isPreconditionFailedError(err) {
			return "", privacy.ErrReplayStoreConflict
		}
		return "", privacy.ErrReplayStoreUnavailable
	}
	snapshot, err := r.Load(ctx)
	if err != nil || snapshot.Revision == "" {
		return "", privacy.ErrReplayStoreUnavailable
	}
	return snapshot.Revision, nil
}

func extractReplayRevision(metadata map[string]string, etag *string) string {
	if metadata != nil {
		if rev := strings.TrimSpace(metadata["revision"]); rev != "" {
			return rev
		}
	}
	if etag != nil {
		return strings.TrimSpace(strings.Trim(*etag, `"`))
	}
	return ""
}

func emptyReplaySnapshot() privacy.SuppressionReplaySnapshot {
	return privacy.SuppressionReplaySnapshot{
		Manifest: privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{}},
	}
}

func decodeReplayManifest(data []byte) (privacy.SuppressionReplayManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest privacy.SuppressionReplayManifest
	if err := decoder.Decode(&manifest); err != nil {
		return privacy.SuppressionReplayManifest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return privacy.SuppressionReplayManifest{}, privacy.ErrInvalid
	}
	normalized, err := privacy.NormalizeSuppressionReplayManifest(manifest)
	if err != nil || manifest.Version != 1 || len(normalized.Entries) != len(manifest.Entries) {
		return privacy.SuppressionReplayManifest{}, privacy.ErrInvalid
	}
	return normalized, nil
}
