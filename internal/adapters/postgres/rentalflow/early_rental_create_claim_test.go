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

const expectedCreateClaimSQL = `INSERT INTO public.arriendo_temprano_idempotencia
(actor_id,arriendo_id,comando,clave,huella,evento_id,creado_en)
SELECT $1,a.id,'crear_oferta',$3,$4,e.id,a.creada_en
FROM public.arriendo_temprano AS a
JOIN public.arriendo_temprano_evento AS e ON e.arriendo_id=a.id
WHERE a.id=$2 AND a.arrendatario_id=$1
AND a.estado='pendiente_identidad' AND a.revision=1
AND e.tipo='oferta_creada' AND e.actor_id=$1 AND e.secuencia=1 AND e.revision=1
AND e.estado_anterior IS NULL AND e.estado_nuevo='pendiente_identidad'
AND e.ocurrido_en=a.creada_en
RETURNING evento_id`

var claimActor = uuid.MustParse("10000000-0000-4000-8000-000000000001")
var claimOffer = uuid.MustParse("20000000-0000-4000-8000-000000000002")
var claimEvent = uuid.MustParse("30000000-0000-4000-8000-000000000003")

type claimRow struct {
	event uuid.UUID
	err   error
	after func()
}

func (r claimRow) Scan(dest ...any) error {
	*dest[0].(*uuid.UUID) = r.event // Populate even on error to test zero-result safety.
	if r.after != nil {
		r.after()
	}
	return r.err
}

type claimTx struct {
	pgx.Tx
	calls     int
	query     string
	args      []any
	err       error
	zeroEvent bool
	after     func()
}

func (f *claimTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.calls++
	f.query = sql
	f.args = append([]any(nil), args...)
	event := claimEvent
	if f.zeroEvent {
		event = uuid.Nil
	}
	return claimRow{event: event, err: f.err, after: f.after}
}
func (*claimTx) Begin(context.Context) (pgx.Tx, error) { panic("claim must not begin transaction") }
func (*claimTx) Commit(context.Context) error          { panic("claim must not commit transaction") }
func (*claimTx) Rollback(context.Context) error        { panic("claim must not roll back transaction") }

func TestInsertEarlyRentalCreateClaimExactSQLAndEvent(t *testing.T) {
	fp := [32]byte{0: 7, 15: 9, 31: 255}
	tx := &claimTx{}
	id, err := insertEarlyRentalCreateClaim(context.Background(), tx, claimActor, claimOffer, "  clé-東京  ", fp)
	if err != nil || id != claimEvent {
		t.Fatalf("event=%s err=%v, want %s", id, err, claimEvent)
	}
	if tx.calls != 1 || tx.query != expectedCreateClaimSQL || !reflect.DeepEqual(tx.args, []any{claimActor, claimOffer, "  clé-東京  ", fp[:]}) {
		t.Fatalf("query/calls/args mismatch: %d %q %#v", tx.calls, tx.query, tx.args)
	}
}

func TestInsertEarlyRentalCreateClaimRuneBoundaries(t *testing.T) {
	for _, key := range []string{strings.Repeat("é", 8), strings.Repeat("界", 200)} {
		tx := &claimTx{}
		id, err := insertEarlyRentalCreateClaim(context.Background(), tx, claimActor, claimOffer, key, [32]byte{})
		if err != nil || id != claimEvent || tx.calls != 1 || tx.args[2] != key {
			t.Fatalf("boundary: id=%s err=%v calls=%d", id, err, tx.calls)
		}
	}
}

func TestInsertEarlyRentalCreateClaimErrorResultsAreZero(t *testing.T) {
	sentinel := errors.New("scan sentinel")
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "synthetic_claim_unique"}
	for _, tc := range []struct {
		name        string
		dbErr, want error
		zero        bool
	}{
		{"scan", sentinel, sentinel, false},
		{"typed duplicate", unique, unique, false},
		{"no rows", pgx.ErrNoRows, earlyrental.ErrInvalid, false},
		{"nil event", nil, earlyrental.ErrInvalid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &claimTx{err: tc.dbErr, zeroEvent: tc.zero}
			id, err := insertEarlyRentalCreateClaim(context.Background(), tx, claimActor, claimOffer, "abcdefgh", [32]byte{})
			if id != uuid.Nil || !errors.Is(err, tc.want) || tx.calls != 1 {
				t.Fatalf("unsafe error result: id=%s err=%v calls=%d", id, err, tx.calls)
			}
			if tc.dbErr == unique {
				var pg *pgconn.PgError
				if !errors.As(err, &pg) || pg.Code != "23505" || pg.ConstraintName != "synthetic_claim_unique" {
					t.Fatalf("SQL error type lost: %v", err)
				}
			}
		})
	}
}

func TestInsertEarlyRentalCreateClaimCancellation(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before query", false: "after scan"}[before], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx := &claimTx{}
			wantCalls := 1
			if before {
				cancel()
				wantCalls = 0
			} else {
				tx.after = cancel
			}
			id, err := insertEarlyRentalCreateClaim(ctx, tx, claimActor, claimOffer, "abcdefgh", [32]byte{})
			if id != uuid.Nil || !errors.Is(err, context.Canceled) || tx.calls != wantCalls {
				t.Fatalf("cancellation: id=%s err=%v calls=%d", id, err, tx.calls)
			}
		})
	}
}

func TestInsertEarlyRentalCreateClaimRejectsInvalidBeforeQuery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		ctx          context.Context
		actor, offer uuid.UUID
		key          string
		nilTx        bool
	}{
		{"nil context", nil, claimActor, claimOffer, "abcdefgh", false},
		{"nil transaction", context.Background(), claimActor, claimOffer, "abcdefgh", true},
		{"nil actor", context.Background(), uuid.Nil, claimOffer, "abcdefgh", false},
		{"nil offer", context.Background(), claimActor, uuid.Nil, "abcdefgh", false},
		{"bad UTF8", context.Background(), claimActor, claimOffer, strings.Repeat(string([]byte{0xff}), 8), false},
		{"NUL", context.Background(), claimActor, claimOffer, "abcdefgh\x00", false},
		{"short Unicode", context.Background(), claimActor, claimOffer, strings.Repeat("界", 7), false},
		{"long Unicode", context.Background(), claimActor, claimOffer, strings.Repeat("界", 201), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &claimTx{}
			var tx pgx.Tx = fake
			if tc.nilTx {
				tx = nil
			}
			id, err := insertEarlyRentalCreateClaim(tc.ctx, tx, tc.actor, tc.offer, tc.key, [32]byte{})
			if !errors.Is(err, earlyrental.ErrInvalid) || id != uuid.Nil || fake.calls != 0 {
				t.Fatalf("invalid input: id=%s err=%v calls=%d", id, err, fake.calls)
			}
		})
	}
}
