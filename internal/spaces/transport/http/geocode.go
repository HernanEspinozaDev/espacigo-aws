package spaceshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

type GeocodeCandidate struct {
	Label     string  `json:"label"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Precision string  `json:"precision"`
}

type Geocoder interface {
	Search(context.Context, string) ([]GeocodeCandidate, error)
}
type ReverseGeocoder interface {
	Reverse(context.Context, float64, float64) (string, error)
}

type LocationCatalog interface {
	ResolveLocation(context.Context, string, string) (region, commune string, found bool, err error)
}

type geocodeWindow struct {
	start time.Time
	count int
}

type geocodeLimiter struct {
	mu    sync.Mutex
	users map[string]geocodeWindow
}

func (l *geocodeLimiter) allow(account string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.users == nil {
		l.users = make(map[string]geocodeWindow)
	}
	if len(l.users) >= 10000 {
		for id, window := range l.users {
			if now.Sub(window.start) >= time.Minute {
				delete(l.users, id)
			}
		}
	}
	if len(l.users) >= 10000 {
		return false
	}
	window := l.users[account]
	if now.Sub(window.start) >= time.Minute {
		window = geocodeWindow{start: now}
	}
	if window.count >= 10 {
		return false
	}
	window.count++
	l.users[account] = window
	return true
}

func (h *Handler) SetGeocoder(geocoder Geocoder, catalog LocationCatalog) {
	h.geocoder, h.locations = geocoder, catalog
	if h.service != nil {
		h.service.SetLocationCatalog(catalog)
	}
}
func (h *Handler) SetReverseGeocoder(reverse ReverseGeocoder, catalog LocationCatalog) {
	h.reverseGeocoder, h.locations = reverse, catalog
	if h.service != nil {
		h.service.SetLocationCatalog(catalog)
	}
}

func (h *Handler) serveGeocode(w http.ResponseWriter, r *http.Request) {
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
	if h.geocoder == nil || h.locations == nil {
		failure(w, 503, "geocoding_unavailable", "La búsqueda de direcciones no está disponible.")
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		failure(w, 415, "unsupported_media_type", "Se requiere application/json.")
		return
	}
	var in struct {
		Address     string `json:"address"`
		RegionCode  string `json:"region_code"`
		CommuneCode string `json:"comuna_code"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 8193))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.Decode(new(any)) != io.EOF {
		failure(w, 400, "invalid_request", "Solicitud JSON no válida.")
		return
	}
	in.Address = strings.TrimSpace(in.Address)
	if in.Address == "" || len([]rune(in.Address)) > 500 || spaces.AddressHasContact(in.Address) || (in.RegionCode == "") != (in.CommuneCode == "") || (in.RegionCode != "" && (len(in.RegionCode) != 2 || len(in.CommuneCode) != 5 || !strings.HasPrefix(in.CommuneCode, in.RegionCode))) {
		failure(w, 422, "validation_error", "Dirección o ubicación no válida.")
		return
	}
	for _, c := range in.RegionCode + in.CommuneCode {
		if c < '0' || c > '9' {
			failure(w, 422, "validation_error", "Dirección o ubicación no válida.")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	query := in.Address + ", Chile"
	if in.RegionCode != "" {
		region, commune, found, err := h.locations.ResolveLocation(ctx, in.RegionCode, in.CommuneCode)
		if err != nil {
			failure(w, 503, "geocoding_unavailable", "La búsqueda de direcciones no está disponible.")
			return
		}
		if !found {
			failureWithDetails(w, 422, "validation_error", "La comuna no pertenece al catálogo vigente.", []spaces.FieldError{{Field: "comuna_code", Code: "invalid_value"}})
			return
		}
		query = in.Address + ", " + commune + ", " + region + ", Chile"
	}
	if len([]rune(query)) > 200 {
		failureWithDetails(w, 422, "validation_error", "La dirección es demasiado larga para buscarla.", []spaces.FieldError{{Field: "address", Code: "max_length"}})
		return
	}
	if !h.geocodeLimit.allow(principal.AccountID, time.Now()) {
		failure(w, 429, "rate_limited", "Demasiadas búsquedas de direcciones. Intenta nuevamente en un minuto.")
		return
	}
	results, err := h.geocoder.Search(ctx, query)
	if err != nil {
		failure(w, 503, "geocoding_unavailable", "La búsqueda de direcciones no está disponible.")
		return
	}
	if results == nil {
		results = []GeocodeCandidate{}
	}
	write(w, 200, map[string]any{"candidates": results})
}
