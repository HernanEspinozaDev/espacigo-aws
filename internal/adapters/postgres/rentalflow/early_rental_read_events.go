package rentalflowpg

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const earlyRentalEventsFinalSQL = `SELECT o.id,o.anfitrion_id,o.arrendatario_id,o.revision,o.estado,
 e.secuencia,e.revision,e.tipo,e.actor_id::text,e.estado_anterior,e.estado_nuevo,e.ocurrido_en,e.confirmacion_id,e.pago_id,e.motivo_codigo,
 c.actor_id,c.clase,c.contrato_version,c.contrato_digest
 FROM public.arriendo_temprano o JOIN public.usuario u ON u.id=$4 AND u.estado='activo'
 LEFT JOIN public.bloqueo_cuenta_administrativo_local b ON b.cuenta_id=$4
 LEFT JOIN public.arriendo_temprano_evento e ON e.arriendo_id=o.id AND e.secuencia>$5
 LEFT JOIN public.arriendo_temprano_confirmacion c ON c.id=e.confirmacion_id AND c.arriendo_id=e.arriendo_id
 WHERE o.id=$1 AND o.anfitrion_id=$2 AND o.arrendatario_id=$3 AND $4 IN(o.anfitrion_id,o.arrendatario_id) AND b.cuenta_id IS NULL ORDER BY e.secuencia`

// The caller owns the READ COMMITTED transaction. Authorization and the
// complete event suffix are read in the same statement after the account locks.
func readEarlyRentalEvents(ctx context.Context, tx pgx.Tx, actorID, offerID uuid.UUID, after uint64) ([]earlyrental.Event, error) {
	invalid := func() ([]earlyrental.Event, error) { return nil, earlyrental.ErrInvalid }
	if ctx == nil || tx == nil || actorID == uuid.Nil || offerID == uuid.Nil || after > math.MaxInt64 {
		return invalid()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var host, renter uuid.UUID
	if err := tx.QueryRow(ctx, earlyRentalOfferHeaderSQL, offerID, actorID).Scan(&host, &renter); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rentalflow.ErrNotFound
		}
		return nil, err
	}
	if host == uuid.Nil || renter == uuid.Nil || host == renter || (actorID != host && actorID != renter) {
		return invalid()
	}
	if err := accountlock.LockAccountIDs(ctx, tx, host.String(), renter.String()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cursor := int64(after)
	rows, err := tx.Query(ctx, earlyRentalEventsFinalSQL, offerID, host, renter, actorID, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]earlyrental.Event, 0)
	var revision int64
	var state string
	seen := false
	expected := cursor
	for rows.Next() {
		var oid, h, r uuid.UUID
		var rev int64
		var current string
		var seq, evrev *int64
		var kind, actor, prev, next, reason *string
		var at *time.Time
		var confirmation, payment *uuid.UUID
		var evidenceActor *uuid.UUID
		var evidenceClass *string
		var contractVersion *int64
		var digest []byte
		if err := rows.Scan(&oid, &h, &r, &rev, &current, &seq, &evrev, &kind, &actor, &prev, &next, &at, &confirmation, &payment, &reason, &evidenceActor, &evidenceClass, &contractVersion, &digest); err != nil {
			return nil, err
		}
		if oid != offerID || h != host || r != renter || rev <= 0 || current == "" {
			return invalid()
		}
		if seen && (revision != rev || state != current || seq == nil) {
			return invalid()
		}
		seen = true
		revision, state = rev, current
		if seq == nil {
			if rev > cursor || evrev != nil || kind != nil || actor != nil || prev != nil || next != nil || at != nil || confirmation != nil || payment != nil || reason != nil || evidenceActor != nil || evidenceClass != nil || contractVersion != nil || digest != nil {
				return invalid()
			}
			continue
		}
		if evrev == nil || *seq > rev || expected == math.MaxInt64 || *seq != expected+1 || *seq != *evrev || kind == nil || next == nil || at == nil {
			return invalid()
		}
		rec := earlyRentalEventRecord{Sequence: *seq, Revision: *evrev, Kind: *kind, OccurredAt: *at, NextState: *next}
		if actor != nil {
			rec.ActorID = *actor
		}
		if prev != nil {
			rec.PreviousState = *prev
		}
		if payment != nil {
			rec.PaymentID = payment.String()
		}
		if reason != nil {
			rec.ReasonCode = *reason
		}
		switch earlyrental.EventKind(rec.Kind) {
		case earlyrental.EventIdentityConfirmed, earlyrental.EventContractAccepted:
			class := "identidad"
			if earlyrental.EventKind(rec.Kind) == earlyrental.EventContractAccepted {
				class = "contrato"
			}
			if confirmation == nil || *confirmation == uuid.Nil || evidenceActor == nil || evidenceClass == nil || *evidenceClass != class || rec.ActorID == "" || evidenceActor.String() != rec.ActorID || (class == "contrato") != (contractVersion != nil) {
				return invalid()
			}
			if contractVersion != nil {
				rec.ContractVersion = *contractVersion
			}
			rec.ContractDigest = digest
		default:
			if confirmation != nil || evidenceActor != nil || evidenceClass != nil || contractVersion != nil || digest != nil {
				return invalid()
			}
		}
		event, ok := mapEarlyRentalEventRecord(rec)
		if !ok {
			return invalid()
		}
		if event.ActorID != "system" && event.ActorID != host.String() && event.ActorID != renter.String() {
			return invalid()
		}
		switch event.Kind {
		case earlyrental.EventOfferCreated, earlyrental.EventPaymentMarkedSent:
			if event.ActorID != renter.String() {
				return invalid()
			}
		case earlyrental.EventPaymentMarkedReceived:
			if event.ActorID != host.String() && event.ActorID != "system" {
				return invalid()
			}
		}
		if len(events) > 0 && event.PreviousState != events[len(events)-1].NextState {
			return invalid()
		}
		events = append(events, event)
		expected = *seq
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !seen {
		return nil, rentalflow.ErrNotFound
	}
	if uint64(revision) <= after {
		if len(events) != 0 {
			return invalid()
		}
		return events, nil
	}
	if expected != revision || len(events) == 0 || events[len(events)-1].NextState != earlyrental.State(state) {
		return invalid()
	}
	return events, nil
}
