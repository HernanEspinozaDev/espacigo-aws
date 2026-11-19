package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrTOTPNotPending        = errors.New("no hay configuración de totp pendiente")
	ErrTOTPAlreadyEnabled    = errors.New("el autenticador totp ya está activado")
	ErrTOTPNotEnabled        = errors.New("el autenticador totp no está activado")
	ErrTOTPInvalidCode       = errors.New("código de verificación inválido")
	ErrTOTPReplayDetected    = errors.New("código totp ya utilizado")
	ErrTOTPInvalidBackupCode = errors.New("código de respaldo inválido o ya utilizado")
	ErrTOTPKeyringMissing    = errors.New("llavero de cifrado totp no configurado")
)

const (
	totpStepSeconds = 30
	totpDigits      = 6
	totpIssuer      = "EspaciGo"
	backupCodeCount = 8
	backupAlphabet  = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// AccountTOTPStatus represents the public state of TOTP for the account.
type AccountTOTPStatus struct {
	Enabled              bool       `json:"enabled"`
	PendingSetup         bool       `json:"pending_setup"`
	EnabledAt            *time.Time `json:"enabled_at,omitempty"`
	RemainingBackupCodes int        `json:"remaining_backup_codes"`
}

// AccountTOTPSetupResponse is returned when initiating TOTP configuration.
type AccountTOTPSetupResponse struct {
	OTPAuthURI string `json:"otpauth_uri"`
	ManualKey  string `json:"manual_key"`
}

// AccountTOTPEnableInput is the payload to confirm setup and activate TOTP.
type AccountTOTPEnableInput struct {
	Code string `json:"code"`
}

// AccountTOTPEnableResponse is returned when TOTP is successfully enabled.
type AccountTOTPEnableResponse struct {
	Enabled     bool      `json:"enabled"`
	EnabledAt   time.Time `json:"enabled_at"`
	BackupCodes []string  `json:"backup_codes"`
}

// AccountTOTPDisableInput is the payload to disable TOTP using a code or backup code.
type AccountTOTPDisableInput struct {
	Code string `json:"code"`
}

// AccountTOTPDisableResponse is returned when TOTP is successfully disabled.
type AccountTOTPDisableResponse struct {
	Enabled bool `json:"enabled"`
}

// AccountTOTPRecord is the storage model for public.cuenta_totp.
type AccountTOTPRecord struct {
	AccountID               string
	SecretCiphertext       []byte
	KeyVersion             *string
	PendingSecretCiphertext []byte
	PendingKeyVersion      *string
	EnabledAt              *time.Time
	LastVerifiedStep       *int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// TOTPRepository defines persistence operations for TOTP and backup codes.
type TOTPRepository interface {
	GetTOTP(ctx context.Context, accountID string) (*AccountTOTPRecord, int, error)
	SavePendingTOTPSecret(ctx context.Context, accountID string, ciphertext []byte, keyVersion string) error
	ActivateTOTP(ctx context.Context, accountID string, verifiedStep int64, backupCodeHashes []string) error
	UpdateTOTPLastVerifiedStep(ctx context.Context, accountID string, verifiedStep int64) error
	ConsumeBackupCode(ctx context.Context, accountID string, codeHash string) (bool, error)
	DisableTOTP(ctx context.Context, accountID string) error
}

// TOTPKeyring manages AES-256-GCM encryption keys for TOTP secrets.
type TOTPKeyring struct {
	current string
	keys    map[string][]byte
}

type totpFileConfig struct {
	Current string            `json:"current"`
	Keys    map[string]string `json:"keys"`
}

// NewTOTPKeyring creates a versioned keyring with validated 32-byte keys.
func NewTOTPKeyring(current string, keys map[string][]byte) (*TOTPKeyring, error) {
	if len(current) == 0 || len(keys) == 0 {
		return nil, errors.New("invalid totp keyring configuration")
	}
	copied := make(map[string][]byte, len(keys))
	for id, k := range keys {
		if len(id) == 0 || len(k) != 32 {
			return nil, errors.New("invalid totp keyring configuration: key must be 32 bytes")
		}
		copied[id] = append([]byte(nil), k...)
	}
	if _, ok := copied[current]; !ok {
		return nil, errors.New("invalid totp keyring configuration: current key missing")
	}
	return &TOTPKeyring{current: current, keys: copied}, nil
}

// LoadTOTPKeyringFromFile loads a keyring from a 0600-permission JSON file.
func LoadTOTPKeyringFromFile(path string) (*TOTPKeyring, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("totp keyring path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, errors.New("invalid totp keyring file permissions or type")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 64*1024 {
		return nil, errors.New("cannot read totp keyring file")
	}
	var config totpFileConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.New("invalid totp keyring json")
	}
	if config.Current == "" || len(config.Keys) == 0 {
		return nil, errors.New("empty totp keyring configuration")
	}
	keys := make(map[string][]byte, len(config.Keys))
	for id, hexStr := range config.Keys {
		raw, err := hex.DecodeString(strings.TrimSpace(hexStr))
		if err != nil || len(raw) != 32 {
			return nil, errors.New("invalid hex key in totp keyring")
		}
		keys[id] = raw
	}
	return NewTOTPKeyring(config.Current, keys)
}

// LoadTOTPKeyring loads the TOTP keyring using environment, secrets file, or fallback in development.
func LoadTOTPKeyring(getenv func(string) string) (*TOTPKeyring, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	path := strings.TrimSpace(getenv("TOTP_KEYRING_FILE"))
	if path == "" {
		path = strings.TrimSpace(getenv("LOCAL_TOTP_KEYRING_FILE"))
	}
	if path != "" {
		return LoadTOTPKeyringFromFile(path)
	}

	isProduction := getenv("APP_PROFILE") == "production"
	if isProduction {
		return nil, errors.New("totp keyring configuration is required in production")
	}

	// In development, check .local/secrets/totp-keyring.json
	localPath, err := filepath.Abs(".local/secrets/totp-keyring.json")
	if err == nil {
		if kr, err := LoadTOTPKeyringFromFile(localPath); err == nil {
			return kr, nil
		}
	}

	// Safe deterministic local fallback for desarrollo-local / test suite
	fallbackKey := sha256.Sum256([]byte("espacigo-local-totp-deterministic-secret-v1"))
	return NewTOTPKeyring("v1", map[string][]byte{"v1": fallbackKey[:]})
}

// Seal encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
func (k *TOTPKeyring) Seal(plaintext []byte) (string, []byte, error) {
	if len(plaintext) == 0 {
		return "", nil, errors.New("empty plaintext")
	}
	key, ok := k.keys[k.current]
	if !ok {
		return "", nil, errors.New("current key not found in keyring")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return k.current, ciphertext, nil
}

// Open decrypts ciphertext using the specified key version.
func (k *TOTPKeyring) Open(version string, ciphertext []byte) ([]byte, error) {
	key, ok := k.keys[version]
	if !ok {
		return nil, errors.New("unknown totp key version")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	sealed := ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, sealed, nil)
}

// GenerateTOTPSecret generates 20 cryptographically secure random bytes (160 bits).
func GenerateTOTPSecret() ([]byte, error) {
	secret := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// EncodeBase32Secret encodes secret to standard Base32 without padding.
func EncodeBase32Secret(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// DecodeBase32Secret decodes a Base32 string (handling whitespace and uppercase).
func DecodeBase32Secret(encoded string) ([]byte, error) {
	clean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(encoded), " ", ""))
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
}

// BuildOTPAuthURI constructs an RFC 6238 compatible otpauth:// URI.
func BuildOTPAuthURI(accountEmail, manualKey string) string {
	label := fmt.Sprintf("%s:%s", totpIssuer, accountEmail)
	v := url.Values{}
	v.Set("secret", manualKey)
	v.Set("issuer", totpIssuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s?%s", url.PathEscape(label), v.Encode())
}

// GenerateTOTPCode generates an RFC 6238 6-digit TOTP code for a given step.
func GenerateTOTPCode(secret []byte, step int64, digits int) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	code := binCode % mod
	return fmt.Sprintf("%0*d", digits, code)
}

// ValidateTOTP verifies a code against the secret within a ±1 step window (30s)
// and enforces anti-replay via lastVerifiedStep.
func ValidateTOTP(secret []byte, code string, t time.Time, lastVerifiedStep *int64) (int64, error) {
	trimmed := strings.TrimSpace(code)
	if len(trimmed) != totpDigits {
		return 0, ErrTOTPInvalidCode
	}
	for _, c := range trimmed {
		if c < '0' || c > '9' {
			return 0, ErrTOTPInvalidCode
		}
	}

	currentStep := t.Unix() / totpStepSeconds
	steps := []int64{currentStep - 1, currentStep, currentStep + 1}

	var matchedStep int64
	var found bool

	for _, step := range steps {
		expected := GenerateTOTPCode(secret, step, totpDigits)
		if hmac.Equal([]byte(expected), []byte(trimmed)) {
			matchedStep = step
			found = true
			break
		}
	}

	if !found {
		return 0, ErrTOTPInvalidCode
	}

	// Anti-replay: candidate step must be strictly greater than lastVerifiedStep
	if lastVerifiedStep != nil && matchedStep <= *lastVerifiedStep {
		return 0, ErrTOTPReplayDetected
	}

	return matchedStep, nil
}

// GenerateBackupCodes generates n backup codes with format XXXX-XXXX and their SHA-256 hashes.
func GenerateBackupCodes(count int) (rawCodes []string, hashes []string, err error) {
	if count <= 0 {
		count = backupCodeCount
	}
	rawCodes = make([]string, count)
	hashes = make([]string, count)
	alphaLen := byte(len(backupAlphabet))

	for i := 0; i < count; i++ {
		buf := make([]byte, 8)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return nil, nil, err
		}
		var sb strings.Builder
		for j := 0; j < 8; j++ {
			if j == 4 {
				sb.WriteByte('-')
			}
			idx := buf[j] % alphaLen
			sb.WriteByte(backupAlphabet[idx])
		}
		raw := sb.String()
		rawCodes[i] = raw
		hashes[i] = HashBackupCode(raw)
	}
	return rawCodes, hashes, nil
}

// NormalizeBackupCode normalizes user input to XXXX-XXXX format.
func NormalizeBackupCode(code string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(code))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	if len(cleaned) == 8 {
		return cleaned[:4] + "-" + cleaned[4:]
	}
	return cleaned
}

