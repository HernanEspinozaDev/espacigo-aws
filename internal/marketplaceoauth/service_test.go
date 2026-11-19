package marketplaceoauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

type memoryRepository struct {
	states      map[string]State
	deadlines   map[string]time.Time
	connections map[string]Connection
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{states: map[string]State{}, deadlines: map[string]time.Time{}, connections: map[string]Connection{}}
}
func (r *memoryRepository) SaveState(_ context.Context, hash, host string, verifier []byte, expires time.Time) error {
	r.states[hash] = State{HostID: host, EncryptedVerifier: append([]byte(nil), verifier...)}
	r.deadlines[hash] = expires
	return nil
}
func (r *memoryRepository) ConsumeState(_ context.Context, hash string, now time.Time) (State, error) {
	s, ok := r.states[hash]
	if !ok || !now.Before(r.deadlines[hash]) {
		return State{}, ErrNotFound
	}
	delete(r.states, hash)
	return s, nil
}
func (r *memoryRepository) SaveConnection(_ context.Context, c Connection) error {
	r.connections[c.HostID] = c
	return nil
}
func (r *memoryRepository) GetConnection(_ context.Context, id string) (Connection, error) {
	c, ok := r.connections[id]
	if !ok {
		return Connection{}, ErrNotFound
	}
	return c, nil
}
func (r *memoryRepository) RefreshConnection(_ context.Context, id string, refresh func(Connection) (Connection, error)) (Connection, error) {
	c, ok := r.connections[id]
	if !ok {
		return Connection{}, ErrNotFound
	}
	updated, err := refresh(c)
	r.connections[id] = updated
	return updated, err
}

type exchangerStub struct {
	exchange, refresh       TokenResponse
	exchangeErr, refreshErr error
	gotVerifier             string
	testMode                bool
}

func (e *exchangerStub) Exchange(_ context.Context, code, verifier string, test bool) (TokenResponse, error) {
	e.gotVerifier = verifier
	e.testMode = test
	if code == "" {
		return TokenResponse{}, ErrProvider
	}
	return e.exchange, e.exchangeErr
}
func (e *exchangerStub) Refresh(_ context.Context, token string, test bool) (TokenResponse, error) {
	e.testMode = test
	if token == "" {
		return TokenResponse{}, ErrProvider
	}
	return e.refresh, e.refreshErr
}

func testService(t *testing.T, repo Repository, exchanger TokenExchanger, now func() time.Time) *Service {
	t.Helper()
	svc, err := New(Config{ClientID: "app-id", ClientSecret: "private-secret", RedirectURI: "https://example.test/api/backend/host/mercadopago/oauth/callback", EncryptionKey: []byte("01234567890123456789012345678901"), TestMode: true}, repo, exchanger, now)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}
func testTokens(live bool, access, refresh string) TokenResponse {
	return TokenResponse{AccessToken: access, RefreshToken: refresh, UserID: "123456789", ExpiresIn: 3600, Scope: "offline_access read", LiveMode: &live}
}

func TestStartCreatesBoundExpiringStateAndS256Challenge(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	svc := testService(t, repo, &exchangerStub{}, func() time.Time { return now })
	authorization, err := svc.Start(context.Background(), "host-1")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authorization)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "auth.mercadopago.com" || q.Get("response_type") != "code" || q.Get("platform_id") != "mp" || q.Get("redirect_uri") != "https://example.test/api/backend/host/mercadopago/oauth/callback" {
		t.Fatalf("unexpected authorization URL: %s", u)
	}
	state := q.Get("state")
	digest := sha256.Sum256([]byte(state))
	hash := hex.EncodeToString(digest[:])
	stored, ok := repo.states[hash]
	if !ok || stored.HostID != "host-1" {
		t.Fatal("state is not durably bound to the host")
	}
	verifierBytes, err := svc.open(stored.EncryptedVerifier)
	if err != nil {
		t.Fatal(err)
	}
	verifier := string(verifierBytes)
	challenge := sha256.Sum256([]byte(verifier))
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Fatal("PKCE challenge does not match encrypted verifier")
	}
	if !repo.deadlines[hash].Equal(now.Add(10 * time.Minute)) {
		t.Fatal("state expiry is not short-lived")
	}
}

