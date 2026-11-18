package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EarlyRentalOfferListPosition is the raw, verified keyset location. The caller
// must authenticate/decode its cursor before passing it here; no cursor key is
// wired into this unmounted repository slice.
type EarlyRentalOfferListPosition struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type EarlyRentalOfferListInput struct {
	ActorID uuid.UUID
	Role    string // empty, "renter", or "host"
	After   *EarlyRentalOfferListPosition
	Limit   int
}

type EarlyRentalOfferListRow struct {
	Offer     earlyrental.Offer
	CreatedAt time.Time
}

type EarlyRentalOfferListResult struct {
	Rows []EarlyRentalOfferListRow
	Next *EarlyRentalOfferListPosition
}

// ErrEarlyRentalOfferListRetry means the candidate page changed while the
// reader acquired participant locks. Roll back this transaction and retry in
// a new READ COMMITTED transaction; never continue with the partial lock set.
var ErrEarlyRentalOfferListRetry = errors.New("early rental offer list changed; retry transaction")

const earlyRentalListOffersSQL = `SELECT o.id,o.creada_en,o.anfitrion_id,o.arrendatario_id
FROM public.arriendo_temprano o
JOIN public.usuario u ON u.id=$1 AND u.estado='activo'
WHERE (o.anfitrion_id=$1 OR o.arrendatario_id=$1)
  AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$1)
  AND ($2='' OR ($2='host' AND o.anfitrion_id=$1) OR ($2='renter' AND o.arrendatario_id=$1))
  AND ($3::timestamptz IS NULL OR (o.creada_en,o.id)<($3,$4::uuid))
ORDER BY o.creada_en DESC,o.id DESC LIMIT $5`

// listEarlyRentalOffers requires an existing caller-owned READ COMMITTED tx.
// Offer snapshots are loaded through the persisted-detail reader, never from
// the mutable current listing. Limit+1 determines continuation without overflow.
func listEarlyRentalOffers(ctx context.Context, tx pgx.Tx, in EarlyRentalOfferListInput) (EarlyRentalOfferListResult, error) {
	invalid := func() (EarlyRentalOfferListResult, error) {
		return EarlyRentalOfferListResult{}, earlyrental.ErrInvalid
	}
	if ctx == nil || tx == nil || in.ActorID == uuid.Nil || in.Limit < 1 || in.Limit > 100 || (in.Role != "" && in.Role != "host" && in.Role != "renter") {
		return invalid()
	}
	if in.After != nil && (in.After.CreatedAt.IsZero() || in.After.ID == uuid.Nil) {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	var afterAt any
	var afterID any
	if in.After != nil {
		afterAt, afterID = in.After.CreatedAt.UTC(), in.After.ID
	}
	rows, err := tx.Query(ctx, earlyRentalListOffersSQL, in.ActorID, in.Role, afterAt, afterID, in.Limit+1)
	if err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	defer rows.Close()
	type key struct {
		id, host, renter uuid.UUID
		at               time.Time
	}
	keys := make([]key, 0, in.Limit+1)
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.id, &k.at, &k.host, &k.renter); err != nil {
			return EarlyRentalOfferListResult{}, err
		}
		if k.id == uuid.Nil || k.at.IsZero() {
			return invalid()
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	// Lock the actor and every discovered counterpart in one canonical order.
	participants := make([][2]uuid.UUID, 0, len(keys))
	for _, k := range keys {
		participants = append(participants, [2]uuid.UUID{k.host, k.renter})
	}
	if err := lockEarlyRentalOfferAccounts(ctx, tx, in.ActorID, participants); err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	// Re-evaluate eligibility and page membership after acquiring participant locks.
	checkRows, err := tx.Query(ctx, earlyRentalListOffersSQL, in.ActorID, in.Role, afterAt, afterID, in.Limit+1)
	if err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	var checked []key
	for checkRows.Next() {
		var k key
		if err := checkRows.Scan(&k.id, &k.at, &k.host, &k.renter); err != nil {
			checkRows.Close()
			return EarlyRentalOfferListResult{}, err
		}
		checked = append(checked, k)
	}
	checkRows.Close()
	if err := checkRows.Err(); err != nil {
		return EarlyRentalOfferListResult{}, err
	}
	if len(checked) != len(keys) {
		return EarlyRentalOfferListResult{}, ErrEarlyRentalOfferListRetry
	}
	for i := range keys {
		if keys[i] != checked[i] {
			return EarlyRentalOfferListResult{}, ErrEarlyRentalOfferListRetry
		}
	}
	result := EarlyRentalOfferListResult{Rows: make([]EarlyRentalOfferListRow, 0, min(len(keys), in.Limit))}
	hasMore := len(keys) > in.Limit
	if hasMore {
		keys = keys[:in.Limit]
	}
	for _, k := range keys {
		stored, err := readEarlyRentalOffer(ctx, tx, in.ActorID, k.id)
		if err != nil {
			if errors.Is(err, rentalflow.ErrNotFound) {
				return EarlyRentalOfferListResult{}, rentalflow.ErrNotFound
			}
			return EarlyRentalOfferListResult{}, err
		}
		if !stored.CreatedAt.Equal(k.at) {
			return invalid()
		}
		result.Rows = append(result.Rows, EarlyRentalOfferListRow{Offer: stored.Offer, CreatedAt: stored.CreatedAt})
	}
	if hasMore && len(result.Rows) > 0 {
		last := result.Rows[len(result.Rows)-1]
		result.Next = &EarlyRentalOfferListPosition{CreatedAt: last.CreatedAt, ID: mustEarlyRentalUUID(last.Offer.ID)}
	}
	return result, nil
}

func lockEarlyRentalOfferAccounts(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, participants [][2]uuid.UUID) error {
	lockIDs := make([]string, 0, 1+2*len(participants))
	lockIDs = append(lockIDs, actorID.String())
	for _, pair := range participants {
		lockIDs = append(lockIDs, pair[0].String(), pair[1].String())
	}
	return accountlock.LockAccountIDs(ctx, tx, lockIDs...)
}

func mustEarlyRentalUUID(value string) uuid.UUID { id, _ := uuid.Parse(value); return id }
