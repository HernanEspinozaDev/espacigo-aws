package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

var _ identity.LocationTrustedRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetLocationPermission(ctx context.Context, accountID string) (*identity.LocationPermissionRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	const query = `
SELECT
    account_id::text,
    enabled,
    consent_updated_at,
    last_region_label
FROM public.cuenta_permiso_ubicacion
WHERE account_id = $1
`
	var (
		rec             identity.LocationPermissionRecord
		lastRegionLabel *string
	)
	err := dbtx.QueryRow(ctx, query, accountID).Scan(
		&rec.AccountID,
		&rec.Enabled,
		&rec.ConsentUpdatedAt,
		&lastRegionLabel,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}
	rec.ConsentUpdatedAt = rec.ConsentUpdatedAt.UTC()
	rec.LastRegionLabel = lastRegionLabel
	return &rec, nil
}

func (r *IdentityRepository) UpsertLocationPermission(ctx context.Context, record identity.LocationPermissionRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}
	const query = `
INSERT INTO public.cuenta_permiso_ubicacion (
    account_id,
    enabled,
    consent_updated_at,
    last_region_label
) VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    consent_updated_at = EXCLUDED.consent_updated_at,
    last_region_label = EXCLUDED.last_region_label
`
	_, err := dbtx.Exec(ctx, query,
		record.AccountID,
		record.Enabled,
		record.ConsentUpdatedAt.UTC(),
		record.LastRegionLabel,
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) ListActiveTrustedContacts(ctx context.Context, accountID string) ([]identity.TrustedContactRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	const query = `
SELECT
    id::text,
    account_id::text,
    full_name,
    email,
    phone_e164,
    relationship,
    status,
    created_at,
    updated_at
FROM public.cuenta_persona_confianza
WHERE account_id = $1 AND status = 'active'
ORDER BY created_at DESC
`
	rows, err := dbtx.Query(ctx, query, accountID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	var contacts []identity.TrustedContactRecord
	for rows.Next() {
		var (
			rec       identity.TrustedContactRecord
			phoneE164 *string
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.AccountID,
			&rec.FullName,
			&rec.Email,
			&phoneE164,
			&rec.Relationship,
			&rec.Status,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, mapError(err)
		}
		rec.CreatedAt = rec.CreatedAt.UTC()
		rec.UpdatedAt = rec.UpdatedAt.UTC()
		rec.PhoneE164 = phoneE164
		contacts = append(contacts, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	if contacts == nil {
		contacts = []identity.TrustedContactRecord{}
	}
	return contacts, nil
}

func (r *IdentityRepository) CountActiveTrustedContacts(ctx context.Context, accountID string) (int, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return 0, errors.New("repository uninitialized")
	}
	const query = `
SELECT COUNT(*)
FROM public.cuenta_persona_confianza
WHERE account_id = $1 AND status = 'active'
`
	var count int
	if err := dbtx.QueryRow(ctx, query, accountID).Scan(&count); err != nil {
		return 0, mapError(err)
	}
	return count, nil
}

func (r *IdentityRepository) GetTrustedContact(ctx context.Context, accountID, contactID string) (*identity.TrustedContactRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	const query = `
SELECT
    id::text,
    account_id::text,
    full_name,
    email,
    phone_e164,
    relationship,
    status,
    created_at,
    updated_at
FROM public.cuenta_persona_confianza
WHERE account_id = $1 AND id = $2
`
	var (
		rec       identity.TrustedContactRecord
		phoneE164 *string
	)
	err := dbtx.QueryRow(ctx, query, accountID, contactID).Scan(
		&rec.ID,
		&rec.AccountID,
		&rec.FullName,
		&rec.Email,
		&phoneE164,
		&rec.Relationship,
		&rec.Status,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	rec.UpdatedAt = rec.UpdatedAt.UTC()
	rec.PhoneE164 = phoneE164
	return &rec, nil
}

func (r *IdentityRepository) InsertTrustedContact(ctx context.Context, contact identity.TrustedContactRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}
	const query = `
INSERT INTO public.cuenta_persona_confianza (
    id,
    account_id,
    full_name,
    email,
    phone_e164,
    relationship,
    status,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`
	_, err := dbtx.Exec(ctx, query,
		contact.ID,
		contact.AccountID,
		contact.FullName,
		contact.Email,
		contact.PhoneE164,
		contact.Relationship,
		contact.Status,
		contact.CreatedAt.UTC(),
		contact.UpdatedAt.UTC(),
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) RevokeTrustedContact(ctx context.Context, accountID, contactID string, updatedAt time.Time) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}
	const query = `
UPDATE public.cuenta_persona_confianza
SET status = 'revoked', updated_at = $3
WHERE account_id = $1 AND id = $2 AND status = 'active'
`
	tag, err := dbtx.Exec(ctx, query, accountID, contactID, updatedAt.UTC())
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}
