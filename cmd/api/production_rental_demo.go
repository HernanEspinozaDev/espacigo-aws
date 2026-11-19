package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/fakebooking"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	earlyrentalhttp "github.com/HernanEspinozaDev/espaciGo/internal/earlyrental/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type productionDemoAuth struct {
	earlyrentalhttp.Authenticator
	scope earlyrentalhttp.DemoScope
}

func (a productionDemoAuth) Authorize(ctx context.Context, secret identity.Secret, role identity.Role, activity identity.ActivityKind) (identity.Principal, error) {
	principal, err := a.Authenticator.Authorize(ctx, secret, role, activity)
	if err == nil && principal.AccountID != a.scope.Host.String() && principal.AccountID != a.scope.Renter.String() {
		return identity.Principal{}, identity.ErrForbidden
	}
	return principal, err
}

func mountProductionRentalDemo(mux *http.ServeMux, pool *pgxpool.Pool, auth earlyrentalhttp.Authenticator, origins []string, getenv func(string) string) error {
	setting := getenv("RENTAL_DEMO_ENABLED")
	if setting == "" || setting == "0" {
		return nil
	}
	if setting != "1" {
		return errors.New("invalid hosted rental demo flag")
	}

	scope := earlyrentalhttp.DemoScope{}
	var err error
	scope.Space, err = uuid.Parse(getenv("RENTAL_DEMO_SPACE_ID"))
	hostEmail := strings.TrimSpace(strings.ToLower(getenv("RENTAL_DEMO_HOST_EMAIL")))
	renterEmail := strings.TrimSpace(strings.ToLower(getenv("RENTAL_DEMO_RENTER_EMAIL")))
	if err != nil || scope.Space == uuid.Nil || hostEmail == "" || renterEmail == "" || hostEmail == renterEmail {
		return errors.New("hosted rental demo requires one space and two distinct accounts")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = pool.QueryRow(ctx, `SELECT h.id, r.id
		FROM public.espacio e
		JOIN public.usuario h ON h.id = e.propietario_id AND h.estado='activo'
		JOIN public.usuario r ON r.correo_normalizado = $3 AND r.estado='activo'
		WHERE e.id = $1 AND h.correo_normalizado = $2 AND h.id <> r.id
		AND EXISTS (SELECT 1 FROM pg_catalog.pg_attribute
			WHERE attrelid = to_regclass('public.arriendo_temprano_evento')
			AND attname = 'procesador_referencia' AND NOT attisdropped)`,
		scope.Space, hostEmail, renterEmail).Scan(&scope.Host, &scope.Renter)
	if err != nil {
		return errors.New("hosted rental demo could not validate configured participants, space and schema")
	}
	key, err := productionCursorKey(getenv)
	if localEarlyRentalRuntime(getenv) {
		key, err = readPublicCatalogCursorKey(getenv("LOCAL_PUBLIC_CATALOG_CURSOR_KEY_FILE"))
	}
	if err != nil {
		return err
	}
	derive := func(purpose string) []byte {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(purpose))
		return mac.Sum(nil)
	}
	processor, err := fakebooking.New(derive("espacigo-hosted-rental-demo-payment"))
	if err != nil {
		return err
	}
	cursor, err := earlyrentalhttp.NewEarlyRentalCursorCodec(derive("espacigo-hosted-rental-demo-cursor"))
	if err != nil {
		return err
	}
	repo, err := rentalflowpg.NewRepository(pool, pool, time.Now)
	if err != nil {
		return err
	}
	scopedAuth := productionDemoAuth{Authenticator: auth, scope: scope}
	demo := http.NewServeMux()
	if err := registerLocalEarlyRentalCapabilitiesRoute(demo, scopedAuth, true, true, origins); err != nil {
		return err
	}
	earlyrentalhttp.RegisterScopedDemo(demo, scopedAuth, repo, processor, cursor, origins, scope)

	mux.Handle("/api/v1/local/early-rentals", demo)
	mux.Handle("/api/v1/local/early-rentals/", demo)
	return nil
}
