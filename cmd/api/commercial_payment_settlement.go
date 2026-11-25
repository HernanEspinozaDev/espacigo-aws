package main

import (
	"context"
	"log"
	"time"

	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

func runCommercialPaymentSettlement(ctx context.Context, repo *rentalflowpg.Repository, tokens checkoutSellerTokenReader) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	provider := &checkoutpro.HTTPGateway{}
	for {
		candidates, err := repo.ClaimSettlementCandidates(ctx)
		if err != nil && ctx.Err() == nil {
			log.Print("commercial payment settlement polling deferred")
		}
		for _, candidate := range candidates {
			if ctx.Err() != nil {
				return
			}
			readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			token, err := tokens.AccessToken(readCtx, candidate.HostID)
			if err == nil {
				payment, lookupErr := provider.LookupPayment(readCtx, token, candidate.PaymentID)
				if lookupErr == nil {
					_ = repo.SyncPaymentSettlement(readCtx, candidate, payment)
				}
			}
			clear(token)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
