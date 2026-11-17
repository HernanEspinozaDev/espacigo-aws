package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const expectedEarlyRentalReplayLookupSQL = `SELECT i.actor_id, i.clave, i.arriendo_id, i.huella, a.arrendatario_id, e.id, e.tipo, e.actor_id, e.secuencia
FROM public.arriendo_temprano_idempotencia AS i
JOIN public.arriendo_temprano AS a ON a.id = i.arriendo_id
JOIN public.arriendo_temprano_evento AS e ON e.id = i.evento_id AND e.arriendo_id = i.arriendo_id
WHERE i.actor_id = $1 AND i.comando = 'crear_oferta' AND i.clave COLLATE "C" = $2::text COLLATE "C"
ORDER BY i.arriendo_id
LIMIT 2`

type replayLookupRow struct {
	idemActor uuid.UUID
	key       string
	offer     uuid.UUID
	digest    []byte
	renter    uuid.UUID
	event     uuid.UUID
	kind      string
	actor     uuid.UUID
	seq       int64
}
type replayLookupRows struct {
	pgx.Rows
	rows       []replayLookupRow
	idx        int
	err        error
	scanErr    error
	scanErrAt  int
	closed     bool
	closeCalls int
	after      func()
}

func (r *replayLookupRows) Close() { r.closed = true; r.closeCalls++ }
func (r *replayLookupRows) Err() error {
	if r.after != nil {
		r.after()
	}
	return r.err
}
func (r *replayLookupRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *replayLookupRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *replayLookupRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}
func (r *replayLookupRows) Scan(dst ...any) error {
	if r.scanErr != nil && r.idx == r.scanErrAt {
		return r.scanErr
	}
	x := r.rows[r.idx-1]
	*dst[0].(*uuid.UUID) = x.idemActor
	*dst[1].(*string) = x.key
	*dst[2].(*uuid.UUID) = x.offer
	*dst[3].(*[]byte) = x.digest
	*dst[4].(*uuid.UUID) = x.renter
	*dst[5].(*uuid.UUID) = x.event
	*dst[6].(*string) = x.kind
	*dst[7].(*uuid.UUID) = x.actor
	*dst[8].(*int64) = x.seq
	return nil
}
func (r *replayLookupRows) Values() ([]any, error) { return nil, nil }
func (r *replayLookupRows) RawValues() [][]byte    { return nil }
func (r *replayLookupRows) Conn() *pgx.Conn        { return nil }

type replayLookupTx struct {
	pgx.Tx
	rows          *replayLookupRows
	calls         int
	query         string
	args          []any
	queryErr      error
	beginCalls    int
	commitCalls   int
	rollbackCalls int
	execCalls     int
}

func (t *replayLookupTx) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	t.calls++
	t.query = q
	t.args = append([]any(nil), args...)
	if t.queryErr != nil {
		return nil, t.queryErr
	}
	return t.rows, nil
}
func (t *replayLookupTx) Begin(context.Context) (pgx.Tx, error) {
	t.beginCalls++
	panic("lookup must not begin a transaction")
}
func (t *replayLookupTx) Commit(context.Context) error {
	t.commitCalls++
	panic("lookup must not commit a transaction")
}
func (t *replayLookupTx) Rollback(context.Context) error {
	t.rollbackCalls++
	panic("lookup must not rollback a transaction")
}
func (t *replayLookupTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	t.execCalls++
	panic("lookup must not execute statements")
}

