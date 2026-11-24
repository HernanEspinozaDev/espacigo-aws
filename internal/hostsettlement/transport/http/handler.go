package hostsettlementhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/hostsettlement"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Service interface {
	List(context.Context, string, string, string, string) (hostsettlement.Page, error)
	Get(context.Context, string, string) (hostsettlement.Item, error)
}
type Handler struct {
	auth    Authenticator
	service Service
	origins map[string]bool
}

func New(auth Authenticator, service Service, origins []string) http.Handler {
	h := &Handler{auth: auth, service: service, origins: map[string]bool{}}
	for _, o := range origins {
		h.origins[o] = true
	}
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	id, err := (credentials.Generator{}).ID()
	if err != nil {
		failure(w, 500, "internal_error")
		return
	}
	w.Header().Set("X-Request-ID", id)
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			failure(w, 403, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	}
	w.Header().Set("Allow", "GET, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodGet {
		failure(w, 405, "method_not_allowed")
		return
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), identity.RoleLandlord, identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			failure(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			failure(w, 401, "unauthenticated")
		}
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "host" || parts[3] != "settlements" {
		failure(w, 404, "not_found")
		return
	}
	var result any
	if len(parts) == 4 {
		query, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil {
			failure(w, 422, "invalid_request")
			return
		}
		for key, values := range query {
			if (key != "month" && key != "mode" && key != "cursor") || len(values) != 1 {
				failure(w, 422, "invalid_request")
				return
			}
		}
		result, err = h.service.List(r.Context(), p.AccountID, query.Get("month"), query.Get("mode"), query.Get("cursor"))
	} else if len(parts) == 5 && r.URL.RawQuery == "" {
		result, err = h.service.Get(r.Context(), p.AccountID, parts[4])
	} else {
		failure(w, 404, "not_found")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, hostsettlement.ErrInvalid):
			failure(w, 422, "invalid_request")
		case errors.Is(err, hostsettlement.ErrNotFound):
			failure(w, 404, "not_found")
		case errors.Is(err, hostsettlement.ErrUnavailable):
			failure(w, 503, "settlement_data_unavailable")
		default:
			failure(w, 500, "internal_error")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": result})
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No fue posible consultar la liquidación.", "request_id": w.Header().Get("X-Request-ID")}})
}
