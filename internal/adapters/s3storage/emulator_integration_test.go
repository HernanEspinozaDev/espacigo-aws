package s3storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/media"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestLiveAWSEmulatorStoreMediaAndReplayRegistry(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:9297", 2*time.Second)
	if err != nil {
		t.Skipf("aws-emulator not reachable at 127.0.0.1:9297: %v", err)
	}
	_ = conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	bucket := fmt.Sprintf("espacigo-s3-test-%d", time.Now().UnixNano())
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		t.Fatalf("CreateBucket(%q): %v", bucket, err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		out, listErr := client.ListObjectsV2(cleanCtx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		if listErr == nil && out != nil {
			for _, obj := range out.Contents {
				_, _ = client.DeleteObject(cleanCtx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucket),
					Key:    obj.Key,
				})
			}
		}
		_, _ = client.DeleteBucket(cleanCtx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	// 1. Test Store (PutWithMetadata, GetWithMetadata, create-only, bounded read, Delete)
	store, err := NewWithPrefix(ctx, bucket, "blobs/v1")
	if err != nil {
		t.Fatalf("NewWithPrefix: %v", err)
	}
	defer store.Close()

	const blobID = "11111111-1111-4111-8111-111111111111"
	payload := []byte("encrypted-document-envelope-bytes")
	meta := map[string]string{"key_version": "v1"}
	if err := store.PutWithMetadata(ctx, blobID, payload, meta); err != nil {
		t.Fatalf("PutWithMetadata: %v", err)
	}
	if err := store.PutWithMetadata(ctx, blobID, payload, meta); err == nil {
		t.Fatal("PutWithMetadata second write succeeded; expected create-only failure")
	}
	gotBlob, gotMeta, err := store.GetWithMetadata(ctx, blobID, int64(len(payload)))
	if err != nil || !bytes.Equal(gotBlob, payload) || gotMeta["key_version"] != "v1" {
		t.Fatalf("GetWithMetadata = %q, %v, %v", gotBlob, gotMeta, err)
	}
	if _, _, err := store.GetWithMetadata(ctx, blobID, int64(len(payload)-1)); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("GetWithMetadata bounded = %v, want ErrObjectTooLarge", err)
	}
	if err := store.Delete(ctx, blobID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, blobID); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}

	// 2. Test MediaStore (Put, Generation, Get, Delete)
	mediaStore, err := NewMediaStore(ctx, bucket)
	if err != nil {
		t.Fatalf("NewMediaStore: %v", err)
	}
	defer mediaStore.Close()

	mediaKey := "media/v1/spaces/00000000-0000-4000-8000-000000000001/00000000-0000-4000-8000-000000000002/1/display.webp"
	webpBytes := []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")
	stored, err := mediaStore.Put(ctx, mediaKey, webpBytes, "image/webp")
	if err != nil {
		t.Fatalf("MediaStore.Put: %v", err)
	}
	if stored.Provider != "s3" || stored.Bucket != bucket || stored.Generation <= 0 || stored.Size != int64(len(webpBytes)) {
		t.Fatalf("unexpected StoredObject: %+v", stored)
	}
	if _, err := mediaStore.Put(ctx, mediaKey, webpBytes, "image/webp"); err == nil {
		t.Fatal("MediaStore.Put overwrote existing object")
	}
	gen, err := mediaStore.Generation(ctx, mediaKey)
	if err != nil || gen != stored.Generation {
		t.Fatalf("MediaStore.Generation = %d, %v; want %d", gen, err, stored.Generation)
	}
	gotMedia, err := mediaStore.Get(ctx, mediaKey, stored.Generation)
	if err != nil || !bytes.Equal(gotMedia, webpBytes) {
		t.Fatalf("MediaStore.Get = %q, %v", gotMedia, err)
	}
	if _, err := mediaStore.Get(ctx, mediaKey, stored.Generation+1); err == nil {
		t.Fatal("MediaStore.Get accepted wrong generation")
	}
	if err := mediaStore.Delete(ctx, mediaKey, stored.Generation+1); err == nil {
		t.Fatal("MediaStore.Delete accepted wrong generation")
	}
	if err := mediaStore.Delete(ctx, mediaKey, stored.Generation); err != nil {
		t.Fatalf("MediaStore.Delete: %v", err)
	}
	if _, err := mediaStore.Generation(ctx, mediaKey); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("MediaStore.Generation after delete = %v, want ErrNotFound", err)
	}

	// 3. Test ReplayRegistry (Load empty, CAS create, CAS conflict, CAS update)
	replay, err := NewReplayRegistry(ctx, bucket, "privacy")
	if err != nil {
		t.Fatalf("NewReplayRegistry: %v", err)
	}
	defer replay.Close()

	snap0, err := replay.Load(ctx)
	if err != nil || snap0.Revision != "" || len(snap0.Manifest.Entries) != 0 {
		t.Fatalf("initial Load = %+v, %v", snap0, err)
	}
	entry1 := replayEntry("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "11111111-1111-4111-8111-111111111111")
	m1 := privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{entry1}}
	rev1, err := replay.CompareAndSwap(ctx, "", m1)
	if err != nil || rev1 == "" {
		t.Fatalf("first CompareAndSwap = %q, %v", rev1, err)
	}
	if _, err := replay.CompareAndSwap(ctx, "", m1); !errors.Is(err, privacy.ErrReplayStoreConflict) {
		t.Fatalf("duplicate create CAS = %v, want ErrReplayStoreConflict", err)
	}
	entry2 := replayEntry("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "22222222-2222-4222-8222-222222222222")
	m2, err := privacy.NormalizeSuppressionReplayManifest(privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{entry1, entry2}})
	if err != nil {
		t.Fatal(err)
	}
	rev2, err := replay.CompareAndSwap(ctx, rev1, m2)
	if err != nil || rev2 == "" || rev2 == rev1 {
		t.Fatalf("second CompareAndSwap = %q (prev %q), %v", rev2, rev1, err)
	}
	snapFinal, err := replay.Load(ctx)
	if err != nil || len(snapFinal.Manifest.Entries) != 2 || snapFinal.Revision != rev2 {
		t.Fatalf("final Load = %+v, %v", snapFinal, err)
	}
}
