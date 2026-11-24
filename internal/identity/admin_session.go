package identity

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	AdminSessionAudience        = "admin"
	AdminSessionDuration        = 8 * time.Hour
	AdminSessionIdleTimeout     = 30 * time.Minute
	AdminMFAChallengeTTL        = 5 * time.Minute
	AdminMFAChallengeMaxAttempt = 5
	AdminRecentAuthTTL          = 10 * time.Minute
)

var (
	ErrAdminStaffRequired         = errors.New("identity: admin staff role required")
	ErrAdminMFAEnrollmentRequired = errors.New("identity: admin mfa enrollment required")
	ErrAdminMFAChallengeExpired   = errors.New("identity: admin mfa challenge expired")
	ErrAdminMFAChallengeExhausted = errors.New("identity: admin mfa challenge exhausted")
	ErrAdminMFACodeInvalid        = errors.New("identity: admin mfa code invalid")
)

// AdminSession represents a dedicated staff session isolated to the "admin" audience.
type AdminSession struct {
	ID             string
	AccountID      string
	Audience       string
	TokenHash      string `json:"-"`
	CreatedAt      time.Time
	LastActivityAt time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	MFAVerifiedAt  *time.Time
	MFAMethod      string
	DeviceLabel    string
	IPMasked       string
	UserAgent      string
}

func (AdminSession) String() string   { return "AdminSession{TokenHash:[REDACTED]}" }
func (AdminSession) GoString() string { return "AdminSession{TokenHash:[REDACTED]}" }

// ActiveAt reports whether the admin session is unrevoked, unexpired, and within idle timeout.
func (s AdminSession) ActiveAt(now time.Time) bool {
	return s.Audience == AdminSessionAudience &&
		s.RevokedAt == nil &&
		now.Before(s.ExpiresAt) &&
		now.Before(s.LastActivityAt.Add(AdminSessionIdleTimeout))
}

// AdminMFAChallenge tracks a login or step-up MFA challenge for staff authentication.
type AdminMFAChallenge struct {
	ID             string
	AccountID      string
	AdminSessionID string
	Purpose        string
	Attempts       int
	MaxAttempts    int
	CreatedAt      time.Time
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	IPMasked       string
	UserAgent      string
}

// AdminAccountLookup selects exactly one key for locking an admin account transaction.
type AdminAccountLookup struct {
	Email            string
	ChallengeID      string
	AdminSessionHash string
}

// AdminSessionRepository provides transactional persistence for staff sessions and MFA challenges.
type AdminSessionRepository interface {
	WithLockedAdminAccount(context.Context, AdminAccountLookup, func(Account, AdminSessionTransaction) error) error
}

// AdminSessionTransaction exposes transactional operations for staff sessions and MFA verification.
type AdminSessionTransaction interface {
	Roles(context.Context, string) ([]Role, error)
	totpLoginTransaction
	CreateAdminSession(context.Context, AdminSession) error
	AdminSessionByTokenHash(context.Context, string) (AdminSession, error)
	TouchAdminSession(context.Context, string, time.Time) (bool, error)
	RevokeAdminSession(context.Context, string, time.Time) error
	UpdateAdminSessionMFA(context.Context, string, time.Time, string) error
	CreateAdminMFAChallenge(context.Context, AdminMFAChallenge) error
	AdminMFAChallengeByID(context.Context, string) (AdminMFAChallenge, error)
	IncrementAdminMFAChallengeAttempt(context.Context, string) (int, error)
	ConsumeAdminMFAChallenge(context.Context, string, time.Time) error
}

// AdminSessionService manages isolated admin-audience staff sessions and MFA step-up.
type AdminSessionService struct {
	repo               AdminSessionRepository
	passwords          PasswordHasher
	totpKeyring        *TOTPKeyring
	credentials        CredentialGenerator
	now                func() time.Time
	requireEnrolledMFA bool
}

// NewAdminSessionService constructs an AdminSessionService with explicit MFA policy.
func NewAdminSessionService(
	repo AdminSessionRepository,
	passwords PasswordHasher,
	totpKeyring *TOTPKeyring,
	credentials CredentialGenerator,
	now func() time.Time,
	requireEnrolledMFA bool,
) (*AdminSessionService, error) {
	if repo == nil || passwords == nil || credentials == nil || now == nil {
		return nil, ErrInvalid
	}
	return &AdminSessionService{
		repo:               repo,
		passwords:          passwords,
		totpKeyring:        totpKeyring,
		credentials:        credentials,
		now:                now,
		requireEnrolledMFA: requireEnrolledMFA,
	}, nil
}

