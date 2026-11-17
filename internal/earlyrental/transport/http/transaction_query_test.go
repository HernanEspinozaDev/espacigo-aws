package earlyrentalhttp

import (
	"net/url"
	"strings"
	"testing"
)

func TestDecodeEarlyRentalTransactionListQueryDefaultsAndPreservesFilters(t *testing.T) {
	got, err := DecodeEarlyRentalTransactionListQuery(url.Values{})
	if err != nil {
		t.Fatalf("DecodeEarlyRentalTransactionListQuery(empty) error = %v", err)
	}
	if got.Role != nil || got.State != nil || got.Cursor != nil || got.Limit != 25 {
		t.Fatalf("default transaction query = %+v", got)
	}

	values := url.Values{"role": {"arrendatario"}, "state": {"recibido"}, "cursor": {"opaque/á?cursor"}, "limit": {"100"}}
	got, err = DecodeEarlyRentalTransactionListQuery(values)
	if err != nil {
		t.Fatalf("DecodeEarlyRentalTransactionListQuery(valid) error = %v", err)
	}
	if got.Role == nil || *got.Role != "arrendatario" || got.State == nil || *got.State != "recibido" ||
		got.Cursor == nil || *got.Cursor != "opaque/á?cursor" || got.Limit != 100 {
		t.Fatalf("decoded transaction query = %+v", got)
	}
}

func TestDecodeEarlyRentalTransactionListQueryRejectsMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
	}{
		{name: "unknown", values: url.Values{"other": {"x"}}},
		{name: "duplicate role", values: url.Values{"role": {"arrendatario", "arrendador"}}},
		{name: "invalid role", values: url.Values{"role": {"admin"}}},
		{name: "duplicate state", values: url.Values{"state": {"enviado", "recibido"}}},
		{name: "invalid state", values: url.Values{"state": {"pagado"}}},
		{name: "empty cursor", values: url.Values{"cursor": {""}}},
		{name: "oversized cursor", values: url.Values{"cursor": {strings.Repeat("x", 2049)}}},
		{name: "duplicate limit", values: url.Values{"limit": {"10", "20"}}},
		{name: "zero limit", values: url.Values{"limit": {"0"}}},
		{name: "limit over maximum", values: url.Values{"limit": {"101"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeEarlyRentalTransactionListQuery(tc.values); err == nil {
				t.Fatal("DecodeEarlyRentalTransactionListQuery() error = nil, want invalid query error")
			}
		})
	}
}
