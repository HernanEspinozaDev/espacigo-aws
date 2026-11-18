package rentalflowpg

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const readEventsActor = "20000000-0000-4000-8000-000000000002"
const readEventsHost = "10000000-0000-4000-8000-000000000001"
const readEventsOffer = "30000000-0000-4000-8000-000000000003"

type eventReaderTx struct {
	pgx.Tx
	calls                     []string
	locks                     []string
	headerErr, finalErr       error
	headerNoRows, finalNoRows bool
	final                     func()
}
type eventReaderRow struct {
	values []any
	err    error
	after  func()
}

func (r eventReaderRow) Scan(dst ...any) error {
	if r.after != nil {
		defer r.after()
	}
	if r.err != nil {
		return r.err
	}
	if len(dst) != len(r.values) {
		return errors.New("scan count")
	}
	for i, v := range r.values {
		p := reflect.ValueOf(dst[i])
		if !p.IsValid() || p.Kind() != reflect.Ptr {
			return errors.New("bad destination")
		}
		if r.values[i] == nil {
			p.Elem().SetZero()
		} else {
			s := reflect.ValueOf(v)
			if !s.Type().AssignableTo(p.Elem().Type()) {
				return errors.New("scan type")
			}
			p.Elem().Set(s)
		}
	}
	return nil
}
func (f *eventReaderTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	f.calls = append(f.calls, "query:"+q)
	if q == earlyRentalOfferHeaderSQL {
		if f.headerErr != nil {
			return eventReaderRow{err: f.headerErr}
		}
		if f.headerNoRows {
			return eventReaderRow{err: pgx.ErrNoRows}
		}
		if len(args) != 2 || args[0] != uuid.MustParse(readEventsOffer) || args[1] != uuid.MustParse(readEventsActor) {
			return eventReaderRow{err: errors.New("header args")}
		}
		return eventReaderRow{values: []any{uuid.MustParse(readEventsHost), uuid.MustParse(readEventsActor)}}
	}
	return eventReaderRow{err: errors.New("unexpected QueryRow")}
}
func (f *eventReaderTx) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	f.calls = append(f.calls, "query:"+q)
	if q != earlyRentalEventsFinalSQL || len(args) != 5 || args[0] != uuid.MustParse(readEventsOffer) || args[1] != uuid.MustParse(readEventsHost) || args[2] != uuid.MustParse(readEventsActor) || args[3] != uuid.MustParse(readEventsActor) || args[4] != int64(1) {
		return nil, errors.New("unexpected final query or arguments")
	}
	if f.finalErr != nil {
		return nil, f.finalErr
	}
	if f.finalNoRows {
		return &eventReaderRows{}, nil
	}
	return &eventReaderRows{final: f.final, values: []any{uuid.MustParse(readEventsOffer), uuid.MustParse(readEventsHost), uuid.MustParse(readEventsActor), int64(1), string(earlyrental.StatePendingIdentity), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil}}, nil
}
func (f *eventReaderTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, "exec:"+q)
	if q != `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))` || len(args) != 1 {
		return pgconn.CommandTag{}, errors.New("unexpected lock")
	}
	s, ok := args[0].(string)
	if !ok {
		return pgconn.CommandTag{}, errors.New("lock id type")
	}
	f.locks = append(f.locks, s)
	return pgconn.NewCommandTag("SELECT 1"), nil
}
func (*eventReaderTx) Begin(context.Context) (pgx.Tx, error) { panic("begin forbidden") }
func (*eventReaderTx) Commit(context.Context) error          { panic("commit forbidden") }
func (*eventReaderTx) Rollback(context.Context) error        { panic("rollback forbidden") }

type eventReaderRows struct {
	pgx.Rows
	final  func()
	values []any
	used   bool
}

func (r *eventReaderRows) Next() bool {
	if r.used {
		return false
	}
	r.used = true
	return len(r.values) > 0
}
func (r *eventReaderRows) Scan(dst ...any) error {
	if len(dst) != len(r.values) {
		return errors.New("scan count")
	}
	for i, v := range r.values {
		p := reflect.ValueOf(dst[i])
		if v == nil {
			p.Elem().SetZero()
		} else {
			p.Elem().Set(reflect.ValueOf(v))
		}
	}
	return nil
}
func (r *eventReaderRows) Err() error { return nil }
func (r *eventReaderRows) Close() {
	if r.final != nil {
		r.final()
		r.final = nil
	}
}
func (r *eventReaderRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *eventReaderRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *eventReaderRows) RawValues() [][]byte                          { return nil }
func (r *eventReaderRows) Conn() *pgx.Conn                              { return nil }

func TestReadEarlyRentalEventsRequiresAuthorizedSingleSnapshotFeed(t *testing.T) {
	f := &eventReaderTx{headerNoRows: true}
	got, err := readEarlyRentalEvents(context.Background(), f, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), math.MaxInt64)
	if !errors.Is(err, rentalflow.ErrNotFound) || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
	f = &eventReaderTx{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.final = cancel
	got, err = readEarlyRentalEvents(ctx, f, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), 1)
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("late context got=%v err=%v", got, err)
	}
	if !reflect.DeepEqual(f.locks, []string{readEventsHost, readEventsActor}) || len(f.calls) != 4 || f.calls[0] != "query:"+earlyRentalOfferHeaderSQL || f.calls[3] != "query:"+earlyRentalEventsFinalSQL {
		t.Fatalf("unexpected lock/query order: %v", f.calls)
	}
	f = &eventReaderTx{}
	got, err = readEarlyRentalEvents(context.Background(), f, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), math.MaxInt64+1)
	if !errors.Is(err, earlyrental.ErrInvalid) || got != nil || len(f.calls) != 0 {
		t.Fatalf("overflow cursor: got=%v err=%v calls=%v", got, err, f.calls)
	}
}
func TestReadEarlyRentalEventsInvalidAndErrorsReturnNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name         string
		ctx          context.Context
		tx           pgx.Tx
		actor, offer uuid.UUID
		want         error
	}{{"nil ctx", nil, &eventReaderTx{}, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), earlyrental.ErrInvalid}, {"cancelled", ctx, &eventReaderTx{}, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), context.Canceled}, {"nil actor", context.Background(), &eventReaderTx{}, uuid.Nil, uuid.MustParse(readEventsOffer), earlyrental.ErrInvalid}, {"nil tx", context.Background(), nil, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), earlyrental.ErrInvalid}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readEarlyRentalEvents(tc.ctx, tc.tx, tc.actor, tc.offer, 0)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("got=%v err=%v", got, err)
			}
		})
	}
	sentinel := errors.New("query error")
	got, err := readEarlyRentalEvents(context.Background(), &eventReaderTx{headerErr: sentinel}, uuid.MustParse(readEventsActor), uuid.MustParse(readEventsOffer), 0)
	if !errors.Is(err, sentinel) || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
