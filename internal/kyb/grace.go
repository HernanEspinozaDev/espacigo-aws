package kyb

import "time"

const DefaultGraceMonths = 2

var santiagoLocation = func() *time.Location {
	location, err := time.LoadLocation("America/Santiago")
	if err != nil {
		panic("America/Santiago timezone data is required")
	}
	return location
}()

// GraceDueAt returns the local-calendar deadline for a commercial profile.
// The returned instant accounts for the Santiago UTC offset in effect at the
// deadline (which can differ from the offset at activation).
func GraceDueAt(start time.Time, months int) (time.Time, error) {
	if start.IsZero() || months < 1 || months > 24 {
		return time.Time{}, ErrInvalid
	}
	return AddSantiagoCalendarMonths(start.In(santiagoLocation), months).UTC(), nil
}

// AddSantiagoCalendarMonths matches PostgreSQL interval month arithmetic by
// clamping dates such as January 31 to the last day of the target month.
func AddSantiagoCalendarMonths(local time.Time, months int) time.Time {
	local = local.In(santiagoLocation)
	first := time.Date(local.Year(), local.Month()+time.Month(months), 1, 0, 0, 0, 0, santiagoLocation)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, santiagoLocation).Day()
	day := local.Day()
	if day > lastDay { day = lastDay }
	return time.Date(first.Year(), first.Month(), day, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), santiagoLocation)
}

// GraceExpired reports whether the deadline has passed. An exactly-on-time
// request remains eligible until its deadline instant.
func GraceExpired(dueAt, now time.Time) (bool, error) {
	if dueAt.IsZero() || now.IsZero() {
		return false, ErrInvalid
	}
	return !now.Before(dueAt), nil
}
