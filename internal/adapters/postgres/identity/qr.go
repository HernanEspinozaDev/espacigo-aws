package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

var _ identity.QRRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetQRDevice(ctx context.Context, accountID string) (*identity.AccountQRDeviceRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    account_id::text,
    enabled_at,
    trusted_device_label,
    updated_at
FROM public.cuenta_qr_dispositivo
WHERE account_id = $1::uuid
`
	var (
		rec                identity.AccountQRDeviceRecord
		enabledAt          *time.Time
		trustedDeviceLabel *string
	)
	err := dbtx.QueryRow(ctx, query, accountID).Scan(
		&rec.AccountID,
		&enabledAt,
		&trustedDeviceLabel,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}

	rec.UpdatedAt = rec.UpdatedAt.UTC()
	if enabledAt != nil {
		t := enabledAt.UTC()
		rec.EnabledAt = &t
	}
	rec.TrustedDeviceLabel = trustedDeviceLabel
	return &rec, nil
}

func (r *IdentityRepository) UpsertQRDevice(ctx context.Context, accountID string, enabledAt time.Time, trustedDeviceLabel string) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
INSERT INTO public.cuenta_qr_dispositivo (
    account_id, enabled_at, trusted_device_label, updated_at
) VALUES ($1::uuid, $2, $3, $2)
ON CONFLICT (account_id) DO UPDATE SET
    enabled_at = EXCLUDED.enabled_at,
    trusted_device_label = EXCLUDED.trusted_device_label,
    updated_at = EXCLUDED.updated_at
`
	var label *string
	if trustedDeviceLabel != "" {
		label = &trustedDeviceLabel
	}
	_, err := dbtx.Exec(ctx, query, accountID, enabledAt.UTC(), label)
	return mapError(err)
}

func (r *IdentityRepository) DisableQRDevice(ctx context.Context, accountID string, updatedAt time.Time) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
UPDATE public.cuenta_qr_dispositivo
SET enabled_at = NULL, updated_at = $2
WHERE account_id = $1::uuid
`
	_, err := dbtx.Exec(ctx, query, accountID, updatedAt.UTC())
	return mapError(err)
}

func (r *IdentityRepository) SaveQRTicket(ctx context.Context, ticket identity.AccountQRTicketRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
INSERT INTO public.cuenta_qr_ticket (
    id, account_id, token_hash, status, device_label, ip_masked, expires_at, created_at
) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8)
`
	_, err := dbtx.Exec(
		ctx,
		query,
		ticket.ID,
		ticket.AccountID,
		ticket.TokenHash,
		ticket.Status,
		ticket.DeviceLabel,
		ticket.IPMasked,
		ticket.ExpiresAt.UTC(),
		ticket.CreatedAt.UTC(),
	)
	return mapError(err)
}

func (r *IdentityRepository) GetQRTicketByID(ctx context.Context, ticketID string) (*identity.AccountQRTicketRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    id::text,
    account_id::text,
    token_hash,
    status,
    COALESCE(device_label, ''),
    COALESCE(ip_masked, ''),
    expires_at,
    resolved_at,
    created_at
FROM public.cuenta_qr_ticket
WHERE id = $1::uuid
`
	var (
		rec        identity.AccountQRTicketRecord
		resolvedAt *time.Time
	)
	err := dbtx.QueryRow(ctx, query, ticketID).Scan(
		&rec.ID,
		&rec.AccountID,
		&rec.TokenHash,
		&rec.Status,
		&rec.DeviceLabel,
		&rec.IPMasked,
		&rec.ExpiresAt,
		&resolvedAt,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}

	rec.ExpiresAt = rec.ExpiresAt.UTC()
	rec.CreatedAt = rec.CreatedAt.UTC()
	if resolvedAt != nil {
		t := resolvedAt.UTC()
		rec.ResolvedAt = &t
	}
	return &rec, nil
}

func (r *IdentityRepository) GetQRTicketByTokenHash(ctx context.Context, tokenHash string) (*identity.AccountQRTicketRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    id::text,
    account_id::text,
    token_hash,
    status,
    COALESCE(device_label, ''),
    COALESCE(ip_masked, ''),
    expires_at,
    resolved_at,
    created_at
FROM public.cuenta_qr_ticket
WHERE token_hash = $1
`
	var (
		rec        identity.AccountQRTicketRecord
		resolvedAt *time.Time
	)
	err := dbtx.QueryRow(ctx, query, tokenHash).Scan(
		&rec.ID,
		&rec.AccountID,
		&rec.TokenHash,
		&rec.Status,
		&rec.DeviceLabel,
		&rec.IPMasked,
		&rec.ExpiresAt,
		&resolvedAt,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}

	rec.ExpiresAt = rec.ExpiresAt.UTC()
	rec.CreatedAt = rec.CreatedAt.UTC()
	if resolvedAt != nil {
		t := resolvedAt.UTC()
		rec.ResolvedAt = &t
	}
	return &rec, nil
}

func (r *IdentityRepository) ApproveQRTicket(ctx context.Context, accountID, ticketID, deviceLabel string, resolvedAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	const updateTicketQuery = `
UPDATE public.cuenta_qr_ticket
SET status = 'approved', resolved_at = $1
WHERE id = $2::uuid AND account_id = $3::uuid AND status = 'pending' AND expires_at > $1
`
	cmd, err := tx.Exec(ctx, updateTicketQuery, resolvedAt.UTC(), ticketID, accountID)
	if err != nil {
		return mapError(err)
	}
	if cmd.RowsAffected() == 0 {
		return identity.ErrQRTicketResolved
	}

	const upsertDeviceQuery = `
INSERT INTO public.cuenta_qr_dispositivo (
    account_id, enabled_at, trusted_device_label, updated_at
) VALUES ($1::uuid, $2, $3, $2)
ON CONFLICT (account_id) DO UPDATE SET
    enabled_at = EXCLUDED.enabled_at,
    trusted_device_label = EXCLUDED.trusted_device_label,
    updated_at = EXCLUDED.updated_at
`
	var label *string
	if deviceLabel != "" {
		label = &deviceLabel
	}
	if _, err := tx.Exec(ctx, upsertDeviceQuery, accountID, resolvedAt.UTC(), label); err != nil {
		return mapError(err)
	}

	return mapError(tx.Commit(ctx))
}

func (r *IdentityRepository) ExpireQRTicket(ctx context.Context, ticketID string) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
UPDATE public.cuenta_qr_ticket
SET status = 'expired'
WHERE id = $1::uuid AND status = 'pending'
`
	_, err := dbtx.Exec(ctx, query, ticketID)
	return mapError(err)
}
