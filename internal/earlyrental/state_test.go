package earlyrental

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	hostID             = "11111111-1111-4111-8111-111111111111"
	renterID           = "22222222-2222-4222-8222-222222222222"
	testOfferID        = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testSpaceID        = "550e8400-e29b-41d4-a716-446655440001"
	testPaymentID      = "550e8400-e29b-41d4-a716-446655440000"
	testContractText   = "EspaciGo contract v1"
	testContractDigest = "23d81455dcee9a65cc87cdee1171a3e07429c4cb9d9461423200a4d37266b1d1"
)

func newOffer() Offer {
	return Offer{
		ID: testOfferID, HostID: hostID, RenterID: renterID, State: StatePendingIdentity,
		Snapshot:        testOfferSnapshot(),
		ContractVersion: 1, ContractText: testContractText, ContractDigest: testContractDigest,
		Revision: 1, ExpiresAt: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
	}
}

func TestValidateOfferAcceptsValidAggregateAndRejectsImpossibleSnapshot(t *testing.T) {
	offer := newOffer()
	if err := ValidateOffer(offer); err != nil {
		t.Fatalf("ValidateOffer(valid) error = %v", err)
	}

	invalid := newOffer()
	invalid.Snapshot.SubtotalCLP++
	if err := ValidateOffer(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateOffer(invalid snapshot) error = %v, want %v", err, ErrInvalid)
	}
}

func validNewOfferInput(intervalStart time.Time) NewOfferInput {
	return NewOfferInput{
		ID: testOfferID, HostID: hostID, RenterID: renterID,
		Snapshot:        testOfferSnapshotInput(intervalStart),
		ContractVersion: 1, ContractText: testContractText,
	}
}

func testOfferSnapshotInput(startAt time.Time) OfferSnapshotInput {
	return OfferSnapshotInput{
		SpaceID: uuid.MustParse(testSpaceID), SpaceTitle: "Espacio de prueba", RateUnit: "hora",
		UnitPriceCLP: 12_000, Units: 1, StartAt: startAt, EndAt: startAt.Add(time.Hour), TimeZone: "UTC",
		CalendarRateOverride: CalendarRateOverrideAbsent,
	}
}

func testOfferSnapshot() OfferSnapshot {
	input := testOfferSnapshotInput(time.Date(2026, 11, 17, 12, 30, 0, 0, time.UTC))
	return OfferSnapshot{
		SpaceID: input.SpaceID, SpaceTitle: input.SpaceTitle, RateUnit: input.RateUnit,
		UnitPriceCLP: input.UnitPriceCLP, Currency: "CLP", Units: input.Units,
		SubtotalCLP: input.UnitPriceCLP * input.Units, StartAt: input.StartAt, EndAt: input.EndAt,
		TimeZone: input.TimeZone,
	}
}

func TestNewOfferInitializesStateAndBoundsHoldExpiry(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.FixedZone("test", -3*60*60))
	input := validNewOfferInput(now.Add(45 * time.Minute))
	offer, creationEvent, err := NewOffer(input, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	if offer.ID != input.ID || offer.HostID != input.HostID || offer.RenterID != input.RenterID ||
		offer.State != StatePendingIdentity || offer.Revision != 1 || offer.ContractVersion != input.ContractVersion ||
		offer.ContractText != input.ContractText || offer.ContractDigest != testContractDigest {
		t.Fatalf("NewOffer() initialized unexpected aggregate: %+v", offer)
	}
	if !offer.ExpiresAt.Equal(now.Add(HoldTTL)) || offer.ExpiresAt.Location() != time.UTC {
		t.Fatalf("NewOffer() expiry = %v, want %v in UTC", offer.ExpiresAt, now.Add(HoldTTL).UTC())
	}
	if len(offer.IdentityConfirmed) != 0 || len(offer.ContractAccepted) != 0 || offer.PaymentSentAt != nil || offer.PaymentReceivedAt != nil {
		t.Fatalf("NewOffer() initialized confirmations or payment evidence: %+v", offer)
	}
	if creationEvent.Kind != EventOfferCreated || creationEvent.ActorID != renterID || creationEvent.Sequence != 1 ||
		creationEvent.Revision != 1 || !creationEvent.At.Equal(now.UTC()) {
		t.Fatalf("NewOffer() creation event = %+v", creationEvent)
	}
	if creationEvent.PreviousState != "" || creationEvent.NextState != StatePendingIdentity {
		t.Fatalf("NewOffer() creation state transition = %q → %q", creationEvent.PreviousState, creationEvent.NextState)
	}
}

func TestNewOfferRejectsUnsupportedCustomRateOverride(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(time.Hour))
	input.Snapshot.CalendarRateOverride = CalendarRateOverridePresent
	if _, _, err := NewOffer(input, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewOffer(custom calendar rate) error = %v, want %v", err, ErrInvalid)
	}
}

