// Package checkoutapi contains the Mercado Pago Payments API transport for
// Marketplace Checkout API. It owns no policy about seller eligibility; the
// caller must supply an OAuth seller token from the verified local service.
package checkoutapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

const providerBaseURL = "https://api.mercadopago.com"

var (
	ErrInvalidRequest = errors.New("checkoutapi: invalid request")
	ErrProvider       = errors.New("checkoutapi: provider unavailable")

	operationIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	identifierPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	requestIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,100}$`)
	digitsPattern      = regexp.MustCompile(`^[0-9]{1,40}$`)
)

type Identification struct {
	Type   string `json:"type"`
	Number string `json:"number"`
}

type Payer struct {
	Email          string          `json:"email"`
	Identification *Identification `json:"identification,omitempty"`
}

// PaymentRequest contains only the single-use payment token and payer fields
// collected by MercadoPago.js. The service fills the amount, fee, metadata,
// and reference from its durable quote and operation.
type PaymentRequest struct {
	Token                string            `json:"-"`
	PaymentMethodID      string            `json:"-"`
	IssuerID             string            `json:"-"`
	Payer                Payer             `json:"-"`
	TransactionAmountCLP int64             `json:"-"`
	ApplicationFeeCLP    int64             `json:"-"`
	ExternalReference    string            `json:"-"`
	Metadata             map[string]string `json:"-"`
}

// Payment is a sanitized projection of the provider response. A nil
// ApplicationFeeCLP means the provider did not return a verifiable fee field.
// LiveModePresent distinguishes an explicit false from an absent field.
type Payment struct {
	Verified              checkoutpro.VerifiedPayment
	HTTPStatus            int
	StatusDetail          string
	RequestID             string
	ApplicationFeeCLP     *int64
	ApplicationFeePresent bool
	LiveModePresent       bool
}

// Gateway creates Payments API payments using the OAuth access token of the
// connected seller. operationID is the stable local UUID used both as the
// provider idempotency key and, by contract, as request.ExternalReference.
type Gateway interface {
	Create(ctx context.Context, sellerAccessToken []byte, operationID string, request PaymentRequest) (Payment, error)
}

type ProviderFailure struct {
	Stage                string
	QuoteID              string
	OperationID          string
	PaymentID            string
	AmountCLP            int64
	SellerContextPresent bool
	HTTPStatus           int
	Code                 string
	RequestID            string
	ProviderError        string
	CauseCount           int
	CauseCodes           []string
	CauseCategories      []string
	UnknownFieldCount    int
	BodyTruncated        bool
	RequestFacts         RequestFacts
}

// RequestFacts contains presence-only facts about the outgoing request. It
// deliberately excludes all values that can identify a payer, card, or token.
type RequestFacts struct {
	SellerTokenPresent bool
	CardTokenPresent   bool
	PaymentMethodSet   bool
	IssuerSet          bool
	PayerEmailSet      bool
	IdentificationSet  bool
	ApplicationFeeSet  bool
}

func (e *ProviderFailure) Error() string { return "checkoutapi: provider unavailable" }

// HTTPGateway performs only the fixed Payments API request. Its production
// base URL is constant; the unexported constructor permits httptest injection.
type HTTPGateway struct {
	baseURL string
	client  *http.Client
}

func NewGateway() *HTTPGateway { return newGateway(providerBaseURL, nil) }

func newGateway(baseURL string, client *http.Client) *HTTPGateway {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	} else {
		copy := *client
		client = &copy
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTPGateway{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func (g *HTTPGateway) Create(ctx context.Context, sellerAccessToken []byte, idempotencyKey string, p PaymentRequest) (Payment, error) {
	if ctx == nil || g == nil || g.client == nil || !operationIDPattern.MatchString(idempotencyKey) || !operationIDPattern.MatchString(p.ExternalReference) || !bytesNonBlank(sellerAccessToken) {
		return Payment{}, ErrInvalidRequest
	}
	if p.TransactionAmountCLP <= 0 || p.ApplicationFeeCLP < 0 || p.ApplicationFeeCLP >= p.TransactionAmountCLP ||
		len(p.Token) == 0 || len(p.Token) > 4096 || strings.TrimSpace(p.Token) != p.Token || strings.ContainsAny(p.Token, "\r\n\x00") ||
		!identifierPattern.MatchString(p.PaymentMethodID) ||
		(p.IssuerID != "" && !digitsPattern.MatchString(p.IssuerID)) || !validPayer(p.Payer) || !validMetadata(p.Metadata) {
		return Payment{}, ErrInvalidRequest
	}
	var applicationFee *int64
	if p.ApplicationFeeCLP > 0 {
		fee := p.ApplicationFeeCLP
		applicationFee = &fee
	}
	payload := struct {
		Token             string            `json:"token"`
		PaymentMethodID   string            `json:"payment_method_id"`
		IssuerID          string            `json:"issuer_id,omitempty"`
		Installments      int               `json:"installments"`
		BinaryMode        bool              `json:"binary_mode"`
		Payer             Payer             `json:"payer"`
		TransactionAmount int64             `json:"transaction_amount"`
		ApplicationFee    *int64            `json:"application_fee,omitempty"`
		ExternalReference string            `json:"external_reference"`
		Metadata          map[string]string `json:"metadata,omitempty"`
	}{
		Token: p.Token, PaymentMethodID: p.PaymentMethodID, IssuerID: p.IssuerID,
		Installments: 1, BinaryMode: false, Payer: p.Payer, TransactionAmount: p.TransactionAmountCLP,
		ApplicationFee: applicationFee, ExternalReference: p.ExternalReference, Metadata: p.Metadata,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Payment{}, ErrInvalidRequest
	}
	defer clear(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/payments", bytes.NewReader(body))
	if err != nil {
		return Payment{}, ErrInvalidRequest
	}
	req.Header.Set("Authorization", "Bearer "+string(sellerAccessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Idempotency-Key", idempotencyKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return Payment{}, &ProviderFailure{Stage: "payment_create", Code: "transport_error"}
	}
	defer resp.Body.Close()
	requestID := safeRequestID(resp.Header.Get("x-request-id"))
	const maxProviderBody = 64 << 10
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody+1))
	if readErr != nil {
		return Payment{}, &ProviderFailure{Stage: "payment_create", HTTPStatus: safeHTTPStatus(resp.StatusCode), Code: "invalid_response", RequestID: requestID}
	}
	bodyTruncated := len(responseBody) > maxProviderBody
	if bodyTruncated {
		responseBody = responseBody[:maxProviderBody]
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		diagnostic := providerDiagnostics(responseBody)
		return Payment{}, &ProviderFailure{
			Stage: "payment_create", HTTPStatus: safeHTTPStatus(resp.StatusCode), Code: diagnostic.Code,
			RequestID: requestID, ProviderError: diagnostic.ProviderError, CauseCount: diagnostic.CauseCount,
			CauseCodes: diagnostic.CauseCodes, CauseCategories: diagnostic.CauseCategories,
			UnknownFieldCount: diagnostic.UnknownFieldCount, BodyTruncated: bodyTruncated,
			RequestFacts: requestFacts(sellerAccessToken, p),
		}
	}
	payment, err := parsePayment(responseBody, requestID)
	if err != nil {
		return Payment{}, &ProviderFailure{Stage: "payment_create", HTTPStatus: safeHTTPStatus(resp.StatusCode), Code: "invalid_response", RequestID: requestID}
	}
	payment.HTTPStatus = safeHTTPStatus(resp.StatusCode)
	return payment, nil
}

func validPayer(p Payer) bool {
	if len(p.Email) == 0 || len(p.Email) > 254 || strings.TrimSpace(p.Email) != p.Email || strings.ContainsAny(p.Email, "\r\n\x00") {
		return false
	}
	parsed, err := mail.ParseAddress(p.Email)
	if err != nil || parsed.Address != p.Email {
		return false
	}
	if p.Identification == nil {
		return true
	}
	identification := *p.Identification
	return identifierPattern.MatchString(identification.Type) && len(identification.Number) >= 1 && len(identification.Number) <= 40 && !strings.ContainsAny(identification.Number, "\r\n\x00")
}

func validMetadata(metadata map[string]string) bool {
	if len(metadata) > 32 {
		return false
	}
	for key, value := range metadata {
		if !identifierPattern.MatchString(key) || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
			return false
		}
	}
	return true
}

func bytesNonBlank(value []byte) bool {
	if len(value) == 0 || len(bytes.TrimSpace(value)) != len(value) {
		return false
	}
	for _, char := range value {
		if char <= 0x20 || char >= 0x7f {
			return false
		}
	}
	return true
}

func safeHTTPStatus(status int) int {
	if status < 100 || status > 599 {
		return 0
	}
	return status
}

func safeRequestID(value string) string {
	if requestIDPattern.MatchString(value) {
		return value
	}
	return ""
}

type paymentWire struct {
	ID                json.RawMessage `json:"id"`
	CollectorID       json.RawMessage `json:"collector_id"`
	ExternalReference string          `json:"external_reference"`
	Status            string          `json:"status"`
	StatusDetail      string          `json:"status_detail"`
	TransactionAmount json.RawMessage `json:"transaction_amount"`
	CurrencyID        string          `json:"currency_id"`
	LiveMode          *bool           `json:"live_mode"`
	PaymentTypeID     string          `json:"payment_type_id"`
	PaymentMethod     struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"payment_method"`
	ApplicationFee json.RawMessage `json:"application_fee"`
	FeeDetails     []struct {
		Type   string          `json:"type"`
		Amount json.RawMessage `json:"amount"`
	} `json:"fee_details"`
	Metadata struct {
		OperationID string `json:"operation_id"`
	} `json:"metadata"`
	DateLastUpdated time.Time `json:"date_last_updated"`
}

