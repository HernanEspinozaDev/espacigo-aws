package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type earlyRentalStoredOffer struct {
	Offer                earlyrental.Offer
	CreatedAt, UpdatedAt time.Time
	LastEventSequence    uint64
}

const earlyRentalOfferHeaderSQL = `SELECT anfitrion_id, arrendatario_id FROM public.arriendo_temprano WHERE id=$1 AND $2 IN (anfitrion_id,arrendatario_id)`

// readEarlyRentalOffer reads one participant-visible persisted snapshot. The
// caller owns a READ COMMITTED transaction; this function never ends it.
func readEarlyRentalOffer(ctx context.Context, tx pgx.Tx, actorID, offerID uuid.UUID) (earlyRentalStoredOffer, error) {
	invalid := func() (earlyRentalStoredOffer, error) { return earlyRentalStoredOffer{}, earlyrental.ErrInvalid }
	if ctx == nil || tx == nil || actorID == uuid.Nil || offerID == uuid.Nil {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalStoredOffer{}, err
	}
	var hostID, renterID uuid.UUID
	if err := tx.QueryRow(ctx, earlyRentalOfferHeaderSQL, offerID, actorID).Scan(&hostID, &renterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return earlyRentalStoredOffer{}, rentalflow.ErrNotFound
		}
		return earlyRentalStoredOffer{}, err
	}
	if hostID == uuid.Nil || renterID == uuid.Nil || hostID == renterID || (actorID != hostID && actorID != renterID) {
		return invalid()
	}
	if err := accountlock.LockAccountIDs(ctx, tx, hostID.String(), renterID.String()); err != nil {
		return earlyRentalStoredOffer{}, err
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalStoredOffer{}, err
	}
	const query = `SELECT o.flow_version,o.espacio_id,o.anfitrion_id,o.arrendatario_id,o.titulo_snapshot,o.modalidad,o.precio_unitario_clp,o.moneda,o.unidades,o.subtotal_clp,o.inicio,o.termino,o.zona_horaria,o.contrato_version,o.contrato_texto,o.contrato_digest,o.estado,o.revision,o.creada_en,o.actualizada_en,o.vence_en,
COALESCE((SELECT jsonb_agg(jsonb_build_object('actor',c.actor_id,'class',c.clase,'at',c.confirmada_en,'version',c.contrato_version,'digest',encode(c.contrato_digest,'hex')) ORDER BY c.actor_id,c.clase) FROM public.arriendo_temprano_confirmacion c WHERE c.arriendo_id=o.id),'[]'::jsonb),
(SELECT jsonb_build_object('id',p.pago_id,'sender',p.emisor_id,'receiver',p.receptor_id,'amount',p.monto_clp,'currency',p.moneda,'state',p.estado,'sent',p.enviado_en,'received',p.recibido_en) FROM public.arriendo_temprano_pago p WHERE p.arriendo_id=o.id),
(SELECT count(*) FROM public.arriendo_temprano_evento e WHERE e.arriendo_id=o.id),(SELECT COALESCE(max(e.secuencia),0) FROM public.arriendo_temprano_evento e WHERE e.arriendo_id=o.id)
FROM public.arriendo_temprano o WHERE o.id=$1 AND o.anfitrion_id=$2 AND o.arrendatario_id=$3 AND $4 IN(o.anfitrion_id,o.arrendatario_id)
AND EXISTS(SELECT 1 FROM public.usuario u WHERE u.id=$4 AND u.estado='activo')
AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$4)`
	var version string
	var space, host, renter uuid.UUID
	var title, unit, currency, zone, contract, state string
	var price, units, subtotal, contractVersion, revision int64
	var start, end, created, updated, expires time.Time
	var digest []byte
	var confirms, payment []byte
	var eventCount, maxSequence int64
	err := tx.QueryRow(ctx, query, offerID, hostID, renterID, actorID).Scan(&version, &space, &host, &renter, &title, &unit, &price, &currency, &units, &subtotal, &start, &end, &zone, &contractVersion, &contract, &digest, &state, &revision, &created, &updated, &expires, &confirms, &payment, &eventCount, &maxSequence)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return earlyRentalStoredOffer{}, rentalflow.ErrNotFound
		}
		return earlyRentalStoredOffer{}, err
	}
	if err := ctx.Err(); err != nil {
		return earlyRentalStoredOffer{}, err
	}
	bad := func() (earlyRentalStoredOffer, error) { return earlyRentalStoredOffer{}, earlyrental.ErrInvalid }
	if version != "manual_early_v1" || revision <= 0 || eventCount != revision || maxSequence != revision || len(digest) != 32 || created.IsZero() || updated.Before(created) || expires.IsZero() {
		return bad()
	}
	if !expires.Equal(minTime(created.Add(earlyrental.HoldTTL), start)) {
		return bad()
	}
	var confirmations []struct {
		Actor   string     `json:"actor"`
		Class   string     `json:"class"`
		At      *time.Time `json:"at"`
		Version *int64     `json:"version"`
		Digest  *string    `json:"digest"`
	}
	if err = json.Unmarshal(confirms, &confirmations); err != nil {
		return bad()
	}
	o := earlyrental.Offer{ID: offerID.String(), HostID: host.String(), RenterID: renter.String(), State: earlyrental.State(state), Revision: uint64(revision), ContractVersion: contractVersion, ContractText: contract, ContractDigest: hex.EncodeToString(digest), IdentityConfirmed: map[string]time.Time{}, ContractAccepted: map[string]time.Time{}, ExpiresAt: expires.UTC()}
	sum := sha256.Sum256([]byte(contract))
	if string(sum[:]) != string(digest) {
		return bad()
	}
	for _, c := range confirmations {
		if c.At == nil || c.At.IsZero() {
			return bad()
		}
		target := o.IdentityConfirmed
		if c.Class == "contrato" {
			if c.Version == nil || *c.Version != contractVersion || c.Digest == nil || *c.Digest != o.ContractDigest {
				return bad()
			}
			target = o.ContractAccepted
		} else if c.Class != "identidad" || c.Version != nil || c.Digest != nil {
			return bad()
		}
		if _, ok := target[c.Actor]; ok {
			return bad()
		}
		target[c.Actor] = c.At.UTC()
	}
	o.Snapshot = earlyrental.OfferSnapshot{SpaceID: space, SpaceTitle: title, RateUnit: unit, UnitPriceCLP: price, Currency: currency, Units: units, SubtotalCLP: subtotal, StartAt: start.UTC(), EndAt: end.UTC(), TimeZone: zone}
	if len(payment) > 0 && string(payment) != "null" {
		var p struct {
			ID               string `json:"id"`
			Sender, Receiver uuid.UUID
			Amount           int64 `json:"amount"`
			Currency, State  string
			Sent, Received   *time.Time
		}
		if json.Unmarshal(payment, &p) != nil {
			return bad()
		}
		if p.Sender != renter || p.Receiver != host || p.Amount != subtotal || p.Currency != "CLP" {
			return bad()
		}
		// A sent payment keeps its identity after cancellation or expiry.
		// The domain models that identity whenever the sent timestamp exists,
		// not only while the payment's current state is sent/received.
		if p.Sent != nil {
			o.PaymentID = p.ID
		}
		if p.Sent != nil {
			t := p.Sent.UTC()
			o.PaymentSentAt = &t
		}
		if p.Received != nil {
			t := p.Received.UTC()
			o.PaymentReceivedAt = &t
		}
	}
	if earlyrental.ValidateOffer(o) != nil {
		return bad()
	}
	return earlyRentalStoredOffer{Offer: o, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), LastEventSequence: uint64(maxSequence)}, nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
