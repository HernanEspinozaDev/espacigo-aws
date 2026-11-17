package earlyrentalhttp

import (
	"errors"
	"net/url"
)

var ErrInvalidEarlyRentalTransactionListQuery = errors.New("invalid early-rental transaction list query")

type EarlyRentalTransactionListQuery struct {
	Role   *string
	State  *string
	Cursor *string
	Limit  int
}

// DecodeEarlyRentalTransactionListQuery validates the exact list filters. An
// omitted role means both participant perspectives and remains nil.
func DecodeEarlyRentalTransactionListQuery(values url.Values) (EarlyRentalTransactionListQuery, error) {
	query := EarlyRentalTransactionListQuery{Limit: defaultEarlyRentalListLimit}
	for key := range values {
		if key != "role" && key != "state" && key != "cursor" && key != "limit" {
			return EarlyRentalTransactionListQuery{}, ErrInvalidEarlyRentalTransactionListQuery
		}
	}
	if raw, exists := values["role"]; exists {
		if len(raw) != 1 || (raw[0] != earlyRentalRoleRenter && raw[0] != earlyRentalRoleHost) {
			return EarlyRentalTransactionListQuery{}, ErrInvalidEarlyRentalTransactionListQuery
		}
		role := raw[0]
		query.Role = &role
	}
	if raw, exists := values["state"]; exists {
		if len(raw) != 1 || !validEarlyRentalTransactionState(raw[0]) {
			return EarlyRentalTransactionListQuery{}, ErrInvalidEarlyRentalTransactionListQuery
		}
		state := raw[0]
		query.State = &state
	}
	cursor, err := decodeEarlyRentalCursor(values, ErrInvalidEarlyRentalTransactionListQuery)
	if err != nil {
		return EarlyRentalTransactionListQuery{}, err
	}
	query.Cursor = cursor
	limit, err := decodeEarlyRentalLimit(values, ErrInvalidEarlyRentalTransactionListQuery)
	if err != nil {
		return EarlyRentalTransactionListQuery{}, err
	}
	query.Limit = limit
	return query, nil
}

func (q EarlyRentalTransactionListQuery) CursorBinding(actorID string) EarlyRentalCursorBinding {
	role, state := "", ""
	if q.Role != nil {
		role = *q.Role
	}
	if q.State != nil {
		state = *q.State
	}
	return EarlyRentalCursorBinding{
		Scope: EarlyRentalCursorScopeTransactions, ActorID: actorID, Role: role, State: state, Limit: q.Limit,
	}
}

func validEarlyRentalTransactionState(state string) bool {
	switch state {
	case "enviado", "recibido", "anulado", "vencido":
		return true
	default:
		return false
	}
}
