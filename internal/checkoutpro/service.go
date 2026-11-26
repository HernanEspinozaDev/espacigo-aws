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
	"unicode/utf8"
)

var (
	ErrInvalid                  = errors.New("checkoutpro: invalid request")
	ErrNotFound                 = errors.New("checkoutpro: quote not found")
	ErrConflict                 = errors.New("checkoutpro: quote unavailable or idempotency conflict")
	ErrIdempotencyConflict      = errors.New("checkoutpro: idempotency key conflict")
	ErrQuoteExpired             = errors.New("checkoutpro: quote or checkout expired")
	ErrNotConfigured            = errors.New("checkoutpro: provider not configured")
	ErrHostNotConnected         = errors.New("checkoutpro: seller account is not connected")
	ErrPaymentMethodUnavailable = errors.New("checkoutpro: required payment method unavailable")
	ErrProvider                 = errors.New("checkoutpro: provider unavailable")
	ErrWalletPurchaseDisabled   = errors.New("checkoutpro: wallet purchase is disabled")
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
	CheckoutMethod, ProviderPaymentID            string
	ExpiresAt, UpdatedAt                         time.Time
	Amounts                                      Amounts `json:"-"`
	TestMode                                     bool    `json:"-"`
}

type Amounts struct {
	BaseCLP            int64 `json:"base_clp"`
	CommissionCLP      int64 `json:"commission_clp"`
	VATCLP             int64 `json:"vat_clp"`
	BuyerCommissionCLP int64 `json:"buyer_commission_clp"`
	BuyerVATCLP        int64 `json:"buyer_vat_clp"`
	BuyerFeeCLP        int64 `json:"buyer_fee_clp"`
	TotalChargedCLP    int64 `json:"total_charged_clp"`
	MarketplaceFeeCLP  int64 `json:"marketplace_fee_clp"`
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
type paymentSearcher interface {
	FindPaymentByExternalReference(context.Context, []byte, string) (VerifiedPayment, error)
}
type Gateway interface {
	PaymentTypes(context.Context, []byte) ([]string, error)
	Create(context.Context, []byte, PreferenceRequest) (Preference, error)
	FindByExternalReference(context.Context, []byte, string) (Preference, error)
}
type Config struct {
	BuyerForPreference    func(context.Context, string) (PreferencePayer, error)
	FrontendOrigin        string
	TestMode              bool
	WalletPurchaseEnabled bool
	Diagnostics           func(context.Context, Diagnostic)
}

// Diagnostic contains only fixed stage/reason values and provider metadata that
// has passed the HTTP adapter allowlist. It must never contain provider bodies,
// credentials, preference URLs, or user supplied values.
type Diagnostic struct {
	Stage        string
	Reason       string
	HTTPStatus   int
	ProviderCode string
}

type ProviderError struct {
	Stage        string
	HTTPStatus   int
	ProviderCode string
}

func (e *ProviderError) Error() string { return "checkoutpro: provider unavailable" }
func (e *ProviderError) Unwrap() error { return ErrProvider }

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
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, ErrNotConfigured
	}
	config.FrontendOrigin = (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
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
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

func CalculateAmounts(base int64) (Amounts, error) {
	if base <= 0 || base > math.MaxInt64/357 {
		return Amounts{}, ErrInvalid
	}
	// Round half up independently: 3% commission, then 19% IVA on commission,
	// applied symmetrically to host and buyer sides.
	commission := (base*3 + 50) / 100
	vat := (commission*19 + 50) / 100
	hostFee := commission + vat

	buyerCommission := (base*3 + 50) / 100
	buyerVAT := (buyerCommission*19 + 50) / 100
	buyerFee := buyerCommission + buyerVAT

	return Amounts{
		BaseCLP:            base,
		CommissionCLP:      commission,
		VATCLP:             vat,
		BuyerCommissionCLP: buyerCommission,
		BuyerVATCLP:        buyerVAT,
		BuyerFeeCLP:        buyerFee,
		TotalChargedCLP:    base + buyerFee,
		MarketplaceFeeCLP:  hostFee + buyerFee,
	}, nil
}

// AllowedPaymentType constrains payment methods by the operation's persisted
// mode. Split production accepts only account money. Test operations may also
// accept card types so test buyers can use test cards.
func AllowedPaymentType(_ bool, paymentType string) bool {
	return paymentType == "account_money" || isTestCardType(paymentType)
}

// PaymentModeMatches requires test preferences to reconcile only a non-live
// payment, and live preferences only a live payment.
func PaymentModeMatches(testMode, liveMode bool) bool {
	return testMode != liveMode
}

func isTestCardType(paymentType string) bool {
	switch paymentType {
	case "credit_card", "debit_card", "prepaid_card":
		return true
	default:
		return false
	}
}

func (s *Service) Start(ctx context.Context, renter, quoteID, key string) (StartResult, error) {
	return s.start(ctx, renter, quoteID, key, false)
}

// StartWalletPurchase creates a preference that requires the buyer to sign in
// to their Mercado Pago account before completing checkout.
func (s *Service) StartWalletPurchase(ctx context.Context, renter, quoteID, key string) (StartResult, error) {
	if !s.config.WalletPurchaseEnabled || !s.config.TestMode {
		return StartResult{}, ErrWalletPurchaseDisabled
	}
	return s.start(ctx, renter, quoteID, key, true)
}

func (s *Service) start(ctx context.Context, renter, quoteID, key string, walletPurchase bool) (StartResult, error) {
	if ctx == nil || !validUUID(renter) || !validUUID(quoteID) || strings.TrimSpace(key) == "" || len(key) > 200 {
		return StartResult{}, ErrInvalid
	}
	operationID := s.newID()
	if !validUUID(operationID) {
		return StartResult{}, ErrProvider
	}
	flow := "checkoutpro-preference-v1\n"
	if walletPurchase {
		flow = "checkoutpro-wallet-purchase-v1\n"
	}
	fingerprint := sha256.Sum256([]byte(flow + quoteID))
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
		s.diagnose(ctx, "seller_token", "unavailable", err)
		return StartResult{}, ErrHostNotConnected
	}
	defer clear(token)
	paymentTypes, err := s.gateway.PaymentTypes(ctx, token)
	if err != nil {
		s.diagnose(ctx, "payment_methods", "provider_unavailable", err)
		return StartResult{}, ErrProvider
	}
	excluded := make([]ExcludedPaymentType, 0, len(paymentTypes))
	seenTypes := map[string]bool{}
	foundProdMethod := false
	foundTestCard := false
	for _, paymentType := range paymentTypes {
		paymentType = strings.TrimSpace(paymentType)
		if paymentType == "" || len(paymentType) > 64 {
			s.diagnose(ctx, "payment_methods", "invalid_response", nil)
			return StartResult{}, ErrProvider
		}
		if s.config.TestMode {
			if paymentType == "account_money" {
				continue
			}
			if isTestCardType(paymentType) {
				foundTestCard = true
				continue
			}
			if !seenTypes[paymentType] {
				excluded = append(excluded, ExcludedPaymentType{ID: paymentType})
				seenTypes[paymentType] = true
			}
			continue
		}
		foundProdMethod = true
	}
	if s.config.TestMode && !foundTestCard {
		s.diagnose(ctx, "payment_methods", "test_card_unavailable", nil)
		return StartResult{}, ErrPaymentMethodUnavailable
	}
	if !s.config.TestMode && !foundProdMethod {
		s.diagnose(ctx, "payment_methods", "payment_methods_unavailable", nil)
		return StartResult{}, ErrPaymentMethodUnavailable
	}
	returnURL := s.config.FrontendOrigin + "/reservas/checkout/" + quote.ID
	notificationURL := s.config.FrontendOrigin + "/api/v1/webhooks/mercadopago"
	title := cleanTitle(quote.Title)
	request := PreferenceRequest{
		Purpose: func() string {
			if walletPurchase {
				return "wallet_purchase"
			}
			return ""
		}(),
		ExternalReference: op.ID, MarketplaceFee: float64(amounts.MarketplaceFeeCLP),
		StatementDescriptor: "ESPACIGO",
		Items: []PreferenceItem{{
			ID: quote.SpaceID, Title: title, Description: "Arriendo de espacio en EspaciGo · " + title,
			CategoryID: "services", Quantity: 1, UnitPrice: float64(amounts.TotalChargedCLP), CurrencyID: "CLP",
		}},
		Metadata:        map[string]any{"quote_id": quote.ID, "operation_id": op.ID, "space_id": quote.SpaceID, "host_id": quote.HostID, "renter_id": quote.RenterID, "commission_base_clp": amounts.BaseCLP, "commission_clp": amounts.CommissionCLP, "vat_clp": amounts.VATCLP, "host_fee_clp": amounts.CommissionCLP + amounts.VATCLP, "buyer_fee_clp": amounts.BuyerFeeCLP, "total_charged_clp": amounts.TotalChargedCLP, "marketplace_fee_clp": amounts.MarketplaceFeeCLP},
		BackURLs:        BackURLs{Success: returnURL, Failure: returnURL, Pending: returnURL},
		AutoReturn:      "approved",
		NotificationURL: notificationURL,
		Expires:         true, ExpirationDateFrom: now.Format(time.RFC3339), ExpirationDateTo: quote.ExpiresAt.UTC().Format(time.RFC3339),
		PaymentMethods: PaymentMethods{ExcludedPaymentTypes: excluded, Installments: 1},
	}
	// TEST checkouts use Mercado Pago test buyers, without EspaciGo account data.
	if !s.config.TestMode && s.config.BuyerForPreference != nil {
		payer, err := s.config.BuyerForPreference(ctx, renter)
		if err != nil {
			s.diagnose(ctx, "buyer_preference", "account_unavailable", nil)
			return StartResult{}, ErrProvider
		}
		request.Payer = &payer
	}
	claimed, claimErr := s.repo.ClaimPreference(ctx, op.ID, now, 2*time.Minute)
	if claimErr != nil {
		s.diagnose(ctx, "claim_preference", "repository_unavailable", nil)
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
		} else if err != nil {
			s.diagnose(ctx, "preference_lookup", "provider_unavailable", err)
		}
	}
	if err == nil && preference.ID == "" {
		preference, err = s.gateway.Create(ctx, token, request)
		if err != nil {
			s.diagnose(ctx, "preference_create", "provider_unavailable", err)
		}
	}
	if err != nil {
		// A timeout may have reached the provider. Search by our durable external reference.
		preference, err = s.gateway.FindByExternalReference(ctx, token, op.ID)
		if err != nil {
			s.diagnose(ctx, "preference_lookup", "provider_unavailable", err)
			return StartResult{}, ErrProvider
		}
	}
	if preference.ID == "" || !validInitPoint(preference.InitPoint, s.config.TestMode) {
		s.diagnose(ctx, "preference_response", "invalid_preference", nil)
		return StartResult{}, ErrProvider
	}
	if err = s.repo.SavePreference(ctx, op.ID, preference.ID, preference.InitPoint, now); err != nil {
		s.diagnose(ctx, "save_preference", "repository_unavailable", nil)
		return StartResult{}, ErrProvider
	}
	op.PreferenceID, op.InitPoint = preference.ID, preference.InitPoint
	return startResult(quote, op), nil
}

