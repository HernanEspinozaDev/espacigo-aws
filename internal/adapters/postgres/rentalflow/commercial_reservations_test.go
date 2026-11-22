package rentalflowpg

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

func TestCommercialReservationCursor_Roundtrip(t *testing.T) {
	actor := "11111111-1111-4111-8111-111111111111"
	pageSize := 20
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	id := "22222222-2222-4222-8222-222222222222"

	cursor := commercialReservationCursor{
		Version:  1,
		Actor:    actor,
		PageSize: pageSize,
		Updated:  now,
		ID:       id,
	}

	encoded, err := encodeCommercialReservationCursor(cursor)
	if err != nil {
		t.Fatalf("failed to encode cursor: %v", err)
	}

	decoded, err := decodeCommercialReservationCursor(encoded, actor, pageSize)
	if err != nil {
		t.Fatalf("failed to decode cursor: %v", err)
	}
	if decoded == nil {
		t.Fatal("expected decoded cursor, got nil")
	}
	if decoded.Actor != actor || decoded.PageSize != pageSize || decoded.ID != id || !decoded.Updated.Equal(now) {
		t.Errorf("decoded cursor does not match: %+v", decoded)
	}
}

func TestCommercialReservationCursor_TamperAndValidation(t *testing.T) {
	actor := "11111111-1111-4111-8111-111111111111"
	pageSize := 20
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	id := "22222222-2222-4222-8222-222222222222"

	cursor := commercialReservationCursor{
		Version:  1,
		Actor:    actor,
		PageSize: pageSize,
		Updated:  now,
		ID:       id,
	}
	encoded, err := encodeCommercialReservationCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}

	// Empty encoded returns nil, nil
	if dec, err := decodeCommercialReservationCursor("", actor, pageSize); err != nil || dec != nil {
		t.Errorf("expected nil on empty cursor, got %v, %v", dec, err)
	}

	// Oversized encoded
	longEncoded := strings.Repeat("A", 4097)
	if _, err := decodeCommercialReservationCursor(longEncoded, actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on long cursor, got %v", err)
	}

	// Invalid base64
	if _, err := decodeCommercialReservationCursor("!@#$%^&*", actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on invalid base64, got %v", err)
	}

	// Wrong actor
	if _, err := decodeCommercialReservationCursor(encoded, "99999999-9999-4999-8999-999999999999", pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on wrong actor, got %v", err)
	}

	// Wrong pageSize
	if _, err := decodeCommercialReservationCursor(encoded, actor, 50); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on wrong pageSize, got %v", err)
	}

	// Non-UTC timezone in cursor
	loc := time.FixedZone("TEST", 3600)
	nonUTCCursor := commercialReservationCursor{
		Version:  1,
		Actor:    actor,
		PageSize: pageSize,
		Updated:  time.Date(2026, 11, 19, 13, 0, 0, 0, loc),
		ID:       id,
	}
	nonUTCEncoded, _ := encodeCommercialReservationCursor(nonUTCCursor)
	if _, err := decodeCommercialReservationCursor(nonUTCEncoded, actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on non-UTC offset, got %v", err)
	}

	// Invalid UUID in cursor
	invalidUUIDCursor := commercialReservationCursor{
		Version:  1,
		Actor:    actor,
		PageSize: pageSize,
		Updated:  now,
		ID:       "not-a-uuid",
	}
	invalidUUIDEncoded, _ := encodeCommercialReservationCursor(invalidUUIDCursor)
	if _, err := decodeCommercialReservationCursor(invalidUUIDEncoded, actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on invalid UUID in cursor, got %v", err)
	}

	// Wrong version
	v2Cursor := commercialReservationCursor{
		Version:  2,
		Actor:    actor,
		PageSize: pageSize,
		Updated:  now,
		ID:       id,
	}
	v2Encoded, _ := encodeCommercialReservationCursor(v2Cursor)
	if _, err := decodeCommercialReservationCursor(v2Encoded, actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on version 2 cursor, got %v", err)
	}

	// Payload larger than 2048 bytes
	largeBytes := make([]byte, 2049)
	largeEncoded := base64.RawURLEncoding.EncodeToString(largeBytes)
	if _, err := decodeCommercialReservationCursor(largeEncoded, actor, pageSize); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on oversized payload, got %v", err)
	}
}

func TestRepository_CommercialReservations_Validation(t *testing.T) {
	ctx := context.Background()
	validActor := "11111111-1111-4111-8111-111111111111"
	validReservation := "22222222-2222-4222-8222-222222222222"

	var nilRepo *Repository
	if _, err := nilRepo.ListCommercialReservations(ctx, validActor, 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on nil repo list, got %v", err)
	}
	if _, err := nilRepo.GetCommercialReservation(ctx, validActor, validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on nil repo get, got %v", err)
	}

	repo := &Repository{} // nil db
	if _, err := repo.ListCommercialReservations(ctx, validActor, 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on repo with nil db list, got %v", err)
	}
	if _, err := repo.GetCommercialReservation(ctx, validActor, validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on repo with nil db get, got %v", err)
	}

	// Invalid actor
	if _, err := repo.ListCommercialReservations(ctx, "bad-actor", 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on bad actor list, got %v", err)
	}
	if _, err := repo.GetCommercialReservation(ctx, "bad-actor", validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on bad actor get, got %v", err)
	}

	// Invalid reservation ID
	if _, err := repo.GetCommercialReservation(ctx, validActor, "bad-res"); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on bad reservation ID get, got %v", err)
	}

	// PageSize bounds
	if _, err := repo.ListCommercialReservations(ctx, validActor, 0, ""); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on pageSize 0, got %v", err)
	}
	if _, err := repo.ListCommercialReservations(ctx, validActor, 101, ""); !errors.Is(err, rentalflow.ErrInvalid) {
		t.Errorf("expected ErrInvalid on pageSize 101, got %v", err)
	}
}
