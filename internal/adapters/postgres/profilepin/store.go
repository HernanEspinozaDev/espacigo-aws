package profilepinpg

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ profilepin.Store = (*Store)(nil)

// Store persists PIN verifiers, throttles, grants, and audit events.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Credential(ctx context.Context, accountID string) (profilepin.Credential, bool, error) {
	var c profilepin.Credential
	err := s.pool.QueryRow(ctx, `SELECT verifier_salt,verifier_hash FROM public.account_pinpass WHERE account_id=$1`, accountID).Scan(&c.Salt, &c.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilepin.Credential{}, false, nil
	}
	return c, err == nil, err
}

func (s *Store) SetCredentialAndRevoke(ctx context.Context, accountID string, c profilepin.Credential, sessionID string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if !active {
		return profilepin.ErrNoAccess
	}
	if sessionOK, err := sessionIsActive(ctx, tx, accountID, sessionID, at); err != nil {
		return err
	} else if !sessionOK {
		return profilepin.ErrNoAccess
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.account_pinpass(account_id,verifier_salt,verifier_hash,created_at,updated_at)
VALUES($1,$2,$3,$4,$4) ON CONFLICT(account_id) DO UPDATE SET verifier_salt=EXCLUDED.verifier_salt,verifier_hash=EXCLUDED.verifier_hash,updated_at=EXCLUDED.updated_at`, accountID, c.Salt, c.Hash, at)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM public.account_pinpass_access WHERE account_id=$1`, accountID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, accountID, sessionID, "pinpass.changed", at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteCredentialAndRevoke(ctx context.Context, accountID, sessionID string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if !active {
		return profilepin.ErrNoAccess
	}
	if sessionOK, err := sessionIsActive(ctx, tx, accountID, sessionID, at); err != nil {
		return err
	} else if !sessionOK {
		return profilepin.ErrNoAccess
	}
	if _, err = tx.Exec(ctx, `DELETE FROM public.account_pinpass WHERE account_id=$1`, accountID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM public.account_pinpass_access WHERE account_id=$1`, accountID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, accountID, sessionID, "pinpass.reset", at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailureState(ctx context.Context, accountID, sessionID string) (profilepin.FailureState, error) {
	var result profilepin.FailureState
	var blocked *time.Time
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(failed_attempts),0),max(blocked_until)
FROM public.account_pinpass_attempt WHERE account_id=$1 AND (session_id IS NULL OR session_id=$2::uuid)`, accountID, sessionID).Scan(&result.Attempts, &blocked)
	if blocked != nil {
		result.BlockedUntil = blocked.UTC()
	}
	return result, err
}

func (s *Store) RecordFailureAndAudit(ctx context.Context, accountID, sessionID string, at time.Time) (profilepin.FailureState, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return profilepin.FailureState{}, err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return profilepin.FailureState{}, err
	}
	if !active {
		return profilepin.FailureState{}, profilepin.ErrNoAccess
	}
	for _, scope := range []any{nil, sessionID} {
		if _, err = tx.Exec(ctx, `INSERT INTO public.account_pinpass_attempt(account_id,session_id,failed_attempts,updated_at)
VALUES($1,$2::uuid,0,$3) ON CONFLICT (account_id,session_id) DO NOTHING`, accountID, scope, at); err != nil {
			return profilepin.FailureState{}, err
		}
		var attempts int
		var blocked *time.Time
		if err = tx.QueryRow(ctx, `SELECT failed_attempts,blocked_until FROM public.account_pinpass_attempt
WHERE account_id=$1 AND session_id IS NOT DISTINCT FROM $2::uuid FOR UPDATE`, accountID, scope).Scan(&attempts, &blocked); err != nil {
			return profilepin.FailureState{}, err
		}
		attempts++
		var nextBlocked any
		if pause := retryPause(attempts); pause > 0 {
			nextBlocked = at.Add(pause)
		} else {
			nextBlocked = blocked
		}
		if _, err = tx.Exec(ctx, `UPDATE public.account_pinpass_attempt SET failed_attempts=$3,blocked_until=$4,updated_at=$5
WHERE account_id=$1 AND session_id IS NOT DISTINCT FROM $2::uuid`, accountID, scope, attempts, nextBlocked, at); err != nil {
			return profilepin.FailureState{}, err
		}
	}
	if err = insertAudit(ctx, tx, accountID, sessionID, "pinpass.unlock_denied", at); err != nil {
		return profilepin.FailureState{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return profilepin.FailureState{}, err
	}
	return s.FailureState(ctx, accountID, sessionID)
}

func retryPause(attempt int) time.Duration {
	if attempt < 3 {
		return 0
	}
	shift := attempt - 3
	if shift > 6 {
		return 15 * time.Minute
	}
	seconds := int64(15)
	for i := 0; i < shift; i++ {
		seconds *= 2
		if seconds >= 900 {
			seconds = 900
			break
		}
	}
	return time.Duration(seconds) * time.Second
}

func (s *Store) ClearFailures(ctx context.Context, accountID, sessionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if !active {
		return profilepin.ErrNoAccess
	}
	if _, err = tx.Exec(ctx, `DELETE FROM public.account_pinpass_attempt WHERE account_id=$1 AND (session_id IS NULL OR session_id=$2::uuid)`, accountID, sessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GrantAndAudit(ctx context.Context, accountID, sessionID string, expected profilepin.Credential, expiresAt, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if !active {
		return profilepin.ErrNoAccess
	}
	sessionOK, err := sessionIsActive(ctx, tx, accountID, sessionID, at)
	if err != nil {
		return err
	}
	if !sessionOK {
		return profilepin.ErrNoAccess
	}
	var salt, hash []byte
	err = tx.QueryRow(ctx, `SELECT verifier_salt,verifier_hash FROM public.account_pinpass WHERE account_id=$1 FOR UPDATE`, accountID).Scan(&salt, &hash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (subtle.ConstantTimeCompare(salt, expected.Salt) != 1 || subtle.ConstantTimeCompare(hash, expected.Hash) != 1) {
		return profilepin.ErrInvalidPIN
	}
	if err != nil {
		return err
	}
	var blocked *time.Time
	err = tx.QueryRow(ctx, `SELECT max(blocked_until) FROM public.account_pinpass_attempt
WHERE account_id=$1 AND (session_id IS NULL OR session_id=$2::uuid)`, accountID, sessionID).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked != nil && at.Before(*blocked) {
		return &profilepin.LockedError{Until: blocked.UTC()}
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.account_pinpass_access(account_id,session_id,expires_at,created_at)
VALUES($1,$2,$3,$4) ON CONFLICT(account_id,session_id) DO UPDATE SET expires_at=EXCLUDED.expires_at,created_at=EXCLUDED.created_at`, accountID, sessionID, expiresAt, at)
	if err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, accountID, sessionID, "pinpass.unlocked", at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ActiveGrant(ctx context.Context, accountID, sessionID string, now time.Time) (profilepin.Access, bool, error) {
	var grant profilepin.Access
	err := s.pool.QueryRow(ctx, `SELECT expires_at FROM public.account_pinpass_access WHERE account_id=$1 AND session_id=$2::uuid AND expires_at>$3`, accountID, sessionID, now).Scan(&grant.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilepin.Access{}, false, nil
	}
	return grant, err == nil, err
}

func (s *Store) WithActiveGrant(ctx context.Context, accountID, sessionID string, now time.Time, action string, reveal func(context.Context) error) (profilepin.Access, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return profilepin.Access{}, err
	}
	defer tx.Rollback(ctx)
	active, err := accountlock.LockActive(ctx, tx, accountID)
	if err != nil {
		return profilepin.Access{}, err
	}
	if !active {
		return profilepin.Access{}, profilepin.ErrNoAccess
	}
	sessionOK, err := sessionIsActive(ctx, tx, accountID, sessionID, now)
	if err != nil {
		return profilepin.Access{}, err
	}
	if !sessionOK {
		return profilepin.Access{}, profilepin.ErrNoAccess
	}
	var grant profilepin.Access
	err = tx.QueryRow(ctx, `SELECT expires_at FROM public.account_pinpass_access WHERE account_id=$1 AND session_id=$2::uuid AND expires_at>$3 FOR UPDATE`, accountID, sessionID, now).Scan(&grant.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilepin.Access{}, profilepin.ErrNoAccess
	}
	if err != nil {
		return profilepin.Access{}, err
	}
	if err = reveal(profilepin.WithSensitiveQueryer(ctx, tx)); err != nil {
		return profilepin.Access{}, err
	}
	if err = insertAudit(ctx, tx, accountID, sessionID, action, now); err != nil {
		return profilepin.Access{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return profilepin.Access{}, err
	}
	grant.ExpiresAt = grant.ExpiresAt.UTC()
	return grant, nil
}

func sessionIsActive(ctx context.Context, tx pgx.Tx, accountID, sessionID string, at time.Time) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.sesion WHERE usuario_id=$1 AND id=$2::uuid AND revocada_en IS NULL
AND expira_en>$3 AND ultima_actividad_en + interval '30 minutes' > $3)`, accountID, sessionID, at).Scan(&active)
	return active, err
}

func insertAudit(ctx context.Context, tx pgx.Tx, accountID, sessionID, action string, at time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.account_pinpass_audit(account_id,session_id,action,occurred_at) VALUES($1,$2::uuid,$3,$4)`, accountID, sessionID, action, at)
	return err
}
