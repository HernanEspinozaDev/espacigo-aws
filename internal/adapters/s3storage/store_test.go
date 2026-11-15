package s3storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func newMockS3Client(t *testing.T, serverURL string) *s3.Client {
	t.Helper()
	return s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(serverURL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("TESTKEY", "TESTSECRET", ""),
		RetryMaxAttempts: 1,
	})
}

func TestStorePrivateObjectLifecycleAndCreateOnlyWrite(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	var storedPath string
	var data []byte
	var deletedIfMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/private-bucket/runs/g03o-1017/"+id+".blob":
			if r.Header.Get("If-None-Match") != "*" {
				http.Error(w, "missing create-only precondition", http.StatusBadRequest)
				return
			}
			storedPath = r.URL.Path
			data, _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"etag-7"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead && r.URL.Path == "/private-bucket/runs/g03o-1017/"+id+".blob":
			w.Header().Set("ETag", `"etag-7"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/private-bucket/runs/g03o-1017/"+id+".blob":
			w.Header().Set("ETag", `"etag-7"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
		case r.Method == http.MethodDelete && r.URL.Path == "/private-bucket/runs/g03o-1017/"+id+".blob":
			deletedIfMatch = r.Header.Get("If-Match")
			if deletedIfMatch != `"etag-7"` {
				http.Error(w, "missing observed ETag precondition", http.StatusPreconditionFailed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newMockS3Client(t, server.URL)
	store := newStoreWithPrefix(client, "private-bucket", "runs/g03o-1017")
	want := []byte("private synthetic bytes")
	if err := store.Put(context.Background(), id, want); err != nil {
		t.Fatal(err)
	}
	if storedPath != "/private-bucket/runs/g03o-1017/"+id+".blob" || string(data) != string(want) {
		t.Fatalf("stored object = %q %q", storedPath, data)
	}
	got, err := store.Get(context.Background(), id)
	if err != nil || string(got) != string(want) {
		t.Fatalf("Get() = %q, %v", got, err)
	}
	if err := store.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if deletedIfMatch != `"etag-7"` {
		t.Fatalf("Delete used If-Match=%q, want observed ETag \"etag-7\"", deletedIfMatch)
	}
}

func TestStoreDeleteDoesNotRemoveReplacementGeneration(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	currentETag := `"etag-8"`
	var deletePrecondition string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.Header().Set("ETag", `"etag-7"`)
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete:
			deletePrecondition = r.Header.Get("If-Match")
			if deletePrecondition != currentETag {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = io.WriteString(w, `<Error><Code>PreconditionFailed</Code><Message>ETag changed</Message></Error>`)
				return
			}
			currentETag = ""
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newMockS3Client(t, server.URL)
	store := newStoreWithClient(client, "private-bucket")
	if err := store.Delete(context.Background(), id); err == nil {
		t.Fatal("Delete ignored an ETag change between metadata read and delete")
	}
	if deletePrecondition != `"etag-7"` || currentETag != `"etag-8"` {
		t.Fatalf("delete precondition=%q current ETag=%q; replacement object was removed", deletePrecondition, currentETag)
	}
}

func TestStoreRejectsInvalidIDAndCanceledContextBeforeAccess(t *testing.T) {
	store := &Store{}
	if err := store.Put(context.Background(), "../public/file", []byte("x")); err == nil {
		t.Fatal("Put accepted path traversal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Get(ctx, "11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("Get ignored canceled context")
	}
}

func TestStoreDeleteReturnsRemoteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "synthetic storage failure", http.StatusForbidden)
	}))
	defer server.Close()
	client := newMockS3Client(t, server.URL)
	store := newStoreWithClient(client, "private-bucket")
	if err := store.Delete(context.Background(), "11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("Delete swallowed a remote storage failure")
	}
}

func TestStoreDeleteIsIdempotentWhenObjectIsAlreadyMissing(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name              string
		missingOnHead     bool
		wantDeleteRequest int
	}{
		{name: "missing before head lookup", missingOnHead: true},
		{name: "disappears after head lookup", wantDeleteRequest: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var headCalls, deleteCalls int
			var deleteETag string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodHead:
					headCalls++
					if tc.missingOnHead {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Header().Set("ETag", `"etag-7"`)
					w.Header().Set("Content-Length", "1")
					w.WriteHeader(http.StatusOK)
				case http.MethodDelete:
					deleteCalls++
					deleteETag = r.Header.Get("If-Match")
					w.WriteHeader(http.StatusNotFound)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			client := newMockS3Client(t, server.URL)
			store := newStoreWithClient(client, "private-bucket")
			if err := store.Delete(context.Background(), id); err != nil {
				t.Fatalf("Delete() for an already-missing object: %v", err)
			}
			if headCalls != 1 || deleteCalls != tc.wantDeleteRequest {
				t.Fatalf("head calls=%d delete calls=%d, want 1 and %d", headCalls, deleteCalls, tc.wantDeleteRequest)
			}
			if tc.wantDeleteRequest == 1 && deleteETag != `"etag-7"` {
				t.Fatalf("delete precondition=%q, want observed ETag \"etag-7\"", deleteETag)
			}
		})
	}
}