const (
	replayLookupActor = "12345678-1234-4234-8234-123456789abc"
	replayLookupOffer = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	replayLookupEvent = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func replayLookupFixture() (uuid.UUID, replayLookupRow, [32]byte) {
	actor := uuid.MustParse(replayLookupActor)
	digest := [32]byte{7}
	row := replayLookupRow{idemActor: actor, key: "abcdefgh", offer: uuid.MustParse(replayLookupOffer), digest: append([]byte(nil), digest[:]...), renter: actor, event: uuid.MustParse(replayLookupEvent), kind: "oferta_creada", actor: actor, seq: 1}
	return actor, row, digest
}
func replayLookupTxWithRows(rows ...replayLookupRow) (*replayLookupTx, *replayLookupRows) {
	rs := &replayLookupRows{rows: rows}
	return &replayLookupTx{rows: rs}, rs
}
func assertReplayLookupOracle(t *testing.T, tx *replayLookupTx, actor uuid.UUID, key string) {
	t.Helper()
	if tx.calls != 1 || tx.query != expectedEarlyRentalReplayLookupSQL || !reflect.DeepEqual(tx.args, []any{actor, key}) {
		t.Fatalf("query oracle mismatch: calls=%d query=%q args=%#v", tx.calls, tx.query, tx.args)
	}
}
func assertReplayLookupNoPartial(t *testing.T, id uuid.UUID, found bool) {
	t.Helper()
	if id != uuid.Nil || found {
		t.Fatalf("partial replay result: id=%s found=%v", id, found)
	}
}

func TestLookupEarlyRentalCreateReplayValidHitAndExactQuery(t *testing.T) {
	actor, row, fingerprint := replayLookupFixture()
	row.key = "  key-abc  "
	tx, rows := replayLookupTxWithRows(row)
	got, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, row.key, fingerprint)
	if err != nil || !found || got != row.offer {
		t.Fatalf("got=%s found=%v err=%v", got, found, err)
	}
	assertReplayLookupOracle(t, tx, actor, row.key)
	if !rows.closed || rows.closeCalls != 1 {
		t.Fatalf("rows close: closed=%v calls=%d", rows.closed, rows.closeCalls)
	}
}
func TestLookupEarlyRentalCreateReplayMiss(t *testing.T) {
	actor, _, fingerprint := replayLookupFixture()
	tx, rows := replayLookupTxWithRows()
	id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, "abcdefgh", fingerprint)
	if err != nil || found || id != uuid.Nil || !rows.closed {
		t.Fatalf("%v %v %v closed=%v", id, found, err, rows.closed)
	}
	assertReplayLookupOracle(t, tx, actor, "abcdefgh")
}
func TestLookupEarlyRentalCreateReplayKeyRuneBoundaries(t *testing.T) {
	actor, _, fingerprint := replayLookupFixture()
	for _, tc := range []struct {
		name, key string
	}{
		{"min-8", "ééééabcd"}, {"max-200", strings.Repeat("界", 200)},
	} {
		key := tc.key
		t.Run(tc.name, func(t *testing.T) {
			tx, _ := replayLookupTxWithRows()
			_, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, key, fingerprint)
			if err != nil || found || tx.calls != 1 {
				t.Fatalf("key rune boundary calls=%d found=%v err=%v", tx.calls, found, err)
			}
		})
	}
}

func TestLookupEarlyRentalCreateReplayConflictAmbiguityAndStoredInvalidFields(t *testing.T) {
	actor, base, fingerprint := replayLookupFixture()
	t.Run("fingerprint conflict", func(t *testing.T) {
		tx, rows := replayLookupTxWithRows(base)
		other := fingerprint
		other[0]++
		id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, other)
		assertReplayLookupNoPartial(t, id, found)
		if !errors.Is(err, errEarlyRentalCreateKeyConflict) || !rows.closed {
			t.Fatalf("err=%v closed=%v", err, rows.closed)
		}
	})
	differentDigest := base
	differentDigest.digest = append([]byte(nil), base.digest...)
	differentDigest.digest[0] ^= 1
	for _, tc := range []struct {
		name     string
		second   replayLookupRow
		supplied [32]byte
	}{
		{"same fingerprint", base, fingerprint},
		{"different stored fingerprints", differentDigest, fingerprint},
		{"first conflicts", base, [32]byte{9}},
	} {
		second := tc.second
		second.offer = uuid.MustParse("cccccccc-cccc-4ccc-8ccc-cccccccccccc")
		second.event = uuid.MustParse("dddddddd-dddd-4ddd-8ddd-dddddddddddd")
		t.Run("ambiguous "+tc.name, func(t *testing.T) {
			tx, rows := replayLookupTxWithRows(base, second)
			id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, tc.supplied)
			assertReplayLookupNoPartial(t, id, found)
			if !errors.Is(err, errEarlyRentalCreateReplayAmbiguous) || !rows.closed {
				t.Fatalf("err=%v closed=%v", err, rows.closed)
			}
		})
	}
	mutations := []struct {
		name   string
		mutate func(*replayLookupRow)
	}{
		{"idempotency actor", func(r *replayLookupRow) { r.idemActor = uuid.MustParse(replayLookupEvent) }}, {"key", func(r *replayLookupRow) { r.key = "abcdefgH" }}, {"nil offer", func(r *replayLookupRow) { r.offer = uuid.Nil }}, {"nil event", func(r *replayLookupRow) { r.event = uuid.Nil }}, {"wrong renter", func(r *replayLookupRow) { r.renter = uuid.MustParse(replayLookupEvent) }}, {"wrong event actor", func(r *replayLookupRow) { r.actor = uuid.MustParse(replayLookupEvent) }}, {"wrong kind", func(r *replayLookupRow) { r.kind = "otra" }}, {"sequence zero", func(r *replayLookupRow) { r.seq = 0 }}, {"sequence two", func(r *replayLookupRow) { r.seq = 2 }}, {"digest 0", func(r *replayLookupRow) { r.digest = make([]byte, 0) }}, {"digest 31", func(r *replayLookupRow) { r.digest = make([]byte, 31) }}, {"digest 33", func(r *replayLookupRow) { r.digest = make([]byte, 33) }},
	}
	for _, tc := range mutations {
		t.Run("stored invalid "+tc.name, func(t *testing.T) {
			baselineTx, baselineRows := replayLookupTxWithRows(base)
			baselineID, baselineFound, baselineErr := lookupEarlyRentalCreateReplay(context.Background(), baselineTx, actor, base.key, fingerprint)
			if baselineErr != nil || !baselineFound || baselineID != base.offer || !baselineRows.closed {
				t.Fatalf("valid baseline not accepted: id=%s found=%v err=%v", baselineID, baselineFound, baselineErr)
			}
			bad := base
			bad.digest = append([]byte(nil), base.digest...)
			tc.mutate(&bad)
			tx, rows := replayLookupTxWithRows(bad)
			id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, fingerprint)
			assertReplayLookupNoPartial(t, id, found)
			if !errors.Is(err, earlyrental.ErrInvalid) || !rows.closed || rows.closeCalls != 1 {
				t.Fatalf("id=%s found=%v err=%v closed=%v", id, found, err, rows.closed)
			}
		})
	}
}

