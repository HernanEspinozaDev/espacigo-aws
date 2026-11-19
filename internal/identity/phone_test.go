package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type memoryPhoneRepo struct {
	records map[string]identity.AccountPhoneVerification
}

func newMemoryPhoneRepo() *memoryPhoneRepo {
	return &memoryPhoneRepo{records: make(map[string]identity.AccountPhoneVerification)}
}

func (m *memoryPhoneRepo) GetPhoneVerification(_ context.Context, accountID string) (*identity.AccountPhoneVerification, error) {
	rec, ok := m.records[accountID]
	if !ok {
		return nil, nil
	}
	cp := rec
	return &cp, nil
}

func (m *memoryPhoneRepo) UpsertPhoneVerification(_ context.Context, record identity.AccountPhoneVerification) error {
	m.records[record.AccountID] = record
	return nil
}

func (m *memoryPhoneRepo) IncrementPhoneAttempts(_ context.Context, accountID string) (int, error) {
	rec, ok := m.records[accountID]
	if !ok {
		return 0, identity.ErrPhoneNotPending
	}
	rec.OTPAttempts++
	m.records[accountID] = rec
	return rec.OTPAttempts, nil
}

func (m *memoryPhoneRepo) MarkPhoneVerified(_ context.Context, accountID string, verifiedAt time.Time) error {
	rec, ok := m.records[accountID]
	if !ok {
		return identity.ErrPhoneNotPending
	}
	rec.VerifiedAt = &verifiedAt
	rec.OTPHash = nil
	rec.OTPExpiresAt = nil
	rec.OTPAttempts = 0
	rec.UpdatedAt = verifiedAt
	m.records[accountID] = rec
	return nil
}

type memoryPhoneSender struct {
	sentCalls []sentCall
}

type sentCall struct {
	phone string
	code  string
}

func (s *memoryPhoneSender) SendPhoneCode(_ context.Context, phoneE164, code string) error {
	s.sentCalls = append(s.sentCalls, sentCall{phone: phoneE164, code: code})
	return nil
}

func TestValidateAndNormalizeChileanPhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"e164 valid", "+56912345678", "+56912345678", false},
		{"with spaces", "+56 9 1234 5678", "+56912345678", false},
		{"with hyphens", "+56-9-1234-5678", "+56912345678", false},
		{"with parens and dots", "+56 (9) 1234.5678", "+56912345678", false},
		{"national 9 digits", "912345678", "+56912345678", false},
		{"national with spaces", "9 1234 5678", "+56912345678", false},
		{"without plus", "56912345678", "+56912345678", false},
		{"empty", "", "", true},
		{"spaces only", "   ", "", true},
		{"non-digits", "91234567a", "", true},
		{"landline 2 Santiago", "+56223456789", "", true},
		{"foreign country code", "+5491112345678", "", true},
		{"too short", "91234567", "", true},
		{"too long", "9123456789", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.ValidateAndNormalizeChileanPhone(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateAndNormalizeChileanPhone(%q) err = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("ValidateAndNormalizeChileanPhone(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPhoneVerificationService_Flow(t *testing.T) {
	repo := newMemoryPhoneRepo()
	sender := &memoryPhoneSender{}
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	svc := identity.NewPhoneVerificationService(repo, sender, clock)
	ctx := context.Background()
	accountID := "acc-1234"

	// 1. Initial status -> not configured
	status, err := svc.GetPhoneVerificationStatus(ctx, accountID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if status.Configured || status.Verified || status.PendingVerification || status.MaskedPhone != "" {
		t.Fatalf("expected unconfigured, got: %+v", status)
	}

	// 2. Send code with invalid phone
	_, err = svc.SendPhoneVerificationCode(ctx, accountID, "invalid")
	if !errors.Is(err, identity.ErrInvalidPhone) {
		t.Fatalf("expected ErrInvalidPhone, got %v", err)
	}

	// 3. Send code valid
	sendResult, err := svc.SendPhoneVerificationCode(ctx, accountID, "+56 9 1234 5678")
	if err != nil {
		t.Fatalf("send code err: %v", err)
	}
	if sendResult.MaskedPhone != "+56 ••••5678" {
		t.Fatalf("unexpected masked phone: %s", sendResult.MaskedPhone)
	}
	if len(sender.sentCalls) != 1 {
		t.Fatalf("expected 1 sent call, got %d", len(sender.sentCalls))
	}
	otpCode := sender.sentCalls[0].code
	if len(otpCode) != 6 {
		t.Fatalf("expected 6-digit code, got %s", otpCode)
	}

	// 4. Cooldown active (within 60s)
	now = now.Add(30 * time.Second)
	_, err = svc.SendPhoneVerificationCode(ctx, accountID, "+56 9 1234 5678")
	if !errors.Is(err, identity.ErrPhoneCooldownActive) {
		t.Fatalf("expected ErrPhoneCooldownActive, got %v", err)
	}

	// Check status -> pending verification
	status, err = svc.GetPhoneVerificationStatus(ctx, accountID)
	if err != nil {
		t.Fatalf("status err: %v", err)
	}
	if !status.Configured || status.Verified || !status.PendingVerification {
		t.Fatalf("expected configured and pending, got: %+v", status)
	}

	// 5. Verify with incorrect code format
	_, err = svc.VerifyPhoneCode(ctx, accountID, "abc")
	if !errors.Is(err, identity.ErrPhoneCodeInvalid) {
		t.Fatalf("expected ErrPhoneCodeInvalid for format, got %v", err)
	}

	// 6. Verify with incorrect code (attempts 1..4)
	for i := 1; i <= 4; i++ {
		_, err = svc.VerifyPhoneCode(ctx, accountID, "000000")
		if !errors.Is(err, identity.ErrPhoneCodeInvalid) {
			t.Fatalf("attempt %d: expected ErrPhoneCodeInvalid, got %v", i, err)
		}
	}

	// 7. 5th failed attempt -> max attempts reached
	_, err = svc.VerifyPhoneCode(ctx, accountID, "000000")
	if !errors.Is(err, identity.ErrPhoneMaxAttempts) {
		t.Fatalf("attempt 5: expected ErrPhoneMaxAttempts, got %v", err)
	}

	// Any subsequent attempt blocked
	_, err = svc.VerifyPhoneCode(ctx, accountID, otpCode)
	if !errors.Is(err, identity.ErrPhoneMaxAttempts) {
		t.Fatalf("expected ErrPhoneMaxAttempts, got %v", err)
	}

	// 8. Cooldown passed (>60s) -> send new code
	now = now.Add(40 * time.Second) // now at 70s
	sendResult, err = svc.SendPhoneVerificationCode(ctx, accountID, "+56 9 1234 5678")
	if err != nil {
		t.Fatalf("send code after cooldown err: %v", err)
	}
	newOTP := sender.sentCalls[len(sender.sentCalls)-1].code

	// 9. Expiration test
	now = now.Add(11 * time.Minute) // past 10m expiration
	_, err = svc.VerifyPhoneCode(ctx, accountID, newOTP)
	if !errors.Is(err, identity.ErrPhoneCodeExpired) {
		t.Fatalf("expected ErrPhoneCodeExpired, got %v", err)
	}

	// 10. Request new code and verify successfully
	now = now.Add(65 * time.Second)
	_, err = svc.SendPhoneVerificationCode(ctx, accountID, "+56 9 1234 5678")
	if err != nil {
		t.Fatalf("send code err: %v", err)
	}
	latestOTP := sender.sentCalls[len(sender.sentCalls)-1].code

	verifyResult, err := svc.VerifyPhoneCode(ctx, accountID, latestOTP)
	if err != nil {
		t.Fatalf("verify code err: %v", err)
	}
	if !verifyResult.Verified || verifyResult.MaskedPhone != "+56 ••••5678" {
		t.Fatalf("unexpected verify result: %+v", verifyResult)
	}

	// 11. Final status -> verified
	status, err = svc.GetPhoneVerificationStatus(ctx, accountID)
	if err != nil {
		t.Fatalf("status err: %v", err)
	}
	if !status.Configured || !status.Verified || status.PendingVerification || status.VerifiedAt == nil {
		t.Fatalf("expected verified status, got: %+v", status)
	}

	// 12. Trying to verify again when no code pending
	_, err = svc.VerifyPhoneCode(ctx, accountID, latestOTP)
	if !errors.Is(err, identity.ErrPhoneNotPending) {
		t.Fatalf("expected ErrPhoneNotPending, got %v", err)
	}
}
