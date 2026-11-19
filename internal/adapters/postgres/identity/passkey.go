package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ identity.PasskeyRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) ListPasskeys(ctx context.Context, accountID string) ([]identity.PasskeyRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    id::text,
    account_id::text,
    credential_id,
    public_key,
    algorithm,
    sign_count,
    label,
    transports,
    last_used_at,
    created_at
FROM public.cuenta_passkey
WHERE account_id = $1::uuid
ORDER BY created_at DESC
`
	rows, err := dbtx.Query(ctx, query, accountID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	var results []identity.PasskeyRecord
	for rows.Next() {
		var (
			rec        identity.PasskeyRecord
			lastUsedAt *time.Time
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.AccountID,
			&rec.CredentialID,
			&rec.PublicKey,
			&rec.Algorithm,
			&rec.SignCount,
			&rec.Label,
			&rec.Transports,
			&lastUsedAt,
			&rec.CreatedAt,
		); err != nil {
			return nil, mapError(err)
		}
		if lastUsedAt != nil {
			t := lastUsedAt.UTC()
			rec.LastUsedAt = &t
		}
		rec.CreatedAt = rec.CreatedAt.UTC()
		if rec.Transports == nil {
			rec.Transports = []string{}
		}
		results = append(results, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return results, nil
}

func (r *IdentityRepository) SaveWebAuthnChallenge(ctx context.Context, challenge identity.WebAuthnChallengeRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
INSERT INTO public.cuenta_webauthn_challenge (
    id, account_id, challenge, purpose, expires_at, created_at
) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
`
	_, err := dbtx.Exec(
		ctx,
		query,
		challenge.ID,
		challenge.AccountID,
		challenge.Challenge,
		challenge.Purpose,
		challenge.ExpiresAt.UTC(),
		challenge.CreatedAt.UTC(),
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*identity.WebAuthnChallengeRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}

	const query = `
SELECT
    id::text,
    account_id::text,
    challenge,
    purpose,
    expires_at,
    consumed_at,
    created_at
FROM public.cuenta_webauthn_challenge
WHERE id = $1::uuid
`
	var (
		rec        identity.WebAuthnChallengeRecord
		consumedAt *time.Time
	)
	err := dbtx.QueryRow(ctx, query, challengeID).Scan(
		&rec.ID,
		&rec.AccountID,
		&rec.Challenge,
		&rec.Purpose,
		&rec.ExpiresAt,
		&consumedAt,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, identity.ErrNotFound
		}
		return nil, mapError(err)
	}
	rec.ExpiresAt = rec.ExpiresAt.UTC()
	rec.CreatedAt = rec.CreatedAt.UTC()
	if consumedAt != nil {
		t := consumedAt.UTC()
		rec.ConsumedAt = &t
	}
	return &rec, nil
}

func (r *IdentityRepository) SavePasskeyAndConsumeChallenge(
	ctx context.Context,
	passkey identity.PasskeyRecord,
	challengeID string,
	consumedAt time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	const consumeQuery = `
UPDATE public.cuenta_webauthn_challenge
SET consumed_at = $2
WHERE id = $1::uuid AND consumed_at IS NULL
`
	cmd, err := tx.Exec(ctx, consumeQuery, challengeID, consumedAt.UTC())
	if err != nil {
		return mapError(err)
	}
	if cmd.RowsAffected() == 0 {
		var isConsumed bool
		errCheck := tx.QueryRow(ctx, `SELECT consumed_at IS NOT NULL FROM public.cuenta_webauthn_challenge WHERE id = $1::uuid`, challengeID).Scan(&isConsumed)
		if errCheck != nil {
			return identity.ErrPasskeyChallengeInvalid
		}
		if isConsumed {
			return identity.ErrPasskeyChallengeConsumed
		}
		return identity.ErrPasskeyChallengeInvalid
	}

	const insertQuery = `
INSERT INTO public.cuenta_passkey (
    id, account_id, credential_id, public_key, algorithm, sign_count, label, transports, created_at
) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)
`
	_, err = tx.Exec(
		ctx,
		insertQuery,
		passkey.ID,
		passkey.AccountID,
		passkey.CredentialID,
		passkey.PublicKey,
		passkey.Algorithm,
		passkey.SignCount,
		passkey.Label,
		passkey.Transports,
		passkey.CreatedAt.UTC(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return identity.ErrPasskeyCredentialConflict
		}
		return mapError(err)
	}

	return tx.Commit(ctx)
}

func (r *IdentityRepository) DeletePasskey(ctx context.Context, accountID, passkeyID string) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}

	const query = `
DELETE FROM public.cuenta_passkey
WHERE id = $1::uuid AND account_id = $2::uuid
`
	cmd, err := dbtx.Exec(ctx, query, passkeyID, accountID)
	if err != nil {
		return mapError(err)
	}
	if cmd.RowsAffected() == 0 {
		return identity.ErrPasskeyNotFound
	}
	return nil
}

func (r *IdentityRepository) GetAccountDisplayName(ctx context.Context, accountID string) (string, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return "", errors.New("repository uninitialized")
	}

	const query = `
SELECT nombre_visible
FROM public.perfil_usuario
WHERE usuario_id = $1::uuid
`
	var name string
	err := dbtx.QueryRow(ctx, query, accountID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", mapError(err)
	}
	return name, nil
}
