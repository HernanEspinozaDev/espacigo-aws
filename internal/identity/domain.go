package identity

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
)

// AccountState is an account lifecycle state, never an HTTP status.
type AccountState string

const (
	AccountEmailPending     AccountState = "correo_pendiente"
	AccountActive           AccountState = "activo"
	AccountBlocked          AccountState = "bloqueado"
	AccountClosureRequested AccountState = "baja_solicitada"
	AccountDeidentified     AccountState = "desidentificado"
)

func (s AccountState) Valid() bool {
	switch s {
	case AccountEmailPending, AccountActive, AccountBlocked, AccountClosureRequested, AccountDeidentified:
		return true
	}
	return false
}

type Role string

const (
	RoleTenant              Role = "arrendatario"
	RoleLandlord            Role = "arrendador"
	RoleAdministrator       Role = "administrador"
	RoleGlobalAdministrator Role = "administrador_global"
)

// Capability is a server-authoritative permission for one administrative area.
// It is derived from current roles on every request; the browser never grants it.
type Capability string

const (
	CapabilityLegacyAdministration Capability = "legacy.administration"
	CapabilityAccountsList         Capability = "accounts.list"
	CapabilityAccountsRead         Capability = "accounts.read"
	CapabilityAccountsEmailReveal  Capability = "accounts.email.reveal"
	CapabilityAdminAccessManage    Capability = "admin_access.manage"
	CapabilityAccountsUpdate       Capability = "accounts.update"
	CapabilityAccountsBlock        Capability = "accounts.block"
	CapabilityAccountsRoles        Capability = "accounts.roles"
	CapabilityPublicationsList     Capability = "publications.list"
	CapabilityPublicationsRead     Capability = "publications.read"
	CapabilityPublicationsUpdate   Capability = "publications.update"
	CapabilityPublicationsModerate Capability = "publications.moderate"
	CapabilityPublicationsRetire   Capability = "publications.retire"
	CapabilityAuditRead            Capability = "audit.read"
	CapabilityAuditExport          Capability = "audit.export"
	CapabilityKPIRead              Capability = "kpi.read"
	CapabilityReservationsRead     Capability = "reservations.read"
	CapabilityContractsRead        Capability = "contracts.read"
	CapabilityKYCCaseRead          Capability = "kyc.case.read"
	CapabilityPaymentsRead         Capability = "payments.read"
	CapabilityPaymentsReconcile    Capability = "payments.reconcile"
	CapabilityPaymentsRefundRequest Capability = "payments.refund.request"
	CapabilityPaymentsRefundApprove Capability = "payments.refund.approve"
	CapabilityPrivacyRequestsManage Capability = "privacy.requests.manage"
	CapabilityPrivacyExportApprove  Capability = "privacy.export.approve"
	CapabilityStaffRolesManage     Capability = "staff.roles.manage"
	CapabilityStaffSessionsRevoke  Capability = "staff.sessions.revoke"
	CapabilitySupportTicketsManage Capability = "support.tickets.manage"
	CapabilityApprovalsRequest     Capability = "approvals.request"
	CapabilityApprovalsDecide      Capability = "approvals.decide"
)

// SpecializedAdminCapabilities returns the granular operational capabilities for global administrators.
func SpecializedAdminCapabilities() []Capability {
	return []Capability{
		CapabilityReservationsRead,
		CapabilityContractsRead,
		CapabilityKYCCaseRead,
		CapabilityPaymentsRead,
		CapabilityPaymentsReconcile,
		CapabilityPaymentsRefundRequest,
		CapabilityPaymentsRefundApprove,
		CapabilityPrivacyRequestsManage,
		CapabilityPrivacyExportApprove,
		CapabilityStaffRolesManage,
		CapabilityStaffSessionsRevoke,
		CapabilitySupportTicketsManage,
		CapabilityApprovalsRequest,
		CapabilityApprovalsDecide,
	}
}

