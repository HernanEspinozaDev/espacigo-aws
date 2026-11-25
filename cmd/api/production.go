package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devphone"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/fakebooking"
	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	bookingpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/booking"
	contractpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/contracts"
	conversationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/conversation"
	damageclaimpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/damageclaim"
	disputepg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/dispute"
	documentverificationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/documentverification"
	gallerypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/gallery"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	localnoticepg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/localnotice"
	marketplaceoauthpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/marketplaceoauth"
	mediapg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/media"
	occupancypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/occupancy"
	operationspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/operations"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/ownerexport"
	pricingpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/pricing"
	publiccatalogpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/publiccatalog"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	reputationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/reputation"
	spacespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaces"
	verificationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/verification"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/zohoauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/adminlocal"
	adminlocalhttp "github.com/HernanEspinozaDev/espaciGo/internal/adminlocal/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/booking"
	bookinghttp "github.com/HernanEspinozaDev/espaciGo/internal/booking/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/contract"
	contracthttp "github.com/HernanEspinozaDev/espaciGo/internal/contract/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/HernanEspinozaDev/espaciGo/internal/damageclaim"
	damageclaimhttp "github.com/HernanEspinozaDev/espaciGo/internal/damageclaim/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/dispute"
	disputehttp "github.com/HernanEspinozaDev/espaciGo/internal/dispute/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/gallery"
	galleryhttp "github.com/HernanEspinozaDev/espaciGo/internal/gallery/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	identityhttp "github.com/HernanEspinozaDev/espaciGo/internal/identity/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/identityverification"
	"github.com/HernanEspinozaDev/espaciGo/internal/localnotice"
	noticehttp "github.com/HernanEspinozaDev/espaciGo/internal/localnotice/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/m02local"
	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/media"
	"github.com/HernanEspinozaDev/espaciGo/internal/occupancy"
	"github.com/HernanEspinozaDev/espaciGo/internal/operation"
	operationhttp "github.com/HernanEspinozaDev/espaciGo/internal/operation/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
	"github.com/HernanEspinozaDev/espaciGo/internal/pricing"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	rentalflowhttp "github.com/HernanEspinozaDev/espaciGo/internal/rentalflow/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/reputation"
	reputationhttp "github.com/HernanEspinozaDev/espaciGo/internal/reputation/transport/http"
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

