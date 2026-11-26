package main

import (
	"context"
	"crypto/hmac"
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
	pool   *pgxpool.Pool
	totp   *identity.TOTPKeyring
	pepper []byte
	mailer productionNoticeSender
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
	var hash, email string
	err = tx.QueryRow(ctx, `SELECT COALESCE(u.hash_clave,''),u.correo_original FROM public.usuario u
		JOIN public.sesion s ON s.usuario_id=u.id AND s.id=$2
		WHERE u.id=$1 AND u.estado='activo' AND s.revocada_en IS NULL AND s.expira_en>clock_timestamp()
		FOR UPDATE OF u`, account, session).Scan(&hash, &email)
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
	var challenge string
	purpose := proof.Purpose
	if proof.ManagePinpass {
		var configured bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.account_pinpass WHERE account_id=$1)`, account).Scan(&configured); err != nil {
			return err
		}
		matches = configured == proof.ReplacingPinpass
		purpose = "pinpass"
	}
	emailRequired := purpose == "pinpass" || purpose == "authenticator"
	if emailRequired && matches {
		var digest []byte
		err = tx.QueryRow(ctx, `SELECT challenge_id::text,code_hmac FROM public.account_pinpass_email_challenge
			WHERE account_id=$1 AND session_id=$2 AND expires_at>$3 AND consumed_at IS NULL AND purpose=$4`,
			account, session, now, purpose).Scan(&challenge, &digest)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		matches = err == nil && len(x.pepper) == 32 &&
			hmac.Equal(digest, x.emailDigest(account, session, email, challenge, string(proof.EmailCode)))
	} else if !emailRequired && proof.EmailCode != "" {
		return profilepin.ErrStepUpRejected
	}
	if matches && proof.CurrentPassword != "" {
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
		if emailRequired {
			if _, err := tx.Exec(ctx, `UPDATE public.account_pinpass_email_challenge SET consumed_at=$3 WHERE account_id=$1 AND challenge_id=$2`, account, challenge, now); err != nil {
				return err
			}
		}
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

func mountPinpass(mux *http.ServeMux, pool *pgxpool.Pool, auth *identity.AuthenticationService, profiles profilepin.SensitiveProfileProvider, stepUp pinpassStepUp, mailer identity.AuthenticationMailer, getenv func(string) string) (*profilepin.Service, error) {
	var service *profilepin.Service
	if configured := strings.TrimSpace(getenv("PROFILE_PINPASS_PEPPER_HEX")); configured != "" {
		pepper, err := hex.DecodeString(configured)
		if err != nil || len(pepper) != 32 {
			return nil, errors.New("profile Pinpass configuration is invalid")
		}
		stepUp.pepper = pepper
		stepUp.mailer, _ = mailer.(productionNoticeSender)
		service, err = profilepin.NewService(profilepinpg.New(pool), stepUp, profiles, pepper)
		if err != nil {
			return nil, err
		}
	}
	auth.SetSecurityChangeVerifier(func(ctx context.Context, p identity.Principal, operation string, password, otp, emailCode, pin identity.Secret) error {
		if password == "" {
			return identity.ErrSecurityConfirmation
		}
		proof := profilepin.Proof{CurrentPassword: password, Purpose: operation}
		switch operation {
		case "password":
			if pinpassCodePattern.MatchString(string(otp)) && pin == "" {
				proof.OTPCode = otp
			} else if otp == "" && pinpassCodePattern.MatchString(string(pin)) && service != nil {
				// Verify the password before trying a PIN so wrong passwords do not create a PIN grant.
				if err := stepUp.Verify(ctx, p.AccountID, p.SessionID, proof); err != nil {
					return identity.ErrSecurityConfirmation
				}
				if err := service.VerifyPIN(ctx, p.AccountID, p.SessionID, string(pin)); err != nil {
					return identity.ErrSecurityConfirmation
				}
				return nil
			} else {
				return identity.ErrSecurityConfirmation
			}
		case "authenticator":
			if len(stepUp.pepper) != 32 || !pinpassCodePattern.MatchString(string(emailCode)) {
				return identity.ErrSecurityConfirmation
			}
			proof.EmailCode = emailCode
		default:
			return identity.ErrSecurityConfirmation
		}
		if err := stepUp.Verify(ctx, p.AccountID, p.SessionID, proof); err != nil {
			return identity.ErrSecurityConfirmation
		}
		return nil
	})
	handler := profilepin.NewHandler(auth, service, stepUp)
	mux.Handle(pinpassBaseRoute, handler)
	mux.Handle(pinpassBaseRoute+"/", handler)
	mux.Handle("POST /api/v1/account/security/confirmation/email-code", handler)
	mux.Handle("POST /api/v1/account/security/email/change", handler)
	mux.Handle("POST /api/v1/account/security/email/confirm", handler)
	return service, nil
}