// HashBackupCode calculates SHA-256 hex string of a normalized backup code.
func HashBackupCode(code string) string {
	norm := NormalizeBackupCode(code)
	h := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(h[:])
}

func (s *AuthenticationService) getTOTPKeyring() (*TOTPKeyring, error) {
	if s.totpKeyring != nil {
		return s.totpKeyring, nil
	}
	kr, err := LoadTOTPKeyring(os.Getenv)
	if err != nil {
		return nil, ErrTOTPKeyringMissing
	}
	s.totpKeyring = kr
	return s.totpKeyring, nil
}

// GetTOTPStatus returns the current TOTP enablement and pending setup state.
func (s *AuthenticationService) GetTOTPStatus(ctx context.Context, accountID string) (AccountTOTPStatus, error) {
	if accountID == "" {
		return AccountTOTPStatus{}, ErrNotFound
	}
	rec, remainingCodes, err := s.repo.GetTOTP(ctx, accountID)
	if err != nil {
		return AccountTOTPStatus{}, err
	}
	if rec == nil {
		return AccountTOTPStatus{
			Enabled:              false,
			PendingSetup:         false,
			RemainingBackupCodes: 0,
		}, nil
	}

	enabled := rec.EnabledAt != nil && len(rec.SecretCiphertext) > 0
	pending := len(rec.PendingSecretCiphertext) > 0

	return AccountTOTPStatus{
		Enabled:              enabled,
		PendingSetup:         pending,
		EnabledAt:            rec.EnabledAt,
		RemainingBackupCodes: remainingCodes,
	}, nil
}

