package commercialcommunityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/reputation"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type Handler struct {
	auth         Authenticator
	conversation *conversation.Service
	reputation   *reputation.Service
	origins      map[string]bool
}

func NewHandler(auth Authenticator, conversations *conversation.Service, reviews *reputation.Service, origins []string) *Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &Handler{auth: auth, conversation: conversations, reputation: reviews, origins: allowed}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rid, err := (credentials.Generator{}).ID()
	if err != nil {
		h.writeError(w, 500, "internal_error", "")
		return
	}
	w.Header().Set("X-Request-ID", rid)
	w.Header().Add("Vary", "Origin")
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			h.writeError(w, 403, "origin_denied", rid)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	const base = "/api/v1/commercial-reservations/"
	if !strings.HasPrefix(path, base) {
		h.writeError(w, 404, "not_found", rid)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, base), "/")
	if (len(parts) != 2 && len(parts) != 3) || parts[0] == "" {
		h.writeError(w, 404, "not_found", rid)
		return
	}
	principal, authErr := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), "", identity.UserOperation)
	if authErr != nil {
		if errors.Is(authErr, identity.ErrForbidden) {
			h.writeError(w, 403, "forbidden", rid)
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			h.writeError(w, 401, "unauthenticated", rid)
		}
		return
	}
	actor, reservation := principal.AccountID, parts[0]
	if len(parts) == 3 {
		if parts[1] == "messages" && parts[2] == "read" && r.Method == http.MethodPost {
			h.markMessagesRead(w, r, actor, reservation, rid)
			return
		}
		h.writeError(w, 404, "not_found", rid)
		return
	}
	switch parts[1] {
	case "messages":
		h.messages(w, r, actor, reservation, rid)
	case "reviews":
		h.reviews(w, r, actor, reservation, rid)
	default:
		h.writeError(w, 404, "not_found", rid)
	}
}

func (h *Handler) markMessagesRead(w http.ResponseWriter, r *http.Request, actor, reservation, rid string) {
	if r.URL.RawQuery != "" {
		h.writeError(w, 400, "invalid_request", rid)
		return
	}
	var in struct {
		ThroughSequence int64 `json:"through_sequence"`
	}
	if !decode(w, r, &in) {
		h.writeError(w, 422, "invalid_request", rid)
		return
	}
	cursor, err := h.conversation.MarkCommercialRead(r.Context(), actor, reservation, in.ThroughSequence)
	h.reply(w, 200, map[string]int64{"read_through_sequence": cursor}, err, rid, false)
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request, actor, reservation, rid string) {
	if r.URL.RawQuery != "" {
		q, parseErr := url.ParseQuery(r.URL.RawQuery)
		if parseErr != nil {
			h.writeError(w, 400, "invalid_request", rid)
			return
		}
		for key, values := range q {
			if (key != "before" && key != "limit") || len(values) != 1 || values[0] == "" {
				h.writeError(w, 400, "invalid_request", rid)
				return
			}
		}
		if q.Get("before") != "" || q.Get("limit") != "" {
			if r.Method != http.MethodGet {
				h.writeError(w, 400, "invalid_request", rid)
				return
			}
		}
	}
	switch r.Method {
	case http.MethodGet:
		var before *int64
		if raw := r.URL.Query().Get("before"); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				h.writeError(w, 400, "invalid_request", rid)
				return
			}
			before = &n
		}
		limit := conversation.DefaultPageSize
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				h.writeError(w, 400, "invalid_request", rid)
				return
			}
			limit = n
		}
		value, err := h.conversation.ListCommercial(r.Context(), actor, reservation, before, limit)
		h.reply(w, 200, value, err, rid, false)
	case http.MethodPost:
		var in struct {
			Body string `json:"body"`
		}
		if !decode(w, r, &in) {
			h.writeError(w, 422, "invalid_request", rid)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			h.writeError(w, 422, "idempotency_key_required", rid)
			return
		}
		value, err := h.conversation.SendCommercial(r.Context(), actor, reservation, key, in.Body)
		status := 201
		if err == nil { /* replay returns the same id; callers can treat it as success. */
		}
		h.reply(w, status, value, err, rid, false)
	default:
		h.writeError(w, 405, "method_not_allowed", rid)
	}
}

func (h *Handler) reviews(w http.ResponseWriter, r *http.Request, actor, reservation, rid string) {
	if r.URL.RawQuery != "" {
		h.writeError(w, 400, "invalid_request", rid)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := h.reputation.ListCommercial(r.Context(), actor, reservation)
		h.reply(w, 200, value, err, rid, true)
	case http.MethodPost:
		var in reputation.ReviewInput
		if !decode(w, r, &in) {
			h.writeError(w, 422, "invalid_request", rid)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			h.writeError(w, 422, "idempotency_key_required", rid)
			return
		}
		value, err := h.reputation.CreateCommercial(r.Context(), actor, reservation, key, in)
		status := 201
		if value.Reused {
			status = 200
		}
		h.reply(w, status, value, err, rid, true)
	default:
		h.writeError(w, 405, "method_not_allowed", rid)
	}
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(d.Decode(&extra), io.EOF)
}

func (h *Handler) reply(w http.ResponseWriter, status int, value any, err error, rid string, review bool) {
	if err != nil {
		var invalid, missing, conflict bool
		if review {
			invalid = errors.Is(err, reputation.ErrInvalid)
			missing = errors.Is(err, reputation.ErrNotFound)
			conflict = errors.Is(err, reputation.ErrConflict)
		} else {
			invalid = errors.Is(err, conversation.ErrInvalid)
			missing = errors.Is(err, conversation.ErrNotFound)
			conflict = errors.Is(err, conversation.ErrConflict)
		}
		switch {
		case invalid:
			h.writeError(w, 422, "invalid_request", rid)
		case missing:
			h.writeError(w, 404, "not_found", rid)
		case conflict:
			h.writeError(w, 409, "state_or_idempotency_conflict", rid)
		default:
			h.writeError(w, 500, "internal_error", rid)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": value})
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, rid string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No fue posible completar la operación.", "request_id": rid}})
}
