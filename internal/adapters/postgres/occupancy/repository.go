package occupancy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaceversion"
	domain "github.com/HernanEspinozaDev/espaciGo/internal/occupancy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) SetTimeZone(ctx context.Context, owner, spaceID, zone string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return err
	}
	if !active {
		return domain.ErrNotFound
	}
	state, currentZone, err := lockCommercialSpace(ctx, tx, owner, spaceID, true)
	if err != nil {
		return err
	}
	if state == "activa" || state == "oculta" {
		if currentZone != nil && strings.TrimSpace(*currentZone) != "" {
			if *currentZone == zone {
				return tx.Commit(ctx)
			}
			return domain.ErrTimezoneLocked
		}
		var hasReservations bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_ensayo_local WHERE espacio_id=$1)`, spaceID).Scan(&hasReservations); err != nil {
			return err
		}
		if hasReservations {
			return domain.ErrTimezoneInUse
		}
	} else if state != "borrador" {
		return domain.ErrNotFound
	}
	result, err := tx.Exec(ctx, `UPDATE public.espacio
		SET zona_horaria=$2, actualizado_en=now()
		WHERE id=$1 AND estado=$3
		AND ($3='borrador' OR zona_horaria IS NULL OR btrim(zona_horaria)='')`, spaceID, zone, state)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *Repository) SetTimeZoneCAS(ctx context.Context, owner, spaceID, zone string, expected time.Time) (time.Time, error) {
	if expected.IsZero() {
		return time.Time{}, domain.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return time.Time{}, err
	}
	if !active {
		return time.Time{}, domain.ErrNotFound
	}
	state, currentZone, err := lockCommercialSpace(ctx, tx, owner, spaceID, true)
	if err != nil {
		return time.Time{}, err
	}
	var revision time.Time
	err = tx.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, spaceID).Scan(&revision)
	if err != nil {
		return time.Time{}, mapError(err)
	}
	if !revision.Equal(expected) {
		return time.Time{}, domain.ErrPreconditionFailed
	}
	if state == "activa" || state == "oculta" {
		if currentZone != nil && strings.TrimSpace(*currentZone) != "" {
			if *currentZone == zone {
				return revision, tx.Commit(ctx)
			}
			return time.Time{}, domain.ErrTimezoneLocked
		}
		var hasReservations bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_ensayo_local WHERE espacio_id=$1)`, spaceID).Scan(&hasReservations); err != nil {
			return time.Time{}, err
		}
		if hasReservations {
			return time.Time{}, domain.ErrTimezoneInUse
		}
	} else if state != "borrador" {
		return time.Time{}, domain.ErrNotFound
	}
	if currentZone != nil && *currentZone == zone {
		return revision, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE public.espacio SET zona_horaria=$2 WHERE id=$1`, spaceID, zone); err != nil {
		return time.Time{}, err
	}
	revision, err = spaceversion.Bump(ctx, tx, spaceID)
	if err != nil {
		return time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return time.Time{}, err
	}
	return revision, nil
}

func (r *Repository) TimeZone(ctx context.Context, owner, spaceID string) (string, error) {
	return r.timeZone(ctx, owner, spaceID)
}

func (r *Repository) Availability(ctx context.Context, owner, spaceID string, start, end time.Time) (domain.Availability, error) {
	var zone *string
	var available bool
	err := r.pool.QueryRow(ctx, `SELECT e.zona_horaria,
		NOT EXISTS (SELECT 1 FROM public.ocupacion o
			WHERE o.espacio_id=e.id AND o.activo AND o.intervalo && tstzrange($3,$4,'[)'))
		FROM public.espacio e WHERE e.id=$1 AND e.estado='borrador' AND `+memberAccess("e", "$2"),
		spaceID, owner, start, end).Scan(&zone, &available)
	if err != nil {
		return domain.Availability{}, mapError(err)
	}
	if zone == nil || *zone == "" {
		return domain.Availability{}, domain.ErrTimezoneRequired
	}
	return domain.Availability{SpaceID: spaceID, TimeZone: *zone, StartAt: start, EndAt: end, Available: available}, nil
}

func (r *Repository) ListBlocks(ctx context.Context, owner, spaceID string, start, end time.Time) (domain.Calendar, error) {
	zone, err := r.timeZone(ctx, owner, spaceID)
	if err != nil {
		return domain.Calendar{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT o.id::text, o.espacio_id::text,
		lower(o.intervalo), upper(o.intervalo), o.motivo, o.creada_en
		FROM public.ocupacion o JOIN public.espacio e ON e.id=o.espacio_id
		WHERE e.id=$1 AND e.estado='borrador' AND `+memberAccess("e", "$2")+`
		AND o.tipo='bloqueo_manual' AND o.activo AND o.intervalo && tstzrange($3,$4,'[)')
		ORDER BY lower(o.intervalo), o.id`, spaceID, owner, start, end)
	if err != nil {
		return domain.Calendar{}, err
	}
	defer rows.Close()
	calendar := domain.Calendar{SpaceID: spaceID, TimeZone: zone, Items: []domain.Block{}}
	for rows.Next() {
		var block domain.Block
		block.TimeZone = zone
		if err := rows.Scan(&block.ID, &block.SpaceID, &block.StartAt, &block.EndAt, &block.Reason, &block.CreatedAt); err != nil {
			return domain.Calendar{}, err
		}
		block.StartAt, block.EndAt, block.CreatedAt = block.StartAt.UTC(), block.EndAt.UTC(), block.CreatedAt.UTC()
		calendar.Items = append(calendar.Items, block)
	}
	return calendar, rows.Err()
}

