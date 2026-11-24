package main

import (
	"net/http"
	"time"

	commercialrentaldocumentspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialrentaldocuments"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialrentaldocument"
	commercialrentaldocumenthttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialrentaldocument/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountCommercialRentalTaxDocumentRoutes(mux *http.ServeMux, pool *pgxpool.Pool, auth commercialrentaldocumenthttp.Authenticator, taxStore commercialrentaldocument.PrivateStore, validator commercialrentaldocument.Validator, bucket string, origins []string) error {
	service, err := commercialrentaldocument.New(commercialrentaldocumentspg.New(pool), taxStore, validator, bucket, time.Now)
	if err != nil {
		return err
	}
	handler := commercialrentaldocumenthttp.New(auth, service, origins)
	mux.Handle("/api/v1/commercial-reservations/{reservationID}/rental-tax-document", handler)
	return nil
}
