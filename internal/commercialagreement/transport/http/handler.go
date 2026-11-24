package commercialagreementhttp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
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
func ETag(id string, revision int64) string {
	return `"commercial-agreement-` + id + `-` + strconv.FormatInt(revision, 10) + `"`
}
func parseETag(value, id string) (int64, bool) {
	prefix := `"commercial-agreement-` + id + `-`
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, `"`) {
		return 0, false
	}
	revision, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(value, prefix), `"`), 10, 64)
	return revision, err == nil && revision > 0 && ETag(id, revision) == value
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
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "ETag, X-Request-ID")
	}
	const prefix = "/api/v1/commercial-agreements"
	if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
		failure(w, 404, "not_found")
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.TrimPrefix(tail, "/"), "/")
	resourceID, action := "", ""
	isOTPChallenge := tail == "/otp-challenge"
	if tail != "" && !isOTPChallenge {
		if len(parts) > 2 || !domain.ValidID(parts[0]) {
			failure(w, 404, "not_found")
			return
		}
		resourceID = parts[0]
		if len(parts) == 2 {
			action = parts[1]
			if action != "landlord-signature" && action != "decision" {
				failure(w, 404, "not_found")
				return
			}
		}
	}
	allowed := "GET, POST, OPTIONS"
	if isOTPChallenge {
		allowed = "POST, OPTIONS"
	} else if resourceID != "" {
		allowed = "GET, OPTIONS"
		if action != "" {
			allowed = "PUT, OPTIONS"
		}
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", allowed)
		w.WriteHeader(204)
		return
	}
	if !strings.Contains(", "+allowed+",", ", "+r.Method+",") {
		w.Header().Set("Allow", allowed)
		failure(w, 405, "method_not_allowed")
		return
	}
	required := identity.Role("")
	if r.Method == http.MethodPost && !isOTPChallenge {
		required = identity.RoleTenant
	}
	if action == "landlord-signature" {
		required = identity.RoleLandlord
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(r.Header.Values("Authorization")) != 1 {
		failure(w, 401, "unauthenticated")
		return
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(header, "Bearer ")), required, identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			failure(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			failure(w, 401, "unauthenticated")
		}
		return
	}
	uaHash := sha256.Sum256([]byte(r.UserAgent()))
	actor := domain.Actor{
		ID:            p.AccountID,
		SessionID:     p.SessionID,
		IPMasked:      extractMaskedIP(r),
		UserAgentHash: uaHash[:],
	}
	if r.Method == http.MethodGet {
		if resourceID != "" {
			if r.URL.RawQuery != "" {
				failure(w, 400, "invalid_request")
				return
			}
			a, e := h.service.Get(r.Context(), p.AccountID, resourceID)
			respond(w, a, e, 200)
			return
		}
		values, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(values) > 1 || (len(values) == 1 && (len(values["role"]) != 1 || values.Get("role") == "")) {
			failure(w, 400, "invalid_request")
			return
		}
		items, e := h.service.List(r.Context(), p.AccountID, values.Get("role"))
		if e != nil {
			replyError(w, e)
			return
		}
		reply(w, 200, map[string]any{"items": items})
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "invalid_request")
		return
	}
	if isOTPChallenge {
		var in domain.OTPChallengeInput
		if !decode(w, r, &in) {
			return
		}
		res, e := h.service.RequestOTPChallenge(r.Context(), actor, in)
		if e != nil {
			replyError(w, e)
			return
		}
		reply(w, 201, res)
		return
	}
	if r.Method == http.MethodPost {
		if len(r.Header.Values("Idempotency-Key")) != 1 || !domain.ValidKey(r.Header.Get("Idempotency-Key")) {
			failure(w, 422, "idempotency_key_required")
			return
		}
		var in domain.CreateInput
		if !decode(w, r, &in) {
			return
		}
		a, e := h.service.Create(r.Context(), actor, in, r.Header.Get("Idempotency-Key"))
		status := 201
		if a.Replayed {
			status = 200
		}
		respond(w, a, e, status)
		return
	}
	if len(r.Header.Values("If-Match")) == 0 {
		failure(w, 428, "precondition_required")
		return
	}
	revision, ok := parseETag(r.Header.Get("If-Match"), resourceID)
	if !ok || len(r.Header.Values("If-Match")) != 1 {
		failure(w, 400, "invalid_precondition")
		return
	}
	if action == "landlord-signature" {
		var in domain.SignInput
		if !decode(w, r, &in) {
			return
		}
		a, e := h.service.Accept(r.Context(), actor, resourceID, revision, in)
		respond(w, a, e, 200)
		return
	}
	var in domain.DecisionInput
	if !decode(w, r, &in) {
		return
	}
	a, e := h.service.Decide(r.Context(), actor, resourceID, revision, in)
	respond(w, a, e, 200)
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, 415, "unsupported_media_type")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(dst); err != nil {
		failure(w, 400, "invalid_request")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		failure(w, 400, "invalid_request")
		return false
	}
	return true
}
func respond(w http.ResponseWriter, a domain.Agreement, err error, status int) {
	if err != nil {
		replyError(w, err)
		return
	}
	w.Header().Set("ETag", ETag(a.ID, a.Revision))
	reply(w, status, a)
}
func replyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		failure(w, 422, "validation_error")
	case errors.Is(err, domain.ErrNotFound):
		failure(w, 404, "not_found")
	case errors.Is(err, domain.ErrPrecondition):
		failure(w, 412, "precondition_failed")
	case errors.Is(err, domain.ErrHash):
		failure(w, 409, "proposal_changed")
	case errors.Is(err, domain.ErrExpired):
		failure(w, 409, "request_expired")
	case errors.Is(err, domain.ErrIdentity):
		failure(w, 409, "identity_verification_required")
	case errors.Is(err, domain.ErrIdempotency):
		failure(w, 409, "idempotency_conflict")
	case errors.Is(err, domain.ErrOTPRequired):
		failure(w, 422, "otp_required")
	case errors.Is(err, domain.ErrOTPInvalid):
		failure(w, 409, "otp_invalid")
	case errors.Is(err, domain.ErrOTPExpired):
		failure(w, 409, "otp_expired")
	case errors.Is(err, domain.ErrConflict):
		failure(w, 409, "agreement_conflict")
	default:
		failure(w, 500, "internal_error")
	}
}

func extractMaskedIP(r *http.Request) string {
	raw := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if raw == "" {
		raw = r.RemoteAddr
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		raw = host
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "unknown"
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String() + "/24"
	}
	masked := ip.Mask(net.CIDRMask(48, 128))
	return masked.String() + "/48"
}
func failure(w http.ResponseWriter, status int, code string) {
	reply(w, status, map[string]any{"error": map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")}})
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
