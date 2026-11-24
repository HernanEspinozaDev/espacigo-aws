package publiccatalog

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type DestinationSuggestion struct {
	Kind        string   `json:"kind"` // "comuna" | "place"
	Label       string   `json:"label"`
	Subtitle    string   `json:"subtitle,omitempty"`
	RegionCode  string   `json:"region_code,omitempty"`
	CommuneCode string   `json:"comuna_code,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
}

type DestinationSuggester interface {
	SuggestDestinations(ctx context.Context, query string, limit int) ([]DestinationSuggestion, error)
}

type NearbyPOI struct {
	Name           string `json:"name"`
	Category       string `json:"category"` // "transit" | "parking"
	DistanceMeters int    `json:"distance_meters"`
	WalkingMinutes *int   `json:"walking_minutes,omitempty"`
	DistanceMode   string `json:"distance_mode"` // "route" | "straight_line"
}

type NearbyPOIsResponse struct {
	Available bool        `json:"available"`
	Items     []NearbyPOI `json:"items"`
}

type NearbyPOILookup interface {
	NearbyPOIs(ctx context.Context, latitude, longitude float64) ([]NearbyPOI, error)
}

// SuggestRateLimiter bounds unauthenticated suggest/POI lookups per client key.
type SuggestRateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
	limit   int
	window  time.Duration
}

func NewSuggestRateLimiter(limit int, window time.Duration) *SuggestRateLimiter {
	return &SuggestRateLimiter{
		buckets: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

func (l *SuggestRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	prev := l.buckets[key]
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.limit {
		l.buckets[key] = kept
		return false
	}
	l.buckets[key] = append(kept, now)
	if len(l.buckets) > 2048 {
		for k, v := range l.buckets {
			if len(v) == 0 || !v[len(v)-1].After(cutoff) {
				delete(l.buckets, k)
			}
		}
	}
	return true
}

func (s *Service) SuggestDestinations(ctx context.Context, query string, limit int) ([]DestinationSuggestion, error) {
	query = normalizeQuery(query)
	if query == "" || utf8.RuneCountInString(query) > 80 || limit < 1 || limit > 12 {
		return nil, ErrInvalid
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return nil, ErrInvalid
		}
	}
	communes, err := s.repo.Locations(ctx, query, "", limit)
	if err != nil {
		return nil, err
	}
	out := make([]DestinationSuggestion, 0, limit)
	for _, loc := range communes {
		out = append(out, DestinationSuggestion{
			Kind:        "comuna",
			Label:       loc.CommuneName,
			Subtitle:    loc.RegionName,
			RegionCode:  loc.RegionCode,
			CommuneCode: loc.CommuneCode,
		})
		if len(out) >= limit {
			return out, nil
		}
	}
	if s.destinationSuggester != nil && len(out) < limit && utf8.RuneCountInString(query) >= 3 {
		places, suggestErr := s.destinationSuggester.SuggestDestinations(ctx, query, limit-len(out))
		if suggestErr == nil {
			for _, p := range places {
				if p.Latitude == nil || p.Longitude == nil || !validPoint(*p.Latitude, *p.Longitude) {
					continue
				}
				p.Kind = "place"
				p.Label = strings.TrimSpace(p.Label)
				if p.Label == "" {
					continue
				}
				out = append(out, p)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out, nil
}

func (s *Service) NearbyPOIs(ctx context.Context, spaceID string) (NearbyPOIsResponse, error) {
	space, err := s.Detail(ctx, spaceID)
	if err != nil {
		return NearbyPOIsResponse{}, err
	}
	if space.Location == nil || s.nearbyPOILookup == nil {
		return NearbyPOIsResponse{Available: false, Items: []NearbyPOI{}}, nil
	}
	// Always query around the public projection (public_lat/public_lng when approximate).
	items, err := s.nearbyPOILookup.NearbyPOIs(ctx, space.Location.Latitude, space.Location.Longitude)
	if err != nil {
		return NearbyPOIsResponse{Available: false, Items: []NearbyPOI{}}, nil
	}
	filtered := make([]NearbyPOI, 0, len(items))
	for _, item := range items {
		if item.Category != "transit" && item.Category != "parking" {
			continue
		}
		if item.DistanceMode != "route" {
			item.DistanceMode = "straight_line"
			item.WalkingMinutes = nil
		}
		filtered = append(filtered, item)
	}
	return NearbyPOIsResponse{Available: true, Items: filtered}, nil
}
