package checkoutpro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// CancelPendingPayment asks Mercado Pago to move the payment to cancelled.
// Read the payment again before treating either a successful or ambiguous PUT
// as a final outcome.
func (g *HTTPGateway) CancelPendingPayment(ctx context.Context, token []byte, paymentID, idempotencyKey string) error {
	if g == nil || ctx == nil || len(token) == 0 || len(token) > 8192 || !positiveDecimal(paymentID) || !uuidLike(idempotencyKey) {
		return ErrInvalid
	}
	_, err := g.requestRawWithIdempotency(ctx, http.MethodPut, "/v1/payments/"+url.PathEscape(paymentID),
		token, strings.NewReader(`{"status":"cancelled"}`), "payment_cancel", idempotencyKey)
	return err
}

// SearchPaymentsByExternalReference queries Mercado Pago's payment search API
// for payments matching externalReference using the seller's OAuth token.
func (g *HTTPGateway) SearchPaymentsByExternalReference(ctx context.Context, token []byte, externalReference string) ([]VerifiedPayment, error) {
	if g == nil || ctx == nil || len(token) == 0 || len(token) > 8192 || strings.TrimSpace(externalReference) == "" || len(externalReference) > 128 {
		return nil, ErrInvalid
	}
	path := "/v1/payments/search?external_reference=" + url.QueryEscape(externalReference) + "&sort=date_created&criteria=desc"
	body, err := g.requestRawAtStage(ctx, http.MethodGet, path, token, nil, "payment_search")
	if err != nil {
		return nil, err
	}
	var searchResult struct {
		Results []json.RawMessage `json:"results"`
	}
	if json.Unmarshal(body, &searchResult) != nil || searchResult.Results == nil {
		return nil, ErrProvider
	}
	payments := make([]VerifiedPayment, 0, len(searchResult.Results))
	for _, rawItem := range searchResult.Results {
		var ref struct {
			ExternalReference string `json:"external_reference"`
		}
		if json.Unmarshal(rawItem, &ref) != nil {
			return nil, ErrProvider
		}
		if ref.ExternalReference != externalReference {
			continue
		}
		vp, parseErr := decodeVerifiedPaymentJSON(rawItem)
		if parseErr != nil {
			return nil, parseErr
		}
		payments = append(payments, vp)
	}
	return payments, nil
}
