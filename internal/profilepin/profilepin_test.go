package profilepin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type fakeStore struct {
	credential Credential
	configured bool
	failures   map[string]FailureState
	grants     map[string]time.Time
	audit      []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{failures: map[string]FailureState{}, grants: map[string]time.Time{}}
}
func sessionKey(account, session string) string { return account + "/" + session }
func (f *fakeStore) Credential(context.Context, string) (Credential, bool, error) {
	return f.credential, f.configured, nil
}
func (f *fakeStore) SetCredentialAndRevoke(_ context.Context, account string, c Credential, session string, _ time.Time) error {
	f.credential, f.configured = c, true
	f.grants = map[string]time.Time{}
	f.audit = append(f.audit, "pinpass.changed:"+account+":"+session)
	return nil
}
func (f *fakeStore) DeleteCredentialAndRevoke(_ context.Context, account, session string, _ time.Time) error {
	f.credential, f.configured = Credential{}, false
	f.grants = map[string]time.Time{}
	f.audit = append(f.audit, "pinpass.reset:"+account+":"+session)
	return nil
}
func (f *fakeStore) FailureState(_ context.Context, account, session string) (FailureState, error) {
	accountState, sessionState := f.failures[sessionKey(account, "")], f.failures[sessionKey(account, session)]
	state := accountState
	if sessionState.Attempts > state.Attempts {
		state.Attempts = sessionState.Attempts
	}
	if sessionState.BlockedUntil.After(state.BlockedUntil) {
		state.BlockedUntil = sessionState.BlockedUntil
	}
	return state, nil
}
func (f *fakeStore) RecordFailureAndAudit(_ context.Context, account, session string, at time.Time) (FailureState, error) {
	for _, k := range []string{sessionKey(account, ""), sessionKey(account, session)} {
		state := f.failures[k]
		state.Attempts++
		if state.Attempts >= 3 {
			state.BlockedUntil = at.Add(15 * time.Second)
		}
		f.failures[k] = state
	}
	state, _ := f.FailureState(context.Background(), account, session)
	f.audit = append(f.audit, "pinpass.unlock_denied:"+account+":"+session)
	return state, nil
}
func (f *fakeStore) ClearFailures(_ context.Context, account, session string) error {
	delete(f.failures, sessionKey(account, ""))
	delete(f.failures, sessionKey(account, session))
	return nil
}
func (f *fakeStore) GrantAndAudit(_ context.Context, account, session string, expected Credential, expires, at time.Time) error {
	if !f.configured || string(expected.Hash) != string(f.credential.Hash) {
		return ErrInvalidPIN
	}
	f.grants[sessionKey(account, session)] = expires
	f.audit = append(f.audit, "pinpass.unlocked:"+account+":"+session)
	return nil
}
func (f *fakeStore) ActiveGrant(_ context.Context, account, session string, now time.Time) (Access, bool, error) {
	expires, ok := f.grants[sessionKey(account, session)]
	return Access{ExpiresAt: expires}, ok && now.Before(expires), nil
}
func (f *fakeStore) WithActiveGrant(_ context.Context, account, session string, now time.Time, action string, reveal func(context.Context) error) (Access, error) {
	expires, ok := f.grants[sessionKey(account, session)]
	if !ok || !now.Before(expires) {
		return Access{}, ErrNoAccess
	}
	if err := reveal(context.Background()); err != nil {
		return Access{}, err
	}
	f.audit = append(f.audit, action+":"+account+":"+session)
	return Access{ExpiresAt: expires}, nil
}

type fakeStepUp struct {
	err      error
	received Proof
}

func (f *fakeStepUp) Verify(_ context.Context, _, _ string, proof Proof) error {
	f.received = proof
	return f.err
}

type fakeProfiles struct {
	profile map[string]string
	account string
}

func (f *fakeProfiles) OwnSensitiveProfile(_ context.Context, account string) (map[string]string, error) {
	f.account = account
	return f.profile, nil
}

type fakeAuthorizer struct {
	principal identity.Principal
	err       error
}

func (a fakeAuthorizer) Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error) {
	return a.principal, a.err
}

func completeSensitiveProfile() map[string]string {
	return map[string]string{"email": "own@example.cl", "run": "12.345.678-5", "phone": "+56912345678", "legal_name": "Nombre Apellido", "date_of_birth": "1990-01-01"}
}

