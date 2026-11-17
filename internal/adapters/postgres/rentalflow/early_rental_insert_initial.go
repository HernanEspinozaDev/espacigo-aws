package rentalflowpg

import (
	"context"
	"errors"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/jackc/pgx/v5"
)

// insertEarlyRentalInitial writes only the initial offer, hold and event in a
// caller-owned transaction. The caller must roll back on any error (including
// cancellation) and commit on success so deferred integrity checks execute.
// This primitive does not authorize, resolve availability or implement replay.
func insertEarlyRentalInitial(ctx context.Context, tx pgx.Tx, record earlyRentalInitialRecord) error {
	if ctx == nil || tx == nil {
		return earlyrental.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	validated, err := newEarlyRentalInitialRecord(record.Offer, record.OccupancyID, record.InitialEvent, record.CreatedAt)
	if err != nil || !record.UpdatedAt.Equal(validated.UpdatedAt) || !equalDigest(record.ContractDigest, validated.ContractDigest) {
		return earlyrental.ErrInvalid
	}
	for _, stmt := range initialInsertStatements {
		if err := ctx.Err(); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, stmt.sql, stmt.args(validated)...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("early rental initial insert: unexpected affected row count")
		}
	}
	return ctx.Err()
}

func equalDigest(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type initialInsert struct {
	sql  string
	args func(earlyRentalInitialRecord) []any
}

var initialInsertStatements = []initialInsert{
	{`INSERT INTO public.arriendo_temprano (id, espacio_id, anfitrion_id, arrendatario_id, titulo_snapshot, modalidad, precio_unitario_clp, moneda, unidades, subtotal_clp, inicio, termino, zona_horaria, contrato_version, contrato_texto, contrato_digest, estado, revision, creada_en, actualizada_en, vence_en, ocupacion_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,1,$18,$18,$19,$20)`, func(r earlyRentalInitialRecord) []any {
		o := r.Offer
		s := o.Snapshot
		return []any{o.ID, s.SpaceID, o.HostID, o.RenterID, s.SpaceTitle, s.RateUnit, s.UnitPriceCLP, s.Currency, s.Units, s.SubtotalCLP, s.StartAt, s.EndAt, s.TimeZone, o.ContractVersion, o.ContractText, r.ContractDigest, o.State, r.CreatedAt, o.ExpiresAt, r.OccupancyID}
	}},
	{`INSERT INTO public.ocupacion (id, espacio_id, reserva_id, intervalo, tipo, activo, expira_en, motivo, creada_en, desactivada_en, arriendo_temprano_id) VALUES ($1,$2,NULL,tstzrange($3,$4,'[)'),'retencion_temprana',true,$5,NULL,$6,NULL,$7)`, func(r earlyRentalInitialRecord) []any {
		o := r.Offer
		s := o.Snapshot
		return []any{r.OccupancyID, s.SpaceID, s.StartAt, s.EndAt, o.ExpiresAt, r.CreatedAt, o.ID}
	}},
	{`INSERT INTO public.arriendo_temprano_evento (arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,ocurrido_en) VALUES ($1,1,1,$2,$3,NULL,$4,$5)`, func(r earlyRentalInitialRecord) []any {
		o := r.Offer
		e := r.InitialEvent
		return []any{o.ID, e.Kind, e.ActorID, e.NextState, e.At}
	}},
}