func TestNewOfferRejectsUnresolvedCustomRatePolicy(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(time.Hour))
	input.Snapshot.CalendarRateOverride = CalendarRateOverrideUnresolved
	if _, _, err := NewOffer(input, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewOffer(unresolved custom-rate check) error = %v, want %v", err, ErrInvalid)
	}
}

func TestApplyEventRecordsPreviousAndNextState(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	offer := newOffer()
	first, firstEvent, err := Apply(offer, renterID, Command{
		Kind: CommandConfirmIdentity, ExpectedRevision: offer.Revision,
	}, now)
	if err != nil {
		t.Fatalf("first identity confirmation = %v", err)
	}
	if firstEvent.PreviousState != StatePendingIdentity || firstEvent.NextState != StatePendingIdentity {
		t.Fatalf("first event state transition = %q → %q", firstEvent.PreviousState, firstEvent.NextState)
	}
	second, secondEvent, err := Apply(first, hostID, Command{
		Kind: CommandConfirmIdentity, ExpectedRevision: first.Revision,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second identity confirmation = %v", err)
	}
	if secondEvent.PreviousState != StatePendingIdentity || secondEvent.NextState != StatePendingContract || second.State != secondEvent.NextState {
		t.Fatalf("transition event = %q → %q, aggregate state %q", secondEvent.PreviousState, secondEvent.NextState, second.State)
	}
}

func TestNewOfferPersistsServerResolvedSnapshotAndDerivesSubtotal(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(time.Hour))
	input.Snapshot.Units = 2
	input.Snapshot.EndAt = input.Snapshot.StartAt.Add(2 * time.Hour)
	input.Snapshot.StartAt = input.Snapshot.StartAt.In(time.FixedZone("CLST", -3*60*60))
	input.Snapshot.EndAt = input.Snapshot.EndAt.In(time.FixedZone("CLST", -3*60*60))

	offer, _, err := NewOffer(input, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	snapshot := offer.Snapshot
	if snapshot.SpaceID != input.Snapshot.SpaceID || snapshot.SpaceTitle != input.Snapshot.SpaceTitle ||
		snapshot.RateUnit != "hora" || snapshot.UnitPriceCLP != 12_000 || snapshot.Currency != "CLP" ||
		snapshot.Units != 2 || snapshot.SubtotalCLP != 24_000 || snapshot.TimeZone != input.Snapshot.TimeZone {
		t.Fatalf("NewOffer() snapshot = %+v", snapshot)
	}
	if !snapshot.StartAt.Equal(input.Snapshot.StartAt) || !snapshot.EndAt.Equal(input.Snapshot.EndAt) ||
		snapshot.StartAt.Location() != time.UTC || snapshot.EndAt.Location() != time.UTC {
		t.Fatalf("NewOffer() interval = %v–%v; want normalized UTC instants", snapshot.StartAt, snapshot.EndAt)
	}
}

func TestNewOfferAcceptsFourHourSnapshotAtMinimumPrice(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(time.Hour))
	input.Snapshot.UnitPriceCLP = 5_001
	input.Snapshot.Units = 4
	input.Snapshot.EndAt = input.Snapshot.StartAt.Add(4 * time.Hour)

	offer, _, err := NewOffer(input, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	if offer.Snapshot.Units != 4 || offer.Snapshot.SubtotalCLP != 20_004 {
		t.Fatalf("NewOffer() four-hour price snapshot = %+v", offer.Snapshot)
	}
}

func TestNewOfferRejectsInvalidSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	valid := validNewOfferInput(now.Add(time.Hour))
	cases := []struct {
		name   string
		mutate func(*OfferSnapshotInput)
	}{
		{name: "missing space", mutate: func(snapshot *OfferSnapshotInput) { snapshot.SpaceID = uuid.Nil }},
		{name: "empty title", mutate: func(snapshot *OfferSnapshotInput) { snapshot.SpaceTitle = "" }},
		{name: "title too long", mutate: func(snapshot *OfferSnapshotInput) { snapshot.SpaceTitle = strings.Repeat("a", 71) }},
		{name: "unsupported rate unit", mutate: func(snapshot *OfferSnapshotInput) { snapshot.RateUnit = "dia" }},
		{name: "price below OpenAPI minimum", mutate: func(snapshot *OfferSnapshotInput) { snapshot.UnitPriceCLP = 5_000 }},
		{name: "unsupported units", mutate: func(snapshot *OfferSnapshotInput) { snapshot.Units = 3 }},
		{name: "duration differs from billed hours", mutate: func(snapshot *OfferSnapshotInput) { snapshot.Units = 2 }},
		{name: "unknown time zone", mutate: func(snapshot *OfferSnapshotInput) { snapshot.TimeZone = "No/Such_Zone" }},
		{name: "machine local zone is not a publication zone", mutate: func(snapshot *OfferSnapshotInput) { snapshot.TimeZone = "Local" }},
		{name: "subtotal would overflow int64", mutate: func(snapshot *OfferSnapshotInput) {
			snapshot.UnitPriceCLP = math.MaxInt64
			snapshot.Units = 4
			snapshot.EndAt = snapshot.StartAt.Add(4 * time.Hour)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.mutate(&input.Snapshot)
			if offer, event, err := NewOffer(input, now); !errors.Is(err, ErrInvalid) || offer.ID != "" || event.Kind != "" {
				t.Fatalf("NewOffer() = %+v, %+v, %v; want empty result and ErrInvalid", offer, event, err)
			}
		})
	}
}

