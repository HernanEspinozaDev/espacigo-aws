package commercialrentaldocumenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialrentaldocument"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type Handler struct {
	auth    Authenticator
	service *domain.Service
	origins map[string]bool
}

func New(auth Authenticator, service *domain.Service, origins []string) http.Handler {
	h := &Handler{auth: auth, service: service, origins: map[string]bool{}}
	for _, origin := range origins {
		h.origins[origin] = true
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	requestID, err := (credentials.Generator{}).ID()
	if err != nil {
		fail(w, 500, "internal_error")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			fail(w, 403, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Disposition, ETag")
	}
	const prefix = "/api/v1/commercial-reservations/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/rental-tax-document") {
		fail(w, 404, "not_found")
		return
	}
	reservationID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/rental-tax-document")
	if reservationID == "" || strings.Contains(reservationID, "/") {
		fail(w, 404, "not_found")
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, PUT, OPTIONS")
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT, OPTIONS")
		fail(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" {
		fail(w, 400, "invalid_request")
		return
	}
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") || len(r.Header.Values("Authorization")) != 1 {
		fail(w, 401, "unauthenticated")
		return
	}
	principal, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(bearer, "Bearer ")), "", identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			fail(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, 401, "unauthenticated")
		}
		return
	}
	if r.Method == http.MethodPut {
		h.upload(w, r, principal.AccountID, reservationID)
		return
	}
	h.download(w, r, principal.AccountID, reservationID)
}

type attachmentResponse struct {
	ReservationID string `json:"reservation_id"`
	State         string `json:"state"`
	SHA256        string `json:"sha256"`
	SizeBytes     int64  `json:"size_bytes"`
	UploadedAt    string `json:"uploaded_at"`
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request, actor, reservationID string) {
	if err := h.service.AuthorizeUpload(r.Context(), actor, reservationID); err != nil {
		replyError(w, err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/pdf" || (r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity") {
		fail(w, 415, "unsupported_media_type")
		return
	}
	if r.ContentLength > domain.MaxPDFBytes {
		fail(w, 413, "payload_too_large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxPDFBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			fail(w, 413, "payload_too_large")
		} else {
			fail(w, 400, "invalid_request")
		}
		return
	}
	doc, err := h.service.Upload(r.Context(), actor, reservationID, raw)
	if err != nil {
		replyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(attachmentResponse{ReservationID: doc.ReservationID, State: "ready", SHA256: doc.SHA256, SizeBytes: doc.Size, UploadedAt: doc.UploadedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request, actor, reservationID string) {
	if r.ContentLength > 0 {
		fail(w, 400, "invalid_request")
		return
	}
	blob, doc, err := h.service.Download(r.Context(), actor, reservationID)
	if err != nil {
		replyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="documento-tributario-arriendo-`+reservationID+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
	w.Header().Set("ETag", `"sha256-`+doc.SHA256+`"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func replyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		fail(w, 404, "not_found")
	case errors.Is(err, domain.ErrConflict):
		fail(w, 409, "document_already_attached")
	case errors.Is(err, domain.ErrInvalid):
		fail(w, 422, "invalid_document")
	case errors.Is(err, domain.ErrInspectionUnavailable):
		w.Header().Set("Retry-After", "30")
		fail(w, 503, "document_inspection_unavailable")
	default:
		w.Header().Set("Retry-After", "30")
		fail(w, 503, "document_unavailable")
	}
}

func fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")}})
}
