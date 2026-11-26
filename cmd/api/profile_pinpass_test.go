package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pinpassTestMailer struct {
	recipient string
	code      identity.Secret
}

func (m *pinpassTestMailer) SendLocalBookingNotice(_ context.Context, recipient, _, body string) error {
	m.recipient = recipient
	_, tail, _ := strings.Cut(body, " es ")
	code, _, _ := strings.Cut(tail, ".")
	m.code = identity.Secret(code)
	return nil
}

func TestPostgresPinpassPasswordStepUp(t *testing.T) {
	dsn := os.Getenv("ESPACIGO_PROFILE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("run against the disposable profile database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE espacigo_runtime")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const secret = "Synthetic-Pass-596!"
	hash, err := (password.Hasher{}).Hash(secret)
	if err != nil {
		t.Fatal(err)
	}
	account, session := uuid.NewString(), uuid.NewString()
	if _, err := admin.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,'reauth@example.test','reauth@example.test',$2,'activo')`, account, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.sesion(id,usuario_id,expira_en,ultima_actividad_en) VALUES ($1,$2,now()+interval '1 hour',now())`, session, account); err != nil {
		t.Fatal(err)
	}
	mailer := &pinpassTestMailer{}
	gate := pinpassStepUp{pool: pool, pepper: make([]byte, 32), mailer: mailer}
	if _, err := gate.SendEmailCode(ctx, account, session, "pinpass"); err != nil {
		t.Fatal(err)
	}
	if mailer.recipient != "reauth@example.test" || !pinpassCodePattern.MatchString(string(mailer.code)) {
		t.Fatal("expected a six-digit code sent to the account email")
	}
	first := profilepin.Proof{CurrentPassword: secret, EmailCode: mailer.code, ManagePinpass: true}
	if err := gate.Verify(ctx, account, session, first); err != nil {
		t.Fatalf("initial Pinpass proof without an authenticator: %v", err)
	}
	if err := gate.Verify(ctx, account, session, first); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("email code replay: %v", err)
	}
	if err := gate.Verify(ctx, account, session, profilepin.Proof{CurrentPassword: secret}); err != nil {
		t.Fatalf("current password and session rejected: %v", err)
	}
	if err := gate.Verify(ctx, account, uuid.NewString(), profilepin.Proof{CurrentPassword: secret}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("proof accepted with missing session: %v", err)
	}
	otpKeys, err := identity.NewTOTPKeyring("test", map[string][]byte{"test": make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	gate.totp = otpKeys
	if err := gate.Verify(ctx, account, session, profilepin.Proof{OTPCode: "123456"}); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("OTP accepted without an enrolled authenticator: %v", err)
	}
	authenticator, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	version, ciphertext, err := otpKeys.Seal(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.cuenta_totp(account_id,secret_ciphertext,key_version,enabled_at)
		VALUES($1,$2,$3,now())`, account, ciphertext, version); err != nil {
		t.Fatal(err)
	}
	repo := identitypg.NewIdentityRepository(pool)
	if err := repo.SavePendingTOTPSecret(ctx, account, ciphertext, version); !errors.Is(err, identity.ErrTOTPAlreadyEnabled) {
		t.Fatalf("an open session could replace an active authenticator: %v", err)
	}
	step := time.Now().Unix() / 30
	otpOnly := profilepin.Proof{OTPCode: identity.Secret(identity.GenerateTOTPCode(authenticator, step, 6))}
	if err := gate.Verify(ctx, account, session, otpOnly); err != nil {
		t.Fatalf("valid OTP alone: %v", err)
	}
	if err := gate.Verify(ctx, account, session, otpOnly); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("OTP replay: %v", err)
	}
	both := profilepin.Proof{CurrentPassword: secret, OTPCode: identity.Secret(identity.GenerateTOTPCode(authenticator, step+1, 6))}
	if err := gate.Verify(ctx, account, session, profilepin.Proof{CurrentPassword: "synthetic-wrong", OTPCode: both.OTPCode}); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("valid OTP bypassed wrong password: %v", err)
	}
	if err := gate.Verify(ctx, account, session, both); err != nil {
		t.Fatalf("valid OTP and password or failed password consumed OTP: %v", err)
	}
	if err := gate.Verify(ctx, account, session, both); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("valid password bypassed replayed OTP: %v", err)
	}
	if err := gate.Verify(ctx, account, session, profilepin.Proof{CurrentPassword: secret}); err != nil {
		t.Fatalf("password-only compatibility: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE public.cuenta_totp SET last_verified_step=NULL WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE public.account_pinpass_email_challenge SET sent_at=now()-interval '2 minutes' WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	change := profilepin.Proof{CurrentPassword: secret, OTPCode: identity.Secret(identity.GenerateTOTPCode(authenticator, time.Now().Unix()/30, 6))}
	if _, err := gate.StartEmailChange(ctx, account, session, "reauth-new@example.test", change); err != nil {
		t.Fatal(err)
	}
	if mailer.recipient != "reauth-new@example.test" {
		t.Fatal("email change challenge did not go to the new address")
	}
	if err := gate.ConfirmEmailChange(ctx, account, session, mailer.code); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := admin.QueryRow(ctx, `SELECT correo_normalizado FROM public.usuario WHERE id=$1`, account).Scan(&email); err != nil || email != "reauth-new@example.test" {
		t.Fatalf("new email was not saved: %v", err)
	}
	if err := gate.ConfirmEmailChange(ctx, account, session, mailer.code); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("new email code replay: %v", err)
	}
	for range 5 {
		if err := gate.Verify(ctx, account, session, profilepin.Proof{CurrentPassword: "synthetic-wrong"}); !errors.Is(err, profilepin.ErrStepUpRejected) {
			t.Fatalf("wrong password result: %v", err)
		}
	}
	if err := gate.Verify(ctx, account, session, profilepin.Proof{CurrentPassword: secret}); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("valid password bypassed lockout: %v", err)
	}
}
