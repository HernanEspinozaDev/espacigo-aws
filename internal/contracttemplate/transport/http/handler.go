package contracttemplatehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Handler struct {
	auth    Authenticator
	repo    contracttemplate.Repository
	origins map[string]bool
}

func New(auth Authenticator, repo contracttemplate.Repository, origins []string) http.Handler {
	h := &Handler{auth: auth, repo: repo, origins: map[string]bool{}}
	for _, o := range origins {
		h.origins[o] = true
	}
	return h
}

func ETag(revision int64) string {
	return `"contract-template-` + strconv.FormatInt(revision, 10) + `"`
}
func parseETag(value string) (int64, bool) {
	if !strings.HasPrefix(value, `"contract-template-`) || !strings.HasSuffix(value, `"`) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(value, `"contract-template-`), `"`), 10, 64)
	return n, err == nil && n >= 0 && ETag(n) == value
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
		w.Header().Set("Access-Control-Expose-Headers", "ETag, X-Request-ID")
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "spaces" || parts[4] != "contract-template" {
		failure(w, 404, "not_found")
		return
	}
	action := parts[5]
	allowed := ""
	switch action {
	case "draft":
		allowed = "GET, PUT, OPTIONS"
	case "publish", "copy":
		allowed = "POST, OPTIONS"
	case "versions":
		allowed = "GET, OPTIONS"
	default:
		failure(w, 404, "not_found")
		return
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
	space := parts[3]
	if !contracttemplate.ValidIDs(p.AccountID, space) || r.URL.RawQuery != "" {
		failure(w, 422, "invalid_request")
		return
	}
	if r.Method == http.MethodGet {
		if action == "draft" {
			d, e := h.repo.Get(r.Context(), p.AccountID, space)
			if e != nil {
				replyError(w, e)
				return
			}
			w.Header().Set("ETag", ETag(d.Revision))
			reply(w, d)
		} else {
			v, e := h.repo.Versions(r.Context(), p.AccountID, space)
			if e != nil {
				replyError(w, e)
				return
			}
			reply(w, map[string]any{"items": v})
		}
		return
	}
	if r.Header.Get("If-Match") == "" {
		failure(w, 428, "precondition_required")
		return
	}
	revision, valid := parseETag(r.Header.Get("If-Match"))
	if !valid || len(r.Header.Values("If-Match")) != 1 {
		failure(w, 400, "invalid_precondition")
		return
	}
	switch action {
	case "draft":
		var body struct {
			Sections []contracttemplate.Section `json:"sections"`
		}
		if !decode(w, r, &body) {
			return
		}
		if _, _, e := contracttemplate.CanonicalSections(body.Sections); e != nil {
			replyError(w, e)
			return
		}
		d, e := h.repo.Save(r.Context(), p.AccountID, space, revision, body.Sections)
		if e != nil {
			replyError(w, e)
			return
		}
		w.Header().Set("ETag", ETag(d.Revision))
		reply(w, d)
	case "publish":
		// Empty body and {} both support ordinary browser POST requests.
		if r.Body != nil {
			blob, e := io.ReadAll(io.LimitReader(r.Body, 3))
			if e != nil || (len(blob) > 0 && string(blob) != "{}") {
				failure(w, 422, "invalid_request")
				return
			}
		}
		v, rev, e := h.repo.Publish(r.Context(), p.AccountID, space, revision)
		if e != nil {
			replyError(w, e)
			return
		}
		w.Header().Set("ETag", ETag(rev))
		reply(w, v)
	case "copy":
		var body struct {
			SourceSpaceID string `json:"source_space_id"`
			SourceVersion int64  `json:"source_version"`
		}
		if !decode(w, r, &body) {
			return
		}
		if !contracttemplate.ValidIDs(body.SourceSpaceID) || body.SourceVersion < 1 {
			failure(w, 422, "invalid_request")
			return
		}
		d, e := h.repo.Copy(r.Context(), p.AccountID, space, revision, body.SourceSpaceID, body.SourceVersion)
		if e != nil {
			replyError(w, e)
			return
		}
		w.Header().Set("ETag", ETag(d.Revision))
		reply(w, d)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, 415, "unsupported_media_type")
		return false
	}
	blob, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 160000))
	if err != nil {
		failure(w, 413, "request_too_large")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(blob))
	if !uniqueJSON(d, 0) {
		failure(w, 422, "invalid_request")
		return false
	}
	if _, err = d.Token(); err != io.EOF {
		failure(w, 422, "invalid_request")
		return false
	}
	d = json.NewDecoder(bytes.NewReader(blob))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		failure(w, 422, "invalid_request")
		return false
	}
	return true
}

// Reject duplicate keys and excessive nesting before decoding typed requests.
func uniqueJSON(d *json.Decoder, depth int) bool {
	if depth > 8 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return false
			}
			s, ok := key.(string)
			if !ok || seen[strings.ToLower(s)] {
				return false
			}
			seen[strings.ToLower(s)] = true
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		end, e := d.Token()
		return e == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueJSON(d, depth+1) {
				return false
			}
		}
		end, e := d.Token()
		return e == nil && end == json.Delim(']')
	}
	return false
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No fue posible completar la operación de plantilla.", "request_id": w.Header().Get("X-Request-ID")}})
}
func replyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, contracttemplate.ErrInvalid):
		failure(w, 422, "invalid_request")
	case errors.Is(err, contracttemplate.ErrNotFound):
		failure(w, 404, "not_found")
	case errors.Is(err, contracttemplate.ErrPrecondition):
		failure(w, 412, "precondition_failed")
	case errors.Is(err, contracttemplate.ErrConflict):
		failure(w, 409, "conflict")
	default:
		failure(w, 500, "internal_error")
	}
}
