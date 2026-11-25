package main

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	profilepinpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/profilepin"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pinpassBaseRoute = "/api/v1/account/security/pinpass"

type pinpassPasswordStepUp struct{ pool *pgxpool.Pool }

func (x pinpassPasswordStepUp) Verify(ctx context.Context, account, session, proof string) error {
	secret, ok := strings.CutPrefix(proof, "password:")
	if !ok || secret == "" || len(secret) > 72 {
		return profilepin.ErrStepUpRejected
	}
	tx, err := x.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hash string
	err = tx.QueryRow(ctx, `SELECT u.hash_clave FROM public.usuario u
		JOIN public.sesion s ON s.usuario_id=u.id AND s.id=$2
		WHERE u.id=$1 AND u.estado='activo' AND s.revocada_en IS NULL AND s.expira_en>clock_timestamp()
		FOR UPDATE OF u`, account, session).Scan(&hash)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var attempts int
	var blocked *time.Time
	err = tx.QueryRow(ctx, `SELECT failed_attempts,blocked_until FROM public.profile_pinpass_reauth
		WHERE account_id=$1`, account).Scan(&attempts, &blocked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if blocked != nil {
		if blocked.After(now) {
			return profilepin.ErrStepUpRejected
		}
		attempts = 0
	}
	matches, err := (password.Hasher{}).Matches(identity.Secret(hash), identity.Secret(secret))
	if err != nil {
		return profilepin.ErrStepUpRejected
	}
	if matches {
		if _, err := tx.Exec(ctx, `DELETE FROM public.profile_pinpass_reauth WHERE account_id=$1`, account); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	attempts++
	var nextBlock *time.Time
	if attempts >= 5 {
		next := now.Add(15 * time.Minute)
		nextBlock = &next
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.profile_pinpass_reauth(account_id,failed_attempts,blocked_until,updated_at)
		VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET
		failed_attempts=EXCLUDED.failed_attempts,blocked_until=EXCLUDED.blocked_until,updated_at=EXCLUDED.updated_at`, account, attempts, nextBlock, now)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return profilepin.ErrStepUpRejected
}

func mountPinpass(mux *http.ServeMux, pool *pgxpool.Pool, auth profilepin.Authorizer, profiles profilepin.SensitiveProfileProvider, getenv func(string) string) error {
	var service *profilepin.Service
	if configured := strings.TrimSpace(getenv("PROFILE_PINPASS_PEPPER_HEX")); configured != "" {
		pepper, err := hex.DecodeString(configured)
		if err != nil || len(pepper) != 32 {
			return errors.New("profile Pinpass configuration is invalid")
		}
		service, err = profilepin.NewService(profilepinpg.New(pool), pinpassPasswordStepUp{pool: pool}, profiles, pepper)
		if err != nil {
			return err
		}
	}
	handler := profilepin.NewHandler(auth, service)
	mux.Handle(pinpassBaseRoute, handler)
	mux.Handle(pinpassBaseRoute+"/", handler)
	return nil
}
