package earlyrentalhttp

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrMissingIfMatch = errors.New("missing early-rental If-Match precondition")
	ErrInvalidIfMatch = errors.New("invalid early-rental If-Match precondition")
	ErrStaleIfMatch   = errors.New("stale early-rental If-Match precondition")
)

// FormatEarlyRentalETag creates a strong resource validator bound to the
// canonical offer ID and its positive revision.
func FormatEarlyRentalETag(offerID string, revision uint64) (string, error) {
	if !validEarlyRentalUUID(offerID) || revision == 0 {
		return "", ErrInvalidIfMatch
	}
	payload := offerID + "\x00" + strconv.FormatUint(revision, 10)
	quote := string([]byte{34})
	return quote + base64.RawURLEncoding.EncodeToString([]byte(payload)) + quote, nil
}

// ParseIfMatchRevision accepts exactly one canonical strong ETag. A tag from a
// different offer is stale; malformed, weak, wildcard, or list forms are invalid.
func ParseIfMatchRevision(values []string, expectedOfferID string) (uint64, error) {
	if len(values) == 0 {
		return 0, ErrMissingIfMatch
	}
	if len(values) != 1 || !validEarlyRentalUUID(expectedOfferID) {
		return 0, ErrInvalidIfMatch
	}
	tag := values[0]
	if len(tag) < 3 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return 0, ErrInvalidIfMatch
	}
	encoded := tag[1 : len(tag)-1]
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(payload) != encoded {
		return 0, ErrInvalidIfMatch
	}
	offerID, revisionText, ok := strings.Cut(string(payload), "\x00")
	if !ok || strings.Contains(revisionText, "\x00") || !validEarlyRentalUUID(offerID) {
		return 0, ErrInvalidIfMatch
	}
	revision, err := strconv.ParseUint(revisionText, 10, 64)
	if err != nil || revision == 0 || strconv.FormatUint(revision, 10) != revisionText {
		return 0, ErrInvalidIfMatch
	}
	if offerID != expectedOfferID {
		return 0, ErrStaleIfMatch
	}
	return revision, nil
}

func validEarlyRentalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
