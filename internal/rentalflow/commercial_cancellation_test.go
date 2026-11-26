package rentalflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type cancellationRepoStub struct {
	operation rentalflow.CommercialCancellation
	status    string
	failed    bool
}

func (r *cancellationRepoStub) ListCommercialReservations(context.Context, string, int, string) (rentalflow.CommercialReservationPage, error) {
	return rentalflow.CommercialReservationPage{}, nil
}
func (r *cancellationRepoStub) GetCommercialReservation(context.Context, string, string) (rentalflow.CommercialReservation, error) {
	return rentalflow.CommercialReservation{}, nil
}

func (r *cancellationRepoStub) PreviewCommercialCancellation(context.Context, string, string, time.Time) (rentalflow.CommercialCancellation, error) {
	return r.operation, nil
}
func (r *cancellationRepoStub) BeginCommercialCancellation(_ context.Context, actor, reservation, reason string, _ int64, now time.Time, id string) (rentalflow.CommercialCancellation, error) {
	r.operation.ActorID = actor
	r.operation.ReservationID = reservation
	r.operation.CheckoutOperationID = reservation
	r.operation.SellerID = "91"
	return r.operation, nil
}
func (r *cancellationRepoStub) FinishCommercialCancellation(_ context.Context, _ string, _, refundID, status string, _ time.Time) (rentalflow.CommercialCancellation, error) {
	r.status = status
	r.operation.RefundStatus = status
	r.operation.ProviderRefundID = refundID
	return r.operation, nil
}
func (r *cancellationRepoStub) FailCommercialCancellation(context.Context, string, string, string, time.Time) error {
	r.failed = true
	return nil
}
func (r *cancellationRepoStub) ListPendingCommercialCancellations(context.Context, int) ([]rentalflow.CommercialCancellation, error) {
	return nil, nil
}

type cancellationTokensStub struct{}

func (cancellationTokensStub) AccessToken(context.Context, string) ([]byte, error) {
	return []byte("seller-oauth"), nil
}

type cancellationGatewayStub struct {
	key             string
	amount          int64
	amountWasNil    bool
	responseAmount  int64
	posts           int
	err             error
	status          string
	refunds         []checkoutpro.RefundResult
	historyErr      error
	verifiedAmount  int64
	paymentCurrency string
	paymentSeller   string
}

func (g *cancellationGatewayStub) LookupPayment(_ context.Context, _ []byte, paymentID string) (checkoutpro.VerifiedPayment, error) {
	currency := g.paymentCurrency
	if currency == "" {
		currency = "CLP"
	}
	seller := g.paymentSeller
	if seller == "" {
		seller = "91"
	}
	return checkoutpro.VerifiedPayment{
		SellerID: seller, ProviderPaymentID: paymentID, OperationID: "22222222-2222-4222-8222-222222222222",
		ExternalReference: "22222222-2222-4222-8222-222222222222", LiveMode: true,
		Currency: currency, TransactionAmountCLP: g.verifiedAmount, Status: "approved",
	}, nil
}

func (g *cancellationGatewayStub) RefundPayment(_ context.Context, token []byte, payment string, amount *int64, key string) (checkoutpro.RefundResult, error) {
	g.posts++
	g.key = key
	g.amountWasNil = amount == nil
	if amount != nil {
		g.amount = *amount
	}
	if g.err != nil {
		return checkoutpro.RefundResult{}, g.err
	}
	responseAmount := g.responseAmount
	if responseAmount == 0 {
		responseAmount = g.amount
	}
	return checkoutpro.RefundResult{ID: "7654321", Status: g.status, Amount: responseAmount}, nil
}

func TestCommercialFullRefundOmitsAmountAndVerifiesProviderTotal(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 5000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{status: "approved", responseAmount: 5000, verifiedAmount: 5000}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	if _, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000); err != nil {
		t.Fatal(err)
	}
	if gateway.posts != 1 || !gateway.amountWasNil || gateway.amount != 0 {
		t.Fatalf("full refund request must omit amount: gateway=%+v", gateway)
	}
}

