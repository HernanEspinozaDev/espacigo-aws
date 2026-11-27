package privacy

import "time"

// SantiagoLocation returns the Chile continental timezone used to anchor
// operational privacy deadlines.
func SantiagoLocation() *time.Location {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		return time.FixedZone("America/Santiago", -3*3600)
	}
	return loc
}

func endOfSantiagoDayUTC(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc).UTC()
}

func isChileanFixedHoliday(t time.Time) bool {
	m, d := t.Month(), t.Day()
	return (m == time.January && d == 1) ||
		(m == time.May && d == 1) ||
		(m == time.May && d == 21) ||
		(m == time.September && d == 18) ||
		(m == time.September && d == 19) ||
		(m == time.December && d == 25)
}

func isChileanBusinessDay(t time.Time) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !isChileanFixedHoliday(t)
}

// ComputeInitialDeadline calculates the operational response deadline in
// America/Santiago: 2 business days for temporal blocking ("bloqueo"), or
// 30 calendar days for the remaining ARCO+ rights.
func ComputeInitialDeadline(submittedAt time.Time, rightType string) time.Time {
	loc := SantiagoLocation()
	local := submittedAt.In(loc)
	if rightType == "bloqueo" {
		cursor := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, loc)
		added := 0
		for added < 2 {
			cursor = cursor.AddDate(0, 0, 1)
			if isChileanBusinessDay(cursor) {
				added++
			}
		}
		return endOfSantiagoDayUTC(cursor, loc)
	}
	target := local.AddDate(0, 0, 30)
	return endOfSantiagoDayUTC(target, loc)
}

// ComputeExtensionDeadline calculates a single 1..30 calendar-day extension
// anchored to the original due date in America/Santiago.
func ComputeExtensionDeadline(baseDueAt time.Time, extraDays int) (time.Time, error) {
	if baseDueAt.IsZero() || extraDays < 1 || extraDays > 30 {
		return time.Time{}, ErrInvalid
	}
	loc := SantiagoLocation()
	local := baseDueAt.In(loc)
	target := local.AddDate(0, 0, extraDays)
	return endOfSantiagoDayUTC(target, loc), nil
}

// EffectiveDeadline returns extendedDueAt when set, otherwise dueAt.
func EffectiveDeadline(dueAt, extendedDueAt *time.Time) *time.Time {
	if extendedDueAt != nil {
		return extendedDueAt
	}
	return dueAt
}