func parsePayment(body []byte, requestID string) (Payment, error) {
	var wire paymentWire
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&wire); err != nil {
		return Payment{}, ErrProvider
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Payment{}, ErrProvider
	}
	id, ok := decimalID(wire.ID)
	if !ok {
		return Payment{}, ErrProvider
	}
	amount, ok := integerAmount(wire.TransactionAmount)
	if !ok || amount <= 0 {
		return Payment{}, ErrProvider
	}
	collectorID, _ := decimalID(wire.CollectorID)
	fee, feePresent, feeOK := paymentFee(wire)
	if !feeOK {
		return Payment{}, ErrProvider
	}
	liveMode := false
	liveModePresent := wire.LiveMode != nil
	if liveModePresent {
		liveMode = *wire.LiveMode
	}
	paymentType := strings.TrimSpace(wire.PaymentTypeID)
	if paymentType == "" {
		paymentType = strings.TrimSpace(wire.PaymentMethod.Type)
	}
	dateUpdated := wire.DateLastUpdated
	status := safePaymentStatus(wire.Status)
	return Payment{
		Verified: checkoutpro.VerifiedPayment{
			OperationID: wire.Metadata.OperationID, ProviderPaymentID: id, SellerID: collectorID,
			ExternalReference: wire.ExternalReference, Status: status,
			TransactionAmountCLP: amount, Currency: wire.CurrencyID,
			PaymentType: paymentType, LiveMode: liveMode,
			ObservedAt: dateUpdated, PaymentUpdatedAt: dateUpdated, ApplicationFeeCLP: fee,
		},
		StatusDetail: safeStatusDetail(wire.StatusDetail), RequestID: requestID,
		ApplicationFeeCLP: fee, ApplicationFeePresent: feePresent, LiveModePresent: liveModePresent,
	}, nil
}

