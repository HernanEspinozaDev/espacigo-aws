package rentalflowpg

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errEarlyRentalCreateKeyConflict = errors.New("early rental create idempotency key conflict")
var errEarlyRentalCreateReplayAmbiguous = errors.New("early rental create replay is ambiguous")

const earlyRentalCreateReplayLookupSQL = `SELECT i.actor_id, i.clave, i.arriendo_id, i.huella, a.arrendatario_id, e.id, e.tipo, e.actor_id, e.secuencia
FROM public.arriendo_temprano_idempotencia AS i
JOIN public.arriendo_temprano AS a ON a.id = i.arriendo_id
JOIN public.arriendo_temprano_evento AS e ON e.id = i.evento_id AND e.arriendo_id = i.arriendo_id
WHERE i.actor_id = $1 AND i.comando = 'crear_oferta' AND i.clave COLLATE "C" = $2::text COLLATE "C"
ORDER BY i.arriendo_id
LIMIT 2`

// lookupEarlyRentalCreateReplay requires the caller's actor/key transaction lock
// and a fresh statement snapshot acquired after that lock (READ COMMITTED).
// A pre-lock repeatable-read snapshot can miss a predecessor's committed row.
// This lookup does not authorize the actor or load the current offer detail.
func lookupEarlyRentalCreateReplay(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, key string, fingerprint [32]byte) (uuid.UUID, bool, error) {
	invalid := func() (uuid.UUID, bool, error) { return uuid.Nil, false, earlyrental.ErrInvalid }
	if ctx == nil || tx == nil || actorID == uuid.Nil || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
		return invalid()
	}
	n := utf8.RuneCountInString(key)
	if n < 8 || n > 200 {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, false, err
	}
	rows, err := tx.Query(ctx, earlyRentalCreateReplayLookupSQL, actorID, key)
	if err != nil {
		return uuid.Nil, false, err
	}
	defer rows.Close()
	type result struct {
		idemActor uuid.UUID
		key       string
		offer     uuid.UUID
		digest    []byte
		renter    uuid.UUID
		event     uuid.UUID
		kind      string
		actor     uuid.UUID
		seq       int64
	}
	collected := make([]result, 0, 2)
	for rows.Next() {
		var r result
		if err := rows.Scan(&r.idemActor, &r.key, &r.offer, &r.digest, &r.renter, &r.event, &r.kind, &r.actor, &r.seq); err != nil {
			return uuid.Nil, false, err
		}
		collected = append(collected, r)
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, false, err
	}
	if len(collected) > 1 {
		return uuid.Nil, false, errEarlyRentalCreateReplayAmbiguous
	}
	if len(collected) == 0 {
		return uuid.Nil, false, nil
	}
	r := collected[0]
	if r.idemActor != actorID || r.key != key || r.offer == uuid.Nil || r.event == uuid.Nil || r.actor != actorID || r.renter != actorID || r.kind != "oferta_creada" || r.seq != 1 || len(r.digest) != 32 {
		return invalid()
	}
	if !bytesEqual32(r.digest, fingerprint[:]) {
		return uuid.Nil, false, errEarlyRentalCreateKeyConflict
	}
	return r.offer, true, nil
}

func bytesEqual32(a, b []byte) bool {
	if len(a) != 32 || len(b) != 32 {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
