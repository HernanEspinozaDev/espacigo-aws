package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
)

var (
	ErrPasskeyNotFound           = errors.New("llave de acceso no encontrada")
	ErrPasskeyChallengeExpired   = errors.New("desafío webauthn expirado")
	ErrPasskeyChallengeConsumed  = errors.New("desafío webauthn ya utilizado")
	ErrPasskeyChallengeInvalid   = errors.New("desafío webauthn inválido")
	ErrPasskeyInvalidClientData  = errors.New("clientDataJSON inválido")
	ErrPasskeyInvalidPublicKey   = errors.New("clave pública webauthn inválida")
	ErrPasskeyInvalidCredential  = errors.New("identificador de credencial inválido")
	ErrPasskeyCredentialConflict = errors.New("la llave de acceso ya se encuentra registrada")
)

// Passkey represents a public WebAuthn / FIDO2 credential.
type Passkey struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Transports []string   `json:"transports"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// PasskeyListResponse is the payload returned by GET /api/v1/account/security/passkeys.
type PasskeyListResponse struct {
	Passkeys []Passkey `json:"passkeys"`
}

// PasskeyRP represents the Relying Party entity.
type PasskeyRP struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// PasskeyUser represents the user entity in WebAuthn registration.
type PasskeyUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// PasskeyPubKeyCredParam represents an accepted public key algorithm.
type PasskeyPubKeyCredParam struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

// PasskeyCredentialDescriptor identifies a credential to exclude or allow.
type PasskeyCredentialDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

// PasskeyRegistrationOptions contains the parameters for navigator.credentials.create().
type PasskeyRegistrationOptions struct {
	ChallengeID        string                        `json:"challenge_id"`
	Challenge          string                        `json:"challenge"`
	RP                 PasskeyRP                     `json:"rp"`
	User               PasskeyUser                   `json:"user"`
	PubKeyCredParams   []PasskeyPubKeyCredParam      `json:"pubKeyCredParams"`
	Timeout            int                           `json:"timeout"`
	ExcludeCredentials []PasskeyCredentialDescriptor `json:"excludeCredentials"`
}

// PasskeyRegistrationVerifyInput is the payload sent to complete passkey registration.
type PasskeyRegistrationVerifyInput struct {
	ChallengeID       string   `json:"challenge_id"`
	Label             string   `json:"label,omitempty"`
	CredentialID      string   `json:"credential_id"`
	PublicKey         string   `json:"public_key"`
	ClientDataJSON    string   `json:"client_data_json"`
	AttestationObject string   `json:"attestation_object,omitempty"`
	Transports        []string `json:"transports,omitempty"`
	Algorithm         int      `json:"algorithm,omitempty"`
}

// PasskeyDeleteResponse is returned when a passkey is deleted.
type PasskeyDeleteResponse struct {
	Status    string `json:"status"`
	PasskeyID string `json:"passkey_id"`
}

// PasskeyRecord represents the storage entity in public.cuenta_passkey.
type PasskeyRecord struct {
	ID           string
	AccountID    string
	CredentialID string
	PublicKey    []byte
	Algorithm    int
	SignCount    int64
	Label        string
	Transports   []string
	LastUsedAt   *time.Time
	CreatedAt    time.Time
}

// WebAuthnChallengeRecord represents the storage entity in public.cuenta_webauthn_challenge.
type WebAuthnChallengeRecord struct {
	ID         string
	AccountID  string
	Challenge  string
	Purpose    string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// PasskeyRepository defines persistence operations for WebAuthn passkeys and challenges.
type PasskeyRepository interface {
	ListPasskeys(ctx context.Context, accountID string) ([]PasskeyRecord, error)
	SaveWebAuthnChallenge(ctx context.Context, challenge WebAuthnChallengeRecord) error
	GetWebAuthnChallenge(ctx context.Context, challengeID string) (*WebAuthnChallengeRecord, error)
	SavePasskeyAndConsumeChallenge(ctx context.Context, passkey PasskeyRecord, challengeID string, consumedAt time.Time) error
	DeletePasskey(ctx context.Context, accountID, passkeyID string) error
	GetAccountDisplayName(ctx context.Context, accountID string) (string, error)
}

// ListPasskeys returns all passkeys registered for the account, ordered by created_at DESC.
func (s *AuthenticationService) ListPasskeys(ctx context.Context, accountID string) (PasskeyListResponse, error) {
	if accountID == "" {
		return PasskeyListResponse{}, ErrNotFound
	}
	records, err := s.repo.ListPasskeys(ctx, accountID)
	if err != nil {
		return PasskeyListResponse{}, err
	}
	passkeys := make([]Passkey, 0, len(records))
	for _, r := range records {
		transports := r.Transports
		if transports == nil {
			transports = []string{}
		}
		passkeys = append(passkeys, Passkey{
			ID:         r.ID,
			Label:      r.Label,
			Transports: transports,
			CreatedAt:  r.CreatedAt.UTC(),
			LastUsedAt: r.LastUsedAt,
		})
	}
	return PasskeyListResponse{Passkeys: passkeys}, nil
}

// BeginPasskeyRegistration initiates WebAuthn credential registration.
func (s *AuthenticationService) BeginPasskeyRegistration(ctx context.Context, accountID, rpID string) (PasskeyRegistrationOptions, error) {
	if accountID == "" {
		return PasskeyRegistrationOptions{}, ErrNotFound
	}

	email, err := s.repo.ActiveAccountEmail(ctx, accountID)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	maskedEmail := MaskEmail(email)
	displayName, _ := s.repo.GetAccountDisplayName(ctx, accountID)
	if strings.TrimSpace(displayName) == "" {
		displayName = maskedEmail
	}

	// Generate 32 cryptographically random bytes for challenge
	challengeBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, challengeBytes); err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes)

	challengeID, err := s.credentials.ID()
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	now := s.now().UTC()
	challengeRec := WebAuthnChallengeRecord{
		ID:        challengeID,
		AccountID: accountID,
		Challenge: challenge,
		Purpose:   "register",
		ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now,
	}

	if err := s.repo.SaveWebAuthnChallenge(ctx, challengeRec); err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	// Fetch existing passkeys to populate excludeCredentials
	existing, err := s.repo.ListPasskeys(ctx, accountID)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	excludeCredentials := make([]PasskeyCredentialDescriptor, 0, len(existing))
	for _, pk := range existing {
		excludeCredentials = append(excludeCredentials, PasskeyCredentialDescriptor{
			Type:       "public-key",
			ID:         pk.CredentialID,
			Transports: pk.Transports,
		})
	}

	return PasskeyRegistrationOptions{
		ChallengeID: challengeID,
		Challenge:   challenge,
		RP: PasskeyRP{
			ID:   rpID,
			Name: "EspaciGo",
		},
		User: PasskeyUser{
			ID:          accountID,
			Name:        maskedEmail,
			DisplayName: displayName,
		},
		PubKeyCredParams: []PasskeyPubKeyCredParam{
			{Type: "public-key", Alg: -7},   // ES256
			{Type: "public-key", Alg: -257}, // RS256
		},
		Timeout:            60000,
		ExcludeCredentials: excludeCredentials,
	}, nil
}

// CompletePasskeyRegistration validates the client response, saves the passkey, consumes the challenge, and records an audit alert.
func (s *AuthenticationService) CompletePasskeyRegistration(
	ctx context.Context,
	accountID string,
	input PasskeyRegistrationVerifyInput,
	allowedOrigins []string,
	deviceLabel, ipMasked string,
) (Passkey, error) {
	if accountID == "" {
		return Passkey{}, ErrNotFound
	}
	if strings.TrimSpace(input.ChallengeID) == "" ||
		strings.TrimSpace(input.CredentialID) == "" ||
		strings.TrimSpace(input.PublicKey) == "" ||
		strings.TrimSpace(input.ClientDataJSON) == "" {
		return Passkey{}, ErrInvalid
	}

	challenge, err := s.repo.GetWebAuthnChallenge(ctx, input.ChallengeID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Passkey{}, ErrPasskeyChallengeInvalid
		}
		return Passkey{}, err
	}

	if challenge.AccountID != accountID || challenge.Purpose != "register" {
		return Passkey{}, ErrPasskeyChallengeInvalid
	}
	if challenge.ConsumedAt != nil {
		return Passkey{}, ErrPasskeyChallengeConsumed
	}
	now := s.now().UTC()
	if now.After(challenge.ExpiresAt) {
		return Passkey{}, ErrPasskeyChallengeExpired
	}

	clientDataBytes, err := decodeClientDataJSON(input.ClientDataJSON)
	if err != nil {
		return Passkey{}, ErrPasskeyInvalidClientData
	}

	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(clientDataBytes, &clientData); err != nil {
		return Passkey{}, ErrPasskeyInvalidClientData
	}

	if clientData.Type != "webauthn.create" {
		return Passkey{}, ErrPasskeyInvalidClientData
	}

	// Match challenge (stripping optional base64url padding)
	normClientChallenge := strings.TrimRight(clientData.Challenge, "=")
	normStoredChallenge := strings.TrimRight(challenge.Challenge, "=")
	if normClientChallenge != normStoredChallenge {
		return Passkey{}, ErrPasskeyChallengeInvalid
	}

	if !isValidWebAuthnOrigin(clientData.Origin, allowedOrigins) {
		return Passkey{}, ErrPasskeyInvalidClientData
	}

	pubKeyBytes, err := decodePublicKey(input.PublicKey)
	if err != nil || len(pubKeyBytes) == 0 {
		return Passkey{}, ErrPasskeyInvalidPublicKey
	}

	alg := input.Algorithm
	if alg == 0 {
		alg = -7 // Default ES256
	}

	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "Llave de acceso"
	}
	if len(label) > 100 {
		label = label[:100]
	}

	transports := input.Transports
	if transports == nil {
		transports = []string{}
	}

	passkeyID, err := s.credentials.ID()
	if err != nil {
		return Passkey{}, err
	}

	passkeyRec := PasskeyRecord{
		ID:           passkeyID,
		AccountID:    accountID,
		CredentialID: strings.TrimSpace(input.CredentialID),
		PublicKey:    pubKeyBytes,
		Algorithm:    alg,
		SignCount:    0,
		Label:        label,
		Transports:   transports,
		CreatedAt:    now,
	}

	if err := s.repo.SavePasskeyAndConsumeChallenge(ctx, passkeyRec, challenge.ID, now); err != nil {
		return Passkey{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "passkey_registered", "Llave de acceso registrada", deviceLabel, ipMasked)

	return Passkey{
		ID:         passkeyID,
		Label:      label,
		Transports: transports,
		CreatedAt:  now,
	}, nil
}

// DeletePasskey removes the passkey belonging to accountID and records an audit alert.
func (s *AuthenticationService) DeletePasskey(ctx context.Context, accountID, passkeyID, deviceLabel, ipMasked string) error {
	if accountID == "" || strings.TrimSpace(passkeyID) == "" {
		return ErrInvalid
	}
	if err := s.repo.DeletePasskey(ctx, accountID, passkeyID); err != nil {
		return err
	}
	_ = s.RecordSecurityAlertEvent(ctx, accountID, "passkey_revoked", "Llave de acceso eliminada", deviceLabel, ipMasked)
	return nil
}

func decodeClientDataJSON(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	return nil, ErrPasskeyInvalidClientData
}

func decodePublicKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, ErrPasskeyInvalidPublicKey
	}
	if b, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := hex.DecodeString(trimmed); err == nil && len(b) > 0 {
		return b, nil
	}
	return []byte(trimmed), nil
}

func isValidWebAuthnOrigin(clientOrigin string, allowedOrigins []string) bool {
	clientOrigin = strings.TrimSpace(clientOrigin)
	if clientOrigin == "" {
		return false
	}
	u, err := url.Parse(clientOrigin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	for _, ao := range allowedOrigins {
		ao = strings.TrimSpace(ao)
		if ao == "" {
			continue
		}
		if strings.EqualFold(clientOrigin, ao) {
			return true
		}
		if aou, err := url.Parse(ao); err == nil {
			if strings.EqualFold(u.Scheme, aou.Scheme) && strings.EqualFold(u.Host, aou.Host) {
				return true
			}
		}
	}

	hostname := u.Hostname()
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || strings.HasSuffix(hostname, ".localhost") {
		return true
	}

	return false
}
