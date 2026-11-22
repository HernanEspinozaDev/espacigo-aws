package spacespg

import (
	"context"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaceversion"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
)

const commercialLocationColumns = `id::text,direccion,COALESCE(region_codigo,''),COALESCE(comuna_codigo,''),ubicacion_latitud,ubicacion_longitud,ubicacion_confirmada_en,ubicacion_revision,actualizado_en`

// The caller holds the space lock and has already rejected enabled synthetic
// fixtures. Previously published spaces retain their commercial visibility.
func requireConfirmedLocationForFirstPublication(ctx context.Context, tx pgx.Tx, id, previousState string) error {
	if previousState != "borrador" {
		return nil
	}
	var eligible bool
	err := tx.QueryRow(ctx, `SELECT ubicacion_confirmada_en IS NOT NULL OR EXISTS (
	 SELECT 1 FROM public.espacio_publicacion_historial_local
	 WHERE espacio_id=$1 AND estado_nuevo='activa')
	 FROM public.espacio WHERE id=$1`, id).Scan(&eligible)
	if err != nil {
		return mapError(err)
	}
	if !eligible {
		return spaces.ErrLocationRequired
	}
	return nil
}

func scanCommercialLocation(row pgx.Row) (spaces.CommercialLocation, error) {
	var item spaces.CommercialLocation
	err := row.Scan(&item.SpaceID, &item.Address, &item.RegionCode, &item.CommuneCode, &item.Latitude, &item.Longitude, &item.ConfirmedAt, &item.Revision, &item.UpdatedAt)
	item.PublicConfirmed = item.ConfirmedAt != nil
	return item, mapError(err)
}
func (r *Repository) GetOwnLocation(ctx context.Context, owner, id string) (spaces.CommercialLocation, error) {
	return scanCommercialLocation(r.pool.QueryRow(ctx, `SELECT `+commercialLocationColumns+` FROM public.espacio WHERE propietario_id=$1 AND id=$2 AND estado IN ('borrador','activa','oculta')`, owner, id))
}
func (r *Repository) SetOwnLocation(ctx context.Context, owner, id string, expected time.Time, in spaces.LocationInput) (spaces.CommercialLocation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return spaces.CommercialLocation{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockActiveOwner(ctx, tx, owner); err != nil {
		return spaces.CommercialLocation{}, err
	}
	old, err := scanCommercialLocation(tx.QueryRow(ctx, `SELECT `+commercialLocationColumns+` FROM public.espacio WHERE propietario_id=$1 AND id=$2 AND estado IN ('borrador','activa','oculta') FOR UPDATE`, owner, id))
	if err != nil {
		return spaces.CommercialLocation{}, err
	}
	if err = checkRevision(expected, old.UpdatedAt); err != nil {
		return spaces.CommercialLocation{}, err
	}
	// Synthetic demo pins must never become commercial coordinates implicitly.
	var fixture bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_ensayo_local_fixture WHERE espacio_id=$1 AND habilitada)`, id).Scan(&fixture); err != nil {
		return spaces.CommercialLocation{}, err
	}
	if fixture {
		return spaces.CommercialLocation{}, spaces.ErrEnabledFixture
	}
	_, err = tx.Exec(ctx, `UPDATE public.espacio SET direccion=$3,
 ubicacion_catalogo_version=CASE WHEN $4='' THEN NULL ELSE 'SUBDERE_DPA_2018' END,
 region_codigo=NULLIF($4,''),comuna_codigo=NULLIF($5,''),ubicacion_latitud=$6,ubicacion_longitud=$7,
 ubicacion_confirmada_en=CASE WHEN $8 THEN GREATEST(clock_timestamp(),COALESCE(ubicacion_confirmada_en,'-infinity'::timestamptz)+interval '1 microsecond') ELSE NULL END
 WHERE propietario_id=$1 AND id=$2`, owner, id, in.Address, in.RegionCode, in.CommuneCode, in.Latitude, in.Longitude, in.PublicConfirmed)
	if err != nil {
		return spaces.CommercialLocation{}, mapError(err)
	}
	if _, err = spaceversion.Bump(ctx, tx, id); err != nil {
		return spaces.CommercialLocation{}, err
	}
	item, err := scanCommercialLocation(tx.QueryRow(ctx, `SELECT `+commercialLocationColumns+` FROM public.espacio WHERE id=$1`, id))
	if err != nil {
		return spaces.CommercialLocation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return spaces.CommercialLocation{}, err
	}
	return item, nil
}
