package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
)

var (
	ErrInvalidPhone        = errors.New("número de teléfono móvil chileno inválido")
	ErrPhoneCooldownActive = errors.New("espera antes de solicitar un nuevo código")
	ErrPhoneCodeInvalid    = errors.New("el código de verificación es incorrecto")
	ErrPhoneCodeExpired    = errors.New("el código de verificación ha expirado")
	ErrPhoneMaxAttempts    = errors.New("has superado el límite de intentos permitidos")
	ErrPhoneNotPending     = errors.New("no hay una verificación de teléfono pendiente")
)

// ValidateAndNormalizeChileanPhone validates and normalizes a Chilean mobile phone number into E.164 format (+569XXXXXXXX).
func ValidateAndNormalizeChileanPhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidPhone
	}

	var cleaned strings.Builder
	for _, r := range raw {
		if unicode.IsSpace(r) || r == '-' || r == '.' || r == '(' || r == ')' {
			continue
		}
		cleaned.WriteRune(r)
	}
	s := cleaned.String()

	var digits string
	if strings.HasPrefix(s, "+56") {
		digits = s[3:]
	} else if strings.HasPrefix(s, "56") {
		digits = s[2:]
	} else if strings.HasPrefix(s, "+") {
		return "", ErrInvalidPhone
	} else {
		digits = s
	}

	if len(digits) != 9 {
		return "", ErrInvalidPhone
	}
	if digits[0] != '9' {
		return "", ErrInvalidPhone
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return "", ErrInvalidPhone
		}
	}

	return "+56" + digits, nil
}