func TestSetRequiresCompleteProfileAndRecentProof(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	stepUp := &fakeStepUp{}
	profiles := &fakeProfiles{profile: completeSensitiveProfile()}
	svc, err := NewService(store, stepUp, profiles, []byte(strings.Repeat("p", 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 123456000, time.UTC)
	svc.now = func() time.Time { return now }
	if err = svc.Set(ctx, "account-1", "session-1", "012345", "012345", Proof{CurrentPassword: "secret"}); err != nil {
		t.Fatal(err)
	}
	if stepUp.received != (Proof{CurrentPassword: "secret"}) {
		t.Fatal("incorrect step-up proof")
	}
	if !store.configured || len(store.credential.Salt) != 16 || len(store.credential.Hash) != 32 {
		t.Fatal("credential was not stored as a salted Argon2id verifier")
	}
	if store.audit[0] != "pinpass.changed:account-1:session-1" {
		t.Fatalf("unexpected audit event: %q", store.audit[0])
	}

	profiles.profile = map[string]string{"email": "own@example.cl"}
	err = svc.Set(ctx, "account-1", "session-1", "654321", "654321", Proof{OTPCode: "123456"})
	if !errors.Is(err, ErrProfileIncomplete) {
		t.Fatalf("Set error = %v, want profile incomplete", err)
	}
}

func TestUnlockRevealIsSessionBoundAndExpiresWithoutExtension(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	profiles := &fakeProfiles{profile: completeSensitiveProfile()}
	svc, err := NewService(store, &fakeStepUp{}, profiles, []byte(strings.Repeat("p", 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 123456789, time.UTC)
	svc.now = func() time.Time { return now }
	if err = svc.Set(ctx, "account-1", "session-1", "012345", "012345", Proof{CurrentPassword: "secret"}); err != nil {
		t.Fatal(err)
	}
	expires, err := svc.Unlock(ctx, "account-1", "session-1", "012345")
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Truncate(time.Microsecond).Add(AccessWindow)) {
		t.Fatalf("expiry %s doesn't match fixed 15 minute window", expires)
	}
	value, revealExpiry, err := svc.Reveal(ctx, "account-1", "session-1", "run")
	if err != nil {
		t.Fatal(err)
	}
	if value != "12.345.678-5" || !revealExpiry.Equal(expires) || profiles.account != "account-1" {
		t.Fatal("did not reveal the own RUN under the current session grant")
	}
	if _, _, err = svc.Reveal(ctx, "account-1", "session-2", "run"); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("other session reveal error = %v", err)
	}
	now = expires
	if _, _, err = svc.Reveal(ctx, "account-1", "session-1", "email"); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("expired reveal error = %v", err)
	}
}

func TestFailedPINUsesPersistentThrottleAndResetRevokesGrant(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc, err := NewService(store, &fakeStepUp{}, &fakeProfiles{profile: completeSensitiveProfile()}, []byte(strings.Repeat("p", 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	if err = svc.Set(ctx, "account-1", "session-1", "012345", "012345", Proof{CurrentPassword: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Unlock(ctx, "account-1", "session-2", "012345"); err != nil {
		t.Fatalf("valid PIN should create an access grant: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err = svc.Unlock(ctx, "account-1", "session-1", "000000"); !errors.Is(err, ErrInvalidPIN) {
			t.Fatalf("failure %d error=%v", i+1, err)
		}
	}
	if _, err = svc.Unlock(ctx, "account-1", "session-1", "000000"); !errors.Is(err, ErrLocked) {
		t.Fatalf("third failure error=%v, want locked", err)
	}
	if _, err = svc.Unlock(ctx, "account-1", "session-2", "012345"); !errors.Is(err, ErrLocked) {
		t.Fatalf("second session must inherit account bucket lock: %v", err)
	}
	if err = svc.Reset(ctx, "account-1", "session-2", Proof{CurrentPassword: "secret"}); err != nil {
		t.Fatal(err)
	}
	if store.configured || len(store.grants) != 0 {
		t.Fatal("reset must remove the verifier and revoke every session grant")
	}
}

func TestHTTPHandlerAuthorizesAndNeverCachesSensitiveResponses(t *testing.T) {
	store := newFakeStore()
	profiles := &fakeProfiles{profile: completeSensitiveProfile()}
	svc, err := NewService(store, &fakeStepUp{}, profiles, []byte(strings.Repeat("p", 32)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	if err = svc.Set(ctx, "account-1", "session-1", "012345", "012345", Proof{CurrentPassword: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Unlock(ctx, "account-1", "session-1", "012345"); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(fakeAuthorizer{principal: identity.Principal{AccountID: "account-1", SessionID: "session-1"}}, svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/account/security/pinpass/reveal/run", nil)
	r.Header.Set("Authorization", "Bearer session-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "12.345.678-5") {
		t.Fatalf("unexpected reveal response: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store, private" {
		t.Fatalf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
	r = httptest.NewRequest(http.MethodPost, "/api/v1/account/security/pinpass/unlock", strings.NewReader(`{"pin":"999999"}`))
	r.Header.Set("Authorization", "Bearer session-token")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "invalid_pin") {
		t.Fatalf("invalid PIN must not invalidate the account session: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/account/security/pinpass/reveal/run", nil)
	r.Header.Set("Authorization", "Bearer session-token")
	unauthenticated := NewHandler(fakeAuthorizer{err: identity.ErrUnauthorized}, svc)
	w = httptest.NewRecorder()
	unauthenticated.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "12.345.678-5") {
		t.Fatalf("unauthorized reveal response: %d %s", w.Code, w.Body.String())
	}
}

func TestPinpassChangeAcceptsEitherFactorAndPassesBoth(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		proof  Proof
		status int
	}{
		{"password", `{"pin":"012345","confirmation":"012345","current_password":"example"}`, Proof{CurrentPassword: "example"}, http.StatusOK},
		{"otp", `{"pin":"012345","confirmation":"012345","otp_code":"042673"}`, Proof{OTPCode: "042673"}, http.StatusOK},
		{"both", `{"pin":"012345","confirmation":"012345","current_password":"example","otp_code":"042673"}`, Proof{CurrentPassword: "example", OTPCode: "042673"}, http.StatusOK},
		{"missing password in combined request", `{"pin":"012345","confirmation":"012345","current_password":"","otp_code":"042673"}`, Proof{}, http.StatusBadRequest},
		{"missing", `{"pin":"012345","confirmation":"012345"}`, Proof{}, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			stepUp := &fakeStepUp{}
			service, err := NewService(newFakeStore(), stepUp, &fakeProfiles{profile: completeSensitiveProfile()}, make([]byte, 32))
			if err != nil {
				t.Fatal(err)
			}
			handler := NewHandler(fakeAuthorizer{principal: identity.Principal{AccountID: "account-1", SessionID: "session-1"}}, service)
			request := httptest.NewRequest(http.MethodPut, "/api/v1/account/security/pinpass", strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer synthetic-session")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || stepUp.received != test.proof {
				t.Fatalf("unexpected verification request or status (%d)", response.Code)
			}
		})
	}
}