func TestValidatePrefixRejectsTraversalAndAcceptsExactRunPrefix(t *testing.T) {
	for _, prefix := range []string{"../other", "runs/../other", "/absolute", "runs//g03o-1017", "runs\\g03o-1017"} {
		if err := validatePrefix(prefix); err == nil {
			t.Fatalf("validatePrefix(%q) accepted unsafe prefix", prefix)
		}
	}
	if err := validatePrefix("runs/g03o-1017"); err != nil {
		t.Fatalf("validatePrefix rejected reviewed run prefix: %v", err)
	}
}

func TestStoreMetadataRoundTripAndBoundedRead(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	want := []byte("encrypted synthetic document")
	wantMetadata := map[string]string{"key_version": "v1"}
	var stored []byte
	storedMetadata := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("If-None-Match") != "*" {
				http.Error(w, "missing create-only precondition", http.StatusBadRequest)
				return
			}
			for k, vals := range r.Header {
				lower := strings.ToLower(k)
				if strings.HasPrefix(lower, "x-amz-meta-") && len(vals) > 0 {
					storedMetadata[strings.TrimPrefix(lower, "x-amz-meta-")] = vals[0]
				}
			}
			stored, _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"etag-7"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			for k, v := range storedMetadata {
				w.Header().Set("x-amz-meta-"+k, v)
			}
			w.Header().Set("ETag", `"etag-7"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(stored)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.Header.Get("If-Match") != `"etag-7"` {
				http.Error(w, "read did not pin the observed object ETag", http.StatusPreconditionFailed)
				return
			}
			for k, v := range storedMetadata {
				w.Header().Set("x-amz-meta-"+k, v)
			}
			w.Header().Set("ETag", `"etag-7"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(stored)))
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(stored)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newMockS3Client(t, server.URL)
	store := newStoreWithClient(client, "private-bucket")
	if err := store.PutWithMetadata(context.Background(), id, want, wantMetadata); err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(want) || storedMetadata["key_version"] != "v1" {
		t.Fatalf("stored object = %q metadata=%v", stored, storedMetadata)
	}
	got, metadata, err := store.GetWithMetadata(context.Background(), id, int64(len(want)))
	if err != nil || string(got) != string(want) || metadata["key_version"] != "v1" {
		t.Fatalf("GetWithMetadata() = %q, %v, %v", got, metadata, err)
	}
	got, metadata, err = store.GetWithMetadata(context.Background(), id, int64(len(want)-1))
	if err == nil || got != nil || metadata != nil {
		t.Fatalf("bounded GetWithMetadata() = %q, %v, %v; want size rejection", got, metadata, err)
	}
}

func TestValidateMetadataRejectsUnsafeValuesAndBounds(t *testing.T) {
	tooMany := make(map[string]string)
	for i := 0; i < 33; i++ {
		tooMany["key"+strconv.Itoa(i)] = "v"
	}
	tooLarge := make(map[string]string)
	for i := 0; i < 9; i++ {
		tooLarge["k"+strconv.Itoa(i)] = strings.Repeat("x", 1000)
	}
	for name, metadata := range map[string]map[string]string{
		"uppercase key":       {"Key": "v1"},
		"empty value":         {"key": ""},
		"line break":          {"key": "v1\nother"},
		"delete control":      {"key": "v1\x7f"},
		"non-ascii":           {"key": "versión"},
		"oversized value":     {"key": strings.Repeat("x", 1025)},
		"too many entries":    tooMany,
		"oversized aggregate": tooLarge,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateMetadata(metadata); err == nil {
				t.Fatalf("validateMetadata(%q) accepted unsafe metadata", name)
			}
		})
	}

	input := map[string]string{"key_version": "v1"}
	clone, err := validateMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	input["key_version"] = "v2"
	if clone["key_version"] != "v1" {
		t.Fatalf("validated metadata aliases caller map: %v", clone)
	}
}
