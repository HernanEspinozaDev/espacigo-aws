package adminglobal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	approvalTTL               = 24 * time.Hour
	approvalCreateAuditAction = "admin.global.approvals.create"
	approvalDecideAuditAction = "admin.global.approvals.decide"
	privacyCreateAuditAction  = "admin.global.privacy_requests.create"
	ticketCreateAuditAction   = "admin.global.support_tickets.create"
	defaultOpsPageSize        = 25
	maxOpsPageSize            = 100
)

// ErrSelfApprovalForbidden indicates an administrator attempted to decide their own approval request.
var ErrSelfApprovalForbidden = errors.New("adminglobal: self approval forbidden")

// SpecializedOpsStore provides an optional persistence override for specialized admin operations and approvals.
type SpecializedOpsStore interface {
	ListPayments(context.Context, string, string, PaymentFilter) (OpsPage[PaymentItem], error)
	ListReservations(context.Context, string, string, ReservationFilter) (OpsPage[ReservationItem], error)
	ListContracts(context.Context, string, string, ContractFilter) (OpsPage[ContractItem], error)
	ListKYCCases(context.Context, string, string, KYCCaseFilter) (OpsPage[KYCCaseItem], error)
	ListPrivacyRequests(context.Context, string, string, PrivacyRequestFilter) (OpsPage[PrivacyRequestItem], error)
	CreatePrivacyRequest(context.Context, CreatePrivacyRequestInput) (PrivacyRequestItem, error)
	ListSupportTickets(context.Context, string, string, SupportTicketFilter) (OpsPage[SupportTicketItem], error)
	CreateSupportTicket(context.Context, CreateSupportTicketInput) (SupportTicketItem, error)
	ListApprovals(context.Context, string, string, ApprovalFilter) (OpsPage[ApprovalItem], error)
	CreateApproval(context.Context, CreateApprovalInput) (ApprovalItem, error)
	DecideApproval(context.Context, DecideApprovalInput) (ApprovalItem, error)
}

// SetSpecializedOpsStore configures an optional store override for specialized operations and approvals.
func (s *Service) SetSpecializedOpsStore(store SpecializedOpsStore) {
	s.specializedOpsStore = store
}

type OpsPage[T any] struct {
	Items         []T       `json:"items"`
	NextCursor    string    `json:"next_cursor"`
	MatchingTotal int64     `json:"matching_total"`
	GeneratedAt   time.Time `json:"generated_at"`
}

type opsCursor struct {
	Version int       `json:"v"`
	Filter  string    `json:"f"`
	At      time.Time `json:"a"`
	ID      string    `json:"i"`
}