// Production exposes the public catalog, availability, host space management,
// profile/privacy, and identity/auth routes backed by Zoho SMTP.
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
	mailer, err := productionAuthMailer(cfg.frontendPublicOrigin, getenv)
	if err != nil {
		return nil, err
	}
	authService, err := mountProductionAuth(mux, pool, cfg, evidence, mediaStore, mailer, getenv)
	if err != nil {
		return nil, err
	}
	if err := mountKYBRoutes(mux, pool, authService, cfg, getenv, true); err != nil {
		return nil, errors.New("production KYB initialization failed")
	}
	marketplaceTokens, err := mountMarketplaceOAuth(mux, pool, authService, cfg, getenv)
	if err != nil {
		return nil, err
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
	calendarService, err := occupancy.NewService(occupancypg.New(pool), credentials.Generator{})
	if err != nil {
		return nil, errors.New("production calendar initialization failed")
	}
	pricingService, err := pricing.NewService(pricingpg.New(pool), calendarService, credentials.Generator{})
	if err != nil {
		return nil, errors.New("production pricing initialization failed")
	}
	spacesHandler := spaceshttp.NewHandlerWithPricing(authService, spaceService, cfg.allowedOrigins, calendarService, pricingService)
	mux.Handle("/api/v1/spaces", spacesHandler)
	mux.Handle("/api/v1/spaces/", spacesHandler)
	registerPublicSpaceMonthRoute(mux, spacesHandler)
	ownerWeeklyHoursService := booking.NewOwnerWeeklyHoursService(bookingpg.New(pool))
	ownerWeeklyHoursHandler := bookinghttp.NewOwnerWeeklyHoursHandler(authService, ownerWeeklyHoursService, cfg.allowedOrigins)
	registerOwnerWeeklyHoursRoutes(mux, ownerWeeklyHoursHandler)
	galleryService, err := gallery.NewService(gallerypg.New(pool), evidence, credentials.Generator{}, time.Now)
	if err != nil {
		return nil, errors.New("production gallery initialization failed")
	}
	mediaService, err := media.NewService(mediapg.New(pool), mediaStore, credentials.Generator{}, time.Now)
	if err != nil {
		return nil, errors.New("production media initialization failed")
	}
	galleryHandler := galleryhttp.NewHandlerWithMedia(authService, galleryService, mediaService, cfg.allowedOrigins)
	mux.Handle("/api/v1/spaces/{spaceID}/gallery", galleryHandler)
	mux.Handle("/api/v1/spaces/{spaceID}/cover", galleryHandler)
	mux.Handle("/api/v1/spaces/{spaceID}/gallery/{photoID}", galleryHandler)
	mux.Handle("/api/v1/spaces/{spaceID}/gallery/{photoID}/content", galleryHandler)
	mux.Handle("/api/v1/spaces/{spaceID}/gallery/{photoID}/variants/{variant}", galleryHandler)
	mux.Handle("/api/v1/spaces/{spaceID}/gallery/{photoID}/publication", galleryHandler)
	m02Service, err := m02local.New(pool, evidence, time.Now)
	if err != nil {
		return nil, errors.New("production account media initialization failed")
	}
	m02Handler := m02local.NewHandlerWithMedia(authService, m02Service, mediaService, cfg.allowedOrigins)
	mux.Handle("/api/v1/profile/photo", m02Handler)
	mux.Handle("/api/v1/profile/photo/", m02Handler)
	mux.Handle("/api/v1/payout-account", m02Handler)
	declarationKeys, err := loadRentalContractDeclarationKeyring(os.Getenv)
	if err != nil {
		return nil, errors.New("commercial rental identity initialization failed")
	}
	commercialQuoteRepo, err := rentalflowpg.NewRepositoryWithDeclarationKeyring(pool, pool, time.Now, declarationKeys)
	if err != nil {
		return nil, err
	}
	availability, err := rentalflow.NewAvailabilityService(commercialQuoteRepo, commercialQuoteRepo, time.Now)
	if err != nil {
		return nil, err
	}
	quoteService, err := rentalflow.NewQuoteService(commercialQuoteRepo, commercialQuoteRepo, commercialQuoteRepo, time.Now)
	if err != nil {
		return nil, err
	}
	identityVerifier, err := identityverification.NewService(documentverificationpg.New(pool))
	if err != nil {
		return nil, err
	}
	previewService, err := rentalflow.NewContractPreviewService(commercialQuoteRepo, commercialQuoteRepo, commercialQuoteRepo, identityVerifier, commercialQuoteRepo, time.Now)
	if err != nil {
		return nil, err
	}
	checkoutService, err := newCheckoutPreferenceService(commercialQuoteRepo, marketplaceTokens, cfg, os.Getenv)
	if err != nil {
		return nil, err
	}
	if checkoutService != nil {
		webhookHandler, _, webhookErr := newCheckoutWebhook(commercialQuoteRepo, pool, marketplaceTokens, os.Getenv)
		if webhookErr != nil {
			return nil, webhookErr
		}
		mux.Handle("/api/v1/webhooks/mercadopago", webhookHandler)
	}
	commercialReservationService, err := rentalflow.NewCommercialReservationService(commercialQuoteRepo)
	if err != nil {
		return nil, err
	}
	rentalHandler := rentalflowhttp.New(authService, availability, quoteService, previewService, cfg.allowedOrigins, checkoutService, commercialReservationService)
	if err := mountCommercialRentalFlowRoutes(mux, rentalHandler); err != nil {
		return nil, err
	}
	startCommercialExpirySweeper(ctx, commercialQuoteRepo)
	if err := mountProductionBookingFlow(mux, pool, authService, evidence, mediaStore, key, cfg, mailer); err != nil {
		return nil, err
	}
	return authService, nil
}

