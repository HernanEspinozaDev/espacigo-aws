package main

import (
	"context"
	"log"
	"net/http"
	"time"

	clamavadapter "github.com/HernanEspinozaDev/espaciGo/internal/adapters/clamav"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialrentaldocument"
	commercialreservationdetailhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialreservationdetail/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/documentverification"
	"github.com/HernanEspinozaDev/espaciGo/internal/documentverification/inspection"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountLocalRentalTaxDocuments(ctx context.Context, mux *http.ServeMux, pool *pgxpool.Pool, auth commercialreservationdetailhttp.Authenticator, origins []string, getenv func(string) string) error {
	store, bucket, err := localCommercialObjectStore(ctx, getenv, "rental-tax-documents")
	if err != nil {
		return err
	}
	if store == nil {
		return mountCommercialRentalTaxDocumentRoutes(mux, pool, auth, nil, nil, "", origins)
	}
	return mountCommercialRentalTaxDocumentRoutes(mux, pool, auth, store, newRentalTaxPDFValidator(getenv), bucket, origins)
}

type rentalTaxPDFValidator struct {
	inspector *inspection.Inspector
	scanner   *clamavadapter.Adapter
}

func newRentalTaxPDFValidator(getenv func(string) string) commercialrentaldocument.Validator {
	scanConfig, enabled, err := documentScanConfigFromEnv(getenv)
	if err != nil {
		log.Print("rental tax documents: invalid scanner configuration")
	}
	if err != nil || !enabled {
		return nil
	}
	scanner, err := clamavadapter.New(clamavadapter.Config{
		Address: scanConfig.address, Timeout: 10 * time.Second,
		MaxBytes: commercialrentaldocument.MaxPDFBytes, MaxSignatureAge: scanConfig.maxSignatureAge,
	})
	if err != nil {
		log.Print("rental tax documents: scanner initialization failed")
		return nil
	}
	inspector := inspection.New(inspection.Config{PDFInfoPath: "/usr/bin/pdfinfo", PDFDetachPath: "/usr/bin/pdfdetach"})
	if inspector == nil {
		log.Print("rental tax documents: trusted Poppler tools unavailable")
		return nil
	}
	return rentalTaxPDFValidator{inspector: inspector, scanner: scanner}
}

func (v rentalTaxPDFValidator) Validate(ctx context.Context, content []byte) error {
	if _, _, err := v.inspector.Inspect(documentverification.PurposeSpace, "rental-tax-document", content); err != nil {
		return commercialrentaldocument.ErrInvalid
	}
	state, err := v.scanner.Scan(ctx, content)
	if err != nil {
		return commercialrentaldocument.ErrInspectionUnavailable
	}
	if state != documentverification.FileClean {
		return commercialrentaldocument.ErrInvalid
	}
	return nil
}