type PaymentFilter struct {
	Status    string `json:"status"`
	TestMode  *bool  `json:"test_mode"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type PaymentItem struct {
	ID                    string     `json:"id"`
	QuoteID               string     `json:"quote_id"`
	SpaceID               string     `json:"space_id"`
	SpaceTitle            string     `json:"space_title"`
	RenterID              string     `json:"renter_id"`
	HostID                string     `json:"host_id"`
	Status                string     `json:"status"`
	BaseCLP               int64      `json:"base_clp"`
	CommissionCLP         int64      `json:"commission_clp"`
	IVACLP                int64      `json:"iva_clp"`
	BuyerFeeCLP           int64      `json:"buyer_fee_clp"`
	TotalChargedCLP       int64      `json:"total_charged_clp"`
	ProviderPaymentID     string     `json:"provider_payment_id"`
	ProviderPaymentStatus string     `json:"provider_payment_status"`
	PaymentVerifiedAt     *time.Time `json:"payment_verified_at,omitempty"`
	ReconciliationReason  string     `json:"reconciliation_reason"`
	TestMode              bool       `json:"test_mode"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ReservationFilter struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type ReservationItem struct {
	ID                  string    `json:"id"`
	CheckoutOperationID string    `json:"checkout_operation_id"`
	QuoteID             string    `json:"quote_id"`
	SpaceID             string    `json:"space_id"`
	SpaceTitle          string    `json:"space_title"`
	HostID              string    `json:"host_id"`
	RenterID            string    `json:"renter_id"`
	Status              string    `json:"status"`
	Modality            string    `json:"modality"`
	UnitPriceCLP        int64     `json:"unit_price_clp"`
	Units               int64     `json:"units"`
	SubtotalCLP         int64     `json:"subtotal_clp"`
	Currency            string    `json:"currency"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ContractFilter struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type ContractItem struct {
	ID              string    `json:"id"`
	ProposalID      string    `json:"proposal_id"`
	QuoteID         string    `json:"quote_id"`
	SpaceID         string    `json:"space_id"`
	SpaceTitle      string    `json:"space_title"`
	OwnerID         string    `json:"owner_id"`
	RenterID        string    `json:"renter_id"`
	Status          string    `json:"status"`
	ProposalVersion int64     `json:"proposal_version"`
	SubtotalCLP     int64     `json:"subtotal_clp"`
	Currency        string    `json:"currency"`
	DocumentStatus  string    `json:"document_status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type KYCCaseFilter struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type KYCCaseItem struct {
	ID              string     `json:"id"`
	AccountID       string     `json:"account_id"`
	Status          string     `json:"status"`
	Environment     string     `json:"environment"`
	IDCountry       string     `json:"id_country"`
	DocumentType    string     `json:"document_type"`
	DecisionStatus  string     `json:"decision_status"`
	RejectionReason string     `json:"rejection_reason"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PrivacyRequestFilter struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type PrivacyRequestItem struct {
	ID          string     `json:"id"`
	AccountID   string     `json:"account_id"`
	RightType   string     `json:"right_type"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason"`
	CaseRef     string     `json:"case_ref"`
	RequestedBy string     `json:"requested_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type CreatePrivacyRequestInput struct {
	ActorID       string
	CorrelationID string
	AccountID     string
	RightType     string
	Reason        string
	CaseRef       string
}

type SupportTicketFilter struct {
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	AccountID string `json:"account_id"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type SupportTicketItem struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id,omitempty"`
	AssignedToID string    `json:"assigned_to_id,omitempty"`
	Category     string    `json:"category"`
	Priority     string    `json:"priority"`
	Status       string    `json:"status"`
	Subject      string    `json:"subject"`
	CaseRef      string    `json:"case_ref"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateSupportTicketInput struct {
	ActorID       string
	CorrelationID string
	AccountID     string
	AssignedToID  string
	Category      string
	Priority      string
	Subject       string
	CaseRef       string
}

type ApprovalFilter struct {
	Status     string `json:"status"`
	ActionType string `json:"action_type"`
	PageSize   int    `json:"-"`
	Cursor     string `json:"-"`
}

type ApprovalItem struct {
	ID             string          `json:"id"`
	ActionType     string          `json:"action_type"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	RequesterID    string          `json:"requester_id"`
	ApproverID     string          `json:"approver_id,omitempty"`
	Status         string          `json:"status"`
	Reason         string          `json:"reason"`
	DecisionReason string          `json:"decision_reason"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	DecidedAt      *time.Time      `json:"decided_at,omitempty"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

type CreateApprovalInput struct {
	ActorID        string          `json:"-"`
	CorrelationID  string          `json:"-"`
	ActionType     string          `json:"action_type"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	Reason         string          `json:"reason"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
}

type DecideApprovalInput struct {
	ApprovalID      string
	ActorID         string
	CorrelationID   string
	Decision        string
	DecisionReason  string
	ExpectedVersion int
}

func validateOpsListArgs(actor, correlation, encodedCursor string, pageSize int, filterObj any) (int, string, *opsCursor, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) || len(encodedCursor) > 2048 {
		return 0, "", nil, ErrInvalid
	}
	if pageSize == 0 {
		pageSize = defaultOpsPageSize
	}
	if pageSize < 1 || pageSize > maxOpsPageSize {
		return 0, "", nil, ErrInvalid
	}
	rawFilter, err := json.Marshal(filterObj)
	if err != nil {
		return 0, "", nil, err
	}
	sum := sha256.Sum256(rawFilter)
	filterHash := hex.EncodeToString(sum[:])
	if encodedCursor == "" {
		return pageSize, filterHash, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encodedCursor)
	if err != nil || len(raw) > 1024 {
		return 0, "", nil, ErrInvalid
	}
	var cursor opsCursor
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.Filter != filterHash || cursor.At.IsZero() || !canonicalUUID.MatchString(cursor.ID) {
		return 0, "", nil, ErrInvalid
	}
	return pageSize, filterHash, &cursor, nil
}

func encodeOpsCursor(filterHash string, at time.Time, id string) (string, error) {
	raw, err := json.Marshal(opsCursor{Version: 1, Filter: filterHash, At: at.UTC(), ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateOptionalUUID(v string) bool {
	return v == "" || canonicalUUID.MatchString(v)
}

func (s *Service) beginGlobalReadTx(ctx context.Context, actor string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	if err := requireGlobalActor(ctx, tx, actor); err != nil {
		_ = tx.Rollback(context.Background())
		return nil, err
	}
	return tx, nil
}

func (s *Service) ListPayments(ctx context.Context, actor, correlation string, filter PaymentFilter) (OpsPage[PaymentItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[PaymentItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[PaymentItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListPayments(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[PaymentItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildPaymentWhere(filter)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.checkout_preferencia_comercial c`+where, args...).Scan(&total); err != nil {
		return OpsPage[PaymentItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "c.actualizada_en", "c.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT c.id::text, c.cotizacion_id::text, c.espacio_id::text, e.titulo, c.arrendatario_id::text, c.host_id::text,
		       c.estado, c.base_clp, c.commission_clp, c.iva_clp, c.buyer_fee_clp, c.total_charged_clp,
		       COALESCE(c.provider_payment_id, ''), COALESCE(c.provider_payment_status, ''), c.payment_verified_at,
		       COALESCE(c.reconciliation_reason, ''), c.test_mode, c.creada_en, c.actualizada_en
		FROM public.checkout_preferencia_comercial c
		JOIN public.espacio e ON e.id = c.espacio_id`+cursorClause+`
		ORDER BY c.actualizada_en DESC, c.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[PaymentItem]{}, err
	}
	defer rows.Close()
	items := make([]PaymentItem, 0, pageSize+1)
	for rows.Next() {
		var item PaymentItem
		if err := rows.Scan(
			&item.ID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle, &item.RenterID, &item.HostID,
			&item.Status, &item.BaseCLP, &item.CommissionCLP, &item.IVACLP, &item.BuyerFeeCLP, &item.TotalChargedCLP,
			&item.ProviderPaymentID, &item.ProviderPaymentStatus, &item.PaymentVerifiedAt,
			&item.ReconciliationReason, &item.TestMode, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return OpsPage[PaymentItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[PaymentItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it PaymentItem) (time.Time, string) { return it.UpdatedAt, it.ID })
}

func (s *Service) ListReservations(ctx context.Context, actor, correlation string, filter ReservationFilter) (OpsPage[ReservationItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[ReservationItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[ReservationItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListReservations(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[ReservationItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildAccountStatusWhere("r.estado", "r.anfitrion_id", "r.arrendatario_id", filter.Status, filter.AccountID)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.reserva_arriendo_comercial r`+where, args...).Scan(&total); err != nil {
		return OpsPage[ReservationItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "r.actualizada_en", "r.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT r.id::text, r.operacion_checkout_id::text, r.cotizacion_id::text, r.espacio_id::text, e.titulo,
		       r.anfitrion_id::text, r.arrendatario_id::text, r.estado, r.modalidad, r.precio_unitario_clp,
		       r.unidades, r.subtotal_clp, r.moneda, r.inicio, r.termino, r.creada_en, r.actualizada_en
		FROM public.reserva_arriendo_comercial r
		JOIN public.espacio e ON e.id = r.espacio_id`+cursorClause+`
		ORDER BY r.actualizada_en DESC, r.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[ReservationItem]{}, err
	}
	defer rows.Close()
	items := make([]ReservationItem, 0, pageSize+1)
	for rows.Next() {
		var item ReservationItem
		if err := rows.Scan(
			&item.ID, &item.CheckoutOperationID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle,
			&item.HostID, &item.RenterID, &item.Status, &item.Modality, &item.UnitPriceCLP,
			&item.Units, &item.SubtotalCLP, &item.Currency, &item.StartAt, &item.EndAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return OpsPage[ReservationItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[ReservationItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it ReservationItem) (time.Time, string) { return it.UpdatedAt, it.ID })
}

func (s *Service) ListContracts(ctx context.Context, actor, correlation string, filter ContractFilter) (OpsPage[ContractItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[ContractItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[ContractItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListContracts(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[ContractItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildAccountStatusWhere("s.estado", "s.propietario_id", "s.arrendatario_id", filter.Status, filter.AccountID)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.contrato_solicitud_comercial s`+where, args...).Scan(&total); err != nil {
		return OpsPage[ContractItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "s.actualizada_en", "s.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT s.id::text, s.propuesta_id::text, s.cotizacion_id::text, s.espacio_id::text, e.titulo,
		       s.propietario_id::text, s.arrendatario_id::text, s.estado, s.propuesta_version,
		       p.subtotal_clp, p.moneda, COALESCE(d.estado, 'none'), s.creada_en, s.actualizada_en
		FROM public.contrato_solicitud_comercial s
		JOIN public.contrato_propuesta_comercial p ON p.id = s.propuesta_id
		JOIN public.espacio e ON e.id = s.espacio_id
		LEFT JOIN public.contrato_documento_comercial d ON d.solicitud_id = s.id AND d.version = s.propuesta_version`+cursorClause+`
		ORDER BY s.actualizada_en DESC, s.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[ContractItem]{}, err
	}
	defer rows.Close()
	items := make([]ContractItem, 0, pageSize+1)
	for rows.Next() {
		var item ContractItem
		if err := rows.Scan(
			&item.ID, &item.ProposalID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle,
			&item.OwnerID, &item.RenterID, &item.Status, &item.ProposalVersion,
			&item.SubtotalCLP, &item.Currency, &item.DocumentStatus, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return OpsPage[ContractItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[ContractItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it ContractItem) (time.Time, string) { return it.UpdatedAt, it.ID })
}

func (s *Service) ListKYCCases(ctx context.Context, actor, correlation string, filter KYCCaseFilter) (OpsPage[KYCCaseItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[KYCCaseItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[KYCCaseItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListKYCCases(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[KYCCaseItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildSingleAccountStatusWhere("k.status", "k.account_id", filter.Status, filter.AccountID)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.identity_verification_attempts k`+where, args...).Scan(&total); err != nil {
		return OpsPage[KYCCaseItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "k.updated_at", "k.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT k.id::text, k.account_id::text, k.status, k.environment, k.id_country, k.document_type,
		       COALESCE(k.decision_status, ''), COALESCE(k.rejection_reason, ''), k.verified_at, k.created_at, k.updated_at
		FROM public.identity_verification_attempts k`+cursorClause+`
		ORDER BY k.updated_at DESC, k.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[KYCCaseItem]{}, err
	}
	defer rows.Close()
	items := make([]KYCCaseItem, 0, pageSize+1)
	for rows.Next() {
		var item KYCCaseItem
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.Status, &item.Environment, &item.IDCountry, &item.DocumentType,
			&item.DecisionStatus, &item.RejectionReason, &item.VerifiedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return OpsPage[KYCCaseItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[KYCCaseItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it KYCCaseItem) (time.Time, string) { return it.UpdatedAt, it.ID })
}

func (s *Service) ListPrivacyRequests(ctx context.Context, actor, correlation string, filter PrivacyRequestFilter) (OpsPage[PrivacyRequestItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[PrivacyRequestItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[PrivacyRequestItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListPrivacyRequests(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[PrivacyRequestItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildSingleAccountStatusWhere("p.estado", "p.cuenta_id", filter.Status, filter.AccountID)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.solicitud_privacidad_administrativa p`+where, args...).Scan(&total); err != nil {
		return OpsPage[PrivacyRequestItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "p.creado_en", "p.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT p.id::text, p.cuenta_id::text, p.tipo_derecho, p.estado, p.motivo, p.caso_ref,
		       COALESCE(p.solicitado_por::text, ''), p.creado_en, p.actualizado_en, p.resuelto_en
		FROM public.solicitud_privacidad_administrativa p`+cursorClause+`
		ORDER BY p.creado_en DESC, p.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[PrivacyRequestItem]{}, err
	}
	defer rows.Close()
	items := make([]PrivacyRequestItem, 0, pageSize+1)
	for rows.Next() {
		var item PrivacyRequestItem
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.RightType, &item.Status, &item.Reason, &item.CaseRef,
			&item.RequestedBy, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt,
		); err != nil {
			return OpsPage[PrivacyRequestItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[PrivacyRequestItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it PrivacyRequestItem) (time.Time, string) { return it.CreatedAt, it.ID })
}

func (s *Service) CreatePrivacyRequest(ctx context.Context, input CreatePrivacyRequestInput) (PrivacyRequestItem, error) {
	reason := strings.TrimSpace(input.Reason)
	caseRef := strings.TrimSpace(input.CaseRef)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.AccountID) || !validPrivacyRightType(input.RightType) ||
		utf8.RuneCountInString(reason) < 5 || utf8.RuneCountInString(reason) > 500 || utf8.RuneCountInString(caseRef) > 120 {
		return PrivacyRequestItem{}, ErrInvalid
	}
	input.Reason = reason
	input.CaseRef = caseRef
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.CreatePrivacyRequest(ctx, input)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyRequestItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyRequestItem{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id=$1)`, input.AccountID).Scan(&exists); err != nil {
		return PrivacyRequestItem{}, err
	}
	if !exists {
		return PrivacyRequestItem{}, ErrNotFound
	}
	id, err := s.ids.ID()
	if err != nil {
		return PrivacyRequestItem{}, err
	}
	now := s.now().UTC()
	var item PrivacyRequestItem
	err = tx.QueryRow(ctx, `
		INSERT INTO public.solicitud_privacidad_administrativa (
			id, cuenta_id, tipo_derecho, estado, motivo, caso_ref, solicitado_por, creado_en, actualizado_en
		) VALUES ($1, $2, $3, 'pendiente', $4, $5, $6, $7, $7)
		RETURNING id::text, cuenta_id::text, tipo_derecho, estado, motivo, caso_ref, COALESCE(solicitado_por::text, ''), creado_en, actualizado_en, resuelto_en
	`, id, input.AccountID, input.RightType, reason, caseRef, input.ActorID, now).Scan(
		&item.ID, &item.AccountID, &item.RightType, &item.Status, &item.Reason, &item.CaseRef,
		&item.RequestedBy, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt,
	)
	if err != nil {
		return PrivacyRequestItem{}, err
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, privacyCreateAuditAction, "exito", input.RightType, input.CorrelationID, now, input.AccountID); err != nil {
		return PrivacyRequestItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyRequestItem{}, err
	}
	return item, nil
}

func (s *Service) ListSupportTickets(ctx context.Context, actor, correlation string, filter SupportTicketFilter) (OpsPage[SupportTicketItem], error) {
	if !validateOptionalUUID(filter.AccountID) {
		return OpsPage[SupportTicketItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[SupportTicketItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListSupportTickets(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[SupportTicketItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildTicketWhere(filter)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.ticket_soporte_administrativo t`+where, args...).Scan(&total); err != nil {
		return OpsPage[SupportTicketItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "t.creado_en", "t.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT t.id::text, COALESCE(t.cuenta_id::text, ''), COALESCE(t.asignado_a_id::text, ''),
		       t.categoria, t.prioridad, t.estado, t.asunto, t.caso_ref, t.creado_en, t.actualizado_en
		FROM public.ticket_soporte_administrativo t`+cursorClause+`
		ORDER BY t.creado_en DESC, t.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[SupportTicketItem]{}, err
	}
	defer rows.Close()
	items := make([]SupportTicketItem, 0, pageSize+1)
	for rows.Next() {
		var item SupportTicketItem
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.AssignedToID,
			&item.Category, &item.Priority, &item.Status, &item.Subject, &item.CaseRef, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return OpsPage[SupportTicketItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[SupportTicketItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it SupportTicketItem) (time.Time, string) { return it.CreatedAt, it.ID })
}

func (s *Service) CreateSupportTicket(ctx context.Context, input CreateSupportTicketInput) (SupportTicketItem, error) {
	category := strings.TrimSpace(input.Category)
	subject := strings.TrimSpace(input.Subject)
	caseRef := strings.TrimSpace(input.CaseRef)
	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = "media"
	}
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!validateOptionalUUID(input.AccountID) || !validateOptionalUUID(input.AssignedToID) ||
		utf8.RuneCountInString(category) < 2 || utf8.RuneCountInString(category) > 64 ||
		!validTicketPriority(priority) ||
		utf8.RuneCountInString(subject) < 3 || utf8.RuneCountInString(subject) > 240 ||
		utf8.RuneCountInString(caseRef) > 120 {
		return SupportTicketItem{}, ErrInvalid
	}
	input.Category = category
	input.Priority = priority
	input.Subject = subject
	input.CaseRef = caseRef
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.CreateSupportTicket(ctx, input)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SupportTicketItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return SupportTicketItem{}, err
	}
	id, err := s.ids.ID()
	if err != nil {
		return SupportTicketItem{}, err
	}
	now := s.now().UTC()
	var accountArg, assignedArg any
	if input.AccountID != "" {
		accountArg = input.AccountID
	}
	if input.AssignedToID != "" {
		assignedArg = input.AssignedToID
	}
	var item SupportTicketItem
	err = tx.QueryRow(ctx, `
		INSERT INTO public.ticket_soporte_administrativo (
			id, cuenta_id, asignado_a_id, categoria, prioridad, estado, asunto, caso_ref, creado_en, actualizado_en
		) VALUES ($1, $2, $3, $4, $5, 'abierto', $6, $7, $8, $8)
		RETURNING id::text, COALESCE(cuenta_id::text, ''), COALESCE(asignado_a_id::text, ''), categoria, prioridad, estado, asunto, caso_ref, creado_en, actualizado_en
	`, id, accountArg, assignedArg, category, priority, subject, caseRef, now).Scan(
		&item.ID, &item.AccountID, &item.AssignedToID,
		&item.Category, &item.Priority, &item.Status, &item.Subject, &item.CaseRef, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return SupportTicketItem{}, err
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, ticketCreateAuditAction, "exito", category, input.CorrelationID, now, input.AccountID); err != nil {
		return SupportTicketItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportTicketItem{}, err
	}
	return item, nil
}

func (s *Service) ListApprovals(ctx context.Context, actor, correlation string, filter ApprovalFilter) (OpsPage[ApprovalItem], error) {
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[ApprovalItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListApprovals(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[ApprovalItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildApprovalWhere(filter)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.aprobacion_administrativa a`+where, args...).Scan(&total); err != nil {
		return OpsPage[ApprovalItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "a.creada_en", "a.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT a.id::text, a.tipo_accion, a.recurso_tipo, a.recurso_id, a.solicitante_id::text,
		       COALESCE(a.aprobador_id::text, ''), a.estado, a.motivo, a.decision_motivo, a.payload,
		       a.idempotency_key, a.version, a.creada_en, a.decidida_en, a.expira_en
		FROM public.aprobacion_administrativa a`+cursorClause+`
		ORDER BY a.creada_en DESC, a.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[ApprovalItem]{}, err
	}
	defer rows.Close()
	items := make([]ApprovalItem, 0, pageSize+1)
	for rows.Next() {
		var item ApprovalItem
		if err := rows.Scan(
			&item.ID, &item.ActionType, &item.ResourceType, &item.ResourceID, &item.RequesterID,
			&item.ApproverID, &item.Status, &item.Reason, &item.DecisionReason, &item.Payload,
			&item.IdempotencyKey, &item.Version, &item.CreatedAt, &item.DecidedAt, &item.ExpiresAt,
		); err != nil {
			return OpsPage[ApprovalItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[ApprovalItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.now().UTC(), func(it ApprovalItem) (time.Time, string) { return it.CreatedAt, it.ID })
}

func (s *Service) CreateApproval(ctx context.Context, input CreateApprovalInput) (ApprovalItem, error) {
	reason := strings.TrimSpace(input.Reason)
	idemKey := strings.TrimSpace(input.IdempotencyKey)
	resourceType := strings.TrimSpace(input.ResourceType)
	resourceID := strings.TrimSpace(input.ResourceID)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!validApprovalActionType(input.ActionType) ||
		utf8.RuneCountInString(resourceType) < 1 || utf8.RuneCountInString(resourceType) > 64 ||
		utf8.RuneCountInString(resourceID) < 1 || utf8.RuneCountInString(resourceID) > 128 ||
		utf8.RuneCountInString(reason) < 5 || utf8.RuneCountInString(reason) > 500 ||
		len(idemKey) < 1 || len(idemKey) > 200 {
		return ApprovalItem{}, ErrInvalid
	}
	payload := input.Payload
	if len(bytes.TrimSpace(payload)) == 0 {
		payload = json.RawMessage(`{}`)
	} else if !json.Valid(payload) {
		return ApprovalItem{}, ErrInvalid
	}
	input.Reason = reason
	input.IdempotencyKey = idemKey
	input.ResourceType = resourceType
	input.ResourceID = resourceID
	input.Payload = payload
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.CreateApproval(ctx, input)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ApprovalItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return ApprovalItem{}, err
	}
	id, err := s.ids.ID()
	if err != nil {
		return ApprovalItem{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(approvalTTL)
	var item ApprovalItem
	err = tx.QueryRow(ctx, `
		INSERT INTO public.aprobacion_administrativa (
			id, tipo_accion, recurso_tipo, recurso_id, solicitante_id, estado, motivo, payload, idempotency_key, version, creada_en, expira_en
		) VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, 1, $9, $10)
		RETURNING id::text, tipo_accion, recurso_tipo, recurso_id, solicitante_id::text,
		          COALESCE(aprobador_id::text, ''), estado, motivo, decision_motivo, payload,
		          idempotency_key, version, creada_en, decidida_en, expira_en
	`, id, input.ActionType, resourceType, resourceID, input.ActorID, reason, payload, idemKey, now, expiresAt).Scan(
		&item.ID, &item.ActionType, &item.ResourceType, &item.ResourceID, &item.RequesterID,
		&item.ApproverID, &item.Status, &item.Reason, &item.DecisionReason, &item.Payload,
		&item.IdempotencyKey, &item.Version, &item.CreatedAt, &item.DecidedAt, &item.ExpiresAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ApprovalItem{}, ErrConflict
		}
		return ApprovalItem{}, err
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, approvalCreateAuditAction, "exito", input.ActionType, input.CorrelationID, now, ""); err != nil {
		return ApprovalItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ApprovalItem{}, err
	}
	return item, nil
}

func (s *Service) DecideApproval(ctx context.Context, input DecideApprovalInput) (ApprovalItem, error) {
	decisionReason := strings.TrimSpace(input.DecisionReason)
	if !canonicalUUID.MatchString(input.ApprovalID) || !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!validApprovalDecision(input.Decision) || input.ExpectedVersion < 1 ||
		utf8.RuneCountInString(decisionReason) < 5 || utf8.RuneCountInString(decisionReason) > 500 {
		return ApprovalItem{}, ErrInvalid
	}
	input.DecisionReason = decisionReason
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.DecideApproval(ctx, input)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ApprovalItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return ApprovalItem{}, err
	}
	var requesterID, status string
	var version int
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT solicitante_id::text, estado, version, expira_en
		FROM public.aprobacion_administrativa
		WHERE id = $1
		FOR UPDATE
	`, input.ApprovalID).Scan(&requesterID, &status, &version, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApprovalItem{}, ErrNotFound
	}
	if err != nil {
		return ApprovalItem{}, err
	}
	if requesterID == input.ActorID {
		return ApprovalItem{}, ErrSelfApprovalForbidden
	}
	now := s.now().UTC()
	if status != "pending" || !now.Before(expiresAt) || version != input.ExpectedVersion {
		return ApprovalItem{}, ErrConflict
	}
	var item ApprovalItem
	err = tx.QueryRow(ctx, `
		UPDATE public.aprobacion_administrativa
		SET aprobador_id = $2, estado = $3, decision_motivo = $4, decidida_en = $5, version = version + 1
		WHERE id = $1 AND version = $6
		RETURNING id::text, tipo_accion, recurso_tipo, recurso_id, solicitante_id::text,
		          COALESCE(aprobador_id::text, ''), estado, motivo, decision_motivo, payload,
		          idempotency_key, version, creada_en, decidida_en, expira_en
	`, input.ApprovalID, input.ActorID, input.Decision, decisionReason, now, input.ExpectedVersion).Scan(
		&item.ID, &item.ActionType, &item.ResourceType, &item.ResourceID, &item.RequesterID,
		&item.ApproverID, &item.Status, &item.Reason, &item.DecisionReason, &item.Payload,
		&item.IdempotencyKey, &item.Version, &item.CreatedAt, &item.DecidedAt, &item.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApprovalItem{}, ErrConflict
	}
	if err != nil {
		return ApprovalItem{}, err
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, approvalDecideAuditAction, "exito", input.Decision, input.CorrelationID, now, requesterID); err != nil {
		return ApprovalItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ApprovalItem{}, err
	}
	return item, nil
}

func validPrivacyRightType(v string) bool {
	switch v {
	case "acceso", "rectificacion", "supresion", "oposicion", "portabilidad", "bloqueo":
		return true
	}
	return false
}

func validTicketPriority(v string) bool {
	switch v {
	case "baja", "media", "alta", "critica":
		return true
	}
	return false
}

func validApprovalActionType(v string) bool {
	switch v {
	case "refund_request", "privacy_export", "staff_role_grant":
		return true
	}
	return false
}

func validApprovalDecision(v string) bool {
	switch v {
	case "approved", "rejected", "cancelled":
		return true
	}
	return false
}

func buildPaymentWhere(f PaymentFilter) (string, []any) {
	conds := make([]string, 0, 3)
	args := make([]any, 0, 6)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "c.estado = $"+strconv.Itoa(len(args)))
	}
	if f.TestMode != nil {
		args = append(args, *f.TestMode)
		conds = append(conds, "c.test_mode = $"+strconv.Itoa(len(args)))
	}
	if f.AccountID != "" {
		args = append(args, f.AccountID)
		n := strconv.Itoa(len(args))
		conds = append(conds, "(c.arrendatario_id = $"+n+" OR c.host_id = $"+n+")")
	}
	return joinWhere(conds), args
}

func buildAccountStatusWhere(statusCol, colA, colB, status, accountID string) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 5)
	if status != "" {
		args = append(args, status)
		conds = append(conds, statusCol+" = $"+strconv.Itoa(len(args)))
	}
	if accountID != "" {
		args = append(args, accountID)
		n := strconv.Itoa(len(args))
		conds = append(conds, "("+colA+" = $"+n+" OR "+colB+" = $"+n+")")
	}
	return joinWhere(conds), args
}

func buildSingleAccountStatusWhere(statusCol, accountCol, status, accountID string) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 5)
	if status != "" {
		args = append(args, status)
		conds = append(conds, statusCol+" = $"+strconv.Itoa(len(args)))
	}
	if accountID != "" {
		args = append(args, accountID)
		conds = append(conds, accountCol+" = $"+strconv.Itoa(len(args)))
	}
	return joinWhere(conds), args
}

func buildTicketWhere(f SupportTicketFilter) (string, []any) {
	conds := make([]string, 0, 3)
	args := make([]any, 0, 6)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "t.estado = $"+strconv.Itoa(len(args)))
	}
	if f.Priority != "" {
		args = append(args, f.Priority)
		conds = append(conds, "t.prioridad = $"+strconv.Itoa(len(args)))
	}
	if f.AccountID != "" {
		args = append(args, f.AccountID)
		conds = append(conds, "t.cuenta_id = $"+strconv.Itoa(len(args)))
	}
	return joinWhere(conds), args
}

func buildApprovalWhere(f ApprovalFilter) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 5)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "a.estado = $"+strconv.Itoa(len(args)))
	}
	if f.ActionType != "" {
		args = append(args, f.ActionType)
		conds = append(conds, "a.tipo_accion = $"+strconv.Itoa(len(args)))
	}
	return joinWhere(conds), args
}

func joinWhere(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

func appendCursorClause(where string, args []any, atCol, idCol string, cursor *opsCursor, pageSize int) (string, []any) {
	if cursor != nil {
		args = append(args, cursor.At.UTC(), cursor.ID)
		atIdx := strconv.Itoa(len(args) - 1)
		idIdx := strconv.Itoa(len(args))
		cond := "(" + atCol + ", " + idCol + ") < ($" + atIdx + "::timestamptz, $" + idIdx + "::uuid)"
		if where == "" {
			where = " WHERE " + cond
		} else {
			where += " AND " + cond
		}
	}
	args = append(args, pageSize+1)
	return where, args
}

func finalizeOpsPage[T any](items []T, pageSize int, total int64, filterHash string, now time.Time, keyFn func(T) (time.Time, string)) (OpsPage[T], error) {
	nextCursor := ""
	if len(items) > pageSize {
		items = items[:pageSize]
		lastAt, lastID := keyFn(items[len(items)-1])
		encoded, err := encodeOpsCursor(filterHash, lastAt, lastID)
		if err != nil {
			return OpsPage[T]{}, err
		}
		nextCursor = encoded
	}
	return OpsPage[T]{
		Items:         items,
		NextCursor:    nextCursor,
		MatchingTotal: total,
		GeneratedAt:   now,
	}, nil
}
