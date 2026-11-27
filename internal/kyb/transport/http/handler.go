package kybhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
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
	OwnForOrganization(context.Context, string, string) (kyb.Verification, error)
	Pending(context.Context) ([]kyb.Verification, error)
	ForReview(context.Context, string) (kyb.Verification, error)
	StartReview(context.Context, string, string) (kyb.Verification, error)
	Decide(context.Context, string, string, bool, string) (kyb.Verification, error)
	DecideWithDocuments(context.Context,string,string,bool,[]kyb.DocumentDecision)(kyb.Verification,error)
	IssueReviewURL(context.Context, string, string, time.Time) (string, time.Time, error)
	ReadSignedDocument(context.Context, string, string, string, int64, time.Time) ([]byte, string, error)
	SubmitPersonalSII(context.Context, string, string, kyb.Upload) (kyb.ComplianceDocument, error)
	PersonalCompliance(context.Context, string) (kyb.ComplianceProfile, error)
	SubmitSpaceProperty(context.Context, string, string, kyb.Upload) (kyb.ComplianceDocument, error)
	SubmitSpaceCategorySII(context.Context, string, string, string, kyb.Upload) (kyb.ComplianceDocument, error)
	SpaceEligibility(context.Context, string, string) (kyb.SpaceEligibility, error)
	ComplianceProfiles(context.Context, string) ([]kyb.ComplianceProfile, error)
	SetComplianceDisabled(context.Context, string, string, string, bool) error
	SetOrganizationDisabled(context.Context, string, string, string, bool) error
	PendingComplianceDocuments(context.Context) ([]kyb.ComplianceDocument, error)
	StartComplianceReview(context.Context, string, string) (kyb.ComplianceDocument, error)
	DecideComplianceDocument(context.Context, string, string, bool, string) (kyb.ComplianceDocument, error)
	IssueComplianceReviewURL(context.Context, string, time.Time) (string, time.Time, error)
	ReadSignedComplianceDocument(context.Context, string, string, string, int64, time.Time) ([]byte, string, error)
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
	if strings.HasPrefix(path, "/api/v1/kyb/personal/") || strings.HasPrefix(path, "/api/v1/kyb/spaces/") || path == "/api/v1/admin/kyb/compliance" || strings.HasPrefix(path, "/api/v1/admin/kyb/compliance/") {
		h.serveCompliance(w, r, path)
		return
	}
	if path == "/api/v1/kyb/documents" || strings.HasPrefix(path, "/api/v1/kyb/documents/") || strings.HasPrefix(path, "/api/v1/kyb/compliance-documents/") {
		h.serveSignedDocument(w, r, path)
		return
	}
	if path == "/api/v1/kyb/verifications" {
		principal, ok := h.authorize(w, r, "")
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodPost:
			input, err := parseSubmission(w, r)
			if strings.TrimSpace(input.OrganizationID) == "" && !slices.Contains(principal.Roles, identity.RoleLandlord) {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request")
				return
			}
			result, err := h.service.Submit(r.Context(), principal.AccountID, input)
			writeServiceResult(w, result, err, http.StatusCreated)
		case http.MethodGet:
			organizationID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
			var result kyb.Verification
			var err error
			if organizationID != "" {
				result, err = h.service.OwnForOrganization(r.Context(), principal.AccountID, organizationID)
			} else if !slices.Contains(principal.Roles, identity.RoleLandlord) {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			} else {
				result, err = h.service.Own(r.Context(), principal.AccountID)
			}
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

func (h *Handler) serveCompliance(w http.ResponseWriter, r *http.Request, path string) {
	if strings.HasPrefix(path, "/api/v1/admin/kyb/compliance") {
		principal, ok := h.authorize(w, r, identity.RoleAdministrator)
		if !ok {
			return
		}
		if path == "/api/v1/admin/kyb/compliance" && r.Method == http.MethodGet {
			items, err := h.service.ComplianceProfiles(r.Context(), r.URL.Query().Get("status"))
			writeServiceResult(w, map[string]any{"items": items}, err, http.StatusOK)
			return
		}
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/admin/kyb/compliance/"), "/")
		if path == "/api/v1/admin/kyb/compliance/documents" && r.Method == http.MethodGet {
			items, err := h.service.PendingComplianceDocuments(r.Context())
			writeServiceResult(w, map[string]any{"items": items}, err, http.StatusOK)
			return
		}
		if len(parts) == 3 && parts[0] == "documents" && kyb.ValidUUID(parts[1]) && parts[2] == "url" && r.Method == http.MethodGet {
			url, expires, err := h.service.IssueComplianceReviewURL(r.Context(), parts[1], time.Now().UTC())
			writeServiceResult(w, map[string]any{"download_url": url, "expires_at": expires}, err, http.StatusOK)
			return
		}
		if len(parts) == 3 && parts[0] == "documents" && kyb.ValidUUID(parts[1]) && parts[2] == "review" && r.Method == http.MethodPost {
			item, err := h.service.StartComplianceReview(r.Context(), parts[1], principal.AccountID)
			writeServiceResult(w, item, err, http.StatusOK)
			return
		}
		if len(parts) == 3 && parts[0] == "documents" && kyb.ValidUUID(parts[1]) && parts[2] == "decision" && r.Method == http.MethodPost {
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
			item, err := h.service.DecideComplianceDocument(r.Context(), parts[1], principal.AccountID, input.Decision == "APPROVE", input.RejectionReason)
			writeServiceResult(w, item, err, http.StatusOK)
			return
		}
		if len(parts) == 3 && parts[0] == "organizations" && kyb.ValidUUID(parts[1]) && (parts[2] == "disable" || parts[2] == "enable") && r.Method == http.MethodPost {
			var input struct {
				Reason string `json:"reason"`
			}
			if parts[2] == "disable" && !decodeJSON(w, r, &input) {
				return
			}
			err := h.service.SetOrganizationDisabled(r.Context(), parts[1], principal.AccountID, input.Reason, parts[2] == "disable")
			writeServiceResult(w, map[string]bool{"updated": err == nil}, err, http.StatusOK)
			return
		}
		if len(parts) == 2 && kyb.ValidUUID(parts[0]) && (parts[1] == "disable" || parts[1] == "enable") && r.Method == http.MethodPost {
			var input struct {
				Reason string `json:"reason"`
			}
			if parts[1] == "disable" && !decodeJSON(w, r, &input) {
				return
			}
			err := h.service.SetComplianceDisabled(r.Context(), parts[0], principal.AccountID, input.Reason, parts[1] == "disable")
			writeServiceResult(w, map[string]bool{"updated": err == nil}, err, http.StatusOK)
			return
		}
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	principal, ok := h.authorize(w, r, "")
	if !ok {
		return
	}
	if path == "/api/v1/kyb/personal/compliance" && r.Method == http.MethodGet {
		item, err := h.service.PersonalCompliance(r.Context(), principal.AccountID)
		writeServiceResult(w, item, err, http.StatusOK)
		return
	}
	if path == "/api/v1/kyb/personal/sii" && r.Method == http.MethodPost {
		fields := map[string]bool{"sii_start_activities": true}
		values, uploads, err := parseSingleUpload(w, r, fields)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		giro := values.Get("sii_giro_code")
		if len(uploads) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		item, err := h.service.SubmitPersonalSII(r.Context(), principal.AccountID, giro, uploads[0])
		writeServiceResult(w, item, err, http.StatusCreated)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/kyb/spaces/"), "/")
	if len(parts) == 2 && kyb.ValidUUID(parts[0]) && parts[1] == "verification" && r.Method == http.MethodPost {
		_, uploads, err := parseSingleUpload(w, r, map[string]bool{"property_or_sublease_authorization": true})
		if err != nil || len(uploads) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		item, err := h.service.SubmitSpaceProperty(r.Context(), principal.AccountID, parts[0], uploads[0])
		writeServiceResult(w, item, err, http.StatusCreated)
		return
	}
	if len(parts) == 2 && kyb.ValidUUID(parts[0]) && parts[1] == "sii" && r.Method == http.MethodPost {
		values, uploads, err := parseSingleUpload(w, r, map[string]bool{"sii_start_activities": true})
		if err != nil || len(uploads) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		item, err := h.service.SubmitSpaceCategorySII(r.Context(), principal.AccountID, parts[0], values.Get("sii_giro_code"), uploads[0])
		writeServiceResult(w, item, err, http.StatusCreated)
		return
	}
	if len(parts) == 2 && kyb.ValidUUID(parts[0]) && parts[1] == "eligibility" && r.Method == http.MethodGet {
		item, err := h.service.SpaceEligibility(r.Context(), principal.AccountID, parts[0])
		writeServiceResult(w, item, err, http.StatusOK)
		return
	}
	writeError(w, http.StatusNotFound, "not_found")
}

func parseSingleUpload(w http.ResponseWriter, r *http.Request, allowed map[string]bool) (url.Values, []kyb.Upload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, kyb.MaxDocumentBytes+(64<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil || r.MultipartForm == nil {
		return nil, nil, kyb.ErrInvalid
	}
	defer r.MultipartForm.RemoveAll()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return nil, nil, kyb.ErrInvalid
	}
	for field, files := range r.MultipartForm.File {
		if !allowed[field] || len(files) != 1 {
			return nil, nil, kyb.ErrInvalid
		}
	}
	uploads := make([]kyb.Upload, 0, len(allowed))
	for field := range allowed {
		files := r.MultipartForm.File[field]
		if len(files) != 1 {
			continue
		}
		file, err := files[0].Open()
		if err != nil {
			return nil, nil, kyb.ErrInvalid
		}
		content, readErr := io.ReadAll(io.LimitReader(file, kyb.MaxDocumentBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(content) > kyb.MaxDocumentBytes {
			return nil, nil, kyb.ErrInvalid
		}
		uploads = append(uploads, kyb.Upload{Kind: kyb.DocumentKind(field), MIME: files[0].Header.Get("Content-Type"), Content: content})
	}
	return r.MultipartForm.Value, uploads, nil
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
			Documents       json.RawMessage `json:"documents"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.Decision != "APPROVE" && input.Decision != "REJECT" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input")
			return
		}
		var result kyb.Verification
		var err error
		if input.Documents!=nil {
			if string(input.Documents)=="null"||input.RejectionReason!="" {writeError(w,http.StatusUnprocessableEntity,"invalid_input");return}
			var docs []struct{DocumentID string `json:"document_id"`;Decision string `json:"decision"`;RejectionReason string `json:"rejection_reason"`}
			decoder:=json.NewDecoder(bytes.NewReader(input.Documents));decoder.DisallowUnknownFields();if decoder.Decode(&docs)!=nil{writeError(w,http.StatusUnprocessableEntity,"invalid_input");return}
			decisions:=make([]kyb.DocumentDecision,0,len(docs));for _,doc:=range docs{status:=kyb.Status(doc.Decision);if doc.Decision=="APPROVE"{status=kyb.StatusApproved}else if doc.Decision=="REJECT"{status=kyb.StatusRejected}else{writeError(w,http.StatusUnprocessableEntity,"invalid_input");return};decisions=append(decisions,kyb.DocumentDecision{DocumentID:doc.DocumentID,Status:status,RejectionReason:doc.RejectionReason})}
			result,err=h.service.DecideWithDocuments(r.Context(),parts[0],principal.AccountID,input.Decision=="APPROVE",decisions)
		}else{result,err=h.service.Decide(r.Context(),parts[0],principal.AccountID,input.Decision=="APPROVE",input.RejectionReason)}
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
	if strings.HasPrefix(path, "/api/v1/kyb/compliance-documents/") {
		storageRef = strings.TrimPrefix(path, "/api/v1/kyb/compliance-documents/")
	}
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
	var content []byte
	var contentType string
	if strings.HasPrefix(path, "/api/v1/kyb/compliance-documents/") {
		content, contentType, err = h.service.ReadSignedComplianceDocument(r.Context(), storageRef, values.Get("key_version"), values.Get("signature"), expires, time.Now().UTC())
	} else {
		content, contentType, err = h.service.ReadSignedDocument(r.Context(), storageRef, values.Get("key_version"), values.Get("signature"), expires, time.Now().UTC())
	}
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
		OrganizationID: r.FormValue("organization_id"),
		LegalName:      r.FormValue("legal_name"), CompanyRUT: r.FormValue("company_rut"),
		RepresentativeName: r.FormValue("representative_name"), RepresentativeRUT: r.FormValue("representative_rut"),
		SIIGiroCode: r.FormValue("sii_giro_code"), IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Documents: make([]kyb.Upload, 0, 4),
	}
	for field, kind := range map[string]kyb.DocumentKind{
		"erut":                               kyb.DocumentERUT,
		"sii_start_activities":               kyb.DocumentSIIStartActivities,
		"power_certificate":                  kyb.DocumentPowerCertificate,
		"property_or_sublease_authorization": kyb.DocumentPropertyOrSublease,
	} {
		files := r.MultipartForm.File[field]
		if (field == "sii_start_activities" || field == "property_or_sublease_authorization") && len(files) == 0 {
			continue
		}
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
	if len(input.Documents) != 3 {
		return kyb.SubmissionInput{}, kyb.ErrInvalid
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
