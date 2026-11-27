package kyb

import (
	"testing"
	"time"
)

func TestGraceDueAtUsesSantiagoCalendarMonthsAcrossDST(t *testing.T) {
	start := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	due, err := GraceDueAt(start, DefaultGraceMonths)
	if err != nil {
		t.Fatal(err)
	}
	local := due.In(santiagoLocation)
	if local.Year() != 2026 || local.Month() != time.March || local.Day() != 15 || local.Hour() != 9 {
		t.Fatalf("unexpected Santiago calendar deadline: %s", local)
	}
}

func TestGraceExpiredIsInclusiveAndRejectsZeroTimes(t *testing.T) {
	due := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	expired, err := GraceExpired(due, due.Add(-time.Nanosecond))
	if err != nil || expired {
		t.Fatalf("before deadline: expired=%v err=%v", expired, err)
	}
	expired, err = GraceExpired(due, due)
	if err != nil || !expired {
		t.Fatalf("at deadline: expired=%v err=%v", expired, err)
	}
	if _, err = GraceExpired(time.Time{}, due); err == nil {
		t.Fatal("zero deadline should be invalid")
	}
}

func TestNextDueReminderUsesCalendarBoundariesAndLatestCatchUp(t *testing.T) {
	loc := santiagoLocation
	start := time.Date(2026, time.December, 31, 10, 0, 0, 0, loc)
	due, err := GraceDueAt(start, 2)
	if err != nil {
		t.Fatal(err)
	}
	month, ok, err := NextDueReminder(start, due, string(ReminderDay0), time.Date(2027, time.January, 31, 10, 0, 0, 0, loc))
	if err != nil || !ok || month.Stage != ReminderMonth1 {
		t.Fatalf("expected Santiago calendar month notice, got %#v, %v, %v", month, ok, err)
	}
	catchup, ok, err := NextDueReminder(start, due, string(ReminderDay0), due.Add(time.Hour))
	if err != nil || !ok || catchup.Stage != ReminderDeadline {
		t.Fatalf("catch-up should select only the due notice, got %#v, %v, %v", catchup, ok, err)
	}
	if _, ok, err = NextDueReminder(start, due, string(ReminderDeadline), due.Add(time.Hour)); err != nil || ok {
		t.Fatalf("due stage should not repeat, ok=%v err=%v", ok, err)
	}
}

func TestAddSantiagoCalendarMonthsClampsMonthEnd(t *testing.T) {
	for _, day := range []int{30, 31} {
		start := time.Date(2027, time.January, day, 12, 0, 0, 0, santiagoLocation)
		got := AddSantiagoCalendarMonths(start, 1)
		if got.Year() != 2027 || got.Month() != time.February || got.Day() != 28 {
			t.Fatalf("Jan %d should clamp to Feb 28, got %s", day, got)
		}
	}
}