func (r *Repository) CreateBlock(ctx context.Context, owner, spaceID, blockID string, start, end time.Time, reason string) (domain.Block, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Block{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return domain.Block{}, err
	}
	if !active {
		return domain.Block{}, domain.ErrNotFound
	}
	state, zone, err := lockCommercialSpace(ctx, tx, owner, spaceID, false)
	if err != nil {
		return domain.Block{}, err
	}
	if state != "borrador" {
		return domain.Block{}, domain.ErrNotFound
	}
	if zone == nil || *zone == "" {
		return domain.Block{}, domain.ErrTimezoneRequired
	}
	var block domain.Block
	block.TimeZone = *zone
	err = tx.QueryRow(ctx, `INSERT INTO public.ocupacion
		(id, espacio_id, reserva_id, intervalo, tipo, activo, motivo)
		SELECT $1,e.id,NULL,tstzrange($4,$5,'[)'),'bloqueo_manual',true,$6
		FROM public.espacio e WHERE e.id=$2 AND e.estado='borrador' AND `+memberAccess("e", "$3")+`
		RETURNING id::text, espacio_id::text, lower(intervalo), upper(intervalo), motivo, creada_en`,
		blockID, spaceID, owner, start, end, reason).Scan(&block.ID, &block.SpaceID, &block.StartAt, &block.EndAt, &block.Reason, &block.CreatedAt)
	if err != nil {
		return domain.Block{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Block{}, mapError(err)
	}
	block.StartAt, block.EndAt, block.CreatedAt = block.StartAt.UTC(), block.EndAt.UTC(), block.CreatedAt.UTC()
	return block, nil
}

func (r *Repository) DeleteBlock(ctx context.Context, owner, spaceID, blockID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	active, err := accountlock.LockActive(ctx, tx, owner)
	if err != nil {
		return err
	}
	if !active {
		return domain.ErrNotFound
	}
	_, _, err = lockCommercialSpace(ctx, tx, owner, spaceID, false)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE public.ocupacion o SET activo=false, desactivada_en=now()
		FROM public.espacio e WHERE o.espacio_id=e.id AND e.id=$1
		AND e.estado='borrador' AND `+memberAccess("e", "$2")+` AND o.id=$3 AND o.tipo='bloqueo_manual' AND o.activo`, spaceID, owner, blockID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func memberAccess(alias, actor string) string {
	return `( (` + alias + `.modo_anfitrion='personal' AND ` + alias + `.propietario_id=` + actor + `) OR (` + alias + `.modo_anfitrion='empresa' AND EXISTS (
		SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=` + alias + `.organizacion_id AND m.usuario_id=` + actor + ` AND m.estado='activo'
		AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(` + actor + `,clock_timestamp()) k WHERE k.verified))) )`
}

func lockCommercialSpace(ctx context.Context, tx pgx.Tx, actor, spaceID string, timezoneEdit bool) (string, *string, error) {
	var state, mode, ownerID string
	var orgID *string
	var zone *string
	err := tx.QueryRow(ctx, `SELECT estado,modo_anfitrion,propietario_id::text,organizacion_id::text,zona_horaria FROM public.espacio WHERE id=$1 FOR UPDATE`, spaceID).Scan(&state, &mode, &ownerID, &orgID, &zone)
	if err != nil {
		return "", nil, mapError(err)
	}
	if mode == "personal" {
		if ownerID != actor {
			return "", nil, domain.ErrNotFound
		}
	} else if mode == "empresa" && orgID != nil {
		var role string
		err = tx.QueryRow(ctx, `SELECT rol FROM public.organizacion_miembro WHERE organizacion_id=$1 AND usuario_id=$2 AND estado='activo' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,clock_timestamp()) k WHERE k.verified) FOR SHARE`, *orgID, actor).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, domain.ErrNotFound
		}
		if err != nil {
			return "", nil, err
		}
		if timezoneEdit && role != "representante_legal" && role != "administrador" {
			return "", nil, domain.ErrNotFound
		}
	} else {
		return "", nil, domain.ErrNotFound
	}
	var canPublish bool
	if err = tx.QueryRow(ctx, `SELECT can_publish FROM public.kyb_space_eligibility($1)`, spaceID).Scan(&canPublish); err != nil {
		return "", nil, err
	}
	if !canPublish {
		return "", nil, domain.ErrNotFound
	}
	return state, zone, nil
}

func (r *Repository) timeZone(ctx context.Context, owner, spaceID string) (string, error) {
	var zone *string
	err := r.pool.QueryRow(ctx, `SELECT zona_horaria FROM public.espacio e
		WHERE e.id=$1 AND e.estado IN ('borrador','activa','oculta') AND `+memberAccess("e", "$2"), spaceID, owner).Scan(&zone)
	if err != nil {
		return "", mapError(err)
	}
	if zone == nil || *zone == "" {
		return "", domain.ErrTimezoneRequired
	}
	return *zone, nil
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01":
			return domain.ErrConflict
		case "23514", "22007", "22008", "22023":
			return domain.ErrInvalid
		}
	}
	return err
}
