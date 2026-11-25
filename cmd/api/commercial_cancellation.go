package main

import (
	"context"
	"log"
	"time"

	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

// configureCommercialCancellation wires a durable repository and the connected
// seller's OAuth token into the commercial reservation service.
func configureCommercialCancellation(service *rentalflow.CommercialReservationService, repository *rentalflowpg.Repository, tokens *marketplaceoauth.Service) {
	if service == nil || repository == nil || tokens == nil {
		return
	}
	service.SetCancellationDependencies(repository, tokens, &checkoutpro.HTTPGateway{}, time.Now)
}

func runCommercialCancellationReconciler(ctx context.Context, service *rentalflow.CommercialReservationService) {
	if ctx == nil || service == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			if err := service.ReconcilePendingCancellations(ctx, 20); err != nil && ctx.Err() == nil {
				log.Print("commercial cancellation reconciliation deferred")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
