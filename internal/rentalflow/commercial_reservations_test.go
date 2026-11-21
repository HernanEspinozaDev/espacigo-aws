package rentalflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type mockCommercialReservationReader struct {
	listFn func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error)
	getFn  func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error)
}

func (m *mockCommercialReservationReader) ListCommercialReservations(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
	if m.listFn != nil {
		return m.listFn(ctx, actor, pageSize, cursor)
	}
	return rentalflow.CommercialReservationPage{}, nil
}

func (m *mockCommercialReservationReader) GetCommercialReservation(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
	if m.getFn != nil {
		return m.getFn(ctx, actor, reservationID)
	}
	return rentalflow.CommercialReservation{}, nil
}

func TestNewCommercialReservationService(t *testing.T) {
	_, err := rentalflow.NewCommercialReservationService(nil)
	if !errors.Is(err, rentalflow.ErrInvalid) {
		t.Fatalf("expected ErrInvalid when reader is nil, got %v", err)
	}
	reader := &mockCommercialReservationReader{}
	svc, err := rentalflow.NewCommercialReservationService(reader)
	if err != nil || svc == nil {
		t.Fatalf("expected service instance, got %v", err)
	}
}

func TestCommercialReservationService_List(t *testing.T) {
	ctx := context.Background()
	validActor := "11111111-1111-4111-8111-111111111111"

	t.Run("invalid inputs", func(t *testing.T) {
		reader := &mockCommercialReservationReader{}
		svc, err := rentalflow.NewCommercialReservationService(reader)
		if err != nil {
			t.Fatal(err)
		}

		// nil service
		var nilSvc *rentalflow.CommercialReservationService
		if _, err := nilSvc.List(ctx, validActor, 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on nil service, got %v", err)
		}

		// nil ctx
		if _, err := svc.List(nil, validActor, 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on nil ctx, got %v", err)
		}

		// invalid participantID
		if _, err := svc.List(ctx, "invalid-uuid", 20, ""); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on invalid participantID, got %v", err)
		}

		// pageSize out of bounds
		if _, err := svc.List(ctx, validActor, 0, ""); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on pageSize 0, got %v", err)
		}
		if _, err := svc.List(ctx, validActor, 101, ""); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on pageSize 101, got %v", err)
		}

		// cursor too long
		longCursor := make([]byte, 4097)
		if _, err := svc.List(ctx, validActor, 20, string(longCursor)); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on long cursor, got %v", err)
		}
	})

	t.Run("reader error mapping", func(t *testing.T) {
		reader := &mockCommercialReservationReader{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				if cursor == "bad-cursor" {
					return rentalflow.CommercialReservationPage{}, rentalflow.ErrInvalid
				}
				if cursor == "not-found" {
					return rentalflow.CommercialReservationPage{}, rentalflow.ErrNotFound
				}
				return rentalflow.CommercialReservationPage{}, errors.New("db connection failure")
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)

		if _, err := svc.List(ctx, validActor, 20, "bad-cursor"); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid, got %v", err)
		}
		if _, err := svc.List(ctx, validActor, 20, "not-found"); !errors.Is(err, rentalflow.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
		if _, err := svc.List(ctx, validActor, 20, "other"); !errors.Is(err, rentalflow.ErrReservationUnavailable) {
			t.Errorf("expected ErrReservationUnavailable, got %v", err)
		}
	})

	t.Run("success with nil items converted to empty slice", func(t *testing.T) {
		reader := &mockCommercialReservationReader{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				return rentalflow.CommercialReservationPage{Items: nil, NextCursor: ""}, nil
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)
		page, err := svc.List(ctx, validActor, 20, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if page.Items == nil || len(page.Items) != 0 {
			t.Errorf("expected non-nil empty slice, got %#v", page.Items)
		}
	})

	t.Run("success with items", func(t *testing.T) {
		expectedItems := []rentalflow.CommercialReservation{
			{ID: "22222222-2222-4222-8222-222222222222", Status: "pendiente_de_pago"},
		}
		reader := &mockCommercialReservationReader{
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				return rentalflow.CommercialReservationPage{Items: expectedItems, NextCursor: "token123"}, nil
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)
		page, err := svc.List(ctx, validActor, 20, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page.Items) != 1 || page.NextCursor != "token123" {
			t.Errorf("unexpected page: %+v", page)
		}
	})
}

