package earlyrentalhttp

import (
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

var errInvalidEarlyRentalListProjection = errors.New("early-rental list cannot be projected")

type earlyRentalSummarySource struct {
	Offer     earlyrental.Offer
	CreatedAt time.Time
}

type earlyRentalListEnvelope struct {
	Data earlyRentalListView `json:"data"`
}

type earlyRentalListView struct {
	Items      []earlyRentalSummaryView `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type earlyRentalSummaryView struct {
	ID         string            `json:"id"`
	ActorRole  string            `json:"actor_role"`
	SpaceID    string            `json:"space_id"`
	SpaceTitle string            `json:"space_title"`
	StartAt    time.Time         `json:"start_at"`
	EndAt      time.Time         `json:"end_at"`
	Subtotal   int64             `json:"subtotal_clp"`
	Currency   string            `json:"currency"`
	State      earlyrental.State `json:"state"`
	Revision   int64             `json:"revision"`
	CreatedAt  time.Time         `json:"created_at"`
	ExpiresAt  time.Time         `json:"expires_at"`
}

// projectEarlyRentalList maps already-authorized source rows to the list wire
// shape. Cursor generation and actor/filter binding belong to the application.
func projectEarlyRentalList(sources []earlyRentalSummarySource, actorID string, nextCursor *string) (earlyRentalListEnvelope, error) {
	if actorID == "" {
		return earlyRentalListEnvelope{}, errInvalidEarlyRentalListProjection
	}
	if nextCursor != nil && (!utf8.ValidString(*nextCursor) || utf8.RuneCountInString(*nextCursor) < 1 || utf8.RuneCountInString(*nextCursor) > maxEarlyRentalCursorRunes) {
		return earlyRentalListEnvelope{}, errInvalidEarlyRentalListProjection
	}

	projected := earlyRentalListEnvelope{Data: earlyRentalListView{Items: make([]earlyRentalSummaryView, 0, len(sources))}}
	if nextCursor != nil {
		cursor := *nextCursor
		projected.Data.NextCursor = &cursor
	}
	for _, source := range sources {
		item, err := projectEarlyRentalSummary(source.Offer, actorID, source.CreatedAt)
		if err != nil {
			return earlyRentalListEnvelope{}, err
		}
		projected.Data.Items = append(projected.Data.Items, item)
	}
	return projected, nil
}

func projectEarlyRentalSummary(offer earlyrental.Offer, actorID string, createdAt time.Time) (earlyRentalSummaryView, error) {
	if earlyrental.ValidateOffer(offer) != nil || createdAt.IsZero() || !createdAt.Before(offer.ExpiresAt) || offer.Revision > uint64(math.MaxInt64) {
		return earlyRentalSummaryView{}, errInvalidEarlyRentalListProjection
	}
	actorRole := ""
	switch actorID {
	case offer.RenterID:
		actorRole = earlyRentalRoleRenter
	case offer.HostID:
		actorRole = earlyRentalRoleHost
	default:
		return earlyRentalSummaryView{}, errInvalidEarlyRentalListProjection
	}
	snapshot := offer.Snapshot
	return earlyRentalSummaryView{
		ID: offer.ID, ActorRole: actorRole, SpaceID: snapshot.SpaceID.String(), SpaceTitle: snapshot.SpaceTitle,
		StartAt: snapshot.StartAt.UTC(), EndAt: snapshot.EndAt.UTC(), Subtotal: snapshot.SubtotalCLP,
		Currency: snapshot.Currency, State: offer.State, Revision: int64(offer.Revision),
		CreatedAt: createdAt.UTC(), ExpiresAt: offer.ExpiresAt.UTC(),
	}, nil
}