func TestNewOfferClampsHoldExpiryToIntervalStart(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(10 * time.Minute))
	offer, _, err := NewOffer(input, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	if !offer.ExpiresAt.Equal(input.Snapshot.StartAt) {
		t.Fatalf("NewOffer() expiry = %v, want interval start %v", offer.ExpiresAt, input.Snapshot.StartAt)
	}
}

func TestNewOfferRejectsInvalidCreationFacts(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	valid := validNewOfferInput(now.Add(time.Hour))
	cases := []struct {
		name  string
		input NewOfferInput
		now   time.Time
	}{
		{name: "missing offer id", input: func() NewOfferInput { v := valid; v.ID = ""; return v }(), now: now},
		{name: "malformed offer id", input: func() NewOfferInput { v := valid; v.ID = "not-a-uuid"; return v }(), now: now},
		{name: "uppercase offer id", input: func() NewOfferInput { v := valid; v.ID = strings.ToUpper(testOfferID); return v }(), now: now},
		{name: "nil offer id", input: func() NewOfferInput { v := valid; v.ID = uuid.Nil.String(); return v }(), now: now},
		{name: "malformed host id", input: func() NewOfferInput { v := valid; v.HostID = "host-account"; return v }(), now: now},
		{name: "nil host id", input: func() NewOfferInput { v := valid; v.HostID = uuid.Nil.String(); return v }(), now: now},
		{name: "malformed renter id", input: func() NewOfferInput { v := valid; v.RenterID = "renter-account"; return v }(), now: now},
		{name: "nil renter id", input: func() NewOfferInput { v := valid; v.RenterID = uuid.Nil.String(); return v }(), now: now},
		{name: "same participants", input: func() NewOfferInput { v := valid; v.RenterID = v.HostID; return v }(), now: now},
		{name: "missing contract version", input: func() NewOfferInput { v := valid; v.ContractVersion = 0; return v }(), now: now},
		{name: "missing contract text", input: func() NewOfferInput { v := valid; v.ContractText = ""; return v }(), now: now},
		{name: "invalid UTF-8 contract text", input: func() NewOfferInput { v := valid; v.ContractText = string([]byte{0xff}); return v }(), now: now},
		{name: "zero now", input: valid, now: time.Time{}},
		{name: "zero interval start", input: validNewOfferInput(time.Time{}), now: now},
		{name: "interval already started", input: validNewOfferInput(now), now: now},
		{name: "interval in the past", input: validNewOfferInput(now.Add(-time.Second)), now: now},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if offer, event, err := NewOffer(tc.input, tc.now); !errors.Is(err, ErrInvalid) || offer.ID != "" || event.Kind != "" {
				t.Fatalf("NewOffer() = %+v, %+v, %v; want empty offer/event and ErrInvalid", offer, event, err)
			}
		})
	}
}