func startCommercialExpirySweeper(ctx context.Context, repo *rentalflowpg.Repository) {
	if repo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, _ = repo.PurgeExpiredCommercialQuotes(sweepCtx, time.Now(), 100)
				cancel()
			}
		}
	}()
}

func mountMarketplaceOAuth(mux *http.ServeMux, pool *pgxpool.Pool, auth marketplaceoauth.Authorizer, cfg config, getenv func(string) string) (*marketplaceoauth.Service, error) {
	settings, err := marketplaceoauth.ConfigFromEnv(getenv)
	if err != nil {
		return nil, errors.New("Mercado Pago OAuth configuration is incomplete or invalid")
	}
	var service *marketplaceoauth.Service
	if settings != nil {
		service, err = marketplaceoauth.New(*settings, marketplaceoauthpg.New(pool), &marketplaceoauth.HTTPExchanger{ClientID: settings.ClientID, ClientSecret: settings.ClientSecret, RedirectURI: settings.RedirectURI}, time.Now)
		if err != nil {
			return nil, errors.New("Mercado Pago OAuth initialization failed")
		}
	}
	handler := marketplaceoauth.NewHandler(auth, service, cfg.frontendPublicOrigin, cfg.allowedOrigins)
	handler.Mount(mux)
	return service, nil
}

func newCheckoutPreferenceService(repo *rentalflowpg.Repository, tokens *marketplaceoauth.Service, cfg config, getenv func(string) string) (*checkoutpro.Service, error) {
	return newCheckoutPreferenceServiceWithGateway(repo, tokens, cfg, getenv, &checkoutpro.HTTPGateway{})
}

func newCheckoutPreferenceServiceWithGateway(repo *rentalflowpg.Repository, tokens *marketplaceoauth.Service, cfg config, getenv func(string) string, gateway checkoutpro.Gateway) (*checkoutpro.Service, error) {
	if getenv == nil {
		return nil, errors.New("checkout configuration is unavailable")
	}
	walletPurchaseEnabled, err := mercadoPagoWalletPurchaseEnabled(getenv)
	if err != nil {
		return nil, errors.New("invalid Mercado Pago wallet purchase configuration")
	}
	enabled := strings.TrimSpace(getenv("MERCADOPAGO_CHECKOUT_ENABLED"))
	if enabled == "" || enabled == "0" {
		return nil, nil
	}
	if enabled != "1" {
		return nil, errors.New("MERCADOPAGO_CHECKOUT_ENABLED must be 1")
	}
	if repo == nil || tokens == nil {
		return nil, errors.New("Checkout Pro requires commercial quotes and configured seller OAuth")
	}
	settings, err := marketplaceoauth.ConfigFromEnv(getenv)
	if err != nil || settings == nil {
		return nil, errors.New("Checkout Pro OAuth configuration is unavailable")
	}
	if walletPurchaseEnabled && !settings.TestMode {
		return nil, errors.New("wallet purchase is only enabled in the test environment")
	}
	return checkoutpro.New(repo, tokens, gateway, checkoutpro.Config{
		FrontendOrigin:        strings.TrimSpace(getenv("MERCADOPAGO_RETURN_ORIGIN")),
		TestMode:              settings.TestMode,
		WalletPurchaseEnabled: walletPurchaseEnabled,
		Diagnostics: func(ctx context.Context, d checkoutpro.Diagnostic) {
			fields := []safelog.Field{safelog.F("stage", d.Stage), safelog.F("reason", d.Reason)}
			if d.HTTPStatus >= 400 && d.HTTPStatus <= 599 {
				fields = append(fields, safelog.F("provider_http_status", d.HTTPStatus))
			}
			if d.ProviderCode != "" {
				fields = append(fields, safelog.F("provider_code", d.ProviderCode))
			}
			safelog.Warn(ctx, "checkout preference start failed", fields...)
		},
	}, time.Now, func() string {
		id, e := (credentials.Generator{}).ID()
		if e != nil {
			return ""
		}
		return id
	})
}

