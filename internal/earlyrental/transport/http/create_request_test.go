package earlyrentalhttp

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDecodeCreateOfferRequestParsesAndNormalizesInstants(t *testing.T) {
	body := []byte(`{"space_id":"550E8400-E29B-41D4-A716-446655440000","start_at":"2026-11-17T10:00:00.125-03:00","end_at":"2026-11-17T18:00:00Z"}`)
	got, err := DecodeCreateOfferRequest(body)
	if err != nil {
		t.Fatalf("DecodeCreateOfferRequest() error = %v", err)
	}
	if got.SpaceID.String() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("space ID = %q", got.SpaceID)
	}
	wantStart := time.Date(2026, 11, 17, 13, 0, 0, 125_000_000, time.UTC)
	wantEnd := time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)
	if !got.StartAt.Equal(wantStart) || got.StartAt.Location() != time.UTC ||
		!got.EndAt.Equal(wantEnd) || got.EndAt.Location() != time.UTC {
		t.Fatalf("decoded interval = %v to %v; want %v to %v in UTC", got.StartAt, got.EndAt, wantStart, wantEnd)
	}
}

func TestDecodeCreateOfferRequestRejectsMalformedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "null", body: "null"},
		{name: "array", body: "[]"},
		{name: "missing end", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z"}`},
		{name: "unknown actor", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z","actor_id":"renter"}`},
		{name: "client price", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z","unit_price_clp":1}`},
		{name: "duplicate space id", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","space_id":"550e8400-e29b-41d4-a716-446655440001","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "case variant property", body: `{"Space_Id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "trailing object", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z"} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if request, err := DecodeCreateOfferRequest([]byte(tc.body)); !errors.Is(err, ErrMalformedCreateOfferBody) || request.SpaceID != uuid.Nil {
				t.Fatalf("DecodeCreateOfferRequest() = %+v, %v; want empty result and malformed-body error", request, err)
			}
		})
	}
	if request, err := DecodeCreateOfferRequest(make([]byte, maxCommandBodyBytes+1)); !errors.Is(err, ErrMalformedCreateOfferBody) || request.SpaceID != uuid.Nil {
		t.Fatalf("oversized body result = %+v, %v; want malformed-body error", request, err)
	}
}

func TestDecodeCreateOfferRequestRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "invalid space UUID", body: `{"space_id":"not-a-uuid","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "noncanonical UUID form", body: `{"space_id":"550e8400e29b41d4a716446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "invalid RFC3339", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026/11/17 10:00","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "timestamp without offset", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00","end_at":"2026-11-17T11:00:00Z"}`},
		{name: "empty interval", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T10:00:00Z","end_at":"2026-11-17T10:00:00Z"}`},
		{name: "reversed interval", body: `{"space_id":"550e8400-e29b-41d4-a716-446655440000","start_at":"2026-11-17T11:00:00Z","end_at":"2026-11-17T10:00:00Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if request, err := DecodeCreateOfferRequest([]byte(tc.body)); !errors.Is(err, ErrInvalidCreateOfferValue) || request.SpaceID != uuid.Nil {
				t.Fatalf("DecodeCreateOfferRequest() = %+v, %v; want empty result and invalid-value error", request, err)
			}
		})
	}
}
