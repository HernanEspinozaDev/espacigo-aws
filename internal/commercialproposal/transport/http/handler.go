package commercialproposalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identityverification"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Service interface {
	Create(context.Context, string, string) (commercialproposal.View, error)
	Get(context.Context, string, string) (commercialproposal.View, error)
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
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	create := len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "rental-quotes" && parts[4] == "contract-proposal"
	get := len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "rental-contract-proposals"
	if !create && !get {
		failure(w, 404, "not_found")
		return
	}
	method := http.MethodGet
	if create {
		method = http.MethodPost
	}
	w.Header().Set("Allow", method+", OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return
	}
	if r.Method != method {
		failure(w, 405, "method_not_allowed")
		return
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), "", identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			failure(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			failure(w, 401, "unauthenticated")
		}
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 422, "invalid_request")
		return
	}
	if r.Body != nil {
		var probe [1]byte
		n, e := r.Body.Read(probe[:])
		if n > 0 || (e != nil && e != io.EOF) {
			failure(w, 422, "invalid_request")
			return
		}
	}
	var view commercialproposal.View
	if create {
		view, err = h.service.Create(r.Context(), p.AccountID, parts[3])
	} else {
		view, err = h.service.Get(r.Context(), p.AccountID, parts[3])
	}
	if err != nil {
		replyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": view})
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No fue posible preparar la propuesta de contrato.", "request_id": w.Header().Get("X-Request-ID")}})
}
func replyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, commercialproposal.ErrInvalid), errors.Is(err, rentalflow.ErrInvalid):
		failure(w, 422, "invalid_request")
	case errors.Is(err, commercialproposal.ErrNotFound), errors.Is(err, rentalflow.ErrNotFound):
		failure(w, 404, "not_found")
	case errors.Is(err, commercialproposal.ErrLocationRequired):
		failure(w, 409, "space_location_confirmation_required")
	case errors.Is(err, commercialproposal.ErrTemplateRequired):
		failure(w, 409, "contract_template_required")
	case errors.Is(err, commercialproposal.ErrConflict), errors.Is(err, rentalflow.ErrConflict):
		failure(w, 409, "contract_terms_changed")
	case errors.Is(err, rentalflow.ErrIdentityRequired), errors.Is(err, identityverification.ErrNotVerified):
		failure(w, 409, "identity_required")
	case errors.Is(err, commercialproposal.ErrTermsUnavailable):
		failure(w, 503, "contract_terms_unavailable")
	case errors.Is(err, commercialproposal.ErrEvidenceUnavailable):
		failure(w, 503, "contract_evidence_unavailable")
	case errors.Is(err, rentalflow.ErrIdentityUnavailable):
		failure(w, 503, "identity_unavailable")
	case errors.Is(err, rentalflow.ErrUnavailable):
		failure(w, 503, "service_unavailable")
	default:
		failure(w, 500, "internal_error")
	}
}
