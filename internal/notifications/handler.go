package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type Item struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Message     string     `json:"message"`
	Href        string     `json:"href"`
	ReferenceID *string    `json:"reference_id,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Handler struct {
	auth    Authenticator
	pool    *pgxpool.Pool
	origins map[string]bool
}

func NewHandler(auth Authenticator, pool *pgxpool.Pool, origins []string) *Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[o] = true
	}
	return &Handler{auth: auth, pool: pool, origins: allowed}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	reqID, err := (credentials.Generator{}).ID()
	if err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	w.Header().Set("X-Request-ID", reqID)
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			replyError(w, 403, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
	}

	const prefix = "/api/v1/notifications"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(204)
		return
	}

	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(r.Header.Values("Authorization")) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		replyError(w, 401, "unauthenticated")
		return
	}
	principal, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(header, "Bearer ")), identity.Role(""), identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			replyError(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			replyError(w, 401, "unauthenticated")
		}
		return
	}

	if tail == "" && r.Method == http.MethodGet {
		h.list(w, r, principal.AccountID)
		return
	}
	if tail == "/read-all" && r.Method == http.MethodPost {
		h.readAll(w, r, principal.AccountID)
		return
	}
	if strings.HasPrefix(tail, "/") && strings.HasSuffix(tail, "/read") && r.Method == http.MethodPost {
		id := strings.TrimSuffix(strings.TrimPrefix(tail, "/"), "/read")
		if _, err := uuid.Parse(id); err != nil {
			replyError(w, 404, "not_found")
			return
		}
		h.markRead(w, r, principal.AccountID, id)
		return
	}
	replyError(w, 404, "not_found")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, accountID string) {
	var unreadCount int
	if err := h.pool.QueryRow(r.Context(), `SELECT count(*)::int FROM public.notificacion_usuario WHERE usuario_id = $1 AND leida_en IS NULL`, accountID).Scan(&unreadCount); err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id::text, tipo, titulo, mensaje, href, referencia_id::text, leida_en, creada_en FROM public.notificacion_usuario WHERE usuario_id = $1 ORDER BY creada_en DESC, id DESC LIMIT 30`, accountID)
	if err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Message, &item.Href, &item.ReferenceID, &item.ReadAt, &item.CreatedAt); err != nil {
			replyError(w, 500, "internal_error")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	replyJSON(w, 200, map[string]any{
		"unread_count": unreadCount,
		"items":        items,
	})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request, accountID, id string) {
	tag, err := h.pool.Exec(r.Context(), `UPDATE public.notificacion_usuario SET leida_en = COALESCE(leida_en, now()) WHERE id = $1 AND usuario_id = $2`, id, accountID)
	if err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	if tag.RowsAffected() == 0 {
		replyError(w, 404, "not_found")
		return
	}
	replyJSON(w, 200, map[string]any{"ok": true})
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request, accountID string) {
	if _, err := h.pool.Exec(r.Context(), `UPDATE public.notificacion_usuario SET leida_en = now() WHERE usuario_id = $1 AND leida_en IS NULL`, accountID); err != nil {
		replyError(w, 500, "internal_error")
		return
	}
	replyJSON(w, 200, map[string]any{"unread_count": 0})
}

func replyError(w http.ResponseWriter, status int, code string) {
	replyJSON(w, status, map[string]any{"error": map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")}})
}

func replyJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
