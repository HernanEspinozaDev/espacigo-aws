package main

import (
	"net/http"

	commercialreservationdetailspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialreservationdetails"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialreservationdetail"
	commercialreservationdetailhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialreservationdetail/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The caller supplies the same private contracts-prefixed store and bucket as
// the PDF worker. No object key or public presigned URL is accepted from HTTP.
func mountCommercialReservationDocuments(mux *http.ServeMux, pool *pgxpool.Pool, auth commercialreservationdetailhttp.Authenticator, store commercialreservationdetail.PrivateStore, bucket string, origins []string) error {
	service, err := commercialreservationdetail.New(commercialreservationdetailspg.New(pool), store, bucket)
	if err != nil {
		return err
	}
	handler := commercialreservationdetailhttp.New(auth, service, origins)
	mux.Handle("/api/v1/commercial-reservations/{reservationID}/documents", handler)
	mux.Handle("/api/v1/commercial-reservations/{reservationID}/contract-document", handler)
	return nil
}
