package rentalflowpg

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func TestEarlyRentalCreateFingerprintMatchesVersionedPreimage(t *testing.T) {
	spaceID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	startAt := time.Date(2030, 4, 5, 6, 7, 8, 123456789, time.FixedZone("offset", 2*60*60))
	endAt := time.Date(2030, 4, 7, 6, 7, 8, 987654321, time.UTC)
	// Independent protocol vector: do not reconstruct the production formatting.
	preimage := "espacigo.early-rental.create-request.v1\x00" +
		"123e4567-e89b-12d3-a456-426614174000\x00" +
		"2030-04-05T04:07:08.123456789Z\x00" +
		"2030-04-07T06:07:08.987654321Z"
	want := sha256.Sum256([]byte(preimage))

	got, err := earlyRentalCreateFingerprint(spaceID, startAt, endAt)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("fingerprint=%s, want %s (preimage %s)", hex.EncodeToString(got[:]), hex.EncodeToString(want[:]), preimage)
	}
}

func TestEarlyRentalCreateFingerprintNormalizesEquivalentOffsets(t *testing.T) {
	spaceID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	start := time.Date(2030, 4, 5, 6, 7, 8, 123456789, time.UTC)
	end := start.Add(48 * time.Hour)
	got, err := earlyRentalCreateFingerprint(spaceID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	gotOffset, err := earlyRentalCreateFingerprint(spaceID, start.In(time.FixedZone("offset", -5*60*60)), end.In(time.FixedZone("offset", -5*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if got != gotOffset {
		t.Fatal("equivalent instants with different offsets produced different fingerprints")
	}
}

func TestEarlyRentalCreateFingerprintChangesForEachField(t *testing.T) {
	spaceID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	start := time.Date(2030, 4, 5, 6, 7, 8, 123456789, time.UTC)
	end := start.Add(48 * time.Hour)
	base, err := earlyRentalCreateFingerprint(spaceID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		id         uuid.UUID
		start, end time.Time
	}{
		{"space", uuid.MustParse("123e4567-e89b-12d3-a456-426614174001"), start, end},
		{"start", spaceID, start.Add(time.Nanosecond), end},
		{"end", spaceID, start, end.Add(time.Nanosecond)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := earlyRentalCreateFingerprint(tc.id, tc.start, tc.end)
			if err != nil {
				t.Fatal(err)
			}
			if got == base {
				t.Fatal("changed request field did not change fingerprint")
			}
		})
	}
}

func TestEarlyRentalCreateFingerprintPreservesNanoseconds(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	start := time.Date(2030, 4, 5, 6, 7, 8, 123456789, time.UTC)
	end := start.Add(48 * time.Hour)
	a, err := earlyRentalCreateFingerprint(id, start, end)
	if err != nil {
		t.Fatal(err)
	}
	b, err := earlyRentalCreateFingerprint(id, start.Add(time.Nanosecond), end)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("nanosecond distinction was lost")
	}
}

func TestEarlyRentalCreateFingerprintRejectsInvalidInputs(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	start := time.Date(2030, 4, 5, 6, 7, 8, 0, time.UTC)
	validEnd := start.Add(48 * time.Hour)
	cases := []struct {
		name       string
		id         uuid.UUID
		start, end time.Time
	}{
		{"nil space", uuid.Nil, start, validEnd},
		{"zero start", id, time.Time{}, validEnd},
		{"zero end", id, start, time.Time{}},
		{"equal interval", id, start, start},
		{"reversed interval", id, validEnd, start},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := earlyRentalCreateFingerprint(tc.id, tc.start, tc.end)
			if !errors.Is(err, earlyrental.ErrInvalid) {
				t.Fatalf("error=%v, want ErrInvalid", err)
			}
			if got != ([32]byte{}) {
				t.Fatalf("invalid request returned nonzero digest: %x", got)
			}
		})
	}
}
