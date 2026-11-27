package main

import (
	"net/http"
	"time"

	organizationspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/organizations"
	"github.com/HernanEspinozaDev/espaciGo/internal/organizations"
	organizationshttp "github.com/HernanEspinozaDev/espaciGo/internal/organizations/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountOrganizationRoutes(mux *http.ServeMux, pool *pgxpool.Pool, auth organizationshttp.Authenticator, keys *verification.RutHMACKeyring) (*organizations.Service, error) {
	service, err := organizations.New(organizationspg.New(pool), keys, time.Now)
	if err != nil {
		return nil, err
	}
	handler := organizationshttp.NewHandler(auth, service)
	mux.Handle("/api/v1/host/organizations", handler)
	mux.Handle("/api/v1/host/organizations/", handler)
	mux.Handle("/api/v1/host/organization-join-requests", handler)
	mux.Handle("/api/v1/host/organization-join-requests/", handler)
	return service, nil
}