func decimalID(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var text string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return "", false
		}
	} else {
		text = string(raw)
	}
	if !digitsPattern.MatchString(text) || strings.TrimLeft(text, "0") == "" {
		return "", false
	}
	return text, true
}

func integerAmount(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var number json.Number
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return 0, false
		}
		number = json.Number(text)
	} else {
		number = json.Number(string(raw))
	}
	value, err := number.Int64()
	return value, err == nil
}

func paymentFee(wire paymentWire) (*int64, bool, bool) {
	if len(wire.ApplicationFee) > 0 && !bytes.Equal(wire.ApplicationFee, []byte("null")) {
		value, ok := integerAmount(wire.ApplicationFee)
		if !ok || value < 0 {
			return nil, false, false
		}
		return &value, true, true
	}
	var result *int64
	for _, detail := range wire.FeeDetails {
		if detail.Type != "application_fee" {
			continue
		}
		if result != nil {
			return nil, false, false
		}
		value, ok := integerAmount(detail.Amount)
		if !ok || value < 0 {
			return nil, false, false
		}
		result = &value
	}
	return result, result != nil, true
}

func safePaymentStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved", "pending", "rejected", "cancelled", "refunded", "in_process", "authorized", "charged_back":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func safeStatusDetail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "accredited", "pending_contingency", "pending_review_manual", "pending_waiting_payment", "pending_waiting_transfer":
		return value
	case "cc_rejected_bad_filled_card_number", "cc_rejected_bad_filled_date", "cc_rejected_bad_filled_security_code", "cc_rejected_bad_filled_other", "cc_rejected_insufficient_amount", "cc_rejected_call_for_authorize", "cc_rejected_card_disabled", "cc_rejected_duplicated_payment", "cc_rejected_high_risk", "cc_rejected_invalid_installments", "cc_rejected_max_attempts", "cc_rejected_other_reason", "cc_rejected_blacklist", "cc_rejected_card_error", "cc_rejected_rate_limit":
		return value
	default:
		return "other"
	}
}

