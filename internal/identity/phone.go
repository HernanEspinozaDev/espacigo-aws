package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
)

var (
	ErrInvalidPhone             = errors.New("número de teléfono móvil chileno inválido")
	ErrPhoneCooldownActive      = errors.New("espera antes de solicitar un nuevo código")
	ErrPhoneCodeInvalid         = errors.New("el código de verificación es incorrecto")
	ErrPhoneCodeExpired         = errors.New("el código de verificación ha expirado")
	ErrPhoneMaxAttempts         = errors.New("has superado el límite de intentos permitidos")
	ErrPhoneNotPending          = errors.New("no hay una verificación de teléfono pendiente")
	ErrPhoneDeliveryUnavailable = errors.New("canal de verificación telefónica no disponible")
	ErrPhoneDeliveryFailed      = errors.New("no se pudo enviar el código")
	ErrPhoneAlreadyVerified     = errors.New("el número de seguridad ya está verificado")
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
	AvailableChannels   []PhoneChannel `json:"available_channels"`
	Configured          bool           `json:"configured"`
	Verified            bool           `json:"verified"`
	MaskedPhone         string         `json:"masked_phone"`
	VerifiedAt          *time.Time     `json:"verified_at"`
	PendingVerification bool           `json:"pending_verification"`
	ResendAvailableAt   *time.Time     `json:"resend_available_at"`
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

type PhoneChannel string

const (
	PhoneChannelSMS      PhoneChannel = "sms"
	PhoneChannelWhatsApp PhoneChannel = "whatsapp"
)

type PhoneCodeSender interface {
	Channels() []PhoneChannel
	SendPhoneCode(ctx context.Context, phoneE164, code string, channel PhoneChannel) error
}

// PhoneVerificationRepository manages persistence for phone verifications.
type PhoneVerificationRepository interface {
	GetPhoneVerification(ctx context.Context, accountID string) (*AccountPhoneVerification, error)
	ReservePhoneChallenge(ctx context.Context, record AccountPhoneVerification) error
	CancelPhoneChallenge(ctx context.Context, accountID, challengeHash string) error
	IncrementPhoneAttempts(ctx context.Context, accountID, challengeHash string, now time.Time) (int, error)
	MarkPhoneVerified(ctx context.Context, accountID, challengeHash string, verifiedAt time.Time) error
}

// PhoneVerificationService provides phone verification business logic.
type PhoneVerificationService struct {
	repo   PhoneVerificationRepository
	sender PhoneCodeSender
	now    func() time.Time
	otpKey []byte
}

func NewPhoneVerificationService(repo PhoneVerificationRepository, sender PhoneCodeSender, now func() time.Time, otpKey []byte) *PhoneVerificationService {
	if now == nil {
		now = time.Now
	}
	return &PhoneVerificationService{
		repo:   repo,
		sender: sender,
		now:    now,
		otpKey: slices.Clone(otpKey),
	}
}

func (s *PhoneVerificationService) deliveryChannels() []PhoneChannel {
	if s.sender == nil {
		return []PhoneChannel{}
	}
	return s.sender.Channels()
}

func (s *PhoneVerificationService) GetPhoneVerificationStatus(ctx context.Context, accountID string) (PhoneVerificationStatus, error) {
	record, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return PhoneVerificationStatus{}, err
	}
	if record == nil {
		return PhoneVerificationStatus{
			AvailableChannels:   s.deliveryChannels(),
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
	isPending := !isVerified && record.OTPHash != nil && strings.Contains(*record.OTPHash, ":") &&
		record.OTPExpiresAt != nil && record.OTPExpiresAt.After(now) &&
		record.OTPAttempts < 5

	return PhoneVerificationStatus{
		AvailableChannels:   s.deliveryChannels(),
		Configured:          true,
		Verified:            isVerified,
		MaskedPhone:         MaskPhone(record.PhoneE164),
		VerifiedAt:          record.VerifiedAt,
		PendingVerification: isPending,
		ResendAvailableAt:   record.ResendAvailableAt,
	}, nil
}

func (s *PhoneVerificationService) SendPhoneVerificationCode(ctx context.Context, accountID, rawPhone string, channel PhoneChannel) (PhoneCodeSentResult, error) {
	if channel == "" {
		channel = PhoneChannelSMS
	}
	if !slices.Contains(s.deliveryChannels(), channel) {
		return PhoneCodeSentResult{}, ErrPhoneDeliveryUnavailable
	}

	now := s.now().UTC()
	record, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return PhoneCodeSentResult{}, err
	}
	if record != nil && record.VerifiedAt != nil {
		return PhoneCodeSentResult{}, ErrPhoneAlreadyVerified
	}
	if record != nil && record.ResendAvailableAt != nil && now.Before(*record.ResendAvailableAt) {
		return PhoneCodeSentResult{}, ErrPhoneCooldownActive
	}

	var phoneE164 string
	if rawPhone == "" && record != nil {
		phoneE164 = record.PhoneE164
	} else {
		phoneE164, err = ValidateAndNormalizeChileanPhone(rawPhone)
		if err != nil {
			return PhoneCodeSentResult{}, err
		}
	}
	otp, err := generateOTP6()
	if err != nil {
		return PhoneCodeSentResult{}, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return PhoneCodeSentResult{}, err
	}

	hash := s.phoneCodeHash(hex.EncodeToString(salt), otp)
	expiresAt := now.Add(10 * time.Minute)
	resendAvailableAt := now.Add(time.Minute)
	newRecord := AccountPhoneVerification{
		AccountID:         accountID,
		PhoneE164:         phoneE164,
		OTPHash:           &hash,
		OTPExpiresAt:      &expiresAt,
		ResendAvailableAt: &resendAvailableAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.ReservePhoneChallenge(ctx, newRecord); err != nil {
		return PhoneCodeSentResult{}, err
	}
	if err := s.sender.SendPhoneCode(ctx, phoneE164, otp, channel); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if cleanupErr := s.repo.CancelPhoneChallenge(cleanupCtx, accountID, hash); cleanupErr != nil {
			safelog.Warn(ctx, "phone challenge cleanup failed")
		}
		return PhoneCodeSentResult{}, ErrPhoneDeliveryFailed
	}

	return PhoneCodeSentResult{
		Status:            "code_sent",
		MaskedPhone:       MaskPhone(phoneE164),
		ResendAvailableAt: resendAvailableAt,
		ExpiresAt:         expiresAt,
	}, nil
}

// Salt identifies the individual challenge, including when two codes happen to match.
func (s *PhoneVerificationService) phoneCodeHash(salt, code string) string {
	mac := hmac.New(sha256.New, s.otpKey)
	_, _ = mac.Write([]byte(salt + ":" + code))
	return salt + ":" + hex.EncodeToString(mac.Sum(nil))
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

	if record.OTPExpiresAt == nil || !now.Before(*record.OTPExpiresAt) {
		return PhoneVerifyResult{}, ErrPhoneCodeExpired
	}

	salt, _, hasSalt := strings.Cut(*record.OTPHash, ":")
	if !hasSalt {
		return PhoneVerifyResult{}, ErrPhoneNotPending
	}
	codeHash := s.phoneCodeHash(salt, code)
	if subtle.ConstantTimeCompare([]byte(*record.OTPHash), []byte(codeHash)) != 1 {
		attempts, err := s.repo.IncrementPhoneAttempts(ctx, accountID, *record.OTPHash, now)
		if err != nil {
			return PhoneVerifyResult{}, err
		}
		if attempts >= 5 {
			return PhoneVerifyResult{}, ErrPhoneMaxAttempts
		}
		return PhoneVerifyResult{}, ErrPhoneCodeInvalid
	}

	if err := s.repo.MarkPhoneVerified(ctx, accountID, *record.OTPHash, now); err != nil {
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
