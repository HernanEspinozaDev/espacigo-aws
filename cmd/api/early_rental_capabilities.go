package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/fakebooking"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	earlyrentalhttp "github.com/HernanEspinozaDev/espaciGo/internal/earlyrental/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const localEarlyRentalCapabilitiesPath = "/api/v1/local/early-rentals/capabilities"

func localEarlyRentalActivationRequested(getenv func(string) string) (bool, error) {
	if getenv == nil {
		return false, errors.New("early rental flag source unavailable")
	}
	switch getenv("LOCAL_EARLY_RENTAL_FLOW_ENABLED") {
	case "", "0":
		return false, nil
	case "1":
		if !localEarlyRentalRuntime(getenv) {
			return false, errors.New("early rental activation is restricted to the local authenticated runtime")
		}
		return true, nil
	default:
		return false, errors.New("invalid early rental activation flag")
	}
}

func localEarlyRentalRuntime(getenv func(string) string) bool {
	if getenv == nil || getenv("APP_PROFILE") != "" || getenv("LOCAL_AUTH_PROTOTYPE") != "1" {
		return false
	}
	runtimeProfile := strings.TrimSpace(getenv("API_RUNTIME_PROFILE"))
	return runtimeProfile == "" || runtimeProfile == "standard"
}

func registerLocalEarlyRentalCapabilitiesRoute(mux *http.ServeMux, auth earlyrentalhttp.Authenticator, activationRequested, ready bool, origins []string) error {
	if mux == nil || auth == nil {
		return errors.New("early rental capabilities dependencies unavailable")
	}
	handler := earlyrentalhttp.NewCapabilitiesHandler(auth, earlyrentalhttp.CapabilityState{
		ActivationRequested: activationRequested,
		FlowReady:           ready,
	}, origins)
	mux.Handle("GET "+localEarlyRentalCapabilitiesPath, handler)
	mux.Handle("OPTIONS "+localEarlyRentalCapabilitiesPath, handler)
	return nil
}

func registerLocalEarlyRental(mux *http.ServeMux, auth earlyrentalhttp.Authenticator, repo *rentalflowpg.Repository, pool *pgxpool.Pool, requested bool, origins []string) error {
	var ready bool
	if requested {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM pg_catalog.pg_attribute
			WHERE attrelid=to_regclass('public.arriendo_temprano_evento')
			  AND attname='procesador_referencia' AND NOT attisdropped
		)`).Scan(&ready); err != nil {
			return errors.New("local early rental database readiness check failed")
		}
		if ready {
			key, err := localPaymentEventKey()
			if err != nil {
				return err
			}
			processor, err := fakebooking.New(key)
			if err != nil {
				return errors.New("local early rental payment processor initialization failed")
			}
			earlyrentalhttp.RegisterDemo(mux, auth, repo, processor, origins)
		} else {
			log.Print("local early rental requires migration V100")
		}
	}
	return registerLocalEarlyRentalCapabilitiesRoute(mux, auth, requested, ready, origins)
}

var _ earlyrentalhttp.Authenticator = (*identity.AuthenticationService)(nil)