type checkoutSellerTokenReader interface {
	AccessToken(context.Context, string) ([]byte, error)
}

func newCheckoutWebhook(repo *rentalflowpg.Repository, pool *pgxpool.Pool, tokens checkoutSellerTokenReader, getenv func(string) string) (http.Handler, *checkoutpro.PaymentWorker, error) {
	return newCheckoutWebhookWithProvider(repo, pool, tokens, getenv, &checkoutpro.HTTPGateway{})
}

func newCheckoutWebhookWithProvider(repo *rentalflowpg.Repository, pool *pgxpool.Pool, tokens checkoutSellerTokenReader, getenv func(string) string, provider checkoutpro.ProviderLookup) (http.Handler, *checkoutpro.PaymentWorker, error) {
	if repo == nil || pool == nil || tokens == nil || getenv == nil {
		return nil, nil, errors.New("Checkout Pro webhook dependencies are unavailable")
	}
	if provider == nil {
		return nil, nil, errors.New("Checkout Pro payment provider is unavailable")
	}
	secret, err := readMercadoPagoWebhookSecret(getenv)
	if err != nil {
		return nil, nil, err
	}
	defer clear(secret)
	credentials, err := marketplaceoauthpg.NewSellerCredentialResolver(pool, tokens)
	if err != nil {
		return nil, nil, errors.New("Mercado Pago webhook seller credentials are unavailable")
	}
	worker, err := checkoutpro.NewPaymentWorker(repo, credentials, provider, repo, time.Now)
	if err != nil {
		return nil, nil, errors.New("Mercado Pago webhook worker initialization failed")
	}
	receiver, err := checkoutpro.NewWebhookReceiver(secret, repo, time.Now, worker.Notify)
	if err != nil {
		return nil, nil, errors.New("Mercado Pago webhook receiver initialization failed")
	}
	return receiver, worker, nil
}