func (s *Service) diagnose(ctx context.Context, stage, reason string, err error) {
	if s.config.Diagnostics == nil {
		return
	}
	d := Diagnostic{Stage: stage, Reason: reason}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		d.HTTPStatus = providerErr.HTTPStatus
		d.ProviderCode = providerErr.ProviderCode
	}
	s.config.Diagnostics(ctx, d)
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
	if op.CheckoutMethod != "payments_api" && (op.Status == "pending" || (op.Status == "expired" && op.ProviderPaymentID == "") || op.Status == "reconciliation_required") {
		if searcher, hasSearcher := s.gateway.(paymentSearcher); hasSearcher {
			if sink, hasSink := s.repo.(PaymentOutcomeSink); hasSink {
				if token, tokenErr := s.tokens.AccessToken(ctx, quote.HostID); tokenErr == nil && len(token) > 0 {
					verified, searchErr := searcher.FindPaymentByExternalReference(ctx, token, op.ID)
					clear(token)
					if searchErr == nil {
						if verified.OperationID == "" {
							verified.OperationID = op.ID
						}
						if _, applyErr := sink.ApplyVerifiedPayment(ctx, verified); applyErr == nil {
							if refreshedQuote, refreshedOp, reloadErr := s.repo.ByQuote(ctx, renter, quoteID); reloadErr == nil && refreshedOp != nil {
								quote, op = refreshedQuote, refreshedOp
							}
						}
					}
				}
			}
		}
	}
	if op.Status == "pending" && !op.ExpiresAt.After(s.now().UTC()) {
		return StatusResult{QuoteID: quote.ID, OperationID: &op.ID, ReservationID: &op.ID, Status: "expired", UpdatedAt: &op.UpdatedAt}, nil
	}
	id, updated, expiresAt := op.ID, op.UpdatedAt, op.ExpiresAt
	result := StatusResult{QuoteID: quote.ID, OperationID: &id, ReservationID: &id, Status: op.Status, UpdatedAt: &updated}
	if op.Status == "pending" {
		result.ExpiresAt = &expiresAt
		if validInitPoint(op.InitPoint, s.config.TestMode) {
			result.InitPoint = op.InitPoint
		}
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
		end := 200
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
		s = strings.TrimSpace(s[:end])
	}
	if s == "" {
		return "Arriendo EspaciGo"
	}
	return s
}

func (s *Service) String() string { return fmt.Sprintf("CheckoutPro(test=%v)", s.config.TestMode) }
