package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

var _ identity.PhoneVerificationRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetPhoneVerification(ctx context.Context, accountID string) (*identity.AccountPhoneVerification, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT account_id::text, phone_e164, verified_at, otp_hash, otp_expires_at,
		       otp_attempts, resend_available_at, created_at, updated_at
		FROM public.cuenta_verificacion_telefono
		WHERE account_id = $1
	`, accountID)

	var (
		rec               identity.AccountPhoneVerification
		verifiedAt        *time.Time
		otpHash           *string
		otpExpiresAt      *time.Time
		resendAvailableAt *time.Time
	)

	err := row.Scan(
		&rec.AccountID,
		&rec.PhoneE164,
		&verifiedAt,
		&otpHash,
		&otpExpiresAt,
		&rec.OTPAttempts,
		&resendAvailableAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}

	if verifiedAt != nil {
		t := verifiedAt.UTC()
		rec.VerifiedAt = &t
	}
	rec.OTPHash = otpHash
	if otpExpiresAt != nil {
		t := otpExpiresAt.UTC()
		rec.OTPExpiresAt = &t
	}
	if resendAvailableAt != nil {
		t := resendAvailableAt.UTC()
		rec.ResendAvailableAt = &t
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	rec.UpdatedAt = rec.UpdatedAt.UTC()

	return &rec, nil
}

func (r *IdentityRepository) UpsertPhoneVerification(ctx context.Context, record identity.AccountPhoneVerification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO public.cuenta_verificacion_telefono (
			account_id,
			phone_e164,
			verified_at,
			otp_hash,
			otp_expires_at,
			otp_attempts,
			resend_available_at,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (account_id) DO UPDATE SET
			phone_e164 = EXCLUDED.phone_e164,
			verified_at = EXCLUDED.verified_at,
			otp_hash = EXCLUDED.otp_hash,
			otp_expires_at = EXCLUDED.otp_expires_at,
			otp_attempts = EXCLUDED.otp_attempts,
			resend_available_at = EXCLUDED.resend_available_at,
			updated_at = EXCLUDED.updated_at
	`, record.AccountID, record.PhoneE164, record.VerifiedAt, record.OTPHash, record.OTPExpiresAt,
		record.OTPAttempts, record.ResendAvailableAt, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) IncrementPhoneAttempts(ctx context.Context, accountID string) (int, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE public.cuenta_verificacion_telefono
		SET otp_attempts = otp_attempts + 1,
		    updated_at = now()
		WHERE account_id = $1
		RETURNING otp_attempts
	`, accountID)

	var attempts int
	if err := row.Scan(&attempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, identity.ErrPhoneNotPending
		}
		return 0, mapError(err)
	}
	return attempts, nil
}

func (r *IdentityRepository) MarkPhoneVerified(ctx context.Context, accountID string, verifiedAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE public.cuenta_verificacion_telefono
		SET verified_at = $2,
		    otp_hash = NULL,
		    otp_expires_at = NULL,
		    otp_attempts = 0,
		    updated_at = $2
		WHERE account_id = $1
	`, accountID, verifiedAt.UTC())
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrPhoneNotPending
	}
	return nil
}
