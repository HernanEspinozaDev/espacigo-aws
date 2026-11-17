package earlyrentalhttp

import (
	"math"
	"net/url"
	"strconv"
	"testing"
)

func TestDecodeEventsAfterDefaultsToZeroAndAcceptsInt64Range(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
		want   uint64
	}{
		{name: "omitted", values: url.Values{}, want: 0},
		{name: "zero", values: url.Values{"after": {"0"}}, want: 0},
		{name: "positive", values: url.Values{"after": {"42"}}, want: 42},
		{name: "maximum int64", values: url.Values{"after": {strconv.FormatInt(math.MaxInt64, 10)}}, want: uint64(math.MaxInt64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeEventsAfter(tc.values)
			if err != nil || got != tc.want {
				t.Fatalf("DecodeEventsAfter() = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestDecodeEventsAfterRejectsMalformedDuplicateAndUnknownParameters(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
	}{
		{name: "empty value", values: url.Values{"after": {""}}},
		{name: "negative", values: url.Values{"after": {"-1"}}},
		{name: "non integer", values: url.Values{"after": {"1.5"}}},
		{name: "overflow", values: url.Values{"after": {"9223372036854775808"}}},
		{name: "duplicate", values: url.Values{"after": {"1", "1"}}},
		{name: "unknown parameter", values: url.Values{"after": {"1"}, "limit": {"5"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := DecodeEventsAfter(tc.values); err == nil || got != 0 {
				t.Fatalf("DecodeEventsAfter() = %d, %v; want zero and error", got, err)
			}
		})
	}
}