// CapabilitiesForRoles keeps the old administrator surface intact while
// granting the global role only the GADM-01 directory/bootstrap capabilities.
func CapabilitiesForRoles(roles []Role) []Capability {
	set := make(map[Capability]struct{})
	for _, role := range roles {
		switch role {
		case RoleAdministrator:
			set[CapabilityLegacyAdministration] = struct{}{}
		case RoleGlobalAdministrator:
			set[CapabilityLegacyAdministration] = struct{}{}
			set[CapabilityAccountsList] = struct{}{}
			set[CapabilityAccountsRead] = struct{}{}
			set[CapabilityAccountsEmailReveal] = struct{}{}
			set[CapabilityAdminAccessManage] = struct{}{}
			set[CapabilityAccountsUpdate] = struct{}{}
			set[CapabilityAccountsBlock] = struct{}{}
			set[CapabilityAccountsRoles] = struct{}{}
			set[CapabilityPublicationsList] = struct{}{}
			set[CapabilityPublicationsRead] = struct{}{}
			set[CapabilityPublicationsUpdate] = struct{}{}
			set[CapabilityPublicationsModerate] = struct{}{}
			set[CapabilityPublicationsRetire] = struct{}{}
			set[CapabilityAuditRead] = struct{}{}
			set[CapabilityAuditExport] = struct{}{}
			set[CapabilityKPIRead] = struct{}{}
			for _, cap := range SpecializedAdminCapabilities() {
				set[cap] = struct{}{}
			}
		}
	}
	capabilities := make([]Capability, 0, len(set))
	for capability := range set {
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	return capabilities
}

func HasCapability(roles []Role, required Capability) bool {
	if required == "" {
		return false
	}
	for _, capability := range CapabilitiesForRoles(roles) {
		if capability == required {
			return true
		}
	}
	return false
}

// HasAnyCapability returns true when the given roles grant at least one of the required capabilities.
func HasAnyCapability(roles []Role, required ...Capability) bool {
	for _, cap := range required {
		if HasCapability(roles, cap) {
			return true
		}
	}
	return false
}

// Role is deliberately explicit: registration must never grant administrator privileges.
func RegistrationRoles() []Role { return nil }

type Account struct {
	ID                    string
	Email                 string
	NormalizedEmail       string
	PasswordHash          Secret
	UsePreference         string
	State                 AccountState
	CreatedAt             time.Time
	UpdatedAt             time.Time
	FailedAttempts        int
	BlockedUntil          *time.Time
	AdministrativeBlocked bool
}

// Secret prevents accidental disclosure in formatting and structured logs.
type Secret string

func (Secret) String() string               { return "[REDACTED]" }
func (Secret) GoString() string             { return "[REDACTED]" }
func (Secret) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

func NormalizeEmail(email string) string { return cases.Fold().String(strings.TrimSpace(email)) }

var (
	ErrNotFound = errors.New("identity: not found")
	ErrConflict = errors.New("identity: conflict")
	ErrInvalid  = errors.New("identity: invalid value")
)

func (a Account) Validate() error {
	if a.ID == "" || a.Email == "" || a.NormalizedEmail == "" || a.NormalizedEmail != NormalizeEmail(a.Email) || a.PasswordHash == "" || !a.State.Valid() || a.FailedAttempts < 0 || (a.UsePreference != "" && a.UsePreference != "ofrecer" && a.UsePreference != "arrendar") {
		return fmt.Errorf("%w: account", ErrInvalid)
	}
	return nil
}

type Session struct {
	ID, AccountID                        string
	TokenHash                            string `json:"-"`
	CreatedAt, LastActivityAt, ExpiresAt time.Time
	RevokedAt                            *time.Time
	ClientSummary                        string
	DeviceLabel                          string
	IPMasked                             string
	UserAgent                            string
}

func (Session) String() string   { return "Session{TokenHash:[REDACTED]}" }
func (Session) GoString() string { return "Session{TokenHash:[REDACTED]}" }

func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt) && now.Before(s.LastActivityAt.Add(30*time.Minute))
}

type TermsVersion struct {
	ID, Code, Type, SHA256 string
	PublishedAt            time.Time
}

type TermsAcceptance struct {
	ID, AccountID, VersionID, Channel string
	AcceptedAt                        time.Time
}

type ActionToken struct {
	ID, AccountID, Purpose    string
	Hash                      string `json:"-"`
	CreatedAt, ExpiresAt      time.Time
	ConsumedAt, InvalidatedAt *time.Time
	Attempts                  int
}

func (ActionToken) String() string   { return "ActionToken{Hash:[REDACTED]}" }
func (ActionToken) GoString() string { return "ActionToken{Hash:[REDACTED]}" }