// SetTOTPKeyring updates the keyring used to decrypt staff TOTP secrets.
func (s *AdminSessionService) SetTOTPKeyring(kr *TOTPKeyring) {
	s.totpKeyring = kr
}

type AdminSessionStartInput struct {
	Email     string
	Password  Secret
	UserAgent string
	IP        string
}

type AdminSessionStartResult struct {
	Status              string       `json:"status"`
	ChallengeID         string       `json:"challenge_id,omitempty"`
	Methods             []string     `json:"methods,omitempty"`
	AccountID           string       `json:"account_id,omitempty"`
	Audience            string       `json:"audience,omitempty"`
	Roles               []Role       `json:"roles,omitempty"`
	Capabilities        []Capability `json:"capabilities,omitempty"`
	Token               Secret       `json:"token,omitempty"`
	MFAVerifiedAt       *time.Time   `json:"mfa_verified_at"`
	RecentAuthExpiresAt *time.Time   `json:"recent_auth_expires_at"`
	ExpiresAt           time.Time    `json:"expires_at"`
}

type AdminSessionMFAInput struct {
	ChallengeID string
	AdminToken  Secret
	Code        string
	UserAgent   string
	IP          string
}

type AdminSessionAuthResult struct {
	Status              string       `json:"status"`
	AccountID           string       `json:"account_id"`
	Audience            string       `json:"audience"`
	Roles               []Role       `json:"roles"`
	Capabilities        []Capability `json:"capabilities"`
	Token               Secret       `json:"token,omitempty"`
	MFAVerifiedAt       *time.Time   `json:"mfa_verified_at"`
	RecentAuthExpiresAt *time.Time   `json:"recent_auth_expires_at"`
	ExpiresAt           time.Time    `json:"expires_at"`
}

type AdminSessionInfo struct {
	AccountID           string       `json:"account_id"`
	Audience            string       `json:"audience"`
	Roles               []Role       `json:"roles"`
	Capabilities        []Capability `json:"capabilities"`
	MFAVerifiedAt       *time.Time   `json:"mfa_verified_at"`
	RecentAuthExpiresAt *time.Time   `json:"recent_auth_expires_at"`
	ExpiresAt           time.Time    `json:"expires_at"`
}

func isAdminStaff(roles []Role) bool {
	for _, r := range roles {
		if r == RoleGlobalAdministrator || r == RoleAdministrator {
			return true
		}
	}
	return false
}

func accountEligibleForAdmin(account Account, now time.Time) error {
	if account.BlockedUntil != nil && now.Before(*account.BlockedUntil) {
		return &LoginBlockedError{*account.BlockedUntil}
	}
	if account.State != AccountActive {
		return ErrAccountDisabled
	}
	if account.AdministrativeBlocked {
		return ErrRestrictedMode
	}
	return nil
}

// Start validates staff credentials and either issues an MFA challenge or an admin session.
func (s *AdminSessionService) Start(ctx context.Context, input AdminSessionStartInput) (AdminSessionStartResult, error) {
	if ValidateEmail(input.Email) != nil || input.Password == "" {
		return AdminSessionStartResult{}, ErrCredentials
	}
	var result AdminSessionStartResult
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{Email: NormalizeEmail(input.Email)}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		if err := accountEligibleForAdmin(account, now); err != nil {
			denied = err
			return nil
		}
		ok, err := s.passwords.Matches(account.PasswordHash, input.Password)
		if err != nil {
			return err
		}
		if !ok {
			denied = ErrCredentials
			return nil
		}
		roles, err := tx.Roles(ctx, account.ID)
		if err != nil {
			return err
		}
		if !isAdminStaff(roles) {
			denied = ErrAdminStaffRequired
			return nil
		}
		totp, _, err := tx.GetTOTP(ctx, account.ID)
		if err != nil {
			return err
		}
		if totp != nil && totp.EnabledAt != nil {
			challengeID, err := s.credentials.ID()
			if err != nil {
				return err
			}
			challenge := AdminMFAChallenge{
				ID:          challengeID,
				AccountID:   account.ID,
				Purpose:     "login",
				Attempts:    0,
				MaxAttempts: AdminMFAChallengeMaxAttempt,
				CreatedAt:   now,
				ExpiresAt:   now.Add(AdminMFAChallengeTTL),
				IPMasked:    MaskIP(input.IP),
				UserAgent:   SanitizeUserAgent(input.UserAgent),
			}
			if err := tx.CreateAdminMFAChallenge(ctx, challenge); err != nil {
				return err
			}
			result = AdminSessionStartResult{
				Status:      "mfa_required",
				ChallengeID: challenge.ID,
				Methods:     []string{"totp", "backup_code"},
				ExpiresAt:   challenge.ExpiresAt,
			}
			return nil
		}
		if s.requireEnrolledMFA {
			denied = ErrAdminMFAEnrollmentRequired
			return nil
		}
		session, rawToken, err := s.newAdminSession(account.ID, now, nil, "", input.UserAgent, input.IP)
		if err != nil {
			return err
		}
		if err := tx.CreateAdminSession(ctx, session); err != nil {
			return err
		}
		result = AdminSessionStartResult{
			Status:              "authenticated",
			AccountID:           account.ID,
			Audience:            AdminSessionAudience,
			Roles:               roles,
			Capabilities:        CapabilitiesForRoles(roles),
			Token:               rawToken,
			MFAVerifiedAt:       nil,
			RecentAuthExpiresAt: nil,
			ExpiresAt:           session.ExpiresAt,
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return AdminSessionStartResult{}, ErrCredentials
	}
	if err != nil {
		return AdminSessionStartResult{}, err
	}
	return result, denied
}

