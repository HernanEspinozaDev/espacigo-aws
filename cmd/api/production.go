package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	publiccatalogpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/publiccatalog"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	spacespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaces"
	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	rentalflowhttp "github.com/HernanEspinozaDev/espaciGo/internal/rentalflow/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	spaceshttp "github.com/HernanEspinozaDev/espaciGo/internal/spaces/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

const productionAppProfile = "production"

func validateProductionRuntime(getenv func(string) string, profile runtimeProfileName) error {
	if profile != runtimeProfileStandard {
		return errors.New("production requires the standard API runtime")
	}
	for _, name := range []string{
		"LOCAL_AUTH_PROTOTYPE", "LOCAL_BOOKING_TRIAL", "LOCAL_EARLY_RENTAL_FLOW_ENABLED",
		"LOCAL_PUBLIC_CATALOG_TRIAL", "COMMERCIAL_RENTAL_FLOW_ENABLED",
		"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3", "AWS_CA_BUNDLE",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE",
	} {
		if getenv(name) != "" {
			return fmt.Errorf("production rejects %s", name)
		}
	}
	host, _, err := net.SplitHostPort(getenv("HTTP_ADDR"))
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("production Go API requires a loopback HTTP_ADDR")
	}
	if getenv("DATABASE_SSLMODE") != "verify-full" || strings.TrimSpace(getenv("DATABASE_SSLROOTCERT")) == "" {
		return errors.New("production requires PostgreSQL certificate and hostname verification")
	}
	if getenv("DATABASE_USER") != "espacigo_api_login" || getenv("DATABASE_SET_ROLE") != "espacigo_runtime" {
		return errors.New("production requires the PostgreSQL runtime login and role")
	}
	origin, err := url.Parse(getenv("FRONTEND_PUBLIC_ORIGIN"))
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" {
		return errors.New("production requires an HTTPS frontend origin")
	}
	if getenv("BLOB_STORE_BACKEND") != "s3" || strings.TrimSpace(getenv("AWS_REGION")) == "" {
		return errors.New("production requires private S3 storage and an explicit AWS_REGION")
	}
	return nil
}

// Production exposes the public catalog and availability. The local startup
// mounts the rehearsal routes separately, including synthetic verification.
func mountProductionCatalog(ctx context.Context, mux *http.ServeMux, pool *pgxpool.Pool, cfg config, getenv func(string) string) error {
	key, err := productionCursorKey(getenv)
	if err != nil {
		return errors.New("production catalog requires a private cursor key file")
	}
	evidence, err := newRuntimeFileStore(ctx, getenv, "")
	if err != nil {
		return errors.New("production catalog evidence storage initialization failed")
	}
	mediaStore, err := newRuntimeMediaStore(ctx, getenv, "")
	if err != nil {
		return errors.New("production catalog media storage initialization failed")
	}
	catalog, err := publiccatalog.NewWithImageAndMediaStore(publiccatalogpg.New(pool), key, evidence, mediaStore)
	if err != nil {
		return errors.New("production public catalog initialization failed")
	}
	if err := mountPublicCatalogRoutes(mux, publiccataloghttp.New(catalog, cfg.allowedOrigins...), true, false, false); err != nil {
		return err
	}
	spaceService, err := spaces.NewService(spacespg.New(pool, credentials.Generator{}))
	if err != nil {
		return err
	}
	registerPublicSpaceMonthRoute(mux, spaceshttp.NewHandler(nil, spaceService, cfg.allowedOrigins))
	availabilityRepo, err := rentalflowpg.NewRepository(pool, nil, time.Now)
	if err != nil {
		return err
	}
	availability, err := rentalflow.NewAvailabilityService(availabilityRepo, availabilityRepo, time.Now)
	if err != nil {
		return err
	}
	return mountPublicRentalAvailabilityRoute(mux, rentalflowhttp.New(nil, availability, nil, nil, cfg.allowedOrigins))
}

func productionCursorKey(getenv func(string) string) ([]byte, error) {
	path := getenv("PUBLIC_CATALOG_CURSOR_KEY_FILE")
	injected := getenv("PUBLIC_CATALOG_ECS_CURSOR_KEY")
	if injected == "" {
		return readPublicCatalogCursorKey(path)
	}
	key, err := hex.DecodeString(injected)
	if path != "" || err != nil || len(key) != 32 {
		return nil, errors.New("invalid ECS catalog cursor key configuration")
	}
	return key, nil
}
