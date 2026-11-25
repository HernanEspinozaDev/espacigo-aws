package checkoutpro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PreferenceItem struct {
	Title      string  `json:"title"`
	Quantity   int     `json:"quantity"`
	UnitPrice  float64 `json:"unit_price"`
	CurrencyID string  `json:"currency_id"`
}

func parseCLPAmount(value json.Number, allowZero bool) (int64, bool) {
	amount, ok := new(big.Rat).SetString(value.String())
	if !ok || !amount.IsInt() || !amount.Num().IsInt64() {
		return 0, false
	}
	clp := amount.Num().Int64()
	if clp < 0 || (!allowZero && clp == 0) {
		return 0, false
	}
	return clp, true
}

type ExcludedPaymentType struct {
	ID string `json:"id"`
}
type PaymentMethods struct {
	ExcludedPaymentTypes []ExcludedPaymentType `json:"excluded_payment_types"`
	Installments         int                   `json:"installments"`
}
type BackURLs struct {
	Success string `json:"success"`
	Failure string `json:"failure"`
	Pending string `json:"pending"`
}
type PreferenceRequest struct {
	Purpose            string           `json:"purpose,omitempty"`
	ExternalReference  string           `json:"external_reference"`
	MarketplaceFee     float64          `json:"marketplace_fee"`
	Items              []PreferenceItem `json:"items"`
	Metadata           map[string]any   `json:"metadata"`
	BackURLs           BackURLs         `json:"back_urls"`
	AutoReturn         string           `json:"auto_return,omitempty"`
	NotificationURL    string           `json:"notification_url,omitempty"`
	Expires            bool             `json:"expires"`
	ExpirationDateFrom string           `json:"expiration_date_from"`
	ExpirationDateTo   string           `json:"expiration_date_to"`
	PaymentMethods     PaymentMethods   `json:"payment_methods"`
}
type Preference struct {
	ID        string `json:"id"`
	InitPoint string `json:"init_point"`
}

func (g *HTTPGateway) LookupPayment(ctx context.Context, token []byte, paymentID string) (VerifiedPayment, error) {
	if paymentID == "" || len(paymentID) > 128 {
		return VerifiedPayment{}, ErrInvalid
	}
	body, err := g.requestRaw(ctx, http.MethodGet, "/v1/payments/"+url.PathEscape(paymentID), token, nil)
	if err != nil {
		return VerifiedPayment{}, err
	}
	return decodeVerifiedPaymentJSON(body)
}

func (g *HTTPGateway) FindPaymentByExternalReference(ctx context.Context, token []byte, externalReference string) (VerifiedPayment, error) {
	if strings.TrimSpace(externalReference) == "" || len(externalReference) > 128 {
		return VerifiedPayment{}, ErrInvalid
	}
	path := "/v1/payments/search?external_reference=" + url.QueryEscape(externalReference) + "&sort=date_last_updated&criteria=desc&limit=5"
	body, err := g.requestRawAtStage(ctx, http.MethodGet, path, token, nil, "payment_search")
	if err == nil {
		var searchResult struct {
			Results []json.RawMessage `json:"results"`
		}
		if json.Unmarshal(body, &searchResult) == nil && len(searchResult.Results) > 0 {
			var firstCandidateID string
			for _, rawItem := range searchResult.Results {
				vp, parseErr := decodeVerifiedPaymentJSON(rawItem)
				if parseErr != nil || vp.ExternalReference != externalReference {
					continue
				}
				if strings.EqualFold(vp.Status, "approved") {
					if full, lookupErr := g.LookupPayment(ctx, token, vp.ProviderPaymentID); lookupErr == nil {
						return full, nil
					}
					return vp, nil
				}
				if firstCandidateID == "" {
					firstCandidateID = vp.ProviderPaymentID
				}
			}
			if firstCandidateID != "" {
				if full, lookupErr := g.LookupPayment(ctx, token, firstCandidateID); lookupErr == nil {
					return full, nil
				}
			}
		}
	}
	return g.findPaymentByMerchantOrder(ctx, token, externalReference)
}