func TestCommercialCancellationWithKnownRefundIDDoesNotRepostWhenListingIsBehind(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", ProviderRefundID: "refund-42", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 10000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{status: "approved", verifiedAmount: 10000}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	got, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000)
	if err != nil || got.RefundStatus != "pendiente" || gateway.posts != 0 || repo.status != "" {
		t.Fatalf("known refund should remain pending without re-POST: got=%+v err=%v gateway=%+v repo=%+v", got, err, gateway, repo)
	}
}
func (g *cancellationGatewayStub) ListRefunds(context.Context, []byte, string) ([]checkoutpro.RefundResult, error) {
	return g.refunds, g.historyErr
}

func TestCommercialCancellationUsesPersistedIdempotencyAndSellerToken(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 10000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{status: "approved", verifiedAmount: 10000}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) })
	got, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.key != repo.operation.IdempotencyKey || gateway.amount != 5000 || gateway.amountWasNil || got.RefundStatus != "aprobado" || repo.status != "aprobado" {
		t.Fatalf("result=%+v gateway=%+v repo=%+v", got, gateway, repo)
	}
}

func TestCommercialCancellationRecordsProviderRejectionWithoutFinalizingReservation(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{
		CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333",
		ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 1036, RefundAmountCLP: 1036,
	}}
	gateway := &cancellationGatewayStub{verifiedAmount: 1036, err: &checkoutpro.ProviderError{Stage: "refund_create", HTTPStatus: 400, ProviderCode: "bad_request"}}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	_, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 1036)
	if !errors.Is(err, rentalflow.ErrCancellationUnavailable) || !repo.failed || repo.status != "" || gateway.posts != 1 {
		t.Fatalf("refund error=%v failed=%t finalized_status=%q posts=%d", err, repo.failed, repo.status, gateway.posts)
	}
}

func TestCommercialCancellationLeavesUncertainProviderOutcomePending(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 10000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{verifiedAmount: 10000, err: errors.New("timeout")}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	_, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000)
	if !errors.Is(err, rentalflow.ErrCancellationUnavailable) || repo.failed || repo.status != "" {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
}

func TestCommercialRefundDoesNotPostWhenHistoryRequiresReview(t *testing.T) {
	for _, tc := range []struct {
		name      string
		history   []checkoutpro.RefundResult
		lookupErr error
		currency  string
		seller    string
		want      error
	}{
		{name: "existing partial refund", history: []checkoutpro.RefundResult{{ID: "56", Status: "approved", Amount: 500}}, want: rentalflow.ErrRefundRequiresReconciliation},
		{name: "existing pending refund", history: []checkoutpro.RefundResult{{ID: "56", Status: "in_process", Amount: 1036}}, want: rentalflow.ErrRefundRequiresReconciliation},
		{name: "different seller", seller: "92", want: rentalflow.ErrRefundRequiresReconciliation},
		{name: "different currency", currency: "USD", want: rentalflow.ErrRefundRequiresReconciliation},
		{name: "inaccessible history", lookupErr: &checkoutpro.ProviderError{HTTPStatus: 404}, want: rentalflow.ErrCancellationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 1036, RefundAmountCLP: 1036}}
			gateway := &cancellationGatewayStub{refunds: tc.history, historyErr: tc.lookupErr, verifiedAmount: 1036, paymentCurrency: tc.currency, paymentSeller: tc.seller}
			service, _ := rentalflow.NewCommercialReservationService(repo)
			service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
			_, err := service.Cancel(t.Context(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio", 1036)
			if !errors.Is(err, tc.want) || gateway.posts != 0 || repo.failed || repo.status != "" {
				t.Fatalf("history sent or finalized a refund: error=%v posts=%d", err, gateway.posts)
			}
		})
	}
}
