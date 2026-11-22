package main

import (
	"net/http"
	"time"

	commercialauthorizationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialauthorization"
	commercialproposalspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialproposals"
	commercialauthorizationhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialauthorization/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mountCommercialAuthorization exposes owner-only opt-in, revocation, history,
// and a deterministic limit check. It deliberately does not enable auto-booking.
func mountCommercialAuthorization(mux *http.ServeMux, pool *pgxpool.Pool, auth commercialauthorizationhttp.Authenticator, origins []string) {
	h := commercialauthorizationhttp.New(auth, commercialauthorizationpg.New(pool), commercialproposalspg.New(pool), origins, time.Now)
	for _, action := range []string{"current", "history", "activate", "revoke", "evaluate"} {
		mux.Handle("/api/v1/spaces/{spaceID}/commercial-authorization/"+action, h)
	}
}
