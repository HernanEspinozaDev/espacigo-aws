package spaceshttp

import (
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

// serveCalendarConfiguration is owner-only. The caller has already authorized
// the session; the repository rechecks ownership within its locked snapshot.
func (h *Handler) serveCalendarConfiguration(w http.ResponseWriter, r *http.Request, path, owner string) {
	const prefix = "/api/v1/spaces/"
	const suffix = "/availability/configuration"
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) || !spaces.ValidID(id) {
		failure(w, 404, "not_found", "Recurso no encontrado.")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "invalid_request", "No se admiten parámetros de URL.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		config, err := h.service.CalendarConfiguration(r.Context(), owner, id)
		if err != nil {
			serviceError(w, err)
			return
		}
		w.Header().Set("ETag", DraftETag(spaces.Draft{ID: id, UpdatedAt: config.UpdatedAt}))
		w.Header().Set("Cache-Control", "no-store")
		write(w, http.StatusOK, config)
	case http.MethodPut:
		// Reuse the strict inventory decoder and the transactional CAS writer; the
		// legacy time-zone-only operation and interval GET remain unchanged.
		h.serveCalendar(w, r, prefix+id+"/availability", owner)
	default:
		w.Header().Set("Allow", "GET, PUT")
		failure(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método no permitido.")
	}
}
