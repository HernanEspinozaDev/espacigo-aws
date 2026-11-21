package checkoutapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

const method = "payments_api"

type Input struct {
	Token                 string `json:"token"`
	PaymentMethodID       string `json:"payment_method_id"`
	IssuerID              string `json:"issuer_id,omitempty"`
	Payer                 Payer  `json:"payer"`
	TokenizationPublicKey string `json:"tokenization_public_key,omitempty"`
}

type Result struct {
	QuoteID            string            `json:"quote_id"`
	AmountCLP          int64             `json:"amount_clp"`
	Status             string            `json:"status"`
	SpaceTitle         string            `json:"space_title,omitempty"`
	StartAt            string            `json:"start_at,omitempty"`
	EndAt              string            `json:"end_at,omitempty"`
	TimeZone           string            `json:"time_zone,omitempty"`
	ExpiresAt          string            `json:"expires_at,omitempty"`
	Units              int64             `json:"units,omitempty"`
	RateUnit           string            `json:"rate_unit,omitempty"`
	PaymentID          string            `json:"payment_id,omitempty"`
	StatusDetail       string            `json:"status_detail,omitempty"`
	ProviderHTTPStatus int               `json:"provider_http_status,omitempty"`
	ProviderCode       string            `json:"provider_code,omitempty"`
	ProviderRequestID  string            `json:"provider_request_id,omitempty"`
	CardTokenization   *CardTokenization `json:"card_tokenization,omitempty"`
}

type CardTokenization struct {
	Mode              string `json:"mode"`
	PublicKey         string `json:"public_key,omitempty"`
	ReconnectRequired bool   `json:"reconnect_required,omitempty"`
}

type sellerPublicKeys interface {
	PublicKey(context.Context, string) (string, bool, error)
}

type Options struct {
	SellerPublicKeyTokenization bool
}

type Repository interface {
	BeginPayment(context.Context, string, string, string, string, []byte, time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error)
	ByQuote(context.Context, string, string) (checkoutpro.Quote, *checkoutpro.Operation, error)
	ClaimPayment(context.Context, string, time.Time) (bool, error)
	checkoutpro.PaymentOutcomeSink
}

type PaymentGateway interface {
	Create(context.Context, []byte, string, PaymentRequest) (Payment, error)
}

type Service struct {
	repo    Repository
	tokens  checkoutpro.SellerTokens
	gateway PaymentGateway
	now     func() time.Time
	newID   func() (string, error)
	log     func(context.Context, ProviderFailure)
	options Options
}

func New(repo Repository, tokens checkoutpro.SellerTokens, gateway PaymentGateway, now func() time.Time, newID func() (string, error), log func(context.Context, ProviderFailure)) (*Service, error) {
	return NewWithOptions(repo, tokens, gateway, now, newID, log, Options{})
}

func NewWithOptions(repo Repository, tokens checkoutpro.SellerTokens, gateway PaymentGateway, now func() time.Time, newID func() (string, error), log func(context.Context, ProviderFailure), options Options) (*Service, error) {
	if repo == nil || tokens == nil || gateway == nil || now == nil || newID == nil {
		return nil, checkoutpro.ErrNotConfigured
	}
	if options.SellerPublicKeyTokenization {
		if _, ok := tokens.(sellerPublicKeys); !ok {
			return nil, checkoutpro.ErrNotConfigured
		}
	}
	return &Service{repo: repo, tokens: tokens, gateway: gateway, now: now, newID: newID, log: log, options: options}, nil
}

func (s *Service) PaymentStatus(ctx context.Context, renter, quoteID string) (Result, error) {
	if ctx == nil || !rentalflow.ValidUUID(renter) || !rentalflow.ValidUUID(quoteID) {
		return Result{}, checkoutpro.ErrInvalid
	}
	q, op, err := s.repo.ByQuote(ctx, renter, quoteID)
	if err != nil {
		return Result{}, err
	}
	result, err := s.current(q, op)
	if err != nil || !s.options.SellerPublicKeyTokenization {
		return result, err
	}
	key, available, err := s.currentSellerPublicKey(ctx, q.HostID)
	if err != nil {
		return Result{}, checkoutpro.ErrProvider
	}
	tokenization := &CardTokenization{Mode: "seller_public_key"}
	if available {
		tokenization.PublicKey = key
	} else {
		tokenization.ReconnectRequired = true
	}
	result.CardTokenization = tokenization
	return result, nil
}

