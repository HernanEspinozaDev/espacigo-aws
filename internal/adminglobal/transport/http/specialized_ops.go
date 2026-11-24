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
	case "/api/v1/admin/global/support-tickets":
		return h.handleSupportTickets(w, r, requestID)
	case "/api/v1/admin/global/approvals":
		return h.handleApprovals(w, r, requestID)
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
		principal, ok := h.authorize(w, r, identity.CapabilityPrivacyRequestsManage, requestID)
		if !ok {
			return true
		}
		status, accountID, pageSize, cursor, valid := parseStatusAccountPagination(r.URL.Query())
		if !valid {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", requestID)
			return true
		}
		value, err := h.service.ListPrivacyRequests(r.Context(), principal.AccountID, requestID, adminglobal.PrivacyRequestFilter{
			Status: status, AccountID: accountID, PageSize: pageSize, Cursor: cursor,
		})
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
