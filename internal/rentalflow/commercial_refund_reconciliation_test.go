package rentalflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type reconciliationRepoStub struct {
	*cancellationRepoStub
	externalCalls int
	externalErr   error
	externalOut   rentalflow.CommercialCancellation
}

func (r *reconciliationRepoStub) RecordExternalFullCommercialRefund(_ context.Context, actor, reservationID string, payment checkoutpro.VerifiedPayment, refund checkoutpro.RefundResult, _ time.Time) (rentalflow.CommercialCancellation, error) {
	r.externalCalls++
	if r.externalErr != nil {
		return rentalflow.CommercialCancellation{}, r.externalErr
	}
	out := r.externalOut
	if out.ReservationID == "" {
		out = r.operation
		out.ReservationID = reservationID
		out.HostID = actor
		out.Status = "reembolsada"
		out.RefundStatus = rentalflow.CommercialRefundApproved
		out.ProviderRefundID = refund.ID
		out.RefundPercent = 100
		out.RefundAmountCLP = refund.Amount
		out.RefundedAmountCLP = refund.Amount
		out.RefundableBalanceCLP = 0
		out.CanCancel = false
		out.Reason = ""
		_ = payment
	}
	return out, nil
}

type reconciliationGatewayStub struct {
	*cancellationGatewayStub
	paymentStatus string
	refundedCLP   *int64
}

func (g *reconciliationGatewayStub) LookupPayment(ctx context.Context, token []byte, paymentID string) (checkoutpro.VerifiedPayment, error) {
	p, err := g.cancellationGatewayStub.LookupPayment(ctx, token, paymentID)
	if err != nil {
		return checkoutpro.VerifiedPayment{}, err
	}
	if g.paymentStatus != "" {
		p.Status = g.paymentStatus
	}
	if g.refundedCLP != nil {
		p.Settlement = &checkoutpro.Settlement{
			PaymentStatus: p.Status,
			RefundedCLP:   g.refundedCLP,
		}
	}
	return p, nil
}

func TestReconcileExternalFullRefundSuccessAndReplay(t *testing.T) {
	const (
		hostID        = "33333333-3333-4333-8333-333333333333"
		reservationID = "22222222-2222-4222-8222-222222222222"
	)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	refunded := int64(12000)
	for _, tc := range []struct {
		name   string
		status string
		reason string
	}{
		{name: "paid reservation", status: "pagada"},
		{name: "reconciliation required refund without cancellation", status: "reconciliation_required", reason: "refund_without_cancellation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &reconciliationRepoStub{
				cancellationRepoStub: &cancellationRepoStub{
					operation: rentalflow.CommercialCancellation{
						ReservationID:                reservationID,
						HostID:                       hostID,
						CheckoutOperationID:          reservationID,
						ProviderPaymentID:            "1234567",
						SellerID:                     "91",
						TestMode:                     false,
						Status:                       tc.status,
						CheckoutReconciliationReason: tc.reason,
						TotalChargedCLP:              12000,
						StartsAt:                     now.Add(24 * time.Hour).Format(time.RFC3339Nano),
					},
				},
			}
			gw := &reconciliationGatewayStub{
				cancellationGatewayStub: &cancellationGatewayStub{
					verifiedAmount: 12000,
					refunds:        []checkoutpro.RefundResult{{ID: "9001", Status: "approved", Amount: 12000}},
				},
				paymentStatus: "refunded",
				refundedCLP:   &refunded,
			}
			svc, _ := rentalflow.NewCommercialReservationService(repo)
			svc.SetCancellationDependencies(repo, cancellationTokensStub{}, gw, func() time.Time { return now })

			got, err := svc.ReconcileExternalRefund(t.Context(), hostID, reservationID, "9001")
			if err != nil {
				t.Fatal(err)
			}
			if repo.externalCalls != 1 || got.Status != "reembolsada" || got.RefundStatus != rentalflow.CommercialRefundApproved || got.ProviderRefundID != "9001" {
				t.Fatalf("unexpected external reconciliation result: %+v calls=%d", got, repo.externalCalls)
			}

			// Replay when already approved with same refund ID skips provider calls.
			repo.operation = got
			gotReplay, err := svc.ReconcileExternalRefund(t.Context(), hostID, reservationID, "9001")
			if err != nil || repo.externalCalls != 1 || gotReplay.ProviderRefundID != "9001" {
				t.Fatalf("unexpected replay result: %+v err=%v calls=%d", gotReplay, err, repo.externalCalls)
			}
		})
	}
}

