package profilepin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authorizer interface {
	Authorize(ctx context.Context, raw identity.Secret, required identity.Role, activity identity.ActivityKind) (identity.Principal, error)
}

// Handler exposes a mountable, authenticated HTTP surface. The outer BFF must
// keep the bearer token server-side and forward requests with no-store.
type Handler struct {
	auth    Authorizer
	service *Service
	now     func() time.Time
}

func NewHandler(auth Authorizer, service *Service) http.Handler {
	return &Handler{auth: auth, service: service, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Vary", "Authorization, Cookie")
	if h.auth == nil || h.service == nil {
		writeError(w, 503, "unavailable")
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, 400, "invalid_request")
		return
	}
	principal, ok := h.authorize(w, r)
	if !ok {
		return
	}
	path := r.URL.Path
	switch {
	case path == "/api/v1/account/security/pinpass" && r.Method == http.MethodGet:
		configured, failure, err := h.service.Status(r.Context(), principal.AccountID, principal.SessionID)
		if err != nil {
			writeError(w, 500, "unavailable")
			return
		}
		var expires *time.Time
		if access, active, err := h.service.store.ActiveGrant(r.Context(), principal.AccountID, principal.SessionID, h.now().UTC()); err != nil {
			writeError(w, 500, "unavailable")
			return
		} else if active {
			t := access.ExpiresAt.UTC()
			expires = &t
		}
		writeJSON(w, 200, map[string]any{"configured": configured, "blocked_until": nullableTime(failure.BlockedUntil), "access_expires_at": expires})
	case path == "/api/v1/account/security/pinpass" && r.Method == http.MethodPut:
		var in struct {
			PIN             string `json:"pin"`
			Confirmation    string `json:"confirmation"`
			CurrentPassword string `json:"current_password"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.CurrentPassword == "" {
			writeError(w, 400, "reauthentication_required")
			return
		}
		if err := h.service.Set(r.Context(), principal.AccountID, principal.SessionID, in.PIN, in.Confirmation, "password:"+in.CurrentPassword); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"configured": true})
	case path == "/api/v1/account/security/pinpass/unlock" && r.Method == http.MethodPost:
		var in struct {
			PIN string `json:"pin"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		expires, err := h.service.Unlock(r.Context(), principal.AccountID, principal.SessionID, in.PIN)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"access_expires_at": expires.UTC()})
	case path == "/api/v1/account/security/pinpass" && r.Method == http.MethodDelete:
		var in struct {
			CurrentPassword string `json:"current_password"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		if in.CurrentPassword == "" {
			writeError(w, 400, "reauthentication_required")
			return
		}
		if err := h.service.Reset(r.Context(), principal.AccountID, principal.SessionID, "password:"+in.CurrentPassword); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/api/v1/account/security/pinpass/reveal/") && r.Method == http.MethodGet:
		field := strings.TrimPrefix(path, "/api/v1/account/security/pinpass/reveal/")
		if !supportedSensitiveField(field) {
			writeError(w, 404, "not_found")
			return
		}
		value, expires, err := h.service.Reveal(r.Context(), principal.AccountID, principal.SessionID, field)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"field": field, "value": value, "access_expires_at": expires.UTC()})
	default:
		writeError(w, 404, "not_found")
	}
}

func supportedSensitiveField(field string) bool {
	switch field {
	case "email", "run", "phone", "legal_name", "date_of_birth":
		return true
	default:
		return false
	}
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, PUT, POST, DELETE")
		writeError(w, 405, "method_not_allowed")
		return identity.Principal{}, false
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeError(w, 401, "unauthorized")
		return identity.Principal{}, false
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
	if p.RestrictedMode || errors.Is(err, identity.ErrRestrictedMode) || errors.Is(err, identity.ErrForbidden) {
		writeError(w, 403, "forbidden")
		return identity.Principal{}, false
	}
	if err != nil {
		writeError(w, 401, "unauthorized")
		return identity.Principal{}, false
	}
	if p.AccountID == "" || p.SessionID == "" {
		writeError(w, 401, "unauthorized")
		return identity.Principal{}, false
	}
	return p, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		writeError(w, 400, "invalid_request")
		return false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, 415, "unsupported_media_type")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_request")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_request")
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidPIN):
		writeError(w, 403, "invalid_pin")
	case errors.Is(err, ErrInvalidInput):
		writeError(w, 400, "invalid_request")
	case errors.Is(err, ErrLocked):
		var locked *LockedError
		retryAfter := int64(60)
		if errors.As(err, &locked) {
			retryAfter = int64(time.Until(locked.Until).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
		}
		w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
		writeError(w, 429, "temporarily_locked")
	case errors.Is(err, ErrNotConfigured):
		writeError(w, 409, "pinpass_not_configured")
	case errors.Is(err, ErrNoAccess):
		writeError(w, 403, "pinpass_access_required")
	case errors.Is(err, ErrProfileIncomplete):
		writeError(w, 409, "profile_incomplete")
	case errors.Is(err, ErrStepUpRejected):
		writeError(w, 403, "reauthentication_failed")
	default:
		writeError(w, 500, "unavailable")
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t.UTC()
	return &v
}
