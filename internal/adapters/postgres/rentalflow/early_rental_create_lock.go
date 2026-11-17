package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// lockEarlyRentalCreateKey serializes an actor/key pair for the caller's transaction.
// It is not proof of equality or replay authority: the later lookup must compare the
// complete actor, key, and fingerprint. Call before account locks and space-row locks.
func lockEarlyRentalCreateKey(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, key string) error {
	if ctx == nil || tx == nil || actorID == uuid.Nil || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
		return earlyrental.ErrInvalid
	}
	n := utf8.RuneCountInString(key)
	if n < 8 || n > 200 {
		return earlyrental.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	preimage := "espacigo.early-rental.create-idempotency.v1\x00" + actorID.String() + "\x00" + key
	digest := sha256.Sum256([]byte(preimage))
	object := int32(binary.BigEndian.Uint32(digest[:4]))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1,$2)`, int32(0x45524331), object); err != nil {
		return err
	}
	return ctx.Err()
}
