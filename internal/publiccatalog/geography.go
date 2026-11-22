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
	if p := f.Proximity; p != nil {
		if !validPoint(p.Latitude, p.Longitude) || !finite(p.RadiusKM) || p.RadiusKM < 1 || p.RadiusKM > 200 {
			return ErrInvalid
		}
	}
	return nil
}

// Map uses the same filtering and authenticated cursor as the grid. Clients
// exhaust next_cursor independently to obtain every marker in the query area.
func (s *Service) Map(ctx context.Context, filters Filters) (Page, error) {
	filters.MapOnly = true
	return s.List(ctx, filters)
}
