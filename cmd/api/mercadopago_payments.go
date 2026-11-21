package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutapi"
	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
)

func mercadoPagoPaymentsEnabled(getenv func(string) string, webhookOnlyEnabled bool) (bool, error) {
	switch strings.TrimSpace(getenv("MERCADOPAGO_PAYMENTS_API_ENABLED")) {
	case "", "0":
		return false, nil
	case "1":
		walletEnabled, walletErr := mercadoPagoWalletPurchaseEnabled(getenv)
		if walletErr != nil {
			return false, walletErr
		}
		if webhookOnlyEnabled && (strings.TrimSpace(getenv("MERCADOPAGO_CHECKOUT_ENABLED")) != "1" || walletEnabled) {
			return true, nil
		}
	}
	return false, errors.New("Mercado Pago Payments API requires local test webhook mode; Checkout Pro can be enabled only with wallet purchase mode")
}

func mercadoPagoSellerPublicKeyTokenizationEnabled(getenv func(string) string) (bool, error) {
	switch strings.TrimSpace(getenv("MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY")) {
	case "", "0":
		return false, nil
	case "1":
		if strings.TrimSpace(getenv("MERCADOPAGO_PAYMENTS_API_ENABLED")) != "1" || strings.ToLower(strings.TrimSpace(getenv("MERCADOPAGO_TEST_MODE"))) != "true" {
			return false, errors.New("seller Public Key tokenization is restricted to the local Payments API test mode")
		}
		return true, nil
	default:
		return false, errors.New("seller Public Key tokenization must be explicitly disabled or enabled with 1")
	}
}

func newMercadoPagoPayments(repo checkoutapi.Repository, tokens *marketplaceoauth.Service, getenv func(string) string) (*checkoutapi.Service, error) {
	if tokens == nil {
		return nil, errors.New("Mercado Pago seller OAuth is unavailable")
	}
	sellerKeyMode, err := mercadoPagoSellerPublicKeyTokenizationEnabled(getenv)
	if err != nil {
		return nil, err
	}
	return checkoutapi.NewWithOptions(repo, tokens, checkoutapi.NewGateway(), time.Now, (credentials.Generator{}).ID,
		func(_ context.Context, diagnostic checkoutapi.ProviderFailure) {
			log.Printf("mercadopago_payments test_only_config=true stage=%s quote_id=%s operation_id=%s payment_id=%s amount_clp=%d seller_context_present=%t http_status=%d code=%s provider_error=%s cause_count=%d cause_codes=%v cause_categories=%v unknown_fields=%d body_truncated=%t seller_token_present=%t card_token_present=%t payment_method_set=%t issuer_set=%t payer_email_set=%t identification_set=%t application_fee_set=%t provider_request_id=%s",
				diagnostic.Stage, diagnostic.QuoteID, diagnostic.OperationID, diagnostic.PaymentID, diagnostic.AmountCLP, diagnostic.SellerContextPresent, diagnostic.HTTPStatus, diagnostic.Code, diagnostic.ProviderError,
				diagnostic.CauseCount, diagnostic.CauseCodes, diagnostic.CauseCategories, diagnostic.UnknownFieldCount,
				diagnostic.BodyTruncated, diagnostic.RequestFacts.SellerTokenPresent, diagnostic.RequestFacts.CardTokenPresent,
				diagnostic.RequestFacts.PaymentMethodSet, diagnostic.RequestFacts.IssuerSet, diagnostic.RequestFacts.PayerEmailSet,
				diagnostic.RequestFacts.IdentificationSet, diagnostic.RequestFacts.ApplicationFeeSet, diagnostic.RequestID)
		}, checkoutapi.Options{SellerPublicKeyTokenization: sellerKeyMode})
}
