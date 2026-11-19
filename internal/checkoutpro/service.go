// Package checkoutpro starts durable Checkout Pro preferences for commercial
// rental quotes. Provider redirects are never treated as payment evidence.
package checkoutpro

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalid             = errors.New("checkoutpro: invalid request")
	ErrNotFound            = errors.New("checkoutpro: quote not found")
	ErrConflict            = errors.New("checkoutpro: quote unavailable or idempotency conflict")
	ErrIdempotencyConflict = errors.New("checkoutpro: idempotency key conflict")
	ErrQuoteExpired        = errors.New("checkoutpro: quote or checkout expired")
	ErrNotConfigured       = errors.New("checkoutpro: provider not configured")
	ErrHostNotConnected    = errors.New("checkoutpro: seller account is not connected")
	ErrProvider            = errors.New("checkoutpro: provider unavailable")
)

type Quote struct {
	ID, SpaceID, HostID, RenterID, Title, Currency string
	UnitPriceCLP, Units, SubtotalCLP               int64
	RateVersion                                    int64
	RateUnit                                       string
	StartAt, EndAt, ExpiresAt                      time.Time
	TimeZone                                       string
}

type Operation struct {
	ID, QuoteID, Status, PreferenceID, InitPoint string
	ExpiresAt, UpdatedAt                         time.Time
	Amounts                                      Amounts `json:"-"`
	TestMode                                     bool    `json:"-"`
}

type Amounts struct {
	BaseCLP           int64 `json:"base_clp"`
	CommissionCLP     int64 `json:"commission_clp"`
	VATCLP            int64 `json:"vat_clp"`
	MarketplaceFeeCLP int64 `json:"marketplace_fee_clp"`
}

// Repository must atomically acquire an exclusive time-range hold for the
// quote's space, persist the idempotency fingerprint and return any existing
// operation. It must ignore/release expired holds and reject overlapping ones.
type Repository interface {
	Begin(context.Context, string, string, string, string, []byte, bool, time.Time) (Quote, Operation, bool, error)
	ClaimPreference(context.Context, string, time.Time, time.Duration) (bool, error)
	SavePreference(context.Context, string, string, string, time.Time) error // final argument is the application clock
	ByQuote(context.Context, string, string) (Quote, *Operation, error)
}

type SellerTokens interface {
	AccessToken(context.Context, string) ([]byte, error)
}
type Gateway interface {
	PaymentTypes(context.Context, []byte) ([]string, error)
	Create(context.Context, []byte, PreferenceRequest) (Preference, error)
	FindByExternalReference(context.Context, []byte, string) (Preference, error)
}
type Config struct {
	FrontendOrigin string
	TestMode       bool
}
type Service struct {
	repo    Repository
	tokens  SellerTokens
	gateway Gateway
	config  Config
	now     func() time.Time
	newID   func() string
}

func New(repo Repository, tokens SellerTokens, gateway Gateway, config Config, now func() time.Time, newID func() string) (*Service, error) {
	if repo == nil || tokens == nil || gateway == nil || now == nil || newID == nil {
		return nil, ErrInvalid
	}
	u, err := url.Parse(config.FrontendOrigin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, ErrNotConfigured
	}
	return &Service{repo: repo, tokens: tokens, gateway: gateway, config: config, now: now, newID: newID}, nil
}

