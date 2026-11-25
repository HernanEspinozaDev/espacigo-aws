package main

import (
	"net/http"

	contracttemplatespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/contracttemplates"
	contracttemplatehttp "github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountContractTemplates(mux *http.ServeMux, pool *pgxpool.Pool, auth contracttemplatehttp.Authenticator, origins []string) {
	h := contracttemplatehttp.New(auth, contracttemplatespg.New(pool), origins)
	for _, action := range []string{"draft", "publish", "versions", "copy"} {
		mux.Handle("/api/v1/spaces/{spaceID}/contract-template/"+action, h)
	}
}
