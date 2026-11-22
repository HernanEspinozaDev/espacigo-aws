package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const expectedClaimPaymentSQL = `UPDATE public.checkout_preferencia_comercial SET payment_create_claimed_at=$2,actualizada_en=$2 WHERE id=$1 AND checkout_method='payments_api' AND test_mode AND estado='pending' AND preference_id IS NULL AND provider_payment_id IS NULL AND payment_create_claimed_at IS NULL AND vence_en>$2 RETURNING id::text`

type paymentClaimRow struct {
	id  string
	err error
}

func (r paymentClaimRow) Scan(dest ...any) error {
	if r.err == nil {
		*dest[0].(*string) = r.id
	}
	return r.err
}

type paymentClaimDB struct {
	query string
	args  []any
	row   pgx.Row
}

func (db *paymentClaimDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.query = query
	db.args = append([]any(nil), args...)
	return db.row
}

func (*paymentClaimDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("ClaimPayment must not query rows")
}

func (*paymentClaimDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("ClaimPayment must use the guarded UPDATE RETURNING")
}

func TestClaimPaymentUsesOneShotTestModeGuard(t *testing.T) {
	const operationID = "10000000-0000-4000-8000-000000000001"
	now := time.Date(2030, time.January, 15, 12, 0, 0, 0, time.UTC)
	db := &paymentClaimDB{row: paymentClaimRow{id: operationID}}
	repo := &Repository{db: db}

	claimed, err := repo.ClaimPayment(context.Background(), operationID, now)
	if err != nil || !claimed {
		t.Fatalf("ClaimPayment() = (%v, %v), want (true, nil)", claimed, err)
	}
	if db.query != expectedClaimPaymentSQL || !reflect.DeepEqual(db.args, []any{operationID, now}) {
		t.Fatalf("claim query/args = (%q, %#v), want exact guarded update and (%q, %#v)", db.query, db.args, expectedClaimPaymentSQL, []any{operationID, now})
	}
}

func TestClaimPaymentReturnsUnclaimedWhenAlreadyClaimedOrIneligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "already claimed", err: pgx.ErrNoRows},
		{name: "expired or non pending", err: pgx.ErrNoRows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &paymentClaimDB{row: paymentClaimRow{err: tc.err}}
			repo := &Repository{db: db}
			claimed, err := repo.ClaimPayment(context.Background(), "10000000-0000-4000-8000-000000000001", time.Now().UTC())
			if err != nil || claimed {
				t.Fatalf("ClaimPayment() = (%v, %v), want (false, nil)", claimed, err)
			}
			if db.query != expectedClaimPaymentSQL {
				t.Fatalf("claim did not retain durable one-shot predicates: %q", db.query)
			}
		})
	}
}

func TestClaimPaymentPropagatesDatabaseFailure(t *testing.T) {
	want := errors.New("synthetic database error")
	db := &paymentClaimDB{row: paymentClaimRow{err: want}}
	repo := &Repository{db: db}
	claimed, err := repo.ClaimPayment(context.Background(), "10000000-0000-4000-8000-000000000001", time.Now().UTC())
	if claimed || !errors.Is(err, want) {
		t.Fatalf("ClaimPayment() = (%v, %v), want (false, %v)", claimed, err, want)
	}
}