// AccountPhoneVerification stores the phone and OTP status for an account.
type AccountPhoneVerification struct {
	AccountID         string
	PhoneE164         string
	VerifiedAt        *time.Time
	OTPHash           *string
	OTPExpiresAt      *time.Time
	OTPAttempts       int
	ResendAvailableAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PhoneVerificationStatus contains public status of phone verification for an account.
type PhoneVerificationStatus struct {
	Configured          bool       `json:"configured"`
	Verified            bool       `json:"verified"`
	MaskedPhone         string     `json:"masked_phone"`
	VerifiedAt          *time.Time `json:"verified_at"`
	PendingVerification bool       `json:"pending_verification"`
	ResendAvailableAt   *time.Time `json:"resend_available_at"`
}

// PhoneCodeSentResult contains response after sending an OTP code.
type PhoneCodeSentResult struct {
	Status            string    `json:"status"`
	MaskedPhone       string    `json:"masked_phone"`
	ResendAvailableAt time.Time `json:"resend_available_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// PhoneVerifyResult contains response after verifying an OTP code.
type PhoneVerifyResult struct {
	Status      string    `json:"status"`
	Verified    bool      `json:"verified"`
	MaskedPhone string    `json:"masked_phone"`
	VerifiedAt  time.Time `json:"verified_at"`
}

// PhoneCodeSender sends a verification code to a phone number.
type PhoneCodeSender interface {
	SendPhoneCode(ctx context.Context, phoneE164 string, code string) error
}

// PhoneVerificationRepository manages persistence for phone verifications.
type PhoneVerificationRepository interface {
	GetPhoneVerification(ctx context.Context, accountID string) (*AccountPhoneVerification, error)
	UpsertPhoneVerification(ctx context.Context, record AccountPhoneVerification) error
	IncrementPhoneAttempts(ctx context.Context, accountID string) (int, error)
	MarkPhoneVerified(ctx context.Context, accountID string, verifiedAt time.Time) error
}

// PhoneVerificationService provides phone verification business logic.
type PhoneVerificationService struct {
	repo   PhoneVerificationRepository
	sender PhoneCodeSender
	now    func() time.Time
}

func NewPhoneVerificationService(repo PhoneVerificationRepository, sender PhoneCodeSender, now func() time.Time) *PhoneVerificationService {
	if now == nil {
		now = time.Now
	}
	return &PhoneVerificationService{
		repo:   repo,
		sender: sender,
		now:    now,
	}
}

func (s *PhoneVerificationService) GetPhoneVerificationStatus(ctx context.Context, accountID string) (PhoneVerificationStatus, error) {
	record, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return PhoneVerificationStatus{}, err
	}
	if record == nil {
		return PhoneVerificationStatus{
			Configured:          false,
			Verified:            false,
			MaskedPhone:         "",
			VerifiedAt:          nil,
			PendingVerification: false,
			ResendAvailableAt:   nil,
		}, nil
	}

	now := s.now().UTC()
	isVerified := record.VerifiedAt != nil
	isPending := !isVerified && record.OTPHash != nil && *record.OTPHash != "" &&
		record.OTPExpiresAt != nil && record.OTPExpiresAt.After(now) &&
		record.OTPAttempts < 5

	return PhoneVerificationStatus{
		Configured:          true,
		Verified:            isVerified,
		MaskedPhone:         MaskPhone(record.PhoneE164),
		VerifiedAt:          record.VerifiedAt,
		PendingVerification: isPending,
		ResendAvailableAt:   record.ResendAvailableAt,
	}, nil
}

func (s *PhoneVerificationService) SendPhoneVerificationCode(ctx context.Context, accountID, rawPhone string) (PhoneCodeSentResult, error) {
	phoneE164, err := ValidateAndNormalizeChileanPhone(rawPhone)
	if err != nil {
		return PhoneCodeSentResult{}, ErrInvalidPhone
	}

	now := s.now().UTC()
	record, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return PhoneCodeSentResult{}, err
	}

	if record != nil && record.ResendAvailableAt != nil && now.Before(*record.ResendAvailableAt) {
		return PhoneCodeSentResult{}, ErrPhoneCooldownActive
	}

	otp, err := generateOTP6()
	if err != nil {
		return PhoneCodeSentResult{}, err
	}

	hash := sha256.Sum256([]byte(otp))
	hashHex := hex.EncodeToString(hash[:])
	expiresAt := now.Add(10 * time.Minute)
	resendAvailableAt := now.Add(60 * time.Second)

	newRecord := AccountPhoneVerification{
		AccountID:         accountID,
		PhoneE164:         phoneE164,
		VerifiedAt:        nil,
		OTPHash:           &hashHex,
		OTPExpiresAt:      &expiresAt,
		OTPAttempts:       0,
		ResendAvailableAt: &resendAvailableAt,
		UpdatedAt:         now,
	}
	if record == nil {
		newRecord.CreatedAt = now
	} else {
		newRecord.CreatedAt = record.CreatedAt
	}

	if err := s.repo.UpsertPhoneVerification(ctx, newRecord); err != nil {
		return PhoneCodeSentResult{}, err
	}

	if s.sender != nil {
		if err := s.sender.SendPhoneCode(ctx, phoneE164, otp); err != nil {
			return PhoneCodeSentResult{}, err
		}
	}

	return PhoneCodeSentResult{
		Status:            "code_sent",
		MaskedPhone:       MaskPhone(phoneE164),
		ResendAvailableAt: resendAvailableAt,
		ExpiresAt:         expiresAt,
	}, nil
}

func (s *PhoneVerificationService) VerifyPhoneCode(ctx context.Context, accountID, code string) (PhoneVerifyResult, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return PhoneVerifyResult{}, ErrPhoneCodeInvalid
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return PhoneVerifyResult{}, ErrPhoneCodeInvalid
		}
	}

	record, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return PhoneVerifyResult{}, err
	}
	if record == nil || record.OTPHash == nil || *record.OTPHash == "" {
		return PhoneVerifyResult{}, ErrPhoneNotPending
	}

	now := s.now().UTC()
	if record.OTPAttempts >= 5 {
		return PhoneVerifyResult{}, ErrPhoneMaxAttempts
	}

	if record.OTPExpiresAt == nil || now.After(*record.OTPExpiresAt) {
		return PhoneVerifyResult{}, ErrPhoneCodeExpired
	}

	codeHash := hex.EncodeToString(sha256Hash([]byte(code)))
	if subtle.ConstantTimeCompare([]byte(*record.OTPHash), []byte(codeHash)) != 1 {
		attempts, err := s.repo.IncrementPhoneAttempts(ctx, accountID)
		if err != nil {
			return PhoneVerifyResult{}, err
		}
		if attempts >= 5 {
			return PhoneVerifyResult{}, ErrPhoneMaxAttempts
		}
		return PhoneVerifyResult{}, ErrPhoneCodeInvalid
	}

	if err := s.repo.MarkPhoneVerified(ctx, accountID, now); err != nil {
		return PhoneVerifyResult{}, err
	}

	safelog.Info(ctx, "phone verification succeeded",
		safelog.F("account_id", accountID),
		safelog.F("masked_phone", MaskPhone(record.PhoneE164)),
	)

	return PhoneVerifyResult{
		Status:      "verified",
		Verified:    true,
		MaskedPhone: MaskPhone(record.PhoneE164),
		VerifiedAt:  now,
	}, nil
}

func generateOTP6() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func sha256Hash(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
