package earlyrentalhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

const (
	earlyRentalTestContractText   = "EspaciGo contract v1"
	earlyRentalTestContractDigest = "23d81455dcee9a65cc87cdee1171a3e07429c4cb9d9461423200a4d37266b1d1"
	earlyRentalTestOfferID        = "33333333-3333-4333-8333-333333333333"
	earlyRentalTestHostID         = "11111111-1111-4111-8111-111111111111"
	earlyRentalTestRenterID       = "22222222-2222-4222-8222-222222222222"
)

func earlyRentalEventProjectionFixture() (earlyrental.Offer, []earlyrental.Event) {
	createdAt := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	offer := earlyrental.Offer{HostID: "private-host-account", RenterID: "private-renter-account", Revision: 7}
	paymentID := "550e8400-e29b-41d4-a716-446655440000"
	events := []earlyrental.Event{
		{Kind: earlyrental.EventOfferCreated, ActorID: offer.RenterID, Sequence: 1, Revision: 1, At: createdAt, NextState: earlyrental.StatePendingIdentity},
		{Kind: earlyrental.EventIdentityConfirmed, ActorID: offer.RenterID, Sequence: 2, Revision: 2, At: createdAt.Add(time.Minute), PreviousState: earlyrental.StatePendingIdentity, NextState: earlyrental.StatePendingIdentity},
		{Kind: earlyrental.EventIdentityConfirmed, ActorID: offer.HostID, Sequence: 3, Revision: 3, At: createdAt.Add(2 * time.Minute), PreviousState: earlyrental.StatePendingIdentity, NextState: earlyrental.StatePendingContract},
		{Kind: earlyrental.EventContractAccepted, ActorID: offer.RenterID, Sequence: 4, Revision: 4, At: createdAt.Add(3 * time.Minute), PreviousState: earlyrental.StatePendingContract, NextState: earlyrental.StatePendingContract, ContractVersion: 2, ContractDigest: "private-contract-digest"},
		{Kind: earlyrental.EventContractAccepted, ActorID: offer.HostID, Sequence: 5, Revision: 5, At: createdAt.Add(4 * time.Minute), PreviousState: earlyrental.StatePendingContract, NextState: earlyrental.StatePendingPaymentSend, ContractVersion: 2, ContractDigest: "private-contract-digest"},
		{Kind: earlyrental.EventPaymentMarkedSent, ActorID: offer.RenterID, Sequence: 6, Revision: 6, At: createdAt.Add(5 * time.Minute), PreviousState: earlyrental.StatePendingPaymentSend, NextState: earlyrental.StatePendingPaymentReceive, PaymentID: &paymentID},
		{Kind: earlyrental.EventPaymentMarkedReceived, ActorID: offer.HostID, Sequence: 7, Revision: 7, At: createdAt.Add(6 * time.Minute), PreviousState: earlyrental.StatePendingPaymentReceive, NextState: earlyrental.StateConfirmed, PaymentID: &paymentID},
	}
	return offer, events
}