func (g *HTTPGateway) findPaymentByMerchantOrder(ctx context.Context, token []byte, externalReference string) (VerifiedPayment, error) {
	path := "/merchant_orders/search?external_reference=" + url.QueryEscape(externalReference)
	body, err := g.requestRawAtStage(ctx, http.MethodGet, path, token, nil, "merchant_order_search")
	if err != nil {
		return VerifiedPayment{}, err
	}
	var orders struct {
		Elements []struct {
			ExternalReference string `json:"external_reference"`
			Payments          []struct {
				ID     json.Number `json:"id"`
				Status string      `json:"status"`
			} `json:"payments"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &orders); err != nil {
		return VerifiedPayment{}, ErrProvider
	}
	var fallbackID string
	for _, elem := range orders.Elements {
		if elem.ExternalReference != externalReference {
			continue
		}
		for _, p := range elem.Payments {
			pid := strings.TrimSpace(p.ID.String())
			if pid == "" {
				continue
			}
			if strings.EqualFold(p.Status, "approved") {
				if full, lookupErr := g.LookupPayment(ctx, token, pid); lookupErr == nil {
					return full, nil
				}
			}
			if fallbackID == "" {
				fallbackID = pid
			}
		}
	}
	if fallbackID == "" {
		return VerifiedPayment{}, ErrNotFound
	}
	return g.LookupPayment(ctx, token, fallbackID)
}

func decodeVerifiedPaymentJSON(body []byte) (VerifiedPayment, error) {
	var p struct {
		ID                json.Number     `json:"id"`
		CollectorID       json.RawMessage `json:"collector_id"`
		ExternalReference string          `json:"external_reference"`
		Status            string          `json:"status"`
		TransactionAmount json.Number     `json:"transaction_amount"`
		ApplicationFee    json.RawMessage `json:"application_fee"`
		Currency          string          `json:"currency_id"`
		LiveMode          bool            `json:"live_mode"`
		PaymentMethod     struct {
			Type string `json:"type"`
		} `json:"payment_method"`
		PaymentTypeID string `json:"payment_type_id"`
		Metadata      struct {
			OperationID string `json:"operation_id"`
		} `json:"metadata"`
		DateLastUpdated    time.Time       `json:"date_last_updated"`
		TransactionDetails json.RawMessage `json:"transaction_details"`
		Fees               json.RawMessage `json:"fee_details"`
		Charges            json.RawMessage `json:"charges_details"`
		Refunded           json.RawMessage `json:"transaction_amount_refunded"`
		ReleaseAt          json.RawMessage `json:"money_release_date"`
		ReleaseStatus      json.RawMessage `json:"money_release_status"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if d.Decode(&p) != nil {
		return VerifiedPayment{}, ErrProvider
	}
	id, e := strconv.ParseInt(p.ID.String(), 10, 64)
	if e != nil || id <= 0 {
		return VerifiedPayment{}, ErrProvider
	}
	sellerID := string(p.CollectorID)
	if len(p.CollectorID) > 0 && p.CollectorID[0] == '"' {
		if json.Unmarshal(p.CollectorID, &sellerID) != nil {
			return VerifiedPayment{}, ErrProvider
		}
	}
	seller, e := strconv.ParseInt(sellerID, 10, 64)
	if e != nil || seller <= 0 {
		return VerifiedPayment{}, ErrProvider
	}
	amount, ok := parseCLPAmount(p.TransactionAmount, false)
	if !ok {
		return VerifiedPayment{}, ErrProvider
	}
	if p.DateLastUpdated.IsZero() {
		return VerifiedPayment{}, ErrProvider
	}
	var applicationFee *int64
	if len(p.ApplicationFee) > 0 && string(p.ApplicationFee) != "null" {
		var feeNumber json.Number
		if json.Unmarshal(p.ApplicationFee, &feeNumber) != nil {
			return VerifiedPayment{}, ErrProvider
		}
		fee, ok := parseCLPAmount(feeNumber, true)
		if !ok {
			return VerifiedPayment{}, ErrProvider
		}
		applicationFeeCLP := fee
		applicationFee = &applicationFeeCLP
	}
	paymentType := p.PaymentTypeID
	if paymentType == "" {
		paymentType = p.PaymentMethod.Type
	}
	var transactionDetails struct {
		Net json.RawMessage `json:"net_received_amount"`
	}
	var fees []paymentFee
	var charges []paymentCharge
	var releaseStatus string
	_ = json.Unmarshal(p.TransactionDetails, &transactionDetails)
	if json.Unmarshal(p.Fees, &fees) != nil {
		fees = nil
	}
	if json.Unmarshal(p.Charges, &charges) != nil {
		charges = nil
	}
	_ = json.Unmarshal(p.ReleaseStatus, &releaseStatus)
	settlement := Settlement{
		NetCLP: optionalCLP(transactionDetails.Net), ProcessingFeeCLP: settlementFee(fees, charges, "mercadopago_fee", "mercadopago_fee", "mp"),
		MarketplaceCLP: settlementFee(fees, charges, "application_fee", "third_payment", "marketplace_owner"), RefundedCLP: optionalCLP(p.Refunded),
		ProviderUpdated: p.DateLastUpdated.UTC(),
	}
	if settlement.MarketplaceCLP == nil {
		settlement.MarketplaceCLP = applicationFee
	}
	if releaseStatus == "released" || releaseStatus == "pending" {
		settlement.ReleaseStatus = releaseStatus
	}
	var release time.Time
	if json.Unmarshal(p.ReleaseAt, &release) == nil && !release.IsZero() {
		settlement.ReleaseAt = &release
	}
	var financial *Settlement
	if settlement.NetCLP != nil || settlement.ProcessingFeeCLP != nil || settlement.MarketplaceCLP != nil || settlement.ReleaseStatus != "" || settlement.ReleaseAt != nil {
		financial = &settlement
	}
	return VerifiedPayment{OperationID: p.Metadata.OperationID, ProviderPaymentID: p.ID.String(), SellerID: sellerID, ExternalReference: p.ExternalReference, Status: p.Status, TransactionAmountCLP: int64(amount), Currency: p.Currency, PaymentType: paymentType, LiveMode: p.LiveMode, ObservedAt: p.DateLastUpdated, PaymentUpdatedAt: p.DateLastUpdated, ApplicationFeeCLP: applicationFee, Settlement: financial}, nil
}

// HTTPGateway is deliberately injectable for offline httptest coverage. It
// uses the seller OAuth token and never forwards provider redirects.
type HTTPGateway struct {
	BaseURL string
	Client  *http.Client
}

type RefundResult struct {
	ID     string
	Status string
	Amount int64
}

func (g *HTTPGateway) ListRefunds(ctx context.Context, token []byte, paymentID string) ([]RefundResult, error) {
	if ctx == nil || len(token) == 0 || len(token) > 8192 || !positiveDecimal(paymentID) {
		return nil, ErrInvalid
	}
	path := "/v1/payments/" + url.PathEscape(paymentID) + "/refunds"
	raw, err := g.requestRawAtStage(ctx, http.MethodGet, path, token, nil, "refund_lookup")
	if err != nil {
		return nil, err
	}
	var values []struct {
		ID     any         `json:"id"`
		Status string      `json:"status"`
		Amount json.Number `json:"amount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&values) != nil || values == nil {
		return nil, ErrProvider
	}
	results := make([]RefundResult, 0, len(values))
	for _, value := range values {
		id := fmt.Sprint(value.ID)
		amount, amountOK := parseCLPAmount(value.Amount, false)
		status := strings.ToLower(strings.TrimSpace(value.Status))
		if !positiveDecimal(id) || !amountOK || status != "approved" && status != "in_process" && status != "rejected" && status != "cancelled" && status != "authorized" {
			return nil, ErrProvider
		}
		results = append(results, RefundResult{ID: id, Status: status, Amount: amount})
	}
	return results, nil
}

// RefundPayment creates a total refund when amountCLP is nil, or a partial
// refund otherwise. The caller must persist idempotencyKey before calling so
// retries after an uncertain network result represent the same operation.
func (g *HTTPGateway) RefundPayment(ctx context.Context, token []byte, paymentID string, amountCLP *int64, idempotencyKey string) (RefundResult, error) {
	if ctx == nil || len(token) == 0 || len(token) > 8192 || !positiveDecimal(paymentID) || !uuidLike(idempotencyKey) {
		return RefundResult{}, ErrInvalid
	}
	var body io.Reader
	if amountCLP != nil {
		if *amountCLP <= 0 {
			return RefundResult{}, ErrInvalid
		}
		encoded, err := json.Marshal(struct {
			Amount int64 `json:"amount"`
		}{Amount: *amountCLP})
		if err != nil {
			return RefundResult{}, ErrProvider
		}
		body = bytes.NewReader(encoded)
	}
	path := "/v1/payments/" + url.PathEscape(paymentID) + "/refunds"
	raw, err := g.requestRawWithIdempotency(ctx, http.MethodPost, path, token, body, "refund_create", idempotencyKey)
	if err != nil {
		return RefundResult{}, err
	}
	var response struct {
		ID     any         `json:"id"`
		Status string      `json:"status"`
		Amount json.Number `json:"amount"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&response) != nil {
		return RefundResult{}, ErrProvider
	}
	id := fmt.Sprint(response.ID)
	if !positiveDecimal(id) {
		return RefundResult{}, ErrProvider
	}
	amount, amountOK := parseCLPAmount(response.Amount, false)
	if !amountOK {
		return RefundResult{}, ErrProvider
	}
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if status != "approved" && status != "in_process" && status != "rejected" && status != "cancelled" && status != "authorized" {
		return RefundResult{}, ErrProvider
	}
	return RefundResult{ID: id, Status: status, Amount: amount}, nil
}

func positiveDecimal(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return strings.Trim(value, "0") != ""
}

func uuidLike(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func (g *HTTPGateway) PaymentTypes(ctx context.Context, token []byte) ([]string, error) {
	body, err := g.requestRawAtStage(ctx, http.MethodGet, "/v1/payment_methods", token, nil, "payment_methods")
	if err != nil {
		return nil, err
	}
	var methods []struct {
		PaymentTypeID string `json:"payment_type_id"`
		Status        string `json:"status"`
	}
	if json.Unmarshal(body, &methods) != nil || methods == nil {
		return nil, ErrProvider
	}
	seen := map[string]bool{}
	types := make([]string, 0, len(methods))
	for _, method := range methods {
		if strings.TrimSpace(method.Status) != "active" {
			continue
		}
		value := strings.TrimSpace(method.PaymentTypeID)
		if value == "" || len(value) > 64 {
			continue
		}
		if !seen[value] {
			seen[value] = true
			types = append(types, value)
		}
	}
	return types, nil
}

func (g *HTTPGateway) Create(ctx context.Context, token []byte, p PreferenceRequest) (Preference, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return Preference{}, ErrInvalid
	}
	return g.requestAtStage(ctx, http.MethodPost, "/checkout/preferences", token, bytes.NewReader(body), "preference_create")
}
func (g *HTTPGateway) FindByExternalReference(ctx context.Context, token []byte, reference string) (Preference, error) {
	if strings.TrimSpace(reference) == "" {
		return Preference{}, ErrInvalid
	}
	path := "/checkout/preferences/search?external_reference=" + url.QueryEscape(reference)
	var result struct {
		Elements []Preference `json:"elements"`
	}
	body, err := g.requestRawAtStage(ctx, http.MethodGet, path, token, nil, "preference_lookup")
	if err != nil {
		return Preference{}, err
	}
	if err = json.Unmarshal(body, &result); err != nil || len(result.Elements) > 1 {
		return Preference{}, ErrProvider
	}
	if len(result.Elements) == 0 {
		return Preference{}, ErrNotFound
	}
	return result.Elements[0], nil
}
func (g *HTTPGateway) request(ctx context.Context, method, path string, token []byte, body io.Reader) (Preference, error) {
	return g.requestAtStage(ctx, method, path, token, body, "provider_request")
}
func (g *HTTPGateway) requestAtStage(ctx context.Context, method, path string, token []byte, body io.Reader, stage string) (Preference, error) {
	raw, err := g.requestRawAtStage(ctx, method, path, token, body, stage)
	if err != nil {
		return Preference{}, err
	}
	var p Preference
	if json.Unmarshal(raw, &p) != nil {
		return Preference{}, ErrProvider
	}
	return p, nil
}
func (g *HTTPGateway) requestRaw(ctx context.Context, method, path string, token []byte, body io.Reader) ([]byte, error) {
	return g.requestRawAtStage(ctx, method, path, token, body, "provider_request")
}
func (g *HTTPGateway) requestRawAtStage(ctx context.Context, method, path string, token []byte, body io.Reader, stage string) ([]byte, error) {
	return g.requestRawWithIdempotency(ctx, method, path, token, body, stage, "")
}
func (g *HTTPGateway) requestRawWithIdempotency(ctx context.Context, method, path string, token []byte, body io.Reader, stage, idempotencyKey string) ([]byte, error) {
	base := g.BaseURL
	if base == "" {
		base = "https://api.mercadopago.com"
	}
	u, err := url.Parse(base + path)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return nil, ErrNotConfigured
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, ErrProvider
	}
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Accept", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &ProviderError{Stage: stage}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		providerCode := readProviderErrorCode(resp.Body)
		return nil, &ProviderError{Stage: stage, HTTPStatus: resp.StatusCode, ProviderCode: providerCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if err != nil {
		return nil, &ProviderError{Stage: stage}
	}
	if len(raw) >= 128<<10 {
		return nil, &ProviderError{Stage: stage}
	}
	return raw, nil
}

func readProviderErrorCode(body io.Reader) string {
	const maxErrorBody = 8 << 10
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBody+1))
	if err != nil || len(raw) > maxErrorBody {
		return "other"
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return "other"
	}
	switch payload.Error {
	case "invalid_access_token", "invalid_marketplace_fee", "invalid_items", "invalid_back_urls",
		"invalid_payment_methods", "invalid_expiration_date_from", "invalid_expiration_date_to",
		"invalid_date", "invalid_collector_id", "collector_does_not_comply_with_current_regulation",
		"invalid_operation_type", "unauthorized", "bad_request", "internal_error":
		return payload.Error
	default:
		return "other"
	}
}
func (g *HTTPGateway) String() string {
	base := g.BaseURL
	if base == "" {
		base = "https://api.mercadopago.com"
	}
	return fmt.Sprintf("CheckoutProGateway(%s)", base)
}
