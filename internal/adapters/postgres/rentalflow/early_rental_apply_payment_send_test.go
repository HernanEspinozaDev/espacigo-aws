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

const earlyRentalPaymentSendCASUpdateOracle = `UPDATE public.arriendo_temprano SET estado=$1,revision=$2,actualizada_en=$3 WHERE id=$4 AND revision=$5 AND estado=$6`

type earlyRentalPaymentSendCASFakeTx struct {
	*offerReadFakeTx
	updateArgs []any
}

func (f *earlyRentalPaymentSendCASFakeTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query == earlyRentalPaymentSendCASUpdateOracle {
		f.calls = append(f.calls, "exec:"+query)
		f.updateArgs = append([]any(nil), args...)
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	return f.offerReadFakeTx.Exec(ctx, query, args...)
}

func TestApplyEarlyRentalPaymentSendCASMissDoesNotWritePaymentOrEvent(t *testing.T) {
	row, _ := offerReadFixture(earlyrental.StatePendingPaymentSend, 5, offerReadConfirms("identidad", "identidad", "contrato", "contrato"), nil)
	readTx := &offerReadFakeTx{row: row}
	f := &earlyRentalPaymentSendCASFakeTx{offerReadFakeTx: readTx}
	at := row.created.Add(20 * time.Minute)
	result, err := applyEarlyRentalPaymentSend(context.Background(), f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID), earlyrental.Command{Kind: earlyrental.CommandSendPayment, ExpectedRevision: 5}, at)
	if !errors.Is(err, earlyrental.ErrPreconditionFailed) {
		t.Fatalf("CAS miss error=%v, want ErrPreconditionFailed", err)
	}
	if !reflect.DeepEqual(result, earlyRentalPaymentSendResult{}) {
		t.Fatalf("CAS miss returned partial result: %+v", result)
	}
	wantArgs := []any{string(earlyrental.StatePendingPaymentReceive), int64(6), at.UTC(), uuid.MustParse(offerReadID), int64(5), string(earlyrental.StatePendingPaymentSend)}
	if !reflect.DeepEqual(f.updateArgs, wantArgs) {
		t.Fatalf("CAS SQL args=%#v, want %#v", f.updateArgs, wantArgs)
	}
	for _, call := range f.calls {
		if strings.Contains(call, "INSERT INTO public.arriendo_temprano_pago") || strings.Contains(call, "UPDATE public.arriendo_temprano_pago") || strings.Contains(call, "INSERT INTO public.arriendo_temprano_evento") {
			t.Fatalf("CAS miss proceeded to payment/event write: %s", call)
		}
	}
}

func TestApplyEarlyRentalPaymentSendRejectsUnsupportedCommandBeforeDatabase(t *testing.T) {
	f := &offerReadFakeTx{}
	_, err := applyEarlyRentalPaymentSend(context.Background(), f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID), earlyrental.Command{Kind: earlyrental.CommandReceivePayment}, time.Now())
	if !errors.Is(err, earlyrental.ErrInvalid) {
		t.Fatalf("unsupported command error=%v, want ErrInvalid", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("unsupported command reached database: %v", f.calls)
	}
}

var _ pgx.Tx = (*earlyRentalPaymentSendCASFakeTx)(nil)
