package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	dbgen "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity/dbgen"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ identity.AdminSessionRepository = (*IdentityRepository)(nil)

type adminSessionTransaction struct {
	*authenticationTransaction
}

func (r *IdentityRepository) WithLockedAdminAccount(
	ctx context.Context,
	lookup identity.AdminAccountLookup,
	fn func(identity.Account, identity.AdminSessionTransaction) error,
) error {
	count := 0
	for _, key := range []string{lookup.Email, lookup.ChallengeID, lookup.AdminSessionHash} {
		if key != "" {
			count++
		}
	}
	if count != 1 || fn == nil {
		return identity.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	q := r.queries.WithTx(tx)
	accountID, err := adminAccountID(ctx, tx, lookup)
	if err != nil {
		return mapError(err)
	}
	if err := accountlock.LockAccountIDs(ctx, tx, accountID); err != nil {
		return err
	}
	var row dbgen.LockAccountByEmailRow
	err = tx.QueryRow(ctx, `
		SELECT id::text, correo_original, correo_normalizado,
			hash_clave, estado, creado_en, actualizado_en,
			intentos_fallidos_consecutivos, bloqueado_hasta, preferencia_uso
		FROM public.usuario
		WHERE id = $1
		FOR UPDATE
	`, accountID).Scan(
		&row.ID, &row.Email, &row.NormalizedEmail,
		&row.PasswordHash, &row.State, &row.CreatedAt, &row.UpdatedAt,
		&row.FailedAttempts, &row.BlockedUntil, &row.UsePreference,
	)
	if err != nil {
		return mapError(err)
	}
	account := accountFrom(
		row.ID, row.Email, row.NormalizedEmail, row.PasswordHash, row.State,
		row.CreatedAt, row.UpdatedAt, row.FailedAttempts, row.BlockedUntil, row.UsePreference,
	)
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local WHERE cuenta_id=$1)`, account.ID).Scan(&account.AdministrativeBlocked); err != nil {
		return err
	}
	unit := &adminSessionTransaction{
		authenticationTransaction: &authenticationTransaction{&IdentityRepository{queries: q, tx: tx}},
	}
	if err := fn(account, unit); err != nil {
		return err
	}
	return mapError(tx.Commit(ctx))
}

func adminAccountID(ctx context.Context, tx pgx.Tx, lookup identity.AdminAccountLookup) (string, error) {
	var query, value string
	switch {
	case lookup.Email != "":
		query, value = `SELECT id::text FROM public.usuario WHERE correo_normalizado=$1`, lookup.Email
	case lookup.ChallengeID != "":
		query, value = `SELECT cuenta_id::text FROM public.desafio_mfa_administrativo WHERE id=$1`, lookup.ChallengeID
	default:
		query, value = `SELECT cuenta_id::text FROM public.sesion_administrativa WHERE token_hash=$1 AND audiencia='admin'`, lookup.AdminSessionHash
	}
	var accountID string
	err := tx.QueryRow(ctx, query, value).Scan(&accountID)
	return accountID, err
}

func (t *adminSessionTransaction) CreateAdminSession(ctx context.Context, session identity.AdminSession) error {
	if session.ID == "" || session.AccountID == "" || session.TokenHash == "" {
		return identity.ErrInvalid
	}
	_, err := t.tx.Exec(ctx, `
		INSERT INTO public.sesion_administrativa (
			id, cuenta_id, audiencia, token_hash, creada_en, ultima_actividad_en,
			expira_en, revocada_en, mfa_verificado_en, mfa_metodo,
			etiqueta_dispositivo, ip_enmascarada, user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		session.ID,
		session.AccountID,
		identity.AdminSessionAudience,
		strings.ToLower(session.TokenHash),
		dbTime(session.CreatedAt),
		dbTime(session.LastActivityAt),
		dbTime(session.ExpiresAt),
		dbNullableTime(session.RevokedAt),
		dbNullableTime(session.MFAVerifiedAt),
		session.MFAMethod,
		session.DeviceLabel,
		session.IPMasked,
		session.UserAgent,
	)
	return mapError(err)
}

func (t *adminSessionTransaction) AdminSessionByTokenHash(ctx context.Context, tokenHash string) (identity.AdminSession, error) {
	var (
		s          identity.AdminSession
		createdAt  pgtype.Timestamptz
		lastAct    pgtype.Timestamptz
		expiresAt  pgtype.Timestamptz
		revokedAt  pgtype.Timestamptz
		mfaVerifAt pgtype.Timestamptz
	)
	err := t.tx.QueryRow(ctx, `
		SELECT id::text, cuenta_id::text, audiencia, token_hash,
			creada_en, ultima_actividad_en, expira_en, revocada_en,
			mfa_verificado_en, mfa_metodo, etiqueta_dispositivo, ip_enmascarada, user_agent
		FROM public.sesion_administrativa
		WHERE token_hash = $1 AND audiencia = 'admin'
	`, strings.ToLower(tokenHash)).Scan(
		&s.ID, &s.AccountID, &s.Audience, &s.TokenHash,
		&createdAt, &lastAct, &expiresAt, &revokedAt,
		&mfaVerifAt, &s.MFAMethod, &s.DeviceLabel, &s.IPMasked, &s.UserAgent,
	)
	if err != nil {
		return identity.AdminSession{}, mapError(err)
	}
	s.CreatedAt = createdAt.Time
	s.LastActivityAt = lastAct.Time
	s.ExpiresAt = expiresAt.Time
	s.RevokedAt = nullableTime(revokedAt)
	s.MFAVerifiedAt = nullableTime(mfaVerifAt)
	return s, nil
}

