package spaceshttp

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

func locationSpaceID(path string) string {
	const prefix, suffix = "/api/v1/spaces/", "/location"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}
func (h *Handler) serveLocation(w http.ResponseWriter, r *http.Request, id, owner string) {
	var item spaces.CommercialLocation
	var err error
	switch r.Method {
	case http.MethodGet:
		item, err = h.service.GetOwnLocation(r.Context(), owner, id)
	case http.MethodPut:
		timestamp, e := parseIfMatch(r.Header.Values("If-Match"), id)
		if e != nil {
			switch {
			case errors.Is(e, ErrMissingPrecondition):
				failure(w, 428, "precondition_required", "Se requiere If-Match.")
			case errors.Is(e, ErrStalePrecondition):
				failure(w, 412, "precondition_failed", "La versión del espacio cambió.")
			default:
				failure(w, 400, "invalid_precondition", "If-Match no es válido.")
			}
			return
		}
		expected, e := time.Parse(time.RFC3339Nano, timestamp)
		if e != nil {
			failure(w, 400, "invalid_precondition", "If-Match no es válido.")
			return
		}
		var body struct {
			Address         string   `json:"address"`
			RegionCode      string   `json:"region_code"`
			CommuneCode     string   `json:"comuna_code"`
			Latitude        *float64 `json:"latitude"`
			Longitude       *float64 `json:"longitude"`
			Source          string   `json:"source,omitempty"`
			Precision       string   `json:"precision,omitempty"`
			PublicConfirmed *bool    `json:"public_confirmed"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.PublicConfirmed == nil {
			failureWithDetails(w, 422, "validation_error", "Revisa los campos obligatorios y sus límites.", []spaces.FieldError{{Field: "public_confirmed", Code: "required"}})
			return
		}
		in := spaces.LocationInput{Address: body.Address, RegionCode: body.RegionCode, CommuneCode: body.CommuneCode, Latitude: body.Latitude, Longitude: body.Longitude, Source: body.Source, Precision: body.Precision, PublicConfirmed: *body.PublicConfirmed}
		item, err = h.service.SetOwnLocation(r.Context(), owner, id, expected, in)
	default:
		failure(w, 405, "method_not_allowed", "Método no permitido.")
		return
	}
	if err != nil {
		if errors.Is(err, spaces.ErrPreconditionFailed) {
			failure(w, 412, "precondition_failed", "La versión del espacio cambió.")
		} else {
			serviceError(w, err)
		}
		return
	}
	w.Header().Set("ETag", draftETag(spaces.Draft{ID: item.SpaceID, UpdatedAt: item.UpdatedAt}))
	write(w, 200, item)
}
