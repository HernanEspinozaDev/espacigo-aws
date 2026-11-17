package rentalflowpg

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

const contractText = "Early rental contract — exact UTF-8 bytes\n"

var (
	createdAt = time.Date(2032, 1, 2, 12, 0, 0, 0, time.FixedZone("minus-three", -3*60*60))
	hostID    = "10000000-0000-4000-8000-000000000001"
	renterID  = "20000000-0000-4000-8000-000000000002"
	offerID   = "30000000-0000-4000-8000-000000000003"
	occupancy = "4a000000-0000-0000-8000-000000000004"
)

func initialFacts(t *testing.T, start time.Time) (earlyrental.Offer, earlyrental.Event) {
	t.Helper()
	offer, event, err := earlyrental.NewOffer(earlyrental.NewOfferInput{
		ID: offerID, HostID: hostID, RenterID: renterID,
		Snapshot: earlyrental.OfferSnapshotInput{
			SpaceID: uuid.MustParse("50000000-0000-4000-8000-000000000005"), SpaceTitle: "Habitación",
			RateUnit: earlyrental.RateUnitHour, UnitPriceCLP: 6000, Units: 1,
			StartAt: start, EndAt: start.Add(time.Hour), TimeZone: "America/Santiago",
			CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent,
		},
		ContractVersion: 7, ContractText: contractText,
	}, createdAt)
	if err != nil {
		t.Fatalf("NewOffer(): %v", err)
	}
	return offer, event
}

func TestNewEarlyRentalInitialRecordValidatesAndCopiesExactFacts(t *testing.T) {
	start := createdAt.Add(2 * time.Hour)
	offer, event := initialFacts(t, start)
	wantDigest := sha256.Sum256([]byte(contractText))
	first, err := newEarlyRentalInitialRecord(offer, occupancy, event, createdAt)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	second, err := newEarlyRentalInitialRecord(offer, occupancy, event, createdAt)
	if err != nil {
		t.Fatalf("second constructor: %v", err)
	}
	if first.OccupancyID != occupancy || !first.CreatedAt.Equal(createdAt.UTC()) || !first.UpdatedAt.Equal(createdAt.UTC()) {
		t.Fatalf("record metadata: %#v", first)
	}
	if !reflect.DeepEqual(first.Offer, offer) {
		t.Fatalf("offer changed: got %#v want %#v", first.Offer, offer)
	}
	if first.InitialEvent != event {
		t.Fatalf("event changed: got %#v want %#v", first.InitialEvent, event)
	}
	if !reflect.DeepEqual(first.ContractDigest, wantDigest[:]) || hex.EncodeToString(first.ContractDigest) != offer.ContractDigest {
		t.Fatalf("digest=%x, want %x (%s)", first.ContractDigest, wantDigest, hex.EncodeToString(wantDigest[:]))
	}
	if &first.ContractDigest[0] == &second.ContractDigest[0] {
		t.Fatal("separate calls share digest backing storage")
	}
	first.ContractDigest[0] ^= 0xff
	if reflect.DeepEqual(first.ContractDigest, second.ContractDigest) || !reflect.DeepEqual(second.ContractDigest, wantDigest[:]) {
		t.Fatal("digest mutation leaked between calls")
	}
	first.Offer.IdentityConfirmed[hostID] = createdAt
	first.Offer.ContractAccepted[renterID] = createdAt
	if len(offer.IdentityConfirmed) != 0 || len(offer.ContractAccepted) != 0 {
		t.Fatal("record map mutation changed input maps")
	}
	offer.IdentityConfirmed[renterID] = createdAt
	offer.ContractAccepted[hostID] = createdAt
	if len(second.Offer.IdentityConfirmed) != 0 || len(second.Offer.ContractAccepted) != 0 {
		t.Fatal("input map mutation changed second record maps")
	}
	if len(first.Offer.IdentityConfirmed) != 1 || len(first.Offer.ContractAccepted) != 1 {
		t.Fatal("input map mutation changed record maps")
	}
	first.Offer.IdentityConfirmed = nil
	first.Offer.ContractAccepted = nil
	if offer.IdentityConfirmed == nil || offer.ContractAccepted == nil {
		t.Fatal("record map reassignment changed input")
	}
	for name, got := range map[string]time.Time{"created": first.CreatedAt, "updated": first.UpdatedAt, "start": first.Offer.Snapshot.StartAt, "end": first.Offer.Snapshot.EndAt, "expiry": first.Offer.ExpiresAt, "event": first.InitialEvent.At} {
		if got.Location() != time.UTC {
			t.Errorf("%s location = %v, want UTC", name, got.Location())
		}
	}
}

func TestNewEarlyRentalInitialRecordAcceptsExpiryBoundariesAndCanonicalOccupancy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start time.Time
		id    string
	}{
		{"normal ttl", createdAt.Add(2 * time.Hour), occupancy},
		{"start before ttl", createdAt.Add(29 * time.Minute), occupancy},
		{"start exactly ttl", createdAt.Add(earlyrental.HoldTTL), occupancy},
		{"canonical non-v4 occupancy", createdAt.Add(2 * time.Hour), "60000000-0000-0000-0000-000000000006"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offer, event := initialFacts(t, tc.start)
			if _, err := newEarlyRentalInitialRecord(offer, tc.id, event, createdAt); err != nil {
				t.Fatalf("valid facts rejected: %v", err)
			}
		})
	}
}

