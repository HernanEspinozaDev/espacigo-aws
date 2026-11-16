package publiccatalog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

const publicAttributesTestMasterKey = "01234567890123456789012345678901"

type publicAttributesRepo struct {
	Repository
	ctx  context.Context
	id   string
	rows []PublicAttribute
	err  error
}

func (r *publicAttributesRepo) PublicAttributes(ctx context.Context, id string) ([]PublicAttribute, error) {
	r.ctx, r.id = ctx, id
	return r.rows, r.err
}

func TestPublicAttributesReturnsTypedRepositoryRowsAndCanonicalID(t *testing.T) {
	const id = "ABCDEF00-0000-4000-8000-000000000001"
	ctx := context.WithValue(context.Background(), struct{}{}, "sentinel")
	want := []PublicAttribute{
		{Code: "capacity", Label: "Capacidad", Type: "number", Order: 2, Unit: "personas", Value: json.RawMessage(`12`)},
		{Code: "wifi", Label: "Wi-Fi", Type: "boolean", Order: 1, Value: json.RawMessage(`true`)},
	}
	repo := &publicAttributesRepo{Repository: memoryRepository{}, rows: want}
	svc, err := New(repo, []byte(publicAttributesTestMasterKey))
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.PublicAttributes(ctx, id)
	if err != nil {
		t.Fatalf("PublicAttributes() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PublicAttributes() = %#v, want %#v", got, want)
	}
	if repo.ctx != ctx {
		t.Fatal("repository did not receive the original context")
	}
	if repo.id != "abcdef00-0000-4000-8000-000000000001" {
		t.Fatalf("repository ID = %q, want lowercase ID", repo.id)
	}
}

func TestPublicAttributesPropagatesRepositoryError(t *testing.T) {
	failure := errors.New("synthetic public attributes failure")
	repo := &publicAttributesRepo{Repository: memoryRepository{}, err: failure}
	svc, err := New(repo, []byte(publicAttributesTestMasterKey))
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.PublicAttributes(context.Background(), "space-id")
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want propagated %v", err, failure)
	}
	if got != nil {
		t.Fatalf("attributes = %#v, want nil on repository error", got)
	}
}

func TestPublicAttributesFallsBackToNonNilEmptySlice(t *testing.T) {
	svc, err := New(memoryRepository{}, []byte(publicAttributesTestMasterKey))
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.PublicAttributes(context.Background(), "space-id")
	if err != nil {
		t.Fatalf("PublicAttributes() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("PublicAttributes() = %#v, want non-nil empty slice", got)
	}
}
