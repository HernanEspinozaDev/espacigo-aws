package pricingpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaceversion"
	"github.com/HernanEspinozaDev/espaciGo/internal/pricing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func scanRate(row pgx.Row) (pricing.Rate, error) {
	var r pricing.Rate
	err := row.Scan(&r.SpaceID, &r.Version, &r.Unit, &r.Amount, &r.Currency, &r.CreatedAt)
	return r, mapError(err)
}

const rateCols = `espacio_id::text,version,modalidad,precio_base_clp,moneda,creada_en`
const joinedRateCols = `t.espacio_id::text,t.version,t.modalidad,t.precio_base_clp,t.moneda,t.creada_en`

func (r *Repository) CurrentRate(ctx context.Context, owner, spaceID string) (pricing.Rate, error) {
	return scanRate(r.pool.QueryRow(ctx, `SELECT `+joinedRateCols+` FROM public.tarifa_espacio t JOIN public.espacio e ON e.id=t.espacio_id WHERE e.id=$1 AND e.estado IN ('borrador','activa','oculta') AND `+memberAccess("e", "$2")+` ORDER BY t.version DESC LIMIT 1`, spaceID, owner))
}
func (r *Repository) RateHistory(ctx context.Context, owner, spaceID string) ([]pricing.Rate, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+joinedRateCols+` FROM public.tarifa_espacio t JOIN public.espacio e ON e.id=t.espacio_id WHERE e.id=$1 AND e.estado IN ('borrador','activa','oculta') AND `+memberAccess("e", "$2")+` ORDER BY t.version`, spaceID, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pricing.Rate{}
	for rows.Next() {
		item, err := scanRate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *Repository) UpdateRate(ctx context.Context, owner, spaceID string, in pricing.RateInput) (pricing.Rate, error) {
	item, _, err := r.updateRateCAS(ctx, owner, spaceID, time.Time{}, in)
	return item, err
}

func (r *Repository) UpdateRateIfRevision(ctx context.Context, owner, spaceID string, expected time.Time, in pricing.RateInput) (pricing.Rate, time.Time, error) {
	if expected.IsZero() {
		return pricing.Rate{}, time.Time{}, pricing.ErrInvalid
	}
	return r.updateRateCAS(ctx, owner, spaceID, expected, in)
}

func (r *Repository) updateRateCAS(ctx context.Context, owner, spaceID string, expected time.Time, in pricing.RateInput) (pricing.Rate, time.Time, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	if !active {
		return pricing.Rate{}, time.Time{}, pricing.ErrNotFound
	}
	unit, amount, revision, err := lockPricingSpace(ctx, tx, owner, spaceID, true)
	if err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	if !expected.IsZero() && !revision.Equal(expected) {
		return pricing.Rate{}, time.Time{}, pricing.ErrPreconditionFailed
	}
	if unit == in.Unit && amount == in.Amount {
		item, err := scanRate(tx.QueryRow(ctx, `SELECT `+rateCols+` FROM public.tarifa_espacio WHERE espacio_id=$1 ORDER BY version DESC LIMIT 1`, spaceID))
		if err != nil {
			return pricing.Rate{}, time.Time{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return pricing.Rate{}, time.Time{}, err
		}
		return item, revision, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE public.espacio SET modalidad_tarifa=$2,precio_base_clp=$3 WHERE id=$1 AND estado IN ('borrador','activa','oculta')`, spaceID, in.Unit, in.Amount); err != nil {
		return pricing.Rate{}, time.Time{}, mapError(err)
	}
	item, err := scanRate(tx.QueryRow(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) SELECT $1,COALESCE(max(version),0)+1,$2,$3 FROM public.tarifa_espacio WHERE espacio_id=$1 RETURNING `+rateCols, spaceID, in.Unit, in.Amount))
	if err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	revision, err = spaceversion.Bump(ctx, tx, spaceID)
	if err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return pricing.Rate{}, time.Time{}, err
	}
	return item, revision, nil
}
func (r *Repository) CreateSimulation(ctx context.Context, owner, spaceID, id, zone string, rate pricing.Rate, start, end time.Time, units, subtotal int64) (pricing.Simulation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pricing.Simulation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return pricing.Simulation{}, err
	}
	if !active {
		return pricing.Simulation{}, pricing.ErrNotFound
	}
	if _, _, _, err = lockPricingSpace(ctx, tx, owner, spaceID, false); err != nil {
		return pricing.Simulation{}, err
	}
	var s pricing.Simulation
	err = tx.QueryRow(ctx, `INSERT INTO public.simulacion_precio_privada(id,espacio_id,propietario_id,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades_facturadas,subtotal_clp,inicio,termino,zona_horaria)
	SELECT $1,e.id,e.propietario_id,$4,$5,$6,'CLP',$7,$8,$9,$10,$11 FROM public.espacio e WHERE e.id=$2 AND e.estado='borrador' AND `+memberAccess("e", "$3")+`
	RETURNING id::text,espacio_id::text,tarifa_version,modalidad,precio_unitario_clp,unidades_facturadas,moneda,subtotal_clp,inicio,termino,zona_horaria,true,creada_en`, id, spaceID, owner, rate.Version, rate.Unit, rate.Amount, units, subtotal, start, end, zone).Scan(&s.ID, &s.SpaceID, &s.RateVersion, &s.RateUnit, &s.BasePrice, &s.BilledUnits, &s.Currency, &s.Subtotal, &s.StartAt, &s.EndAt, &s.TimeZone, &s.Private, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.Simulation{}, pricing.ErrNotFound
	}
	if err != nil {
		return pricing.Simulation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return pricing.Simulation{}, err
	}
	s.StartAt = s.StartAt.UTC()
	s.EndAt = s.EndAt.UTC()
	s.CreatedAt = s.CreatedAt.UTC()
	return s, nil
}
func (r *Repository) GetSimulation(ctx context.Context, owner, spaceID, id string) (pricing.Simulation, error) {
	var s pricing.Simulation
	err := r.pool.QueryRow(ctx, `SELECT s.id::text,s.espacio_id::text,s.tarifa_version,s.modalidad,s.precio_unitario_clp,s.unidades_facturadas,s.moneda,s.subtotal_clp,s.inicio,s.termino,s.zona_horaria,true,s.creada_en FROM public.simulacion_precio_privada s JOIN public.espacio e ON e.id=s.espacio_id WHERE s.id=$1 AND s.espacio_id=$2 AND `+memberAccess("e", "$3"), id, spaceID, owner).Scan(&s.ID, &s.SpaceID, &s.RateVersion, &s.RateUnit, &s.BasePrice, &s.BilledUnits, &s.Currency, &s.Subtotal, &s.StartAt, &s.EndAt, &s.TimeZone, &s.Private, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.Simulation{}, pricing.ErrNotFound
	}
	if err != nil {
		return pricing.Simulation{}, err
	}
	s.StartAt = s.StartAt.UTC()
	s.EndAt = s.EndAt.UTC()
	s.CreatedAt = s.CreatedAt.UTC()
	return s, nil
}

