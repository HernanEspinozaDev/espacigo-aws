package earlyrentalhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectEarlyRentalListEmitsParticipantSafeOpenAPIItems(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	cursor := "opaque-next-cursor"
	sources := []earlyRentalSummarySource{{Offer: offer, CreatedAt: createdAt}}

	for _, tc := range []struct {
		actor string
		role  string
	}{
		{actor: offer.RenterID, role: "arrendatario"},
		{actor: offer.HostID, role: "arrendador"},
	} {
		projected, err := projectEarlyRentalList(sources, tc.actor, &cursor)
		if err != nil {
			t.Fatalf("projectEarlyRentalList(%s) error = %v", tc.role, err)
		}
		body, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		wire := string(body)
		if strings.Contains(wire, offer.HostID) || strings.Contains(wire, offer.RenterID) {
			t.Fatalf("participant account ID leaked in list: %s", wire)
		}
		var decoded struct {
			Data struct {
				Items []struct {
					ID         string    `json:"id"`
					ActorRole  string    `json:"actor_role"`
					SpaceID    string    `json:"space_id"`
					SpaceTitle string    `json:"space_title"`
					StartAt    time.Time `json:"start_at"`
					EndAt      time.Time `json:"end_at"`
					Subtotal   int64     `json:"subtotal_clp"`
					Currency   string    `json:"currency"`
					State      string    `json:"state"`
					Revision   int64     `json:"revision"`
					CreatedAt  time.Time `json:"created_at"`
					ExpiresAt  time.Time `json:"expires_at"`
				} `json:"items"`
				NextCursor *string `json:"next_cursor"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.Data.Items) != 1 || decoded.Data.NextCursor == nil || *decoded.Data.NextCursor != cursor {
			t.Fatalf("list envelope = %+v", decoded.Data)
		}
		item := decoded.Data.Items[0]
		if item.ID != offer.ID || item.ActorRole != tc.role || item.SpaceID != offer.Snapshot.SpaceID.String() ||
			item.SpaceTitle != offer.Snapshot.SpaceTitle || !item.StartAt.Equal(offer.Snapshot.StartAt) || !item.EndAt.Equal(offer.Snapshot.EndAt) ||
			item.Subtotal != offer.Snapshot.SubtotalCLP || item.Currency != "CLP" || item.State != string(offer.State) ||
			item.Revision != 1 || !item.CreatedAt.Equal(createdAt) || !item.ExpiresAt.Equal(offer.ExpiresAt) {
			t.Fatalf("summary item = %+v", item)
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(body, &root); err != nil {
			t.Fatal(err)
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(root["data"], &data); err != nil {
			t.Fatal(err)
		}
		if len(data) != 2 {
			t.Fatalf("list data keys = %v, want exactly items and next_cursor", data)
		}
		var wireItems []json.RawMessage
		if err := json.Unmarshal(data["items"], &wireItems); err != nil {
			t.Fatal(err)
		}
		var itemKeys map[string]json.RawMessage
		if err := json.Unmarshal(wireItems[0], &itemKeys); err != nil {
			t.Fatal(err)
		}
		if len(itemKeys) != 12 {
			t.Fatalf("summary has %d keys, want exactly 12 OpenAPI fields", len(itemKeys))
		}
	}
}

func TestProjectEarlyRentalListEmitsEmptyArrayAndNullCursor(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	projected, err := projectEarlyRentalList(nil, offer.RenterID, nil)
	if err != nil {
		t.Fatalf("project empty list: %v", err)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"data":{"items":[],"next_cursor":null}}` {
		t.Fatalf("empty list wire = %s", body)
	}
}

func TestProjectEarlyRentalListRejectsInvalidActorMetadataAndCursor(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	source := []earlyRentalSummarySource{{Offer: offer, CreatedAt: createdAt}}
	if _, err := projectEarlyRentalList(source, "33333333-3333-4333-8333-333333333333", nil); err == nil {
		t.Fatal("non-participant list projection unexpectedly succeeded")
	}
	if _, err := projectEarlyRentalList([]earlyRentalSummarySource{{Offer: offer}}, offer.RenterID, nil); err == nil {
		t.Fatal("missing creation timestamp unexpectedly succeeded")
	}
	if _, err := projectEarlyRentalList([]earlyRentalSummarySource{{Offer: offer, CreatedAt: offer.ExpiresAt}}, offer.RenterID, nil); err == nil {
		t.Fatal("creation timestamp at expiry unexpectedly succeeded")
	}
	longCursor := strings.Repeat("x", 2049)
	if _, err := projectEarlyRentalList(source, offer.RenterID, &longCursor); err == nil {
		t.Fatal("oversized next cursor unexpectedly succeeded")
	}
	invalidCursor := string([]byte{0xff})
	if _, err := projectEarlyRentalList(source, offer.RenterID, &invalidCursor); err == nil {
		t.Fatal("invalid UTF-8 next cursor unexpectedly succeeded")
	}
}
