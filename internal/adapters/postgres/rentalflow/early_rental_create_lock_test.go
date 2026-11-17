package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type earlyLockTx struct {
	pgx.Tx
	calls  int
	query  string
	args   []any
	err    error
	onExec func()
}

func (f *earlyLockTx) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	if f == nil {
		return pgconn.CommandTag{}, errors.New("nil tx invoked")
	}
	f.calls++
	f.query = q
	f.args = append([]any(nil), args...)
	if f.onExec != nil {
		f.onExec()
	}
	return pgconn.CommandTag{}, f.err
}

func (*earlyLockTx) Begin(context.Context) (pgx.Tx, error) {
	panic("create-key lock must not begin a transaction")
}
func (*earlyLockTx) Commit(context.Context) error {
	panic("create-key lock must not commit a transaction")
}
func (*earlyLockTx) Rollback(context.Context) error {
	panic("create-key lock must not roll back a transaction")
}

func TestLockEarlyRentalCreateKeyAlreadyCanceledDoesNotExec(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &earlyLockTx{}
	err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse("12345678-1234-4234-8234-123456789abc"), "abcdefgh")
	if !errors.Is(err, context.Canceled) || tx.calls != 0 {
		t.Fatalf("already canceled: error=%v exec calls=%d", err, tx.calls)
	}
}

func TestLockEarlyRentalCreateKeyUsesIndependentNamespacedPreimage(t *testing.T) {
	id := uuid.MustParse("12345678-1234-4234-8234-123456789abc")
	key := "  AbÇ界xy  "
	tx := &earlyLockTx{}
	if err := lockEarlyRentalCreateKey(context.Background(), tx, id, key); err != nil {
		t.Fatal(err)
	}
	// Literal fixture preimage, deliberately independent of production's concatenation.
	digest := sha256.Sum256([]byte("espacigo.early-rental.create-idempotency.v1\x0012345678-1234-4234-8234-123456789abc\x00  AbÇ界xy  "))
	want := []any{int32(0x45524331), int32(binary.BigEndian.Uint32(digest[:4]))}
	if tx.calls != 1 || tx.query != "SELECT pg_advisory_xact_lock($1,$2)" || !reflect.DeepEqual(tx.args, want) {
		t.Fatalf("calls=%d query=%q args=%#v want=%#v", tx.calls, tx.query, tx.args, want)
	}
}

func TestLockEarlyRentalCreateKeyActorAndOpaqueKeyChangeFixture(t *testing.T) {
	id := uuid.MustParse("12345678-1234-4234-8234-123456789abc")
	cases := []struct {
		actor uuid.UUID
		key   string
	}{{id, "abcdefgh"}, {id, "abcdefgH"}, {uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), "abcdefgh"}}
	seen := map[int32]bool{}
	for _, tc := range cases {
		tx := &earlyLockTx{}
		if err := lockEarlyRentalCreateKey(context.Background(), tx, tc.actor, tc.key); err != nil {
			t.Fatal(err)
		}
		obj := tx.args[1].(int32)
		if seen[obj] {
			t.Fatalf("fixture object collision %d", obj)
		}
		seen[obj] = true
	}
}

func TestLockEarlyRentalCreateKeyBoundsAndValidation(t *testing.T) {
	id := uuid.MustParse("12345678-1234-4234-8234-123456789abc")
	for _, key := range []string{strings.Repeat("é", 8), strings.Repeat("界", 200)} {
		tx := &earlyLockTx{}
		if err := lockEarlyRentalCreateKey(context.Background(), tx, id, key); err != nil || tx.calls != 1 {
			t.Errorf("boundary runes=%d err=%v calls=%d", len([]rune(key)), err, tx.calls)
		}
	}
	bad := []struct {
		ctx   context.Context
		tx    *earlyLockTx
		nilTx bool
		actor uuid.UUID
		key   string
	}{{context.Background(), &earlyLockTx{}, false, uuid.Nil, "abcdefgh"}, {context.Background(), &earlyLockTx{}, false, id, "short"}, {context.Background(), &earlyLockTx{}, false, id, strings.Repeat("x", 201)}, {context.Background(), &earlyLockTx{}, false, id, "abcdefgh\x00x"}, {context.Background(), &earlyLockTx{}, false, id, string([]byte{0xff})}, {nil, &earlyLockTx{}, false, id, "abcdefgh"}, {context.Background(), nil, true, id, "abcdefgh"}}
	for i, tc := range bad {
		if err := lockEarlyRentalCreateKey(tc.ctx, func() pgx.Tx {
			if tc.nilTx {
				return nil
			}
			return tc.tx
		}(), tc.actor, tc.key); !errors.Is(err, earlyrental.ErrInvalid) {
			t.Errorf("case %d err=%v", i, err)
		}
		if tc.tx != nil && tc.tx.calls != 0 {
			t.Errorf("case %d exec calls=%d", i, tc.tx.calls)
		}
	}
}

func TestLockEarlyRentalCreateKeyPropagatesExecAndCancellation(t *testing.T) {
	sentinel := errors.New("exec sentinel")
	tx := &earlyLockTx{err: sentinel}
	if err := lockEarlyRentalCreateKey(context.Background(), tx, uuid.MustParse("12345678-1234-4234-8234-123456789abc"), "abcdefgh"); !errors.Is(err, sentinel) {
		t.Fatalf("exec err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tx = &earlyLockTx{onExec: cancel}
	if err := lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse("12345678-1234-4234-8234-123456789abc"), "abcdefgh"); !errors.Is(err, context.Canceled) || tx.calls != 1 {
		t.Fatalf("post-exec cancellation err=%v calls=%d", err, tx.calls)
	}
}
