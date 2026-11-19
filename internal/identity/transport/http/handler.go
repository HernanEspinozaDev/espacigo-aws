package identityhttp

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type Handler struct {
	service                 *identity.AuthenticationService
	terms                   identity.AuthenticationRepository
	origins                 map[string]bool
	privacy                 *privacy.Service
	phoneService            *identity.PhoneVerificationService
	evidenceCleaner         privacy.SyntheticEvidenceCleaner
	suppressionRegistryPath string
	suppressionReplayStore  privacy.SuppressionReplayStore
	now                     func() time.Time
}

func (h *Handler) SetPhoneVerificationService(s *identity.PhoneVerificationService) {
	h.phoneService = s
}

func AttachPhoneVerificationService(target http.Handler, s *identity.PhoneVerificationService) {
	if h, ok := target.(*Handler); ok {
		h.phoneService = s
	}
}


func NewHandler(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, privacyServices ...*privacy.Service) http.Handler {
	return newHandler(service, terms, origins, nil, privacyServices...)
}

func NewHandlerWithSuppressionCleaner(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, privacyService *privacy.Service, cleaner privacy.SyntheticEvidenceCleaner) http.Handler {
	return newHandler(service, terms, origins, cleaner, privacyService)
}

func NewHandlerWithSuppressionClock(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, privacyService *privacy.Service, cleaner privacy.SyntheticEvidenceCleaner, now func() time.Time) http.Handler {
	h := newHandler(service, terms, origins, cleaner, privacyService).(*Handler)
	if now != nil {
		h.now = now
	}
	return h
}

// NewHandlerWithSuppressionRegistry keeps the local restore sidecar current
// whenever a suppression execution has committed successfully.
func NewHandlerWithSuppressionRegistry(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, privacyService *privacy.Service, cleaner privacy.SyntheticEvidenceCleaner, registryPath string, replayStores ...privacy.SuppressionReplayStore) http.Handler {
	h := newHandler(service, terms, origins, cleaner, privacyService).(*Handler)
	h.suppressionRegistryPath = registryPath
	if len(replayStores) > 0 {
		h.suppressionReplayStore = replayStores[0]
	}
	return h
}

func NewHandlerWithSuppressionClockAndRegistry(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, privacyService *privacy.Service, cleaner privacy.SyntheticEvidenceCleaner, now func() time.Time, registryPath string, replayStores ...privacy.SuppressionReplayStore) http.Handler {
	h := newHandler(service, terms, origins, cleaner, privacyService).(*Handler)
	if now != nil {
		h.now = now
	}
	h.suppressionRegistryPath = registryPath
	if len(replayStores) > 0 {
		h.suppressionReplayStore = replayStores[0]
	}
	return h
}

