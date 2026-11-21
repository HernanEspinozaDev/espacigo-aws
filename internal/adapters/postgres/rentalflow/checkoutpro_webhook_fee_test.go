package rentalflowpg

import "testing"

func TestApplicationFeeMatchesSupportsZeroFeeAndPreservesLegacyFee(t *testing.T) {
	fee := int64(428)
	zero := int64(0)
	cases := []struct {
		name     string
		expected int64
		observed *int64
		want     bool
	}{
		{name: "zero fee omitted", expected: 0, observed: nil, want: true},
		{name: "zero fee explicit", expected: 0, observed: &zero, want: true},
		{name: "legacy fee exact", expected: 428, observed: &fee, want: true},
		{name: "legacy fee absent", expected: 428, observed: nil, want: false},
		{name: "legacy fee changed", expected: 428, observed: &zero, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applicationFeeMatches(tc.expected, tc.observed); got != tc.want {
				t.Fatalf("applicationFeeMatches(%d, %v) = %t, want %t", tc.expected, tc.observed, got, tc.want)
			}
		})
	}
}
