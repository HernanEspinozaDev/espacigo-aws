// Package profilepin implements session-bound, time-limited access to a
// person's own sensitive identity fields.
package profilepin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

const AccessWindow = 15 * time.Minute

var (
	ErrNotConfigured     = errors.New("pinpass not configured")
	ErrInvalidPIN        = errors.New("invalid pinpass")
	ErrInvalidInput      = errors.New("invalid pinpass input")
	ErrProfileIncomplete = errors.New("identity profile is incomplete")
	ErrLocked            = errors.New("pinpass temporarily locked")
	ErrNoAccess          = errors.New("sensitive profile access unavailable")
	ErrStepUpRejected    = errors.New("recent authentication rejected")
	pinPattern           = regexp.MustCompile(`^[0-9]{6}$`)
)

// Credential contains only an Argon2id verifier and its per-account salt.
type Credential struct{ Salt, Hash []byte }

// FailureState is maintained atomically and persistently by Store.
type FailureState struct {
	Attempts     int
	BlockedUntil time.Time
}

// Access is a fixed-expiry grant scoped to both account and authenticated session.
type Access struct{ ExpiresAt time.Time }

// Store implementations must make failure counting, credential changes,
// grants, failed attempts, audit events, and revocation durable and atomic
// across API instances. Failure methods enforce both account and session buckets.
type Store interface {
	Credential(context.Context, string) (Credential, bool, error)
	SetCredentialAndRevoke(context.Context, string, Credential, string, time.Time) error
	DeleteCredentialAndRevoke(context.Context, string, string, time.Time) error
	FailureState(context.Context, string, string) (FailureState, error)
	RecordFailureAndAudit(context.Context, string, string, time.Time) (FailureState, error)
	ClearFailures(context.Context, string, string) error
	GrantAndAudit(context.Context, string, string, Credential, time.Time, time.Time) error
	ActiveGrant(context.Context, string, string, time.Time) (Access, bool, error)
	WithActiveGrant(context.Context, string, string, time.Time, string, func(context.Context) error) (Access, error)
}

type Proof struct {
	CurrentPassword identity.Secret
	OTPCode         identity.Secret
}

func (p Proof) Present() bool {
	return p.CurrentPassword != "" || p.OTPCode != ""
}

// StepUp validates an account-owned recent authentication proof. Implementors
// must consume/bind the proof to accountID and sessionID and reject replay.
type StepUp interface {
	Verify(context.Context, string, string, Proof) error
}

// SensitiveProfileProvider returns only the authenticated account's decrypted
// email and RUN. Implementations must enforce ownership themselves.
type SensitiveProfileProvider interface {
	OwnSensitiveProfile(context.Context, string) (map[string]string, error)
}

// SensitiveQueryer gives a profile reader query-only access to the same
// transaction that holds the account lock during a reveal.
type SensitiveQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type sensitiveQueryerContextKey struct{}

func WithSensitiveQueryer(ctx context.Context, queryer SensitiveQueryer) context.Context {
	return context.WithValue(ctx, sensitiveQueryerContextKey{}, queryer)
}

func SensitiveQueryerFromContext(ctx context.Context) (SensitiveQueryer, bool) {
	queryer, ok := ctx.Value(sensitiveQueryerContextKey{}).(SensitiveQueryer)
	return queryer, ok && queryer != nil
}

type Service struct {
	store     Store
	stepUp    StepUp
	profiles  SensitiveProfileProvider
	pepper    []byte
	now       func() time.Time
	random    io.Reader
	hashSlots chan struct{}
}

func NewService(store Store, stepUp StepUp, profiles SensitiveProfileProvider, pepper []byte) (*Service, error) {
	if store == nil || stepUp == nil || profiles == nil || len(pepper) < 32 {
		return nil, errors.New("pinpass dependencies and a 32-byte server pepper are required")
	}
	return &Service{store: store, stepUp: stepUp, profiles: profiles, pepper: append([]byte(nil), pepper...), now: time.Now, random: rand.Reader, hashSlots: make(chan struct{}, 8)}, nil
}

