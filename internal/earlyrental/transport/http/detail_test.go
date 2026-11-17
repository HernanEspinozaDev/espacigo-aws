package earlyrentalhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func TestProjectEarlyRentalDetailEmitsParticipantWireView(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	projected, err := projectEarlyRentalDetail(offer, offer.RenterID, createdAt, createdAt, createdAt)
	if err != nil {
		t.Fatalf("projectEarlyRentalDetail() error = %v", err)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(body)
	for _, privateValue := range []string{offer.HostID, offer.RenterID} {
		if strings.Contains(wire, privateValue) {
			t.Fatalf("participant account ID leaked in detail: %s", wire)
		}
	}

	var decoded struct {
		Data struct {
			ID                    string    `json:"id"`
			FlowType              string    `json:"flow_type"`
			ActorRole             string    `json:"actor_role"`
			SpaceID               string    `json:"space_id"`
			SpaceTitle            string    `json:"space_title"`
			RateUnit              string    `json:"rate_unit"`
			UnitPriceCLP          int64     `json:"unit_price_clp"`
			Currency              string    `json:"currency"`
			Units                 int64     `json:"units"`
			SubtotalCLP           int64     `json:"subtotal_clp"`
			StartAt               time.Time `json:"start_at"`
			EndAt                 time.Time `json:"end_at"`
			TimeZone              string    `json:"time_zone"`
			State                 string    `json:"state"`
			Revision              int64     `json:"revision"`
			LastEventSequence     int64     `json:"last_event_sequence"`
			CreatedAt             time.Time `json:"created_at"`
			UpdatedAt             time.Time `json:"updated_at"`
			ExpiresAt             time.Time `json:"expires_at"`
			IdentityConfirmations struct {
				Renter *time.Time `json:"arrendatario"`
				Host   *time.Time `json:"arrendador"`
			} `json:"identity_confirmations"`
			Contract struct {
				Version     int64  `json:"version"`
				Digest      string `json:"digest"`
				TermsText   string `json:"terms_text"`
				Acceptances struct {
					Renter *time.Time `json:"arrendatario"`
					Host   *time.Time `json:"arrendador"`
				} `json:"acceptances"`
			} `json:"contract"`
			Payment        json.RawMessage `json:"payment"`
			AllowedActions []string        `json:"allowed_actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	data := decoded.Data
	if data.ID != offer.ID || data.FlowType != "manual_early_v1" || data.ActorRole != "arrendatario" ||
		data.SpaceID != offer.Snapshot.SpaceID.String() || data.SpaceTitle != offer.Snapshot.SpaceTitle ||
		data.RateUnit != "hora" || data.UnitPriceCLP != offer.Snapshot.UnitPriceCLP || data.Currency != "CLP" ||
		data.Units != offer.Snapshot.Units || data.SubtotalCLP != offer.Snapshot.SubtotalCLP ||
		!data.StartAt.Equal(offer.Snapshot.StartAt) || !data.EndAt.Equal(offer.Snapshot.EndAt) || data.TimeZone != offer.Snapshot.TimeZone ||
		data.State != string(earlyrental.StatePendingIdentity) || data.Revision != 1 || data.LastEventSequence != 1 ||
		!data.CreatedAt.Equal(createdAt) || !data.UpdatedAt.Equal(createdAt) || !data.ExpiresAt.Equal(offer.ExpiresAt) {
		t.Fatalf("detail wire does not match offer snapshot: %+v", data)
	}
	if data.IdentityConfirmations.Renter != nil || data.IdentityConfirmations.Host != nil ||
		data.Contract.Version != offer.ContractVersion || data.Contract.Digest != offer.ContractDigest || data.Contract.TermsText != offer.ContractText ||
		data.Contract.Acceptances.Renter != nil || data.Contract.Acceptances.Host != nil || string(data.Payment) != "null" {
		t.Fatalf("pending confirmation/contract/payment wire = %+v", data)
	}
	if len(data.AllowedActions) != 3 || data.AllowedActions[0] != string(earlyrental.CommandConfirmIdentity) {
		t.Fatalf("allowed_actions = %v", data.AllowedActions)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(body, &topLevel); err != nil {
		t.Fatal(err)
	}
	var detailKeys map[string]json.RawMessage
	if err := json.Unmarshal(topLevel["data"], &detailKeys); err != nil {
		t.Fatal(err)
	}
	if len(detailKeys) != 23 {
		t.Fatalf("detail has %d wire fields, want all 23 OpenAPI fields", len(detailKeys))
	}
}

func TestProjectEarlyRentalDetailReflectsSharedPaymentLifecycle(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	paymentID := "550e8400-e29b-41d4-a716-446655440000"
	apply := func(actor string, command earlyrental.Command, at time.Time) {
		t.Helper()
		command.ExpectedRevision = offer.Revision
		updated, _, err := earlyrental.Apply(offer, actor, command, at)
		if err != nil {
			t.Fatalf("Apply(%q) error = %v", command.Kind, err)
		}
		offer = updated
	}
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, createdAt.Add(time.Minute))
	apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, createdAt.Add(2*time.Minute))
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, createdAt.Add(3*time.Minute))
	apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, createdAt.Add(4*time.Minute))
	apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: paymentID}, createdAt.Add(5*time.Minute))

	projected, err := projectEarlyRentalDetail(offer, offer.RenterID, createdAt, createdAt.Add(5*time.Minute), createdAt.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("project sent payment: %v", err)
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Data struct {
			Payment *struct {
				ID         string     `json:"id"`
				AmountCLP  int64      `json:"amount_clp"`
				Currency   string     `json:"currency"`
				State      string     `json:"state"`
				SentAt     *time.Time `json:"sent_at"`
				ReceivedAt *time.Time `json:"received_at"`
			} `json:"payment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Data.Payment == nil || sent.Data.Payment.ID != paymentID || sent.Data.Payment.AmountCLP != offer.Snapshot.SubtotalCLP ||
		sent.Data.Payment.Currency != "CLP" || sent.Data.Payment.State != "enviado" || sent.Data.Payment.SentAt == nil || sent.Data.Payment.ReceivedAt != nil {
		t.Fatalf("sent payment projection = %+v", sent.Data.Payment)
	}

	apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandReceivePayment}, createdAt.Add(6*time.Minute))
	projected, err = projectEarlyRentalDetail(offer, offer.HostID, createdAt, createdAt.Add(6*time.Minute), createdAt.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("project received payment: %v", err)
	}
	body, err = json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Data.Payment == nil || sent.Data.Payment.ID != paymentID || sent.Data.Payment.State != "recibido" || sent.Data.Payment.ReceivedAt == nil {
		t.Fatalf("received payment projection = %+v", sent.Data.Payment)
	}
}

func TestProjectEarlyRentalDetailKeepsSentPaymentWithoutInventingReceipt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state string
	}{
		{name: "cancelled", state: "anulado"},
		{name: "expired", state: "vencido"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
			offer := newEarlyRentalDetailTestOffer(t, createdAt)
			paymentID := "550e8400-e29b-41d4-a716-446655440000"
			apply := func(actor string, command earlyrental.Command, at time.Time) {
				t.Helper()
				command.ExpectedRevision = offer.Revision
				updated, _, err := earlyrental.Apply(offer, actor, command, at)
				if err != nil {
					t.Fatalf("Apply(%q) error = %v", command.Kind, err)
				}
				offer = updated
			}
			apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, createdAt.Add(time.Minute))
			apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}, createdAt.Add(2*time.Minute))
			apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, createdAt.Add(3*time.Minute))
			apply(offer.HostID, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: offer.ContractVersion, ContractDigest: offer.ContractDigest}, createdAt.Add(4*time.Minute))
			apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: paymentID}, createdAt.Add(5*time.Minute))

			updatedAt := createdAt.Add(6 * time.Minute)
			if tc.name == "cancelled" {
				apply(offer.RenterID, earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}, updatedAt)
			} else {
				updated, _, changed, err := earlyrental.Expire(offer, offer.ExpiresAt)
				if err != nil || !changed {
					t.Fatalf("Expire() changed=%t error=%v", changed, err)
				}
				offer = updated
				updatedAt = offer.ExpiresAt
			}
			projected, err := projectEarlyRentalDetail(offer, offer.RenterID, createdAt, updatedAt, updatedAt)
			if err != nil {
				t.Fatalf("project terminal detail: %v", err)
			}
			body, err := json.Marshal(projected)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Data struct {
					Payment *struct {
						ID         string     `json:"id"`
						State      string     `json:"state"`
						SentAt     *time.Time `json:"sent_at"`
						ReceivedAt *time.Time `json:"received_at"`
					} `json:"payment"`
					AllowedActions []string `json:"allowed_actions"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.Payment == nil || result.Data.Payment.ID != paymentID || result.Data.Payment.State != tc.state ||
				result.Data.Payment.SentAt == nil || result.Data.Payment.ReceivedAt != nil || result.Data.AllowedActions == nil || len(result.Data.AllowedActions) != 0 {
				t.Fatalf("terminal payment projection = %+v", result.Data)
			}
		})
	}
}

func TestProjectEarlyRentalDetailRejectsNonParticipantAndInvalidReadMetadata(t *testing.T) {
	createdAt := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	offer := newEarlyRentalDetailTestOffer(t, createdAt)
	if _, err := projectEarlyRentalDetail(offer, "33333333-3333-4333-8333-333333333333", createdAt, createdAt, createdAt); err == nil {
		t.Fatal("non-participant detail projection unexpectedly succeeded")
	}
	if _, err := projectEarlyRentalDetail(offer, offer.RenterID, time.Time{}, createdAt, createdAt); err == nil {
		t.Fatal("zero creation timestamp unexpectedly succeeded")
	}
	if _, err := projectEarlyRentalDetail(offer, offer.RenterID, createdAt, createdAt.Add(-time.Second), createdAt); err == nil {
		t.Fatal("updated_at before created_at unexpectedly succeeded")
	}
}

func newEarlyRentalDetailTestOffer(t *testing.T, now time.Time) earlyrental.Offer {
	t.Helper()
	offer, _, err := earlyrental.NewOffer(earlyrental.NewOfferInput{
		ID: earlyRentalTestOfferID, HostID: earlyRentalTestHostID, RenterID: earlyRentalTestRenterID,
		Snapshot: earlyrental.OfferSnapshotInput{
			SpaceID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), SpaceTitle: "Espacio de prueba",
			RateUnit: "hora", UnitPriceCLP: 12_000, Units: 1,
			StartAt: now.Add(2 * time.Hour), EndAt: now.Add(3 * time.Hour),
			TimeZone: "UTC", CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent,
		},
		ContractVersion: 1, ContractText: earlyRentalTestContractText,
	}, now)
	if err != nil {
		t.Fatalf("NewOffer() error = %v", err)
	}
	return offer
}