func memberAccess(alias, actor string) string {
	return `( (` + alias + `.modo_anfitrion='personal' AND ` + alias + `.propietario_id=` + actor + `) OR (` + alias + `.modo_anfitrion='empresa' AND EXISTS (
		SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=` + alias + `.organizacion_id AND m.usuario_id=` + actor + ` AND m.estado='activo'
		AND EXISTS(SELECT 1 FROM public.account_profile_identity_status(` + actor + `,clock_timestamp()) k WHERE k.verified))) )`
}

func lockPricingSpace(ctx context.Context, tx pgx.Tx, actor, spaceID string, edit bool) (string, int64, time.Time, error) {
	var unit, state, mode, ownerID string
	var amount int64
	var orgID *string
	var revision time.Time
	err := tx.QueryRow(ctx, `SELECT modalidad_tarifa,precio_base_clp,actualizado_en,estado,modo_anfitrion,propietario_id::text,organizacion_id::text FROM public.espacio WHERE id=$1 FOR UPDATE`, spaceID).Scan(&unit, &amount, &revision, &state, &mode, &ownerID, &orgID)
	if err != nil {
		return "", 0, time.Time{}, mapError(err)
	}
	if state != "borrador" && state != "activa" && state != "oculta" {
		return "", 0, time.Time{}, pricing.ErrNotFound
	}
	if mode == "personal" {
		if ownerID != actor {
			return "", 0, time.Time{}, pricing.ErrNotFound
		}
	} else if mode == "empresa" && orgID != nil {
		var role string
		err = tx.QueryRow(ctx, `SELECT rol FROM public.organizacion_miembro WHERE organizacion_id=$1 AND usuario_id=$2 AND estado='activo' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,clock_timestamp()) k WHERE k.verified) FOR SHARE`, *orgID, actor).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, time.Time{}, pricing.ErrNotFound
		}
		if err != nil {
			return "", 0, time.Time{}, err
		}
		if edit && role != "representante_legal" && role != "administrador" {
			return "", 0, time.Time{}, pricing.ErrNotFound
		}
	} else {
		return "", 0, time.Time{}, pricing.ErrNotFound
	}
	if edit {
		var canPublish bool
		if err = tx.QueryRow(ctx, `SELECT can_publish FROM public.kyb_space_eligibility($1)`, spaceID).Scan(&canPublish); err != nil {
			return "", 0, time.Time{}, err
		}
		if !canPublish {
			return "", 0, time.Time{}, pricing.ErrNotFound
		}
	}
	return unit, amount, revision, nil
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23514" && (pgErr.ConstraintName == "modalidad_horario_semanal_activo_ck" || pgErr.ConstraintName == "tarifa_horario_semanal_activo_ck") {
			return pricing.ErrCalendarConflict
		}
		if pgErr.Code == "23503" || pgErr.Code == "23514" || pgErr.Code == "22003" || pgErr.Code == "23505" {
			return pricing.ErrInvalid
		}
	}
	return err
}