func providerCode(body []byte) string {
	return providerDiagnostics(body).Code
}

// providerDiagnostics extracts only small, bounded structural facts from an
// error response. Field names, descriptions, data, and all other values are
// discarded. Numeric cause codes are safe to retain when bounded, including
// codes not yet known to this version of the integration.
type providerDiagnostic struct {
	Code              string
	ProviderError     string
	CauseCount        int
	CauseCodes        []string
	CauseCategories   []string
	UnknownFieldCount int
}

const maxDiagnosticCauses = 8

func providerDiagnostics(body []byte) providerDiagnostic {
	result := providerDiagnostic{Code: "unknown", ProviderError: "unknown"}
	if len(body) > 64<<10 {
		body = body[:64<<10]
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return result
	}
	knownRoot := map[string]bool{"error": true, "message": true, "cause": true, "status": true}
	for key := range root {
		if !knownRoot[key] {
			result.UnknownFieldCount++
		}
	}
	var providerError string
	if raw := root["error"]; len(raw) != 0 && json.Unmarshal(raw, &providerError) == nil {
		result.ProviderError = allowlistedProviderCode(providerError)
	}
	topLevelCode := result.ProviderError
	var causes []json.RawMessage
	if raw := root["cause"]; len(raw) != 0 && json.Unmarshal(raw, &causes) == nil {
		result.CauseCount = len(causes)
		for i, rawCause := range causes {
			if i >= maxDiagnosticCauses {
				break
			}
			var cause map[string]json.RawMessage
			if json.Unmarshal(rawCause, &cause) != nil || cause == nil {
				continue
			}
			for key := range cause {
				if key != "code" && key != "description" && key != "data" {
					result.UnknownFieldCount++
				}
			}
			code := safeCauseCode(cause["code"])
			if code != "" {
				result.CauseCodes = append(result.CauseCodes, code)
			}
			result.CauseCategories = append(result.CauseCategories, classifyCause(cause["description"]))
		}
	}
	if topLevelCode != "unknown" && topLevelCode != "bad_request" {
		result.Code = topLevelCode
		return result
	}
	for _, code := range result.CauseCodes {
		if code != "" {
			result.Code = code
			return result
		}
	}
	if topLevelCode != "unknown" {
		result.Code = topLevelCode
	}
	return result
}

