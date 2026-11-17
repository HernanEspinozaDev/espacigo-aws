package earlyrentalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type CapabilityState struct {
	ActivationRequested bool
	FlowReady           bool
}

type capabilitiesHandler struct {
	auth    Authenticator
	state   CapabilityState
	origins map[string]bool
}

type capabilitiesEnvelope struct {
	Data capabilitiesData `json:"data"`
}

type capabilitiesData struct {
	Enabled            bool     `json:"enabled"`
	FlowVersion        string   `json:"flow_version"`
	SupportedRateUnits []string `json:"supported_rate_units"`
	HoldTTLSeconds     int      `json:"hold_ttl_seconds"`
}

func NewCapabilitiesHandler(auth Authenticator, state CapabilityState, origins []string) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &capabilitiesHandler{auth: auth, state: state, origins: allowed}
}

func (h *capabilitiesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID, err := (credentials.Generator{}).ID()
	if err != nil {
		capabilitiesFail(w, http.StatusInternalServerError, "internal_error", "")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Add("Vary", "Origin")

	origins := r.Header.Values("Origin")
	if len(origins) > 1 {
		capabilitiesFail(w, http.StatusForbidden, "origin_denied", requestID)
		return
	}
	if len(origins) == 1 && origins[0] != "" {
		if !h.origins[origins[0]] {
			capabilitiesFail(w, http.StatusForbidden, "origin_denied", requestID)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origins[0])
		w.Header().Set("Access-Control-Allow-Headers", "Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, OPTIONS")
		capabilitiesFail(w, http.StatusMethodNotAllowed, "method_not_allowed", requestID)
		return
	}
	if r.URL.RawQuery != "" {
		capabilitiesFail(w, http.StatusBadRequest, "invalid_request", requestID)
		return
	}
	if h.auth == nil {
		capabilitiesFail(w, http.StatusServiceUnavailable, "service_unavailable", requestID)
		return
	}
	principal, authErr := h.auth.Authorize(r.Context(), identity.Secret(bearerToken(r.Header.Get("Authorization"))), "", identity.UserOperation)
	if authErr != nil {
		switch {
		case errors.Is(authErr, identity.ErrForbidden), errors.Is(authErr, identity.ErrRestrictedMode):
			capabilitiesFail(w, http.StatusForbidden, "forbidden", requestID)
		case errors.Is(authErr, identity.ErrUnauthorized):
			capabilitiesFail(w, http.StatusUnauthorized, "unauthenticated", requestID)
		default:
			capabilitiesFail(w, http.StatusServiceUnavailable, "service_unavailable", requestID)
		}
		return
	}
	if principal.AccountID == "" {
		capabilitiesFail(w, http.StatusUnauthorized, "unauthenticated", requestID)
		return
	}
	if h.state.ActivationRequested && !h.state.FlowReady {
		capabilitiesFail(w, http.StatusServiceUnavailable, "service_unavailable", requestID)
		return
	}
	writeCapabilities(w, http.StatusOK, capabilitiesEnvelope{Data: capabilitiesData{
		Enabled:            h.state.ActivationRequested && h.state.FlowReady,
		FlowVersion:        "manual_early_v1",
		SupportedRateUnits: []string{"hora"},
		HoldTTLSeconds:     int(earlyrental.HoldTTL / time.Second),
	}})
}

func bearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func capabilitiesFail(w http.ResponseWriter, status int, code, requestID string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeCapabilities(w, status, map[string]any{"error": map[string]string{
		"code": code, "message": "No fue posible completar la solicitud.", "request_id": requestID,
	}})
}

func writeCapabilities(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
