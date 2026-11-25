package kybhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type ApplicationService interface {
	Submit(context.Context, string, kyb.SubmissionInput) (kyb.Verification, error)
	Own(context.Context, string) (kyb.Verification, error)
	Pending(context.Context) ([]kyb.Verification, error)
	ForReview(context.Context, string) (kyb.Verification, error)
	StartReview(context.Context, string, string) (kyb.Verification, error)
	Decide(context.Context, string, string, bool, string) (kyb.Verification, error)
	IssueReviewURL(context.Context, string, string, time.Time) (string, time.Time, error)
	ReadSignedDocument(context.Context, string, string, string, int64, time.Time) ([]byte, string, error)
}

type Handler struct {
	auth    Authenticator
	service ApplicationService
	origins map[string]bool
}

func NewHandler(auth Authenticator, service ApplicationService, origins []string) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return &Handler{auth: auth, service: service, origins: allowed}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			writeError(w, http.StatusForbidden, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h == nil || h.auth == nil || h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "/api/v1/kyb/documents" || strings.HasPrefix(path, "/api/v1/kyb/documents/") {
		h.serveSignedDocument(w, r, path)
		return
	}
	if path == "/api/v1/kyb/verifications" {
		principal, ok := h.authorize(w, r, identity.RoleLandlord)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodPost:
			input, err := parseSubmission(w, r)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request")
				return
			}
			result, err := h.service.Submit(r.Context(), principal.AccountID, input)
			writeServiceResult(w, result, err, http.StatusCreated)
		case http.MethodGet:
			result, err := h.service.Own(r.Context(), principal.AccountID)
			writeServiceResult(w, result, err, http.StatusOK)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}
	if path == "/api/v1/admin/kyb/verifications" {
		if _, ok := h.authorize(w, r, identity.RoleAdministrator); !ok {
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		items, err := h.service.Pending(r.Context())
		writeServiceResult(w, map[string]any{"items": items}, err, http.StatusOK)
		return
	}
	if strings.HasPrefix(path, "/api/v1/admin/kyb/verifications/") {
		if _, ok := h.authorize(w, r, identity.RoleAdministrator); !ok {
			return
		}
		h.serveAdmin(w, r, path)
		return
	}
	writeError(w, http.StatusNotFound, "not_found")
}

func (h *Handler) serveAdmin(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/admin/kyb/verifications/"), "/")
	if len(parts) == 1 && kyb.ValidUUID(parts[0]) && r.Method == http.MethodGet {
		result, err := h.service.ForReview(r.Context(), parts[0])
		writeServiceResult(w, result, err, http.StatusOK)
		return
	}
	if len(parts) == 2 && kyb.ValidUUID(parts[0]) && parts[1] == "review" && r.Method == http.MethodPost {
		principal, ok := h.authorize(w, r, identity.RoleAdministrator)
		if !ok {
			return
		}
		result, err := h.service.StartReview(r.Context(), parts[0], principal.AccountID)
		writeServiceResult(w, result, err, http.StatusOK)
		return
	}
	if len(parts) == 2 && kyb.ValidUUID(parts[0]) && parts[1] == "decision" && r.Method == http.MethodPost {
		principal, ok := h.authorize(w, r, identity.RoleAdministrator)
		if !ok {
			return
		}
		var input struct {
			Decision        string `json:"decision"`
			RejectionReason string `json:"rejection_reason"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.Decision != "APPROVE" && input.Decision != "REJECT" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input")
			return
		}
		result, err := h.service.Decide(r.Context(), parts[0], principal.AccountID, input.Decision == "APPROVE", input.RejectionReason)
		writeServiceResult(w, result, err, http.StatusOK)
		return
	}
	if len(parts) == 4 && kyb.ValidUUID(parts[0]) && parts[1] == "documents" && kyb.ValidUUID(parts[2]) && parts[3] == "url" && r.Method == http.MethodGet {
		url, expiresAt, err := h.service.IssueReviewURL(r.Context(), parts[0], parts[2], time.Now().UTC())
		writeServiceResult(w, map[string]any{"download_url": url, "expires_at": expiresAt}, err, http.StatusOK)
		return
	}
	writeError(w, http.StatusNotFound, "not_found")
}

func (h *Handler) serveSignedDocument(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodGet || r.URL.RawQuery == "" {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	storageRef := strings.TrimPrefix(path, "/api/v1/kyb/documents/")
	if !kyb.ValidUUID(storageRef) || strings.Contains(storageRef, "/") {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	values := r.URL.Query()
	if len(values) != 3 || len(values["expires"]) != 1 || len(values["key_version"]) != 1 || len(values["signature"]) != 1 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	expires, err := strconv.ParseInt(values.Get("expires"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	content, contentType, err := h.service.ReadSignedDocument(r.Context(), storageRef, values.Get("key_version"), values.Get("signature"), expires, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid_or_expired_link")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="kyb-document"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, role identity.Role) (identity.Principal, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return identity.Principal{}, false
	}
	principal, err := h.auth.Authorize(r.Context(), identity.Secret(parts[1]), role, identity.UserOperation)
	if err != nil {
		status := http.StatusUnauthorized
		code := "unauthenticated"
		if errors.Is(err, identity.ErrForbidden) {
			status, code = http.StatusForbidden, "forbidden"
		}
		writeError(w, status, code)
		return identity.Principal{}, false
	}
	return principal, true
}

func parseSubmission(w http.ResponseWriter, r *http.Request) (kyb.SubmissionInput, error) {
	const maxRequestBytes = 3*kyb.MaxDocumentBytes + (64 << 10)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil || r.MultipartForm == nil {
		return kyb.SubmissionInput{}, kyb.ErrInvalid
	}
	defer r.MultipartForm.RemoveAll()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return kyb.SubmissionInput{}, kyb.ErrInvalid
	}
	input := kyb.SubmissionInput{
		LegalName: r.FormValue("legal_name"), CompanyRUT: r.FormValue("company_rut"),
		RepresentativeName: r.FormValue("representative_name"), RepresentativeRUT: r.FormValue("representative_rut"),
		SIIGiroCode: r.FormValue("sii_giro_code"), IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Documents: make([]kyb.Upload, 0, 3),
	}
	for field, kind := range map[string]kyb.DocumentKind{
		"erut":                               kyb.DocumentERUT,
		"power_certificate":                  kyb.DocumentPowerCertificate,
		"property_or_sublease_authorization": kyb.DocumentPropertyOrSublease,
	} {
		files := r.MultipartForm.File[field]
		if len(files) != 1 {
			return kyb.SubmissionInput{}, kyb.ErrInvalid
		}
		file, openErr := files[0].Open()
		if openErr != nil {
			return kyb.SubmissionInput{}, kyb.ErrInvalid
		}
		content, readErr := io.ReadAll(io.LimitReader(file, kyb.MaxDocumentBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(content) > kyb.MaxDocumentBytes {
			return kyb.SubmissionInput{}, kyb.ErrInvalid
		}
		input.Documents = append(input.Documents, kyb.Upload{Kind: kind, MIME: files[0].Header.Get("Content-Type"), Content: content})
	}
	return input, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func writeServiceResult(w http.ResponseWriter, result any, err error, successStatus int) {
	if err != nil {
		switch {
		case errors.Is(err, kyb.ErrInvalid):
			writeError(w, http.StatusUnprocessableEntity, "invalid_input")
		case errors.Is(err, kyb.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found")
		case errors.Is(err, kyb.ErrConflict):
			writeError(w, http.StatusConflict, "state_conflict")
		case errors.Is(err, kyb.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(successStatus)
	_ = json.NewEncoder(w).Encode(result)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}
