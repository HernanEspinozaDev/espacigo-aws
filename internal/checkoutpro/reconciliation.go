package checkoutpro

import (
	"context"
	"time"
)

// VerifiedPayment is a provider-authenticated payment lookup result. A webhook
// receiver must call ProviderLookup first and pass only that result here; raw
// notification payloads/redirect parameters cannot implement this contract.
type VerifiedPayment struct {
	OperationID          string
	ProviderPaymentID    string
	SellerID             string
	ExternalReference    string
	Status               string
	TransactionAmountCLP int64
	Currency             string
	PaymentType          string
	LiveMode             bool
	ObservedAt           time.Time
	PaymentUpdatedAt     time.Time
	ApplicationFeeCLP    *int64
	Settlement           *Settlement
}

// PaymentOutcomeSink is the application port the webhook/reconciler should use.
// Implementations atomically validate all amounts/IDs/mode, deduplicate provider
// payment IDs and apply the state transition exactly once.
type PaymentOutcomeSink interface {
	ApplyVerifiedPayment(context.Context, VerifiedPayment) (bool, error)
}

// ProviderLookup resolves notification IDs through Mercado Pago with the
// connected seller credential before invoking PaymentOutcomeSink.
type ProviderLookup interface {
	LookupPayment(context.Context, []byte, string) (VerifiedPayment, error)
}
