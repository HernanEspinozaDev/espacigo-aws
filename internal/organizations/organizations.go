package organizations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalid   = errors.New("organizations: invalid request")
	ErrNotFound  = errors.New("organizations: not found")
	ErrForbidden = errors.New("organizations: forbidden")
	ErrConflict  = errors.New("organizations: conflict")
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(s string) bool { return uuidPattern.MatchString(s) }

type Role string

const (
	LegalRepresentative Role = "representante_legal"
	Administrator       Role = "administrador"
	Operator            Role = "operador"
)

func validRole(r Role) bool { return r == LegalRepresentative || r == Administrator || r == Operator }

type Organization struct {
	ID               string     `json:"id"`
	LegalName        string     `json:"legal_name"`
	TradeName        string     `json:"trade_name,omitempty"`
	PublicSummary    string     `json:"public_summary,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	MyRole           Role       `json:"my_role,omitempty"`
	ComplianceStatus string     `json:"compliance_status,omitempty"`
	GraceDueAt       *time.Time `json:"grace_due_at,omitempty"`
}
type Member struct {
	ID       string     `json:"id"`
	UserID   string     `json:"usuario_id"`
	Role     Role       `json:"rol"`
	State    string     `json:"estado"`
	JoinedAt *time.Time `json:"joined_at,omitempty"`
}
type Invitation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizacion_id"`
	Email          string    `json:"target_email,omitempty"`
	Role           Role      `json:"rol_propuesto"`
	State          string    `json:"estado"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}
type JoinRequest struct {
	ID        string    `json:"id"`
	UserID    string    `json:"usuario_id,omitempty"`
	Role      Role      `json:"rol_propuesto"`
	State     string    `json:"estado"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
type Repository interface {
	ListMine(context.Context, string) ([]Organization, error)
	Create(context.Context, string, string, string, string, []string) (Organization, error)
	Get(context.Context, string, string) (Organization, error)
	Update(context.Context, string, string, string, string, string) (Organization, error)
	Members(context.Context, string, string) ([]Member, error)
	Invite(context.Context, string, string, string, Role, string, []byte, time.Time) (Invitation, error)
	Accept(context.Context, string, string, string, []byte, time.Time) (Member, error)
	Revoke(context.Context, string, string, string, time.Time) error
	SetRole(context.Context, string, string, string, Role, time.Time) error
	Role(context.Context, string, string) (Role, error)
	RequestJoin(context.Context, string, []string, Role, time.Time) (JoinRequest, error)
	OwnJoinRequest(context.Context, string, string) (JoinRequest, error)
	JoinRequests(context.Context, string, string) ([]JoinRequest, error)
	ApproveJoinRequest(context.Context, string, string, string, []byte, time.Time, time.Time) (Invitation, error)
	RejectJoinRequest(context.Context, string, string, string, time.Time) error
}
type Action string

const (
	ActionOperate         Action = "operate"
	ActionManageSpace     Action = "manage_space"
	ActionManageDocuments Action = "manage_documents"
	ActionSignContract    Action = "sign_contract"
	ActionManageKYB       Action = "manage_kyb"
	ActionManageTeam      Action = "manage_team"
	ActionManagePayouts   Action = "manage_payouts"
)

type RutFingerprinter interface {
	Fingerprints(string) (verification.RutFingerprintSet, error)
}
type Service struct {
	repo    Repository
	rutKeys RutFingerprinter
	now     func() time.Time
}

func New(repo Repository, rutKeys RutFingerprinter, now func() time.Time) (*Service, error) {
	if repo == nil || rutKeys == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, rutKeys: rutKeys, now: now}, nil
}
func (s *Service) List(ctx context.Context, user string) ([]Organization, error) {
	if s == nil || !validID(user) {
		return nil, ErrInvalid
	}
	return s.repo.ListMine(ctx, user)
}
func (s *Service) Create(ctx context.Context, user, legal, trade, companyRUT string) (Organization, error) {
	legal = strings.TrimSpace(legal)
	trade = strings.TrimSpace(trade)
	if s == nil || !validID(user) || len([]rune(legal)) < 2 || len([]rune(legal)) > 200 || len([]rune(trade)) > 200 {
		return Organization{}, ErrInvalid
	}
	normalized, err := verification.NormalizeRut(companyRUT)
	if err != nil {
		return Organization{}, ErrInvalid
	}
	fps, err := s.rutKeys.Fingerprints(normalized)
	if err != nil || verification.ValidateRutFingerprintSet(fps) != nil {
		return Organization{}, ErrInvalid
	}
	return s.repo.Create(ctx, user, legal, trade, fps.Active, append([]string(nil), fps.Candidates...))
}
func (s *Service) Get(ctx context.Context, user, id string) (Organization, error) {
	if s == nil || !validID(user) || !validID(id) {
		return Organization{}, ErrInvalid
	}
	return s.repo.Get(ctx, user, id)
}
func (s *Service) Update(ctx context.Context, user, id, legal, trade, publicSummary string) (Organization, error) {
	legal = strings.TrimSpace(legal)
	trade = strings.TrimSpace(trade)
	publicSummary = strings.TrimSpace(publicSummary)
	if s == nil || !validID(user) || !validID(id) || len([]rune(legal)) < 2 || len([]rune(legal)) > 200 || len([]rune(trade)) > 200 || len([]rune(publicSummary)) > 2000 {
		return Organization{}, ErrInvalid
	}
	return s.repo.Update(ctx, user, id, legal, trade, publicSummary)
}
func (s *Service) Members(ctx context.Context, user, id string) ([]Member, error) {
	if s == nil || !validID(user) || !validID(id) {
		return nil, ErrInvalid
	}
	return s.repo.Members(ctx, user, id)
}
func (s *Service) Invite(ctx context.Context, user, id, email string, role Role) (Invitation, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if s == nil || !validID(user) || !validID(id) || !validRole(role) || !strings.Contains(email, "@") || len(email) > 254 {
		return Invitation{}, "", ErrInvalid
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Invitation{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := s.now().UTC().Add(7 * 24 * time.Hour)
	inv, err := s.repo.Invite(ctx, user, id, email, role, token, hash[:], expires)
	return inv, token, err
}
func (s *Service) Accept(ctx context.Context, user, id, invitationID, token string) (Member, error) {
	if s == nil || !validID(user) || !validID(id) || !validID(invitationID) || len(token) < 32 || len(token) > 128 {
		return Member{}, ErrInvalid
	}
	sum := sha256.Sum256([]byte(token))
	return s.repo.Accept(ctx, user, id, invitationID, sum[:], s.now().UTC())
}
func (s *Service) Revoke(ctx context.Context, user, id, member string) error {
	if s == nil || !validID(user) || !validID(id) || !validID(member) {
		return ErrInvalid
	}
	return s.repo.Revoke(ctx, user, id, member, s.now().UTC())
}
func (s *Service) SetRole(ctx context.Context, user, id, member string, role Role) error {
	if s == nil || !validID(user) || !validID(id) || !validID(member) || !validRole(role) {
		return ErrInvalid
	}
	return s.repo.SetRole(ctx, user, id, member, role, s.now().UTC())
}
func (s *Service) Authorize(ctx context.Context, actorID, organizationID string, action Action) (Role, error) {
	if s == nil || !validID(actorID) || !validID(organizationID) {
		return "", ErrInvalid
	}
	role, err := s.repo.Role(ctx, actorID, organizationID)
	if err != nil {
		return "", err
	}
	allowed := false
	switch action {
	case ActionOperate:
		allowed = validRole(role)
	case ActionManageSpace, ActionManageDocuments:
		allowed = role == LegalRepresentative || role == Administrator
	case ActionSignContract, ActionManageKYB, ActionManageTeam, ActionManagePayouts:
		allowed = role == LegalRepresentative
	default:
		return "", ErrInvalid
	}
	if !allowed {
		return "", ErrForbidden
	}
	return role, nil
}

func (s *Service) RequestJoin(ctx context.Context, user, companyRUT string, role Role) (JoinRequest, error) {
	if s == nil || !validID(user) || !validRole(role) {
		return JoinRequest{}, ErrInvalid
	}
	normalized, err := verification.NormalizeRut(companyRUT)
	if err != nil {
		return JoinRequest{}, ErrInvalid
	}
	fps, err := s.rutKeys.Fingerprints(normalized)
	if err != nil || verification.ValidateRutFingerprintSet(fps) != nil {
		return JoinRequest{}, ErrInvalid
	}
	request, err := s.repo.RequestJoin(ctx, user, append([]string(nil), fps.Candidates...), role, s.now().UTC())
	request.UserID = ""
	return request, err
}
func (s *Service) OwnJoinRequest(ctx context.Context, user, id string) (JoinRequest, error) {
	if s == nil || !validID(user) || !validID(id) {
		return JoinRequest{}, ErrInvalid
	}
	return s.repo.OwnJoinRequest(ctx, user, id)
}
func (s *Service) JoinRequests(ctx context.Context, actor, org string) ([]JoinRequest, error) {
	if s == nil || !validID(actor) || !validID(org) {
		return nil, ErrInvalid
	}
	role, err := s.repo.Role(ctx, actor, org)
	if err != nil {
		return nil, err
	}
	if role != LegalRepresentative {
		return nil, ErrForbidden
	}
	return s.repo.JoinRequests(ctx, actor, org)
}
func (s *Service) ApproveJoinRequest(ctx context.Context, actor, org, request string) (Invitation, string, error) {
	if s == nil || !validID(actor) || !validID(org) || !validID(request) {
		return Invitation{}, "", ErrInvalid
	}
	role, err := s.repo.Role(ctx, actor, org)
	if err != nil {
		return Invitation{}, "", err
	}
	if role != LegalRepresentative {
		return Invitation{}, "", ErrForbidden
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return Invitation{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	inv, err := s.repo.ApproveJoinRequest(ctx, actor, org, request, sum[:], now, now.Add(7*24*time.Hour))
	return inv, token, err
}
func (s *Service) RejectJoinRequest(ctx context.Context, actor, org, request string) error {
	if s == nil || !validID(actor) || !validID(org) || !validID(request) {
		return ErrInvalid
	}
	role, err := s.repo.Role(ctx, actor, org)
	if err != nil {
		return err
	}
	if role != LegalRepresentative {
		return ErrForbidden
	}
	return s.repo.RejectJoinRequest(ctx, actor, org, request, s.now().UTC())
}
