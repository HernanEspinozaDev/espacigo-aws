package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func (h *Handler) serveSpecializedOps(w http.ResponseWriter, r *http.Request, path, requestID string) bool {
	switch path {
	case "/api/v1/admin/global/payments":
		return h.handleListPayments(w, r, requestID)
	case "/api/v1/admin/global/reservations":
		return h.handleListReservations(w, r, requestID)
	case "/api/v1/admin/global/contracts":
		return h.handleListContracts(w, r, requestID)
	case "/api/v1/admin/global/kyc-cases":
		return h.handleListKYCCases(w, r, requestID)
	case "/api/v1/admin/global/privacy-requests":
		return h.handlePrivacyRequests(w, r, requestID)
	case "/api/v1/admin/global/privacy-incidents":
		return h.handlePrivacyIncidents(w, r, requestID)
	case "/api/v1/admin/global/privacy-legal-holds":
		return h.handlePrivacyLegalHolds(w, r, requestID)
	case "/api/v1/admin/global/support-tickets":
		return h.handleSupportTickets(w, r, requestID)
	case "/api/v1/admin/global/approvals":
		return h.handleApprovals(w, r, requestID)
	}
	const incidentsPrefix = "/api/v1/admin/global/privacy-incidents/"
	if strings.HasPrefix(path, incidentsPrefix) {
		rest := strings.TrimPrefix(path, incidentsPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			return h.handlePrivacyIncidentDetail(w, r, parts[0], requestID)
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "transitions" {
			return h.handlePrivacyIncidentTransition(w, r, parts[0], requestID)
		}
	}
	const holdsPrefix = "/api/v1/admin/global/privacy-legal-holds/"
	if strings.HasPrefix(path, holdsPrefix) {
		rest := strings.TrimPrefix(path, holdsPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "release" {
			return h.handlePrivacyLegalHoldRelease(w, r, parts[0], requestID)
		}
	}
	const privacyPrefix = "/api/v1/admin/global/privacy-requests/"
	if strings.HasPrefix(path, privacyPrefix) {
		rest := strings.TrimPrefix(path, privacyPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			return h.handlePrivacyDetail(w, r, parts[0], requestID)
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "resolution" {
			return h.handlePrivacyResolution(w, r, parts[0], requestID)
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "dispatch-receipt" {
			return h.handlePrivacyDispatchReceipt(w, r, parts[0], requestID)
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "workflow" {
			return h.handlePrivacyWorkflow(w, r, parts[0], requestID)
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "executions" {
			return h.handlePrivacyExecution(w, r, parts[0], requestID)
		}
		if len(parts) == 3 && parts[0] != "" && parts[1] == "notifications" && parts[2] == "retry" {
			return h.handlePrivacyNotificationsRetry(w, r, parts[0], requestID)
		}
	}
	const approvalsPrefix = "/api/v1/admin/global/approvals/"
	if strings.HasPrefix(path, approvalsPrefix) {
		rest := strings.TrimPrefix(path, approvalsPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "decisions" {
			return h.handleApprovalDecision(w, r, parts[0], requestID)
		}
	}
	return false
}

func (h *Handler) handlePrivacyDetail(w http.ResponseWriter, r *http.Request, targetID, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityPrivacyRequestsRead, identity.CapabilityPrivacyRequestsManage)
	if !ok {
		return true
	}
	value, err := h.service.GetPrivacyRequest(r.Context(), principal.AccountID, requestID, targetID)
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyWorkflow(w http.ResponseWriter, r *http.Request, targetID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		Action          string `json:"action"`
		AssignedToID    string `json:"assigned_to_id"`
		AccountID       string `json:"account_id"`
		Note            string `json:"note"`
		ExtraDays       int    `json:"extra_days"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.TransitionPrivacyRequest(r.Context(), adminglobal.TransitionPrivacyRequestInput{
		ActorID:         principal.AccountID,
		CorrelationID:   requestID,
		RequestID:       targetID,
		Action:          body.Action,
		AssignedToID:    body.AssignedToID,
		AccountID:       body.AccountID,
		Note:            body.Note,
		ExtraDays:       body.ExtraDays,
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyExecution(w http.ResponseWriter, r *http.Request, targetID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		Action          string     `json:"action"`
		ExpectedVersion int        `json:"expected_version"`
		Summary         string     `json:"summary"`
		DisplayName     *string    `json:"display_name"`
		Phone           *string    `json:"phone"`
		Purpose         string     `json:"purpose"`
		PurposeState    string     `json:"purpose_state"`
		Scope           string     `json:"scope"`
		BlockID         string     `json:"block_id"`
		ReviewAt        *time.Time `json:"review_at"`
		TargetReference string     `json:"target_reference"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ExecutePrivacyRight(r.Context(), adminglobal.ExecutePrivacyRightInput{
		ActorID:         principal.AccountID,
		CorrelationID:   requestID,
		RequestID:       targetID,
		Action:          body.Action,
		ExpectedVersion: body.ExpectedVersion,
		Summary:         body.Summary,
		DisplayName:     body.DisplayName,
		Phone:           body.Phone,
		Purpose:         body.Purpose,
		PurposeState:    body.PurposeState,
		Scope:           body.Scope,
		BlockID:         body.BlockID,
		ReviewAt:        body.ReviewAt,
		TargetReference: body.TargetReference,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyNotificationsRetry(w http.ResponseWriter, r *http.Request, targetID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	value, err := h.service.RetryPrivacyNotifications(r.Context(), principal.AccountID, requestID, targetID)
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyDispatchReceipt(w http.ResponseWriter, r *http.Request, targetID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		Channel     string    `json:"channel"`
		Destination string    `json:"destination"`
		SentAt      time.Time `json:"sent_at"`
		EvidenceRef string    `json:"evidence_ref"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.RecordPrivacyDispatch(r.Context(), adminglobal.RecordPrivacyDispatchInput{ActorID: principal.AccountID, CorrelationID: requestID, RequestID: targetID, Channel: body.Channel, Destination: body.Destination, SentAt: body.SentAt, EvidenceRef: body.EvidenceRef})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) authorizeAny(w http.ResponseWriter, r *http.Request, requestID string, capabilities ...identity.Capability) (identity.Principal, bool) {
	secret := identity.Secret(bearer(r.Header.Get("Authorization")))
	for _, cap := range capabilities {
		principal, err := h.auth.AuthorizeCapability(r.Context(), secret, cap, identity.UserOperation)
		if err == nil {
			return principal, true
		}
		if errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrRestrictedMode) {
			continue
		}
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthenticated", requestID)
		return identity.Principal{}, false
	}
	writeError(w, http.StatusForbidden, "forbidden", requestID)
	return identity.Principal{}, false
}

func (h *Handler) handleListPayments(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPaymentsRead, requestID)
	if !ok {
		return true
	}
	filter, valid := parsePaymentFilter(r.URL.Query())
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ListPayments(r.Context(), principal.AccountID, requestID, filter)
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handleListReservations(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityReservationsRead, requestID)
	if !ok {
		return true
	}
	status, accountID, pageSize, cursor, valid := parseStatusAccountPagination(r.URL.Query())
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ListReservations(r.Context(), principal.AccountID, requestID, adminglobal.ReservationFilter{
		Status: status, AccountID: accountID, PageSize: pageSize, Cursor: cursor,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handleListContracts(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityContractsRead, requestID)
	if !ok {
		return true
	}
	status, accountID, pageSize, cursor, valid := parseStatusAccountPagination(r.URL.Query())
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ListContracts(r.Context(), principal.AccountID, requestID, adminglobal.ContractFilter{
		Status: status, AccountID: accountID, PageSize: pageSize, Cursor: cursor,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handleListKYCCases(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityKYCCaseRead, requestID)
	if !ok {
		return true
	}
	status, accountID, pageSize, cursor, valid := parseStatusAccountPagination(r.URL.Query())
	if !valid {
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ListKYCCases(r.Context(), principal.AccountID, requestID, adminglobal.KYCCaseFilter{
		Status: status, AccountID: accountID, PageSize: pageSize, Cursor: cursor,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyRequests(w http.ResponseWriter, r *http.Request, requestID string) bool {
	switch r.Method {
	case http.MethodGet:
		principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityPrivacyRequestsRead, identity.CapabilityPrivacyRequestsManage)
		if !ok {
			return true
		}
		filter, valid := parsePrivacyRequestFilter(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListPrivacyRequests(r.Context(), principal.AccountID, requestID, filter)
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": value})
		return true
	case http.MethodPost:
		principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
		if !ok {
			return true
		}
		var body struct {
			AccountID string `json:"account_id"`
			RightType string `json:"right_type"`
			Reason    string `json:"reason"`
			CaseRef   string `json:"case_ref"`
		}
		if !decodeManagementBody(w, r, &body) {
			writeError(w, http.StatusBadRequest, "invalid_request", requestID)
			return true
		}
		value, err := h.service.CreatePrivacyRequest(r.Context(), adminglobal.CreatePrivacyRequestInput{
			ActorID:       principal.AccountID,
			CorrelationID: requestID,
			AccountID:     body.AccountID,
			RightType:     body.RightType,
			Reason:        body.Reason,
			CaseRef:       body.CaseRef,
		})
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		return true
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
}

func (h *Handler) handlePrivacyResolution(w http.ResponseWriter, r *http.Request, requestID, correlationID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", correlationID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, correlationID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", correlationID)
		return true
	}
	var body struct {
		Decision string `json:"decision"`
		Response string `json:"response"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", correlationID)
		return true
	}
	value, err := h.service.ResolvePrivacyRequest(r.Context(), adminglobal.ResolvePrivacyRequestInput{ActorID: principal.AccountID, CorrelationID: correlationID, RequestID: requestID, Decision: body.Decision, Response: body.Response})
	if err != nil {
		replyError(w, err, correlationID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handleSupportTickets(w http.ResponseWriter, r *http.Request, requestID string) bool {
	switch r.Method {
	case http.MethodGet:
		principal, ok := h.authorize(w, r, identity.CapabilitySupportTicketsManage, requestID)
		if !ok {
			return true
		}
		filter, valid := parseTicketFilter(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListSupportTickets(r.Context(), principal.AccountID, requestID, filter)
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": value})
		return true
	case http.MethodPost:
		principal, ok := h.authorize(w, r, identity.CapabilitySupportTicketsManage, requestID)
		if !ok {
			return true
		}
		var body struct {
			AccountID    string `json:"account_id"`
			AssignedToID string `json:"assigned_to_id"`
			Category     string `json:"category"`
			Priority     string `json:"priority"`
			Subject      string `json:"subject"`
			CaseRef      string `json:"case_ref"`
		}
		if !decodeManagementBody(w, r, &body) {
			writeError(w, http.StatusBadRequest, "invalid_request", requestID)
			return true
		}
		value, err := h.service.CreateSupportTicket(r.Context(), adminglobal.CreateSupportTicketInput{
			ActorID:       principal.AccountID,
			CorrelationID: requestID,
			AccountID:     body.AccountID,
			AssignedToID:  body.AssignedToID,
			Category:      body.Category,
			Priority:      body.Priority,
			Subject:       body.Subject,
			CaseRef:       body.CaseRef,
		})
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		return true
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
}

func (h *Handler) handleApprovals(w http.ResponseWriter, r *http.Request, requestID string) bool {
	switch r.Method {
	case http.MethodGet:
		principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityApprovalsRequest, identity.CapabilityApprovalsDecide)
		if !ok {
			return true
		}
		filter, valid := parseApprovalFilter(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListApprovals(r.Context(), principal.AccountID, requestID, filter)
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": value})
		return true
	case http.MethodPost:
		principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityApprovalsRequest, identity.CapabilityPaymentsRefundRequest)
		if !ok {
			return true
		}
		if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
			writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
			return true
		}
		var body struct {
			ActionType     string          `json:"action_type"`
			ResourceType   string          `json:"resource_type"`
			ResourceID     string          `json:"resource_id"`
			Reason         string          `json:"reason"`
			IdempotencyKey string          `json:"idempotency_key"`
			Payload        json.RawMessage `json:"payload"`
		}
		if !decodeManagementBody(w, r, &body) {
			writeError(w, http.StatusBadRequest, "invalid_request", requestID)
			return true
		}
		idemKey := strings.TrimSpace(body.IdempotencyKey)
		if idemKey == "" {
			idemKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		value, err := h.service.CreateApproval(r.Context(), adminglobal.CreateApprovalInput{
			ActorID:        principal.AccountID,
			CorrelationID:  requestID,
			ActionType:     body.ActionType,
			ResourceType:   body.ResourceType,
			ResourceID:     body.ResourceID,
			Reason:         body.Reason,
			IdempotencyKey: idemKey,
			Payload:        body.Payload,
		})
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		return true
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
}

func (h *Handler) handleApprovalDecision(w http.ResponseWriter, r *http.Request, approvalID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityApprovalsDecide, identity.CapabilityPaymentsRefundApprove)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		Decision        string `json:"decision"`
		DecisionReason  string `json:"decision_reason"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.DecideApproval(r.Context(), adminglobal.DecideApprovalInput{
		ApprovalID:      approvalID,
		ActorID:         principal.AccountID,
		CorrelationID:   requestID,
		Decision:        body.Decision,
		DecisionReason:  body.DecisionReason,
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func parsePageSize(raw string) (int, bool) {
	if raw == "" {
		return 25, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 || v > 100 {
		return 0, false
	}
	return v, true
}

func validateAllowedQueryKeys(q url.Values, allowed map[string]bool) bool {
	for k, vals := range q {
		if !allowed[k] || len(vals) != 1 {
			return false
		}
	}
	return true
}

func parsePaymentFilter(q url.Values) (adminglobal.PaymentFilter, bool) {
	allowed := map[string]bool{"status": true, "test_mode": true, "account_id": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.PaymentFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.PaymentFilter{}, false
	}
	var testMode *bool
	if raw := q.Get("test_mode"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return adminglobal.PaymentFilter{}, false
		}
		testMode = &parsed
	}
	return adminglobal.PaymentFilter{
		Status:    q.Get("status"),
		TestMode:  testMode,
		AccountID: q.Get("account_id"),
		PageSize:  pageSize,
		Cursor:    q.Get("cursor"),
	}, true
}

func parseStatusAccountPagination(q url.Values) (string, string, int, string, bool) {
	allowed := map[string]bool{"status": true, "account_id": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return "", "", 0, "", false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return "", "", 0, "", false
	}
	return q.Get("status"), q.Get("account_id"), pageSize, q.Get("cursor"), true
}

func parsePrivacyRequestFilter(q url.Values) (adminglobal.PrivacyRequestFilter, bool) {
	allowed := map[string]bool{
		"status":         true,
		"right_type":     true,
		"account_id":     true,
		"assigned_to_id": true,
		"channel":        true,
		"overdue":        true,
		"page_size":      true,
		"cursor":         true,
	}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.PrivacyRequestFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.PrivacyRequestFilter{}, false
	}
	rightType := q.Get("right_type")
	switch rightType {
	case "", "acceso", "rectificacion", "supresion", "oposicion", "portabilidad", "bloqueo":
	default:
		return adminglobal.PrivacyRequestFilter{}, false
	}
	channel := q.Get("channel")
	switch channel {
	case "", "web", "api", "publico", "correo":
	default:
		return adminglobal.PrivacyRequestFilter{}, false
	}
	var overdue *bool
	if raw := q.Get("overdue"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return adminglobal.PrivacyRequestFilter{}, false
		}
		overdue = &parsed
	}
	return adminglobal.PrivacyRequestFilter{
		Status:       q.Get("status"),
		RightType:    rightType,
		AccountID:    q.Get("account_id"),
		AssignedToID: q.Get("assigned_to_id"),
		Channel:      channel,
		Overdue:      overdue,
		PageSize:     pageSize,
		Cursor:       q.Get("cursor"),
	}, true
}

func parseTicketFilter(q url.Values) (adminglobal.SupportTicketFilter, bool) {
	allowed := map[string]bool{"status": true, "priority": true, "account_id": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.SupportTicketFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.SupportTicketFilter{}, false
	}
	return adminglobal.SupportTicketFilter{
		Status:    q.Get("status"),
		Priority:  q.Get("priority"),
		AccountID: q.Get("account_id"),
		PageSize:  pageSize,
		Cursor:    q.Get("cursor"),
	}, true
}

func parseApprovalFilter(q url.Values) (adminglobal.ApprovalFilter, bool) {
	allowed := map[string]bool{"status": true, "action_type": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.ApprovalFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.ApprovalFilter{}, false
	}
	return adminglobal.ApprovalFilter{
		Status:     q.Get("status"),
		ActionType: q.Get("action_type"),
		PageSize:   pageSize,
		Cursor:     q.Get("cursor"),
	}, true
}

func (h *Handler) handlePrivacyIncidents(w http.ResponseWriter, r *http.Request, requestID string) bool {
	switch r.Method {
	case http.MethodGet:
		principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityPrivacyIncidentsRead, identity.CapabilityPrivacyIncidentsManage)
		if !ok {
			return true
		}
		filter, valid := parsePrivacyIncidentFilter(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListPrivacyIncidents(r.Context(), principal.AccountID, requestID, filter)
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": value})
		return true
	case http.MethodPost:
		principal, ok := h.authorize(w, r, identity.CapabilityPrivacyIncidentsManage, requestID)
		if !ok {
			return true
		}
		if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
			writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
			return true
		}
		var body struct {
			Title                 string    `json:"title"`
			Severity              string    `json:"severity"`
			DataCategories        []string  `json:"data_categories"`
			AffectedSystems       []string  `json:"affected_systems"`
			EstimatedSubjects     int       `json:"estimated_subjects"`
			Summary               string    `json:"summary"`
			ContainmentMeasures   string    `json:"containment_measures"`
			RequiresAgencyNotice  bool      `json:"requires_agency_notice"`
			RequiresSubjectNotice bool      `json:"requires_subject_notice"`
			LeaderID              string    `json:"leader_id"`
			DeputyID              string    `json:"deputy_id"`
			DetectedAt            time.Time `json:"detected_at"`
		}
		if !decodeManagementBody(w, r, &body) {
			writeError(w, http.StatusBadRequest, "invalid_request", requestID)
			return true
		}
		value, err := h.service.CreatePrivacyIncident(r.Context(), adminglobal.CreatePrivacyIncidentInput{
			ActorID:               principal.AccountID,
			CorrelationID:         requestID,
			Title:                 body.Title,
			Severity:              body.Severity,
			DataCategories:        body.DataCategories,
			AffectedSystems:       body.AffectedSystems,
			EstimatedSubjects:     body.EstimatedSubjects,
			Summary:               body.Summary,
			ContainmentMeasures:   body.ContainmentMeasures,
			RequiresAgencyNotice:  body.RequiresAgencyNotice,
			RequiresSubjectNotice: body.RequiresSubjectNotice,
			LeaderID:              body.LeaderID,
			DeputyID:              body.DeputyID,
			DetectedAt:            body.DetectedAt,
		})
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		return true
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
}

func (h *Handler) handlePrivacyIncidentDetail(w http.ResponseWriter, r *http.Request, incidentID, requestID string) bool {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorizeAny(w, r, requestID, identity.CapabilityPrivacyIncidentsRead, identity.CapabilityPrivacyIncidentsManage)
	if !ok {
		return true
	}
	value, err := h.service.GetPrivacyIncident(r.Context(), principal.AccountID, requestID, incidentID)
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyIncidentTransition(w http.ResponseWriter, r *http.Request, incidentID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyIncidentsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		Action                string `json:"action"`
		ExpectedVersion       int    `json:"expected_version"`
		Detail                string `json:"detail"`
		ContainmentMeasures   string `json:"containment_measures"`
		RequiresAgencyNotice  *bool  `json:"requires_agency_notice"`
		RequiresSubjectNotice *bool  `json:"requires_subject_notice"`
		AgencyReference       string `json:"agency_reference"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.TransitionPrivacyIncident(r.Context(), adminglobal.TransitionPrivacyIncidentInput{
		ActorID:               principal.AccountID,
		CorrelationID:         requestID,
		IncidentID:            incidentID,
		Action:                body.Action,
		ExpectedVersion:       body.ExpectedVersion,
		Detail:                body.Detail,
		ContainmentMeasures:   body.ContainmentMeasures,
		RequiresAgencyNotice:  body.RequiresAgencyNotice,
		RequiresSubjectNotice: body.RequiresSubjectNotice,
		AgencyReference:       body.AgencyReference,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func (h *Handler) handlePrivacyLegalHolds(w http.ResponseWriter, r *http.Request, requestID string) bool {
	switch r.Method {
	case http.MethodGet:
		principal, ok := h.authorizeAny(w, r, requestID,
			identity.CapabilityPrivacyIncidentsRead,
			identity.CapabilityPrivacyIncidentsManage,
			identity.CapabilityPrivacyRequestsRead,
			identity.CapabilityPrivacyRequestsManage,
		)
		if !ok {
			return true
		}
		filter, valid := parsePrivacyLegalHoldFilter(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListPrivacyLegalHolds(r.Context(), principal.AccountID, requestID, filter)
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": value})
		return true
	case http.MethodPost:
		principal, ok := h.authorize(w, r, identity.CapabilityPrivacyIncidentsManage, requestID)
		if !ok {
			return true
		}
		if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
			writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
			return true
		}
		var body struct {
			AccountID  string     `json:"account_id"`
			RequestID  string     `json:"request_id"`
			IncidentID string     `json:"incident_id"`
			Scope      string     `json:"scope"`
			Authority  string     `json:"authority"`
			LegalBasis string     `json:"legal_basis"`
			ReviewAt   *time.Time `json:"review_at"`
		}
		if !decodeManagementBody(w, r, &body) {
			writeError(w, http.StatusBadRequest, "invalid_request", requestID)
			return true
		}
		value, err := h.service.CreatePrivacyLegalHold(r.Context(), adminglobal.CreatePrivacyLegalHoldInput{
			ActorID:       principal.AccountID,
			CorrelationID: requestID,
			AccountID:     body.AccountID,
			RequestID:     body.RequestID,
			IncidentID:    body.IncidentID,
			Scope:         body.Scope,
			Authority:     body.Authority,
			LegalBasis:    body.LegalBasis,
			ReviewAt:      body.ReviewAt,
		})
		if err != nil {
			replyError(w, err, requestID)
			return true
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		return true
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
}

func (h *Handler) handlePrivacyLegalHoldRelease(w http.ResponseWriter, r *http.Request, holdID, requestID string) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return true
	}
	principal, ok := h.authorize(w, r, identity.CapabilityPrivacyIncidentsManage, requestID)
	if !ok {
		return true
	}
	if !recentlyAuthenticated(principal.AuthenticatedAt, time.Now().UTC()) {
		writeError(w, http.StatusPreconditionRequired, "recent_authentication_required", requestID)
		return true
	}
	var body struct {
		ReleaseReason string `json:"release_reason"`
	}
	if !decodeManagementBody(w, r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return true
	}
	value, err := h.service.ReleasePrivacyLegalHold(r.Context(), adminglobal.ReleasePrivacyLegalHoldInput{
		ActorID:       principal.AccountID,
		CorrelationID: requestID,
		HoldID:        holdID,
		ReleaseReason: body.ReleaseReason,
	})
	if err != nil {
		replyError(w, err, requestID)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": value})
	return true
}

func parsePrivacyIncidentFilter(q url.Values) (adminglobal.PrivacyIncidentFilter, bool) {
	allowed := map[string]bool{"status": true, "severity": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.PrivacyIncidentFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.PrivacyIncidentFilter{}, false
	}
	status := q.Get("status")
	switch status {
	case "", "abierto", "contenido", "evaluado", "notificado", "cerrado":
	default:
		return adminglobal.PrivacyIncidentFilter{}, false
	}
	severity := q.Get("severity")
	switch severity {
	case "", "baja", "media", "alta", "critica":
	default:
		return adminglobal.PrivacyIncidentFilter{}, false
	}
	return adminglobal.PrivacyIncidentFilter{
		Status:   status,
		Severity: severity,
		PageSize: pageSize,
		Cursor:   q.Get("cursor"),
	}, true
}

func parsePrivacyLegalHoldFilter(q url.Values) (adminglobal.PrivacyLegalHoldFilter, bool) {
	allowed := map[string]bool{"status": true, "account_id": true, "scope": true, "page_size": true, "cursor": true}
	if !validateAllowedQueryKeys(q, allowed) {
		return adminglobal.PrivacyLegalHoldFilter{}, false
	}
	pageSize, ok := parsePageSize(q.Get("page_size"))
	if !ok {
		return adminglobal.PrivacyLegalHoldFilter{}, false
	}
	status := q.Get("status")
	switch status {
	case "", "activa", "liberada":
	default:
		return adminglobal.PrivacyLegalHoldFilter{}, false
	}
	scope := q.Get("scope")
	switch scope {
	case "", "expediente_privacidad", "cuenta_completa", "evidencia_kyc", "contratos_y_pagos", "incidente_seguridad":
	default:
		return adminglobal.PrivacyLegalHoldFilter{}, false
	}
	return adminglobal.PrivacyLegalHoldFilter{
		Status:    status,
		AccountID: q.Get("account_id"),
		Scope:     scope,
		PageSize:  pageSize,
		Cursor:    q.Get("cursor"),
	}, true
}
