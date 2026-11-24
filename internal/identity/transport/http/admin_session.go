package identityhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

// AdminSessionHandler serves the isolated /api/v1/admin/session* endpoints.
type AdminSessionHandler struct {
	service *identity.AdminSessionService
	origins map[string]bool
}

// NewAdminSessionHandler constructs an HTTP handler for admin staff sessions.
func NewAdminSessionHandler(service *identity.AdminSessionService, origins []string) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &AdminSessionHandler{service: service, origins: allowed}
}

func (h *AdminSessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID, err := (credentials.Generator{}).ID()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Add("Vary", "Origin")
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			h.writeError(w, http.StatusForbidden, "origin_denied", requestID)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.service == nil {
		h.writeError(w, http.StatusNotFound, "not_found", requestID)
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/api/v1/admin/session/start":
		if r.Method != http.MethodPost {
			h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
			return
		}
		h.handleStart(w, r, requestID)
	case "/api/v1/admin/session/mfa":
		if r.Method != http.MethodPost {
			h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
			return
		}
		h.handleMFA(w, r, requestID)
	case "/api/v1/admin/session":
		switch r.Method {
		case http.MethodGet:
			h.handleGet(w, r, requestID)
		case http.MethodDelete:
			h.handleDelete(w, r, requestID)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		}
	default:
		h.writeError(w, http.StatusNotFound, "not_found", requestID)
	}
}

func (h *AdminSessionHandler) handleStart(w http.ResponseWriter, r *http.Request, requestID string) {
	var body struct {
		Email    string          `json:"email"`
		Password identity.Secret `json:"password"`
	}
	if !decodeAdminJSON(w, r, &body) {
		h.writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return
	}
	res, err := h.service.Start(r.Context(), identity.AdminSessionStartInput{
		Email:     body.Email,
		Password:  body.Password,
		UserAgent: r.UserAgent(),
		IP:        clientIPForDevice(r),
	})
	if err != nil {
		h.serviceError(w, err, requestID)
		return
	}
	if res.Status == "mfa_required" {
		h.writeJSON(w, http.StatusOK, map[string]any{
			"status":       res.Status,
			"challenge_id": res.ChallengeID,
			"methods":      res.Methods,
			"expires_at":   res.ExpiresAt,
		})
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"status":                 res.Status,
		"account_id":             res.AccountID,
		"audience":               res.Audience,
		"roles":                  res.Roles,
		"capabilities":           res.Capabilities,
		"token":                  string(res.Token),
		"mfa_verified_at":        res.MFAVerifiedAt,
		"recent_auth_expires_at": res.RecentAuthExpiresAt,
		"expires_at":             res.ExpiresAt,
	})
}

func (h *AdminSessionHandler) handleMFA(w http.ResponseWriter, r *http.Request, requestID string) {
	var body struct {
		ChallengeID string `json:"challenge_id"`
		Code        string `json:"code"`
	}
	if !decodeAdminJSON(w, r, &body) {
		h.writeError(w, http.StatusBadRequest, "invalid_request", requestID)
		return
	}
	res, err := h.service.VerifyMFA(r.Context(), identity.AdminSessionMFAInput{
		ChallengeID: strings.TrimSpace(body.ChallengeID),
		AdminToken:  identity.Secret(adminBearerToken(r)),
		Code:        body.Code,
		UserAgent:   r.UserAgent(),
		IP:          clientIPForDevice(r),
	})
	if err != nil {
		h.serviceError(w, err, requestID)
		return
	}
	payload := map[string]any{
		"status":                 res.Status,
		"account_id":             res.AccountID,
		"audience":               res.Audience,
		"roles":                  res.Roles,
		"capabilities":           res.Capabilities,
		"mfa_verified_at":        res.MFAVerifiedAt,
		"recent_auth_expires_at": res.RecentAuthExpiresAt,
		"expires_at":             res.ExpiresAt,
	}
	if res.Token != "" {
		payload["token"] = string(res.Token)
	}
	h.writeJSON(w, http.StatusOK, payload)
}

func (h *AdminSessionHandler) handleGet(w http.ResponseWriter, r *http.Request, requestID string) {
	token := adminBearerToken(r)
	if token == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		h.writeError(w, http.StatusUnauthorized, "unauthenticated", requestID)
		return
	}
	info, err := h.service.GetSession(r.Context(), identity.Secret(token))
	if err != nil {
		h.serviceError(w, err, requestID)
		return
	}
	h.writeJSON(w, http.StatusOK, info)
}

func (h *AdminSessionHandler) handleDelete(w http.ResponseWriter, r *http.Request, requestID string) {
	token := adminBearerToken(r)
	if token == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		h.writeError(w, http.StatusUnauthorized, "unauthenticated", requestID)
		return
	}
	if err := h.service.RevokeSession(r.Context(), identity.Secret(token)); err != nil {
		h.serviceError(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminSessionHandler) serviceError(w http.ResponseWriter, err error, requestID string) {
	var blocked *identity.LoginBlockedError
	switch {
	case errors.As(err, &blocked):
		h.writeError(w, http.StatusTooManyRequests, "login_blocked", requestID)
	case errors.Is(err, identity.ErrCredentials):
		h.writeError(w, http.StatusUnauthorized, "invalid_credentials", requestID)
	case errors.Is(err, identity.ErrAccountDisabled):
		h.writeError(w, http.StatusForbidden, "account_disabled", requestID)
	case errors.Is(err, identity.ErrRestrictedMode):
		h.writeError(w, http.StatusForbidden, "restricted_account", requestID)
	case errors.Is(err, identity.ErrAdminStaffRequired):
		h.writeError(w, http.StatusForbidden, "forbidden", requestID)
	case errors.Is(err, identity.ErrAdminMFAEnrollmentRequired):
		h.writeError(w, http.StatusForbidden, "mfa_enrollment_required", requestID)
	case errors.Is(err, identity.ErrAdminMFAChallengeExpired):
		h.writeError(w, http.StatusUnauthorized, "mfa_challenge_expired", requestID)
	case errors.Is(err, identity.ErrAdminMFAChallengeExhausted):
		h.writeError(w, http.StatusTooManyRequests, "mfa_attempts_exceeded", requestID)
	case errors.Is(err, identity.ErrAdminMFACodeInvalid):
		h.writeError(w, http.StatusUnauthorized, "invalid_mfa_code", requestID)
	case errors.Is(err, identity.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", "Bearer")
		h.writeError(w, http.StatusUnauthorized, "unauthenticated", requestID)
	case errors.Is(err, identity.ErrForbidden):
		h.writeError(w, http.StatusForbidden, "forbidden", requestID)
	case errors.Is(err, identity.ErrTOTPKeyringMissing):
		h.writeError(w, http.StatusServiceUnavailable, "mfa_unavailable", requestID)
	default:
		h.writeError(w, http.StatusInternalServerError, "internal_error", requestID)
	}
}

func (h *AdminSessionHandler) writeError(w http.ResponseWriter, status int, code, requestID string) {
	h.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    "No fue posible completar la operación.",
			"request_id": requestID,
		},
	})
}

func (h *AdminSessionHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func adminBearerToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}
