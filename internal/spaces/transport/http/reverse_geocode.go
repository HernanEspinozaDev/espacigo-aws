package spaceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

func (h *Handler) serveReverseGeocode(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		failure(w, 400, "invalid_request", "No se admiten parámetros de URL.")
		return
	}
	if r.Method != http.MethodPost {
		failure(w, 405, "method_not_allowed", "Método no permitido.")
		return
	}
	principal, err := h.auth.Authorize(r.Context(), identity.Secret(bearer(r.Header.Get("Authorization"))), identity.Role(""), identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			failure(w, 403, "forbidden", "Permiso insuficiente.")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			failure(w, 401, "unauthenticated", "Credencial ausente, incorrecta o expirada.")
		}
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		failure(w, 415, "unsupported_media_type", "Se requiere application/json.")
		return
	}
	var in struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4097))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF {
		failure(w, 400, "invalid_request", "Solicitud JSON no válida.")
		return
	}
	if in.Latitude == nil || in.Longitude == nil || math.IsNaN(*in.Latitude) || math.IsInf(*in.Latitude, 0) || math.Abs(*in.Latitude) > 90 || math.IsNaN(*in.Longitude) || math.IsInf(*in.Longitude, 0) || math.Abs(*in.Longitude) > 180 {
		failure(w, 422, "validation_error", "Coordenadas no válidas.")
		return
	}
	if !h.geocodeLimit.allow(principal.AccountID, time.Now()) {
		failure(w, 429, "rate_limited", "Demasiadas búsquedas de direcciones. Intenta nuevamente en un minuto.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	if h.service == nil || !h.service.HasLocationBoundaryChecker() || h.locations == nil {
		failure(w, 503, "geocoding_unavailable", "No pudimos resolver la ubicación.")
		return
	}
	regionCode, communeCode, resolveErr := h.service.ResolvePinCoordinates(ctx, *in.Latitude, *in.Longitude)
	if errors.Is(resolveErr, spaces.ErrLocationBoundaryUnavailable) {
		failure(w, 503, "geocoding_unavailable", "No pudimos resolver la ubicación.")
		return
	}
	if resolveErr != nil {
		failure(w, 422, "validation_error", "No encontramos una comuna para este punto.")
		return
	}
	out := map[string]string{"region_code": regionCode, "comuna_code": communeCode}
	if h.reverseGeocoder != nil {
		if address, err := h.reverseGeocoder.Reverse(ctx, *in.Latitude, *in.Longitude); err == nil && address != "" {
			out["address"] = address
		}
	}
	write(w, http.StatusOK, out)
}
