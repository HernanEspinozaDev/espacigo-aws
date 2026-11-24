package rentalflowpg

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const CommercialAgreementPaymentHoldLifetime = 15 * time.Minute

type CommercialAgreementHoldInput struct {
	RequestID        string
	Quote            rentalflow.Quote
	LocationRevision int64
}
type CommercialAgreementHold struct {
	OccupancyID string
	ExpiresAt   time.Time
	Replayed    bool
}

// LockCommercialAgreementInterval is called after sorted active participant
// locks and before locking the request. All request mutations use this order,
// shared with checkout's quote -> space -> publication locks. It never commits.
func LockCommercialAgreementInterval(ctx context.Context, tx pgx.Tx, quoteID, spaceID string) error {
	if ctx == nil || tx == nil || !validUUIDText(quoteID) || !validUUIDText(spaceID) {
		return rentalflow.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('checkout-quote:' || $1, 0))`, quoteID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, spaceID); err != nil {
		return err
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM public.espacio WHERE id=$1 FOR UPDATE`, spaceID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rentalflow.ErrNotFound
		}
		return err
	}
	return nil
}

// RetainCommercialAgreementInterval must run in the same transaction as the
// second signature and ready_for_payment transition. Callers have already
// acquired participant, quote, space and request locks in that order. The quote
// is the authenticated immutable proposal snapshot, not browser supplied data;
// its original discovery expiry does not bound a separately signed request.
func RetainCommercialAgreementInterval(ctx context.Context, tx pgx.Tx, input CommercialAgreementHoldInput, now time.Time) (CommercialAgreementHold, error) {
	q := input.Quote
	if ctx == nil || tx == nil || now.IsZero() || !validUUIDText(input.RequestID) || !validUUIDText(q.ID) || !validUUIDText(q.SpaceID) || !validUUIDText(q.OwnerID) || !validUUIDText(q.RenterID) || q.OwnerID == q.RenterID || input.LocationRevision < 0 || q.Units < 1 || q.Units > math.MaxInt32 || q.UnitPriceCLP < 1 || q.Units > math.MaxInt64/q.UnitPriceCLP || q.SubtotalCLP != q.Units*q.UnitPriceCLP || q.Currency != "CLP" || !q.StartAt.After(now) || !q.EndAt.After(q.StartAt) {
		return CommercialAgreementHold{}, rentalflow.ErrInvalid
	}
	if err := LockCommercialAgreementInterval(ctx, tx, q.ID, q.SpaceID); err != nil {
		return CommercialAgreementHold{}, err
	}
	// Exactly one historical hold per request. Replays never extend its lease or
	// resurrect a released interval, even when the quote's original TTL elapsed.
	var prior CommercialAgreementHold
	var spaceID string
	var start, end time.Time
	var active bool
	err := tx.QueryRow(ctx, `SELECT id::text,espacio_id::text,lower(intervalo),upper(intervalo),activo,COALESCE(expira_en,'epoch'::timestamptz) FROM public.ocupacion WHERE solicitud_comercial_id=$1 FOR UPDATE`, input.RequestID).Scan(&prior.OccupancyID, &spaceID, &start, &end, &active, &prior.ExpiresAt)
	if err == nil {
		if !active || !prior.ExpiresAt.After(now) || spaceID != q.SpaceID || !start.Equal(q.StartAt) || !end.Equal(q.EndAt) {
			return CommercialAgreementHold{}, rentalflow.ErrConflict
		}
		prior.Replayed = true
		return prior, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CommercialAgreementHold{}, err
	}
	var current bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.espacio e JOIN LATERAL (SELECT version,modalidad,precio_base_clp,moneda FROM public.tarifa_espacio WHERE espacio_id=e.id ORDER BY version DESC LIMIT 1) t ON true JOIN public.usuario host ON host.id=e.propietario_id AND host.estado='activo' WHERE e.id=$1 AND e.propietario_id=$2 AND e.estado='activa' AND t.version=$3 AND t.modalidad=$4 AND t.precio_base_clp=$5 AND t.moneda=$6 AND e.zona_horaria=$7 AND e.ubicacion_revision=$8 AND e.ubicacion_confirmada_en IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=host.id))`, q.SpaceID, q.OwnerID, q.RateVersion, q.RateUnit, q.UnitPriceCLP, q.Currency, q.TimeZone, input.LocationRevision).Scan(&current)
	if err != nil {
		return CommercialAgreementHold{}, err
	}
	if !current {
		return CommercialAgreementHold{}, rentalflow.ErrConflict
	}
	zone, err := time.LoadLocation(q.TimeZone)
	if err != nil {
		return CommercialAgreementHold{}, rentalflow.ErrConflict
	}
	listing := rentalflow.Listing{ID: q.SpaceID, OwnerID: q.OwnerID, State: "activa", Title: q.SpaceTitle, RateUnit: q.RateUnit, UnitPriceCLP: q.UnitPriceCLP, Currency: q.Currency, RateVersion: q.RateVersion, TimeZone: q.TimeZone}
	var quoteIntervals []rentalflow.QuoteInterval
	ivRows, ivErr := tx.Query(ctx, `SELECT inicio, termino, unidades FROM public.cotizacion_arriendo_tramo WHERE cotizacion_id=$1 ORDER BY orden`, q.ID)
	if ivErr != nil {
		return CommercialAgreementHold{}, ivErr
	}
	for ivRows.Next() {
		var iv rentalflow.QuoteInterval
		if scanErr := ivRows.Scan(&iv.StartAt, &iv.EndAt, &iv.Units); scanErr != nil {
			ivRows.Close()
			return CommercialAgreementHold{}, scanErr
		}
		quoteIntervals = append(quoteIntervals, iv)
	}
	if ivRows.Err() != nil {
		ivRows.Close()
		return CommercialAgreementHold{}, ivRows.Err()
	}
	ivRows.Close()
	if len(quoteIntervals) == 0 {
		quoteIntervals = []rentalflow.QuoteInterval{{StartAt: q.StartAt, EndAt: q.EndAt, Units: q.Units}}
	}
	// Reuse checkout's complete candidate authority, including DST-safe weekly
	// periods, legacy calendar overrides, active occupancies and reconciliation.
	repo := &Repository{db: tx, now: func() time.Time { return now }}
	for _, iv := range quoteIntervals {
		intervals, err := repo.PublicIntervals(ctx, listing, iv.StartAt.In(zone).Format("2006-01-02"), int(iv.Units))
		if err != nil {
			return CommercialAgreementHold{}, err
		}
		available := false
		for _, candidate := range intervals {
			if candidate.StartAt.Equal(iv.StartAt) && candidate.EndAt.Equal(iv.EndAt) {
				available = true
				break
			}
		}
		if !available {
			return CommercialAgreementHold{}, rentalflow.ErrConflict
		}
	}
	expires := now.Add(CommercialAgreementPaymentHoldLifetime).UTC()
	if q.StartAt.Before(expires) {
		expires = q.StartAt.UTC()
	}
	hold := CommercialAgreementHold{OccupancyID: uuid.NewString(), ExpiresAt: expires}
	_, err = tx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,solicitud_comercial_id,intervalo,tipo,activo,expira_en) VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'retencion_contrato_comercial',true,$6)`, hold.OccupancyID, q.SpaceID, input.RequestID, q.StartAt.UTC(), q.EndAt.UTC(), expires)
	if err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && (p.Code == "23P01" || p.Code == "23505") {
			return CommercialAgreementHold{}, rentalflow.ErrConflict
		}
		return CommercialAgreementHold{}, err
	}
	return hold, nil
}

// ExpireCommercialAgreementInterval releases a due hold without deleting its
// history. The caller owns the common locks and commits the request's expired
// transition and audit event in this same transaction. Future payment holds
// are outside this helper's scope.
func ExpireCommercialAgreementInterval(ctx context.Context, tx pgx.Tx, requestID, spaceID string, now time.Time) error {
	if ctx == nil || tx == nil || !validUUIDText(requestID) || !validUUIDText(spaceID) || now.IsZero() {
		return rentalflow.ErrInvalid
	}
	_, err := tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$3 WHERE solicitud_comercial_id=$1 AND espacio_id=$2 AND tipo='retencion_contrato_comercial' AND activo AND expira_en<=$3`, requestID, spaceID, now.UTC())
	return err
}
