package marketplaceoauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("marketplace oauth: not found")
	ErrInvalidState  = errors.New("marketplace oauth: invalid state")
	ErrNotConfigured = errors.New("marketplace oauth: not configured")
	ErrProvider      = errors.New("marketplace oauth: provider failure")
	ErrWrongMode     = errors.New("marketplace oauth: credential mode mismatch")
	ErrUserDenied    = errors.New("marketplace oauth: seller denied authorization")
)

type Config struct {
	ClientID, ClientSecret, RedirectURI string
	EncryptionKey                       []byte
	TestMode                            bool
	AuthorizationURL, TokenURL          string
	StateTTL, RefreshBefore             time.Duration
}

func (c Config) validate() error {
	u, err := url.Parse(c.RedirectURI)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("OAuth redirect URI must be an absolute HTTPS URL")
	}
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" || len(c.EncryptionKey) != 32 {
		return ErrNotConfigured
	}
	return nil
}

type State struct {
	HostID            string
	EncryptedVerifier []byte
}
type Connection struct {
	HostID, SellerID          string
	PublicKey                 string // non-secret seller key, exposed only to the authorized renter in test tokenization mode
	AccessToken, RefreshToken []byte // encrypted at rest
	ExpiresAt                 time.Time
	Scopes                    []string
	LiveMode                  bool
	NeedsReauthorization      bool
}

type Repository interface {
	SaveState(context.Context, string, string, []byte, time.Time) error
	ConsumeState(context.Context, string, time.Time) (State, error)
	SaveConnection(context.Context, Connection) error
	GetConnection(context.Context, string) (Connection, error)
	RefreshConnection(context.Context, string, func(Connection) (Connection, error)) (Connection, error)
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	PublicKey    string `json:"public_key"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	LiveMode     *bool  `json:"live_mode"`
}

// Mercado Pago examples encode user_id as a JSON number, while some account
// responses encode it as a string. Normalize both without accepting fractions.
func (t *TokenResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		AccessToken  string          `json:"access_token"`
		PublicKey    string          `json:"public_key"`
		RefreshToken string          `json:"refresh_token"`
		UserID       json.RawMessage `json:"user_id"`
		ExpiresIn    int64           `json:"expires_in"`
		Scope        string          `json:"scope"`
		LiveMode     *bool           `json:"live_mode"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var userID string
	if len(wire.UserID) > 0 && wire.UserID[0] == '"' {
		if err := json.Unmarshal(wire.UserID, &userID); err != nil {
			return err
		}
	} else {
		userID = string(wire.UserID)
	}
	*t = TokenResponse{AccessToken: wire.AccessToken, PublicKey: wire.PublicKey, RefreshToken: wire.RefreshToken, UserID: userID, ExpiresIn: wire.ExpiresIn, Scope: wire.Scope, LiveMode: wire.LiveMode}
	return nil
}

type TokenExchanger interface {
	Exchange(context.Context, string, string, bool) (TokenResponse, error)
	Refresh(context.Context, string, bool) (TokenResponse, error)
}

type Service struct {
	config    Config
	repo      Repository
	exchanger TokenExchanger
	now       func() time.Time
	random    io.Reader
}

func New(config Config, repo Repository, exchanger TokenExchanger, now func() time.Time) (*Service, error) {
	if repo == nil || exchanger == nil {
		return nil, errors.New("marketplace oauth dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	if config.StateTTL == 0 {
		config.StateTTL = 10 * time.Minute
	}
	if config.RefreshBefore == 0 {
		config.RefreshBefore = 5 * time.Minute
	}
	if config.AuthorizationURL == "" {
		config.AuthorizationURL = "https://auth.mercadopago.cl/authorization"
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Service{config: config, repo: repo, exchanger: exchanger, now: now, random: rand.Reader}, nil
}

func (s *Service) Start(ctx context.Context, hostID string) (string, error) {
	if err := s.config.validate(); err != nil {
		return "", err
	}
	stateBytes := make([]byte, 32)
	verifierBytes := make([]byte, 32)
	if _, err := io.ReadFull(s.random, stateBytes); err != nil {
		return "", err
	}
	if _, err := io.ReadFull(s.random, verifierBytes); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	hash := sha256.Sum256([]byte(state))
	encrypted, err := s.seal([]byte(verifier))
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if err = s.repo.SaveState(ctx, hex.EncodeToString(hash[:]), hostID, encrypted, now.Add(s.config.StateTTL)); err != nil {
		return "", err
	}
	challengeHash := sha256.Sum256([]byte(verifier))
	params := url.Values{}
	params.Set("client_id", s.config.ClientID)
	params.Set("response_type", "code")
	params.Set("platform_id", "mp")
	params.Set("redirect_uri", s.config.RedirectURI)
	params.Set("state", state)
	params.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challengeHash[:]))
	params.Set("code_challenge_method", "S256")
	return s.config.AuthorizationURL + "?" + params.Encode(), nil
}