func readMercadoPagoWebhookSecret(getenv func(string) string) ([]byte, error) {
	if getenv == nil {
		return nil, errors.New("Mercado Pago webhook signature secret is unavailable")
	}
	if secretPath := strings.TrimSpace(getenv("MERCADOPAGO_WEBHOOK_SECRET_FILE")); secretPath != "" {
		absPath, err := filepath.Abs(secretPath)
		if err != nil || filepath.Clean(absPath) != absPath {
			return nil, errors.New("Mercado Pago webhook signature secret file is invalid")
		}
		info, err := os.Lstat(absPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 4096 {
			return nil, errors.New("Mercado Pago webhook signature secret file is invalid")
		}
		contents, err := os.ReadFile(absPath)
		if err != nil {
			return nil, errors.New("Mercado Pago webhook signature secret is unavailable")
		}
		defer clear(contents)
		trimmed := bytes.TrimSpace(contents)
		if len(trimmed) < 32 {
			return nil, errors.New("Mercado Pago webhook signature secret is invalid")
		}
		secret := make([]byte, len(trimmed))
		copy(secret, trimmed)
		return secret, nil
	}
	raw := strings.TrimSpace(getenv("MERCADOPAGO_WEBHOOK_SECRET"))
	if raw == "" {
		return nil, errors.New("Mercado Pago webhook signature secret is unavailable")
	}
	if len(raw) < 32 || len(raw) > 4096 {
		return nil, errors.New("Mercado Pago webhook signature secret is invalid")
	}
	return []byte(raw), nil
}

func mountProductionAuth(
	mux *http.ServeMux,
	pool *pgxpool.Pool,
	cfg config,
	evidence runtimeFileStore,
	mediaStore media.ObjectStore,
	mailer identity.AuthenticationMailer,
	getenv func(string) string,
) (*identity.AuthenticationService, error) {
	repo := identitypg.NewIdentityRepository(pool)
	service, err := identity.NewAuthenticationService(repo, password.Hasher{}, mailer, devauth.NewIPLimiter(20), credentials.Generator{}, time.Now)
	if err != nil {
		return nil, errors.New("production authentication initialization failed")
	}
	totpKeyring, err := identity.LoadTOTPKeyring(getenv)
	if err != nil {
		return nil, errors.New("production totp keyring initialization failed: " + err.Error())
	}
	service.SetTOTPKeyring(totpKeyring)
	archiveRepo := ownerexport.NewWithMediaStore(pool, evidence, mediaStore)
	privacyService, err := privacy.NewService(repo, archiveRepo)
	if err != nil {
		return nil, errors.New("production privacy initialization failed")
	}
	verificationRepo := verificationpg.New(pool)
	verificationService, err := verification.NewService(verificationRepo, credentials.Generator{}, verification.LocalFixtureProvider{}, time.Now)
	if err != nil {
		return nil, errors.New("production verification initialization failed")
	}
	baseVerificationHandler := verificationhttp.NewHandler(service, verificationService, cfg.allowedOrigins)
	quickVerificationHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/verifications" && r.Method == http.MethodPost {
			handleQuickVerification(w, r, service, verificationService, verificationRepo)
			return
		}
		baseVerificationHandler.ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/verifications", quickVerificationHandler)
	mux.Handle("/api/v1/verifications/", quickVerificationHandler)
	identityHandler := identityhttp.NewHandler(service, repo, cfg.allowedOrigins, privacyService)
	phoneSender := devphone.NewStubPhoneSender()
	phoneService := identity.NewPhoneVerificationService(repo, phoneSender, time.Now)
	identityhttp.AttachPhoneVerificationService(identityHandler, phoneService)
	mux.Handle("/api/v1/", identityHandler)
	return service, nil
}

type quickVerificationAuthorizer interface {
	Authorize(ctx context.Context, token identity.Secret, requiredRole identity.Role, activity identity.ActivityKind) (identity.Principal, error)
}

type quickVerificationReviewer interface {
	Review(ctx context.Context, id, reviewer string, approved bool, reason string, clock func() time.Time) (verification.Case, error)
}