func TestNewOfferDerivesDigestFromExactContractText(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	input := validNewOfferInput(now.Add(time.Hour))
	input.ContractText = "EspaciGo contract v1\n"
	offer, _, err := NewOffer(input, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	if offer.ContractText != input.ContractText || offer.ContractDigest != "dd1882977e62f7727a9e472dd8aff944ba2ba3f6e4cb65e1617c3449b2fb5366" {
		t.Fatalf("NewOffer() contract snapshot = text %q digest %q", offer.ContractText, offer.ContractDigest)
	}
}

func offerAtState(state State) Offer {
	offer := newOffer()
	identityAt := offer.ExpiresAt.Add(-2 * time.Hour)
	contractAt := offer.ExpiresAt.Add(-105 * time.Minute)
	sentAt := offer.ExpiresAt.Add(-90 * time.Minute)
	receivedAt := offer.ExpiresAt.Add(-75 * time.Minute)
	switch state {
	case StatePendingContract, StatePendingPaymentSend, StatePendingPaymentReceive, StateConfirmed:
		offer.IdentityConfirmed = map[string]time.Time{hostID: identityAt, renterID: identityAt}
	}
	switch state {
	case StatePendingPaymentSend, StatePendingPaymentReceive, StateConfirmed:
		offer.ContractAccepted = map[string]time.Time{hostID: contractAt, renterID: contractAt}
	}
	switch state {
	case StatePendingPaymentReceive, StateConfirmed:
		offer.PaymentSentAt = &sentAt
		offer.PaymentID = testPaymentID
	}
	if state == StateConfirmed {
		offer.PaymentReceivedAt = &receivedAt
	}
	offer.State = state
	return offer
}

func TestPaymentMovementIDIsCreatedWithSendAndReusedOnReceipt(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	offer := offerAtState(StatePendingPaymentSend)
	sent, sentEvent, err := Apply(offer, renterID, Command{
		Kind: CommandSendPayment, ExpectedRevision: offer.Revision, PaymentID: testPaymentID,
	}, now)
	if err != nil {
		t.Fatalf("send payment = %v", err)
	}
	if sent.PaymentID != testPaymentID || sentEvent.PaymentID == nil || *sentEvent.PaymentID != testPaymentID {
		t.Fatalf("sent offer/event movement ID mismatch: offer=%+v event=%+v", sent, sentEvent)
	}
	otherPaymentID := "550e8400-e29b-41d4-a716-446655440001"
	if updated, event, err := Apply(sent, hostID, Command{
		Kind: CommandReceivePayment, ExpectedRevision: sent.Revision, PaymentID: otherPaymentID,
	}, now.Add(time.Minute)); !errors.Is(err, ErrInvalid) || updated.Revision != 0 || event.Sequence != 0 || sent.PaymentID != testPaymentID {
		t.Fatalf("receive with replacement movement ID = offer %+v event %+v err %v", updated, event, err)
	}

	received, receivedEvent, err := Apply(sent, hostID, Command{
		Kind: CommandReceivePayment, ExpectedRevision: sent.Revision,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("receive payment = %v", err)
	}
	if received.PaymentID != testPaymentID || receivedEvent.PaymentID == nil || *receivedEvent.PaymentID != testPaymentID {
		t.Fatalf("received offer/event changed movement ID: offer=%+v event=%+v", received, receivedEvent)
	}
}

func TestPaymentSendRequiresCanonicalMovementID(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	for _, paymentID := range []string{"", "not-a-uuid", "550E8400-E29B-41D4-A716-446655440000", uuid.Nil.String()} {
		t.Run(paymentID, func(t *testing.T) {
			offer := offerAtState(StatePendingPaymentSend)
			updated, event, err := Apply(offer, renterID, Command{
				Kind: CommandSendPayment, ExpectedRevision: offer.Revision, PaymentID: paymentID,
			}, now)
			if !errors.Is(err, ErrInvalid) || updated.Revision != 0 || event.Sequence != 0 || offer.PaymentID != "" {
				t.Fatalf("send with payment ID %q = offer %+v event %+v err %v", paymentID, updated, event, err)
			}
		})
	}
}

func TestAllowedActionsReflectParticipantStateAndDeadline(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		offer Offer
		actor string
		at    time.Time
		want  []CommandKind
	}{
		{
			name:  "identity not yet confirmed",
			offer: newOffer(), actor: renterID, at: now,
			want: []CommandKind{CommandConfirmIdentity, CommandReject, CommandCancel},
		},
		{
			name:  "identity already confirmed by actor",
			offer: func() Offer { o := newOffer(); o.IdentityConfirmed = map[string]time.Time{renterID: now}; return o }(), actor: renterID, at: now,
			want: []CommandKind{CommandReject, CommandCancel},
		},
		{
			name:  "contract not yet accepted",
			offer: offerAtState(StatePendingContract), actor: renterID, at: now,
			want: []CommandKind{CommandAcceptContract, CommandReject, CommandCancel},
		},
		{
			name: "contract already accepted by actor",
			offer: func() Offer {
				o := offerAtState(StatePendingContract)
				o.ContractAccepted = map[string]time.Time{renterID: now}
				return o
			}(), actor: renterID, at: now,
			want: []CommandKind{CommandReject, CommandCancel},
		},
		{
			name:  "renter may send payment",
			offer: offerAtState(StatePendingPaymentSend), actor: renterID, at: now,
			want: []CommandKind{CommandSendPayment, CommandCancel},
		},
		{
			name:  "host may not send payment",
			offer: offerAtState(StatePendingPaymentSend), actor: hostID, at: now,
			want: []CommandKind{CommandCancel},
		},
		{
			name:  "host may receive payment",
			offer: offerAtState(StatePendingPaymentReceive), actor: hostID, at: now,
			want: []CommandKind{CommandReceivePayment, CommandCancel},
		},
		{
			name:  "renter may not receive payment",
			offer: offerAtState(StatePendingPaymentReceive), actor: renterID, at: now,
			want: []CommandKind{CommandCancel},
		},
		{
			name:  "terminal offer",
			offer: offerAtState(StateConfirmed), actor: renterID, at: now,
			want: nil,
		},
		{
			name:  "deadline reached",
			offer: newOffer(), actor: renterID, at: newOffer().ExpiresAt,
			want: nil,
		},
		{
			name:  "nonparticipant",
			offer: newOffer(), actor: "intruder", at: now,
			want: nil,
		},
		{
			name:  "invalid aggregate",
			offer: func() Offer { o := newOffer(); o.State = StatePendingPaymentReceive; return o }(), actor: renterID, at: now,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllowedActions(tc.offer, tc.actor, tc.at); !slices.Equal(got, tc.want) {
				t.Fatalf("AllowedActions() = %v, want %v", got, tc.want)
			}
		})
	}
}