func TestCompletePersistsOnlyEncryptedTokensAndConsumesStateOnce(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	exchange := &exchangerStub{exchange: testTokens(false, "seller-access-secret", "seller-refresh-secret")}
	svc := testService(t, repo, exchange, func() time.Time { return now })
	authorization, err := svc.Start(context.Background(), "host-1")
	if err != nil {
		t.Fatal(err)
	}
	state := mustURL(t, authorization).Query().Get("state")
	host, err := svc.Complete(context.Background(), "authorization-code", state)
	if err != nil {
		t.Fatal(err)
	}
	if host != "host-1" || !exchange.testMode || exchange.gotVerifier == "" {
		t.Fatal("callback did not use stored host, PKCE verifier, and test mode")
	}
	connection := repo.connections[host]
	if connection.SellerID != "123456789" || connection.LiveMode || !connection.ExpiresAt.Equal(now.Add(time.Hour)) || strings.Join(connection.Scopes, " ") != "offline_access read" {
		t.Fatalf("unexpected connection metadata: %#v", connection)
	}
	if strings.Contains(string(connection.AccessToken), "seller-access-secret") || strings.Contains(string(connection.RefreshToken), "seller-refresh-secret") {
		t.Fatal("credential was persisted in plaintext")
	}
	if _, err = svc.Complete(context.Background(), "second-code", state); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replayed state accepted: %v", err)
	}
}

func TestDeniedCallbackConsumesStateAndRejectsReplay(t *testing.T) {
	repo := newMemoryRepository()
	svc := testService(t, repo, &exchangerStub{}, time.Now)
	start, err := svc.Start(context.Background(), "host")
	if err != nil {
		t.Fatal(err)
	}
	state := mustURL(t, start).Query().Get("state")
	if _, err = svc.Complete(context.Background(), "", state); !errors.Is(err, ErrUserDenied) {
		t.Fatalf("expected denied result, got %v", err)
	}
	if _, err = svc.Complete(context.Background(), "code", state); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("denied state replayed: %v", err)
	}
}

func TestCompleteRejectsWrongCredentialMode(t *testing.T) {
	repo := newMemoryRepository()
	svc := testService(t, repo, &exchangerStub{exchange: testTokens(true, "live", "refresh")}, time.Now)
	start, err := svc.Start(context.Background(), "host")
	if err != nil {
		t.Fatal(err)
	}
	state := mustURL(t, start).Query().Get("state")
	if _, err = svc.Complete(context.Background(), "code", state); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("live credentials accepted in test mode: %v", err)
	}
}

func TestTokenResponseAcceptsNumericOrStringSellerID(t *testing.T) {
	for _, body := range []string{
		`{"access_token":"a","refresh_token":"r","user_id":123456789,"expires_in":3600,"live_mode":false}`,
		`{"access_token":"a","refresh_token":"r","user_id":"123456789","expires_in":3600,"live_mode":false}`,
	} {
		var response TokenResponse
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			t.Fatal(err)
		}
		if response.UserID != "123456789" {
			t.Fatalf("unexpected seller id %q", response.UserID)
		}
	}
}

func TestStatusMarksExpiredConnectionForReauthorizationAndBlocksTokenUse(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	exchange := &exchangerStub{refreshErr: ErrProvider}
	svc := testService(t, repo, exchange, func() time.Time { return now })
	access, _ := svc.seal([]byte("access"))
	refresh, _ := svc.seal([]byte("refresh"))
	repo.connections["host"] = Connection{HostID: "host", SellerID: "123456789", AccessToken: access, RefreshToken: refresh, ExpiresAt: now.Add(time.Minute), LiveMode: false}
	status, err := svc.Status(context.Background(), "host")
	if err != nil {
		t.Fatal(err)
	}
	if !status.NeedsReauthorization {
		t.Fatal("failed refresh did not mark reauthorization")
	}
	if _, err = svc.AccessToken(context.Background(), "host"); !errors.Is(err, ErrProvider) {
		t.Fatalf("access token use was not blocked: %v", err)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
