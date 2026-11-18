package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// This literal is the independent query contract for the fake. The fake only
// verifies the SQL sent and orchestration/arguments; PostgreSQL tests must prove
// filtering, authorization, and keyset semantics of the SQL itself.
const earlyRentalTransactionListSQLOracle = `SELECT o.id,o.creada_en,p.pago_id,o.anfitrion_id,o.arrendatario_id
FROM public.arriendo_temprano o
JOIN public.arriendo_temprano_pago p ON p.arriendo_id=o.id
JOIN public.usuario u ON u.id=$1 AND u.estado='activo'
WHERE (o.anfitrion_id=$1 OR o.arrendatario_id=$1)
  AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$1)
  AND p.enviado_en IS NOT NULL
  AND ($2='' OR ($2='host' AND o.anfitrion_id=$1) OR ($2='renter' AND o.arrendatario_id=$1))
  AND ($3='' OR p.estado=$3)
  AND ($4::timestamptz IS NULL OR (o.creada_en,p.pago_id)<($4,$5::uuid))
ORDER BY o.creada_en DESC,p.pago_id DESC LIMIT $6`

type earlyRentalTransactionListCandidate struct {
	offerID, paymentID, hostID, renterID uuid.UUID
	createdAt                            time.Time
}

type earlyRentalTransactionListFakeTx struct {
	pgx.Tx
	pages      [][]earlyRentalTransactionListCandidate
	queryCount int
	lockIDs    []string
}

func (tx *earlyRentalTransactionListFakeTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	if query != earlyRentalTransactionListSQLOracle || len(args) != 6 {
		return nil, errors.New("unexpected transaction-list SQL or argument count")
	}
	if args[0] != uuid.MustParse("10000000-0000-4000-8000-000000000001") || args[1] != "renter" || args[2] != "enviado" || args[3] != nil || args[4] != nil || args[5] != 11 {
		return nil, errors.New("unexpected transaction-list arguments")
	}
	if tx.queryCount >= len(tx.pages) {
		return nil, errors.New("unexpected transaction-list query count")
	}
	page := tx.pages[tx.queryCount]
	tx.queryCount++
	return &earlyRentalTransactionListFakeRows{items: page}, nil
}

func (tx *earlyRentalTransactionListFakeTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query != `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))` || len(args) != 1 {
		return pgconn.CommandTag{}, errors.New("unexpected account-lock SQL")
	}
	tx.lockIDs = append(tx.lockIDs, args[0].(string))
	return pgconn.NewCommandTag("SELECT 1"), nil
}

type earlyRentalTransactionListFakeRows struct {
	items []earlyRentalTransactionListCandidate
	index int
}

