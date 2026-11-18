package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const earlyRentalConfirmationCASUpdateOracle = `UPDATE public.arriendo_temprano SET estado=$1,revision=$2,actualizada_en=$3 WHERE id=$4 AND revision=$5 AND estado=$6`

type earlyRentalConfirmationCASFakeTx struct {
	*offerReadFakeTx
	updateArgs []any
}

func (f *earlyRentalConfirmationCASFakeTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query == earlyRentalConfirmationCASUpdateOracle {
		f.calls = append(f.calls, "exec:"+query)
		f.updateArgs = append([]any(nil), args...)
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	return f.offerReadFakeTx.Exec(ctx, query, args...)
}

func TestApplyEarlyRentalConfirmationCASMissDoesNotWriteEvidence(t *testing.T) {
	row, _ := offerReadFixture(earlyrental.StatePendingIdentity, 1, nil, nil)
	readTx := &offerReadFakeTx{row: row}
	f := &earlyRentalConfirmationCASFakeTx{offerReadFakeTx: readTx}
	at := row.created.Add(time.Minute)
	result, err := applyEarlyRentalConfirmation(context.Background(), f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID), earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity, ExpectedRevision: 1}, at)
	if !errors.Is(err, earlyrental.ErrPreconditionFailed) {
		t.Fatalf("CAS miss error=%v, want ErrPreconditionFailed", err)
	}
	if !reflect.DeepEqual(result, earlyRentalConfirmationResult{}) {
		t.Fatalf("CAS miss returned partial result: %+v", result)
	}
	wantArgs := []any{string(earlyrental.StatePendingIdentity), int64(2), at.UTC(), uuid.MustParse(offerReadID), int64(1), string(earlyrental.StatePendingIdentity)}
	if !reflect.DeepEqual(f.updateArgs, wantArgs) {
		t.Fatalf("CAS SQL args=%#v, want %#v", f.updateArgs, wantArgs)
	}
	for _, call := range f.calls {
		if strings.Contains(call, "INSERT INTO public.arriendo_temprano_confirmacion") || strings.Contains(call, "INSERT INTO public.arriendo_temprano_evento") {
			t.Fatalf("CAS miss proceeded to an evidence write: %s", call)
		}
	}
}

func TestApplyEarlyRentalConfirmationRejectsUnsupportedCommandBeforeDatabase(t *testing.T) {
	f := &offerReadFakeTx{}
	_, err := applyEarlyRentalConfirmation(context.Background(), f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID), earlyrental.Command{Kind: earlyrental.CommandReject}, time.Now())
	if !errors.Is(err, earlyrental.ErrInvalid) {
		t.Fatalf("unsupported command error=%v, want ErrInvalid", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("unsupported command reached database: %v", f.calls)
	}
}

var _ pgx.Tx = (*earlyRentalConfirmationCASFakeTx)(nil)
