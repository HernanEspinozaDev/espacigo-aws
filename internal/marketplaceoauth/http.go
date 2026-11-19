package marketplaceoauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authorizer interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Handler struct {
	auth           Authorizer
	service        *Service
	frontendOrigin string
	allowedOrigins map[string]struct{}
}

func NewHandler(auth Authorizer, service *Service, frontendOrigin string, allowedOrigins []string) *Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return &Handler{auth: auth, service: service, frontendOrigin: strings.TrimRight(frontendOrigin, "/"), allowedOrigins: allowed}
}
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("/api/v1/host/mercadopago", h)
	mux.Handle("/api/v1/host/mercadopago/", h)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/api/v1/host/mercadopago/oauth/callback" {
		h.callback(w, r)
		return
	}
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/host/mercadopago":
		h.status(w, r, principal)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/host/mercadopago/oauth/start":
		h.start(w, r, principal)
	default:
		h.writeError(w, http.StatusNotFound, "not_found", "No se encontró la operación solicitada.")
	}
}
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	if h.auth == nil {
		h.writeError(w, http.StatusServiceUnavailable, "connection_unavailable", "La conexión de Mercado Pago no está disponible.")
		return identity.Principal{}, false
	}
	header := strings.Fields(r.Header.Get("Authorization"))
	if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
		h.writeError(w, http.StatusUnauthorized, "unauthenticated", "Inicia sesión para continuar.")
		return identity.Principal{}, false
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(header[1]), identity.RoleLandlord, identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			h.writeError(w, http.StatusForbidden, "forbidden", "Esta acción requiere una cuenta anfitriona.")
		} else {
			h.writeError(w, http.StatusUnauthorized, "unauthenticated", "Inicia sesión para continuar.")
		}
		return identity.Principal{}, false
	}
	return p, true
}
func (h *Handler) status(w http.ResponseWriter, r *http.Request, p identity.Principal) {
	if h.service == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{"connected": false, "authorization_available": false})
		return
	}
	c, err := h.service.Status(r.Context(), p.AccountID)
	if errors.Is(err, ErrNotFound) {
		h.writeJSON(w, http.StatusOK, map[string]any{"connected": false, "authorization_available": true})
		return
	}
	if err != nil {
		h.writeError(w, http.StatusServiceUnavailable, "connection_unavailable", "No se pudo consultar la conexión de Mercado Pago.")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"connected": !c.NeedsReauthorization, "seller_id": c.SellerID, "expires_at": c.ExpiresAt, "authorization_available": true, "reauthorization_required": c.NeedsReauthorization})
}
func (h *Handler) start(w http.ResponseWriter, r *http.Request, p identity.Principal) {
	if origin := r.Header.Get("Origin"); origin != "" {
		if _, ok := h.allowedOrigins[origin]; !ok {
			h.writeError(w, http.StatusForbidden, "origin_not_allowed", "Origen no permitido.")
			return
		}
	}
	if h.service == nil {
		h.writeError(w, http.StatusServiceUnavailable, "oauth_not_configured", "OAuth de Mercado Pago aún no está configurado.")
		return
	}
	url, err := h.service.Start(r.Context(), p.AccountID)
	if err != nil {
		h.writeError(w, http.StatusServiceUnavailable, "oauth_start_failed", "No se pudo iniciar la conexión con Mercado Pago.")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"authorization_url": url})
}
func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	status := "invalid_state"
	if h.service == nil {
		status = "not_configured"
	} else {
		state := r.URL.Query().Get("state")
		if providerError := r.URL.Query().Get("error"); providerError != "" {
			_, err := h.service.Complete(r.Context(), "", state)
			if errors.Is(err, ErrUserDenied) && providerError == "access_denied" {
				status = "denied"
			} else if errors.Is(err, ErrUserDenied) {
				status = "exchange_failed"
			}
		}
		if r.URL.Query().Get("error") == "" {
			_, err := h.service.Complete(r.Context(), r.URL.Query().Get("code"), state)
			switch {
			case err == nil:
				status = "connected"
			case errors.Is(err, ErrInvalidState):
				status = "invalid_state"
			case errors.Is(err, ErrUserDenied):
				status = "denied"
			case errors.Is(err, ErrWrongMode):
				status = "mode_mismatch"
			default:
				status = "exchange_failed"
			}
		}
	}
	location := h.frontendOrigin + "/anfitrion/mercado-pago?connection=" + url.QueryEscape(status)
	http.Redirect(w, r, location, http.StatusSeeOther)
}
func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func ConfigFromEnv(getenv func(string) string) (*Config, error) {
	id, secret, redirect, keyText := strings.TrimSpace(getenv("MERCADOPAGO_CLIENT_ID")), getenv("MERCADOPAGO_CLIENT_SECRET"), strings.TrimSpace(getenv("MERCADOPAGO_OAUTH_REDIRECT_URI")), strings.TrimSpace(getenv("MERCADOPAGO_TOKEN_ENCRYPTION_KEY"))
	if id == "" && secret == "" && redirect == "" && keyText == "" {
		return nil, nil
	}
	if id == "" || secret == "" || redirect == "" || keyText == "" {
		return nil, ErrNotConfigured
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return nil, ErrNotConfigured
	}
	mode := strings.TrimSpace(getenv("MERCADOPAGO_TEST_MODE"))
	if mode != "true" && mode != "false" {
		return nil, ErrNotConfigured
	}
	return &Config{ClientID: id, ClientSecret: secret, RedirectURI: redirect, EncryptionKey: key, TestMode: mode == "true"}, nil
}
