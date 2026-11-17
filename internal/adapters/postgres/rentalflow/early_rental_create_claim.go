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

const earlyRentalCreateClaimSQL = `INSERT INTO public.arriendo_temprano_idempotencia
(actor_id,arriendo_id,comando,clave,huella,evento_id,creado_en)
SELECT $1,a.id,'crear_oferta',$3,$4,e.id,a.creada_en
FROM public.arriendo_temprano AS a
JOIN public.arriendo_temprano_evento AS e ON e.arriendo_id=a.id
WHERE a.id=$2 AND a.arrendatario_id=$1
AND a.estado='pendiente_identidad' AND a.revision=1
AND e.tipo='oferta_creada' AND e.actor_id=$1 AND e.secuencia=1 AND e.revision=1
AND e.estado_anterior IS NULL AND e.estado_nuevo='pendiente_identidad'
AND e.ocurrido_en=a.creada_en
RETURNING evento_id`

// insertEarlyRentalCreateClaim binds the create command to its initial event.
// The caller must hold the actor/key lock, observe a lookup miss, and insert the
// initial offer/hold/event in this same transaction. It must roll back on any
// error, including cancellation after INSERT. This helper never commits and
// does not authorize, resolve availability or implement the complete replay.
func insertEarlyRentalCreateClaim(ctx context.Context, tx pgx.Tx, actorID, offerID uuid.UUID, key string, fingerprint [32]byte) (uuid.UUID, error) {
	if ctx == nil || tx == nil || actorID == uuid.Nil || offerID == uuid.Nil || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
		return uuid.Nil, earlyrental.ErrInvalid
	}
	if n := utf8.RuneCountInString(key); n < 8 || n > 200 {
		return uuid.Nil, earlyrental.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}
	var eventID uuid.UUID
	if err := tx.QueryRow(ctx, earlyRentalCreateClaimSQL, actorID, offerID, key, fingerprint[:]).Scan(&eventID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, earlyrental.ErrInvalid
		}
		return uuid.Nil, err
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}
	if eventID == uuid.Nil {
		return uuid.Nil, earlyrental.ErrInvalid
	}
	return eventID, nil
}
