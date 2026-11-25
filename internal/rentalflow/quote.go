package rentalflow

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialpolicy"
	"github.com/HernanEspinozaDev/espaciGo/internal/pricing"
)

// QuoteLifetime is the validity window assigned when a quote is created.
const QuoteLifetime = 6 * time.Minute

const (
	BillingModeFull                = "full"
	BillingModeMonthlyInstallments = "monthly_installments"
)

type MonthlyRule struct {
	DisponibleDesde    *string
	EstadiaMinimaMeses int
}

type MonthlyRuleReader interface {
	PublicMonthlyRule(context.Context, string) (MonthlyRule, error)
}

type QuoteBillingModeRepository interface {
	SaveQuoteBillingMode(context.Context, string, string) error
}

func normalizeBillingMode(rateUnit, raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", BillingModeFull:
		return BillingModeFull, nil
	case BillingModeMonthlyInstallments:
		if rateUnit != "mes" {
			return "", ErrInvalid
		}
		return BillingModeMonthlyInstallments, nil
	default:
		return "", ErrInvalid
	}
}

type QuoteRequest struct {
	SpaceID     string `json:"space_id"`
	StartAt     string `json:"start_at"`
	EndAt       string `json:"end_at"`
	BillingMode string `json:"billing_mode,omitempty"`
}

type MultiQuoteIntervalInput struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type MultiQuoteRequest struct {
	SpaceID     string                    `json:"space_id"`
	Intervals   []MultiQuoteIntervalInput `json:"intervals"`
	BillingMode string                    `json:"billing_mode,omitempty"`
}

