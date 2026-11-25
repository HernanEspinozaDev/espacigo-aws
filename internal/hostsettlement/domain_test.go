package hostsettlement

import (
	"testing"
	"time"
)

func TestRecordedSettlementUsesPersistedFeesAndPreservesUnknowns(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		base, commission, vat, fee int64
		policy, after              string
	}{
		{"standard", 100000, 3000, 570, 3570, "three_percent_plus_vat", "96430"},
		{"half-up rounding", 12000, 360, 68, 428, "three_percent_plus_vat", "11572"},
		{"test waiver", 12000, 0, 0, 0, "no_commission_applied", "12000"},
		{"recorded custom", 12000, 100, 19, 119, "recorded_fees", "11881"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, policy, rate, vatRate, err := RecordedAmounts(tc.base, tc.base, tc.commission, tc.vat, tc.fee)
			if err != nil || policy != tc.policy || a.AfterMarketplaceFeeCLP != tc.after {
				t.Fatalf("recorded projection %+v %s %v", a, policy, err)
			}
			if a.ProcessorFeeCLP != nil || a.AdjustmentsCLP != nil || a.ReconciledNetCLP != nil {
				t.Fatal("fabricated missing provider expenses/net")
			}
			if (rate != nil) != (policy == "three_percent_plus_vat") || (vatRate != nil) != (policy == "three_percent_plus_vat") {
				t.Fatal("unearned rate label")
			}
		})
	}
	for _, amounts := range [][5]int64{{100, 99, 3, 1, 4}, {100, 100, -1, 0, 0}, {100, 100, 3, 1, 5}, {100, 100, 100, 0, 100}} {
		if _, _, _, _, err := RecordedAmounts(amounts[0], amounts[1], amounts[2], amounts[3], amounts[4]); err == nil {
			t.Fatalf("invalid ledger values accepted: %v", amounts)
		}
	}
}
func TestSettlementMonthUsesChileanDSTAndSeparatesTestMode(t *testing.T) {
	q, err := QueryFor("2026-09", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation("America/Santiago")
	if q.StartAt.In(zone).Day() != 1 || q.StartAt.In(zone).Hour() != 0 || q.EndAt.In(zone).Month() != time.October || q.Mode != "test" {
		t.Fatalf("month boundaries %+v", q)
	}
	_, startOffset := q.StartAt.In(zone).Zone()
	_, endOffset := q.EndAt.In(zone).Zone()
	if startOffset == endOffset {
		t.Fatal("September fixture should cross DST")
	}
	for _, bad := range []string{"2026-9", "2026-13", "2026-09-01", "1999-12"} {
		if _, err := QueryFor(bad, "live", ""); err == nil {
			t.Fatalf("invalid month %s", bad)
		}
	}
	if _, err := QueryFor("2026-09", "all", ""); err == nil {
		t.Fatal("mixed live/test monthly report allowed")
	}
}
