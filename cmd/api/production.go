package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devauth"
	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	publiccatalogpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/publiccatalog"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	spacespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaces"
	verificationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/verification"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/zohoauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	identityhttp "github.com/HernanEspinozaDev/espaciGo/internal/identity/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	rentalflowhttp "github.com/HernanEspinozaDev/espaciGo/internal/rentalflow/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	spaceshttp "github.com/HernanEspinozaDev/espaciGo/internal/spaces/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	verificationhttp "github.com/HernanEspinozaDev/espaciGo/internal/verification/transport/http"
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

// Production exposes the public catalog, availability, and identity/auth
// routes backed by Zoho SMTP. The local startup mounts rehearsal routes separately.
func mountProductionCatalog(ctx context.Context, mux *http.ServeMux, pool *pgxpool.Pool, cfg config, getenv func(string) string) (*identity.AuthenticationService, error) {
	key, err := productionCursorKey(getenv)
	if err != nil {
		return nil, errors.New("production catalog requires a private cursor key file")
	}
	evidence, err := newRuntimeFileStore(ctx, getenv, "")
	if err != nil {
		return nil, errors.New("production catalog evidence storage initialization failed")
	}
	mediaStore, err := newRuntimeMediaStore(ctx, getenv, "")
	if err != nil {
		return nil, errors.New("production catalog media storage initialization failed")
	}
	catalog, err := publiccatalog.NewWithImageAndMediaStore(publiccatalogpg.New(pool), key, evidence, mediaStore)
	if err != nil {
		return nil, errors.New("production public catalog initialization failed")
	}
	if err := mountPublicCatalogRoutes(mux, publiccataloghttp.New(catalog, cfg.allowedOrigins...), true, false, false); err != nil {
		return nil, err
	}
	spaceService, err := spaces.NewService(spacespg.New(pool, credentials.Generator{}))
	if err != nil {
		return nil, err
	}
	registerPublicSpaceMonthRoute(mux, spaceshttp.NewHandler(nil, spaceService, cfg.allowedOrigins))
	availabilityRepo, err := rentalflowpg.NewRepository(pool, nil, time.Now)
	if err != nil {
		return nil, err
	}
	availability, err := rentalflow.NewAvailabilityService(availabilityRepo, availabilityRepo, time.Now)
	if err != nil {
		return nil, err
	}
	if err := mountPublicRentalAvailabilityRoute(mux, rentalflowhttp.New(nil, availability, nil, nil, cfg.allowedOrigins)); err != nil {
		return nil, err
	}
	return mountProductionAuth(mux, pool, cfg, getenv)
}

func mountProductionAuth(mux *http.ServeMux, pool *pgxpool.Pool, cfg config, getenv func(string) string) (*identity.AuthenticationService, error) {
	mailer, err := productionAuthMailer(cfg.frontendPublicOrigin, getenv)
	if err != nil {
		return nil, err
	}
	repo := identitypg.NewIdentityRepository(pool)
	service, err := identity.NewAuthenticationService(repo, password.Hasher{}, mailer, devauth.NewIPLimiter(20), credentials.Generator{}, time.Now)
	if err != nil {
		return nil, errors.New("production authentication initialization failed")
	}
	verificationRepo := verificationpg.New(pool)
	verificationService, err := verification.NewService(verificationRepo, credentials.Generator{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		return nil, errors.New("production verification initialization failed")
	}
	baseVerificationHandler := verificationhttp.NewHandler(service, verificationService, cfg.allowedOrigins)
	quickVerificationHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/verifications" && r.Method == http.MethodPost {
			handleProductionQuickVerification(w, r, service, verificationService, verificationRepo)
			return
		}
		baseVerificationHandler.ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/verifications", quickVerificationHandler)
	mux.Handle("/api/v1/verifications/", quickVerificationHandler)
	mux.Handle("/api/v1/", identityhttp.NewHandler(service, repo, cfg.allowedOrigins))
	return service, nil
}

func handleProductionQuickVerification(
	w http.ResponseWriter,
	r *http.Request,
	auth *identity.AuthenticationService,
	service *verification.Service,
	repo *verificationpg.Repository,
) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "unauthenticated", "message": "Credencial ausente, incorrecta o expirada."}})
		return
	}
	principal, err := auth.Authorize(r.Context(), identity.Secret(strings.TrimSpace(parts[1])), "", identity.UserOperation)
	if err != nil {
		status := http.StatusUnauthorized
		code := "unauthenticated"
		if errors.Is(err, identity.ErrForbidden) {
			status = http.StatusForbidden
			code = "forbidden"
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": "No fue posible autorizar la solicitud."}})
		return
	}
	var in struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "invalid_json", "message": "Solicitud inválida."}})
		return
	}
	item, err := service.Start(r.Context(), principal.AccountID, in.Type, r.Header.Get("Idempotency-Key"))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "invalid_value", "message": "No fue posible iniciar la verificación."}})
		return
	}
	if item.State == "en_revision" {
		if approved, reviewErr := repo.Review(r.Context(), item.ID, principal.AccountID, true, "", time.Now); reviewErr == nil {
			item = approved
		}
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(item)
}

func productionAuthMailer(publicOrigin string, getenv func(string) string) (identity.AuthenticationMailer, error) {
	host := strings.TrimSpace(getenv("AUTH_SMTP_HOST"))
	if host == "" {
		return zohoauth.UnavailableMailer{}, nil
	}
	return zohoauth.New(zohoauth.Config{
		Host:         host,
		Port:         strings.TrimSpace(getenv("AUTH_SMTP_PORT")),
		Security:     strings.TrimSpace(getenv("AUTH_SMTP_SECURITY")),
		Username:     strings.TrimSpace(getenv("AUTH_SMTP_USERNAME")),
		Password:     strings.TrimSpace(getenv("AUTH_SMTP_ECS_PASSWORD")),
		PasswordFile: strings.TrimSpace(getenv("AUTH_SMTP_PASSWORD_FILE")),
		From:         strings.TrimSpace(getenv("AUTH_SMTP_FROM")),
		FromName:     "EspaciGo",
		PublicOrigin: publicOrigin,
	})
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
