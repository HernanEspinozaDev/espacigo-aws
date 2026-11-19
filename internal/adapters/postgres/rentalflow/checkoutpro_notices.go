package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// enqueueCheckoutPaymentNotices is called only inside the transaction that
// applies a provider-verified payment or expires its commercial hold.
func enqueueCheckoutPaymentNotices(ctx context.Context, tx pgx.Tx, operationID, transition string, at time.Time) error {
	if ctx == nil || tx == nil || operationID == "" || at.IsZero() {
		return errors.New("invalid checkout notice intent")
	}
	switch transition {
	case "approved", "rejected", "expired":
	default:
		return errors.New("unsupported checkout notice transition")
	}
	_, err := tx.Exec(ctx, `SELECT public.enqueue_checkout_payment_notices($1::uuid,$2,$3)`, operationID, transition, at.UTC())
	return err
}
