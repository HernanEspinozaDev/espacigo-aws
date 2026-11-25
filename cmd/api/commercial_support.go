package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialsupport"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountCommercialSupport(mux *http.ServeMux, pool *pgxpool.Pool, files runtimeFileStore, user *identity.AuthenticationService, admin *identity.AdminSessionService, origins []string) {
	service := commercialsupport.New(pool, files, time.Now)
	participantRoutes := map[string]string{
		"GET /api/v1/commercial-reservations/{reservationID}/post-sale-summary":                            "summary",
		"GET /api/v1/commercial-reservations/{reservationID}/support-cases":                                "list",
		"POST /api/v1/commercial-reservations/{reservationID}/support-cases":                               "open",
		"POST /api/v1/commercial-reservations/{reservationID}/support-cases/{caseID}/actions":              "action",
		"POST /api/v1/commercial-reservations/{reservationID}/support-cases/{caseID}/evidence":             "attach",
		"GET /api/v1/commercial-reservations/{reservationID}/support-cases/{caseID}/evidence/{evidenceID}": "download",
	}
	adminRoutes := map[string]string{
		"GET /api/v1/admin/global/reservation-support-cases":                                "list",
		"POST /api/v1/admin/global/reservation-support-cases/{caseID}/actions":              "action",
		"GET /api/v1/admin/global/reservation-support-cases/{caseID}/evidence/{evidenceID}": "download",
	}
	userAuth := func(ctx context.Context, token identity.Secret) (identity.Principal, error) {
		return user.Authorize(ctx, token, "", identity.UserOperation)
	}
	adminAuth := func(ctx context.Context, token identity.Secret) (identity.Principal, error) {
		return admin.AuthorizeCapability(ctx, token, identity.CapabilitySupportTicketsManage, identity.UserOperation)
	}
	for pattern, operation := range participantRoutes {
		mux.HandleFunc(pattern, supportHandler(service, userAuth, operation, false, origins))
	}
	for pattern, operation := range adminRoutes {
		mux.HandleFunc(pattern, supportHandler(service, adminAuth, operation, true, origins))
	}
	preflight := supportHandler(service, userAuth, "", false, origins)
	preflightRoutes := map[string]bool{}
	for _, routes := range []map[string]string{participantRoutes, adminRoutes} {
		for pattern := range routes {
			_, path, _ := strings.Cut(pattern, " ")
			if !preflightRoutes[path] {
				mux.HandleFunc("OPTIONS "+path, preflight)
				preflightRoutes[path] = true
			}
		}
	}
}

type supportAuth func(context.Context, identity.Secret) (identity.Principal, error)

func supportHandler(service *commercialsupport.Service, authorize supportAuth, operation string, admin bool, origins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Origin")
		requestID, _ := (credentials.Generator{}).ID()
		w.Header().Set("X-Request-ID", requestID)
		if origin := r.Header.Get("Origin"); origin != "" {
			allowed := false
			for _, candidate := range origins {
				if candidate == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				supportError(w, http.StatusForbidden, "origin_denied", requestID)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Disposition")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			supportUnauthorized(w, requestID)
			return
		}
		ctx := r.Context()
		principal, err := authorize(ctx, identity.Secret(token))
		if err != nil {
			if errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrRestrictedMode) {
				supportError(w, http.StatusForbidden, "forbidden", requestID)
				return
			}
			supportUnauthorized(w, requestID)
			return
		}
		var payload any
		reservationID, caseID := r.PathValue("reservationID"), r.PathValue("caseID")
		key := r.Header.Get("Idempotency-Key")
		switch operation {
		case "summary":
			payload, err = service.Summary(ctx, reservationID, principal.AccountID)
		case "list":
			payload, err = service.List(ctx, reservationID, principal.AccountID, admin)
		case "open":
			var input struct {
				Category    string `json:"category"`
				Description string `json:"description"`
			}
			if !decodeSupport(w, r, &input, 16<<10) {
				supportError(w, 422, "invalid_request", requestID)
				return
			}
			payload, err = service.Open(ctx, reservationID, principal.AccountID, input.Category, input.Description, key)
		case "action":
			var input struct {
				Action      string `json:"action"`
				Description string `json:"description"`
			}
			if !decodeSupport(w, r, &input, 16<<10) {
				supportError(w, 422, "invalid_request", requestID)
				return
			}
			payload, err = service.Act(ctx, reservationID, caseID, principal.AccountID, input.Action, input.Description, key, admin)
		case "attach":
			var input struct {
				Content []byte `json:"content"`
			}
			if !decodeSupport(w, r, &input, 7<<20) {
				supportError(w, 422, "invalid_request", requestID)
				return
			}
			payload, err = service.Attach(ctx, reservationID, caseID, principal.AccountID, input.Content)
		case "download":
			var item commercialsupport.Evidence
			var data []byte
			item, data, err = service.Evidence(ctx, reservationID, caseID, r.PathValue("evidenceID"), principal.AccountID, admin)
			if err == nil {
				extension := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}[item.MIME]
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", `attachment; filename="evidencia-`+item.ID+extension+`"`)
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
				_, _ = w.Write(data)
				return
			}
		}
		switch {
		case errors.Is(err, commercialsupport.ErrInvalid):
			supportError(w, 422, "invalid_request", requestID)
		case errors.Is(err, commercialsupport.ErrNotFound):
			supportError(w, 404, "not_found", requestID)
		case errors.Is(err, commercialsupport.ErrConflict):
			supportError(w, 409, "support_action_unavailable", requestID)
		case err != nil:
			supportError(w, 503, "support_unavailable", requestID)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
		}
	}
}

func decodeSupport(w http.ResponseWriter, r *http.Request, output any, max int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	decoder.DisallowUnknownFields()
	return decoder.Decode(output) == nil && decoder.Decode(new(any)) == io.EOF
}

func supportUnauthorized(w http.ResponseWriter, requestID string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	supportError(w, 401, "unauthenticated", requestID)
}

func supportError(w http.ResponseWriter, status int, code, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": http.StatusText(status), "request_id": requestID}})
}
