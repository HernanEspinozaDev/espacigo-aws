package earlyrental

import (
	"math"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	RateUnitHour = "hora"
	CurrencyCLP  = "CLP"
)

type CalendarRateOverrideStatus uint8

const (
	CalendarRateOverrideUnresolved CalendarRateOverrideStatus = iota
	CalendarRateOverrideAbsent
	CalendarRateOverridePresent
)

// OfferSnapshotInput contains server-resolved listing and quote facts. The
// public create request must never populate price, units, title, or time zone.
type OfferSnapshotInput struct {
	SpaceID      uuid.UUID
	SpaceTitle   string
	RateUnit     string
	UnitPriceCLP int64
	Units        int64
	StartAt      time.Time
	EndAt        time.Time
	TimeZone     string
	// Must come from the server's authoritative calendar read. An unresolved
	// check and any custom-date rate both fail closed for early rental.
	CalendarRateOverride CalendarRateOverrideStatus
}

// OfferSnapshot is the commercial snapshot attached to an offer. Domain
// transitions preserve it; persistence must enforce immutability after create.
// EndAt is exclusive; timestamps are normalized to UTC while TimeZone retains
// the publication's authoritative IANA zone.
type OfferSnapshot struct {
	SpaceID      uuid.UUID
	SpaceTitle   string
	RateUnit     string
	UnitPriceCLP int64
	Currency     string
	Units        int64
	SubtotalCLP  int64
	StartAt      time.Time
	EndAt        time.Time
	TimeZone     string
}

func newOfferSnapshot(input OfferSnapshotInput) (OfferSnapshot, error) {
	if input.CalendarRateOverride != CalendarRateOverrideAbsent {
		return OfferSnapshot{}, ErrInvalid
	}
	snapshot := OfferSnapshot{
		SpaceID: input.SpaceID, SpaceTitle: input.SpaceTitle, RateUnit: input.RateUnit,
		UnitPriceCLP: input.UnitPriceCLP, Currency: CurrencyCLP, Units: input.Units,
		StartAt: input.StartAt.UTC(), EndAt: input.EndAt.UTC(), TimeZone: input.TimeZone,
	}
	if !validOfferSnapshotFields(snapshot, false) {
		return OfferSnapshot{}, ErrInvalid
	}
	snapshot.SubtotalCLP = snapshot.UnitPriceCLP * snapshot.Units
	return snapshot, nil
}

func validOfferSnapshot(snapshot OfferSnapshot) bool {
	return validOfferSnapshotFields(snapshot, true)
}

func validOfferSnapshotFields(snapshot OfferSnapshot, checkSubtotal bool) bool {
	if snapshot.SpaceID == uuid.Nil || snapshot.SpaceTitle == "" || !utf8.ValidString(snapshot.SpaceTitle) ||
		utf8.RuneCountInString(snapshot.SpaceTitle) > 70 || snapshot.RateUnit != RateUnitHour ||
		snapshot.Currency != CurrencyCLP || snapshot.UnitPriceCLP < 5_001 ||
		(snapshot.Units != 1 && snapshot.Units != 2 && snapshot.Units != 4) ||
		snapshot.StartAt.IsZero() || snapshot.EndAt.IsZero() || !snapshot.EndAt.After(snapshot.StartAt) ||
		snapshot.EndAt.Sub(snapshot.StartAt) != time.Duration(snapshot.Units)*time.Hour ||
		snapshot.TimeZone == "" || snapshot.TimeZone == "Local" || !utf8.ValidString(snapshot.TimeZone) ||
		utf8.RuneCountInString(snapshot.TimeZone) > 100 {
		return false
	}
	if snapshot.UnitPriceCLP > math.MaxInt64/snapshot.Units {
		return false
	}
	if checkSubtotal && snapshot.SubtotalCLP != snapshot.UnitPriceCLP*snapshot.Units {
		return false
	}
	_, err := time.LoadLocation(snapshot.TimeZone)
	return err == nil
}