func newHandler(service *identity.AuthenticationService, terms identity.AuthenticationRepository, origins []string, cleaner privacy.SyntheticEvidenceCleaner, privacyServices ...*privacy.Service) http.Handler {
	h := &Handler{service: service, terms: terms, origins: map[string]bool{}, now: time.Now}
	h.evidenceCleaner = cleaner
	if len(privacyServices) > 0 {
		h.privacy = privacyServices[0]
	}
	for _, origin := range origins {
		h.origins[origin] = true
	}
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID, err := (credentials.Generator{}).ID()
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	origin := r.Header.Get("Origin")
	if origin != "" {
		if !h.origins[origin] {
			h.fail(w, http.StatusForbidden, "origin_denied", "Origen no permitido.")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After")
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.RawQuery != "" {
		h.fail(w, 400, "invalid_request", "No se admiten parámetros de URL en autenticación.")
		return
	}
	const (
		accountPhonePath               = "/api/v1/account/security/phone"
		accountPhoneSendCodePath       = "/api/v1/account/security/phone/send-code"
		accountPhoneVerifyPath         = "/api/v1/account/security/phone/verify"
		accountDevicesPath             = "/api/v1/account/security/devices"
		accountDevicesRevokeOthersPath = "/api/v1/account/security/devices/revoke-others"
		accountDevicesPrefix           = "/api/v1/account/security/devices/"
		accountAlertsPath              = "/api/v1/account/security/alerts"
		accountAlertsChannelsPath      = "/api/v1/account/security/alerts/channels"
		accountTOTPPath                = "/api/v1/account/security/totp"
		accountTOTPSetupPath           = "/api/v1/account/security/totp/setup"
		accountTOTPEnablePath          = "/api/v1/account/security/totp/enable"
		accountTOTPDisablePath         = "/api/v1/account/security/totp/disable"
		accountPasskeysPath            = "/api/v1/account/security/passkeys"
		accountPasskeysRegisterOptionsPath = "/api/v1/account/security/passkeys/register/options"
		accountPasskeysRegisterVerifyPath  = "/api/v1/account/security/passkeys/register/verify"
		accountPasskeysPrefix          = "/api/v1/account/security/passkeys/"
		accountQRPath                  = "/api/v1/account/security/qr"
		accountQRTicketPath            = "/api/v1/account/security/qr/ticket"
		accountQRTicketPrefix          = "/api/v1/account/security/qr/ticket/"
		accountQRApprovePath           = "/api/v1/account/security/qr/approve"
	)
	deviceDeletePath := strings.HasPrefix(r.URL.Path, accountDevicesPrefix) && r.URL.Path != accountDevicesRevokeOthersPath
	passkeyDeletePath := strings.HasPrefix(r.URL.Path, accountPasskeysPrefix) && r.URL.Path != accountPasskeysRegisterOptionsPath && r.URL.Path != accountPasskeysRegisterVerifyPath
	qrTicketStatusPath := strings.HasPrefix(r.URL.Path, accountQRTicketPrefix)
	expected := http.MethodPost
	if r.URL.Path == "/api/v1/auth/session" || r.URL.Path == "/api/v1/auth/terms" || r.URL.Path == "/api/v1/profile" || r.URL.Path == "/api/v1/rights-requests" || r.URL.Path == "/api/v1/privacy/export" || r.URL.Path == "/api/v1/privacy/export/archive" || r.URL.Path == accountPhonePath || r.URL.Path == accountDevicesPath || r.URL.Path == accountAlertsPath || r.URL.Path == accountTOTPPath || r.URL.Path == accountPasskeysPath || r.URL.Path == accountQRPath || qrTicketStatusPath {
		expected = http.MethodGet
	}
	if r.URL.Path == accountAlertsChannelsPath {
		expected = http.MethodPut
	}
	if deviceDeletePath || passkeyDeletePath || (r.URL.Path == accountQRPath && r.Method == http.MethodDelete) {
		expected = http.MethodDelete
	}

	if r.URL.Path == "/api/v1/profile" && r.Method == http.MethodPut || r.URL.Path == "/api/v1/rights-requests" && r.Method == http.MethodPost {
		expected = r.Method
	}
	const suppressionQueuePath = "/api/v1/privacy/suppression-requests"
	if r.URL.Path == suppressionQueuePath {
		expected = http.MethodGet
	}
	const credentialNoticeQueuePath = "/api/v1/admin/credential-notices"
	const credentialNoticePrefix = "/api/v1/admin/credential-notices/"
	credentialNoticeReopenPath := strings.HasPrefix(r.URL.Path, credentialNoticePrefix) && strings.HasSuffix(r.URL.Path, "/reopen")
	if r.URL.Path == credentialNoticeQueuePath {
		expected = http.MethodGet
	}
	if credentialNoticeReopenPath {
		expected = http.MethodPost
	}
	const reviewPrefix = "/api/v1/privacy/suppression-requests/"
	reviewSuffix := "/review"
	reviewPath := strings.HasPrefix(r.URL.Path, reviewPrefix) && strings.HasSuffix(r.URL.Path, reviewSuffix)
	executeSuffix := "/execute"
	executePath := strings.HasPrefix(r.URL.Path, reviewPrefix) && strings.HasSuffix(r.URL.Path, executeSuffix)
	if reviewPath || executePath {
		expected = http.MethodPost
	}
	paths := map[string]bool{
		"/api/v1/auth/register":                  true,
		"/api/v1/auth/verification/reissue":      true,
		"/api/v1/auth/verification":              true,
		"/api/v1/auth/verification/status":       true,
		"/api/v1/auth/login":                     true,
		"/api/v1/auth/session":                   true,
		"/api/v1/auth/logout":                    true,
		"/api/v1/auth/terms":                     true,
		"/api/v1/auth/password/recovery":         true,
		"/api/v1/auth/password/recovery/consume": true,
		"/api/v1/auth/password/recovery/status":  true,
		"/api/v1/auth/password/change":           true,
		"/api/v1/profile":                        true,
		"/api/v1/rights-requests":                true,
		"/api/v1/privacy/export":                 true,
		"/api/v1/privacy/export/archive":         true,
		"/api/v1/privacy/retention/purge":        true,
		accountPhonePath:                         true,
		accountPhoneSendCodePath:                 true,
		accountPhoneVerifyPath:                   true,
		accountDevicesPath:                       true,
		accountDevicesRevokeOthersPath:           true,
		accountAlertsPath:                        true,
		accountAlertsChannelsPath:                true,
		accountTOTPPath:                          true,
		accountTOTPSetupPath:                     true,
		accountTOTPEnablePath:                    true,
		accountTOTPDisablePath:                   true,
		accountPasskeysPath:                      true,
		accountPasskeysRegisterOptionsPath:        true,
		accountPasskeysRegisterVerifyPath:         true,
		accountQRPath:                            true,
		accountQRTicketPath:                      true,
		accountQRApprovePath:                     true,
		suppressionQueuePath:                     true,
		credentialNoticeQueuePath:                true,
	}
	if !paths[r.URL.Path] && !reviewPath && !executePath && !credentialNoticeReopenPath && !deviceDeletePath && !passkeyDeletePath && !qrTicketStatusPath {
		h.fail(w, 404, "not_found", "Recurso no encontrado.")
		return
	}
	if r.Method != expected {
		allow := expected
		if r.URL.Path == "/api/v1/profile" {
			allow = "GET, PUT"
		}
		if r.URL.Path == "/api/v1/rights-requests" {
			allow = "GET, POST"
		}
		if r.URL.Path == accountAlertsChannelsPath {
			allow = "PUT"
		}
		if r.URL.Path == accountQRPath {
			allow = "GET, DELETE"
		}
		w.Header().Set("Allow", allow)
		h.fail(w, 405, "method_not_allowed", "Método no permitido.")
		return
	}
	clientIP, err := clientIPFromRequest(r)
	if err != nil {
		h.fail(w, 400, "invalid_request", "Dirección de cliente inválida.")
		return
	}
	if r.URL.Path == credentialNoticeQueuePath || credentialNoticeReopenPath {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleAdministrator, identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if r.URL.Path == credentialNoticeQueuePath {
			items, err := h.service.TerminalCredentialNotices(r.Context())
			if err != nil {
				h.serviceError(w, err)
				return
			}
			h.write(w, http.StatusOK, map[string]any{"items": items})
			return
		}
		relative := strings.TrimPrefix(r.URL.Path, credentialNoticePrefix)
		eventID := strings.TrimSuffix(relative, "/reopen")
		if strings.Contains(eventID, "/") || !canonicalUUID(eventID) {
			h.fail(w, http.StatusNotFound, "not_found", "Evento no encontrado.")
			return
		}
		var input struct {
			ReasonCode string `json:"reason_code"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		result, err := h.service.ReopenCredentialNotice(r.Context(), eventID, principal.AccountID, input.ReasonCode, r.Header.Get("Idempotency-Key"), w.Header().Get("X-Request-ID"))
		if err != nil {
			h.serviceError(w, err, r.URL.Path)
			return
		}
		h.write(w, http.StatusOK, result)
		return
	}
	if deviceDeletePath {
		sessionID := strings.TrimPrefix(r.URL.Path, accountDevicesPrefix)
		if sessionID == "" || strings.Contains(sessionID, "/") {
			h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if err := h.service.RevokeLinkedDevice(r.Context(), principal.AccountID, sessionID); err != nil {
			h.deviceError(w, err)
			return
		}
		_ = h.service.RecordSecurityAlertEvent(r.Context(), principal.AccountID, "session_revoked", "Sesión de dispositivo revocada", identity.DeriveDeviceLabel(r.UserAgent()), identity.MaskIP(clientIPForDevice(r)))
		h.write(w, http.StatusOK, map[string]any{
			"status":     "revoked",
			"session_id": sessionID,
		})
		return
	}
	if passkeyDeletePath {
		passkeyID := strings.TrimPrefix(r.URL.Path, accountPasskeysPrefix)
		if passkeyID == "" || strings.Contains(passkeyID, "/") {
			h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		deviceLabel := identity.DeriveDeviceLabel(r.UserAgent())
		ipMasked := identity.MaskIP(clientIPForDevice(r))
		if err := h.service.DeletePasskey(r.Context(), principal.AccountID, passkeyID, deviceLabel, ipMasked); err != nil {
			h.passkeyError(w, err)
			return
		}
		h.write(w, http.StatusOK, map[string]any{
			"status":     "deleted",
			"passkey_id": passkeyID,
		})
		return
	}
	if qrTicketStatusPath {
		ticketID := strings.TrimPrefix(r.URL.Path, accountQRTicketPrefix)
		if ticketID == "" || strings.Contains(ticketID, "/") || !canonicalUUID(ticketID) {
			h.fail(w, http.StatusNotFound, "not_found", "Ticket no encontrado.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		status, err := h.service.GetQRTicketStatus(r.Context(), principal.AccountID, ticketID)
		if err != nil {
			h.qrError(w, err)
			return
		}
		h.write(w, http.StatusOK, status)
		return
	}
	// RemoteAddr is trusted for the local direct topology. Never trust forwarded headers.
	if reviewPath || executePath {
		suffix := reviewSuffix
		if executePath {
			suffix = executeSuffix
		}
		relativePath := strings.TrimPrefix(r.URL.Path, reviewPrefix)
		if !strings.HasSuffix(relativePath, suffix) {
			h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
			return
		}
		requestID := strings.TrimSuffix(relativePath, suffix)
		if strings.Contains(requestID, "/") || !canonicalUUID(requestID) {
			h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
			return
		}
		if h.privacy == nil {
			h.fail(w, http.StatusServiceUnavailable, "privacy_unavailable", "Servicio de privacidad no disponible.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleAdministrator, identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if executePath {
			// The API records durable cleanup work; the dedicated worker owns GCS deletion.
			result, err := h.privacy.ExecuteSuppression(r.Context(), principal.AccountID, requestID, r.Header.Get("Idempotency-Key"), w.Header().Get("X-Request-ID"), h.now, nil)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			if result.Status == "completada" {
				if h.suppressionReplayStore != nil {
					if err := h.privacy.ExportSuppressionReplayManifestToStore(r.Context(), h.suppressionReplayStore); err != nil {
						h.fail(w, http.StatusServiceUnavailable, "privacy_replay_registry_unavailable", "La baja se completó, pero el registro de recuperación durable requiere reintento.")
						return
					}
				}
				if h.suppressionRegistryPath != "" {
					if err := h.privacy.ExportSuppressionReplayManifestToFile(r.Context(), h.suppressionRegistryPath); err != nil {
						h.fail(w, http.StatusServiceUnavailable, "privacy_replay_registry_unavailable", "La baja se completó, pero el registro local de recuperación requiere reintento.")
						return
					}
				}
			}
			h.write(w, http.StatusOK, result)
		} else {
			review, err := h.privacy.ReviewSuppression(r.Context(), principal.AccountID, requestID, r.Header.Get("Idempotency-Key"), w.Header().Get("X-Request-ID"), h.now)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			h.write(w, http.StatusOK, review)
		}
		return
	}
	if r.URL.Path == "/api/v1/privacy/retention/purge" {
		if r.Method != http.MethodPost {
			h.fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método no permitido.")
			return
		}
		if h.privacy == nil {
			h.fail(w, http.StatusServiceUnavailable, "privacy_unavailable", "Servicio de privacidad no disponible.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		if _, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleAdministrator, identity.UserOperation); err != nil {
			h.serviceError(w, err)
			return
		}
		var result privacy.RetentionPurgeResult
		var err error
		if h.evidenceCleaner != nil {
			result, err = h.privacy.PurgeExpiredLocalRetentions(r.Context(), h.now().UTC(), 100, h.evidenceCleaner)
		} else {
			result, err = h.privacy.PurgeExpiredReservationLinks(r.Context(), h.now().UTC(), 100)
		}
		if err != nil {
			h.privacyError(w, err)
			return
		}
		outboxPurged, err := h.service.PurgeExpiredCredentialNotices(r.Context(), h.now().UTC(), 100)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		h.write(w, http.StatusOK, map[string]any{
			"scanned": result.Scanned, "purged": result.Purged, "deferred": result.Deferred,
			"outbox_events_purged": outboxPurged, "terms_purged": result.TermsPurged,
			"rights_purged": result.RightsPurged, "quotes_purged": result.QuotesPurged,
			"verifications_purged":       result.VerificationsPurged,
			"evidence_files_queued":      result.EvidenceFilesQueued,
			"evidence_files_removed":     result.EvidenceFilesRemoved,
			"evidence_files_deferred":    result.EvidenceFilesDeferred,
			"contracts_purged":           result.ContractsPurged,
			"publication_history_purged": result.PublicationHistoryPurged,
		})
		return
	}
	if r.URL.Path == suppressionQueuePath {
		if h.privacy == nil {
			h.fail(w, http.StatusServiceUnavailable, "privacy_unavailable", "Servicio de privacidad no disponible.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		if _, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleAdministrator, identity.UserOperation); err != nil {
			h.serviceError(w, err)
			return
		}
		items, err := h.privacy.PendingSuppressions(r.Context())
		if err != nil {
			h.privacyError(w, err)
			return
		}
		h.write(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	switch r.URL.Path {
	case "/api/v1/profile", "/api/v1/rights-requests", "/api/v1/privacy/export", "/api/v1/privacy/export/archive":
		if h.privacy == nil {
			h.fail(w, http.StatusServiceUnavailable, "privacy_unavailable", "Servicio de perfil no disponible.")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if r.URL.Path == "/api/v1/privacy/export/archive" {
			archive, err := h.privacy.ExportOwnArchive(r.Context(), principal.AccountID)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", `attachment; filename="espacigo-datos-propios.zip"`)
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archive)
			return
		}
		if r.URL.Path == "/api/v1/privacy/export" {
			data, err := h.privacy.ExportOwnData(r.Context(), principal.AccountID)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			h.write(w, http.StatusOK, data)
			return
		}
		if r.URL.Path == "/api/v1/profile" {
			if r.Method == http.MethodGet {
				profile, err := h.privacy.Profile(r.Context(), principal.AccountID)
				if err != nil {
					h.privacyError(w, err)
					return
				}
				h.write(w, http.StatusOK, map[string]any{"display_name": profile.Name, "phone": profile.Phone, "updated_at": profile.UpdatedAt.UTC()})
				return
			}
			var input struct {
				Name  string `json:"display_name"`
				Phone string `json:"phone"`
			}
			if !h.decode(w, r, &input) {
				return
			}
			profile, err := h.privacy.UpdateProfile(r.Context(), principal.AccountID, input.Name, input.Phone)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			h.write(w, http.StatusOK, map[string]any{"display_name": profile.Name, "phone": profile.Phone, "updated_at": profile.UpdatedAt.UTC()})
			return
		}
		if r.Method == http.MethodGet {
			items, err := h.privacy.OwnRequests(r.Context(), principal.AccountID)
			if err != nil {
				h.privacyError(w, err)
				return
			}
			h.write(w, http.StatusOK, map[string]any{"items": items})
			return
		}
		var input struct {
			Kind string `json:"type"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		item, err := h.privacy.RequestRight(r.Context(), principal.AccountID, input.Kind, "web")
		if err != nil {
			h.privacyError(w, err)
			return
		}
		h.write(w, http.StatusAccepted, item)
	case accountPhonePath, accountPhoneSendCodePath, accountPhoneVerifyPath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if h.phoneService == nil {
			h.fail(w, http.StatusServiceUnavailable, "service_unavailable", "Servicio de verificación telefónica no disponible.")
			return
		}

		switch r.URL.Path {
		case accountPhonePath:
			status, err := h.phoneService.GetPhoneVerificationStatus(r.Context(), principal.AccountID)
			if err != nil {
				h.phoneError(w, err)
				return
			}
			h.write(w, http.StatusOK, status)
			return

		case accountPhoneSendCodePath:
			var input struct {
				Phone string `json:"phone"`
			}
			if !h.decode(w, r, &input) {
				return
			}
			result, err := h.phoneService.SendPhoneVerificationCode(r.Context(), principal.AccountID, input.Phone)
			if err != nil {
				h.phoneError(w, err)
				return
			}
			h.write(w, http.StatusOK, result)
			return

		case accountPhoneVerifyPath:
			var input struct {
				Code string `json:"code"`
			}
			if !h.decode(w, r, &input) {
				return
			}
			result, err := h.phoneService.VerifyPhoneCode(r.Context(), principal.AccountID, input.Code)
			if err != nil {
				h.phoneError(w, err)
				return
			}
			_ = h.service.RecordSecurityAlertEvent(r.Context(), principal.AccountID, "phone_verified", "Teléfono de seguridad verificado", identity.DeriveDeviceLabel(r.UserAgent()), identity.MaskIP(clientIPForDevice(r)))
			h.write(w, http.StatusOK, result)
			return
		}
	case accountDevicesPath, accountDevicesRevokeOthersPath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		if r.URL.Path == accountDevicesPath {
			devices, err := h.service.ListLinkedDevices(r.Context(), principal.AccountID, principal.SessionID)
			if err != nil {
				h.deviceError(w, err)
				return
			}
			if devices == nil {
				devices = []identity.LinkedDevice{}
			}
			h.write(w, http.StatusOK, map[string]any{"devices": devices})
			return
		}
		if r.URL.Path == accountDevicesRevokeOthersPath {
			count, err := h.service.RevokeOtherLinkedDevices(r.Context(), principal.AccountID, principal.SessionID)
			if err != nil {
				h.deviceError(w, err)
				return
			}
			_ = h.service.RecordSecurityAlertEvent(r.Context(), principal.AccountID, "sessions_revoked_others", "Otras sesiones revocadas", identity.DeriveDeviceLabel(r.UserAgent()), identity.MaskIP(clientIPForDevice(r)))
			h.write(w, http.StatusOK, map[string]any{
				"status":        "revoked_others",
				"revoked_count": count,
			})
			return
		}
	case accountAlertsPath, accountAlertsChannelsPath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		if r.URL.Path == accountAlertsPath {
			alerts, err := h.service.GetSecurityAlerts(r.Context(), principal.AccountID)
			if err != nil {
				h.alertsError(w, err)
				return
			}
			h.write(w, http.StatusOK, alerts)
			return
		}
		if r.URL.Path == accountAlertsChannelsPath {
			var input identity.UpdateSecurityAlertChannelsInput
			if !h.decode(w, r, &input) {
				return
			}
			input.DeviceLabel = identity.DeriveDeviceLabel(r.UserAgent())
			input.IPMasked = identity.MaskIP(clientIPForDevice(r))
			prefs, err := h.service.UpdateSecurityAlertChannels(r.Context(), principal.AccountID, input)
			if err != nil {
				h.alertsError(w, err)
				return
			}
			h.write(w, http.StatusOK, prefs)
			return
		}
	case accountTOTPPath, accountTOTPSetupPath, accountTOTPEnablePath, accountTOTPDisablePath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		switch r.URL.Path {
		case accountTOTPPath:
			status, err := h.service.GetTOTPStatus(r.Context(), principal.AccountID)
			if err != nil {
				h.totpError(w, err)
				return
			}
			h.write(w, http.StatusOK, status)
			return

		case accountTOTPSetupPath:
			resp, err := h.service.SetupTOTP(r.Context(), principal.AccountID)
			if err != nil {
				h.totpError(w, err)
				return
			}
			h.write(w, http.StatusOK, resp)
			return

		case accountTOTPEnablePath:
			var input identity.AccountTOTPEnableInput
			if !h.decode(w, r, &input) {
				return
			}
			deviceLabel := identity.DeriveDeviceLabel(r.UserAgent())
			ipMasked := identity.MaskIP(clientIPForDevice(r))
			resp, err := h.service.EnableTOTP(r.Context(), principal.AccountID, input.Code, deviceLabel, ipMasked)
			if err != nil {
				h.totpError(w, err)
				return
			}
			h.write(w, http.StatusOK, resp)
			return

		case accountTOTPDisablePath:
			var input identity.AccountTOTPDisableInput
			if !h.decode(w, r, &input) {
				return
			}
			deviceLabel := identity.DeriveDeviceLabel(r.UserAgent())
			ipMasked := identity.MaskIP(clientIPForDevice(r))
			resp, err := h.service.DisableTOTP(r.Context(), principal.AccountID, input.Code, deviceLabel, ipMasked)
			if err != nil {
				h.totpError(w, err)
				return
			}
			h.write(w, http.StatusOK, resp)
			return
		}
	case accountPasskeysPath, accountPasskeysRegisterOptionsPath, accountPasskeysRegisterVerifyPath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		switch r.URL.Path {
		case accountPasskeysPath:
			list, err := h.service.ListPasskeys(r.Context(), principal.AccountID)
			if err != nil {
				h.passkeyError(w, err)
				return
			}
			h.write(w, http.StatusOK, list)
			return

		case accountPasskeysRegisterOptionsPath:
			rpID := ""
			if host, _, err := net.SplitHostPort(r.Host); err == nil && host != "" {
				rpID = host
			} else if r.Host != "" {
				rpID = r.Host
			}
			if rpID == "" {
				rpID = "localhost"
			}
			opts, err := h.service.BeginPasskeyRegistration(r.Context(), principal.AccountID, rpID)
			if err != nil {
				h.passkeyError(w, err)
				return
			}
			h.write(w, http.StatusOK, opts)
			return

		case accountPasskeysRegisterVerifyPath:
			var input identity.PasskeyRegistrationVerifyInput
			if !h.decode(w, r, &input) {
				return
			}
			deviceLabel := identity.DeriveDeviceLabel(r.UserAgent())
			ipMasked := identity.MaskIP(clientIPForDevice(r))
			allowedOrigins := make([]string, 0, len(h.origins))
			for o := range h.origins {
				allowedOrigins = append(allowedOrigins, o)
			}
			passkey, err := h.service.CompletePasskeyRegistration(r.Context(), principal.AccountID, input, allowedOrigins, deviceLabel, ipMasked)
			if err != nil {
				h.passkeyError(w, err)
				return
			}
			h.write(w, http.StatusOK, passkey)
			return
		}
	case accountQRPath, accountQRTicketPath, accountQRApprovePath:
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		principal, err := h.service.Authorize(r.Context(), identity.Secret(parts[1]), "", identity.UserOperation)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		switch r.URL.Path {
		case accountQRPath:
			if r.Method == http.MethodDelete {
				deviceLabel := identity.DeriveDeviceLabel(r.UserAgent())
				ipMasked := identity.MaskIP(clientIPForDevice(r))
				resp, err := h.service.DisableQR(r.Context(), principal.AccountID, deviceLabel, ipMasked)
				if err != nil {
					h.qrError(w, err)
					return
				}
				h.write(w, http.StatusOK, resp)
				return
			}
			status, err := h.service.GetQRStatus(r.Context(), principal.AccountID)
			if err != nil {
				h.qrError(w, err)
				return
			}
			h.write(w, http.StatusOK, status)
			return

		case accountQRTicketPath:
			var input struct {
				DeviceLabel string `json:"device_label"`
			}
			if r.Body != nil && r.ContentLength > 0 {
				_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input)
			}
			deviceLabel := input.DeviceLabel
			if deviceLabel == "" {
				deviceLabel = identity.DeriveDeviceLabel(r.UserAgent())
			}
			ipMasked := identity.MaskIP(clientIPForDevice(r))
			resp, err := h.service.CreateQRTicket(r.Context(), principal.AccountID, deviceLabel, ipMasked)
			if err != nil {
				h.qrError(w, err)
				return
			}
			h.write(w, http.StatusOK, resp)
			return

		case accountQRApprovePath:
			var input identity.ApproveQRTicketInput
			if !h.decode(w, r, &input) {
				return
			}
			deviceLabel := input.DeviceLabel
			if deviceLabel == "" {
				deviceLabel = identity.DeriveDeviceLabel(r.UserAgent())
			}
			ipMasked := identity.MaskIP(clientIPForDevice(r))
			resp, err := h.service.ApproveQRTicket(r.Context(), principal.AccountID, input.Token, deviceLabel, ipMasked)
			if err != nil {
				h.qrError(w, err)
				return
			}
			h.write(w, http.StatusOK, resp)
			return
		}
	case "/api/v1/auth/terms":
		documents, err := identity.PublicConsentDocuments()
		if err != nil {
			h.fail(w, 503, "terms_unavailable", "Los documentos no están disponibles.")
			return
		}
		items := []map[string]any{}
		for _, document := range documents {
			version, err := h.terms.TermsVersion(r.Context(), document.ID)
			if err != nil || version.Code != document.Code || version.Type != document.Type || version.SHA256 != document.SHA256 || version.PublishedAt.After(h.now()) {
				h.fail(w, 503, "terms_unavailable", "Los documentos todavía no están disponibles. Inténtalo más tarde.")
				return
			}
			items = append(items, map[string]any{"id": version.ID, "code": version.Code, "type": version.Type, "sha256": version.SHA256, "published_at": version.PublishedAt.UTC(), "synthetic": true, "document": document})
		}
		h.write(w, 200, map[string]any{"items": items})
	case "/api/v1/auth/register":
		var input struct {
			Email           string          `json:"email"`
			Password        identity.Secret `json:"password"`
			TermsVersionIDs []string        `json:"terms_version_ids"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		documents, documentErr := identity.PublicConsentDocuments()
		if documentErr != nil {
			h.fail(w, 503, "consent_unavailable", "Los documentos no están disponibles. Inténtalo más tarde.")
			return
		}
		fields := accountInputErrors(input.Email, input.Password, true)
		if len(input.TermsVersionIDs) != len(documents) {
			fields = append(fields, accountFieldError{"terms_version_ids", "consent_invalid"})
		}
		if len(fields) > 0 {
			h.accountFields(w, 422, "invalid_value", fields)
			return
		}
		id, err := h.service.Register(r.Context(), identity.RegisterInput{Email: input.Email, Password: input.Password, TermsVersionIDs: input.TermsVersionIDs, Channel: "web", ClientIP: clientIP})
		if err != nil {
			if id != "" {
				h.write(w, 202, map[string]any{"account_id": id, "status": "verification_pending", "message": "La cuenta quedó pendiente de verificación. Puedes solicitar otro correo desde esta pantalla."})
				return
			}
			if errors.Is(err, identity.ErrConsentInvalid) {
				h.accountFields(w, 422, "invalid_value", []accountFieldError{{"terms_version_ids", "consent_invalid"}})
				return
			}
			if errors.Is(err, identity.ErrEmailRegistered) {
				h.accountFields(w, 409, "email_registered", []accountFieldError{{"email", "already_registered"}})
				return
			}
			h.serviceError(w, err)
			return
		}
		h.write(w, 201, map[string]any{"account_id": id, "status": "email_pending"})
	case "/api/v1/auth/verification/reissue":
		var input struct {
			Email string `json:"email"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		if err := h.service.ReissueVerification(r.Context(), input.Email, clientIP); err != nil &&
			!errors.Is(err, identity.ErrNotFound) && !errors.Is(err, identity.ErrAccountDisabled) &&
			!errors.Is(err, identity.ErrRateLimited) && !errors.Is(err, identity.ErrDelivery) {
			h.serviceError(w, err)
			return
		}
		w.WriteHeader(204)
	case "/api/v1/auth/verification":
		var input struct {
			TokenID string          `json:"token_id"`
			Token   identity.Secret `json:"token"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		if err := h.service.VerifyEmail(r.Context(), input.TokenID, input.Token, clientIP); err != nil {
			h.serviceError(w, err)
			return
		}
		w.WriteHeader(204)
	case "/api/v1/auth/verification/status":
		var input struct {
			TokenID string          `json:"token_id"`
			Token   identity.Secret `json:"token"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		status, err := h.service.InspectVerificationLink(r.Context(), input.TokenID, input.Token, clientIP)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		h.write(w, http.StatusOK, map[string]any{"status": status})
	case "/api/v1/auth/password/recovery":
		var input struct {
			Email string `json:"email"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		if err := h.service.RequestPasswordRecovery(r.Context(), input.Email, clientIP); err != nil {
			h.serviceError(w, err)
			return
		}
		h.write(w, 202, map[string]any{"status": "accepted", "message": "Si la cuenta es elegible, recibirás instrucciones en el correo registrado."})
	case "/api/v1/auth/password/recovery/consume":
		var input struct {
			TokenID      string          `json:"token_id"`
			Token        identity.Secret `json:"token"`
			Password     identity.Secret `json:"new_password"`
			Confirmation identity.Secret `json:"confirm_password"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		err := h.service.ResetPassword(r.Context(), identity.ResetPasswordInput{TokenID: input.TokenID, Token: input.Token, Password: input.Password, Confirmation: input.Confirmation, ClientIP: clientIP})
		if err != nil {
			h.serviceError(w, err, r.URL.Path)
			return
		}
		w.WriteHeader(204)
	case "/api/v1/auth/password/recovery/status":
		var input struct {
			TokenID string          `json:"token_id"`
			Token   identity.Secret `json:"token"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		status, err := h.service.InspectRecoveryLink(r.Context(), input.TokenID, input.Token, clientIP)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		h.write(w, http.StatusOK, map[string]any{"status": status})
	case "/api/v1/auth/password/change":
		var input struct {
			Current      identity.Secret `json:"current_password"`
			Password     identity.Secret `json:"new_password"`
			Confirmation identity.Secret `json:"confirm_password"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		err := h.service.ChangePassword(r.Context(), identity.ChangePasswordInput{SessionToken: parts[1], CurrentPassword: input.Current, Password: input.Password, Confirmation: input.Confirmation})
		if err != nil {
			h.serviceError(w, err, r.URL.Path)
			return
		}
		w.WriteHeader(204)
	case "/api/v1/auth/login":
		var input struct {
			Email    string          `json:"email"`
			Password identity.Secret `json:"password"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		if fields := accountInputErrors(input.Email, input.Password, false); len(fields) > 0 {
			h.accountFields(w, 422, "invalid_value", fields)
			return
		}
		clientIP := clientIPForDevice(r)
		userAgent := r.UserAgent()
		result, err := h.service.Login(r.Context(), identity.LoginInput{
			Email:     input.Email,
			Password:  input.Password,
			UserAgent: userAgent,
			IP:        clientIP,
		})
		if err != nil {
			h.serviceError(w, err)
			return
		}
		// Explicit transport serialization; Secret remains redacted in all other formatting.
		h.write(w, 200, map[string]any{"account_id": result.AccountID, "roles": result.Roles, "capabilities": result.Capabilities, "access_token": string(result.Token), "token_type": "Bearer", "expires_at": result.ExpiresAt.UTC(), "restricted_mode": result.RestrictedMode})
	case "/api/v1/auth/session", "/api/v1/auth/logout":
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.serviceError(w, identity.ErrUnauthorized)
			return
		}
		token := identity.Secret(parts[1])
		principal, err := h.service.Authorize(r.Context(), token, "", identity.AutomaticPolling)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if r.URL.Path == "/api/v1/auth/session" {
			h.write(w, 200, map[string]any{"account_id": principal.AccountID, "roles": principal.Roles, "capabilities": principal.Capabilities, "restricted_mode": principal.RestrictedMode})
			return
		}
		if err := h.service.Logout(r.Context(), token); err != nil {
			h.serviceError(w, err)
			return
		}
		w.WriteHeader(204)
	}
}

// The public SSR adds this sanitized value only after extracting the client
// address from the trusted load-balancer suffix of X-Forwarded-For. The API is
// invoker-protected, and the SSR never forwards a browser-supplied copy.
// Direct/local callers continue to use the socket peer; X-Forwarded-For is
// intentionally ignored here.
func clientIPFromRequest(r *http.Request) (string, error) {
	values := r.Header.Values("X-Espacigo-Client-IP")
	if len(values) > 1 {
		return "", errors.New("multiple internal client IP headers")
	}
	var raw string
	if len(values) == 1 {
		raw = strings.TrimSpace(values[0])
	} else {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "", err
		}
		raw = host
	}
	address, err := netip.ParseAddr(raw)
	if err != nil {
		return "", err
	}
	return address.Unmap().String(), nil
}
func (h *Handler) decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		h.fail(w, 415, "unsupported_media_type", "Usa application/json.")
		return false
	}
	reader := http.MaxBytesReader(w, r.Body, 16<<10)
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.fail(w, 400, "invalid_request", "JSON inválido o campos no admitidos.")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		h.fail(w, 400, "invalid_request", "Solo se admite un objeto JSON.")
		return false
	}
	return true
}
func (h *Handler) serviceError(w http.ResponseWriter, err error, paths ...string) {
	requestPath := ""
	if len(paths) > 0 {
		requestPath = paths[0]
	}
	status, code, message := 500, "internal_error", "Ocurrió un error inesperado."
	var blocked *identity.LoginBlockedError
	switch {
	case errors.As(err, &blocked):
		status = 429
		code = "login_blocked"
		message = "Login bloqueado temporalmente."
		seconds := int(time.Until(blocked.Until).Seconds()) + 1
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	case errors.Is(err, identity.ErrEmailRegistered):
		status = 409
		code = "email_registered"
		message = identity.ErrEmailRegistered.Error()
	case errors.Is(err, identity.ErrCredentials), errors.Is(err, identity.ErrUnauthorized):
		status = 401
		code = "unauthenticated"
		message = "Credencial ausente, incorrecta o expirada."
		w.Header().Set("WWW-Authenticate", "Bearer")
	case errors.Is(err, identity.ErrEmailUnverified):
		status = 403
		code = "email_unverified"
		message = "Confirma tu correo antes de iniciar sesión."
	case errors.Is(err, identity.ErrAccountDisabled), errors.Is(err, identity.ErrForbidden):
		status = 403
		code = "forbidden"
		message = "La cuenta no permite esta operación."
	case errors.Is(err, identity.ErrCredentialNoticeState):
		status = http.StatusConflict
		code = "credential_notice_not_recoverable"
		message = "El evento no está en estado terminal recuperable."
	case errors.Is(err, identity.ErrCredentialNoticeRecipient):
		status = http.StatusConflict
		code = "credential_notice_recipient_inactive"
		message = "El destinatario no mantiene una cuenta activa."
	case errors.Is(err, identity.ErrTokenInvalid):
		status = 422
		code = "invalid_token"
		message = "Token inválido, expirado o ya utilizado."
	case errors.Is(err, identity.ErrCurrentPassword):
		status = 422
		code = "current_password_invalid"
		message = identity.ErrCurrentPassword.Error()
	case errors.Is(err, identity.ErrPasswordSame):
		status = 422
		code = "password_unchanged"
		message = identity.ErrPasswordSame.Error()
	case errors.Is(err, identity.ErrPasswordRecentlyUsed):
		status = 422
		code = "password_recently_used"
		message = identity.ErrPasswordRecentlyUsed.Error()
	case errors.Is(err, identity.ErrPasswordConfirm):
		status = 422
		code = "password_confirmation_mismatch"
		message = identity.ErrPasswordConfirm.Error()
	case errors.Is(err, identity.ErrRateLimited):
		status = 429
		code = "rate_limited"
		message = "Límite de verificación alcanzado; intenta más tarde."
		w.Header().Set("Retry-After", "3600")
	case errors.Is(err, identity.ErrInvalid):
		status = 422
		code = "validation_error"
		message = "Revisa correo, contraseña y aceptación de términos."
	case errors.Is(err, identity.ErrNotFound):
		status = 404
		code = "not_found"
		message = "Recurso no encontrado."
	case errors.Is(err, identity.ErrDelivery):
		status = 503
		code = "mail_unavailable"
		message = "Correo de desarrollo no disponible; solicita reenvío."
		if requestPath == "/api/v1/auth/password/change" || requestPath == "/api/v1/auth/password/recovery/consume" {
			message = "Contraseña actualizada y sesiones revocadas; revisa Mailpit e inicia sesión."
		}
	}
	h.fail(w, status, code, message)
}

func (h *Handler) privacyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrConflict):
		h.fail(w, http.StatusConflict, "conflict", "La solicitud de supresión cambió o la clave idempotente pertenece a otra revisión.")
	case errors.Is(err, privacy.ErrInvalid):
		h.fail(w, http.StatusUnprocessableEntity, "validation_error", "Revisa los datos enviados para esta operación de privacidad.")
	case errors.Is(err, privacy.ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
	default:
		h.fail(w, http.StatusInternalServerError, "internal_error", "No se pudo completar la operación.")
	}
}

func canonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
				return false
			}
		}
	}
	return true
}
func (h *Handler) fail(w http.ResponseWriter, status int, code, message string) {
	h.write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")}})
}
func (h *Handler) write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Only stable field paths and local codes cross the public boundary; no submitted
// values, repository errors, or credential contents are reflected in responses.
type accountFieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

func accountInputErrors(email string, password identity.Secret, registering bool) []accountFieldError {
	var fields []accountFieldError
	if identity.ValidateEmail(email) != nil {
		fields = append(fields, accountFieldError{"email", "invalid_email"})
	}
	if password == "" {
		fields = append(fields, accountFieldError{"password", "required"})
	} else if registering && identity.ValidatePassword(password) != nil {
		fields = append(fields, accountFieldError{"password", "password_policy"})
	}
	return fields
}

func (h *Handler) accountFields(w http.ResponseWriter, status int, code string, fields []accountFieldError) {
	h.write(w, status, map[string]any{"error": map[string]any{
		"code": code, "message": "Revisa los campos indicados.",
		"request_id": w.Header().Get("X-Request-ID"), "details": fields,
	}})
}

func (h *Handler) phoneError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidPhone):
		h.fail(w, http.StatusBadRequest, "invalid_phone", "Número de teléfono móvil chileno inválido.")
	case errors.Is(err, identity.ErrPhoneCooldownActive):
		w.Header().Set("Retry-After", "60")
		h.fail(w, http.StatusTooManyRequests, "cooldown_active", "Debes esperar 60 segundos antes de solicitar un nuevo código.")
	case errors.Is(err, identity.ErrPhoneNotPending):
		h.fail(w, http.StatusBadRequest, "no_pending_verification", "No hay una verificación de teléfono pendiente.")
	case errors.Is(err, identity.ErrPhoneMaxAttempts):
		h.fail(w, http.StatusTooManyRequests, "max_attempts_exceeded", "Has superado el límite de intentos permitidos. Solicita un nuevo código.")
	case errors.Is(err, identity.ErrPhoneCodeExpired):
		h.fail(w, http.StatusUnprocessableEntity, "code_expired", "El código de verificación ha expirado. Solicita uno nuevo.")
	case errors.Is(err, identity.ErrPhoneCodeInvalid):
		h.fail(w, http.StatusUnprocessableEntity, "invalid_code", "El código de verificación es incorrecto.")
	default:
		h.fail(w, http.StatusInternalServerError, "internal_error", "Ocurrió un error inesperado al procesar la verificación telefónica.")
	}
}

func (h *Handler) deviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrNotFound):
		h.fail(w, http.StatusNotFound, "device_not_found", "El dispositivo no fue encontrado o ya ha sido revocado.")
	case errors.Is(err, identity.ErrInvalid):
		h.fail(w, http.StatusBadRequest, "invalid_device", "Identificador de dispositivo inválido.")
	default:
		h.serviceError(w, err)
	}
}

func (h *Handler) alertsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidExtraEmail):
		h.fail(w, http.StatusBadRequest, "invalid_email", "Formato de correo electrónico secundario inválido.")
	case errors.Is(err, identity.ErrExtraEmailMatchesPrimary):
		h.fail(w, http.StatusBadRequest, "email_same_as_primary", "El correo secundario no puede ser igual al correo principal.")
	case errors.Is(err, identity.ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "Cuenta no encontrada.")
	default:
		h.serviceError(w, err)
	}
}

func (h *Handler) totpError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrTOTPNotPending):
		h.fail(w, http.StatusBadRequest, "totp_not_pending", "No hay una configuración de autenticador TOTP pendiente.")
	case errors.Is(err, identity.ErrTOTPAlreadyEnabled):
		h.fail(w, http.StatusBadRequest, "totp_already_enabled", "El autenticador TOTP ya está activado.")
	case errors.Is(err, identity.ErrTOTPNotEnabled):
		h.fail(w, http.StatusBadRequest, "totp_not_enabled", "El autenticador TOTP no se encuentra activado.")
	case errors.Is(err, identity.ErrTOTPReplayDetected):
		h.fail(w, http.StatusUnprocessableEntity, "code_replayed", "Código ya utilizado. Espera al siguiente intervalo de 30 segundos.")
	case errors.Is(err, identity.ErrTOTPInvalidCode), errors.Is(err, identity.ErrTOTPInvalidBackupCode):
		h.fail(w, http.StatusUnprocessableEntity, "invalid_code", "Código de autenticación incorrecto o expirado.")
	case errors.Is(err, identity.ErrInvalid):
		h.fail(w, http.StatusBadRequest, "invalid_request", "Parámetros de solicitud inválidos.")
	case errors.Is(err, identity.ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "Cuenta no encontrada.")
	default:
		h.serviceError(w, err)
	}
}

func (h *Handler) passkeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrPasskeyChallengeExpired):
		h.fail(w, http.StatusUnprocessableEntity, "challenge_expired", "El desafío WebAuthn ha expirado. Inicia el registro nuevamente.")
	case errors.Is(err, identity.ErrPasskeyChallengeConsumed), errors.Is(err, identity.ErrPasskeyChallengeInvalid):
		h.fail(w, http.StatusUnprocessableEntity, "challenge_invalid", "El desafío WebAuthn es inválido o ya fue utilizado.")
	case errors.Is(err, identity.ErrPasskeyInvalidClientData):
		h.fail(w, http.StatusBadRequest, "invalid_client_data", "Los datos de clientDataJSON no son válidos.")
	case errors.Is(err, identity.ErrPasskeyInvalidPublicKey):
		h.fail(w, http.StatusBadRequest, "invalid_public_key", "La clave pública WebAuthn es inválida.")
	case errors.Is(err, identity.ErrPasskeyCredentialConflict):
		h.fail(w, http.StatusConflict, "credential_conflict", "Esta llave de acceso ya se encuentra registrada.")
	case errors.Is(err, identity.ErrPasskeyNotFound):
		h.fail(w, http.StatusNotFound, "passkey_not_found", "Llave de acceso no encontrada.")
	case errors.Is(err, identity.ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
	case errors.Is(err, identity.ErrInvalid):
		h.fail(w, http.StatusBadRequest, "invalid_request", "Parámetros de solicitud inválidos.")
	default:
		h.serviceError(w, err)
	}
}

func (h *Handler) qrError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrQRTicketNotFound):
		h.fail(w, http.StatusNotFound, "ticket_not_found", "Ticket de verificación QR no encontrado.")
	case errors.Is(err, identity.ErrQRTicketExpired):
		h.fail(w, http.StatusUnprocessableEntity, "ticket_expired", "El ticket de verificación QR ha expirado.")
	case errors.Is(err, identity.ErrQRTicketResolved):
		h.fail(w, http.StatusConflict, "ticket_already_resolved", "El ticket de verificación QR ya fue resuelto.")
	case errors.Is(err, identity.ErrQRTicketInvalid):
		h.fail(w, http.StatusBadRequest, "invalid_qr_token", "El token de verificación QR es inválido.")
	case errors.Is(err, identity.ErrQRDeviceNotEnabled):
		h.fail(w, http.StatusBadRequest, "qr_not_enabled", "El dispositivo QR no se encuentra activado.")
	case errors.Is(err, identity.ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "Recurso no encontrado.")
	case errors.Is(err, identity.ErrInvalid):
		h.fail(w, http.StatusBadRequest, "invalid_request", "Parámetros de solicitud inválidos.")
	default:
		h.serviceError(w, err)
	}
}

func clientIPForDevice(r *http.Request) string {
	if values := r.Header.Values("X-Espacigo-Client-IP"); len(values) == 1 && values[0] != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(values[0])); err == nil {
			return addr.Unmap().String()
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		first := strings.TrimSpace(parts[0])
		if addr, err := netip.ParseAddr(first); err == nil {
			return addr.Unmap().String()
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.Unmap().String()
		}
	}
	if addr, err := netip.ParseAddr(r.RemoteAddr); err == nil {
		return addr.Unmap().String()
	}
	return ""
}