func TestReconcileExternalRefundRejectsMismatches(t *testing.T) {
	const (
		hostID        = "33333333-3333-4333-8333-333333333333"
		reservationID = "22222222-2222-4222-8222-222222222222"
	)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	full := int64(12000)
	partial := int64(6000)

	for _, tc := range []struct {
		name          string
		paymentStatus string
		refundedCLP   *int64
		refunds       []checkoutpro.RefundResult
		seller        string
		want          error
	}{
		{
			name:          "partial external refund cannot close as external full",
			paymentStatus: "refunded",
			refundedCLP:   &partial,
			refunds:       []checkoutpro.RefundResult{{ID: "9001", Status: "approved", Amount: 6000}},
			want:          rentalflow.ErrRefundRequiresReconciliation,
		},
		{
			name:          "multiple refunds in listing",
			paymentStatus: "refunded",
			refundedCLP:   &full,
			refunds: []checkoutpro.RefundResult{
				{ID: "9001", Status: "approved", Amount: 6000},
				{ID: "9002", Status: "approved", Amount: 6000},
			},
			want: rentalflow.ErrRefundRequiresReconciliation,
		},
		{
			name:          "pending refund in listing",
			paymentStatus: "refunded",
			refundedCLP:   &full,
			refunds:       []checkoutpro.RefundResult{{ID: "9001", Status: "in_process", Amount: 12000}},
			want:          rentalflow.ErrRefundRequiresReconciliation,
		},
		{
			name:          "settlement refunded mismatch",
			paymentStatus: "refunded",
			refundedCLP:   &partial,
			refunds:       []checkoutpro.RefundResult{{ID: "9001", Status: "approved", Amount: 12000}},
			want:          rentalflow.ErrRefundRequiresReconciliation,
		},
		{
			name:          "charged_back payment",
			paymentStatus: "charged_back",
			refunds:       []checkoutpro.RefundResult{{ID: "9001", Status: "approved", Amount: 12000}},
			want:          rentalflow.ErrRefundRequiresReconciliation,
		},
		{
			name:          "seller mismatch",
			paymentStatus: "refunded",
			seller:        "92",
			refunds:       []checkoutpro.RefundResult{{ID: "9001", Status: "approved", Amount: 12000}},
			want:          rentalflow.ErrRefundRequiresReconciliation,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &reconciliationRepoStub{
				cancellationRepoStub: &cancellationRepoStub{
					operation: rentalflow.CommercialCancellation{
						ReservationID:       reservationID,
						HostID:              hostID,
						CheckoutOperationID: reservationID,
						ProviderPaymentID:   "1234567",
						SellerID:            "91",
						Status:              "pagada",
						TotalChargedCLP:     12000,
						StartsAt:            now.Add(24 * time.Hour).Format(time.RFC3339Nano),
					},
				},
			}
			gw := &reconciliationGatewayStub{
				cancellationGatewayStub: &cancellationGatewayStub{
					verifiedAmount: 12000,
					paymentSeller:  tc.seller,
					refunds:        tc.refunds,
				},
				paymentStatus: tc.paymentStatus,
				refundedCLP:   tc.refundedCLP,
			}
			svc, _ := rentalflow.NewCommercialReservationService(repo)
			svc.SetCancellationDependencies(repo, cancellationTokensStub{}, gw, func() time.Time { return now })

			_, err := svc.ReconcileExternalRefund(t.Context(), hostID, reservationID, "9001")
			if !errors.Is(err, tc.want) || repo.externalCalls != 0 {
				t.Fatalf("got err=%v calls=%d, want %v", err, repo.externalCalls, tc.want)
			}
		})
	}
}

func TestReconcileExternalRefundSupportsPendingRemainderRefund(t *testing.T) {
	const (
		hostID        = "33333333-3333-4333-8333-333333333333"
		reservationID = "22222222-2222-4222-8222-222222222222"
	)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	totalRefunded := int64(10000)

	repo := &reconciliationRepoStub{
		cancellationRepoStub: &cancellationRepoStub{
			operation: rentalflow.CommercialCancellation{
				ReservationID:        reservationID,
				HostID:               hostID,
				CheckoutOperationID:  reservationID,
				ProviderPaymentID:    "1234567",
				SellerID:             "91",
				Status:               "pagada",
				RefundStatus:         rentalflow.CommercialRefundPending,
				IdempotencyKey:       "44444444-4444-4444-8444-444444444444",
				TotalChargedCLP:      10000,
				RefundedAmountCLP:    2000,
				RefundableBalanceCLP: 8000,
				RefundAmountCLP:      8000,
				StartsAt:             now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
		},
	}
	gw := &reconciliationGatewayStub{
		cancellationGatewayStub: &cancellationGatewayStub{
			verifiedAmount: 10000,
			refunds: []checkoutpro.RefundResult{
				{ID: "9000", Status: "approved", Amount: 2000},
				{ID: "9001", Status: "approved", Amount: 8000},
			},
		},
		paymentStatus: "refunded",
		refundedCLP:   &totalRefunded,
	}
	svc, _ := rentalflow.NewCommercialReservationService(repo)
	svc.SetCancellationDependencies(repo, cancellationTokensStub{}, gw, func() time.Time { return now })

	got, err := svc.ReconcileExternalRefund(t.Context(), hostID, reservationID, "9001")
	if err != nil {
		t.Fatal(err)
	}
	if repo.status != rentalflow.CommercialRefundApproved || got.ProviderRefundID != "9001" {
		t.Fatalf("unexpected remainder reconciliation: %+v status=%q", got, repo.status)
	}

	// Mismatched settlement total on a pending remainder refund must fail reconciliation.
	wrongTotal := int64(9000)
	gw.refundedCLP = &wrongTotal
	repoMismatch := &reconciliationRepoStub{
		cancellationRepoStub: &cancellationRepoStub{
			operation: rentalflow.CommercialCancellation{
				ReservationID:        reservationID,
				HostID:               hostID,
				CheckoutOperationID:  reservationID,
				ProviderPaymentID:    "1234567",
				SellerID:             "91",
				Status:               "pagada",
				RefundStatus:         rentalflow.CommercialRefundPending,
				IdempotencyKey:       "44444444-4444-4444-8444-444444444444",
				TotalChargedCLP:      10000,
				RefundedAmountCLP:    2000,
				RefundableBalanceCLP: 8000,
				RefundAmountCLP:      8000,
				StartsAt:             now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
		},
	}
	svcMismatch, _ := rentalflow.NewCommercialReservationService(repoMismatch)
	svcMismatch.SetCancellationDependencies(repoMismatch, cancellationTokensStub{}, gw, func() time.Time { return now })
	if _, err := svcMismatch.ReconcileExternalRefund(t.Context(), hostID, reservationID, "9001"); !errors.Is(err, rentalflow.ErrRefundRequiresReconciliation) {
		t.Fatalf("expected ErrRefundRequiresReconciliation on mismatched settlement total, got %v", err)
	}
}
