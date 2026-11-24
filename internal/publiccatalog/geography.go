package publiccatalog

import (
	"context"
	"math"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validPoint(lat, lon float64) bool {
	return finite(lat) && finite(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
func validateGeo(f Filters) error {
	if f.Bounds != nil && f.Proximity != nil {
		return ErrInvalid
	}
	if b := f.Bounds; b != nil {
		if !validPoint(b.South, b.West) || !validPoint(b.North, b.East) || b.South >= b.North || b.West >= b.East {
			return ErrInvalid
		}
	}
	if b := f.ExcludeBounds; b != nil {
		if !validPoint(b.South, b.West) || !validPoint(b.North, b.East) || b.South >= b.North || b.West >= b.East {
			return ErrInvalid
		}
	}
	if p := f.Proximity; p != nil {
		if !validPoint(p.Latitude, p.Longitude) {
			return ErrInvalid
		}
		if !f.NearbyFallback && (!finite(p.RadiusKM) || p.RadiusKM < 1 || p.RadiusKM > 200) {
			return ErrInvalid
		}
	}
	if f.ExcludeRadiusKM != nil && (!finite(*f.ExcludeRadiusKM) || *f.ExcludeRadiusKM < 0 || *f.ExcludeRadiusKM > 200) {
		return ErrInvalid
	}
	return nil
}

// Map returns up to 250 lightweight markers in a single response with total_count
// and has_more for the active query area.
func (s *Service) Map(ctx context.Context, filters Filters) (Page, error) {
	filters.MapOnly = true
	page, err := s.List(ctx, filters)
	if err != nil {
		return Page{}, err
	}
	if len(page.Items) > 0 {
		page.TotalCount = page.Items[0].TotalCount
	}
	page.HasMore = page.TotalCount > len(page.Items) || page.NextCursor != ""
	return page, nil
}

// Nearby returns up to 6 closest spaces outside the current empty area while
// preserving non-geographic filters.
func (s *Service) Nearby(ctx context.Context, filters Filters) (Page, error) {
	if filters.Proximity == nil {
		return Page{}, ErrInvalid
	}
	filters.MapOnly = true
	filters.NearbyFallback = true
	filters.Bounds = nil
	filters.RegionCode = ""
	filters.CommuneCode = ""
	filters.PageSize = NearbyLimit
	filters.PageSizeSet = true
	filters.Sort = "distance_asc"
	filters.Cursor = ""
	page, err := s.List(ctx, filters)
	if err != nil {
		return Page{}, err
	}
	page.NextCursor = ""
	page.HasMore = false
	page.TotalCount = len(page.Items)
	return page, nil
}
