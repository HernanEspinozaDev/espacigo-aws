package s3storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type replayObjectServer struct {
	mu           sync.Mutex
	data         []byte
	generation   int64
	revision     string
	writeCount   int
	loseNext     bool
	requests     []string
	serverErrors []string
}

func (s *replayObjectServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Method+" "+r.URL.String())
	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/private-bucket/privacy/privacy-replay-v1.json":
		ifNoneMatch := r.Header.Get("If-None-Match")
		ifMatch := r.Header.Get("If-Match")
		currentETag := `"` + strconv.FormatInt(s.generation, 10) + `"`
		if ifNoneMatch == "*" && s.generation != 0 {
			s.serverErrors = append(s.serverErrors, "if-none-match failed")
			http.Error(w, "precondition failed", http.StatusPreconditionFailed)
			return
		}
		if ifMatch != "" && ifMatch != currentETag {
			s.serverErrors = append(s.serverErrors, "if-match failed: "+ifMatch)
			http.Error(w, "precondition failed", http.StatusPreconditionFailed)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		s.data = append([]byte(nil), data...)
		s.generation++
		s.writeCount++
		s.revision = r.Header.Get("X-Amz-Meta-Revision")
		if s.revision == "" {
			s.revision = strconv.FormatInt(s.generation, 10)
		}
		if s.loseNext {
			s.loseNext = false
			http.Error(w, "response lost after commit", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("ETag", `"`+strconv.FormatInt(s.generation, 10)+`"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/private-bucket/"):
		if s.generation == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", `"`+strconv.FormatInt(s.generation, 10)+`"`)
		if s.revision != "" {
			w.Header().Set("x-amz-meta-revision", s.revision)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)))
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/private-bucket/"):
		if s.generation == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", `"`+strconv.FormatInt(s.generation, 10)+`"`)
		if s.revision != "" {
			w.Header().Set("x-amz-meta-revision", s.revision)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.data)
	default:
		http.NotFound(w, r)
	}
}

func newTestReplayRegistry(t *testing.T, endpoint string) *ReplayRegistry {
	t.Helper()
	client := newMockS3Client(t, endpoint)
	return newReplayRegistryWithClient(client, "private-bucket", "privacy")
}

func replayEntry(executionID, requestID string) privacy.SuppressionReplayEntry {
	return privacy.SuppressionReplayEntry{
		ExecutionID: executionID,
		RequestID:   requestID,
		AccountID:   "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		RequestedAt: time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC),
	}
}

func TestReplayRegistryCASPreventsConcurrentInstancesFromLosingEntries(t *testing.T) {
	serverState := &replayObjectServer{}
	server := httptest.NewServer(serverState)
	defer server.Close()
	first := newTestReplayRegistry(t, server.URL)
	second := newTestReplayRegistry(t, server.URL)
	ctx := context.Background()

	firstSnapshot, err := first.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := second.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	one := replayEntry("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "11111111-1111-4111-8111-111111111111")
	two := replayEntry("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "22222222-2222-4222-8222-222222222222")
	manifestOne := privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{one}}
	if _, err := first.CompareAndSwap(ctx, firstSnapshot.Revision, manifestOne); err != nil {
		t.Fatalf("first CAS: %v requests=%v errors=%v", err, serverState.requests, serverState.serverErrors)
	}
	manifestTwo := privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{two}}
	if _, err := second.CompareAndSwap(ctx, secondSnapshot.Revision, manifestTwo); err != privacy.ErrReplayStoreConflict {
		t.Fatalf("stale generation CAS error=%v, want conflict", err)
	}
	latest, err := second.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := privacy.NormalizeSuppressionReplayManifest(privacy.SuppressionReplayManifest{Version: 1, Entries: append(latest.Manifest.Entries, two)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.CompareAndSwap(ctx, latest.Revision, merged); err != nil {
		t.Fatal(err)
	}
	final, err := first.Load(ctx)
	if err != nil || len(final.Manifest.Entries) != 2 || final.Manifest.Entries[0].ExecutionID == "" {
		t.Fatalf("final manifest=%+v err=%v", final.Manifest, err)
	}
}

func TestReplayRegistryWriteResponseLossIsVisibleAsUncertainGeneration(t *testing.T) {
	serverState := &replayObjectServer{loseNext: true}
	server := httptest.NewServer(serverState)
	defer server.Close()
	registry := newTestReplayRegistry(t, server.URL)
	entry := replayEntry("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "11111111-1111-4111-8111-111111111111")
	manifest := privacy.SuppressionReplayManifest{Version: 1, Entries: []privacy.SuppressionReplayEntry{entry}}
	_, err := registry.CompareAndSwap(context.Background(), "", manifest)
	if !errors.Is(err, privacy.ErrReplayStoreUnavailable) && !errors.Is(err, privacy.ErrReplayStoreConflict) {
		t.Fatalf("CAS after committed response loss returned %v, want uncertain result; requests=%v", err, serverState.requests)
	}
	snapshot, err := registry.Load(context.Background())
	if err != nil || len(snapshot.Manifest.Entries) != 1 || snapshot.Manifest.Entries[0] != entry {
		t.Fatalf("committed generation not visible after lost response: snapshot=%+v err=%v requests=%v errors=%v", snapshot, err, serverState.requests, serverState.serverErrors)
	}
}

func TestReplayRegistryRejectsMalformedAndOversizedManifests(t *testing.T) {
	for _, manifest := range []privacy.SuppressionReplayManifest{
		{Version: 2, Entries: []privacy.SuppressionReplayEntry{}},
		{Version: 1, Entries: []privacy.SuppressionReplayEntry{replayEntry("not-a-uuid", "11111111-1111-4111-8111-111111111111")}},
	} {
		if _, err := decodeReplayManifest(mustJSON(t, manifest)); err == nil {
			t.Fatalf("accepted malformed manifest: %+v", manifest)
		}
	}
	if err := validatePrefix("../outside"); err == nil {
		t.Fatal("invalid prefix accepted")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
