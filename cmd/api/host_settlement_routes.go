package main

import (
	"net/http"

	hostsettlementspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/hostsettlements"
	"github.com/HernanEspinozaDev/espaciGo/internal/hostsettlement"
	hostsettlementhttp "github.com/HernanEspinozaDev/espaciGo/internal/hostsettlement/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountHostSettlements(mux *http.ServeMux, pool *pgxpool.Pool, auth hostsettlementhttp.Authenticator, origins []string) {
	h := hostsettlementhttp.New(auth, hostsettlement.New(hostsettlementspg.New(pool)), origins)
	mux.Handle("/api/v1/host/settlements", h)
	mux.Handle("/api/v1/host/settlements/{reservationID}", h)
}
