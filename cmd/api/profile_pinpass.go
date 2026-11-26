package main

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	profilepinpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/profilepin"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pinpassBaseRoute = "/api/v1/account/security/pinpass"

type pinpassStepUp struct {
	pool *pgxpool.Pool
	totp *identity.TOTPKeyring
}

func (x pinpassStepUp) Verify(ctx context.Context, account, session string, proof profilepin.Proof) error {
	if !proof.Present() || len(proof.CurrentPassword) > 72 || len(proof.OTPCode) > 6 {
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
	matches := true
	if proof.CurrentPassword != "" {
		matches, err = (password.Hasher{}).Matches(identity.Secret(hash), proof.CurrentPassword)
		if err != nil {
			return profilepin.ErrStepUpRejected
		}
	}
	// Verify the password first so a rejected password cannot consume a valid OTP.
	if matches && proof.OTPCode != "" {
		err = identitypg.VerifyAuthenticatorCode(ctx, tx, x.totp, account, proof.OTPCode, time.Now().UTC())
		switch {
		case err == nil:
		case errors.Is(err, identity.ErrTOTPInvalidCode), errors.Is(err, identity.ErrTOTPReplayDetected):
			matches = false
		default:
			return err
		}
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

func mountPinpass(mux *http.ServeMux, pool *pgxpool.Pool, auth profilepin.Authorizer, profiles profilepin.SensitiveProfileProvider, stepUp pinpassStepUp, getenv func(string) string) (*profilepin.Service, error) {
	var service *profilepin.Service
	if configured := strings.TrimSpace(getenv("PROFILE_PINPASS_PEPPER_HEX")); configured != "" {
		pepper, err := hex.DecodeString(configured)
		if err != nil || len(pepper) != 32 {
			return nil, errors.New("profile Pinpass configuration is invalid")
		}
		service, err = profilepin.NewService(profilepinpg.New(pool), stepUp, profiles, pepper)
		if err != nil {
			return nil, err
		}
	}
	handler := profilepin.NewHandler(auth, service)
	mux.Handle(pinpassBaseRoute, handler)
	mux.Handle(pinpassBaseRoute+"/", handler)
	return service, nil
}