func applyOK(t *testing.T, offer Offer, actor string, command Command, now time.Time) Offer {
	t.Helper()
	command.ExpectedRevision = offer.Revision
	updated, _, err := Apply(offer, actor, command, now)
	if err != nil {
		t.Fatalf("Apply(%q, %q) error = %v", command.Kind, actor, err)
	}
	return updated
}

func TestIdentityAndContractMustBeBilateralBeforePayment(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	offer := newOffer()
	offer = applyOK(t, offer, renterID, Command{Kind: CommandConfirmIdentity}, now)
	if offer.State != StatePendingIdentity {
		t.Fatalf("state after one identity = %q", offer.State)
	}
	if _, _, err := Apply(offer, renterID, Command{Kind: CommandSendPayment, ExpectedRevision: offer.Revision}, now); !errors.Is(err, ErrWrongState) {
		t.Fatalf("payment before bilateral identity/contract error = %v, want ErrWrongState", err)
	}
	offer = applyOK(t, offer, hostID, Command{Kind: CommandConfirmIdentity}, now)
	if offer.State != StatePendingContract {
		t.Fatalf("state after both identities = %q", offer.State)
	}
	offer = applyOK(t, offer, renterID, Command{Kind: CommandAcceptContract, ContractVersion: 1, ContractDigest: testContractDigest}, now)
	if offer.State != StatePendingContract {
		t.Fatalf("state after one acceptance = %q", offer.State)
	}
	offer = applyOK(t, offer, hostID, Command{Kind: CommandAcceptContract, ContractVersion: 1, ContractDigest: testContractDigest}, now)
	if offer.State != StatePendingPaymentSend {
		t.Fatalf("state after both acceptances = %q", offer.State)
	}
	offer = applyOK(t, offer, renterID, Command{Kind: CommandSendPayment, PaymentID: testPaymentID}, now)
	if offer.State != StatePendingPaymentReceive || offer.PaymentSentAt == nil || offer.PaymentID != testPaymentID {
		t.Fatalf("payment-send result = state %q, sent_at %v, payment_id %q", offer.State, offer.PaymentSentAt, offer.PaymentID)
	}
	offer = applyOK(t, offer, hostID, Command{Kind: CommandReceivePayment}, now)
	if offer.State != StateConfirmed || offer.PaymentReceivedAt == nil {
		t.Fatalf("payment-receive result = state %q, received_at %v", offer.State, offer.PaymentReceivedAt)
	}
}

func TestOnlyParticipantOwnsIdentityAndOnlyHostReceivesPayment(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	if _, _, err := Apply(newOffer(), "intruder", Command{Kind: CommandConfirmIdentity, ExpectedRevision: 1}, now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nonparticipant confirmation error = %v, want ErrForbidden", err)
	}
	offer := newOffer()
	offer = applyOK(t, offer, renterID, Command{Kind: CommandConfirmIdentity}, now)
	offer = applyOK(t, offer, hostID, Command{Kind: CommandConfirmIdentity}, now)
	offer = applyOK(t, offer, renterID, Command{Kind: CommandAcceptContract, ContractVersion: 1, ContractDigest: testContractDigest}, now)
	offer = applyOK(t, offer, hostID, Command{Kind: CommandAcceptContract, ContractVersion: 1, ContractDigest: testContractDigest}, now)
	offer = applyOK(t, offer, renterID, Command{Kind: CommandSendPayment, PaymentID: testPaymentID}, now)
	if _, _, err := Apply(offer, renterID, Command{Kind: CommandReceivePayment, ExpectedRevision: offer.Revision}, now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("renter receive-payment error = %v, want ErrForbidden", err)
	}
}

func TestRejectsStaleContractAndExpiredCommands(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	offer := newOffer()
	offer = applyOK(t, offer, renterID, Command{Kind: CommandConfirmIdentity}, now)
	offer = applyOK(t, offer, hostID, Command{Kind: CommandConfirmIdentity}, now)
	_, _, err := Apply(offer, renterID, Command{Kind: CommandAcceptContract, ExpectedRevision: offer.Revision, ContractVersion: 2, ContractDigest: "sha256:other"}, now)
	if !errors.Is(err, ErrContractMismatch) {
		t.Fatalf("stale contract error = %v, want ErrContractMismatch", err)
	}
	_, _, err = Apply(offer, renterID, Command{Kind: CommandAcceptContract, ExpectedRevision: offer.Revision, ContractVersion: 1, ContractDigest: testContractDigest}, offer.ExpiresAt)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expired command error = %v, want ErrExpired", err)
	}
}