func TestProjectEarlyRentalEventsEmitsSafeOpenAPIShape(t *testing.T) {
	offer, events := earlyRentalEventProjectionFixture()
	projected, err := projectEarlyRentalEvents(offer, events, 0)
	if err != nil {
		t.Fatalf("projectEarlyRentalEvents() error = %v", err)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(body)
	for _, privateValue := range []string{offer.HostID, offer.RenterID, "private-contract-digest"} {
		if strings.Contains(wire, privateValue) {
			t.Fatalf("private value leaked in event projection: %s", wire)
		}
	}

	var decoded struct {
		Data struct {
			Items []struct {
				Sequence        int64     `json:"sequence"`
				Kind            string    `json:"kind"`
				ActorRole       string    `json:"actor_role"`
				Revision        int64     `json:"revision"`
				OccurredAt      time.Time `json:"occurred_at"`
				ContractVersion *int64    `json:"contract_version"`
				PaymentID       *string   `json:"payment_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data.Items) != len(events) {
		t.Fatalf("projected item count = %d, want %d", len(decoded.Data.Items), len(events))
	}
	created := decoded.Data.Items[0]
	if created.Sequence != 1 || created.Revision != 1 || created.Kind != "oferta_creada" || created.ActorRole != "arrendatario" || created.ContractVersion != nil || created.PaymentID != nil {
		t.Fatalf("creation event wire = %+v", created)
	}
	accepted := decoded.Data.Items[3]
	if accepted.ContractVersion == nil || *accepted.ContractVersion != 2 || accepted.ActorRole != "arrendatario" {
		t.Fatalf("contract event wire = %+v", accepted)
	}
	sent, received := decoded.Data.Items[5], decoded.Data.Items[6]
	if sent.PaymentID == nil || received.PaymentID == nil || *sent.PaymentID != *received.PaymentID || sent.ActorRole != "arrendatario" || received.ActorRole != "arrendador" {
		t.Fatalf("payment events do not share one safe movement id: sent=%+v received=%+v", sent, received)
	}
	var first map[string]json.RawMessage
	if err := json.Unmarshal(mustJSONMarshal(t, created), &first); err != nil {
		t.Fatal(err)
	}
	if len(first) != 7 {
		t.Fatalf("event wire keys = %v, want exactly 7 contract fields", first)
	}
}

func TestProjectEarlyRentalEventsMapsSystemExpiryAndEmptyPage(t *testing.T) {
	offer, _ := earlyRentalEventProjectionFixture()
	offer.Revision = 8
	projected, err := projectEarlyRentalEvents(offer, []earlyrental.Event{{
		Kind: earlyrental.EventOfferExpired, ActorID: "system", Sequence: 8, Revision: 8,
		At:            time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		PreviousState: earlyrental.StatePendingPaymentReceive, NextState: earlyrental.StateExpired,
	}}, 7)
	if err != nil {
		t.Fatalf("project expiry event: %v", err)
	}
	if len(projected.Data.Items) != 1 || projected.Data.Items[0].ActorRole != "sistema" {
		t.Fatalf("projected expiry = %+v", projected.Data.Items)
	}
	empty, err := projectEarlyRentalEvents(offer, nil, 8)
	if err != nil {
		t.Fatalf("project empty page: %v", err)
	}
	body, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"data":{"items":[]}}` {
		t.Fatalf("empty event page = %s, want a non-null empty items array", body)
	}
}

func TestDomainPaymentLifecycleProjectsOneSharedMovement(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	offer, createdEvent, err := earlyrental.NewOffer(earlyrental.NewOfferInput{
		ID: earlyRentalTestOfferID, HostID: earlyRentalTestHostID, RenterID: earlyRentalTestRenterID,
		Snapshot: earlyrental.OfferSnapshotInput{
			SpaceID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), SpaceTitle: "Espacio de prueba",
			RateUnit: "hora", UnitPriceCLP: 12_000, Units: 1,
			StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour),
			TimeZone: "UTC", CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent,
		},
		ContractVersion: 1, ContractText: earlyRentalTestContractText,
	}, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	events := []earlyrental.Event{createdEvent}
	apply := func(actor string, command earlyrental.Command, at time.Time) {
		t.Helper()
		command.ExpectedRevision = offer.Revision
		updated, event, err := earlyrental.Apply(offer, actor, command, at)
		if err != nil {
			t.Fatalf("Apply(%q) error = %v", command.Kind, err)
		}
		offer = updated
		events = append(events, event)
	}
	apply(earlyRentalTestRenterID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(time.Minute))
	apply(earlyRentalTestHostID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(2*time.Minute))
	apply(earlyRentalTestRenterID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: 1, ContractDigest: earlyRentalTestContractDigest}, now.Add(3*time.Minute))
	apply(earlyRentalTestHostID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: 1, ContractDigest: earlyRentalTestContractDigest}, now.Add(4*time.Minute))
	apply(earlyRentalTestRenterID, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: "550e8400-e29b-41d4-a716-446655440000"}, now.Add(5*time.Minute))
	apply(earlyRentalTestHostID, earlyrental.Command{Kind: earlyrental.CommandReceivePayment}, now.Add(6*time.Minute))

	projected, err := projectEarlyRentalEvents(offer, events, 0)
	if err != nil {
		t.Fatalf("project domain events: %v", err)
	}
	items := projected.Data.Items
	if len(items) != 7 || items[5].PaymentID == nil || items[6].PaymentID == nil || *items[5].PaymentID != *items[6].PaymentID {
		t.Fatalf("projected domain movement = %+v", items)
	}
	if items[5].ActorRole != "arrendatario" || items[6].ActorRole != "arrendador" || items[5].Sequence >= items[6].Sequence {
		t.Fatalf("projected movement perspectives/order = sent %+v received %+v", items[5], items[6])
	}
}

