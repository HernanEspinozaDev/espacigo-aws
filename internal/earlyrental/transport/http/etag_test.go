package earlyrentalhttp

import (
	"encoding/base64"
	"math"
	"testing"
)

const (
	earlyRentalETagID  = "11111111-1111-4111-8111-111111111111"
	earlyRentalOtherID = "22222222-2222-4222-8222-222222222222"
)

func strongTestTag(value string) string {
	quote := string([]byte{34})
	return quote + value + quote
}

func TestEarlyRentalETagRoundTripBindsResourceAndRevision(t *testing.T) {
	const revision = uint64(42)
	tag, err := FormatEarlyRentalETag(earlyRentalETagID, revision)
	if err != nil {
		t.Fatalf("FormatEarlyRentalETag() error = %v", err)
	}
	got, err := ParseIfMatchRevision([]string{tag}, earlyRentalETagID)
	if err != nil {
		t.Fatalf("ParseIfMatchRevision() error = %v", err)
	}
	if got != revision {
		t.Fatalf("ParseIfMatchRevision() = %d, want %d", got, revision)
	}
	if _, err := ParseIfMatchRevision([]string{tag}, earlyRentalOtherID); err != ErrStaleIfMatch {
		t.Fatalf("wrong-resource error = %v, want %v", err, ErrStaleIfMatch)
	}
}

func TestEarlyRentalETagSupportsMaximumRevision(t *testing.T) {
	tag, err := FormatEarlyRentalETag(earlyRentalETagID, math.MaxUint64)
	if err != nil {
		t.Fatalf("FormatEarlyRentalETag() error = %v", err)
	}
	got, err := ParseIfMatchRevision([]string{tag}, earlyRentalETagID)
	if err != nil || got != math.MaxUint64 {
		t.Fatalf("ParseIfMatchRevision() = %d, %v; want %d", got, err, uint64(math.MaxUint64))
	}
}

func TestEarlyRentalETagRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		rev  uint64
	}{
		{name: "noncanonical id", id: "11111111111141118111111111111111", rev: 1},
		{name: "nil id", id: "00000000-0000-0000-0000-000000000000", rev: 1},
		{name: "zero revision", id: earlyRentalETagID, rev: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FormatEarlyRentalETag(tc.id, tc.rev); err != ErrInvalidIfMatch {
				t.Fatalf("FormatEarlyRentalETag() error = %v, want %v", err, ErrInvalidIfMatch)
			}
		})
	}
}

func TestParseIfMatchRevisionRejectsMissingDuplicateAndMalformedTags(t *testing.T) {
	encoded := func(payload string) string {
		return strongTestTag(base64.RawURLEncoding.EncodeToString([]byte(payload)))
	}
	for _, tc := range []struct {
		name   string
		values []string
		want   error
	}{
		{name: "missing", want: ErrMissingIfMatch},
		{name: "duplicate", values: []string{strongTestTag("tag-a"), strongTestTag("tag-b")}, want: ErrInvalidIfMatch},
		{name: "weak", values: []string{"W/" + strongTestTag("tag")}, want: ErrInvalidIfMatch},
		{name: "wildcard", values: []string{"*"}, want: ErrInvalidIfMatch},
		{name: "unquoted", values: []string{"tag"}, want: ErrInvalidIfMatch},
		{name: "wrong resource", values: []string{encoded(earlyRentalOtherID + "\x001")}, want: ErrStaleIfMatch},
		{name: "zero revision", values: []string{encoded(earlyRentalETagID + "\x000")}, want: ErrInvalidIfMatch},
		{name: "leading zero", values: []string{encoded(earlyRentalETagID + "\x0001")}, want: ErrInvalidIfMatch},
		{name: "noncanonical base64", values: []string{strongTestTag("AB")}, want: ErrInvalidIfMatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseIfMatchRevision(tc.values, earlyRentalETagID)
			if err != tc.want {
				t.Fatalf("ParseIfMatchRevision() error = %v, want %v", err, tc.want)
			}
		})
	}
}