func TestCommercialReservationService_Get(t *testing.T) {
	ctx := context.Background()
	validActor := "11111111-1111-4111-8111-111111111111"
	validReservation := "22222222-2222-4222-8222-222222222222"

	t.Run("invalid inputs", func(t *testing.T) {
		reader := &mockCommercialReservationReader{}
		svc, _ := rentalflow.NewCommercialReservationService(reader)

		var nilSvc *rentalflow.CommercialReservationService
		if _, err := nilSvc.Get(ctx, validActor, validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on nil service, got %v", err)
		}
		if _, err := svc.Get(nil, validActor, validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on nil ctx, got %v", err)
		}
		if _, err := svc.Get(ctx, "invalid", validReservation); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on invalid actor, got %v", err)
		}
		if _, err := svc.Get(ctx, validActor, "invalid"); !errors.Is(err, rentalflow.ErrInvalid) {
			t.Errorf("expected ErrInvalid on invalid reservation, got %v", err)
		}
	})

	t.Run("error mapping", func(t *testing.T) {
		reader := &mockCommercialReservationReader{
			getFn: func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
				if reservationID == "33333333-3333-4333-8333-333333333333" {
					return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
				}
				return rentalflow.CommercialReservation{}, errors.New("db timeout")
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)

		if _, err := svc.Get(ctx, validActor, "33333333-3333-4333-8333-333333333333"); !errors.Is(err, rentalflow.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
		if _, err := svc.Get(ctx, validActor, validReservation); !errors.Is(err, rentalflow.ErrReservationUnavailable) {
			t.Errorf("expected ErrReservationUnavailable, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		expected := rentalflow.CommercialReservation{
			ID:              validReservation,
			SpaceTitle:      "Oficina Centro",
			ParticipantRole: "arrendatario",
			Status:          "pagada",
		}
		reader := &mockCommercialReservationReader{
			getFn: func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
				return expected, nil
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)
		got, err := svc.Get(ctx, validActor, validReservation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != expected.ID || got.Status != expected.Status {
			t.Errorf("expected %+v, got %+v", expected, got)
		}
	})

	t.Run("reconciles pending reservation before returning", func(t *testing.T) {
		quoteID := "44444444-4444-4444-8444-444444444444"
		currentState := "pendiente_de_pago"
		currentCheckout := "pending"
		reconciledQuotes := []string{}
		reader := &mockCommercialReservationReader{
			getFn: func(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
				return rentalflow.CommercialReservation{
					ID:             validReservation,
					QuoteID:        quoteID,
					Status:         currentState,
					CheckoutStatus: currentCheckout,
				}, nil
			},
			listFn: func(ctx context.Context, actor string, pageSize int, cursor string) (rentalflow.CommercialReservationPage, error) {
				return rentalflow.CommercialReservationPage{
					Items: []rentalflow.CommercialReservation{{
						ID:             validReservation,
						QuoteID:        quoteID,
						Status:         currentState,
						CheckoutStatus: currentCheckout,
					}},
				}, nil
			},
		}
		svc, _ := rentalflow.NewCommercialReservationService(reader)
		svc.SetReconciler(func(ctx context.Context, actor, qid string) {
			reconciledQuotes = append(reconciledQuotes, qid)
			currentState = "pagada"
			currentCheckout = "approved"
		})
		got, err := svc.Get(ctx, validActor, validReservation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reconciledQuotes) != 1 || reconciledQuotes[0] != quoteID || got.Status != "pagada" || got.CheckoutStatus != "approved" {
			t.Fatalf("expected reconciliation on Get, got quotes=%v item=%+v", reconciledQuotes, got)
		}

		currentState = "pendiente_de_pago"
		currentCheckout = "pending"
		page, err := svc.List(ctx, validActor, 20, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reconciledQuotes) != 2 || len(page.Items) != 1 || page.Items[0].Status != "pagada" {
			t.Fatalf("expected reconciliation on List, got quotes=%v page=%+v", reconciledQuotes, page)
		}
	})
}
