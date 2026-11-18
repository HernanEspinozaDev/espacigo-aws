package rentalflowpg

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresEarlyRentalListOffers(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	dsn := dbURL.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	outsider := uuid.New()
	insertEarlyReadActor(t, ctx, dsn, outsider, "outsider")

	// Three offers share one participant pair and timestamp but each owns a
	// distinct space/offer/occupancy identity. UUID ordering is deterministic.
	baseID := uuid.MustParse(ids.offer)
	offerIDs := []uuid.UUID{baseID, uuid.New(), uuid.New()}
	offerIDs[1], offerIDs[2] = offerIDs[2], offerIDs[1]
	timeAt := time.Date(2030, 8, 9, 10, 11, 12, 123456000, time.UTC)
	records := make(map[uuid.UUID]earlyRentalInitialRecord, len(offerIDs))
	for _, offerID := range offerIDs {
		fixtureIDs := ids
		fixtureIDs.offer = offerID.String()
		fixtureIDs.space = uuid.NewString()
		fixtureIDs.occupancy = uuid.NewString()
		seedEarlyReadSpace(t, ctx, dsn, fixtureIDs)
		record := earlyInsertRecord(t, fixtureIDs, timeAt)
		commitEarlyReadFixtureOffer(t, ctx, dsn, record)
		records[offerID] = record
	}
	before := earlyOfferReadCounts(t, ctx, dsn)

	t.Run("BothParticipants", func(t *testing.T) {
		for _, actor := range []uuid.UUID{uuid.MustParse(ids.renter), uuid.MustParse(ids.host)} {
			result := listInEarlyReadTx(t, ctx, dsn, EarlyRentalOfferListInput{ActorID: actor, Limit: 10})
			if len(result.Rows) != len(offerIDs) || result.Next != nil {
				t.Fatalf("actor %s got %d rows, next=%v", actor, len(result.Rows), result.Next)
			}
		}
	})

	t.Run("OutsiderHidden", func(t *testing.T) { assertEarlyListEmpty(t, ctx, dsn, outsider) })

	t.Run("RoleFiltering", func(t *testing.T) {
		for _, tc := range []struct{ role, participant string }{{"renter", ids.renter}, {"host", ids.host}} {
			result := listInEarlyReadTx(t, ctx, dsn, EarlyRentalOfferListInput{ActorID: uuid.MustParse(tc.participant), Role: tc.role, Limit: 10})
			if len(result.Rows) != len(offerIDs) {
				t.Fatalf("role %s rows=%d", tc.role, len(result.Rows))
			}
		}
		got := listInEarlyReadTx(t, ctx, dsn, EarlyRentalOfferListInput{ActorID: uuid.MustParse(ids.renter), Role: "host", Limit: 10})
		if len(got.Rows) != 0 || got.Next != nil {
			t.Fatalf("renter requesting host role received rows: %+v", got)
		}
	})

	ordered := append([]uuid.UUID(nil), offerIDs...)
	// Equal timestamp tie-break is descending UUID, independent of insert order.
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() > ordered[j].String() })
	t.Run("StableKeysetPages", func(t *testing.T) {
		first := listInEarlyReadTx(t, ctx, dsn, EarlyRentalOfferListInput{ActorID: uuid.MustParse(ids.renter), Limit: 2})
		if len(first.Rows) != 2 || first.Next == nil || first.Next.ID != ordered[1] || !first.Next.CreatedAt.Equal(timeAt) {
			t.Fatalf("first page/cursor mismatch: %+v", first)
		}
		assertListIDs(t, first.Rows, ordered[:2])
		for _, row := range first.Rows {
			want := records[uuid.MustParse(row.Offer.ID)]
			if !earlyOfferValuesEqual(row.Offer, want.Offer) {
				t.Fatalf("offer %s snapshot differs from persisted initial record", row.Offer.ID)
			}
		}
		second := listInEarlyReadTx(t, ctx, dsn, EarlyRentalOfferListInput{ActorID: uuid.MustParse(ids.renter), Limit: 2, After: first.Next})
		if len(second.Rows) != 1 || second.Next != nil {
			t.Fatalf("second page mismatch: %+v", second)
		}
		assertListIDs(t, second.Rows, ordered[2:])
	})
	t.Run("ReadOnlyNoWrites", func(t *testing.T) {
		if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
			t.Fatalf("listing changed counts: %v -> %v", before, after)
		}
	})
}

func seedEarlyReadSpace(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs) {
	t.Helper()
	c := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := c.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado,zona_horaria) VALUES($1,$2,'oficina','Synthetic rental',repeat('Synthetic fixture description. ',4),20,2,'Synthetic rules','hora',6000,'Synthetic address','activa','UTC')`, ids.space, ids.host); err != nil {
		t.Fatal(err)
	}
}

func listInEarlyReadTx(t *testing.T, ctx context.Context, dsn string, in EarlyRentalOfferListInput) EarlyRentalOfferListResult {
	t.Helper()
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := listEarlyRentalOffers(ctx, tx, in)
	if err != nil {
		t.Fatalf("list offers: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertEarlyListEmpty(t *testing.T, ctx context.Context, dsn string, actor uuid.UUID) {
	t.Helper()
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := listEarlyRentalOffers(ctx, tx, EarlyRentalOfferListInput{ActorID: actor, Limit: 10})
	if err != nil || len(got.Rows) != 0 || got.Next != nil {
		t.Fatalf("outsider list result=%+v err=%v", got, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertListIDs(t *testing.T, rows []EarlyRentalOfferListRow, want []uuid.UUID) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("rows=%d want %d", len(rows), len(want))
	}
	for i := range want {
		got, err := uuid.Parse(rows[i].Offer.ID)
		if err != nil || got != want[i] {
			t.Fatalf("row %d id=%s want %s err=%v", i, rows[i].Offer.ID, want[i], err)
		}
	}
}
