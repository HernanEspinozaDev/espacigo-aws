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
	return r.operation, nil
}
func (r *cancellationRepoStub) FinishCommercialCancellation(_ context.Context, _ string, refundID, status string, _ time.Time) (rentalflow.CommercialCancellation, error) {
	r.status = status
	r.operation.RefundStatus = status
	r.operation.ProviderPaymentID = refundID
	return r.operation, nil
}
func (r *cancellationRepoStub) FailCommercialCancellation(context.Context, string, time.Time) error {
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
	key            string
	amount         int64
	amountWasNil   bool
	responseAmount int64
	posts          int
	err            error
	status         string
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
	gateway := &cancellationGatewayStub{status: "approved", responseAmount: 5000}
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
	gateway := &cancellationGatewayStub{status: "approved"}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	got, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000)
	if err != nil || got.RefundStatus != "pendiente" || gateway.posts != 0 || repo.status != "pendiente" {
		t.Fatalf("known refund should remain pending without re-POST: got=%+v err=%v gateway=%+v repo=%+v", got, err, gateway, repo)
	}
}
func (g *cancellationGatewayStub) ListRefunds(context.Context, []byte, string) ([]checkoutpro.RefundResult, error) {
	return nil, nil
}

func TestCommercialCancellationUsesPersistedIdempotencyAndSellerToken(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 10000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{status: "approved"}
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

func TestCommercialCancellationLeavesUncertainProviderOutcomePending(t *testing.T) {
	repo := &cancellationRepoStub{operation: rentalflow.CommercialCancellation{CanCancel: true, ReservationID: "22222222-2222-4222-8222-222222222222", HostID: "33333333-3333-4333-8333-333333333333", ProviderPaymentID: "1234567", IdempotencyKey: "44444444-4444-4444-8444-444444444444", TotalChargedCLP: 10000, RefundAmountCLP: 5000}}
	gateway := &cancellationGatewayStub{err: errors.New("timeout")}
	service, _ := rentalflow.NewCommercialReservationService(repo)
	service.SetCancellationDependencies(repo, cancellationTokensStub{}, gateway, time.Now)
	_, err := service.Cancel(context.Background(), "55555555-5555-4555-8555-555555555555", repo.operation.ReservationID, "Cambio de planes", 5000)
	if !errors.Is(err, rentalflow.ErrCancellationUnavailable) || repo.failed || repo.status != "" {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
}
