package rentalflowpg

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/pricing"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EarlyRentalOfferSnapshot struct {
	HostID string
	Input  earlyrental.OfferSnapshotInput
}

// ResolveEarlyRentalOfferSnapshot reads authoritative public-listing,
// participant, calendar and occupancy facts inside the caller's write
// transaction. Keep that transaction open through offer and ocupacion writes.
func (r *Repository) ResolveEarlyRentalOfferSnapshot(ctx context.Context, tx pgx.Tx, renterID, spaceID string, interval rentalflow.Interval) (EarlyRentalOfferSnapshot, error) {
	if r == nil || ctx == nil || tx == nil || r.now == nil || interval.StartAt.IsZero() || interval.EndAt.IsZero() || !interval.EndAt.After(interval.StartAt) {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrInvalid
	}
	renterUUID, err := uuid.Parse(renterID)
	if err != nil || renterUUID.String() != strings.ToLower(renterID) {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrInvalid
	}
	spaceUUID, err := uuid.Parse(spaceID)
	if err != nil || spaceUUID.String() != strings.ToLower(spaceID) {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrInvalid
	}

	txRepo, err := NewRepository(tx, nil, r.now)
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	listing, err := txRepo.PublicListing(ctx, spaceUUID.String())
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	ownerUUID, err := uuid.Parse(listing.OwnerID)
	if err != nil || ownerUUID == renterUUID {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrNotFound
	}
	if err := accountlock.LockAccountIDs(ctx, tx, renterUUID.String(), ownerUUID.String()); err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	for _, participantID := range []string{renterUUID.String(), ownerUUID.String()} {
		var accountState string
		var administrativelyBlocked bool
		err := tx.QueryRow(ctx, `SELECT u.estado, EXISTS(
			SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id
		) FROM public.usuario u WHERE u.id=$1`, participantID).Scan(&accountState, &administrativelyBlocked)
		if errors.Is(err, pgx.ErrNoRows) {
			return EarlyRentalOfferSnapshot{}, rentalflow.ErrNotFound
		}
		if err != nil {
			return EarlyRentalOfferSnapshot{}, err
		}
		if accountState != "activo" || administrativelyBlocked {
			return EarlyRentalOfferSnapshot{}, rentalflow.ErrNotFound
		}
	}

	var lockedSpaceID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM public.espacio WHERE id=$1 AND estado='activa' FOR SHARE`, spaceUUID.String()).Scan(&lockedSpaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrNotFound
	}
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	currentListing, err := txRepo.PublicListing(ctx, spaceUUID.String())
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	if currentListing != listing {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrConflict
	}
	if listing.RateUnit != earlyrental.RateUnitHour || listing.Currency != earlyrental.CurrencyCLP || listing.UnitPriceCLP < 5_001 {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrUnavailable
	}
	loc, err := time.LoadLocation(listing.TimeZone)
	if err != nil {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrUnavailable
	}
	units, err := pricing.BilledUnits(listing.RateUnit, interval.StartAt.UTC(), interval.EndAt.UTC(), listing.TimeZone)
	if err != nil || (units != 1 && units != 2 && units != 4) || listing.UnitPriceCLP > math.MaxInt64/units {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrInvalid
	}

	dateText := interval.StartAt.In(loc).Format("2006-01-02")
	intervals, err := txRepo.PublicIntervals(ctx, listing, dateText, int(units))
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	available := false
	for _, candidate := range intervals {
		if candidate.StartAt.Equal(interval.StartAt) && candidate.EndAt.Equal(interval.EndAt) {
			available = true
			break
		}
	}
	if !available {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrConflict
	}

	firstDate, lastDate, err := localIntervalDateRange(interval, loc)
	if err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	var hasCustomTariff bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM public.espacio_calendario_override
		WHERE espacio_id=$1 AND fecha >= $2::date AND fecha <= $3::date
		  AND tarifa_personalizada_clp IS NOT NULL
	)`, spaceUUID.String(), firstDate, lastDate).Scan(&hasCustomTariff); err != nil {
		return EarlyRentalOfferSnapshot{}, err
	}
	if hasCustomTariff {
		return EarlyRentalOfferSnapshot{}, rentalflow.ErrConflict
	}

	return EarlyRentalOfferSnapshot{
		HostID: ownerUUID.String(),
		Input: earlyrental.OfferSnapshotInput{
			SpaceID: spaceUUID, SpaceTitle: listing.Title, RateUnit: listing.RateUnit,
			UnitPriceCLP: listing.UnitPriceCLP, Units: units,
			StartAt: interval.StartAt.UTC(), EndAt: interval.EndAt.UTC(), TimeZone: listing.TimeZone,
			CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent,
		},
	}, nil
}

func localIntervalDateRange(interval rentalflow.Interval, loc *time.Location) (string, string, error) {
	if loc == nil || interval.StartAt.IsZero() || interval.EndAt.IsZero() || !interval.EndAt.After(interval.StartAt) {
		return "", "", rentalflow.ErrInvalid
	}
	lastCoveredInstant := interval.EndAt.Add(-time.Nanosecond)
	if !lastCoveredInstant.Before(interval.EndAt) {
		return "", "", rentalflow.ErrInvalid
	}
	return interval.StartAt.In(loc).Format("2006-01-02"), lastCoveredInstant.In(loc).Format("2006-01-02"), nil
}