type QuoteInterval struct {
	ID          string    `json:"id,omitempty"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	Units       int64     `json:"units"`
	UsageStatus string    `json:"usage_status,omitempty"`
	CheckInAt   string    `json:"check_in_at,omitempty"`
	CheckOutAt  string    `json:"check_out_at,omitempty"`
	PassCode    string    `json:"pass_code,omitempty"`
	QRPayload   string    `json:"qr_payload,omitempty"`
}

type MultiQuote struct {
	Quote
	Intervals []QuoteInterval `json:"intervals"`
}

type Quote struct {
	ID                 string                   `json:"id"`
	SpaceID            string                   `json:"space_id"`
	OwnerID            string                   `json:"-"`
	RenterID           string                   `json:"-"`
	SpaceTitle         string                   `json:"space_title"`
	RateVersion        int64                    `json:"-"`
	RateUnit           string                   `json:"rate_unit"`
	UnitPriceCLP       int64                    `json:"unit_price_clp"`
	Currency           string                   `json:"currency"`
	Units              int64                    `json:"units"`
	SubtotalCLP        int64                    `json:"subtotal_clp"`
	StartAt            time.Time                `json:"start_at"`
	EndAt              time.Time                `json:"end_at"`
	TimeZone           string                   `json:"time_zone"`
	CreatedAt          time.Time                `json:"created_at"`
	ExpiresAt          time.Time                `json:"expires_at"`
	BillingMode        string                   `json:"billing_mode,omitempty"`
	CancellationPolicy *commercialpolicy.Policy `json:"cancellation_policy,omitempty"`
}

// QuoteRepository must atomically re-read the active publication and current
// rate, re-check the interval against schedule and occupancy, and persist only
// the quote snapshot; it must never create an occupancy hold.
type QuoteRepository interface {
	CreateValidatedQuote(context.Context, string, Listing, Interval, int64, int64, time.Time) (Quote, error)
	GetForParticipant(context.Context, string, string) (Quote, error)
}

type MultiQuoteRepository interface {
	CreateMultiValidatedQuote(context.Context, string, Listing, []QuoteInterval, int64, int64, time.Time) (MultiQuote, error)
}

type QuoteService struct {
	listings PublicListingReader
	calendar AvailabilityReader
	quotes   QuoteRepository
	now      func() time.Time
}

func NewQuoteService(listings PublicListingReader, calendar AvailabilityReader, quotes QuoteRepository, now func() time.Time) (*QuoteService, error) {
	if listings == nil || calendar == nil || quotes == nil || now == nil {
		return nil, ErrInvalid
	}
	return &QuoteService{listings: listings, calendar: calendar, quotes: quotes, now: now}, nil
}

func (s *QuoteService) Create(ctx context.Context, renterID string, in QuoteRequest) (Quote, error) {
	if s == nil || ctx == nil || !validUUID(renterID) || !validUUID(in.SpaceID) {
		return Quote{}, ErrInvalid
	}
	renterID = canonicalUUID(renterID)
	in.SpaceID = canonicalUUID(in.SpaceID)
	if ctx.Err() != nil {
		return Quote{}, ErrUnavailable
	}
	now := s.now()
	if now.IsZero() {
		return Quote{}, ErrInvalid
	}
	start, valid := parseRFC3339Timestamp(in.StartAt)
	if !valid {
		return Quote{}, ErrInvalid
	}
	end, valid := parseRFC3339Timestamp(in.EndAt)
	if !valid || !end.After(start) {
		return Quote{}, ErrInvalid
	}
	now = now.UTC()
	if !start.After(now) {
		return Quote{}, ErrInvalid
	}
	listing, err := s.listings.PublicListing(ctx, in.SpaceID)
	if ctx.Err() != nil {
		return Quote{}, ErrUnavailable
	}
	listing.ID = canonicalUUID(listing.ID)
	listing.OwnerID = canonicalUUID(listing.OwnerID)
	if errors.Is(err, ErrNotFound) {
		return Quote{}, ErrNotFound
	}
	if err != nil {
		return Quote{}, ErrUnavailable
	}
	if listing.ID != in.SpaceID || listing.OwnerID == renterID || listing.State != "activa" {
		return Quote{}, ErrNotFound
	}
	loc, err := validateListing(listing)
	if err != nil {
		return Quote{}, err
	}
	billingMode, err := normalizeBillingMode(listing.RateUnit, in.BillingMode)
	if err != nil {
		return Quote{}, err
	}
	units, err := pricing.BilledUnits(listing.RateUnit, start.UTC(), end.UTC(), listing.TimeZone)
	if err != nil || units < 1 || units > math.MaxInt64/listing.UnitPriceCLP || !ValidFlexibleDuration(listing.RateUnit, int(units)) {
		return Quote{}, ErrInvalid
	}
	interval := Interval{StartAt: start.UTC(), EndAt: end.UTC()}
	dateText := start.In(loc).Format("2006-01-02")
	if !dateWithinHorizon(dateText, now, loc) || !intervalMatchesRequest(interval, listing.RateUnit, dateText, int(units), listing.TimeZone, now) {
		return Quote{}, ErrInvalid
	}
	if err := s.validateMonthlyRule(ctx, listing, []string{dateText}, units); err != nil {
		return Quote{}, err
	}
	available, err := s.calendar.IntervalAvailable(ctx, listing, interval)
	if err != nil {
		return Quote{}, ErrUnavailable
	}
	if ctx.Err() != nil {
		return Quote{}, ErrUnavailable
	}
	if !available {
		return Quote{}, ErrConflict
	}
	subtotal := units * listing.UnitPriceCLP
	quote, err := s.quotes.CreateValidatedQuote(ctx, renterID, listing, interval, units, subtotal, now)
	if err != nil {
		if ctx.Err() != nil {
			return Quote{}, ErrUnavailable
		}
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
			return Quote{}, err
		}
		return Quote{}, ErrUnavailable
	}
	quote.ID = canonicalUUID(quote.ID)
	quote.SpaceID = canonicalUUID(quote.SpaceID)
	quote.OwnerID = canonicalUUID(quote.OwnerID)
	quote.RenterID = canonicalUUID(quote.RenterID)
	if !validQuote(quote, renterID, listing, interval, units, subtotal, now) {
		return Quote{}, ErrUnavailable
	}
	if billingMode == BillingModeMonthlyInstallments {
		if bmRepo, ok := s.quotes.(QuoteBillingModeRepository); ok {
			if err := bmRepo.SaveQuoteBillingMode(ctx, quote.ID, billingMode); err != nil {
				return Quote{}, ErrUnavailable
			}
		}
	}
	if listing.RateUnit == "mes" {
		quote.BillingMode = billingMode
	}
	return quote, nil
}

// CreateMulti validates and persists a quote spanning multiple non-contiguous
// or multi-day intervals for the same active space.
func (s *QuoteService) CreateMulti(ctx context.Context, renterID string, in MultiQuoteRequest) (MultiQuote, error) {
	if s == nil || ctx == nil || !validUUID(renterID) || !validUUID(in.SpaceID) || len(in.Intervals) == 0 || len(in.Intervals) > 64 {
		return MultiQuote{}, ErrInvalid
	}
	renterID = canonicalUUID(renterID)
	in.SpaceID = canonicalUUID(in.SpaceID)
	if ctx.Err() != nil {
		return MultiQuote{}, ErrUnavailable
	}
	now := s.now()
	if now.IsZero() {
		return MultiQuote{}, ErrInvalid
	}
	now = now.UTC()

	listing, err := s.listings.PublicListing(ctx, in.SpaceID)
	if ctx.Err() != nil {
		return MultiQuote{}, ErrUnavailable
	}
	listing.ID = canonicalUUID(listing.ID)
	listing.OwnerID = canonicalUUID(listing.OwnerID)
	if errors.Is(err, ErrNotFound) {
		return MultiQuote{}, ErrNotFound
	}
	if err != nil {
		return MultiQuote{}, ErrUnavailable
	}
	if listing.ID != in.SpaceID || listing.OwnerID == renterID || listing.State != "activa" {
		return MultiQuote{}, ErrNotFound
	}
	loc, err := validateListing(listing)
	if err != nil {
		return MultiQuote{}, err
	}
	billingMode, err := normalizeBillingMode(listing.RateUnit, in.BillingMode)
	if err != nil {
		return MultiQuote{}, err
	}

	parsed := make([]QuoteInterval, 0, len(in.Intervals))
	dates := make([]string, 0, len(in.Intervals))
	var totalUnits int64
	for _, raw := range in.Intervals {
		start, okStart := parseRFC3339Timestamp(raw.StartAt)
		end, okEnd := parseRFC3339Timestamp(raw.EndAt)
		if !okStart || !okEnd || !end.After(start) || !start.After(now) {
			return MultiQuote{}, ErrInvalid
		}
		startUTC, endUTC := start.UTC(), end.UTC()
		units, unitErr := pricing.BilledUnits(listing.RateUnit, startUTC, endUTC, listing.TimeZone)
		if unitErr != nil || units < 1 || !ValidFlexibleDuration(listing.RateUnit, int(units)) {
			return MultiQuote{}, ErrInvalid
		}
		dateText := startUTC.In(loc).Format("2006-01-02")
		iv := Interval{StartAt: startUTC, EndAt: endUTC}
		if !dateWithinHorizon(dateText, now, loc) || !intervalMatchesRequest(iv, listing.RateUnit, dateText, int(units), listing.TimeZone, now) {
			return MultiQuote{}, ErrInvalid
		}
		if totalUnits > math.MaxInt64-units {
			return MultiQuote{}, ErrInvalid
		}
		totalUnits += units
		dates = append(dates, dateText)
		parsed = append(parsed, QuoteInterval{StartAt: startUTC, EndAt: endUTC, Units: units})
	}
	if totalUnits < 1 || totalUnits > math.MaxInt64/listing.UnitPriceCLP {
		return MultiQuote{}, ErrInvalid
	}
	if err := s.validateMonthlyRule(ctx, listing, dates, totalUnits); err != nil {
		return MultiQuote{}, err
	}

	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].StartAt.Before(parsed[j].StartAt)
	})
	for i := 1; i < len(parsed); i++ {
		if parsed[i].StartAt.Before(parsed[i-1].EndAt) {
			return MultiQuote{}, ErrInvalid
		}
	}

	for _, item := range parsed {
		available, availErr := s.calendar.IntervalAvailable(ctx, listing, Interval{StartAt: item.StartAt, EndAt: item.EndAt})
		if availErr != nil || ctx.Err() != nil {
			return MultiQuote{}, ErrUnavailable
		}
		if !available {
			return MultiQuote{}, ErrConflict
		}
	}

	multiRepo, ok := s.quotes.(MultiQuoteRepository)
	if !ok {
		return MultiQuote{}, ErrUnavailable
	}
	subtotal := totalUnits * listing.UnitPriceCLP
	quote, err := multiRepo.CreateMultiValidatedQuote(ctx, renterID, listing, parsed, totalUnits, subtotal, now)
	if err != nil {
		if ctx.Err() != nil {
			return MultiQuote{}, ErrUnavailable
		}
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
			return MultiQuote{}, err
		}
		return MultiQuote{}, ErrUnavailable
	}
	quote.ID = canonicalUUID(quote.ID)
	quote.SpaceID = canonicalUUID(quote.SpaceID)
	quote.OwnerID = canonicalUUID(quote.OwnerID)
	quote.RenterID = canonicalUUID(quote.RenterID)
	if billingMode == BillingModeMonthlyInstallments {
		if bmRepo, ok := s.quotes.(QuoteBillingModeRepository); ok {
			if err := bmRepo.SaveQuoteBillingMode(ctx, quote.ID, billingMode); err != nil {
				return MultiQuote{}, ErrUnavailable
			}
		}
	}
	if listing.RateUnit == "mes" {
		quote.BillingMode = billingMode
	}
	return quote, nil
}

func (s *QuoteService) validateMonthlyRule(ctx context.Context, listing Listing, startDates []string, totalUnits int64) error {
	if listing.RateUnit != "mes" {
		return nil
	}
	reader, ok := s.listings.(MonthlyRuleReader)
	if !ok {
		return nil
	}
	rule, err := reader.PublicMonthlyRule(ctx, listing.ID)
	if err != nil {
		return ErrUnavailable
	}
	if rule.EstadiaMinimaMeses > 1 && totalUnits < int64(rule.EstadiaMinimaMeses) {
		return ErrInvalid
	}
	if rule.DisponibleDesde != nil && *rule.DisponibleDesde != "" {
		for _, d := range startDates {
			if d < *rule.DisponibleDesde {
				return ErrConflict
			}
		}
	}
	return nil
}

func parseRFC3339Timestamp(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil && !parsed.IsZero()
}

// ValidTimestamp reports whether value is a non-zero RFC3339 timestamp.
func ValidTimestamp(value string) bool {
	_, valid := parseRFC3339Timestamp(value)
	return valid
}

func validQuote(quote Quote, renterID string, listing Listing, interval Interval, units, subtotal int64, now time.Time) bool {
	return validUUID(quote.ID) && quote.SpaceID == listing.ID && quote.OwnerID == listing.OwnerID &&
		quote.RenterID == renterID && quote.SpaceTitle == listing.Title && quote.RateVersion == listing.RateVersion &&
		quote.RateUnit == listing.RateUnit && quote.UnitPriceCLP == listing.UnitPriceCLP && quote.Currency == "CLP" &&
		quote.Units == units && quote.SubtotalCLP == subtotal && quote.StartAt.Equal(interval.StartAt) &&
		quote.EndAt.Equal(interval.EndAt) && quote.TimeZone == listing.TimeZone &&
		!quote.CreatedAt.IsZero() && !quote.CreatedAt.After(now) && quote.ExpiresAt.After(now)
}
