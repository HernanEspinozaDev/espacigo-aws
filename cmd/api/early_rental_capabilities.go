package main

import (
	"errors"
	"net/http"
	"strings"

	earlyrentalhttp "github.com/HernanEspinozaDev/espaciGo/internal/earlyrental/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
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

func registerLocalEarlyRentalCapabilitiesRoute(mux *http.ServeMux, auth earlyrentalhttp.Authenticator, activationRequested bool, origins []string) error {
	if mux == nil || auth == nil {
		return errors.New("early rental capabilities dependencies unavailable")
	}
	handler := earlyrentalhttp.NewCapabilitiesHandler(auth, earlyrentalhttp.CapabilityState{
		ActivationRequested: activationRequested,
		FlowReady:           false,
	}, origins)
	mux.Handle("GET "+localEarlyRentalCapabilitiesPath, handler)
	mux.Handle("OPTIONS "+localEarlyRentalCapabilitiesPath, handler)
	return nil
}

var _ earlyrentalhttp.Authenticator = (*identity.AuthenticationService)(nil)