func (s *Service) Complete(ctx context.Context, code, state string) (string, error) {
	if len(state) < 32 || len(state) > 128 {
		return "", ErrInvalidState
	}
	hash := sha256.Sum256([]byte(state))
	record, err := s.repo.ConsumeState(ctx, hex.EncodeToString(hash[:]), s.now().UTC())
	if err != nil {
		return "", ErrInvalidState
	}
	if strings.TrimSpace(code) == "" {
		return record.HostID, ErrUserDenied
	}
	verifierBytes, err := s.open(record.EncryptedVerifier)
	if err != nil {
		return "", ErrInvalidState
	}
	defer clear(verifierBytes)
	tokens, err := s.exchanger.Exchange(ctx, code, string(verifierBytes), s.config.TestMode)
	if err != nil {
		return record.HostID, CallbackFailure{Stage: "exchange", Reason: safeProviderReason(err), Cause: err}
	}
	if err = validateTokens(tokens, s.config.TestMode); err != nil {
		reason := "invalid_token_response"
		var tokenFailure TokenValidationFailure
		if errors.As(err, &tokenFailure) {
			reason = tokenFailure.Reason
		}
		if errors.Is(err, ErrWrongMode) {
			reason = "mode_mismatch"
		}
		return record.HostID, CallbackFailure{Stage: "validation", Reason: reason, Cause: err}
	}
	tokens = normalizeVerifiedSandboxLiveMode(tokens, s.config.TestMode)
	access, err := s.seal([]byte(tokens.AccessToken))
	if err != nil {
		return record.HostID, CallbackFailure{Stage: "storage", Reason: "encryption_failed", Cause: err}
	}
	refresh, err := s.seal([]byte(tokens.RefreshToken))
	if err != nil {
		return record.HostID, CallbackFailure{Stage: "storage", Reason: "encryption_failed", Cause: err}
	}
	connection := Connection{HostID: record.HostID, SellerID: tokens.UserID, PublicKey: validPublicKey(tokens.PublicKey), AccessToken: access, RefreshToken: refresh, ExpiresAt: s.now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second), Scopes: strings.Fields(tokens.Scope), LiveMode: *tokens.LiveMode}
	if err = s.repo.SaveConnection(ctx, connection); err != nil {
		return record.HostID, CallbackFailure{Stage: "storage", Reason: "connection_save_failed", Cause: err}
	}
	return record.HostID, nil
}

// CallbackFailure is safe for structured operational diagnostics. Its fields
// are fixed enums; it never includes OAuth parameters, provider body, or tokens.
type CallbackFailure struct {
	Stage, Reason string
	Cause         error
}

func (e CallbackFailure) Error() string { return "marketplace oauth: callback failure" }
func (e CallbackFailure) Unwrap() error { return e.Cause }

func safeProviderReason(err error) string {
	var failure ProviderFailure
	if errors.As(err, &failure) {
		if failure.Code == "transport_error" {
			return failure.Code
		}
		code := allowlistedProviderCode(failure.Code)
		status := failure.Status
		if status < 100 || status > 599 {
			status = 0
		}
		return fmt.Sprintf("http_%d:%s", status, code)
	}
	return "provider_rejected"
}

// TokenValidationFailure carries only a fixed field-level reason for safe
// diagnostics. It never contains a response value.
type TokenValidationFailure struct{ Reason string }

func (TokenValidationFailure) Error() string { return "marketplace oauth: invalid token response" }
func (TokenValidationFailure) Unwrap() error { return ErrProvider }

func validateTokens(t TokenResponse, testMode bool) error {
	if t.AccessToken == "" {
		return TokenValidationFailure{Reason: "missing_access_token"}
	}
	if t.RefreshToken == "" {
		return TokenValidationFailure{Reason: "missing_refresh_token"}
	}
	if t.UserID == "" {
		return TokenValidationFailure{Reason: "missing_user_id"}
	}
	if t.ExpiresIn <= 0 {
		return TokenValidationFailure{Reason: "invalid_expires_in"}
	}
	if t.ExpiresIn > int64((365*24*time.Hour)/time.Second) {
		return TokenValidationFailure{Reason: "expires_in_out_of_range"}
	}
	if _, err := strconvPositiveDecimal(t.UserID); err != nil {
		return TokenValidationFailure{Reason: "invalid_user_id"}
	}
	if t.LiveMode == nil {
		// Mercado Pago documents TEST tokens as sandbox-only. APP_USR is used
		// for both test and production credentials, so it is not evidence of mode.
		if testMode && strings.HasPrefix(t.AccessToken, "TEST-") {
			return nil
		}
		if testMode {
			return TokenValidationFailure{Reason: "sandbox_mode_unverified"}
		}
		return TokenValidationFailure{Reason: "missing_live_mode"}
	}
	if *t.LiveMode == testMode {
		return ErrWrongMode
	}
	return nil
}

