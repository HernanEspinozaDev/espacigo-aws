package spaceshttp

import (
	"errors"
	"net/http"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type communeBounds struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

func (h *Handler) serveCommuneBounds(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodOptions)
		failure(w, 405, "method_not_allowed", "Método no permitido.")
		return
	}
	_, err := h.auth.Authorize(r.Context(), identity.Secret(bearer(r.Header.Get("Authorization"))), "", identity.UserOperation)
	if errors.Is(err, identity.ErrForbidden) {
		failure(w, 403, "forbidden", "Permiso insuficiente.")
		return
	}
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		failure(w, 401, "unauthenticated", "Credencial ausente, incorrecta o expirada.")
		return
	}
	west, south, east, north, known := h.service.CommuneBounds(code)
	if !known {
		failure(w, 404, "boundary_unavailable", "No se encontró la extensión cartográfica de la comuna.")
		return
	}
	write(w, http.StatusOK, communeBounds{West: west, South: south, East: east, North: north})
}
