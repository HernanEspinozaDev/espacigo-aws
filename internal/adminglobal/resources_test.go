package adminglobal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAccountResourceQueriesCoverTrialAndCommercialKinds(t *testing.T) {
	validKinds := []struct {
		kind       string
		wantOrigin string
	}{
		{"reservations", "'trial'::text AS data_origin"},
		{"incidents", "'trial'::text AS data_origin"},
		{"commercial_reservations", "'commercial'::text AS data_origin"},
		{"commercial_contracts", "'commercial'::text AS data_origin"},
		{"commercial_payments", "'commercial'::text AS data_origin"},
	}

	for _, tc := range validKinds {
		q, ok := accountResourceQuery(tc.kind)
		if !ok {
			t.Fatalf("accountResourceQuery(%q) returned false", tc.kind)
		}
		if !strings.Contains(q, tc.wantOrigin) {
			t.Fatalf("accountResourceQuery(%q) missing %s", tc.kind, tc.wantOrigin)
		}
		dq, ok := accountResourceDetailQuery(tc.kind)
		if !ok || dq == "" {
			t.Fatalf("accountResourceDetailQuery(%q) returned false or empty", tc.kind)
		}
	}

	for _, invalid := range []string{"", "unknown", "commercial_other"} {
		if _, ok := accountResourceQuery(invalid); ok {
			t.Fatalf("accountResourceQuery(%q) unexpectedly succeeded", invalid)
		}
		if _, ok := accountResourceDetailQuery(invalid); ok {
			t.Fatalf("accountResourceDetailQuery(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestAccountResourceJSONOmitsEmptyCommercialFields(t *testing.T) {
	trial := AccountResource{
		ID:          "00000000-0000-4000-8000-000000000001",
		Kind:        "reservation",
		DataOrigin:  "trial",
		SpaceID:     "00000000-0000-4000-8000-000000000002",
		State:       "pagada",
		Participant: "arrendatario",
		OccurredAt:  time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(trial)
	if err != nil {
		t.Fatalf("marshal trial resource: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal trial resource: %v", err)
	}
	if m["data_origin"] != "trial" {
		t.Fatalf("expected data_origin=trial, got %v", m["data_origin"])
	}
	for _, omitted := range []string{"test_mode", "role_in_record", "amount_clp", "total_charged_clp", "currency", "space_title", "provider_payment_id", "provider_status", "document_status", "related_ids"} {
		if _, exists := m[omitted]; exists {
			t.Fatalf("expected %q to be omitted for trial resource", omitted)
		}
	}
}