func requestFacts(sellerToken []byte, request PaymentRequest) RequestFacts {
	return RequestFacts{
		SellerTokenPresent: len(sellerToken) > 0,
		CardTokenPresent:   len(request.Token) > 0,
		PaymentMethodSet:   request.PaymentMethodID != "",
		IssuerSet:          request.IssuerID != "",
		PayerEmailSet:      request.Payer.Email != "",
		IdentificationSet:  request.Payer.Identification != nil,
		ApplicationFeeSet:  request.ApplicationFeeCLP > 0,
	}
}

// classifyCause scans only a short prefix of the provider description and
// returns one fixed label. It never retains or returns provider-supplied text.
// These labels are clues, not authoritative mappings for undocumented codes.
func classifyCause(raw json.RawMessage) string {
	var description string
	if len(raw) == 0 || json.Unmarshal(raw, &description) != nil {
		return "unclassified"
	}
	if len(description) > 512 {
		description = description[:512]
	}
	description = strings.ToLower(description)
	switch {
	case strings.Contains(description, "invalid users involved"), strings.Contains(description, "involved users are either"):
		return "involved_users"
	case strings.Contains(description, "oauth"), strings.Contains(description, "seller access token"):
		return "seller_authorization"
	case strings.Contains(description, "payer.email"), strings.Contains(description, "payer email"), strings.Contains(description, "invalid test user email"):
		return "payer_email"
	case strings.Contains(description, "identification"):
		return "payer_identification"
	case strings.Contains(description, "payment_method"), strings.Contains(description, "payment method"):
		return "payment_method"
	case strings.Contains(description, "issuer"):
		return "issuer"
	case strings.Contains(description, "test token"), strings.Contains(description, "test credentials"), strings.Contains(description, "test mode"):
		return "test_credentials"
	case strings.Contains(description, "card token"), strings.Contains(description, "card_token"), strings.Contains(description, "token") && strings.Contains(description, "card"):
		return "card_token"
	case strings.Contains(description, "application_fee"), strings.Contains(description, "application fee"), strings.Contains(description, "fee") && strings.Contains(description, "amount"):
		return "amount_or_fee"
	case strings.Contains(description, "account setting"), strings.Contains(description, "account configuration"):
		return "account_settings"
	case strings.Contains(description, "permission"), strings.Contains(description, "not authorized"):
		return "permission"
	default:
		return "unclassified"
	}
}

func safeCauseCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return ""
		}
	} else {
		text = string(raw)
	}
	if len(text) >= 1 && len(text) <= 8 && digitsPattern.MatchString(text) {
		return text
	}
	if allowed := allowlistedProviderCode(text); allowed != "unknown" {
		return allowed
	}
	return ""
}

func allowlistedProviderCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "bad_request", "unauthorized", "forbidden", "not_found", "internal_error", "invalid_token", "invalid_access_token", "invalid_card_token", "invalid_payment_method", "invalid_parameters", "invalid_amount", "invalid_application_fee", "invalid_installments", "payer_email_invalid", "idempotency_key_conflict", "card_token_used", "cc_rejected_bad_filled_card_number", "cc_rejected_bad_filled_date", "cc_rejected_bad_filled_security_code", "cc_rejected_bad_filled_other", "cc_rejected_insufficient_amount", "cc_rejected_call_for_authorize", "cc_rejected_card_disabled", "cc_rejected_duplicated_payment", "cc_rejected_high_risk", "cc_rejected_invalid_installments", "cc_rejected_max_attempts", "cc_rejected_other_reason", "cc_rejected_blacklist", "cc_rejected_card_error", "cc_rejected_rate_limit":
		return strings.ToLower(strings.TrimSpace(code))
	case "3", "2000", "2001", "2002", "2004", "2006", "2007", "2009", "2034", "2059", "2062", "2063", "2065", "2066", "2067", "2198", "3012", "4000", "4033", "4038", "4039", "4050", "4051":
		return code
	default:
		return "unknown"
	}
}