func TestNewEarlyRentalInitialRecordAcceptsNilEmptyConfirmations(t *testing.T) {
	for _, identityNil := range []bool{false, true} {
		for _, contractNil := range []bool{false, true} {
			offer, event := initialFacts(t, createdAt.Add(2*time.Hour))
			if identityNil {
				offer.IdentityConfirmed = nil
			}
			if contractNil {
				offer.ContractAccepted = nil
			}
			if err := earlyrental.ValidateOffer(offer); err != nil {
				t.Fatalf("domain fixture rejected: %v", err)
			}
			record, err := newEarlyRentalInitialRecord(offer, occupancy, event, createdAt)
			if err != nil {
				t.Fatalf("empty confirmations rejected: %v", err)
			}
			if (record.Offer.IdentityConfirmed == nil) != identityNil || (record.Offer.ContractAccepted == nil) != contractNil {
				t.Fatal("nil/empty map distinction changed")
			}
		}
	}
}

func TestNewEarlyRentalInitialRecordRejectsInvalidFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*earlyrental.Offer, *earlyrental.Event, *string, *time.Time)
	}{
		{"created zero", func(_ *earlyrental.Offer, _ *earlyrental.Event, _ *string, at *time.Time) { *at = time.Time{} }},
		{"created at start", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, at *time.Time) { *at = o.Snapshot.StartAt }},
		{"created after start", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, at *time.Time) {
			*at = o.Snapshot.StartAt.Add(time.Nanosecond)
		}},
		{"state", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.State = earlyrental.StatePendingContract
		}},
		{"revision", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) { o.Revision = 2 }},
		{"revision exceeds int64", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.Revision = uint64(math.MaxInt64) + 1
		}},
		{"identity confirmations", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.IdentityConfirmed[hostID] = createdAt
		}},
		{"contract acceptances", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.ContractAccepted[renterID] = createdAt
		}},
		{"payment id", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) { o.PaymentID = "payment" }},
		{"payment sent timestamp", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			v := createdAt
			o.PaymentSentAt = &v
		}},
		{"payment received timestamp", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			v := createdAt
			o.PaymentReceivedAt = &v
		}},
		{"occupancy uppercase", func(_ *earlyrental.Offer, _ *earlyrental.Event, id *string, _ *time.Time) {
			*id = strings.ToUpper(occupancy)
		}},
		{"occupancy malformed", func(_ *earlyrental.Offer, _ *earlyrental.Event, id *string, _ *time.Time) { *id = "not-a-uuid" }},
		{"occupancy nil", func(_ *earlyrental.Offer, _ *earlyrental.Event, id *string, _ *time.Time) { *id = uuid.Nil.String() }},
		{"digest uppercase", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.ContractDigest = strings.ToUpper(o.ContractDigest)
		}},
		{"digest malformed", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) { o.ContractDigest = "xyz" }},
		{"digest mismatch", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.ContractDigest = strings.Repeat("0", 64)
		}},
		{"expiry lower", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.ExpiresAt = o.ExpiresAt.Add(-time.Nanosecond)
		}},
		{"expiry higher", func(o *earlyrental.Offer, _ *earlyrental.Event, _ *string, _ *time.Time) {
			o.ExpiresAt = o.ExpiresAt.Add(time.Nanosecond)
		}},
		{"event kind", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) {
			e.Kind = earlyrental.EventOfferCancelled
		}},
		{"event actor", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.ActorID = hostID }},
		{"event sequence", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.Sequence = 2 }},
		{"event revision", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.Revision = 2 }},
		{"event previous state", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) {
			e.PreviousState = earlyrental.StatePendingIdentity
		}},
		{"event next state", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) {
			e.NextState = earlyrental.StatePendingContract
		}},
		{"event timestamp", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) {
			e.At = e.At.Add(time.Second)
		}},
		{"event contract version", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.ContractVersion = 1 }},
		{"event digest", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.ContractDigest = "digest" }},
		{"event reason", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) { e.ReasonCode = "reason" }},
		{"event payment", func(_ *earlyrental.Offer, e *earlyrental.Event, _ *string, _ *time.Time) {
			v := "payment"
			e.PaymentID = &v
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, e := initialFacts(t, createdAt.Add(2*time.Hour))
			id, at := occupancy, createdAt
			tc.mutate(&o, &e, &id, &at)
			got, err := newEarlyRentalInitialRecord(o, id, e, at)
			if !errors.Is(err, earlyrental.ErrInvalid) {
				t.Fatalf("error=%v, want ErrInvalid", err)
			}
			if !reflect.DeepEqual(got, earlyRentalInitialRecord{}) {
				t.Fatalf("invalid input returned nonzero record: %#v", got)
			}
		})
	}
}
