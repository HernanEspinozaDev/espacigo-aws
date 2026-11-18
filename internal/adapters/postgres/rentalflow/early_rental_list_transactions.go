package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EarlyRentalTransactionListPosition struct {
	CreatedAt time.Time
	PaymentID uuid.UUID
}

type EarlyRentalTransactionListInput struct {
	ActorID uuid.UUID
	Role    string // empty, "renter", or "host"
	State   string // empty, "enviado", "recibido", "anulado", or "vencido"
	After   *EarlyRentalTransactionListPosition
	Limit   int
}

type EarlyRentalTransactionListRow struct {
	Offer     earlyrental.Offer
	CreatedAt time.Time
}

type EarlyRentalTransactionListResult struct {
	Rows []EarlyRentalTransactionListRow
	Next *EarlyRentalTransactionListPosition
}

// ErrEarlyRentalTransactionListRetry means page membership changed while the
// caller's transaction acquired participant locks. Roll back and retry in a
// fresh READ COMMITTED transaction; do not continue with a partial lock set.
var ErrEarlyRentalTransactionListRetry = errors.New("early rental transaction list changed; retry transaction")

const earlyRentalListTransactionsSQL = `SELECT o.id,o.creada_en,p.pago_id,o.anfitrion_id,o.arrendatario_id
FROM public.arriendo_temprano o
JOIN public.arriendo_temprano_pago p ON p.arriendo_id=o.id
JOIN public.usuario u ON u.id=$1 AND u.estado='activo'
WHERE (o.anfitrion_id=$1 OR o.arrendatario_id=$1)
  AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$1)
  AND p.enviado_en IS NOT NULL
  AND ($2='' OR ($2='host' AND o.anfitrion_id=$1) OR ($2='renter' AND o.arrendatario_id=$1))
  AND ($3='' OR p.estado=$3)
  AND ($4::timestamptz IS NULL OR (o.creada_en,p.pago_id)<($4,$5::uuid))
ORDER BY o.creada_en DESC,p.pago_id DESC LIMIT $6`

type earlyRentalTransactionListKey struct {
	offerID, paymentID, hostID, renterID uuid.UUID
	createdAt                            time.Time
}

// listEarlyRentalTransactions requires a caller-owned READ COMMITTED tx. It
// lists only canonical movements with persisted sent-payment evidence. Snapshot
// fields come from the offer reader; the current listing is never consulted.
func listEarlyRentalTransactions(ctx context.Context, tx pgx.Tx, in EarlyRentalTransactionListInput) (EarlyRentalTransactionListResult, error) {
	invalid := func() (EarlyRentalTransactionListResult, error) {
		return EarlyRentalTransactionListResult{}, earlyrental.ErrInvalid
	}
	if ctx == nil || tx == nil || in.ActorID == uuid.Nil || in.Limit < 1 || in.Limit > 100 ||
		(in.Role != "" && in.Role != "renter" && in.Role != "host") ||
		(in.State != "" && in.State != "enviado" && in.State != "recibido" && in.State != "anulado" && in.State != "vencido") {
		return invalid()
	}
	if in.After != nil && (in.After.CreatedAt.IsZero() || in.After.PaymentID == uuid.Nil) {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return EarlyRentalTransactionListResult{}, err
	}

	var afterAt any
	var afterPaymentID any
	if in.After != nil {
		afterAt, afterPaymentID = in.After.CreatedAt.UTC(), in.After.PaymentID
	}
	args := []any{in.ActorID, in.Role, in.State, afterAt, afterPaymentID, in.Limit + 1}
	keys, err := queryEarlyRentalTransactionListKeys(ctx, tx, args)
	if err != nil {
		return EarlyRentalTransactionListResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return EarlyRentalTransactionListResult{}, err
	}

	participants := make([][2]uuid.UUID, 0, len(keys))
	for _, key := range keys {
		participants = append(participants, [2]uuid.UUID{key.hostID, key.renterID})
	}
	if err := lockEarlyRentalOfferAccounts(ctx, tx, in.ActorID, participants); err != nil {
		return EarlyRentalTransactionListResult{}, err
	}

	checked, err := queryEarlyRentalTransactionListKeys(ctx, tx, args)
	if err != nil {
		return EarlyRentalTransactionListResult{}, err
	}
	if !sameEarlyRentalTransactionListKeys(keys, checked) {
		return EarlyRentalTransactionListResult{}, ErrEarlyRentalTransactionListRetry
	}

	result := EarlyRentalTransactionListResult{Rows: make([]EarlyRentalTransactionListRow, 0, min(len(keys), in.Limit))}
	hasMore := len(keys) > in.Limit
	if hasMore {
		keys = keys[:in.Limit]
	}
	for _, key := range keys {
		stored, err := readEarlyRentalOffer(ctx, tx, in.ActorID, key.offerID)
		if err != nil {
			if errors.Is(err, rentalflow.ErrNotFound) {
				return EarlyRentalTransactionListResult{}, rentalflow.ErrNotFound
			}
			return EarlyRentalTransactionListResult{}, err
		}
		if !stored.CreatedAt.Equal(key.createdAt) || stored.Offer.PaymentID != key.paymentID.String() || stored.Offer.PaymentSentAt == nil {
			return EarlyRentalTransactionListResult{}, earlyrental.ErrInvalid
		}
		result.Rows = append(result.Rows, EarlyRentalTransactionListRow{Offer: stored.Offer, CreatedAt: stored.CreatedAt})
	}
	if hasMore && len(result.Rows) > 0 {
		last := keys[len(keys)-1]
		result.Next = &EarlyRentalTransactionListPosition{CreatedAt: last.createdAt, PaymentID: last.paymentID}
	}
	return result, nil
}

func queryEarlyRentalTransactionListKeys(ctx context.Context, tx pgx.Tx, args []any) ([]earlyRentalTransactionListKey, error) {
	rows, err := tx.Query(ctx, earlyRentalListTransactionsSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]earlyRentalTransactionListKey, 0, args[len(args)-1].(int))
	for rows.Next() {
		var key earlyRentalTransactionListKey
		if err := rows.Scan(&key.offerID, &key.createdAt, &key.paymentID, &key.hostID, &key.renterID); err != nil {
			return nil, err
		}
		if key.offerID == uuid.Nil || key.paymentID == uuid.Nil || key.hostID == uuid.Nil || key.renterID == uuid.Nil || key.hostID == key.renterID || key.createdAt.IsZero() {
			return nil, earlyrental.ErrInvalid
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func sameEarlyRentalTransactionListKeys(left, right []earlyRentalTransactionListKey) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