// VerifyMFA completes either a login MFA challenge or a step-up MFA refresh on an existing admin session.
func (s *AdminSessionService) VerifyMFA(ctx context.Context, input AdminSessionMFAInput) (AdminSessionAuthResult, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return AdminSessionAuthResult{}, ErrAdminMFACodeInvalid
	}
	if input.ChallengeID != "" {
		return s.verifyLoginChallengeMFA(ctx, input, code)
	}
	if input.AdminToken != "" {
		return s.verifyStepUpMFA(ctx, input.AdminToken, code)
	}
	return AdminSessionAuthResult{}, ErrUnauthorized
}

func (s *AdminSessionService) verifyLoginChallengeMFA(ctx context.Context, input AdminSessionMFAInput, code string) (AdminSessionAuthResult, error) {
	var result AdminSessionAuthResult
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{ChallengeID: input.ChallengeID}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		if err := accountEligibleForAdmin(account, now); err != nil {
			denied = err
			return nil
		}
		roles, err := tx.Roles(ctx, account.ID)
		if err != nil {
			return err
		}
		if !isAdminStaff(roles) {
			denied = ErrAdminStaffRequired
			return nil
		}
		challenge, err := tx.AdminMFAChallengeByID(ctx, input.ChallengeID)
		if err != nil {
			return err
		}
		if challenge.AccountID != account.ID || challenge.Purpose != "login" || challenge.ConsumedAt != nil || !now.Before(challenge.ExpiresAt) {
			denied = ErrAdminMFAChallengeExpired
			return nil
		}
		if challenge.Attempts >= challenge.MaxAttempts {
			denied = ErrAdminMFAChallengeExhausted
			return nil
		}
		method, err := verifyTOTPOrBackupCode(ctx, tx, s.totpKeyring, account.ID, code, now, false)
		if err != nil {
			if errors.Is(err, ErrTOTPInvalidCode) || errors.Is(err, ErrTOTPReplayDetected) || errors.Is(err, ErrLoginTOTPRequired) {
				attempts, incErr := tx.IncrementAdminMFAChallengeAttempt(ctx, challenge.ID)
				if incErr != nil {
					return incErr
				}
				if attempts >= challenge.MaxAttempts {
					denied = ErrAdminMFAChallengeExhausted
				} else {
					denied = ErrAdminMFACodeInvalid
				}
				return nil
			}
			return err
		}
		if err := tx.ConsumeAdminMFAChallenge(ctx, challenge.ID, now); err != nil {
			return err
		}
		verifiedAt := now
		session, rawToken, err := s.newAdminSession(account.ID, now, &verifiedAt, method, input.UserAgent, input.IP)
		if err != nil {
			return err
		}
		if err := tx.CreateAdminSession(ctx, session); err != nil {
			return err
		}
		recentExp := verifiedAt.Add(AdminRecentAuthTTL)
		result = AdminSessionAuthResult{
			Status:              "authenticated",
			AccountID:           account.ID,
			Audience:            AdminSessionAudience,
			Roles:               roles,
			Capabilities:        CapabilitiesForRoles(roles),
			Token:               rawToken,
			MFAVerifiedAt:       &verifiedAt,
			RecentAuthExpiresAt: &recentExp,
			ExpiresAt:           session.ExpiresAt,
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return AdminSessionAuthResult{}, ErrAdminMFAChallengeExpired
	}
	if err != nil {
		return AdminSessionAuthResult{}, err
	}
	return result, denied
}

