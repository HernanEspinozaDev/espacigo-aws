package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type offerLockTx struct {
	pgx.Tx
	ids []string
}

func (f *offerLockTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query != `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))` {
		return pgconn.CommandTag{}, nil
	}
	f.ids = append(f.ids, args[0].(string))
	return pgconn.CommandTag{}, nil
}

func TestEarlyRentalOfferAccountLocksAreGloballySortedAndUnique(t *testing.T) {
	actor := uuid.MustParse("ffffffff-ffff-4fff-8fff-ffffffffffff")
	host := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	renter := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	tx := &offerLockTx{}
	participants := [][2]uuid.UUID{{host, renter}, {renter, actor}}
	if err := lockEarlyRentalOfferAccounts(context.Background(), tx, actor, participants); err != nil {
		t.Fatal(err)
	}
	want := []string{host.String(), renter.String(), actor.String()}
	if !reflect.DeepEqual(tx.ids, want) {
		t.Fatalf("lock IDs = %v, want globally sorted unique %v", tx.ids, want)
	}
}

const earlyRentalListRetrySQLOracle = `SELECT o.id,o.creada_en,o.anfitrion_id,o.arrendatario_id
FROM public.arriendo_temprano o
JOIN public.usuario u ON u.id=$1 AND u.estado='activo'
WHERE (o.anfitrion_id=$1 OR o.arrendatario_id=$1)
  AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$1)
  AND ($2='' OR ($2='host' AND o.anfitrion_id=$1) OR ($2='renter' AND o.arrendatario_id=$1))
  AND ($3::timestamptz IS NULL OR (o.creada_en,o.id)<($3,$4::uuid))
ORDER BY o.creada_en DESC,o.id DESC LIMIT $5`

type earlyRentalListRetryTx struct {
	pgx.Tx
	pages [][]earlyRentalListRetryRow
	locks []string
	query int
}

type earlyRentalListRetryRow struct {
	id, host, renter uuid.UUID
	createdAt        time.Time
}

func (tx *earlyRentalListRetryTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	if query != earlyRentalListRetrySQLOracle || len(args) != 5 {
		return nil, errors.New("unexpected offer list query")
	}
	page := tx.pages[tx.query]
	tx.query++
	return &earlyRentalListRetryRows{items: page}, nil
}

func (tx *earlyRentalListRetryTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query != `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))` || len(args) != 1 {
		return pgconn.CommandTag{}, errors.New("unexpected account lock query")
	}
	tx.locks = append(tx.locks, args[0].(string))
	return pgconn.NewCommandTag("SELECT 1"), nil
}

type earlyRentalListRetryRows struct {
	items []earlyRentalListRetryRow
	index int
}

func (rows *earlyRentalListRetryRows) Close()                                       {}
func (rows *earlyRentalListRetryRows) Err() error                                   { return nil }
func (rows *earlyRentalListRetryRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (rows *earlyRentalListRetryRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (rows *earlyRentalListRetryRows) Next() bool {
	if rows.index >= len(rows.items) {
		return false
	}
	rows.index++
	return true
}
func (rows *earlyRentalListRetryRows) Scan(dest ...any) error {
	item := rows.items[rows.index-1]
	values := []any{item.id, item.createdAt, item.host, item.renter}
	if len(dest) != len(values) {
		return errors.New("unexpected offer list scan shape")
	}
	for i := range dest {
		switch target := dest[i].(type) {
		case *uuid.UUID:
			*target = values[i].(uuid.UUID)
		case *time.Time:
			*target = values[i].(time.Time)
		default:
			return errors.New("unexpected offer list scan target")
		}
	}
	return nil
}
func (rows *earlyRentalListRetryRows) Values() ([]any, error) { return nil, errors.New("unused") }
func (rows *earlyRentalListRetryRows) RawValues() [][]byte    { return nil }
func (rows *earlyRentalListRetryRows) Conn() *pgx.Conn        { return nil }
func (rows *earlyRentalListRetryRows) TypeMap() *pgtype.Map   { return pgtype.NewMap() }

func TestListEarlyRentalOffersReportsRetryWhenPageChangesAfterLocks(t *testing.T) {
	actor := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	host := uuid.MustParse("20000000-0000-4000-8000-000000000002")
	created := testListTime()
	initial := earlyRentalListRetryRow{id: uuid.MustParse("30000000-0000-4000-8000-000000000003"), createdAt: created, host: host, renter: actor}
	changed := initial
	changed.id = uuid.MustParse("40000000-0000-4000-8000-000000000004")
	tx := &earlyRentalListRetryTx{pages: [][]earlyRentalListRetryRow{{initial}, {changed}}}
	_, err := listEarlyRentalOffers(context.Background(), tx, EarlyRentalOfferListInput{ActorID: actor, Limit: 10})
	if !errors.Is(err, ErrEarlyRentalOfferListRetry) {
		t.Fatalf("changed page error=%v, want ErrEarlyRentalOfferListRetry", err)
	}
	if tx.query != 2 {
		t.Fatalf("page queries=%d, want initial discovery and post-lock recheck", tx.query)
	}
}

func TestListEarlyRentalOffersRejectsInvalidInputsBeforeQuery(t *testing.T) {
	validActor := uuid.New()
	for _, tc := range []struct {
		name string
		in   EarlyRentalOfferListInput
	}{
		{"nil actor", EarlyRentalOfferListInput{Limit: 10}},
		{"zero limit", EarlyRentalOfferListInput{ActorID: validActor, Limit: 0}},
		{"over limit", EarlyRentalOfferListInput{ActorID: validActor, Limit: 101}},
		{"unknown role", EarlyRentalOfferListInput{ActorID: validActor, Role: "other", Limit: 10}},
		{"nil cursor id", EarlyRentalOfferListInput{ActorID: validActor, Limit: 10, After: &EarlyRentalOfferListPosition{CreatedAt: testListTime()}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := listEarlyRentalOffers(context.Background(), nil, tc.in); err == nil {
				t.Fatal("expected invalid input error")
			}
		})
	}
}

func testListTime() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}
