package kyb

import (
	"context"
	"errors"
	"strings"
	"time"
)

type ReminderStage string

const (
	ReminderDay0        ReminderStage = "day0"
	ReminderMonth1      ReminderStage = "month1"
	ReminderDay15       ReminderStage = "day15"
	ReminderDay7        ReminderStage = "day7"
	ReminderDay1        ReminderStage = "day1"
	ReminderDeadline    ReminderStage = "due"
	ReminderReactivated ReminderStage = "reactivated"
)

type ScheduledReminder struct {
	Stage       ReminderStage
	AvailableAt time.Time
}

// NextDueReminder returns the latest scheduled reminder that is due and has
// not already been enqueued. Choosing the latest due stage avoids sending a
// burst of stale warnings after a worker outage.
func NextDueReminder(startedAt, dueAt time.Time, lastStage string, now time.Time) (ScheduledReminder, bool, error) {
	if startedAt.IsZero() || dueAt.IsZero() || now.IsZero() || !dueAt.After(startedAt) {
		return ScheduledReminder{}, false, ErrInvalid
	}
	last := -1
	switch ReminderStage(lastStage) {
	case "":
	case ReminderDay0:
		last = 0
	case ReminderMonth1:
		last = 1
	case ReminderDay15:
		last = 2
	case ReminderDay7:
		last = 3
	case ReminderDay1:
		last = 4
	case ReminderDeadline:
		last = 5
	default:
		return ScheduledReminder{}, false, ErrInvalid
	}
	schedule := []ScheduledReminder{
		{ReminderDay0, startedAt},
		{ReminderMonth1, AddSantiagoCalendarMonths(startedAt.In(santiagoLocation), 1).UTC()},
		{ReminderDay15, localCalendarDays(dueAt, -15)},
		{ReminderDay7, localCalendarDays(dueAt, -7)},
		{ReminderDay1, localCalendarDays(dueAt, -1)},
		{ReminderDeadline, dueAt},
	}
	latest := -1
	for index := last + 1; index < len(schedule); index++ {
		if !schedule[index].AvailableAt.After(now) {
			latest = index
		}
	}
	if latest < 0 {
		return ScheduledReminder{}, false, nil
	}
	return schedule[latest], true, nil
}

type ComplianceReminderEmail struct {
	ID, ProfileType, ProfileID, RecipientID, RecipientEmail string
	Stage                                                   ReminderStage
	DueAt                                                   time.Time
	Attempt                                                 int
}

type ReminderRepository interface {
	EnqueueDueReminders(context.Context, time.Time, int) (int, error)
	ClaimReminderEmail(context.Context, time.Time) (ComplianceReminderEmail, bool, error)
	FinishReminderEmail(context.Context, ComplianceReminderEmail, bool, string, time.Time) error
}

func (s *Service) EnqueueDueReminders(ctx context.Context, now time.Time, limit int) (int, error) {
	repo, ok := s.repository.(ReminderRepository)
	if !ok || ctx == nil || now.IsZero() || limit < 1 || limit > 500 {
		return 0, ErrInvalid
	}
	return repo.EnqueueDueReminders(ctx, now.UTC(), limit)
}

func (s *Service) ClaimReminderEmail(ctx context.Context, now time.Time) (ComplianceReminderEmail, bool, error) {
	repo, ok := s.repository.(ReminderRepository)
	if !ok || ctx == nil || now.IsZero() {
		return ComplianceReminderEmail{}, false, ErrInvalid
	}
	return repo.ClaimReminderEmail(ctx, now.UTC())
}

func (s *Service) FinishReminderEmail(ctx context.Context, notice ComplianceReminderEmail, delivered bool, code string, at time.Time) error {
	repo, ok := s.repository.(ReminderRepository)
	if !ok || ctx == nil || notice.ID == "" || notice.Attempt < 1 || at.IsZero() {
		return ErrInvalid
	}
	return repo.FinishReminderEmail(ctx, notice, delivered, strings.TrimSpace(code), at.UTC())
}

type GraceMaintenanceResult struct {
	RemindersEnqueued int
	ProfilesExpired   int
}

func (s *Service) RunGraceMaintenance(ctx context.Context, now time.Time, limit int) (GraceMaintenanceResult, error) {
	reminders, enqueueErr := s.EnqueueDueReminders(ctx, now, limit)
	expired, expireErr := s.ExpireNoncompliant(ctx, now)
	return GraceMaintenanceResult{RemindersEnqueued: reminders, ProfilesExpired: expired}, errors.Join(enqueueErr, expireErr)
}

func localCalendarDays(value time.Time, days int) time.Time {
	local := value.In(santiagoLocation)
	return time.Date(local.Year(), local.Month(), local.Day()+days, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), santiagoLocation).UTC()
}