func TestLookupEarlyRentalCreateReplayQueryScanRowsErrorsAndCancellation(t *testing.T) {
	actor, base, fingerprint := replayLookupFixture()
	sentinel := errors.New("sentinel")
	t.Run("query error no rows", func(t *testing.T) {
		tx, _ := replayLookupTxWithRows()
		tx.queryErr = sentinel
		id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, fingerprint)
		assertReplayLookupNoPartial(t, id, found)
		if !errors.Is(err, sentinel) || tx.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, tx.calls)
		}
	})
	t.Run("scan error closes", func(t *testing.T) {
		tx, rows := replayLookupTxWithRows(base)
		rows.scanErr = sentinel
		rows.scanErrAt = 1
		id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, fingerprint)
		assertReplayLookupNoPartial(t, id, found)
		if !errors.Is(err, sentinel) || !rows.closed {
			t.Fatalf("err=%v closed=%v", err, rows.closed)
		}
	})
	t.Run("rows error after valid row", func(t *testing.T) {
		tx, rows := replayLookupTxWithRows(base)
		rows.err = sentinel
		id, found, err := lookupEarlyRentalCreateReplay(context.Background(), tx, actor, base.key, fingerprint)
		assertReplayLookupNoPartial(t, id, found)
		if !errors.Is(err, sentinel) || !rows.closed {
			t.Fatalf("err=%v closed=%v", err, rows.closed)
		}
	})
	t.Run("cancel after final dependency", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tx, rows := replayLookupTxWithRows(base)
		rows.after = cancel
		id, found, err := lookupEarlyRentalCreateReplay(ctx, tx, actor, base.key, fingerprint)
		assertReplayLookupNoPartial(t, id, found)
		if !errors.Is(err, context.Canceled) || !rows.closed {
			t.Fatalf("err=%v closed=%v", err, rows.closed)
		}
	})
}

func TestLookupEarlyRentalCreateReplayRejectsInvalidArgumentsWithoutQuery(t *testing.T) {
	actor, _, fingerprint := replayLookupFixture()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name  string
		ctx   context.Context
		tx    pgx.Tx
		actor uuid.UUID
		key   string
		want  error
	}{
		{"nil context", nil, &replayLookupTx{}, actor, "abcdefgh", earlyrental.ErrInvalid}, {"nil tx", context.Background(), nil, actor, "abcdefgh", earlyrental.ErrInvalid}, {"nil actor", context.Background(), &replayLookupTx{}, uuid.Nil, "abcdefgh", earlyrental.ErrInvalid}, {"invalid UTF-8", context.Background(), &replayLookupTx{}, actor, string([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8}), earlyrental.ErrInvalid}, {"NUL", context.Background(), &replayLookupTx{}, actor, "abcdefgh\x00", earlyrental.ErrInvalid}, {"7 runes", context.Background(), &replayLookupTx{}, actor, "ééééééé", earlyrental.ErrInvalid}, {"201 runes", context.Background(), &replayLookupTx{}, actor, strings.Repeat("a", 201), earlyrental.ErrInvalid}, {"pre-canceled", cancelled, &replayLookupTx{}, actor, "abcdefgh", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, found, err := lookupEarlyRentalCreateReplay(tc.ctx, tc.tx, tc.actor, tc.key, fingerprint)
			assertReplayLookupNoPartial(t, id, found)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tx, ok := tc.tx.(*replayLookupTx); ok && tx.calls != 0 {
				t.Fatalf("invalid input queried %d times", tx.calls)
			}
		})
	}
}