func (s *AdminSessionService) verifyStepUpMFA(ctx context.Context, raw Secret, code string) (AdminSessionAuthResult, error) {
	hash := CredentialHash(raw)
	var result AdminSessionAuthResult
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{AdminSessionHash: hash}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		if err := accountEligibleForAdmin(account, now); err != nil {
			denied = ErrUnauthorized
			return nil
		}
		session, err := tx.AdminSessionByTokenHash(ctx, hash)
		if err != nil {
			return err
		}
		if session.AccountID != account.ID || now.Before(session.CreatedAt) || !session.ActiveAt(now) {
			denied = ErrUnauthorized
			return nil
		}
		roles, err := tx.Roles(ctx, account.ID)
		if err != nil {
			return err
		}
		if !isAdminStaff(roles) {
			denied = ErrForbidden
			return nil
		}
		totp, _, err := tx.GetTOTP(ctx, account.ID)
		if err != nil {
			return err
		}
		if totp == nil || totp.EnabledAt == nil {
			denied = ErrAdminMFAEnrollmentRequired
			return nil
		}
		method, err := verifyTOTPOrBackupCode(ctx, tx, s.totpKeyring, account.ID, code, now, false)
		if err != nil {
			if errors.Is(err, ErrTOTPInvalidCode) || errors.Is(err, ErrTOTPReplayDetected) || errors.Is(err, ErrLoginTOTPRequired) {
				denied = ErrAdminMFACodeInvalid
				return nil
			}
			return err
		}
		if err := tx.UpdateAdminSessionMFA(ctx, session.ID, now, method); err != nil {
			return err
		}
		if _, err := tx.TouchAdminSession(ctx, session.ID, now); err != nil {
			return err
		}
		verifiedAt := now
		recentExp := verifiedAt.Add(AdminRecentAuthTTL)
		result = AdminSessionAuthResult{
			Status:              "authenticated",
			AccountID:           account.ID,
			Audience:            AdminSessionAudience,
			Roles:               roles,
			Capabilities:        CapabilitiesForRoles(roles),
			MFAVerifiedAt:       &verifiedAt,
			RecentAuthExpiresAt: &recentExp,
			ExpiresAt:           session.ExpiresAt,
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return AdminSessionAuthResult{}, ErrUnauthorized
	}
	if err != nil {
		return AdminSessionAuthResult{}, err
	}
	return result, denied
}

// GetSession validates an admin-audience Bearer token and returns current session metadata.
func (s *AdminSessionService) GetSession(ctx context.Context, raw Secret) (AdminSessionInfo, error) {
	if raw == "" {
		return AdminSessionInfo{}, ErrUnauthorized
	}
	hash := CredentialHash(raw)
	var info AdminSessionInfo
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{AdminSessionHash: hash}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		if account.State != AccountActive || (account.BlockedUntil != nil && now.Before(*account.BlockedUntil)) {
			denied = ErrUnauthorized
			return nil
		}
		if account.AdministrativeBlocked {
			denied = ErrRestrictedMode
			return nil
		}
		session, err := tx.AdminSessionByTokenHash(ctx, hash)
		if err != nil {
			return err
		}
		if session.AccountID != account.ID || now.Before(session.CreatedAt) || !session.ActiveAt(now) {
			denied = ErrUnauthorized
			return nil
		}
		roles, err := tx.Roles(ctx, account.ID)
		if err != nil {
			return err
		}
		if !isAdminStaff(roles) {
			denied = ErrForbidden
			return nil
		}
		var mfaVerifiedAt *time.Time
		var recentAuthExpiresAt *time.Time
		if session.MFAVerifiedAt != nil {
			v := session.MFAVerifiedAt.UTC()
			exp := v.Add(AdminRecentAuthTTL)
			mfaVerifiedAt = &v
			recentAuthExpiresAt = &exp
		}
		info = AdminSessionInfo{
			AccountID:           account.ID,
			Audience:            AdminSessionAudience,
			Roles:               roles,
			Capabilities:        CapabilitiesForRoles(roles),
			MFAVerifiedAt:       mfaVerifiedAt,
			RecentAuthExpiresAt: recentAuthExpiresAt,
			ExpiresAt:           session.ExpiresAt.UTC(),
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return AdminSessionInfo{}, ErrUnauthorized
	}
	if err != nil {
		return AdminSessionInfo{}, err
	}
	return info, denied
}