func (rows *earlyRentalTransactionListFakeRows) Close()     {}
func (rows *earlyRentalTransactionListFakeRows) Err() error { return nil }
func (rows *earlyRentalTransactionListFakeRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}
func (rows *earlyRentalTransactionListFakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (rows *earlyRentalTransactionListFakeRows) Next() bool {
	if rows.index >= len(rows.items) {
		return false
	}
	rows.index++
	return true
}
func (rows *earlyRentalTransactionListFakeRows) Scan(dest ...any) error {
	item := rows.items[rows.index-1]
	values := []any{item.offerID, item.createdAt, item.paymentID, item.hostID, item.renterID}
	if len(dest) != len(values) {
		return errors.New("unexpected transaction-list scan shape")
	}
	for i := range dest {
		switch target := dest[i].(type) {
		case *uuid.UUID:
			*target = values[i].(uuid.UUID)
		case *time.Time:
			*target = values[i].(time.Time)
		default:
			return errors.New("unexpected transaction-list scan target")
		}
	}
	return nil
}
func (rows *earlyRentalTransactionListFakeRows) Values() ([]any, error) {
	return nil, errors.New("unused")
}
func (rows *earlyRentalTransactionListFakeRows) RawValues() [][]byte  { return nil }
func (rows *earlyRentalTransactionListFakeRows) Conn() *pgx.Conn      { return nil }
func (rows *earlyRentalTransactionListFakeRows) TypeMap() *pgtype.Map { return pgtype.NewMap() }

func TestListEarlyRentalTransactionsRejectsInvalidInputsBeforeQuery(t *testing.T) {
	actor := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	zeroTime := time.Time{}
	nonzeroTime := time.Date(2030, 8, 9, 10, 11, 12, 0, time.UTC)
	cases := []struct {
		name   string
		change func(*EarlyRentalTransactionListInput)
	}{
		{"nil_actor", func(in *EarlyRentalTransactionListInput) { in.ActorID = uuid.Nil }},
		{"invalid_role", func(in *EarlyRentalTransactionListInput) { in.Role = "owner" }},
		{"invalid_state", func(in *EarlyRentalTransactionListInput) { in.State = "pendiente" }},
		{"zero_limit", func(in *EarlyRentalTransactionListInput) { in.Limit = 0 }},
		{"over_limit", func(in *EarlyRentalTransactionListInput) { in.Limit = 101 }},
		{"zero_cursor_time", func(in *EarlyRentalTransactionListInput) {
			in.After = &EarlyRentalTransactionListPosition{CreatedAt: zeroTime, PaymentID: uuid.New()}
		}},
		{"nil_cursor_payment", func(in *EarlyRentalTransactionListInput) {
			in.After = &EarlyRentalTransactionListPosition{CreatedAt: nonzeroTime, PaymentID: uuid.Nil}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := EarlyRentalTransactionListInput{ActorID: actor, Limit: 10}
			tc.change(&in)
			tx := &earlyRentalTransactionListFakeTx{}
			_, err := listEarlyRentalTransactions(context.Background(), tx, in)
			if !errors.Is(err, earlyrental.ErrInvalid) {
				t.Fatalf("error=%v, want earlyrental.ErrInvalid", err)
			}
			if tx.queryCount != 0 {
				t.Fatalf("queries=%d, want no query for invalid input", tx.queryCount)
			}
		})
	}
}

func TestListEarlyRentalTransactionsRetriesWhenPageChangesAfterLocks(t *testing.T) {
	actor := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	host := uuid.MustParse("20000000-0000-4000-8000-000000000002")
	createdAt := time.Date(2030, 8, 9, 10, 11, 12, 123456000, time.UTC)
	initial := earlyRentalTransactionListCandidate{
		offerID:   uuid.MustParse("30000000-0000-4000-8000-000000000003"),
		paymentID: uuid.MustParse("40000000-0000-4000-8000-000000000004"),
		hostID:    host, renterID: actor, createdAt: createdAt,
	}
	changed := initial
	changed.offerID = uuid.MustParse("50000000-0000-4000-8000-000000000005")
	changed.paymentID = uuid.MustParse("60000000-0000-4000-8000-000000000006")
	tx := &earlyRentalTransactionListFakeTx{pages: [][]earlyRentalTransactionListCandidate{{initial}, {changed}}}
	_, err := listEarlyRentalTransactions(context.Background(), tx, EarlyRentalTransactionListInput{
		ActorID: actor, Role: "renter", State: "enviado", Limit: 10,
	})
	if !errors.Is(err, ErrEarlyRentalTransactionListRetry) {
		t.Fatalf("page-change error=%v, want ErrEarlyRentalTransactionListRetry", err)
	}
	if tx.queryCount != 2 {
		t.Fatalf("queries=%d, want discovery and post-lock recheck", tx.queryCount)
	}
	wantLocks := []string{actor.String(), host.String()}
	if !reflect.DeepEqual(tx.lockIDs, wantLocks) {
		t.Fatalf("lock IDs=%v, want globally ordered %v", tx.lockIDs, wantLocks)
	}
}
