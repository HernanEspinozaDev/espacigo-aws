package organizationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/organizations"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Application interface {
	List(context.Context, string) ([]organizations.Organization, error)
	Create(context.Context, string, string, string, string) (organizations.Organization, error)
	Get(context.Context, string, string) (organizations.Organization, error)
	Update(context.Context, string, string, string, string, string) (organizations.Organization, error)
	Members(context.Context, string, string) ([]organizations.Member, error)
	Invite(context.Context, string, string, string, organizations.Role) (organizations.Invitation, string, error)
	Accept(context.Context, string, string, string, string) (organizations.Member, error)
	Revoke(context.Context, string, string, string) error
	SetRole(context.Context, string, string, string, organizations.Role) error
	RequestJoin(context.Context, string, string, organizations.Role) (organizations.JoinRequest, error)
	OwnJoinRequest(context.Context, string, string) (organizations.JoinRequest, error)
	JoinRequests(context.Context, string, string) ([]organizations.JoinRequest, error)
	ApproveJoinRequest(context.Context, string, string, string) (organizations.Invitation, string, error)
	RejectJoinRequest(context.Context, string, string, string) error
}
type Handler struct {
	auth Authenticator
	app  Application
}

func NewHandler(auth Authenticator, app Application) http.Handler {
	return &Handler{auth: auth, app: app}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if h == nil || h.auth == nil || h.app == nil {
		writeError(w, 503, "service_unavailable")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/host/organizations"), "/"), "/")
	if r.URL.Path == "/api/v1/host/organizations" || r.URL.Path == "/api/v1/host/organizations/" {
		p, ok := h.authorize(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := h.app.List(r.Context(), p.AccountID)
			writeResult(w, map[string]any{"items": items}, err, 200)
		case http.MethodPost:
			var in struct {
				LegalName  string `json:"legal_name"`
				TradeName  string `json:"trade_name"`
				CompanyRUT string `json:"company_rut"`
			}
			if !decode(w, r, &in) {
				return
			}
			o, err := h.app.Create(r.Context(), p.AccountID, in.LegalName, in.TradeName, in.CompanyRUT)
			writeResult(w, map[string]any{"organization": o}, err, 201)
		default:
			method(w, "GET, POST")
		}
		return
	}
	const joinRequestsPath = "/api/v1/host/organization-join-requests"
	if r.URL.Path == joinRequestsPath {
		if r.Method != http.MethodPost {
			method(w, "POST")
			return
		}
		p, ok := h.authorize(w, r)
		if !ok {
			return
		}
		var in struct {
			CompanyRUT string             `json:"company_rut"`
			Role       organizations.Role `json:"rol_propuesto"`
		}
		if !decode(w, r, &in) {
			return
		}
		item, err := h.app.RequestJoin(r.Context(), p.AccountID, in.CompanyRUT, in.Role)
		writeResult(w, map[string]any{"request": item}, err, 202)
		return
	}
	if strings.HasPrefix(r.URL.Path, joinRequestsPath+"/") {
		if r.Method != http.MethodGet {
			method(w, "GET")
			return
		}
		p, ok := h.authorize(w, r)
		if !ok {
			return
		}
		requestID := strings.TrimPrefix(r.URL.Path, joinRequestsPath+"/")
		if strings.Contains(requestID, "/") {
			writeError(w, 404, "not_found")
			return
		}
		item, err := h.app.OwnJoinRequest(r.Context(), p.AccountID, requestID)
		writeResult(w, map[string]any{"request": item}, err, 200)
		return
	}
	p, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, 404, "not_found")
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			o, err := h.app.Get(r.Context(), p.AccountID, id)
			writeResult(w, map[string]any{"organization": o}, err, 200)
		case http.MethodPatch:
			var in struct {
				LegalName     string `json:"legal_name"`
				TradeName     string `json:"trade_name"`
				PublicSummary string `json:"public_summary"`
			}
			if !decode(w, r, &in) {
				return
			}
			o, err := h.app.Update(r.Context(), p.AccountID, id, in.LegalName, in.TradeName, in.PublicSummary)
			writeResult(w, map[string]any{"organization": o}, err, 200)
		default:
			method(w, "GET, PATCH")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "members" {
		if r.Method != http.MethodGet {
			method(w, "GET")
			return
		}
		items, err := h.app.Members(r.Context(), p.AccountID, id)
		writeResult(w, map[string]any{"items": items}, err, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "join-requests" {
		if r.Method != http.MethodGet {
			method(w, "GET")
			return
		}
		items, err := h.app.JoinRequests(r.Context(), p.AccountID, id)
		writeResult(w, map[string]any{"items": items}, err, 200)
		return
	}
	if len(parts) == 4 && parts[1] == "join-requests" && r.Method == http.MethodPost {
		switch parts[3] {
		case "approve":
			inv, token, err := h.app.ApproveJoinRequest(r.Context(), p.AccountID, id, parts[2])
			writeResult(w, map[string]any{"invitation": inv, "invitation_token": token}, err, 200)
		case "reject":
			err := h.app.RejectJoinRequest(r.Context(), p.AccountID, id, parts[2])
			writeErrorResult(w, err, 204)
		default:
			writeError(w, 404, "not_found")
		}
		return
	}
	if len(parts) == 3 && parts[1] == "members" && r.Method == http.MethodDelete {
		err := h.app.Revoke(r.Context(), p.AccountID, id, parts[2])
		writeErrorResult(w, err, 204)
		return
	}
	if len(parts) == 3 && parts[1] == "members" && r.Method == http.MethodPatch {
		var in struct {
			Role organizations.Role `json:"rol"`
		}
		if !decode(w, r, &in) {
			return
		}
		err := h.app.SetRole(r.Context(), p.AccountID, id, parts[2], in.Role)
		writeErrorResult(w, err, 204)
		return
	}
	if len(parts) == 2 && parts[1] == "invitations" {
		if r.Method != http.MethodPost {
			method(w, "POST")
			return
		}
		var in struct {
			Email string             `json:"email"`
			Role  organizations.Role `json:"rol"`
		}
		if !decode(w, r, &in) {
			return
		}
		inv, token, err := h.app.Invite(r.Context(), p.AccountID, id, in.Email, in.Role)
		writeResult(w, map[string]any{"invitation": inv, "invitation_token": token}, err, 201)
		return
	}
	if len(parts) == 4 && parts[1] == "invitations" && parts[3] == "accept" {
		if r.Method != http.MethodPost {
			method(w, "POST")
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &in) {
			return
		}
		m, err := h.app.Accept(r.Context(), p.AccountID, id, parts[2], in.Token)
		writeResult(w, map[string]any{"member": m}, err, 200)
		return
	}
	writeError(w, 404, "not_found")
}
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, 401, "unauthenticated")
		return identity.Principal{}, false
	}
	// Either account role is accepted: organization work begins before a user's
	// first listing, so it must not require the global landlord role.
	p, err := h.auth.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleTenant, identity.UserOperation)
	if errors.Is(err, identity.ErrForbidden) {
		p, err = h.auth.Authorize(r.Context(), identity.Secret(parts[1]), identity.RoleLandlord, identity.UserOperation)
	}
	if err != nil {
		status, code := 401, "unauthenticated"
		if errors.Is(err, identity.ErrForbidden) {
			status, code = 403, "forbidden"
		}
		writeError(w, status, code)
		return identity.Principal{}, false
	}
	return p, true
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil {
		writeError(w, 400, "invalid_request")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_request")
		return false
	}
	return true
}
func writeResult(w http.ResponseWriter, v any, err error, success int) {
	if err != nil {
		writeErrorResult(w, err, success)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(success)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErrorResult(w http.ResponseWriter, err error, success int) {
	if err == nil {
		w.WriteHeader(success)
		return
	}
	status, code := 500, "internal_error"
	switch {
	case errors.Is(err, organizations.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, organizations.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, organizations.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, organizations.ErrConflict):
		status, code = 409, "conflict"
	}
	writeError(w, status, code)
}
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func method(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, 405, "method_not_allowed")
}
