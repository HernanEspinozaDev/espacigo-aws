package spacespg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
)

type publicationGateTx struct {
	pgx.Tx
	kycReady          bool
	mpConnected       bool
	contractPublished bool
	reads             int
}

func (tx *publicationGateTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	tx.reads++
	return monthReadRow(func(dest ...any) error {
		switch {
		case strings.Contains(q, "elegibilidad_verificacion_local"):
			if len(args) != 1 || args[0] != calendarOwnerID {
				return errors.New("kyc check must filter by owner")
			}
			*dest[0].(*bool) = tx.kycReady
		case strings.Contains(q, "mercadopago_seller_connection"):
			if len(args) != 1 || args[0] != calendarOwnerID {
				return errors.New("mercadopago check must filter by owner")
			}
			*dest[0].(*bool) = tx.mpConnected
		case strings.Contains(q, "espacio_contrato_borrador"):
			if len(args) != 1 || args[0] != monthReadSpaceID {
				return errors.New("contract template check must filter by space")
			}
			*dest[0].(*bool) = tx.contractPublished
		default:
			return errors.New("unexpected publication gate query")
		}
		return nil
	})
}

func TestPublicationPrerequisitesOrderAndEnforcement(t *testing.T) {
	for _, tc := range []struct {
		name              string
		kycReady          bool
		mpConnected       bool
		contractPublished bool
		want              error
		reads             int
	}{
		{name: "missing kyc", kycReady: false, mpConnected: true, contractPublished: true, want: spaces.ErrIdentityRequired, reads: 1},
		{name: "missing mercadopago", kycReady: true, mpConnected: false, contractPublished: true, want: spaces.ErrMercadoPagoRequired, reads: 2},
		{name: "missing contract template", kycReady: true, mpConnected: true, contractPublished: false, want: spaces.ErrContractTemplateRequired, reads: 3},
		{name: "all prerequisites satisfied", kycReady: true, mpConnected: true, contractPublished: true, want: nil, reads: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &publicationGateTx{
				kycReady:          tc.kycReady,
				mpConnected:       tc.mpConnected,
				contractPublished: tc.contractPublished,
			}
			err := requirePublicationPrerequisites(context.Background(), tx, calendarOwnerID, monthReadSpaceID)
			if !errors.Is(err, tc.want) || tx.reads != tc.reads {
				t.Fatalf("err=%v reads=%d want %v/%d", err, tx.reads, tc.want, tc.reads)
			}
		})
	}
}
