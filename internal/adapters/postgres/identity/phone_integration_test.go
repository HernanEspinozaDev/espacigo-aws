package postgres_test

import (
	"context"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type testPhoneSender struct {
	lastPhone string
	lastCode  string
}

func (s *testPhoneSender) Channels() []identity.PhoneChannel {
	return []identity.PhoneChannel{identity.PhoneChannelSMS}
}

func (s *testPhoneSender) SendPhoneCode(_ context.Context, phone, code string, _ identity.PhoneChannel) error {
	s.lastPhone = phone
	s.lastCode = code
	return nil
}

func TestPostgresPhoneVerification_Flow(t *testing.T) {
	ctx, pool := newIdentityTestPool(t)
	repo := identitypg.NewIdentityRepository(pool)

	account := testAccount("00000000-0000-4000-8000-000000000095", "phoneuser@ejemplo.invalid")
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create account: %v", err)
	}

	sender := &testPhoneSender{}
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	svc := identity.NewPhoneVerificationService(repo, sender, clock, []byte("test-key"))

	// 1. Initial status -> not configured
	status, err := svc.GetPhoneVerificationStatus(ctx, account.ID)
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if status.Configured || status.Verified || status.PendingVerification {
		t.Fatalf("expected unconfigured, got: %+v", status)
	}

	// 2. Send code
	res, err := svc.SendPhoneVerificationCode(ctx, account.ID, "+56 9 8765 4321", "")
	if err != nil {
		t.Fatalf("send code error: %v", err)
	}
	if res.MaskedPhone != "+56 ••••4321" {
		t.Fatalf("unexpected masked phone: %s", res.MaskedPhone)
	}
	if sender.lastCode == "" || len(sender.lastCode) != 6 {
		t.Fatalf("invalid generated code: %s", sender.lastCode)
	}

	// 3. Status is now pending
	status, err = svc.GetPhoneVerificationStatus(ctx, account.ID)
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if !status.Configured || status.Verified || !status.PendingVerification {
		t.Fatalf("expected configured and pending, got: %+v", status)
	}
	if status.MaskedPhone != "+56 ••••4321" {
		t.Fatalf("expected masked phone +56 ••••4321, got %s", status.MaskedPhone)
	}

	// 4. Failed verification attempt
	incorrect := "000000"
	if sender.lastCode == incorrect {
		incorrect = "000001"
	}
	_, err = svc.VerifyPhoneCode(ctx, account.ID, incorrect)
	if err == nil {
		t.Fatalf("expected error on wrong code, got nil")
	}

	// 5. Successful verification
	verifyRes, err := svc.VerifyPhoneCode(ctx, account.ID, sender.lastCode)
	if err != nil {
		t.Fatalf("verify code error: %v", err)
	}
	if !verifyRes.Verified || verifyRes.MaskedPhone != "+56 ••••4321" {
		t.Fatalf("unexpected verify response: %+v", verifyRes)
	}

	// 6. Final status -> verified
	status, err = svc.GetPhoneVerificationStatus(ctx, account.ID)
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if !status.Configured || !status.Verified || status.PendingVerification || status.VerifiedAt == nil {
		t.Fatalf("expected verified status, got: %+v", status)
	}
}