func TestApplyIsImmutableAndAdvancesRevisionOnce(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	original := newOffer()
	updated, event, err := Apply(original, renterID, Command{Kind: CommandConfirmIdentity, ExpectedRevision: original.Revision}, now)
	if err != nil {
		t.Fatal(err)
	}
	if original.Revision != 1 || len(original.IdentityConfirmed) != 0 {
		t.Fatalf("Apply mutated source offer: %+v", original)
	}
	if updated.Revision != 2 || event.Sequence != 2 || event.Revision != 2 || event.ActorID != renterID || event.Kind != EventIdentityConfirmed {
		t.Fatalf("revision/event mismatch: offer=%d event=%+v", updated.Revision, event)
	}
}

func TestApplyEmitsCanonicalEventKindsAndMonotonicSequence(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		offer   Offer
		actor   string
		command Command
		want    EventKind
	}{
		{name: "identity confirmation", offer: newOffer(), actor: renterID, command: Command{Kind: CommandConfirmIdentity}, want: EventIdentityConfirmed},
		{name: "contract acceptance", offer: offerAtState(StatePendingContract), actor: renterID, command: Command{Kind: CommandAcceptContract, ContractVersion: 1, ContractDigest: testContractDigest}, want: EventContractAccepted},
		{name: "payment sent", offer: offerAtState(StatePendingPaymentSend), actor: renterID, command: Command{Kind: CommandSendPayment, PaymentID: testPaymentID}, want: EventPaymentMarkedSent},
		{name: "payment received", offer: offerAtState(StatePendingPaymentReceive), actor: hostID, command: Command{Kind: CommandReceivePayment}, want: EventPaymentMarkedReceived},
		{name: "rejected", offer: newOffer(), actor: renterID, command: Command{Kind: CommandReject, ReasonCode: "cambio_de_planes"}, want: EventOfferRejected},
		{name: "cancelled", offer: newOffer(), actor: renterID, command: Command{Kind: CommandCancel, ReasonCode: "cambio_de_planes"}, want: EventOfferCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.command.ExpectedRevision = tc.offer.Revision
			updated, event, err := Apply(tc.offer, tc.actor, tc.command, now)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if event.Kind != tc.want || event.Sequence != updated.Revision || event.Revision != updated.Revision {
				t.Fatalf("Apply() event = %+v, updated revision = %d; want kind %q and matching sequence", event, updated.Revision, tc.want)
			}
		})
	}
}

func TestApplyRejectsStaleRevisionWithoutMutation(t *testing.T) {
	offer := newOffer()
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	updated, event, err := Apply(offer, renterID, Command{Kind: CommandConfirmIdentity, ExpectedRevision: 2}, now)
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale revision error = %v, want ErrPreconditionFailed", err)
	}
	if updated.Revision != 0 || event.Revision != 0 || len(offer.IdentityConfirmed) != 0 || offer.Revision != 1 {
		t.Fatalf("stale command mutated or returned state: updated=%+v event=%+v original=%+v", updated, event, offer)
	}
}

func TestApplyRejectsAggregateSnapshotsWithImpossibleHistory(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*Offer)
	}{
		{
			name:   "contract phase without bilateral identity",
			mutate: func(offer *Offer) { offer.State = StatePendingContract },
		},
		{
			name:   "noncanonical contract digest in snapshot",
			mutate: func(offer *Offer) { offer.ContractDigest = "sha256:terms-v1" },
		},
		{
			name:   "contract text and digest disagree",
			mutate: func(offer *Offer) { offer.ContractText += "\n" },
		},
		{
			name:   "receive phase without sent payment",
			mutate: func(offer *Offer) { offer.State = StatePendingPaymentReceive },
		},
		{
			name: "sent payment without movement id",
			mutate: func(offer *Offer) {
				*offer = offerAtState(StatePendingPaymentReceive)
				offer.PaymentID = ""
			},
		},
		{
			name:   "movement id before payment is sent",
			mutate: func(offer *Offer) { offer.PaymentID = testPaymentID },
		},
		{
			name: "noncanonical movement id in snapshot",
			mutate: func(offer *Offer) {
				*offer = offerAtState(StatePendingPaymentReceive)
				offer.PaymentID = "550E8400-E29B-41D4-A716-446655440000"
			},
		},
		{
			name: "accepted contract without identity assertions",
			mutate: func(offer *Offer) {
				offer.State = StatePendingContract
				offer.ContractAccepted = map[string]time.Time{renterID: now}
			},
		},
		{
			name: "identity confirmation at deadline",
			mutate: func(offer *Offer) {
				offer.IdentityConfirmed = map[string]time.Time{renterID: offer.ExpiresAt}
			},
		},
		{
			name:   "subtotal inconsistent with immutable price snapshot",
			mutate: func(offer *Offer) { offer.Snapshot.SubtotalCLP++ },
		},
		{
			name: "hold expires after the interval starts",
			mutate: func(offer *Offer) {
				offer.ExpiresAt = offer.Snapshot.StartAt.Add(time.Second)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offer := newOffer()
			tc.mutate(&offer)
			updated, event, err := Apply(offer, renterID, Command{
				Kind: CommandCancel, ExpectedRevision: offer.Revision, ReasonCode: "cambio_de_planes",
			}, now)
			if !errors.Is(err, ErrInvalid) || updated.Revision != 0 || event.Revision != 0 || offer.Revision != 1 {
				t.Fatalf("inconsistent snapshot result = offer %+v event %+v err %v", updated, event, err)
			}
		})
	}
}

