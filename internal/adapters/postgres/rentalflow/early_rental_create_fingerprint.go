package rentalflowpg

import (
	"crypto/sha256"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func earlyRentalCreateFingerprint(spaceID uuid.UUID, startAt, endAt time.Time) ([32]byte, error) {
	if spaceID == uuid.Nil || startAt.IsZero() || endAt.IsZero() || !endAt.After(startAt) {
		return [32]byte{}, earlyrental.ErrInvalid
	}

	preimage := "espacigo.early-rental.create-request.v1\x00" + spaceID.String() + "\x00" +
		startAt.UTC().Format(time.RFC3339Nano) + "\x00" + endAt.UTC().Format(time.RFC3339Nano)
	return sha256.Sum256([]byte(preimage)), nil
}
