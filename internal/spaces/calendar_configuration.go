package spaces

import (
	"context"
	"time"
)

// CalendarConfiguration is the private owner's calendar configuration view.
type CalendarConfiguration struct {
	SpaceID   string                  `json:"space_id"`
	TimeZone  string                  `json:"time_zone"`
	Rule      AvailabilityRule        `json:"rule"`
	Overrides []CalendarOverrideInput `json:"overrides"`
	UpdatedAt time.Time               `json:"updated_at"`
}

// CalendarConfigurationRepository is deliberately separate from the broad draft repository.
type CalendarConfigurationRepository interface {
	GetCalendarConfiguration(ctx context.Context, owner, spaceID string) (CalendarConfiguration, error)
}

func (s *Service) CalendarConfiguration(ctx context.Context, owner, spaceID string) (CalendarConfiguration, error) {
	if !validOwner(owner) || !validID(spaceID) {
		return CalendarConfiguration{}, ErrNotFound
	}
	r, ok := s.repo.(CalendarConfigurationRepository)
	if !ok {
		return CalendarConfiguration{}, ErrNotFound
	}
	return r.GetCalendarConfiguration(ctx, owner, spaceID)
}
