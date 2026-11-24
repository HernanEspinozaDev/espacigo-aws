package commercialreservationdetail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

type repoFake struct {
	record Record
	err    error
}

func (r repoFake) Get(context.Context, string, string) (Record, error) { return r.record, r.err }

type storeFake struct {
	blob     []byte
	metadata map[string]string
	err      error
}

func (s storeFake) GetWithMetadata(context.Context, string, int64) ([]byte, map[string]string, error) {
	return s.blob, s.metadata, s.err
}

func TestDownloadRequiresReadyVerifiedPrivateArtifact(t *testing.T) {
	actor, reservation, object := uuid.NewString(), uuid.NewString(), uuid.NewString()
	blob := []byte("%PDF-1.7\nprivate")
	pdfHash := sha256.Sum256(blob)
	documentHash := sha256.Sum256([]byte("accepted proposal"))
	store := storeFake{blob: blob, metadata: map[string]string{
		"sha256":           hex.EncodeToString(pdfHash[:]),
		"document_sha256":  hex.EncodeToString(documentHash[:]),
		"proposal_version": "3",
	}}
	record := Record{
		Documents: Documents{ReservationID: reservation, Contract: Contract{State: "ready"}},
		ObjectID:  object, Bucket: "private-evidence", PDFSHA256: hex.EncodeToString(pdfHash[:]),
		DocumentSHA256: hex.EncodeToString(documentHash[:]), Version: 3, Size: int64(len(blob)),
	}
	service, err := New(repoFake{record: record}, store, "private-evidence")
	if err != nil {
		t.Fatal(err)
	}
	got, digest, err := service.Download(context.Background(), actor, reservation)
	if err != nil || string(got) != string(blob) || digest != hex.EncodeToString(pdfHash[:]) {
		t.Fatalf("Download() = %q, %q, %v", got, digest, err)
	}
	store.metadata["proposal_version"] = "2"
	service, _ = New(repoFake{record: record}, store, "private-evidence")
	if _, _, err := service.Download(context.Background(), actor, reservation); err != ErrUnavailable {
		t.Fatalf("metadata mismatch error = %v, want ErrUnavailable", err)
	}
}

func TestDownloadPendingAndInvalidIdentity(t *testing.T) {
	reservation := uuid.NewString()
	service, err := New(repoFake{record: Record{Documents: Documents{ReservationID: reservation, Contract: Contract{State: "processing"}}}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Download(context.Background(), uuid.NewString(), reservation); err != ErrDocumentPending {
		t.Fatalf("pending download error = %v, want ErrDocumentPending", err)
	}
	if _, _, err := service.Download(context.Background(), "not-a-uuid", reservation); err != ErrNotFound {
		t.Fatalf("invalid actor error = %v, want ErrNotFound", err)
	}
}