// Set requires a recent account factor for both enrollment and replacement.
func (s *Service) Set(ctx context.Context, accountID, sessionID, pin, confirmation string, proof Proof) error {
	if accountID == "" || sessionID == "" || !pinPattern.MatchString(pin) || subtle.ConstantTimeCompare([]byte(pin), []byte(confirmation)) != 1 {
		return ErrInvalidInput
	}
	if err := s.stepUp.Verify(ctx, accountID, sessionID, proof); err != nil {
		return ErrStepUpRejected
	}
	profile, err := s.profiles.OwnSensitiveProfile(ctx, accountID)
	if err != nil {
		return err
	}
	for _, field := range []string{"email", "run", "phone", "legal_name", "date_of_birth"} {
		if strings.TrimSpace(profile[field]) == "" {
			return ErrProfileIncomplete
		}
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(s.random, salt); err != nil {
		return err
	}
	hash := s.derive(pin, salt)
	if err := s.store.SetCredentialAndRevoke(ctx, accountID, Credential{Salt: salt, Hash: hash}, sessionID, s.now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Service) Status(ctx context.Context, accountID, sessionID string) (bool, FailureState, error) {
	_, configured, err := s.store.Credential(ctx, accountID)
	if err != nil {
		return false, FailureState{}, err
	}
	f, err := s.store.FailureState(ctx, accountID, sessionID)
	return configured, f, err
}

func (s *Service) Unlock(ctx context.Context, accountID, sessionID, pin string) (time.Time, error) {
	now := s.now().UTC().Truncate(time.Microsecond)
	if f, err := s.store.FailureState(ctx, accountID, sessionID); err != nil {
		return time.Time{}, err
	} else if now.Before(f.BlockedUntil) {
		return time.Time{}, &LockedError{Until: f.BlockedUntil}
	}
	c, ok, err := s.store.Credential(ctx, accountID)
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, ErrNotConfigured
	}
	valid := false
	if pinPattern.MatchString(pin) {
		select {
		case s.hashSlots <- struct{}{}:
			valid = subtle.ConstantTimeCompare(s.derive(pin, c.Salt), c.Hash) == 1
			<-s.hashSlots
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
	if !valid {
		f, err := s.store.RecordFailureAndAudit(ctx, accountID, sessionID, now)
		if err != nil {
			return time.Time{}, err
		}
		if now.Before(f.BlockedUntil) {
			return time.Time{}, &LockedError{Until: f.BlockedUntil}
		}
		return time.Time{}, ErrInvalidPIN
	}
	if err = s.store.ClearFailures(ctx, accountID, sessionID); err != nil {
		return time.Time{}, err
	}
	expires := now.Add(AccessWindow)
	if err = s.store.GrantAndAudit(ctx, accountID, sessionID, c, expires, now); err != nil {
		return time.Time{}, err
	}
	return expires, nil
}

func (s *Service) Reveal(ctx context.Context, accountID, sessionID, field string) (string, time.Time, error) {
	supported := map[string]bool{"email": true, "run": true, "phone": true, "legal_name": true, "date_of_birth": true}
	if !supported[field] {
		return "", time.Time{}, ErrNoAccess
	}
	now := s.now().UTC()
	var value string
	grant, err := s.store.WithActiveGrant(ctx, accountID, sessionID, now, "pinpass.sensitive_data_viewed", func(readCtx context.Context) error {
		profile, readErr := s.profiles.OwnSensitiveProfile(readCtx, accountID)
		if readErr != nil {
			return readErr
		}
		var ok bool
		value, ok = profile[field]
		if !ok {
			return ErrNoAccess
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return value, grant.ExpiresAt, nil
}

func (s *Service) Reset(ctx context.Context, accountID, sessionID string, proof Proof) error {
	if err := s.stepUp.Verify(ctx, accountID, sessionID, proof); err != nil {
		return ErrStepUpRejected
	}
	if err := s.store.DeleteCredentialAndRevoke(ctx, accountID, sessionID, s.now().UTC()); err != nil {
		return err
	}
	return nil
}

type LockedError struct{ Until time.Time }

func (e *LockedError) Error() string        { return ErrLocked.Error() }
func (e *LockedError) Is(target error) bool { return target == ErrLocked }

func (s *Service) derive(pin string, salt []byte) []byte {
	input := make([]byte, 0, len(pin)+len(s.pepper))
	input = append(input, pin...)
	input = append(input, s.pepper...)
	return argon2.IDKey(input, salt, 3, 64*1024, 1, 32)
}
