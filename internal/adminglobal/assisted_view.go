package adminglobal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	assistedViewTTL         = 15 * time.Minute
	assistedViewStartAction = "admin.global.assisted_view.start"
	assistedViewCloseAction = "admin.global.assisted_view.close"
)

// AssistedViewStore persists and resolves assisted client-view grants (used by tests or custom adapters).
type AssistedViewStore interface {
	StartAssistedView(context.Context, AssistedViewRecord) (AssistedViewGrant, error)
	GetAssistedViewProjection(context.Context, AssistedViewProjectionInput) (AssistedViewProjection, error)
	CloseAssistedView(context.Context, AssistedViewCloseInput) error
}

// SetAssistedViewStore configures an optional store override for assisted client views.
func (s *Service) SetAssistedViewStore(store AssistedViewStore) {
	s.assistedViewStore = store
}

type AssistedViewStartInput struct {
	ActorID        string
	TargetID       string
	AdminSessionID string
	CorrelationID  string
	Role           string
	CaseID         string
	Reason         string
}

type AssistedViewRecord struct {
	ID             string
	AdminSessionID string
	ActorID        string
	TargetID       string
	Role           string
	CaseID         string
	Reason         string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type AssistedViewGrant struct {
	GrantID        string    `json:"grant_id"`
	AccountID      string    `json:"account_id"`
	ActorAccountID string    `json:"actor_account_id"`
	Role           string    `json:"role"`
	CaseID         string    `json:"case_id"`
	Reason         string    `json:"reason"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type AssistedViewProjectionInput struct {
	GrantID        string
	ActorID        string
	AdminSessionID string
	CorrelationID  string
}

type AssistedViewCloseInput struct {
	GrantID        string
	ActorID        string
	AdminSessionID string
	CorrelationID  string
}

type AssistedViewAccountSummary struct {
	AccountID          string   `json:"account_id"`
	State              string   `json:"state"`
	EmailVerified      bool     `json:"email_verified"`
	PhoneVerified      bool     `json:"phone_verified"`
	KYCStatus          string   `json:"kyc_status"`
	KYCRejectionReason string   `json:"kyc_rejection_reason,omitempty"`
	Roles              []string `json:"roles"`
	AdminBlocked       bool     `json:"admin_blocked"`
}

type AssistedViewDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Area     string `json:"area"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

type AssistedViewPublishStep struct {
	Step   int    `json:"step"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type AssistedViewPublishGuide struct {
	KYCStatus         string                    `json:"kyc_status"`
	DraftStatus       string                    `json:"draft_status"`
	MercadoPagoStatus string                    `json:"mercadopago_status"`
	ContractStatus    string                    `json:"contract_status"`
	Steps             []AssistedViewPublishStep `json:"steps"`
}

type AssistedViewMercadoPago struct {
	Connected       bool       `json:"connected"`
	ConnectionState string     `json:"connection_state"`
	LiveMode        bool       `json:"live_mode"`
	ConnectedAt     *time.Time `json:"connected_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type AssistedViewSpace struct {
	ID                       string    `json:"id"`
	Title                    string    `json:"title"`
	State                    string    `json:"state"`
	CategoryCode             string    `json:"category_code"`
	Modality                 string    `json:"modality"`
	PriceCLP                 int64     `json:"price_clp"`
	MaxCapacity              int       `json:"max_capacity"`
	AreaM2                   float64   `json:"area_m2"`
	TimeZone                 string    `json:"time_zone"`
	LocationConfirmed        bool      `json:"location_confirmed"`
	ContractPublishedVersion int       `json:"contract_published_version"`
	PublishBlockers          []string  `json:"publish_blockers"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type AssistedViewReservation struct {
	ID               string     `json:"id"`
	SpaceID          string     `json:"space_id"`
	SpaceTitle       string     `json:"space_title"`
	State            string     `json:"state"`
	Modality         string     `json:"modality"`
	SubtotalCLP      int64      `json:"subtotal_clp"`
	Currency         string     `json:"currency"`
	StartAt          time.Time  `json:"start_at"`
	EndAt            time.Time  `json:"end_at"`
	PaymentExpiresAt *time.Time `json:"payment_expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type AssistedViewContract struct {
	ID             string    `json:"id"`
	ProposalID     string    `json:"proposal_id"`
	SpaceID        string    `json:"space_id"`
	SpaceTitle     string    `json:"space_title"`
	State          string    `json:"state"`
	SigningMode    string    `json:"signing_mode"`
	DocumentStatus string    `json:"document_status"`
	SubtotalCLP    int64     `json:"subtotal_clp"`
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
}

type AssistedViewPayment struct {
	ID                    string    `json:"id"`
	ReservationID         string    `json:"reservation_id"`
	SpaceID               string    `json:"space_id"`
	SpaceTitle            string    `json:"space_title"`
	Status                string    `json:"status"`
	CheckoutMethod        string    `json:"checkout_method"`
	TestMode              bool      `json:"test_mode"`
	TotalChargedCLP       int64     `json:"total_charged_clp"`
	ProviderPaymentStatus string    `json:"provider_payment_status"`
	ReconciliationReason  string    `json:"reconciliation_reason"`
	CreatedAt             time.Time `json:"created_at"`
}

type AssistedViewProjection struct {
	GrantID               string                     `json:"grant_id"`
	AccountID             string                     `json:"account_id"`
	ActorAccountID        string                     `json:"actor_account_id"`
	Role                  string                     `json:"role"`
	CaseID                string                     `json:"case_id"`
	ExpiresAt             time.Time                  `json:"expires_at"`
	ReadOnly              bool                       `json:"read_only"`
	AccountSummary        AssistedViewAccountSummary `json:"account_summary"`
	Diagnostics           []AssistedViewDiagnostic   `json:"diagnostics"`
	PublishGuide          *AssistedViewPublishGuide  `json:"publish_guide,omitempty"`
	MercadoPagoConnection *AssistedViewMercadoPago   `json:"mercadopago_connection,omitempty"`
	Spaces                []AssistedViewSpace        `json:"spaces"`
	Reservations          []AssistedViewReservation  `json:"reservations"`
	Contracts             []AssistedViewContract     `json:"contracts"`
	Payments              []AssistedViewPayment      `json:"payments"`
}

func (s *Service) currentTime() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// StartAssistedView validates, records, and audits a time-bounded assisted client-view grant.
func (s *Service) StartAssistedView(ctx context.Context, input AssistedViewStartInput) (AssistedViewGrant, error) {
	role := strings.TrimSpace(input.Role)
	if role != "arrendador" && role != "arrendatario" {
		return AssistedViewGrant{}, ErrAssistedViewInvalid
	}
	caseID := strings.TrimSpace(input.CaseID)
	if n := utf8.RuneCountInString(caseID); n < 3 || n > 100 {
		return AssistedViewGrant{}, ErrAssistedViewInvalid
	}
	reason := strings.TrimSpace(input.Reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 500 {
		return AssistedViewGrant{}, ErrAssistedViewInvalid
	}
	if !canonicalUUID.MatchString(input.ActorID) ||
		!canonicalUUID.MatchString(input.TargetID) ||
		!canonicalUUID.MatchString(input.AdminSessionID) ||
		!canonicalUUID.MatchString(input.CorrelationID) {
		return AssistedViewGrant{}, ErrAssistedViewInvalid
	}
	if input.ActorID == input.TargetID {
		return AssistedViewGrant{}, ErrAssistedViewInvalid
	}

	now := s.currentTime()
	expiresAt := now.Add(assistedViewTTL)
	grantID, err := s.ids.ID()
	if err != nil {
		return AssistedViewGrant{}, err
	}

	rec := AssistedViewRecord{
		ID:             grantID,
		AdminSessionID: input.AdminSessionID,
		ActorID:        input.ActorID,
		TargetID:       input.TargetID,
		Role:           role,
		CaseID:         caseID,
		Reason:         reason,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	}

	if s.assistedViewStore != nil {
		return s.assistedViewStore.StartAssistedView(ctx, rec)
	}
	if s.pool == nil {
		return AssistedViewGrant{}, ErrInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssistedViewGrant{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return AssistedViewGrant{}, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id = $1)`, input.TargetID).Scan(&exists); err != nil {
		return AssistedViewGrant{}, err
	}
	if !exists {
		return AssistedViewGrant{}, ErrNotFound
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO public.vista_asistida_administrativa (
			id, sesion_administrativa_id, actor_cuenta_id, objetivo_cuenta_id,
			rol_vista, caso_id, motivo, creada_en, expira_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, grantID, input.AdminSessionID, input.ActorID, input.TargetID, role, caseID, reason, now, expiresAt)
	if err != nil {
		return AssistedViewGrant{}, err
	}

	if err := s.auditWithTarget(ctx, tx, input.ActorID, grantID, assistedViewStartAction, "exito", reason, input.CorrelationID, now, input.TargetID); err != nil {
		return AssistedViewGrant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AssistedViewGrant{}, err
	}

	return AssistedViewGrant{
		GrantID:        grantID,
		AccountID:      input.TargetID,
		ActorAccountID: input.ActorID,
		Role:           role,
		CaseID:         caseID,
		Reason:         reason,
		ExpiresAt:      expiresAt,
	}, nil
}

// GetAssistedViewProjection returns a read-only projection for an active assisted client-view grant.
func (s *Service) GetAssistedViewProjection(ctx context.Context, input AssistedViewProjectionInput) (AssistedViewProjection, error) {
	if !canonicalUUID.MatchString(input.GrantID) ||
		!canonicalUUID.MatchString(input.ActorID) ||
		!canonicalUUID.MatchString(input.AdminSessionID) ||
		!canonicalUUID.MatchString(input.CorrelationID) {
		return AssistedViewProjection{}, ErrInvalid
	}

	if s.assistedViewStore != nil {
		return s.assistedViewStore.GetAssistedViewProjection(ctx, input)
	}
	if s.pool == nil {
		return AssistedViewProjection{}, ErrInvalid
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return AssistedViewProjection{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return AssistedViewProjection{}, err
	}

	var (
		actorID, targetID, sessionID, role, caseID string
		expiresAt, sessionExpiresAt                time.Time
		revokedAt, sessionRevokedAt                *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT v.actor_cuenta_id::text, v.objetivo_cuenta_id::text, v.sesion_administrativa_id::text,
		       v.rol_vista, v.caso_id, v.expira_en, v.revocada_en,
		       s.revocada_en, s.expira_en
		FROM public.vista_asistida_administrativa v
		JOIN public.sesion_administrativa s ON s.id = v.sesion_administrativa_id
		WHERE v.id = $1
	`, input.GrantID).Scan(
		&actorID, &targetID, &sessionID,
		&role, &caseID, &expiresAt, &revokedAt,
		&sessionRevokedAt, &sessionExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistedViewProjection{}, ErrNotFound
	}
	if err != nil {
		return AssistedViewProjection{}, err
	}

	now := s.currentTime()
	if actorID != input.ActorID || sessionID != input.AdminSessionID ||
		revokedAt != nil || !now.Before(expiresAt) ||
		sessionRevokedAt != nil || !now.Before(sessionExpiresAt) {
		return AssistedViewProjection{}, ErrForbidden
	}

	summary, err := queryAssistedViewAccountSummary(ctx, tx, targetID)
	if err != nil {
		return AssistedViewProjection{}, err
	}
	mpConn, err := queryAssistedViewMercadoPago(ctx, tx, targetID, role)
	if err != nil {
		return AssistedViewProjection{}, err
	}
	spaces, err := queryAssistedViewSpaces(ctx, tx, targetID, role, summary, mpConn)
	if err != nil {
		return AssistedViewProjection{}, err
	}
	reservations, err := queryAssistedViewReservations(ctx, tx, targetID, role)
	if err != nil {
		return AssistedViewProjection{}, err
	}
	contracts, err := queryAssistedViewContracts(ctx, tx, targetID, role)
	if err != nil {
		return AssistedViewProjection{}, err
	}
	payments, err := queryAssistedViewPayments(ctx, tx, targetID, role)
	if err != nil {
		return AssistedViewProjection{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AssistedViewProjection{}, err
	}

	publishGuide := buildAssistedViewPublishGuide(role, summary, mpConn, spaces)
	diagnostics := buildAssistedViewDiagnostics(role, summary, mpConn, spaces, contracts, payments)

	return AssistedViewProjection{
		GrantID:               input.GrantID,
		AccountID:             targetID,
		ActorAccountID:        actorID,
		Role:                  role,
		CaseID:                caseID,
		ExpiresAt:             expiresAt,
		ReadOnly:              true,
		AccountSummary:        summary,
		Diagnostics:           diagnostics,
		PublishGuide:          publishGuide,
		MercadoPagoConnection: mpConn,
		Spaces:                spaces,
		Reservations:          reservations,
		Contracts:             contracts,
		Payments:              payments,
	}, nil
}

// CloseAssistedView revokes an assisted client-view grant and records an audit event.
func (s *Service) CloseAssistedView(ctx context.Context, input AssistedViewCloseInput) error {
	if !canonicalUUID.MatchString(input.GrantID) ||
		!canonicalUUID.MatchString(input.ActorID) ||
		!canonicalUUID.MatchString(input.AdminSessionID) ||
		!canonicalUUID.MatchString(input.CorrelationID) {
		return ErrInvalid
	}

	if s.assistedViewStore != nil {
		return s.assistedViewStore.CloseAssistedView(ctx, input)
	}
	if s.pool == nil {
		return ErrInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return err
	}

	var (
		actorID, targetID, sessionID string
		revokedAt                    *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT actor_cuenta_id::text, objetivo_cuenta_id::text, sesion_administrativa_id::text, revocada_en
		FROM public.vista_asistida_administrativa
		WHERE id = $1
		FOR UPDATE
	`, input.GrantID).Scan(&actorID, &targetID, &sessionID, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if actorID != input.ActorID || sessionID != input.AdminSessionID {
		return ErrForbidden
	}

	now := s.currentTime()
	if revokedAt == nil {
		if _, err := tx.Exec(ctx, `
			UPDATE public.vista_asistida_administrativa
			SET revocada_en = $2
			WHERE id = $1
		`, input.GrantID, now); err != nil {
			return err
		}
		if err := s.auditWithTarget(ctx, tx, input.ActorID, input.GrantID, assistedViewCloseAction, "exito", "cierre_vista_asistida", input.CorrelationID, now, targetID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func queryAssistedViewAccountSummary(ctx context.Context, tx pgx.Tx, targetID string) (AssistedViewAccountSummary, error) {
	summary := AssistedViewAccountSummary{
		AccountID: targetID,
		Roles:     make([]string, 0),
	}
	var (
		kycEligible       bool
		attemptStatus     string
		decisionStatus    string
		rejectionReason   string
		verificationState string
	)
	err := tx.QueryRow(ctx, `
		SELECT u.estado,
		       u.estado = 'activo' AS email_verified,
		       EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id = u.id) AS admin_blocked,
		       EXISTS(SELECT 1 FROM public.cuenta_verificacion_telefono t WHERE t.account_id = u.id AND t.verified_at IS NOT NULL) AS phone_verified,
		       EXISTS(
		           SELECT 1
		           FROM public.elegibilidad_verificacion_local el
		           JOIN public.verificacion v ON v.id = el.verificacion_id AND v.usuario_id = el.usuario_id AND v.tipo = el.tipo AND v.estado = 'aprobada'
		           WHERE el.usuario_id = u.id AND el.tipo = 'kyc' AND el.estado = 'elegible'
		       ) AS kyc_eligible,
		       COALESCE((SELECT a.status FROM public.identity_verification_attempts a WHERE a.account_id = u.id ORDER BY a.created_at DESC LIMIT 1), '') AS attempt_status,
		       COALESCE((SELECT a.decision_status FROM public.identity_verification_attempts a WHERE a.account_id = u.id ORDER BY a.created_at DESC LIMIT 1), '') AS decision_status,
		       COALESCE((SELECT a.rejection_reason FROM public.identity_verification_attempts a WHERE a.account_id = u.id ORDER BY a.created_at DESC LIMIT 1), '') AS rejection_reason,
		       COALESCE((SELECT v.estado FROM public.verificacion v WHERE v.usuario_id = u.id AND v.tipo = 'kyc' ORDER BY v.creada_en DESC LIMIT 1), '') AS verification_state
		FROM public.usuario u
		WHERE u.id = $1
	`, targetID).Scan(
		&summary.State,
		&summary.EmailVerified,
		&summary.AdminBlocked,
		&summary.PhoneVerified,
		&kycEligible,
		&attemptStatus,
		&decisionStatus,
		&rejectionReason,
		&verificationState,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistedViewAccountSummary{}, ErrNotFound
	}
	if err != nil {
		return AssistedViewAccountSummary{}, err
	}

	summary.KYCStatus, summary.KYCRejectionReason = resolveAssistedViewKYCStatus(kycEligible, attemptStatus, decisionStatus, rejectionReason, verificationState)

	rows, err := tx.Query(ctx, `SELECT rol FROM public.rol_usuario WHERE usuario_id = $1 ORDER BY rol ASC`, targetID)
	if err != nil {
		return AssistedViewAccountSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return AssistedViewAccountSummary{}, err
		}
		summary.Roles = append(summary.Roles, r)
	}
	if err := rows.Err(); err != nil {
		return AssistedViewAccountSummary{}, err
	}
	return summary, nil
}

func resolveAssistedViewKYCStatus(kycEligible bool, attemptStatus, decisionStatus, rejectionReason, verificationState string) (string, string) {
	if kycEligible {
		return "verified", ""
	}
	switch strings.ToUpper(strings.TrimSpace(attemptStatus)) {
	case "APPROVED":
		return "verified", ""
	case "REJECTED", "FAILED":
		return "rejected", strings.TrimSpace(rejectionReason)
	case "CREATED", "PENDING_USER_INPUT", "PROCESSING", "IN_REVIEW":
		return "pending", ""
	}
	if strings.EqualFold(strings.TrimSpace(decisionStatus), "Approved") {
		return "verified", ""
	}
	switch strings.ToLower(strings.TrimSpace(verificationState)) {
	case "aprobada":
		return "verified", ""
	case "rechazada":
		return "rejected", strings.TrimSpace(rejectionReason)
	case "pendiente", "en_revision":
		return "pending", ""
	}
	return "not_started", ""
}

func queryAssistedViewMercadoPago(ctx context.Context, tx pgx.Tx, targetID, role string) (*AssistedViewMercadoPago, error) {
	if role != "arrendador" {
		return nil, nil
	}
	var (
		state       string
		liveMode    bool
		connectedAt time.Time
		updatedAt   time.Time
	)
	err := tx.QueryRow(ctx, `
		SELECT connection_state, live_mode, connected_at, updated_at
		FROM public.mercadopago_seller_connection
		WHERE anfitrion_id = $1
	`, targetID).Scan(&state, &liveMode, &connectedAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &AssistedViewMercadoPago{
			Connected:       false,
			ConnectionState: "not_connected",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &AssistedViewMercadoPago{
		Connected:       state == "active",
		ConnectionState: state,
		LiveMode:        liveMode,
		ConnectedAt:     &connectedAt,
		UpdatedAt:       &updatedAt,
	}, nil
}

func queryAssistedViewSpaces(ctx context.Context, tx pgx.Tx, targetID, role string, summary AssistedViewAccountSummary, mp *AssistedViewMercadoPago) ([]AssistedViewSpace, error) {
	spaces := make([]AssistedViewSpace, 0)
	if role != "arrendador" {
		return spaces, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT e.id::text, e.titulo, e.estado, COALESCE(e.categoria_codigo, ''),
		       e.modalidad_tarifa, e.precio_base_clp, e.capacidad_maxima,
		       e.superficie_m2::float8, COALESCE(e.zona_horaria, ''),
		       e.ubicacion_confirmada_en IS NOT NULL,
		       COALESCE(ecb.version_publicada, 0)::int,
		       e.actualizado_en
		FROM public.espacio e
		LEFT JOIN public.espacio_contrato_borrador ecb ON ecb.espacio_id = e.id
		WHERE e.propietario_id = $1
		ORDER BY e.actualizado_en DESC, e.id DESC
		LIMIT 50
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AssistedViewSpace
		if err := rows.Scan(
			&item.ID, &item.Title, &item.State, &item.CategoryCode,
			&item.Modality, &item.PriceCLP, &item.MaxCapacity,
			&item.AreaM2, &item.TimeZone, &item.LocationConfirmed,
			&item.ContractPublishedVersion, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.PublishBlockers = computeSpacePublishBlockers(item, summary, mp)
		spaces = append(spaces, item)
	}
	return spaces, rows.Err()
}

func queryAssistedViewReservations(ctx context.Context, tx pgx.Tx, targetID, role string) ([]AssistedViewReservation, error) {
	reservations := make([]AssistedViewReservation, 0)
	filterCol := "r.arrendatario_id"
	if role == "arrendador" {
		filterCol = "r.anfitrion_id"
	}
	query := `
		SELECT r.id::text, r.espacio_id::text, e.titulo, r.estado, r.modalidad,
		       r.subtotal_clp, btrim(r.moneda::text), r.inicio, r.termino,
		       r.pago_vence_en, r.creada_en
		FROM public.reserva_arriendo_comercial r
		JOIN public.espacio e ON e.id = r.espacio_id
		WHERE ` + filterCol + ` = $1
		ORDER BY r.creada_en DESC, r.id DESC
		LIMIT 50
	`
	rows, err := tx.Query(ctx, query, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AssistedViewReservation
		if err := rows.Scan(
			&item.ID, &item.SpaceID, &item.SpaceTitle, &item.State, &item.Modality,
			&item.SubtotalCLP, &item.Currency, &item.StartAt, &item.EndAt,
			&item.PaymentExpiresAt, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		reservations = append(reservations, item)
	}
	return reservations, rows.Err()
}

func queryAssistedViewContracts(ctx context.Context, tx pgx.Tx, targetID, role string) ([]AssistedViewContract, error) {
	contracts := make([]AssistedViewContract, 0)
	filterCol := "cs.arrendatario_id"
	if role == "arrendador" {
		filterCol = "cs.propietario_id"
	}
	query := `
		SELECT cs.id::text, cs.propuesta_id::text, cs.espacio_id::text, e.titulo,
		       cs.estado, cs.modalidad_firma, COALESCE(cd.estado, ''),
		       cp.subtotal_clp, cp.moneda, cs.creada_en
		FROM public.contrato_solicitud_comercial cs
		JOIN public.contrato_propuesta_comercial cp ON cp.id = cs.propuesta_id
		JOIN public.espacio e ON e.id = cs.espacio_id
		LEFT JOIN public.contrato_documento_comercial cd ON cd.solicitud_id = cs.id
		WHERE ` + filterCol + ` = $1
		ORDER BY cs.creada_en DESC, cs.id DESC
		LIMIT 50
	`
	rows, err := tx.Query(ctx, query, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AssistedViewContract
		if err := rows.Scan(
			&item.ID, &item.ProposalID, &item.SpaceID, &item.SpaceTitle,
			&item.State, &item.SigningMode, &item.DocumentStatus,
			&item.SubtotalCLP, &item.Currency, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		contracts = append(contracts, item)
	}
	return contracts, rows.Err()
}

func queryAssistedViewPayments(ctx context.Context, tx pgx.Tx, targetID, role string) ([]AssistedViewPayment, error) {
	payments := make([]AssistedViewPayment, 0)
	filterCol := "cp.arrendatario_id"
	if role == "arrendador" {
		filterCol = "cp.host_id"
	}
	query := `
		SELECT cp.id::text, COALESCE(r.id::text, ''), cp.espacio_id::text, e.titulo,
		       cp.estado, cp.checkout_method, cp.test_mode, cp.total_charged_clp,
		       COALESCE(cp.provider_payment_status, ''), COALESCE(cp.reconciliation_reason, ''),
		       cp.creada_en
		FROM public.checkout_preferencia_comercial cp
		JOIN public.espacio e ON e.id = cp.espacio_id
		LEFT JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id = cp.id
		WHERE ` + filterCol + ` = $1
		ORDER BY cp.creada_en DESC, cp.id DESC
		LIMIT 50
	`
	rows, err := tx.Query(ctx, query, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AssistedViewPayment
		if err := rows.Scan(
			&item.ID, &item.ReservationID, &item.SpaceID, &item.SpaceTitle,
			&item.Status, &item.CheckoutMethod, &item.TestMode, &item.TotalChargedCLP,
			&item.ProviderPaymentStatus, &item.ReconciliationReason, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	return payments, rows.Err()
}

func computeSpacePublishBlockers(space AssistedViewSpace, summary AssistedViewAccountSummary, mp *AssistedViewMercadoPago) []string {
	blockers := make([]string, 0)
	if summary.AdminBlocked {
		blockers = append(blockers, "account_admin_blocked")
	}
	if !summary.EmailVerified {
		blockers = append(blockers, "email_not_verified")
	}
	if summary.KYCStatus != "verified" {
		blockers = append(blockers, "kyc_not_verified")
	}
	if !space.LocationConfirmed {
		blockers = append(blockers, "location_not_confirmed")
	}
	if strings.TrimSpace(space.TimeZone) == "" {
		blockers = append(blockers, "time_zone_missing")
	}
	if space.PriceCLP <= 0 {
		blockers = append(blockers, "price_not_configured")
	}
	if space.ContractPublishedVersion <= 0 {
		blockers = append(blockers, "contract_template_not_published")
	}
	if mp == nil || !mp.Connected {
		if mp != nil && mp.ConnectionState == "reauthorization_required" {
			blockers = append(blockers, "mercadopago_reauthorization_required")
		} else {
			blockers = append(blockers, "mercadopago_not_connected")
		}
	}
	return blockers
}

func buildAssistedViewPublishGuide(role string, summary AssistedViewAccountSummary, mp *AssistedViewMercadoPago, spaces []AssistedViewSpace) *AssistedViewPublishGuide {
	if role != "arrendador" {
		return nil
	}

	kycStepStatus := "action_required"
	kycDetail := "Completa la verificación de identidad (KYC) para habilitar publicaciones."
	switch summary.KYCStatus {
	case "verified":
		kycStepStatus = "completed"
		kycDetail = "Identidad verificada."
	case "pending":
		kycStepStatus = "pending"
		kycDetail = "Verificación de identidad en revisión."
	case "rejected":
		kycStepStatus = "blocked"
		kycDetail = "Verificación de identidad rechazada."
		if summary.KYCRejectionReason != "" {
			kycDetail = fmt.Sprintf("Verificación rechazada: %s", summary.KYCRejectionReason)
		}
	}

	draftStatus := "not_started"
	draftStepStatus := "action_required"
	draftDetail := "Crea al menos un espacio con tarifa, zona horaria y ubicación confirmada."
	if len(spaces) > 0 {
		draftStatus = "incomplete"
		draftDetail = "Completa ubicación confirmada, zona horaria y tarifa en tu borrador."
		for _, sp := range spaces {
			if sp.State == "activa" || sp.State == "publicado" {
				draftStatus = "published"
				draftStepStatus = "completed"
				draftDetail = "Espacio publicado y visible."
				break
			}
			if sp.LocationConfirmed && strings.TrimSpace(sp.TimeZone) != "" && sp.PriceCLP > 0 {
				draftStatus = "ready"
				draftStepStatus = "completed"
				draftDetail = "Borrador completo listo para publicar."
			}
		}
	}

	mpStatus := "not_connected"
	mpStepStatus := "action_required"
	mpDetail := "Conecta tu cuenta de Mercado Pago para recibir pagos."
	if mp != nil && mp.Connected {
		mpStatus = "connected"
		mpStepStatus = "completed"
		mpDetail = "Cuenta de Mercado Pago conectada."
	} else if mp != nil && mp.ConnectionState == "reauthorization_required" {
		mpStatus = "reauthorization_required"
		mpStepStatus = "blocked"
		mpDetail = "La conexión con Mercado Pago requiere reautorización."
	}

	contractStatus := "not_started"
	contractStepStatus := "pending"
	contractDetail := "Publica una plantilla de contrato comercial en el espacio."
	if len(spaces) > 0 {
		contractStatus = "not_published"
		contractStepStatus = "action_required"
		for _, sp := range spaces {
			if sp.ContractPublishedVersion > 0 {
				contractStatus = "published"
				contractStepStatus = "completed"
				contractDetail = "Plantilla de contrato publicada."
				break
			}
		}
	}

	return &AssistedViewPublishGuide{
		KYCStatus:         summary.KYCStatus,
		DraftStatus:       draftStatus,
		MercadoPagoStatus: mpStatus,
		ContractStatus:    contractStatus,
		Steps: []AssistedViewPublishStep{
			{Step: 1, Key: "kyc", Title: "KYC", Status: kycStepStatus, Detail: kycDetail},
			{Step: 2, Key: "draft", Title: "Borrador del espacio", Status: draftStepStatus, Detail: draftDetail},
			{Step: 3, Key: "mercadopago", Title: "Sincronizar Mercado Pago", Status: mpStepStatus, Detail: mpDetail},
			{Step: 4, Key: "contract", Title: "Contrato listo", Status: contractStepStatus, Detail: contractDetail},
		},
	}
}

func buildAssistedViewDiagnostics(
	role string,
	summary AssistedViewAccountSummary,
	mp *AssistedViewMercadoPago,
	spaces []AssistedViewSpace,
	contracts []AssistedViewContract,
	payments []AssistedViewPayment,
) []AssistedViewDiagnostic {
	diags := make([]AssistedViewDiagnostic, 0)

	if summary.AdminBlocked {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "account_admin_blocked",
			Severity: "error",
			Area:     "account",
			Title:    "Cuenta bloqueada administrativamente",
			Detail:   "La cuenta tiene un bloqueo administrativo local activo.",
		})
	}
	if !summary.EmailVerified {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "email_not_verified",
			Severity: "warning",
			Area:     "account",
			Title:    "Correo no verificado",
			Detail:   "La cuenta aún no está activa con correo verificado.",
		})
	}
	if !summary.PhoneVerified {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "phone_not_verified",
			Severity: "info",
			Area:     "account",
			Title:    "Teléfono no verificado",
			Detail:   "La verificación telefónica por OTP no ha sido completada.",
		})
	}

	switch summary.KYCStatus {
	case "rejected":
		detail := "El intento de verificación de identidad fue rechazado."
		if summary.KYCRejectionReason != "" {
			detail = fmt.Sprintf("Verificación rechazada: %s", summary.KYCRejectionReason)
		}
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "kyc_rejected",
			Severity: "error",
			Area:     "kyc",
			Title:    "Verificación de identidad rechazada",
			Detail:   detail,
		})
	case "pending":
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "kyc_pending",
			Severity: "info",
			Area:     "kyc",
			Title:    "Verificación de identidad en curso",
			Detail:   "Existe un intento de KYC en proceso o en revisión.",
		})
	case "not_started":
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "kyc_not_started",
			Severity: "warning",
			Area:     "kyc",
			Title:    "Verificación de identidad pendiente",
			Detail:   "La cuenta no registra verificación KYC completada.",
		})
	}

	if role == "arrendador" {
		if mp != nil && mp.ConnectionState == "reauthorization_required" {
			diags = append(diags, AssistedViewDiagnostic{
				Code:     "mercadopago_reauthorization_required",
				Severity: "error",
				Area:     "mercadopago",
				Title:    "Reautorización de Mercado Pago requerida",
				Detail:   "La cuenta de vendedor debe reautorizar OAuth con Mercado Pago.",
			})
		} else if mp == nil || !mp.Connected {
			diags = append(diags, AssistedViewDiagnostic{
				Code:     "mercadopago_not_connected",
				Severity: "warning",
				Area:     "mercadopago",
				Title:    "Mercado Pago no conectado",
				Detail:   "El arrendador no tiene una conexión activa de Mercado Pago.",
			})
		}

		for _, sp := range spaces {
			if len(sp.PublishBlockers) > 0 {
				diags = append(diags, AssistedViewDiagnostic{
					Code:     "space_publish_blocked",
					Severity: "warning",
					Area:     "spaces",
					Title:    fmt.Sprintf("Espacio con bloqueos de publicación: %s", sp.Title),
					Detail:   strings.Join(sp.PublishBlockers, ", "),
				})
			}
		}
	}

	pendingLandlord := 0
	docFailed := 0
	for _, c := range contracts {
		if c.State == "pending_landlord" {
			pendingLandlord++
		}
		if c.DocumentStatus == "failed" {
			docFailed++
		}
	}
	if pendingLandlord > 0 {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "contracts_pending_landlord",
			Severity: "info",
			Area:     "contracts",
			Title:    "Contratos pendientes de firma del arrendador",
			Detail:   fmt.Sprintf("%d solicitud(es) esperando firma del arrendador.", pendingLandlord),
		})
	}
	if docFailed > 0 {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "contract_document_failed",
			Severity: "error",
			Area:     "contracts",
			Title:    "Error en documento de contrato",
			Detail:   fmt.Sprintf("%d contrato(s) presentan fallo en la generación de documento.", docFailed),
		})
	}

	reconciliationCount := 0
	rejectedPayments := 0
	for _, p := range payments {
		if p.Status == "reconciliation_required" {
			reconciliationCount++
		}
		if p.Status == "rejected" {
			rejectedPayments++
		}
	}
	if reconciliationCount > 0 {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "payments_reconciliation_required",
			Severity: "error",
			Area:     "payments",
			Title:    "Pagos requieren conciliación",
			Detail:   fmt.Sprintf("%d pago(s) en estado reconciliation_required.", reconciliationCount),
		})
	}
	if rejectedPayments > 0 {
		diags = append(diags, AssistedViewDiagnostic{
			Code:     "payments_rejected",
			Severity: "warning",
			Area:     "payments",
			Title:    "Pagos rechazados",
			Detail:   fmt.Sprintf("%d pago(s) rechazado(s) en el historial reciente.", rejectedPayments),
		})
	}

	return diags
}