func TestExpireRejectsConfirmedAggregateWithoutPaymentEvidence(t *testing.T) {
	offer := newOffer()
	offer.State = StateConfirmed
	updated, event, changed, err := Expire(offer, offer.ExpiresAt.Add(time.Second))
	if !errors.Is(err, ErrInvalid) || changed || updated.Revision != 0 || event.Revision != 0 || offer.State != StateConfirmed {
		t.Fatalf("invalid confirmed snapshot = offer %+v event %+v changed %v err %v", updated, event, changed, err)
	}
}

func TestParticipantCanCancelBeforeConfirmationAndPreserveUnreceivedPayment(t *testing.T) {
	offer := offerAtState(StatePendingPaymentReceive)
	sentAt := *offer.PaymentSentAt
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

	updated, event, err := Apply(offer, renterID, Command{
		Kind:             CommandKind("cancelar"),
		ExpectedRevision: offer.Revision,
		ReasonCode:       "cambio_de_planes",
	}, now)
	if err != nil {
		t.Fatalf("Apply(cancelar) error = %v", err)
	}
	if updated.State != StateCancelled || updated.Revision != offer.Revision+1 {
		t.Fatalf("cancellation result = state %q, revision %d", updated.State, updated.Revision)
	}
	if updated.PaymentSentAt == nil || !updated.PaymentSentAt.Equal(sentAt) || updated.PaymentReceivedAt != nil || updated.PaymentID != testPaymentID {
		t.Fatalf("cancellation changed payment evidence: sent_at=%v received_at=%v payment_id=%q", updated.PaymentSentAt, updated.PaymentReceivedAt, updated.PaymentID)
	}
	if event.ActorID != renterID || event.Revision != updated.Revision || event.ReasonCode != "cambio_de_planes" {
		t.Fatalf("cancellation event = %+v", event)
	}
}

func TestHostCanCancelUnconfirmedOffer(t *testing.T) {
	offer := newOffer()
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	updated, event, err := Apply(offer, hostID, Command{
		Kind:             CommandKind("cancelar"),
		ExpectedRevision: offer.Revision,
		ReasonCode:       "condiciones_no_aceptadas",
	}, now)
	if err != nil {
		t.Fatalf("Apply(cancelar) error = %v", err)
	}
	if updated.State != StateCancelled || event.ActorID != hostID {
		t.Fatalf("host cancellation = offer %+v event %+v", updated, event)
	}
}

func TestCancelRequiresAllowlistedReasonAndCannotReopenTerminalOffer(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	for _, reason := range []string{"", "unknown"} {
		t.Run("reason_"+reason, func(t *testing.T) {
			offer := newOffer()
			updated, event, err := Apply(offer, renterID, Command{
				Kind:             CommandKind("cancelar"),
				ExpectedRevision: offer.Revision,
				ReasonCode:       reason,
			}, now)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid reason error = %v, want ErrInvalid", err)
			}
			if updated.Revision != 0 || event.Revision != 0 || offer.State != StatePendingIdentity || offer.Revision != 1 {
				t.Fatalf("invalid cancellation mutated state: updated=%+v event=%+v offer=%+v", updated, event, offer)
			}
		})
	}

	offer := offerAtState(StateConfirmed)
	if _, _, err := Apply(offer, renterID, Command{
		Kind:             CommandKind("cancelar"),
		ExpectedRevision: offer.Revision,
		ReasonCode:       "otro",
	}, now); !errors.Is(err, ErrWrongState) {
		t.Fatalf("confirmed-offer cancellation error = %v, want ErrWrongState", err)
	}
}

