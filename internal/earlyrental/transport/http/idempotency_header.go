package earlyrentalhttp

import (
	"errors"
	"net/http"
	"unicode/utf8"
)

var ErrMalformedIdempotencyKey = errors.New("malformed early-rental idempotency key")

const (
	minIdempotencyKeyLength = 8
	maxIdempotencyKeyLength = 200
)

// DecodeIdempotencyKey requires exactly one opaque key and preserves its value.
// It validates the OpenAPI character-length bounds but does not provide replay
// protection; that requires a transactional idempotency record.
func DecodeIdempotencyKey(header http.Header) (string, error) {
	values := header.Values("Idempotency-Key")
	if len(values) != 1 || !utf8.ValidString(values[0]) {
		return "", ErrMalformedIdempotencyKey
	}
	length := utf8.RuneCountInString(values[0])
	if length < minIdempotencyKeyLength || length > maxIdempotencyKeyLength {
		return "", ErrMalformedIdempotencyKey
	}
	return values[0], nil
}
