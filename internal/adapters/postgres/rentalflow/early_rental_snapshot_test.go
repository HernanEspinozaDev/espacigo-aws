package rentalflowpg

import (
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

func TestLocalIntervalDateRangeUsesHalfOpenLocalDates(t *testing.T) {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		start     time.Time
		end       time.Time
		firstDate string
		lastDate  string
	}{
		{
			name:      "ending at local midnight excludes the following date",
			start:     time.Date(2030, time.April, 1, 23, 0, 0, 0, loc),
			end:       time.Date(2030, time.April, 2, 0, 0, 0, 0, loc),
			firstDate: "2030-04-01",
			lastDate:  "2030-04-01",
		},
		{
			name:      "interval crossing local midnight includes both dates",
			start:     time.Date(2030, time.April, 1, 23, 0, 0, 0, loc),
			end:       time.Date(2030, time.April, 2, 1, 0, 0, 0, loc),
			firstDate: "2030-04-01",
			lastDate:  "2030-04-02",
		},
		{
			name:      "UTC timestamps are mapped to listing local dates",
			start:     time.Date(2030, time.April, 2, 2, 0, 0, 0, time.UTC),
			end:       time.Date(2030, time.April, 2, 3, 0, 0, 0, time.UTC),
			firstDate: "2030-04-01",
			lastDate:  "2030-04-01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, last, err := localIntervalDateRange(rentalflow.Interval{StartAt: test.start, EndAt: test.end}, loc)
			if err != nil {
				t.Fatal(err)
			}
			if first != test.firstDate || last != test.lastDate {
				t.Fatalf("date range = [%s,%s], want [%s,%s]", first, last, test.firstDate, test.lastDate)
			}
		})
	}
}