func TestParticipantsCanRejectOnlyOfferOrContractPending(t *testing.T) {
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		actorID string
		state   State
	}{
		{name: "host rejects new offer", actorID: hostID, state: StatePendingIdentity},
		{name: "renter rejects pending contract", actorID: renterID, state: StatePendingContract},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offer := offerAtState(tc.state)
			updated, event, err := Apply(offer, tc.actorID, Command{
				Kind:             CommandKind("rechazar"),
				ExpectedRevision: offer.Revision,
				ReasonCode:       "condiciones_no_aceptadas",
			}, now)
			if err != nil {
				t.Fatalf("Apply(rechazar) error = %v", err)
			}
			if updated.State != StateRejected || updated.Revision != offer.Revision+1 || event.ActorID != tc.actorID || event.ReasonCode != "condiciones_no_aceptadas" {
				t.Fatalf("rejection result = offer %+v event %+v", updated, event)
			}
		})
	}

	offer := offerAtState(StatePendingPaymentSend)
	if _, _, err := Apply(offer, renterID, Command{
		Kind:             CommandKind("rechazar"),
		ExpectedRevision: offer.Revision,
		ReasonCode:       "otro",
	}, now); !errors.Is(err, ErrWrongState) {
		t.Fatalf("post-contract rejection error = %v, want ErrWrongState", err)
	}
}

func TestExpireDueOfferCreatesSystemEventAndPreservesUnreceivedPayment(t *testing.T) {
	offer := offerAtState(StatePendingPaymentReceive)
	sentAt := *offer.PaymentSentAt
	now := offer.ExpiresAt

	updated, event, changed, err := Expire(offer, now)
	if err != nil {
		t.Fatalf("Expire() error = %v", err)
	}
	if !changed || updated.State != StateExpired || updated.Revision != offer.Revision+1 {
		t.Fatalf("expiry result = changed %v, state %q, revision %d", changed, updated.State, updated.Revision)
	}
	if event.Kind != EventOfferExpired || event.ActorID != "system" || event.Sequence != updated.Revision || event.Revision != updated.Revision || !event.At.Equal(now) ||
		event.PreviousState != StatePendingPaymentReceive || event.NextState != StateExpired {
		t.Fatalf("expiry event = %+v", event)
	}
	if updated.PaymentSentAt == nil || !updated.PaymentSentAt.Equal(sentAt) || updated.PaymentReceivedAt != nil || updated.PaymentID != testPaymentID {
		t.Fatalf("expiry changed payment evidence: sent_at=%v received_at=%v payment_id=%q", updated.PaymentSentAt, updated.PaymentReceivedAt, updated.PaymentID)
	}
	if offer.State != StatePendingPaymentReceive || offer.Revision != 1 || offer.PaymentReceivedAt != nil {
		t.Fatalf("Expire() mutated source offer: %+v", offer)
	}

	replayed, replayEvent, replayChanged, err := Expire(updated, now.Add(time.Minute))
	if err != nil || replayChanged || replayEvent.Revision != 0 || replayed.Revision != updated.Revision {
		t.Fatalf("replayed expiry = offer %+v event %+v changed %v err %v", replayed, replayEvent, replayChanged, err)
	}
}

func TestExpireBeforeDeadlineIsNoOp(t *testing.T) {
	offer := newOffer()
	now := offer.ExpiresAt.Add(-time.Nanosecond)
	updated, event, changed, err := Expire(offer, now)
	if err != nil || changed || updated.State != offer.State || updated.Revision != offer.Revision || event.Revision != 0 {
		t.Fatalf("early expiry = offer %+v event %+v changed %v err %v", updated, event, changed, err)
	}
}

// Characterizes the boundary shared by user commands and the expiry worker.
func TestLatePaymentReceiptCannotBeatExpiryTransition(t *testing.T) {
	offer := offerAtState(StatePendingPaymentReceive)
	now := offer.ExpiresAt

	updated, event, err := Apply(offer, hostID, Command{
		Kind:             CommandReceivePayment,
		ExpectedRevision: offer.Revision,
	}, now)
	if !errors.Is(err, ErrExpired) || updated.Revision != 0 || event.Revision != 0 || offer.PaymentReceivedAt != nil {
		t.Fatalf("receipt at deadline = offer %+v event %+v err %v", updated, event, err)
	}

	expired, expiryEvent, changed, err := Expire(offer, now)
	if err != nil || !changed || expired.State != StateExpired || expired.PaymentSentAt == nil || expired.PaymentReceivedAt != nil || expiryEvent.Kind != EventOfferExpired || expiryEvent.Sequence != expired.Revision {
		t.Fatalf("expiry after rejected receipt = offer %+v event %+v changed %v err %v", expired, expiryEvent, changed, err)
	}
}

func TestExpireIsNoOpForEveryTerminalState(t *testing.T) {
	for _, state := range []State{StateConfirmed, StateRejected, StateCancelled, StateExpired} {
		t.Run(string(state), func(t *testing.T) {
			offer := offerAtState(state)
			updated, event, changed, err := Expire(offer, offer.ExpiresAt.Add(time.Minute))
			if err != nil || changed || updated.State != state || updated.Revision != offer.Revision || event.Revision != 0 {
				t.Fatalf("terminal expiry = offer %+v event %+v changed %v err %v", updated, event, changed, err)
			}
		})
	}
}
