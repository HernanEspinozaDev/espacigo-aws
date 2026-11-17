package earlyrentalhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

func TestProjectEarlyRentalTransactionsShowsOneSharedMovementToBothParticipants(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	cursor := "opaque-next-cursor"
	for _, state := range []string{"enviado", "recibido", "anulado", "vencido"} {
		t.Run(state, func(t *testing.T) {
			offer := newEarlyRentalTransactionTestOffer(t, createdAt, state)
			for _, tc := range []struct {
				actor       string
				role        string
				perspective string
			}{
				{actor: offer.RenterID, role: "arrendatario", perspective: "egreso"},
				{actor: offer.HostID, role: "arrendador", perspective: "ingreso"},
			} {
				projected, err := projectEarlyRentalTransactions([]earlyrental.Offer{offer}, tc.actor, &cursor)
				if err != nil {
					t.Fatalf("projectEarlyRentalTransactions(%s) error = %v", tc.role, err)
				}
				body, err := json.Marshal(projected)
				if err != nil {
					t.Fatal(err)
				}
				wire := string(body)
				if strings.Contains(wire, offer.HostID) || strings.Contains(wire, offer.RenterID) {
					t.Fatalf("participant account ID leaked in transaction list: %s", wire)
				}
				var decoded struct {
					Data struct {
						Items []struct {
							ID            string     `json:"id"`
							EarlyRentalID string     `json:"early_rental_id"`
							ActorRole     string     `json:"actor_role"`
							Perspective   string     `json:"perspective"`
							SpaceTitle    string     `json:"space_title"`
							Amount        int64      `json:"amount_clp"`
							Currency      string     `json:"currency"`
							State         string     `json:"state"`
							SentAt        *time.Time `json:"sent_at"`
							ReceivedAt    *time.Time `json:"received_at"`
						} `json:"items"`
						NextCursor *string `json:"next_cursor"`
					} `json:"data"`
				}
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatal(err)
				}
				if len(decoded.Data.Items) != 1 || decoded.Data.NextCursor == nil || *decoded.Data.NextCursor != cursor {
					t.Fatalf("transaction envelope = %+v", decoded.Data)
				}
				item := decoded.Data.Items[0]
				if item.ID != offer.PaymentID || item.EarlyRentalID != offer.ID || item.ActorRole != tc.role || item.Perspective != tc.perspective ||
					item.SpaceTitle != offer.Snapshot.SpaceTitle || item.Amount != offer.Snapshot.SubtotalCLP || item.Currency != "CLP" || item.State != state ||
					item.SentAt == nil || !item.SentAt.Equal(*offer.PaymentSentAt) {
					t.Fatalf("transaction item = %+v", item)
				}
				if state == "recibido" {
					if item.ReceivedAt == nil || offer.PaymentReceivedAt == nil || !item.ReceivedAt.Equal(*offer.PaymentReceivedAt) {
						t.Fatalf("received movement = %+v", item)
					}
				} else if item.ReceivedAt != nil {
					t.Fatalf("movement invented receipt for state %q: %+v", state, item)
				}
				var root map[string]json.RawMessage
				if err := json.Unmarshal(body, &root); err != nil {
					t.Fatal(err)
				}
				var data map[string]json.RawMessage
				if err := json.Unmarshal(root["data"], &data); err != nil {
					t.Fatal(err)
				}
				var items []json.RawMessage
				if err := json.Unmarshal(data["items"], &items); err != nil || len(items) != 1 {
					t.Fatalf("transaction items = %s, error = %v", data["items"], err)
				}
				var itemKeys map[string]json.RawMessage
				if err := json.Unmarshal(items[0], &itemKeys); err != nil {
					t.Fatal(err)
				}
				if len(itemKeys) != 10 {
					t.Fatalf("transaction has %d wire fields, want exactly 10 OpenAPI fields", len(itemKeys))
				}
			}
		})
	}
}

func TestProjectEarlyRentalTransactionsReturnsEmptyArrayAndNullCursor(t *testing.T) {
	projected, err := projectEarlyRentalTransactions(nil, earlyRentalTestRenterID, nil)
	if err != nil {
		t.Fatalf("project empty transactions: %v", err)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"data":{"items":[],"next_cursor":null}}` {
		t.Fatalf("empty transaction wire = %s", body)
	}
}

func TestProjectEarlyRentalTransactionsRejectsInvalidRowsAndActors(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalTransactionTestOffer(t, createdAt, "enviado")
	if _, err := projectEarlyRentalTransactions([]earlyrental.Offer{offer}, "33333333-3333-4333-8333-333333333333", nil); err == nil {
		t.Fatal("non-participant transaction projection unexpectedly succeeded")
	}
	if _, err := projectEarlyRentalTransactions([]earlyrental.Offer{newEarlyRentalDetailTestOffer(t, createdAt)}, offer.RenterID, nil); err == nil {
		t.Fatal("offer without sent payment unexpectedly projected as a transaction")
	}
	longCursor := strings.Repeat("x", 2049)
	if _, err := projectEarlyRentalTransactions([]earlyrental.Offer{offer}, offer.RenterID, &longCursor); err == nil {
		t.Fatal("oversized transaction cursor unexpectedly succeeded")
	}
	duplicateMovement := offer
	duplicateMovement.ID = "550e8400-e29b-41d4-a716-446655440099"
	if _, err := projectEarlyRentalTransactions([]earlyrental.Offer{offer, duplicateMovement}, offer.RenterID, nil); err == nil {
		t.Fatal("duplicate canonical movement unexpectedly succeeded")
	}
}

func newEarlyRentalTransactionTestOffer(t *testing.T, now time.Time, targetState string) earlyrental.Offer {
	t.Helper()
	offer := newEarlyRentalDetailTestOffer(t, now)
	paymentID := "550e8400-e29b-41d4-a716-446655440090"
	apply := func(actor string, command earlyrental.Command, at time.Time) {
		t.Helper()
		command.ExpectedRevision = offer.Revision
		updated, _, err := earlyrental.Apply(offer, actor, command, at)
		if err != nil {
			t.Fatalf("Apply(%q) error = %v", command.Kind, err)
		}
		offer = updated
	}
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(time.Minute))
	apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, now.Add(2*time.Minute))
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, now.Add(3*time.Minute))
	apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, now.Add(4*time.Minute))
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: paymentID}, now.Add(5*time.Minute))
	switch targetState {
	case "enviado":
	case "recibido":
		apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandReceivePayment}, now.Add(6*time.Minute))
	case "anulado":
		apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}, now.Add(6*time.Minute))
	case "vencido":
		updated, _, changed, err := earlyrental.Expire(offer, offer.ExpiresAt)
		if err != nil || !changed {
			t.Fatalf("Expire() changed=%t error=%v", changed, err)
		}
		offer = updated
	default:
		t.Fatalf("unsupported test state %q", targetState)
	}
	return offer
}
