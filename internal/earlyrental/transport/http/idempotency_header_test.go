package earlyrentalhttp

import (
	"net/http"
	"strings"
	"testing"
)

func TestDecodeIdempotencyKeyPreservesValidOpaqueValue(t *testing.T) {
	const key = "ábcdefgh"
	header := make(http.Header)
	header.Set("Idempotency-Key", key)

	got, err := DecodeIdempotencyKey(header)
	if err != nil {
		t.Fatalf("DecodeIdempotencyKey() error = %v", err)
	}
	if got != key {
		t.Fatalf("DecodeIdempotencyKey() = %q, want exact key %q", got, key)
	}
}

func TestDecodeIdempotencyKeyAcceptsUnicodeLengthBoundaries(t *testing.T) {
	for _, key := range []string{strings.Repeat("á", 8), strings.Repeat("界", 200)} {
		header := make(http.Header)
		header.Set("Idempotency-Key", key)
		if got, err := DecodeIdempotencyKey(header); err != nil || got != key {
			t.Errorf("DecodeIdempotencyKey() for %d runes = %q, %v", len([]rune(key)), got, err)
		}
	}
}

func TestDecodeIdempotencyKeyRejectsMissingDuplicateMalformedAndOutOfRangeValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "duplicate", values: []string{"abcdefgh", "ijklmnop"}},
		{name: "too short", values: []string{"abcdefg"}},
		{name: "too long", values: []string{strings.Repeat("a", 201)}},
		{name: "invalid UTF-8", values: []string{string([]byte{0xff, 'a', 'b', 'c', 'd', 'e', 'f', 'g'})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := make(http.Header)
			for _, value := range tc.values {
				header.Add("Idempotency-Key", value)
			}
			if got, err := DecodeIdempotencyKey(header); err == nil || got != "" {
				t.Fatalf("DecodeIdempotencyKey() = %q, %v; want empty value and error", got, err)
			}
		})
	}
}