// SetupTOTP initiates TOTP enrollment by generating a pending secret and otpauth URI.
func (s *AuthenticationService) SetupTOTP(ctx context.Context, accountID string) (AccountTOTPSetupResponse, error) {
	if accountID == "" {
		return AccountTOTPSetupResponse{}, ErrNotFound
	}
	email, err := s.repo.ActiveAccountEmail(ctx, accountID)
	if err != nil {
		return AccountTOTPSetupResponse{}, err
	}

	kr, err := s.getTOTPKeyring()
	if err != nil {
		return AccountTOTPSetupResponse{}, err
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return AccountTOTPSetupResponse{}, err
	}

	version, ciphertext, err := kr.Seal(secret)
	if err != nil {
		return AccountTOTPSetupResponse{}, err
	}

	if err := s.repo.SavePendingTOTPSecret(ctx, accountID, ciphertext, version); err != nil {
		return AccountTOTPSetupResponse{}, err
	}

	manualKey := EncodeBase32Secret(secret)
	otpAuthURI := BuildOTPAuthURI(email, manualKey)

	return AccountTOTPSetupResponse{
		OTPAuthURI: otpAuthURI,
		ManualKey:  manualKey,
	}, nil
}

// EnableTOTP verifies the first code against the pending secret and enables TOTP.
func (s *AuthenticationService) EnableTOTP(ctx context.Context, accountID string, code string, deviceLabel, ipMasked string) (AccountTOTPEnableResponse, error) {
	if accountID == "" {
		return AccountTOTPEnableResponse{}, ErrNotFound
	}
	rec, _, err := s.repo.GetTOTP(ctx, accountID)
	if err != nil {
		return AccountTOTPEnableResponse{}, err
	}
	if rec == nil || len(rec.PendingSecretCiphertext) == 0 || rec.PendingKeyVersion == nil {
		return AccountTOTPEnableResponse{}, ErrTOTPNotPending
	}

	kr, err := s.getTOTPKeyring()
	if err != nil {
		return AccountTOTPEnableResponse{}, err
	}

	secret, err := kr.Open(*rec.PendingKeyVersion, rec.PendingSecretCiphertext)
	if err != nil {
		return AccountTOTPEnableResponse{}, ErrTOTPInvalidCode
	}

	step, err := ValidateTOTP(secret, code, s.now(), rec.LastVerifiedStep)
	if err != nil {
		return AccountTOTPEnableResponse{}, err
	}

	backupCodes, hashes, err := GenerateBackupCodes(backupCodeCount)
	if err != nil {
		return AccountTOTPEnableResponse{}, err
	}

	if err := s.repo.ActivateTOTP(ctx, accountID, step, hashes); err != nil {
		return AccountTOTPEnableResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "totp_enabled", "Autenticador TOTP activado", deviceLabel, ipMasked)

	return AccountTOTPEnableResponse{
		Enabled:     true,
		EnabledAt:   s.now().UTC(),
		BackupCodes: backupCodes,
	}, nil
}

