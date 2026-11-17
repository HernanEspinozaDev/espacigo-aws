package earlyrentalhttp

import (
	"errors"
	"net/url"
	"strconv"
	"unicode/utf8"
)

const (
	defaultEarlyRentalListLimit = 25
	maxEarlyRentalListLimit     = 100
	maxEarlyRentalCursorRunes   = 2048
)

var ErrInvalidEarlyRentalListQuery = errors.New("invalid early-rental list query")

type EarlyRentalListQuery struct {
	Role   *string
	Cursor *string
	Limit  int
}

// DecodeEarlyRentalListQuery validates the OpenAPI list filters. An omitted role
// means both participant perspectives and remains nil; an omitted cursor stays nil.
func DecodeEarlyRentalListQuery(values url.Values) (EarlyRentalListQuery, error) {
	query := EarlyRentalListQuery{Limit: defaultEarlyRentalListLimit}
	for key := range values {
		if key != "role" && key != "cursor" && key != "limit" {
			return EarlyRentalListQuery{}, ErrInvalidEarlyRentalListQuery
		}
	}

	if raw, exists := values["role"]; exists {
		if len(raw) != 1 || (raw[0] != earlyRentalRoleRenter && raw[0] != earlyRentalRoleHost) {
			return EarlyRentalListQuery{}, ErrInvalidEarlyRentalListQuery
		}
		role := raw[0]
		query.Role = &role
	}
	cursor, err := decodeEarlyRentalCursor(values, ErrInvalidEarlyRentalListQuery)
	if err != nil {
		return EarlyRentalListQuery{}, err
	}
	query.Cursor = cursor
	limit, err := decodeEarlyRentalLimit(values, ErrInvalidEarlyRentalListQuery)
	if err != nil {
		return EarlyRentalListQuery{}, err
	}
	query.Limit = limit
	return query, nil
}

func (q EarlyRentalListQuery) CursorBinding(actorID string) EarlyRentalCursorBinding {
	role := ""
	if q.Role != nil {
		role = *q.Role
	}
	return EarlyRentalCursorBinding{
		Scope: EarlyRentalCursorScopeOffers, ActorID: actorID, Role: role, Limit: q.Limit,
	}
}

func decodeEarlyRentalCursor(values url.Values, invalid error) (*string, error) {
	raw, exists := values["cursor"]
	if !exists {
		return nil, nil
	}
	if len(raw) != 1 || !utf8.ValidString(raw[0]) {
		return nil, invalid
	}
	length := utf8.RuneCountInString(raw[0])
	if length < 1 || length > maxEarlyRentalCursorRunes {
		return nil, invalid
	}
	cursor := raw[0]
	return &cursor, nil
}

func decodeEarlyRentalLimit(values url.Values, invalid error) (int, error) {
	raw, exists := values["limit"]
	if !exists {
		return defaultEarlyRentalListLimit, nil
	}
	if len(raw) != 1 {
		return 0, invalid
	}
	limit, err := strconv.ParseUint(raw[0], 10, 8)
	if err != nil || limit < 1 || limit > maxEarlyRentalListLimit {
		return 0, invalid
	}
	return int(limit), nil
}