func (s *Service) current(q checkoutpro.Quote, op *checkoutpro.Operation) (Result, error) {
	result := Result{
		QuoteID: q.ID, AmountCLP: q.SubtotalCLP, Status: "not_started",
		SpaceTitle: q.Title, StartAt: q.StartAt.Format(time.RFC3339), EndAt: q.EndAt.Format(time.RFC3339),
		TimeZone: q.TimeZone, ExpiresAt: q.ExpiresAt.Format(time.RFC3339), Units: q.Units, RateUnit: q.RateUnit,
	}
	if op == nil {
		if !q.ExpiresAt.After(s.now()) {
			result.Status = "expired"
		}
		return result, nil
	}
	if op.CheckoutMethod != method {
		return Result{}, checkoutpro.ErrConflict
	}
	result.Status = op.Status
	result.PaymentID = op.ProviderPaymentID
	if result.Status == "pending" && !op.ExpiresAt.After(s.now()) {
		result.Status = "expired"
	}
	return result, nil
}

func (s *Service) Pay(ctx context.Context, renter, quoteID, key string, input Input) (Result, error) {
	if ctx == nil || !rentalflow.ValidUUID(renter) || !rentalflow.ValidUUID(quoteID) || !rentalflow.ValidUUID(key) || !validInput(input) {
		return Result{}, checkoutpro.ErrInvalid
	}
	now := s.now().UTC()
	q, existing, err := s.repo.ByQuote(ctx, renter, quoteID)
	if err != nil {
		s.diagnose(ctx, ProviderFailure{Stage: "quote_load", Code: "quote_unavailable", QuoteID: quoteID})
		return Result{}, err
	}
	s.diagnose(ctx, ProviderFailure{Stage: "quote_verified", Code: "quote_loaded", QuoteID: q.ID, AmountCLP: q.SubtotalCLP})
	if s.options.SellerPublicKeyTokenization {
		currentKey, available, keyErr := s.currentSellerPublicKey(ctx, q.HostID)
		if keyErr != nil || !available || input.TokenizationPublicKey != currentKey {
			s.diagnose(ctx, ProviderFailure{Stage: "seller_public_key", Code: "reconnect_required"})
			return Result{}, checkoutpro.ErrHostNotConnected
		}
	}
	var token []byte
	if existing == nil {
		if !q.ExpiresAt.After(now) {
			return Result{}, checkoutpro.ErrQuoteExpired
		}
		_, err := testPaymentAmounts(q.SubtotalCLP)
		if err != nil || q.Currency != "CLP" {
			return Result{}, checkoutpro.ErrConflict
		}
		token, err = s.tokens.AccessToken(ctx, q.HostID)
		if err != nil {
			s.diagnose(ctx, ProviderFailure{Stage: "seller_context", Code: "host_mp_not_connected", QuoteID: q.ID})
			return Result{}, checkoutpro.ErrHostNotConnected
		}
		s.diagnose(ctx, ProviderFailure{Stage: "seller_context", Code: "oauth_token_available", QuoteID: q.ID, SellerContextPresent: true})
		defer clear(token)
	}
	if s.options.SellerPublicKeyTokenization {
		currentKey, available, keyErr := s.currentSellerPublicKey(ctx, q.HostID)
		if keyErr != nil || !available || input.TokenizationPublicKey != currentKey {
			s.diagnose(ctx, ProviderFailure{Stage: "seller_public_key", Code: "reconnect_required", QuoteID: q.ID, SellerContextPresent: available})
			return Result{}, checkoutpro.ErrHostNotConnected
		}
		s.diagnose(ctx, ProviderFailure{Stage: "seller_public_key", Code: "seller_context_verified", QuoteID: q.ID, SellerContextPresent: true})
	}

	id, err := s.newID()
	if err != nil || !rentalflow.ValidUUID(id) {
		return Result{}, checkoutpro.ErrProvider
	}
	wire, err := json.Marshal(input)
	if err != nil {
		return Result{}, checkoutpro.ErrInvalid
	}
	fingerprint := sha256.Sum256(append([]byte(method+"\n"+quoteID+"\n"), wire...))
	clear(wire)
	q, op, created, err := s.repo.BeginPayment(ctx, renter, quoteID, key, id, fingerprint[:], now)
	if err != nil {
		s.diagnose(ctx, ProviderFailure{Stage: "operation_claim", Code: "operation_begin_failed", QuoteID: quoteID})
		return Result{}, err
	}
	s.diagnose(ctx, ProviderFailure{Stage: "operation_claim", Code: "operation_persisted", QuoteID: q.ID, OperationID: op.ID, AmountCLP: q.SubtotalCLP})
	if op.CheckoutMethod != method || !op.TestMode {
		return Result{}, checkoutpro.ErrConflict
	}
	amounts, err := testPaymentAmounts(q.SubtotalCLP)
	if !created {
		// Idempotent retries keep the fee snapshot from the existing operation.
		// Previously claimed operations cannot be submitted again, and an
		// unclaimed pre-change row (if any) must preserve its original contract.
		amounts = op.Amounts
	}
	if err != nil || q.Currency != "CLP" || q.SubtotalCLP != amounts.BaseCLP || (created && op.Amounts.MarketplaceFeeCLP != 0) {
		return Result{}, checkoutpro.ErrConflict
	}
	result, err := s.current(q, &op)
	if err != nil || op.Status != "pending" || op.ProviderPaymentID != "" {
		return result, err
	}
	if !op.ExpiresAt.After(now) || !q.ExpiresAt.After(now) {
		return Result{}, checkoutpro.ErrQuoteExpired
	}
	if token == nil {
		token, err = s.tokens.AccessToken(ctx, q.HostID)
		if err != nil {
			s.diagnose(ctx, ProviderFailure{Stage: "seller_context", Code: "host_mp_not_connected", QuoteID: q.ID, OperationID: op.ID})
			return Result{}, checkoutpro.ErrHostNotConnected
		}
		s.diagnose(ctx, ProviderFailure{Stage: "seller_context", Code: "oauth_token_available", QuoteID: q.ID, OperationID: op.ID, SellerContextPresent: true})
		defer clear(token)
	}
	claimed, err := s.repo.ClaimPayment(ctx, op.ID, s.now().UTC())
	if err != nil {
		s.diagnose(ctx, ProviderFailure{Stage: "payment_claim", Code: "claim_failed", QuoteID: q.ID, OperationID: op.ID})
		return Result{}, checkoutpro.ErrProvider
	}
	if !claimed {
		s.diagnose(ctx, ProviderFailure{Stage: "payment_claim", Code: "claim_not_acquired", QuoteID: q.ID, OperationID: op.ID})
		return s.PaymentStatus(ctx, renter, quoteID)
	}
	s.diagnose(ctx, ProviderFailure{Stage: "payment_claim", Code: "claim_acquired", QuoteID: q.ID, OperationID: op.ID, AmountCLP: amounts.BaseCLP})
	s.diagnose(ctx, ProviderFailure{Stage: "payment_request", Code: "request_submitted", QuoteID: q.ID, OperationID: op.ID, AmountCLP: amounts.BaseCLP, SellerContextPresent: true})
	payment, err := s.gateway.Create(ctx, token, op.ID, PaymentRequest{
		Token: input.Token, PaymentMethodID: input.PaymentMethodID, IssuerID: input.IssuerID, Payer: input.Payer,
		TransactionAmountCLP: amounts.BaseCLP, ApplicationFeeCLP: amounts.MarketplaceFeeCLP, ExternalReference: op.ID,
		Metadata: map[string]string{"operation_id": op.ID},
	})
	if err != nil {
		failure := &ProviderFailure{Stage: "payment_create", Code: "payment_result_unknown"}
		var providerErr *ProviderFailure
		if errors.As(err, &providerErr) {
			failure = providerErr
		}
		failure.QuoteID, failure.OperationID, failure.AmountCLP = q.ID, op.ID, amounts.BaseCLP
		s.diagnose(ctx, *failure)
		result.ProviderHTTPStatus = failure.HTTPStatus
		result.ProviderCode = failure.Code
		result.ProviderRequestID = failure.RequestID
		result.Status = "reconciliation_required"
		if failure.HTTPStatus >= 400 && failure.HTTPStatus < 500 && failure.HTTPStatus != 408 && failure.HTTPStatus != 409 && failure.HTTPStatus != 429 {
			result.Status = "provider_error"
		}
		return result, nil
	}

	result.StatusDetail = payment.StatusDetail
	result.ProviderRequestID = payment.RequestID
	result.PaymentID = payment.Verified.ProviderPaymentID
	s.diagnose(ctx, ProviderFailure{Stage: "payment_response", Code: "provider_response_received", QuoteID: q.ID, OperationID: op.ID, PaymentID: result.PaymentID, RequestID: payment.RequestID, HTTPStatus: payment.HTTPStatus, AmountCLP: amounts.BaseCLP})
	if !payment.LiveModePresent || payment.Verified.OperationID != op.ID || payment.Verified.ExternalReference != op.ID || payment.Verified.LiveMode {
		s.diagnose(ctx, ProviderFailure{Stage: "payment_verify", Code: "payment_facts_mismatch", QuoteID: q.ID, OperationID: op.ID, PaymentID: result.PaymentID, RequestID: payment.RequestID})
		result.Status = "reconciliation_required"
		return result, nil
	}
	payment.Verified.ApplicationFeeCLP = payment.ApplicationFeeCLP
	if _, err := s.repo.ApplyVerifiedPayment(ctx, payment.Verified); err != nil {
		s.diagnose(ctx, ProviderFailure{Stage: "payment_verify", Code: "payment_persistence_unavailable", QuoteID: q.ID, OperationID: op.ID, PaymentID: result.PaymentID, RequestID: payment.RequestID})
		result.Status = "reconciliation_required"
		return result, nil
	}
	s.diagnose(ctx, ProviderFailure{Stage: "payment_verify", Code: "payment_verified_and_persisted", QuoteID: q.ID, OperationID: op.ID, PaymentID: result.PaymentID, RequestID: payment.RequestID})
	state, err := s.PaymentStatus(ctx, renter, quoteID)
	if err != nil {
		result.Status = "reconciliation_required"
		return result, nil
	}
	state.StatusDetail = result.StatusDetail
	state.ProviderRequestID = result.ProviderRequestID
	return state, nil
}

