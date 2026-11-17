package earlyrentalhttp

import (
	"net/url"
	"strings"
	"testing"
)

func TestDecodeEarlyRentalListQueryDefaultsAndPreservesFilters(t *testing.T) {
	got, err := DecodeEarlyRentalListQuery(url.Values{})
	if err != nil {
		t.Fatalf("DecodeEarlyRentalListQuery(empty) error = %v", err)
	}
	if got.Role != nil || got.Cursor != nil || got.Limit != 25 {
		t.Fatalf("default list query = %+v, want nil filters and limit 25", got)
	}

	values := url.Values{
		"role":   {"arrendador"},
		"cursor": {"opaque/á?cursor"},
		"limit":  {"100"},
	}
	got, err = DecodeEarlyRentalListQuery(values)
	if err != nil {
		t.Fatalf("DecodeEarlyRentalListQuery(valid) error = %v", err)
	}
	if got.Role == nil || *got.Role != "arrendador" || got.Cursor == nil || *got.Cursor != "opaque/á?cursor" || got.Limit != 100 {
		t.Fatalf("decoded list query = %+v", got)
	}
	boundaryCursor := strings.Repeat("界", 2048)
	got, err = DecodeEarlyRentalListQuery(url.Values{"cursor": {boundaryCursor}, "limit": {"1"}})
	if err != nil || got.Cursor == nil || *got.Cursor != boundaryCursor || got.Limit != 1 {
		t.Fatalf("minimum limit and maximum Unicode cursor = %+v, %v", got, err)
	}
}

func TestDecodeEarlyRentalListQueryRejectsMalformedAndOutOfContractValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
	}{
		{name: "unknown", values: url.Values{"other": {"x"}}},
		{name: "duplicate role", values: url.Values{"role": {"arrendatario", "arrendador"}}},
		{name: "invalid role", values: url.Values{"role": {"admin"}}},
		{name: "empty cursor", values: url.Values{"cursor": {""}}},
		{name: "oversized cursor", values: url.Values{"cursor": {strings.Repeat("x", 2049)}}},
		{name: "invalid UTF-8 cursor", values: url.Values{"cursor": {string([]byte{0xff})}}},
		{name: "zero limit", values: url.Values{"limit": {"0"}}},
		{name: "limit above maximum", values: url.Values{"limit": {"101"}}},
		{name: "negative limit", values: url.Values{"limit": {"-1"}}},
		{name: "invalid limit", values: url.Values{"limit": {"many"}}},
		{name: "duplicate limit", values: url.Values{"limit": {"10", "20"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeEarlyRentalListQuery(tc.values); err == nil {
				t.Fatal("DecodeEarlyRentalListQuery() error = nil, want invalid query error")
			}
		})
	}
}