func handleQuickVerification(
	w http.ResponseWriter,
	r *http.Request,
	auth quickVerificationAuthorizer,
	service *verification.Service,
	repo quickVerificationReviewer,
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
		} else {
			safelog.Error(r.Context(), "quick verification review error", reviewErr)
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

type productionNoticeSender interface {
	SendLocalBookingNotice(context.Context, string, string, string) error
}

func mountProductionBookingFlow(
	mux *http.ServeMux,
	pool *pgxpool.Pool,
	authService *identity.AuthenticationService,
	evidence runtimeFileStore,
	mediaStore media.ObjectStore,
	key []byte,
	cfg config,
	mailer identity.AuthenticationMailer,
) error {
	quoteTTL := 15 * time.Minute
	payTTL := 15 * time.Minute
	hostTTL := 24 * time.Hour
	now := time.Now

	paymentAdapter, err := fakebooking.NewWithStoreAndClock(key, bookingpg.New(pool), now)
	if err != nil {
		return errors.New("production booking payment initialization failed")
	}

	bookingService, err := booking.NewServiceWithTTLsAndGlobalExpirySweeps(
		bookingpg.New(pool), credentials.Generator{}, now, paymentAdapter, quoteTTL, payTTL, hostTTL, true,
	)
	if err != nil {
		return errors.New("production booking service initialization failed")
	}

	identityVerifier, err := identityverification.NewService(documentverificationpg.New(pool))
	if err != nil {
		return errors.New("production identity verifier initialization failed")
	}
	bookingService.SetBookingIdentityVerifier(identityVerifier)
	bookingService.SetLocalRefundAdapter(paymentAdapter)
	bookingService.SetLocalSettlementAdapter(paymentAdapter)
	if sender, ok := mailer.(productionNoticeSender); ok {
		bookingService.SetLocalNoticeSender(sender)
	}

	conversationService, err := conversation.NewService(conversationpg.New(pool), credentials.Generator{}, now)
	if err != nil {
		return errors.New("production conversation service initialization failed")
	}

	bookingHandler := bookinghttp.NewHandlerWithClock(authService, bookingService, cfg.allowedOrigins, now, conversationService)
	mux.Handle("/api/v1/local/booking-trial/", bookingHandler)
	mux.Handle("/api/v1/admin/local/reservations", bookingHandler)
	mux.Handle("/api/v1/admin/local/reservations/", bookingHandler)
	mux.Handle("/api/v1/admin/local/audit-events", bookingHandler)
	mux.Handle("/api/v1/admin/local/audit-events/export", bookingHandler)

	contractRepo, err := contractpg.New(pool, key)
	if err != nil {
		return errors.New("production contract repository initialization failed")
	}
	contractService, err := contract.NewService(contractRepo, now)
	if err != nil {
		return errors.New("production contract service initialization failed")
	}
	registerContractRoutes(mux, contracthttp.NewHandler(authService, contractService, cfg.allowedOrigins))

	operationService, err := operation.NewService(operationspg.New(pool), evidence, credentials.Generator{}, now)
	if err != nil {
		return errors.New("production operation service initialization failed")
	}
	registerOperationRoutes(mux, operationhttp.NewHandler(authService, operationService, cfg.allowedOrigins))

	claimService, err := damageclaim.NewWithEvidenceStore(damageclaimpg.New(pool), evidence, now)
	if err != nil {
		return errors.New("production damage claim service initialization failed")
	}
	registerDamageClaimRoutes(mux, damageclaimhttp.NewHandler(authService, claimService, cfg.allowedOrigins))

	disputeService, err := dispute.NewService(disputepg.New(pool), now)
	if err != nil {
		return errors.New("production dispute service initialization failed")
	}
	registerDisputeRoutes(mux, disputehttp.NewHandler(authService, disputeService, cfg.allowedOrigins))

	reputationService, err := reputation.New(reputationpg.New(pool), credentials.Generator{}, now)
	if err != nil {
		return errors.New("production reputation service initialization failed")
	}
	reputationHandler := reputationhttp.NewHandler(authService, reputationService, cfg.allowedOrigins)
	mux.Handle("GET /api/v1/spaces/{spaceID}/reviews", reputationHandler)
	mux.Handle("/api/v1/admin/local/review-reports", reputationHandler)
	mux.Handle("/api/v1/admin/local/review-reports/", reputationHandler)
	registerReputationReservationRoutes(mux, reputationHandler)
	mux.Handle("GET /api/v1/local/reputation/me", reputationHandler)

	adminLocalService, err := adminlocal.New(pool, now)
	if err != nil {
		return errors.New("production admin local service initialization failed")
	}
	adminLocalHandler := adminlocalhttp.NewHandler(authService, adminLocalService, cfg.allowedOrigins)
	mux.Handle("/api/v1/admin/local/accounts/", adminLocalHandler)
	mux.Handle("/api/v1/admin/local/reports/", adminLocalHandler)
	mux.Handle("/api/v1/admin/local/published-spaces", adminLocalHandler)
	mux.Handle("/api/v1/admin/local/published-spaces/", adminLocalHandler)

	if sender, ok := mailer.(productionNoticeSender); ok {
		localNoticeService, err := localnotice.NewWithFrontendOrigin(localnoticepg.New(pool), sender, now, cfg.frontendPublicOrigin)
		if err == nil {
			mux.Handle("/api/v1/admin/local/notices", noticehttp.NewHandler(authService, localNoticeService, cfg.allowedOrigins))
			mux.Handle("/api/v1/admin/local/notices/", noticehttp.NewHandler(authService, localNoticeService, cfg.allowedOrigins))
		}
	}

	return nil
}