func testPaymentAmounts(base int64) (checkoutpro.Amounts, error) {
	if base <= 0 {
		return checkoutpro.Amounts{}, checkoutpro.ErrInvalid
	}
	return checkoutpro.Amounts{BaseCLP: base}, nil
}

func (s *Service) diagnose(ctx context.Context, diagnostic ProviderFailure) {
	if s.log != nil {
		s.log(ctx, diagnostic)
	}
}

func (s *Service) currentSellerPublicKey(ctx context.Context, hostID string) (string, bool, error) {
	provider, ok := s.tokens.(sellerPublicKeys)
	if !ok {
		return "", false, checkoutpro.ErrNotConfigured
	}
	key, available, err := provider.PublicKey(ctx, hostID)
	if err != nil || !available || validSellerPublicKey(key) == "" {
		return "", false, err
	}
	return key, true, nil
}

var safeToken = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)
var safeMethod = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
var safeIssuer = regexp.MustCompile(`^[0-9]{1,24}$`)
var safeSellerPublicKey = regexp.MustCompile(`^(TEST-|APP_USR-)[A-Za-z0-9_-]{16,120}$`)

func validSellerPublicKey(key string) string {
	if !safeSellerPublicKey.MatchString(key) {
		return ""
	}
	return key
}

func validInput(input Input) bool {
	if !safeToken.MatchString(input.Token) || !safeMethod.MatchString(input.PaymentMethodID) || input.PaymentMethodID == "account_money" || input.IssuerID != "" && !safeIssuer.MatchString(input.IssuerID) || input.TokenizationPublicKey != "" && validSellerPublicKey(input.TokenizationPublicKey) == "" {
		return false
	}
	return validPayer(input.Payer)
}