func (t *adminSessionTransaction) TouchAdminSession(ctx context.Context, sessionID string, at time.Time) (bool, error) {
	idleCutoff := at.Add(-identity.AdminSessionIdleTimeout)
	tag, err := t.tx.Exec(ctx, `
		UPDATE public.sesion_administrativa
		SET ultima_actividad_en = $2
		WHERE id = $1
		  AND revocada_en IS NULL
		  AND expira_en > $2
		  AND ultima_actividad_en > $3
	`, sessionID, dbTime(at), dbTime(idleCutoff))
	if err != nil {
		return false, mapError(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (t *adminSessionTransaction) RevokeAdminSession(ctx context.Context, sessionID string, at time.Time) error {
	tag, err := t.tx.Exec(ctx, `
		UPDATE public.sesion_administrativa
		SET revocada_en = COALESCE(revocada_en, $2)
		WHERE id = $1
	`, sessionID, dbTime(at))
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}

func (t *adminSessionTransaction) UpdateAdminSessionMFA(ctx context.Context, sessionID string, verifiedAt time.Time, method string) error {
	tag, err := t.tx.Exec(ctx, `
		UPDATE public.sesion_administrativa
		SET mfa_verificado_en = $2, mfa_metodo = $3
		WHERE id = $1 AND revocada_en IS NULL
	`, sessionID, dbTime(verifiedAt), method)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}

func (t *adminSessionTransaction) CreateAdminMFAChallenge(ctx context.Context, challenge identity.AdminMFAChallenge) error {
	if challenge.ID == "" || challenge.AccountID == "" {
		return identity.ErrInvalid
	}
	var adminSessionID any
	if challenge.AdminSessionID != "" {
		adminSessionID = challenge.AdminSessionID
	}
	_, err := t.tx.Exec(ctx, `
		INSERT INTO public.desafio_mfa_administrativo (
			id, cuenta_id, sesion_administrativa_id, proposito,
			intentos, max_intentos, creado_en, expira_en, consumido_en,
			ip_enmascarada, user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		challenge.ID,
		challenge.AccountID,
		adminSessionID,
		challenge.Purpose,
		challenge.Attempts,
		challenge.MaxAttempts,
		dbTime(challenge.CreatedAt),
		dbTime(challenge.ExpiresAt),
		dbNullableTime(challenge.ConsumedAt),
		challenge.IPMasked,
		challenge.UserAgent,
	)
	return mapError(err)
}

func (t *adminSessionTransaction) AdminMFAChallengeByID(ctx context.Context, challengeID string) (identity.AdminMFAChallenge, error) {
	var (
		c          identity.AdminMFAChallenge
		sessionID  *string
		createdAt  pgtype.Timestamptz
		expiresAt  pgtype.Timestamptz
		consumedAt pgtype.Timestamptz
	)
	err := t.tx.QueryRow(ctx, `
		SELECT id::text, cuenta_id::text, sesion_administrativa_id::text, proposito,
			intentos, max_intentos, creado_en, expira_en, consumido_en,
			ip_enmascarada, user_agent
		FROM public.desafio_mfa_administrativo
		WHERE id = $1
		FOR UPDATE
	`, challengeID).Scan(
		&c.ID, &c.AccountID, &sessionID, &c.Purpose,
		&c.Attempts, &c.MaxAttempts, &createdAt, &expiresAt, &consumedAt,
		&c.IPMasked, &c.UserAgent,
	)
	if err != nil {
		return identity.AdminMFAChallenge{}, mapError(err)
	}
	if sessionID != nil {
		c.AdminSessionID = *sessionID
	}
	c.CreatedAt = createdAt.Time
	c.ExpiresAt = expiresAt.Time
	c.ConsumedAt = nullableTime(consumedAt)
	return c, nil
}

func (t *adminSessionTransaction) IncrementAdminMFAChallengeAttempt(ctx context.Context, challengeID string) (int, error) {
	var attempts int
	err := t.tx.QueryRow(ctx, `
		UPDATE public.desafio_mfa_administrativo
		SET intentos = intentos + 1
		WHERE id = $1
		RETURNING intentos
	`, challengeID).Scan(&attempts)
	if err != nil {
		return 0, mapError(err)
	}
	return attempts, nil
}

func (t *adminSessionTransaction) ConsumeAdminMFAChallenge(ctx context.Context, challengeID string, at time.Time) error {
	tag, err := t.tx.Exec(ctx, `
		UPDATE public.desafio_mfa_administrativo
		SET consumido_en = $2
		WHERE id = $1 AND consumido_en IS NULL
	`, challengeID, dbTime(at))
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}
