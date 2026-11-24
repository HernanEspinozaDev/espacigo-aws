package commercialreservationdetailhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialreservationdetail"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Handler struct {
	auth    Authenticator
	service *domain.Service
	origins map[string]bool
}

func New(auth Authenticator, service *domain.Service, origins []string) http.Handler {
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
	requestID, err := (credentials.Generator{}).ID()
	if err != nil {
		failure(w, 500, "internal_error")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			failure(w, 403, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After, Content-Disposition, ETag")
	}
	const prefix = "/api/v1/commercial-reservations/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		failure(w, 404, "not_found")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) != 2 || (parts[1] != "documents" && parts[1] != "contract-document") {
		failure(w, 404, "not_found")
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, OPTIONS")
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, OPTIONS")
		failure(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "invalid_request")
		return
	}
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") || len(r.Header.Values("Authorization")) != 1 {
		failure(w, 401, "unauthenticated")
		return
	}
	principal, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(bearer, "Bearer ")), "", identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			failure(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			failure(w, 401, "unauthenticated")
		}
		return
	}
	if parts[1] == "documents" {
		documents, e := h.service.Get(r.Context(), principal.AccountID, parts[0])
		if e != nil {
			replyError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(documents)
		return
	}
	blob, digest, e := h.service.Download(r.Context(), principal.AccountID, parts[0])
	if e != nil {
		replyError(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="contrato-reserva-`+parts[0]+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
	w.Header().Set("ETag", `"sha256-`+digest+`"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(200)
	_, _ = w.Write(blob)
}
func replyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		failure(w, 404, "not_found")
	case errors.Is(err, domain.ErrPaymentRequired):
		failure(w, 409, "payment_required")
	case errors.Is(err, domain.ErrUnverified):
		failure(w, 409, "payment_not_verified")
	case errors.Is(err, domain.ErrDocumentPending):
		w.Header().Set("Retry-After", "5")
		failure(w, 409, "document_processing")
	case errors.Is(err, domain.ErrInvalid):
		failure(w, 422, "validation_error")
	default:
		w.Header().Set("Retry-After", "30")
		failure(w, 503, "document_unavailable")
	}
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")}})
}
