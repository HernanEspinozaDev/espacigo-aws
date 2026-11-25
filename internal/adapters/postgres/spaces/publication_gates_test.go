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
	profileComplete   bool
	kycReady          bool
	mpConnected       bool
	contractPublished bool
	reads             int
}

func (tx *publicationGateTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	tx.reads++
	return monthReadRow(func(dest ...any) error {
		switch {
		case strings.Contains(q, "account_profile_identity_status"):
			if len(args) != 1 || args[0] != calendarOwnerID {
				return errors.New("kyc check must filter by owner")
			}
			*dest[0].(*bool) = tx.profileComplete
			*dest[1].(*bool) = tx.kycReady
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
		profileComplete   bool
		kycReady          bool
		mpConnected       bool
		contractPublished bool
		want              error
		reads             int
	}{
		{name: "incomplete profile", want: spaces.ErrProfileIncomplete, reads: 1},
		{name: "missing kyc", profileComplete: true, kycReady: false, mpConnected: true, contractPublished: true, want: spaces.ErrIdentityRequired, reads: 1},
		{name: "missing mercadopago", profileComplete: true, kycReady: true, mpConnected: false, contractPublished: true, want: spaces.ErrMercadoPagoRequired, reads: 2},
		{name: "missing contract template", profileComplete: true, kycReady: true, mpConnected: true, contractPublished: false, want: spaces.ErrContractTemplateRequired, reads: 3},
		{name: "all prerequisites satisfied", profileComplete: true, kycReady: true, mpConnected: true, contractPublished: true, want: nil, reads: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &publicationGateTx{
				profileComplete:   tc.profileComplete,
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
