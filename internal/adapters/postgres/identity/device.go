package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

var _ identity.DeviceRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) ListActiveSessions(ctx context.Context, accountID string, now time.Time) ([]identity.Session, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	const query = `
SELECT
    id::text,
    usuario_id::text,
    token_hash,
    creada_en,
    ultima_actividad_en,
    expira_en,
    revocada_en,
    COALESCE(cliente_resumen, ''),
    COALESCE(device_label, ''),
    COALESCE(ip_masked, ''),
    COALESCE(user_agent, '')
FROM public.sesion
WHERE usuario_id = $1
  AND revocada_en IS NULL
  AND expira_en > $2
ORDER BY ultima_actividad_en DESC
`
	rows, err := dbtx.Query(ctx, query, accountID, now)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	var sessions []identity.Session
	for rows.Next() {
		var s identity.Session
		var revokedAt *time.Time
		if err := rows.Scan(
			&s.ID,
			&s.AccountID,
			&s.TokenHash,
			&s.CreatedAt,
			&s.LastActivityAt,
			&s.ExpiresAt,
			&revokedAt,
			&s.ClientSummary,
			&s.DeviceLabel,
			&s.IPMasked,
			&s.UserAgent,
		); err != nil {
			return nil, mapError(err)
		}
		s.RevokedAt = revokedAt
		s.CreatedAt = s.CreatedAt.UTC()
		s.LastActivityAt = s.LastActivityAt.UTC()
		s.ExpiresAt = s.ExpiresAt.UTC()
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return sessions, nil
}

func (r *IdentityRepository) RevokeSessionForAccount(ctx context.Context, accountID string, sessionID string, revokedAt time.Time) (bool, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return false, errors.New("repository uninitialized")
	}
	const query = `
UPDATE public.sesion
SET revocada_en = $1
WHERE id = $2
  AND usuario_id = $3
  AND revocada_en IS NULL
`
	tag, err := dbtx.Exec(ctx, query, revokedAt, sessionID, accountID)
	if err != nil {
		return false, mapError(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *IdentityRepository) RevokeOtherSessionsForAccount(ctx context.Context, accountID string, keepSessionID string, revokedAt time.Time) (int64, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return 0, errors.New("repository uninitialized")
	}
	const query = `
UPDATE public.sesion
SET revocada_en = $1
WHERE usuario_id = $2
  AND id != $3
  AND revocada_en IS NULL
  AND expira_en > $1
`
	tag, err := dbtx.Exec(ctx, query, revokedAt, accountID, keepSessionID)
	if err != nil {
		return 0, mapError(err)
	}
	return tag.RowsAffected(), nil
}
