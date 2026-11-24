package main

import (
	"context"
	"net/http"
	"time"

	commercialagreementspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialagreements"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	commercialagreementhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Call from the commercial-flow assembly and append the returned workers to the
// runtime worker registry. Both workers stop with the supplied runtime context.
// Sender matches localNoticeSender (devauth/Zoho), not AuthenticationMailer's
// narrower interface; it resolves no transport credentials in this module.
func mountCommercialAgreementRoutes(trackedWorkers *trackedWorkerGroup, mux *http.ServeMux, pool *pgxpool.Pool, auth commercialagreementhttp.Authenticator, gate rentalflow.IdentityGate, sender commercialagreement.Sender, origins []string) ([]namedWorker, error) {
	repo, err := commercialagreementspg.New(pool, gate, time.Now)
	if err != nil {
		return nil, err
	}
	service, err := commercialagreement.New(repo)
	if err != nil {
		return nil, err
	}
	service.ConfigureOTP(repo, sender, true)
	notices, err := commercialagreement.NewNoticeDispatcher(repo, sender, time.Now)
	if err != nil {
		return nil, err
	}
	handler := commercialagreementhttp.New(auth, service, origins)
	mux.Handle("/api/v1/commercial-agreements", handler)
	mux.Handle("/api/v1/commercial-agreements/", handler)
	return []namedWorker{
		{"commercial-agreement-expiration", func() {
			trackedWorkers.Go(func(workerCtx context.Context) { service.RunExpiration(workerCtx, time.Second) })
		}},
		{"commercial-agreement-notice-delivery", func() { trackedWorkers.Go(func(workerCtx context.Context) { notices.Run(workerCtx, time.Second) }) }},
	}, nil
}