func normalizeVerifiedSandboxLiveMode(tokens TokenResponse, testMode bool) TokenResponse {
	if testMode && tokens.LiveMode == nil && strings.HasPrefix(tokens.AccessToken, "TEST-") {
		liveMode := false
		tokens.LiveMode = &liveMode
	}
	return tokens
}

func strconvPositiveDecimal(s string) (uint64, error) {
	var n uint64
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not decimal")
		}
		n = n*10 + uint64(r-'0')
	}
	if n == 0 {
		return 0, errors.New("zero")
	}
	return n, nil
}

func (s *Service) Status(ctx context.Context, hostID string) (Connection, error) {
	c, err := s.repo.GetConnection(ctx, hostID)
	if err != nil {
		return Connection{}, err
	}
	if c.NeedsReauthorization {
		return c, nil
	}
	if c.LiveMode == s.config.TestMode {
		c.NeedsReauthorization = true
		return c, nil
	}
	if !s.now().Before(c.ExpiresAt.Add(-s.config.RefreshBefore)) {
		updated, refreshErr := s.Refresh(ctx, hostID)
		if refreshErr != nil && !updated.NeedsReauthorization {
			return Connection{}, refreshErr
		}
		return updated, nil
	}
	return c, nil
}

// AccessToken is the only token accessor for server-side checkout code.
func (s *Service) AccessToken(ctx context.Context, hostID string) ([]byte, error) {
	c, err := s.Status(ctx, hostID)
	if err != nil {
		return nil, err
	}
	if c.NeedsReauthorization {
		return nil, ErrProvider
	}
	return s.open(c.AccessToken)
}

// PublicKey returns only the active sandbox seller's browser-safe public key.
// It intentionally reports unavailable for legacy connections and accounts
// requiring reauthorization, so checkout code cannot silently use another key.
func (s *Service) PublicKey(ctx context.Context, hostID string) (string, bool, error) {
	c, err := s.Status(ctx, hostID)
	if errors.Is(err, ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if c.NeedsReauthorization || c.LiveMode || c.PublicKey == "" {
		return "", false, nil
	}
	return c.PublicKey, true, nil
}

func (s *Service) Refresh(ctx context.Context, hostID string) (Connection, error) {
	return s.repo.RefreshConnection(ctx, hostID, func(current Connection) (Connection, error) {
		if current.NeedsReauthorization {
			return current, ErrProvider
		}
		refresh, err := s.open(current.RefreshToken)
		if err != nil {
			return current, err
		}
		defer clear(refresh)
		updated, err := s.exchanger.Refresh(ctx, string(refresh), s.config.TestMode)
		if err != nil {
			current.NeedsReauthorization = true
			return current, ErrProvider
		}
		if err = validateTokens(updated, s.config.TestMode); err != nil {
			current.NeedsReauthorization = true
			return current, err
		}
		updated = normalizeVerifiedSandboxLiveMode(updated, s.config.TestMode)
		if updated.UserID != current.SellerID {
			current.NeedsReauthorization = true
			return current, ErrProvider
		}
		access, err := s.seal([]byte(updated.AccessToken))
		if err != nil {
			return current, err
		}
		newRefresh, err := s.seal([]byte(updated.RefreshToken))
		if err != nil {
			return current, err
		}
		current.SellerID = updated.UserID
		current.AccessToken = access
		current.RefreshToken = newRefresh
		current.ExpiresAt = s.now().UTC().Add(time.Duration(updated.ExpiresIn) * time.Second)
		current.Scopes = strings.Fields(updated.Scope)
		current.LiveMode = *updated.LiveMode
		if key := validPublicKey(updated.PublicKey); key != "" {
			current.PublicKey = key
		}
		current.NeedsReauthorization = false
		return current, nil
	})
}

func validPublicKey(key string) string {
	if len(key) < 20 || len(key) > 128 || strings.TrimSpace(key) != key || !(strings.HasPrefix(key, "TEST-") || strings.HasPrefix(key, "APP_USR-")) {
		return ""
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return ""
		}
	}
	return key
}

func (s *Service) seal(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(s.random, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, []byte("espacigo:mercadopago-oauth:v1")), nil
}
func (s *Service) open(value []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(value) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("invalid encrypted OAuth value")
	}
	return gcm.Open(nil, value[:gcm.NonceSize()], value[gcm.NonceSize():], []byte("espacigo:mercadopago-oauth:v1"))
}

// Constant-time comparison is exposed for mode and test assertions without revealing token material.
