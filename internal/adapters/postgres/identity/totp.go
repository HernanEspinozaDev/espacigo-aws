package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

var _ identity.TOTPRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetTOTP(ctx context.Context, accountID string) (*identity.AccountTOTPRecord, int, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, 0, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    account_id::text,
    secret_ciphertext,
    key_version,
    pending_secret_ciphertext,
    pending_key_version,
    enabled_at,
    last_verified_step,
    created_at,
    updated_at
FROM public.cuenta_totp
WHERE account_id = $1
`
	var (
		rec               identity.AccountTOTPRecord
		keyVersion        *string
		pendingKeyVersion *string
		enabledAt         *time.Time
		lastVerifiedStep  *int64
	)
	err := dbtx.QueryRow(ctx, query, accountID).Scan(
		&rec.AccountID,
		&rec.SecretCiphertext,
		&keyVersion,
		&rec.PendingSecretCiphertext,
		&pendingKeyVersion,
		&enabledAt,
		&lastVerifiedStep,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, mapError(err)
		}
		return nil, 0, nil
	}

	rec.KeyVersion = keyVersion
	rec.PendingKeyVersion = pendingKeyVersion
	if enabledAt != nil {
		t := enabledAt.UTC()
		rec.EnabledAt = &t
	}
	rec.LastVerifiedStep = lastVerifiedStep
	rec.CreatedAt = rec.CreatedAt.UTC()
	rec.UpdatedAt = rec.UpdatedAt.UTC()

	var remainingCodes int
	const countQuery = `
SELECT COUNT(*)
FROM public.cuenta_totp_backup_code
WHERE account_id = $1 AND used_at IS NULL
`
	if err := dbtx.QueryRow(ctx, countQuery, accountID).Scan(&remainingCodes); err != nil {
		return nil, 0, mapError(err)
	}

	return &rec, remainingCodes, nil
}

func (r *IdentityRepository) SavePendingTOTPSecret(ctx context.Context, accountID string, ciphertext []byte, keyVersion string) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
INSERT INTO public.cuenta_totp (
    account_id,
    pending_secret_ciphertext,
    pending_key_version,
    updated_at
) VALUES ($1, $2, $3, now())
ON CONFLICT (account_id) DO UPDATE SET
    pending_secret_ciphertext = EXCLUDED.pending_secret_ciphertext,
    pending_key_version = EXCLUDED.pending_key_version,
    updated_at = now()
`
	_, err := dbtx.Exec(ctx, query, accountID, ciphertext, keyVersion)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) ActivateTOTP(ctx context.Context, accountID string, verifiedStep int64, backupCodeHashes []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	const updateQuery = `
UPDATE public.cuenta_totp
SET secret_ciphertext = pending_secret_ciphertext,
    key_version = pending_key_version,
    pending_secret_ciphertext = NULL,
    pending_key_version = NULL,
    enabled_at = now(),
    last_verified_step = $2,
    updated_at = now()
WHERE account_id = $1 AND pending_secret_ciphertext IS NOT NULL
`
	cmd, err := tx.Exec(ctx, updateQuery, accountID, verifiedStep)
	if err != nil {
		return mapError(err)
	}
	if cmd.RowsAffected() == 0 {
		return identity.ErrTOTPNotPending
	}

	const deleteBackupQuery = `
DELETE FROM public.cuenta_totp_backup_code
WHERE account_id = $1
`
	if _, err := tx.Exec(ctx, deleteBackupQuery, accountID); err != nil {
		return mapError(err)
	}

	const insertBackupQuery = `
INSERT INTO public.cuenta_totp_backup_code (
    id, account_id, code_hash, created_at
) VALUES ($1, $2, $3, now())
`
	for _, hash := range backupCodeHashes {
		id, err := (credentials.Generator{}).ID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, insertBackupQuery, id, accountID, hash); err != nil {
			return mapError(err)
		}
	}

	return tx.Commit(ctx)
}

func (r *IdentityRepository) UpdateTOTPLastVerifiedStep(ctx context.Context, accountID string, verifiedStep int64) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
UPDATE public.cuenta_totp
SET last_verified_step = $2,
    updated_at = now()
WHERE account_id = $1
`
	_, err := dbtx.Exec(ctx, query, accountID, verifiedStep)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) ConsumeBackupCode(ctx context.Context, accountID string, codeHash string) (bool, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return false, errors.New("repository uninitialized")
	}

	const query = `
UPDATE public.cuenta_totp_backup_code
SET used_at = now()
WHERE id = (
    SELECT id
    FROM public.cuenta_totp_backup_code
    WHERE account_id = $1 AND code_hash = $2 AND used_at IS NULL
    LIMIT 1
    FOR UPDATE
)
RETURNING id
`
	var id string
	err := dbtx.QueryRow(ctx, query, accountID, codeHash).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, mapError(err)
	}
	return true, nil
}

func (r *IdentityRepository) DisableTOTP(ctx context.Context, accountID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	const updateQuery = `
UPDATE public.cuenta_totp
SET secret_ciphertext = NULL,
    key_version = NULL,
    pending_secret_ciphertext = NULL,
    pending_key_version = NULL,
    enabled_at = NULL,
    last_verified_step = NULL,
    updated_at = now()
WHERE account_id = $1
`
	if _, err := tx.Exec(ctx, updateQuery, accountID); err != nil {
		return mapError(err)
	}

	const deleteBackupQuery = `
DELETE FROM public.cuenta_totp_backup_code
WHERE account_id = $1
`
	if _, err := tx.Exec(ctx, deleteBackupQuery, accountID); err != nil {
		return mapError(err)
	}

	return tx.Commit(ctx)
}