// RevokeSession revokes an admin-audience session by Bearer token.
func (s *AdminSessionService) RevokeSession(ctx context.Context, raw Secret) error {
	if raw == "" {
		return ErrUnauthorized
	}
	hash := CredentialHash(raw)
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{AdminSessionHash: hash}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		session, err := tx.AdminSessionByTokenHash(ctx, hash)
		if err != nil {
			return err
		}
		if session.AccountID != account.ID || now.Before(session.CreatedAt) {
			denied = ErrUnauthorized
			return nil
		}
		return tx.RevokeAdminSession(ctx, session.ID, now)
	})
	if errors.Is(err, ErrNotFound) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	return denied
}

// AuthorizeCapability implements adminglobalhttp.CapabilityAuthenticator using admin-audience sessions only.
func (s *AdminSessionService) AuthorizeCapability(ctx context.Context, raw Secret, required Capability, activity ActivityKind) (Principal, error) {
	return s.AuthorizeAdminCapability(ctx, raw, required, activity)
}

// AuthorizeAdminCapability authorizes a capability strictly against sesion_administrativa.
// Commercial sessions are rejected with ErrUnauthorized.
func (s *AdminSessionService) AuthorizeAdminCapability(ctx context.Context, raw Secret, required Capability, activity ActivityKind) (Principal, error) {
	if required == "" {
		return Principal{}, ErrForbidden
	}
	if raw == "" || activity < UserOperation || activity > TokenRenewal {
		return Principal{}, ErrUnauthorized
	}
	hash := CredentialHash(raw)
	var result Principal
	var denied error
	err := s.repo.WithLockedAdminAccount(ctx, AdminAccountLookup{AdminSessionHash: hash}, func(account Account, tx AdminSessionTransaction) error {
		now := s.now().UTC()
		if account.State != AccountActive || (account.BlockedUntil != nil && now.Before(*account.BlockedUntil)) {
			denied = ErrUnauthorized
			return nil
		}
		if account.AdministrativeBlocked {
			denied = ErrRestrictedMode
			return nil
		}
		session, err := tx.AdminSessionByTokenHash(ctx, hash)
		if err != nil {
			return err
		}
		if session.AccountID != account.ID || now.Before(session.CreatedAt) || !session.ActiveAt(now) {
			denied = ErrUnauthorized
			return nil
		}
		roles, err := tx.Roles(ctx, account.ID)
		if err != nil {
			return err
		}
		if !isAdminStaff(roles) || !HasCapability(roles, required) {
			denied = ErrForbidden
			return nil
		}
		if activity == UserOperation {
			updated, err := tx.TouchAdminSession(ctx, session.ID, now)
			if err != nil {
				return err
			}
			if !updated {
				denied = ErrUnauthorized
				return nil
			}
		}
		var authenticatedAt time.Time
		if session.MFAVerifiedAt != nil {
			authenticatedAt = session.MFAVerifiedAt.UTC()
		}
		result = Principal{
			AccountID:       account.ID,
			Roles:           roles,
			Capabilities:    CapabilitiesForRoles(roles),
			RestrictedMode:  account.AdministrativeBlocked,
			AuthenticatedAt: authenticatedAt,
			SessionID:       session.ID,
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	return result, denied
}

func (s *AdminSessionService) newAdminSession(accountID string, now time.Time, mfaVerifiedAt *time.Time, mfaMethod, userAgent, ip string) (AdminSession, Secret, error) {
	raw, err := s.credentials.Token()
	if err != nil {
		return AdminSession{}, "", err
	}
	id, err := s.credentials.ID()
	if err != nil {
		return AdminSession{}, "", err
	}
	return AdminSession{
		ID:             id,
		AccountID:      accountID,
		Audience:       AdminSessionAudience,
		TokenHash:      CredentialHash(raw),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(AdminSessionDuration),
		MFAVerifiedAt:  mfaVerifiedAt,
		MFAMethod:      mfaMethod,
		DeviceLabel:    DeriveDeviceLabel(userAgent),
		IPMasked:       MaskIP(ip),
		UserAgent:      SanitizeUserAgent(userAgent),
	}, raw, nil
}
