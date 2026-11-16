package spacespg

import (
	"testing"
	"time"
)

func TestHourlyDayStatus(t *testing.T) {
	base := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return base.Add(time.Duration(h) * time.Hour) }
	tests := []struct {
		name       string
		start, end time.Time
		occ        []occItem
		want       string
	}{
		{"two reservations leave real gaps", at(10), at(18), []occItem{{at(10), at(12)}, {at(14), at(16)}}, "PARTIAL"},
		{"adjacent intervals cover day", at(10), at(18), []occItem{{at(14), at(18)}, {at(10), at(14)}}, "BOOKED"},
		{"overlap unordered cover day", at(10), at(18), []occItem{{at(13), at(18)}, {at(10), at(14)}}, "BOOKED"},
		{"outside operating range", at(10), at(18), []occItem{{at(8), at(10)}, {at(18), at(19)}}, "AVAILABLE"},
		{"half open exact boundaries", at(10), at(18), []occItem{{at(8), at(10)}, {at(18), at(20)}}, "AVAILABLE"},
		{"close at midnight", at(0), at(24), []occItem{{at(0), at(12)}, {at(12), at(24)}}, "BOOKED"},
		{"midnight trailing gap", at(0), at(24), []occItem{{at(0), at(23)}}, "PARTIAL"},
		{"no occupations", at(10), at(18), nil, "AVAILABLE"},
		{"leading gap", at(10), at(18), []occItem{{at(12), at(18)}}, "PARTIAL"},
		{"clips enclosing occupation", at(10), at(18), []occItem{{at(0), at(24)}}, "BOOKED"},
		{"empty operating range fails closed", at(10), at(10), nil, "BLOCKED"},
		{"reversed operating range fails closed", at(18), at(10), nil, "BLOCKED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hourlyDayStatus(tt.start, tt.end, tt.occ); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