type StartResult struct {
	QuoteID       string    `json:"quote_id"`
	OperationID   string    `json:"operation_id"`
	ReservationID string    `json:"reservation_id"`
	Status        string    `json:"status"`
	InitPoint     string    `json:"init_point"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type StatusResult struct {
	QuoteID       string     `json:"quote_id"`
	OperationID   *string    `json:"operation_id,omitempty"`
	ReservationID *string    `json:"reservation_id,omitempty"`
	Status        string     `json:"status"`
	InitPoint     string     `json:"init_point,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

func CalculateAmounts(base int64) (Amounts, error) {
	if base <= 0 || base > math.MaxInt64/357 {
		return Amounts{}, ErrInvalid
	}
	// Round half up independently: 3% commission, then 19% IVA on commission.
	commission := (base*3 + 50) / 100
	vat := (commission*19 + 50) / 100
	return Amounts{BaseCLP: base, CommissionCLP: commission, VATCLP: vat, MarketplaceFeeCLP: commission + vat}, nil
}

func (s *Service) Start(ctx context.Context, renter, quoteID, key string) (StartResult, error) {
	if ctx == nil || !validUUID(renter) || !validUUID(quoteID) || strings.TrimSpace(key) == "" || len(key) > 200 {
		return StartResult{}, ErrInvalid
	}
	operationID := s.newID()
	if !validUUID(operationID) {
		return StartResult{}, ErrProvider
	}
	fingerprint := sha256.Sum256([]byte("checkoutpro-preference-v1\n" + quoteID))
	now := s.now().UTC()
	quote, op, created, err := s.repo.Begin(ctx, renter, quoteID, key, operationID, fingerprint[:], s.config.TestMode, now)
	if err != nil {
		return StartResult{}, err
	}
	if !op.ExpiresAt.After(now) {
		return StartResult{}, ErrQuoteExpired
	}
	if op.Status == "pending" && op.PreferenceID != "" && validInitPoint(op.InitPoint, s.config.TestMode) {
		return startResult(quote, op), nil
	}
	if op.Status != "pending" {
		return StartResult{}, ErrConflict
	}
	amounts, err := CalculateAmounts(quote.SubtotalCLP)
	if err != nil {
		return StartResult{}, ErrConflict
	}
	token, err := s.tokens.AccessToken(ctx, quote.HostID)
	if err != nil {
		return StartResult{}, ErrHostNotConnected
	}
	defer clear(token)
	paymentTypes, err := s.gateway.PaymentTypes(ctx, token)
	if err != nil {
		return StartResult{}, ErrProvider
	}
	excluded := make([]ExcludedPaymentType, 0, len(paymentTypes))
	seenTypes := map[string]bool{}
	foundAccountMoney := false
	for _, paymentType := range paymentTypes {
		paymentType = strings.TrimSpace(paymentType)
		if paymentType == "account_money" {
			foundAccountMoney = true
			continue
		}
		if paymentType == "" || len(paymentType) > 64 {
			return StartResult{}, ErrProvider
		}
		if !seenTypes[paymentType] {
			excluded = append(excluded, ExcludedPaymentType{ID: paymentType})
			seenTypes[paymentType] = true
		}
	}
	if !foundAccountMoney {
		return StartResult{}, ErrProvider
	}
	returnURL := s.config.FrontendOrigin + "/reservas/checkout/" + quote.ID
	request := PreferenceRequest{
		ExternalReference: op.ID, MarketplaceFee: float64(amounts.MarketplaceFeeCLP),
		Items:    []PreferenceItem{{Title: cleanTitle(quote.Title), Quantity: 1, UnitPrice: float64(quote.SubtotalCLP), CurrencyID: "CLP"}},
		Metadata: map[string]any{"quote_id": quote.ID, "operation_id": op.ID, "space_id": quote.SpaceID, "host_id": quote.HostID, "renter_id": quote.RenterID, "commission_base_clp": amounts.BaseCLP, "commission_clp": amounts.CommissionCLP, "vat_clp": amounts.VATCLP, "marketplace_fee_clp": amounts.MarketplaceFeeCLP},
		BackURLs: BackURLs{Success: returnURL, Failure: returnURL, Pending: returnURL},
		Expires:  true, ExpirationDateFrom: now.Format(time.RFC3339), ExpirationDateTo: quote.ExpiresAt.UTC().Format(time.RFC3339),
		PaymentMethods: PaymentMethods{ExcludedPaymentTypes: excluded, Installments: 1},
	}
	claimed, claimErr := s.repo.ClaimPreference(ctx, op.ID, now, 2*time.Minute)
	if claimErr != nil {
		return StartResult{}, ErrProvider
	}
	if !claimed {
		q, current, readErr := s.repo.ByQuote(ctx, renter, quoteID)
		if readErr == nil && current != nil && current.PreferenceID != "" && validInitPoint(current.InitPoint, s.config.TestMode) {
			return startResult(q, *current), nil
		}
		return StartResult{}, ErrProvider
	}
	var preference Preference
	if !created {
		preference, err = s.gateway.FindByExternalReference(ctx, token, op.ID)
		if errors.Is(err, ErrNotFound) {
			err = nil
		}
	}
	if err == nil && preference.ID == "" {
		preference, err = s.gateway.Create(ctx, token, request)
	}
	if err != nil {
		// A timeout may have reached the provider. Search by our durable external reference.
		preference, err = s.gateway.FindByExternalReference(ctx, token, op.ID)
		if err != nil {
			return StartResult{}, ErrProvider
		}
	}
	if preference.ID == "" || !validInitPoint(preference.InitPoint, s.config.TestMode) {
		return StartResult{}, ErrProvider
	}
	if err = s.repo.SavePreference(ctx, op.ID, preference.ID, preference.InitPoint, now); err != nil {
		return StartResult{}, ErrProvider
	}
	op.PreferenceID, op.InitPoint = preference.ID, preference.InitPoint
	return startResult(quote, op), nil
}

func (s *Service) Status(ctx context.Context, renter, quoteID string) (StatusResult, error) {
	if ctx == nil || !validUUID(renter) || !validUUID(quoteID) {
		return StatusResult{}, ErrInvalid
	}
	quote, op, err := s.repo.ByQuote(ctx, renter, quoteID)
	if err != nil {
		return StatusResult{}, err
	}
	if op == nil {
		return StatusResult{QuoteID: quote.ID, Status: "not_started"}, nil
	}
	if op.Status == "pending" && !op.ExpiresAt.After(s.now().UTC()) {
		return StatusResult{QuoteID: quote.ID, OperationID: &op.ID, ReservationID: &op.ID, Status: "expired", UpdatedAt: &op.UpdatedAt}, nil
	}
	id, updated := op.ID, op.UpdatedAt
	result := StatusResult{QuoteID: quote.ID, OperationID: &id, ReservationID: &id, Status: op.Status, UpdatedAt: &updated}
	if op.Status == "pending" && validInitPoint(op.InitPoint, s.config.TestMode) {
		result.InitPoint = op.InitPoint
	}
	return result, nil
}

func startResult(q Quote, o Operation) StartResult {
	return StartResult{QuoteID: q.ID, OperationID: o.ID, ReservationID: o.ID, Status: o.Status, InitPoint: o.InitPoint, ExpiresAt: o.ExpiresAt}
}
func validUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range v {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
func validInitPoint(v string, _ bool) bool {
	u, e := url.Parse(v)
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.Fragment != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "www.mercadopago.com" || h == "www.mercadopago.cl"
}
func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	if s == "" {
		return "Arriendo EspaciGo"
	}
	return s
}

func (s *Service) String() string { return fmt.Sprintf("CheckoutPro(test=%v)", s.config.TestMode) }