// DisableTOTP verifies a valid TOTP code or backup code and deactivates TOTP.
func (s *AuthenticationService) DisableTOTP(ctx context.Context, accountID string, code string, deviceLabel, ipMasked string) (AccountTOTPDisableResponse, error) {
	if accountID == "" {
		return AccountTOTPDisableResponse{}, ErrNotFound
	}
	rec, _, err := s.repo.GetTOTP(ctx, accountID)
	if err != nil {
		return AccountTOTPDisableResponse{}, err
	}
	if rec == nil || rec.EnabledAt == nil || len(rec.SecretCiphertext) == 0 || rec.KeyVersion == nil {
		return AccountTOTPDisableResponse{}, ErrTOTPNotEnabled
	}

	kr, err := s.getTOTPKeyring()
	if err != nil {
		return AccountTOTPDisableResponse{}, err
	}

	trimmed := strings.TrimSpace(code)
	var verified bool

	// Check if code is 6-digit TOTP
	if len(trimmed) == totpDigits {
		secret, err := kr.Open(*rec.KeyVersion, rec.SecretCiphertext)
		if err == nil {
			step, err := ValidateTOTP(secret, trimmed, s.now(), rec.LastVerifiedStep)
			if err == nil {
				verified = true
				_ = s.repo.UpdateTOTPLastVerifiedStep(ctx, accountID, step)
			}
		}
	}

	// If not verified as TOTP, try backup code
	if !verified {
		hash := HashBackupCode(trimmed)
		consumed, err := s.repo.ConsumeBackupCode(ctx, accountID, hash)
		if err == nil && consumed {
			verified = true
		}
	}

	if !verified {
		return AccountTOTPDisableResponse{}, ErrTOTPInvalidCode
	}

	if err := s.repo.DisableTOTP(ctx, accountID); err != nil {
		return AccountTOTPDisableResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "totp_disabled", "Autenticador TOTP desactivado", deviceLabel, ipMasked)

	return AccountTOTPDisableResponse{
		Enabled: false,
	}, nil
}

