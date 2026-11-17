package earlyrentalhttp

import (
	"errors"
	"math"
	"net/url"
	"strconv"
)

var ErrInvalidEventsAfter = errors.New("invalid early-rental events cursor")

// DecodeEventsAfter parses the only supported event-history query parameter.
// An omitted cursor means sequence zero; the wire contract is a nonnegative int64.
func DecodeEventsAfter(values url.Values) (uint64, error) {
	for key := range values {
		if key != "after" {
			return 0, ErrInvalidEventsAfter
		}
	}
	raw, exists := values["after"]
	if !exists {
		return 0, nil
	}
	if len(raw) != 1 {
		return 0, ErrInvalidEventsAfter
	}
	after, err := strconv.ParseUint(raw[0], 10, 63)
	if err != nil || after > math.MaxInt64 {
		return 0, ErrInvalidEventsAfter
	}
	return after, nil
}