func TestProjectEarlyRentalEventsAcceptsContiguousCursorPage(t *testing.T) {
	offer, events := earlyRentalEventProjectionFixture()
	page := events[5:]
	projected, err := projectEarlyRentalEvents(offer, page, 5)
	if err != nil {
		t.Fatalf("project cursor page beginning after creation: %v", err)
	}
	if len(projected.Data.Items) != len(page) || projected.Data.Items[0].Sequence != 6 || projected.Data.Items[1].Sequence != 7 {
		t.Fatalf("cursor page projection = %+v", projected.Data.Items)
	}
}

func TestProjectEarlyRentalEventsRequiresCompleteRangeAfterCursor(t *testing.T) {
	offer, events := earlyRentalEventProjectionFixture()
	if _, err := projectEarlyRentalEvents(offer, events[5:], 4); err == nil {
		t.Fatal("cursor gap before first returned event was accepted")
	}
	if _, err := projectEarlyRentalEvents(offer, events[5:6], 5); err == nil {
		t.Fatal("incomplete tail before current revision was accepted")
	}
	if _, err := projectEarlyRentalEvents(offer, nil, 5); err == nil {
		t.Fatal("empty page before current revision was accepted")
	}
	if _, err := projectEarlyRentalEvents(offer, events[:1], 7); err == nil {
		t.Fatal("events at or before the cursor were accepted")
	}
}

func TestProjectEarlyRentalEventsRejectsInconsistentOrUnmappableEvents(t *testing.T) {
	offer, base := earlyRentalEventProjectionFixture()
	cases := []struct {
		name   string
		mutate func([]earlyrental.Event) []earlyrental.Event
	}{
		{name: "unordered sequence", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[1], events[2] = events[2], events[1]
			return events
		}},
		{name: "missing state transition", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[3].PreviousState = earlyrental.StatePendingIdentity
			return events
		}},
		{name: "disconnected state chain", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[4].PreviousState = earlyrental.StatePendingPaymentSend
			return events
		}},
		{name: "nonconsecutive sequence and revision", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			for index := 3; index < len(events); index++ {
				events[index].Sequence++
				events[index].Revision++
			}
			return events
		}},
		{name: "unknown actor", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[0].ActorID = "unrelated-account"
			return events
		}},
		{name: "created by host", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[0].ActorID = offer.HostID; return events }},
		{name: "payment without movement id", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[5].PaymentID = nil; return events }},
		{name: "malformed movement id", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			invalidID := "not-a-uuid"
			events[5].PaymentID = &invalidID
			return events
		}},
		{name: "nil movement id", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			invalidID := uuid.Nil.String()
			events[5].PaymentID = &invalidID
			events[6].PaymentID = &invalidID
			return events
		}},
		{name: "different movement ids", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			otherID := "550e8400-e29b-41d4-a716-446655440001"
			events[6].PaymentID = &otherID
			return events
		}},
		{name: "payment sent by host", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[5].ActorID = offer.HostID; return events }},
		{name: "payment received by renter", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[6].ActorID = offer.RenterID
			return events
		}},
		{name: "contract event without version", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[3].ContractVersion = 0; return events }},
		{name: "contract version on unrelated event", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[0].ContractVersion = 2; return events }},
		{name: "unknown event kind", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[0].Kind = earlyrental.EventKind("unknown")
			return events
		}},
		{name: "system actor on participant event", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[0].ActorID = "system"
			return events
		}},
		{name: "expiry attributed to participant", mutate: func(events []earlyrental.Event) []earlyrental.Event {
			events[0].Kind = earlyrental.EventOfferExpired
			return events
		}},
		{name: "zero timestamp", mutate: func(events []earlyrental.Event) []earlyrental.Event { events[0].At = time.Time{}; return events }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := append([]earlyrental.Event(nil), base...)
			if _, err := projectEarlyRentalEvents(offer, tc.mutate(events), 0); err == nil {
				t.Fatal("projectEarlyRentalEvents() error = nil, want validation failure")
			}
		})
	}
}

func mustJSONMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
