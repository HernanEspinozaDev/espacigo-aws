package checkoutpro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		DateLastUpdated time.Time `json:"date_last_updated"`
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
	amount, e := strconv.ParseFloat(p.TransactionAmount.String(), 64)
	if e != nil || amount <= 0 || amount != float64(int64(amount)) {
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
		fee, parseErr := strconv.ParseFloat(feeNumber.String(), 64)
		if parseErr != nil || fee < 0 || fee != float64(int64(fee)) {
			return VerifiedPayment{}, ErrProvider
		}
		applicationFeeCLP := int64(fee)
		applicationFee = &applicationFeeCLP
	}
	paymentType := p.PaymentTypeID
	if paymentType == "" {
		paymentType = p.PaymentMethod.Type
	}
	return VerifiedPayment{OperationID: p.Metadata.OperationID, ProviderPaymentID: p.ID.String(), SellerID: sellerID, ExternalReference: p.ExternalReference, Status: p.Status, TransactionAmountCLP: int64(amount), Currency: p.Currency, PaymentType: paymentType, LiveMode: p.LiveMode, ObservedAt: p.DateLastUpdated, PaymentUpdatedAt: p.DateLastUpdated, ApplicationFeeCLP: applicationFee}, nil
}

// HTTPGateway is deliberately injectable for offline httptest coverage. It
// uses the seller OAuth token and never forwards provider redirects.
type HTTPGateway struct {
	BaseURL string
	Client  *http.Client
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
