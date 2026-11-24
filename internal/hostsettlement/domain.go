// Package hostsettlement projects verified commercial receipts for their
// original landlord. It never estimates provider expenses or issues tax files.
package hostsettlement

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

var (
	ErrInvalid     = errors.New("hostsettlement: invalid")
	ErrNotFound    = errors.New("hostsettlement: not found")
	ErrUnavailable = errors.New("hostsettlement: inconsistent recorded amounts")
)

type Amounts struct {
	GrossCLP               string  `json:"gross_clp"`
	BaseCLP                string  `json:"base_clp"`
	CommissionCLP          string  `json:"commission_clp"`
	CommissionVATCLP       string  `json:"commission_vat_clp"`
	MarketplaceFeeCLP      string  `json:"marketplace_fee_clp"`
	AfterMarketplaceFeeCLP string  `json:"after_marketplace_fee_clp"`
	ProcessorFeeCLP        *string `json:"processor_fee_clp"`
	AdjustmentsCLP         *string `json:"adjustments_clp"`
	ReconciledNetCLP       *string `json:"reconciled_net_clp"`
}
type Item struct {
	ReservationID     string    `json:"reservation_id"`
	SpaceID           string    `json:"space_id"`
	SpaceTitle        string    `json:"space_title"`
	ProviderReference string    `json:"provider_reference"`
	VerifiedAt        time.Time `json:"verified_at"`
	Currency          string    `json:"currency"`
	TestMode          bool      `json:"test_mode"`
	Amounts
	FeePolicy                string `json:"fee_policy"`
	CommissionRatePercent    *int   `json:"commission_rate_percent"`
	CommissionVATRatePercent *int   `json:"commission_vat_rate_percent"`
}
type Summary struct {
	Month        string `json:"month"`
	TimeZone     string `json:"time_zone"`
	Mode         string `json:"mode"`
	Currency     string `json:"currency"`
	PaymentCount int64  `json:"payment_count"`
	Amounts
}
type Page struct {
	Summary    Summary `json:"summary"`
	Items      []Item  `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type Query struct {
	Month, Mode, Cursor string
	StartAt, EndAt      time.Time
}
type Repository interface {
	List(context.Context, string, Query) (Page, error)
	Get(context.Context, string, string) (Item, error)
}
type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo: repo} }
func QueryFor(month, mode, cursor string) (Query, error) {
	parsed, err := time.Parse("2006-01", month)
	if err != nil || parsed.Format("2006-01") != month || parsed.Year() < 2000 || parsed.Year() > 2100 {
		return Query{}, ErrInvalid
	}
	if mode == "" {
		mode = "live"
	}
	if mode != "live" && mode != "test" {
		return Query{}, ErrInvalid
	}
	zone, err := time.LoadLocation("America/Santiago")
	if err != nil {
		return Query{}, ErrUnavailable
	}
	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, zone)
	return Query{Month: month, Mode: mode, Cursor: cursor, StartAt: start.UTC(), EndAt: start.AddDate(0, 1, 0).UTC()}, nil
}
func (s *Service) List(ctx context.Context, actor, month, mode, cursor string) (Page, error) {
	q, err := QueryFor(month, mode, cursor)
	if err != nil {
		return Page{}, err
	}
	return s.repo.List(ctx, actor, q)
}
func (s *Service) Get(ctx context.Context, actor, id string) (Item, error) {
	return s.repo.Get(ctx, actor, id)
}

// RecordedAmounts uses the ledger's immutable values; it does not backfill a
// theoretical commission over an operation where the recorded fee was waived.
func RecordedAmounts(gross, base, commission, vat, fee int64) (Amounts, string, *int, *int, error) {
	if gross <= 0 || gross != base || commission < 0 || vat < 0 || fee < 0 || commission > base || vat > base || commission > base-vat || fee != commission+vat || fee >= base {
		return Amounts{}, "", nil, nil, ErrUnavailable
	}
	a := Amounts{GrossCLP: strconv.FormatInt(gross, 10), BaseCLP: strconv.FormatInt(base, 10), CommissionCLP: strconv.FormatInt(commission, 10), CommissionVATCLP: strconv.FormatInt(vat, 10), MarketplaceFeeCLP: strconv.FormatInt(fee, 10), AfterMarketplaceFeeCLP: strconv.FormatInt(base-fee, 10)}
	if fee == 0 {
		return a, "no_commission_applied", nil, nil, nil
	}
	expected, err := checkoutpro.CalculateAmounts(base)
	if err == nil && expected.CommissionCLP == commission && expected.VATCLP == vat && expected.MarketplaceFeeCLP == fee {
		rate, vatRate := 3, 19
		return a, "three_percent_plus_vat", &rate, &vatRate, nil
	}
	return a, "recorded_fees", nil, nil, nil
}
