package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var pinpassCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

const pinpassEmailTTL = 5 * time.Minute
const pinpassEmailCooldown = time.Minute

func (x pinpassStepUp) emailDigest(account, session, email, challenge, code string) []byte {
	mac := hmac.New(sha256.New, x.pepper)
	for _, part := range []string{"espacigo:pinpass-email:v1", account, session, email, challenge, code} {
		mac.Write([]byte(part))
		mac.Write([]byte{0})
	}
	return mac.Sum(nil)
}

func (x pinpassStepUp) SendEmailCode(ctx context.Context, account, session, purpose string) (profilepin.EmailChallenge, error) {
	return x.sendEmailCode(ctx, account, session, purpose, "")
}

func (x pinpassStepUp) StartEmailChange(ctx context.Context, account, session, destination string, proof profilepin.Proof) (profilepin.EmailChallenge, error) {
	destination = strings.TrimSpace(destination)
	if len(destination) > 254 || identity.ValidateEmail(destination) != nil || proof.CurrentPassword == "" ||
		!pinpassCodePattern.MatchString(string(proof.OTPCode)) {
		return profilepin.EmailChallenge{}, profilepin.ErrInvalidInput
	}
	if err := x.Verify(ctx, account, session, proof); err != nil {
		return profilepin.EmailChallenge{}, profilepin.ErrStepUpRejected
	}
	return x.sendEmailCode(ctx, account, session, "new_email", destination)
}

func (x pinpassStepUp) sendEmailCode(ctx context.Context, account, session, purpose, destination string) (profilepin.EmailChallenge, error) {
	if x.mailer == nil || len(x.pepper) != 32 {
		return profilepin.EmailChallenge{}, profilepin.ErrEmailUnavailable
	}
	tx, err := x.pool.Begin(ctx)
	if err != nil {
		return profilepin.EmailChallenge{}, err
	}
	defer tx.Rollback(ctx)

	var email string
	err = tx.QueryRow(ctx, `SELECT u.correo_original FROM public.usuario u
		JOIN public.sesion s ON s.usuario_id=u.id AND s.id=$2
		WHERE u.id=$1 AND u.estado='activo' AND s.revocada_en IS NULL AND s.expira_en>clock_timestamp()
		FOR UPDATE OF u`, account, session).Scan(&email)
	if err != nil {
		return profilepin.EmailChallenge{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if destination != "" && strings.EqualFold(email, destination) {
		return profilepin.EmailChallenge{}, profilepin.ErrInvalidInput
	}
	recipient := email
	var target *string
	if destination != "" {
		recipient, target = destination, &destination
	}
	var sentAt time.Time
	err = tx.QueryRow(ctx, `SELECT sent_at FROM public.account_pinpass_email_challenge WHERE account_id=$1`, account).Scan(&sentAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return profilepin.EmailChallenge{}, err
	}
	if next := sentAt.Add(pinpassEmailCooldown); now.Before(next) {
		return profilepin.EmailChallenge{}, &profilepin.LockedError{Until: next}
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return profilepin.EmailChallenge{}, err
	}
	code := fmt.Sprintf("%06d", number.Int64())
	challenge := uuid.NewString()
	expiresAt := now.Add(pinpassEmailTTL)
	_, err = tx.Exec(ctx, `INSERT INTO public.account_pinpass_email_challenge
		(account_id,session_id,challenge_id,code_hmac,sent_at,expires_at,purpose,destination)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(account_id) DO UPDATE SET
		session_id=EXCLUDED.session_id,challenge_id=EXCLUDED.challenge_id,code_hmac=EXCLUDED.code_hmac,
		sent_at=EXCLUDED.sent_at,expires_at=EXCLUDED.expires_at,purpose=EXCLUDED.purpose,destination=EXCLUDED.destination,consumed_at=NULL`,
		account, session, challenge, x.emailDigest(account, session, email, challenge, code), now, expiresAt, purpose, target)
	if err != nil {
		return profilepin.EmailChallenge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return profilepin.EmailChallenge{}, err
	}
	if err := x.mailer.SendLocalBookingNotice(ctx, recipient, "Código de seguridad de EspaciGo",
		"Tu código para confirmar una operación de seguridad en EspaciGo es "+code+". Vence en cinco minutos."); err != nil {
		return profilepin.EmailChallenge{}, profilepin.ErrEmailUnavailable
	}
	return profilepin.EmailChallenge{MaskedEmail: identity.MaskEmail(recipient), ExpiresAt: expiresAt}, nil
}

func (x pinpassStepUp) ConfirmEmailChange(ctx context.Context, account, session string, code identity.Secret) error {
	if len(x.pepper) != 32 || !pinpassCodePattern.MatchString(string(code)) {
		return profilepin.ErrStepUpRejected
	}
	tx, err := x.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var email string
	if err := tx.QueryRow(ctx, `SELECT u.correo_original FROM public.usuario u
		JOIN public.sesion s ON s.usuario_id=u.id AND s.id=$2
		WHERE u.id=$1 AND u.estado='activo' AND s.revocada_en IS NULL AND s.expira_en>clock_timestamp()
		FOR UPDATE OF u`, account, session).Scan(&email); err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var challenge, destination string
	var digest []byte
	err = tx.QueryRow(ctx, `SELECT challenge_id::text,destination,code_hmac FROM public.account_pinpass_email_challenge
		WHERE account_id=$1 AND session_id=$2 AND purpose='new_email' AND expires_at>$3 AND consumed_at IS NULL`,
		account, session, now).Scan(&challenge, &destination, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilepin.ErrStepUpRejected
	}
	if err != nil {
		return err
	}
	if !hmac.Equal(digest, x.emailDigest(account, session, email, challenge, string(code))) {
		// A wrong code invalidates this challenge. A resend is rate limited.
		if _, err := tx.Exec(ctx, `UPDATE public.account_pinpass_email_challenge SET consumed_at=$2,destination=NULL WHERE account_id=$1`, account, now); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return profilepin.ErrStepUpRejected
	}
	var conflict *pgconn.PgError
	if _, err := tx.Exec(ctx, `UPDATE public.usuario SET correo_original=$2,correo_normalizado=$3,actualizado_en=$4 WHERE id=$1`,
		account, destination, identity.NormalizeEmail(destination), now); err != nil {
		if errors.As(err, &conflict) && conflict.ConstraintName == "usuario_correo_normalizado_uk" {
			return profilepin.ErrEmailInUse
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.sesion SET revocada_en=$3 WHERE usuario_id=$1 AND id<>$2 AND revocada_en IS NULL`, account, session, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.token_accion SET invalidado_en=$2 WHERE usuario_id=$1 AND invalidado_en IS NULL AND consumido_en IS NULL`, account, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.account_pinpass_email_challenge SET consumed_at=$2,destination=NULL WHERE account_id=$1`, account, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
