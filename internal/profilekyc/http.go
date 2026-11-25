package profilekyc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

const BaseRoute = "/api/v1/account/identity-profile"
const WebhookRoute = "/api/v1/webhooks/profile-identity"

var hostReturnPath = regexp.MustCompile(`(?i)^/anfitrion/espacios(?:/nuevo|/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/editar(?:\?tab=(?:info|location|pricing|schedule|fotos|photos|contrato|contract))?)?$`)

type Authorizer interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type DecodeNotice func(http.Header, []byte) (Notice, error)

type RecentAuthentication interface {
	Verify(context.Context, string, string, string) error
}

func Register(mux *http.ServeMux, auth Authorizer, s *Verifier, decodeNotice DecodeNotice, recent RecentAuthentication) {
	mux.HandleFunc("GET "+BaseRoute, withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		out, err := s.Status(r.Context(), id)
		writeResult(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("PUT "+BaseRoute, withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		var data Data
		if !readJSON(w, r, &data) {
			return
		}
		out, err := s.Save(r.Context(), id, data)
		writeResult(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("PATCH "+BaseRoute+"/draft", withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		var data Data
		if !readJSON(w, r, &data) {
			return
		}
		writeResult(w, http.StatusOK, map[string]bool{"saved": true}, s.SaveDraft(r.Context(), id, data))
	}))
	mux.HandleFunc("POST "+BaseRoute+"/draft/restore", withPrincipal(auth, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var input struct {
			CurrentPassword string `json:"current_password"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if recent == nil || recent.Verify(r.Context(), p.AccountID, p.SessionID, "password:"+input.CurrentPassword) != nil {
			writeError(w, 403, "reauthentication_required", nil)
			return
		}
		out, err := s.ReadDraft(r.Context(), p.AccountID)
		writeResult(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("PATCH "+BaseRoute+"/username", withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		var in struct {
			Username string `json:"username"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		out, err := s.SaveUsername(r.Context(), id, in.Username)
		writeResult(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("POST "+BaseRoute+"/verification", withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		var input struct {
			ReturnTo string `json:"return_to"`
		}
		if r.ContentLength != 0 && !readJSON(w, r, &input) {
			return
		}
		if input.ReturnTo != "" && !hostReturnPath.MatchString(input.ReturnTo) {
			writeError(w, 422, "invalid_return_path", nil)
			return
		}
		out, err := s.Start(r.Context(), id, input.ReturnTo)
		writeResult(w, http.StatusCreated, map[string]string{"url": out.URL}, err)
	}))
	mux.HandleFunc("POST "+BaseRoute+"/verification/refresh", withAccount(auth, func(w http.ResponseWriter, r *http.Request, id string) {
		out, err := s.Refresh(r.Context(), id)
		writeResult(w, http.StatusOK, out, err)
	}))
	if decodeNotice != nil {
		mux.HandleFunc("POST "+WebhookRoute, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil {
				writeError(w, 400, "invalid_webhook", nil)
				return
			}
			notice, err := decodeNotice(r.Header, body)
			if err != nil {
				writeError(w, 400, "invalid_webhook", nil)
				return
			}
			err = s.ProcessNotice(r.Context(), notice)
			if err != nil {
				writeError(w, 503, "identity_verification_unavailable", nil)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func withAccount(auth Authorizer, fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return withPrincipal(auth, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		fn(w, r, p.AccountID)
	})
}

func withPrincipal(auth Authorizer, fn func(http.ResponseWriter, *http.Request, identity.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		header, token, valid := strings.Cut(r.Header.Get("Authorization"), " ")
		if !valid || !strings.EqualFold(header, "Bearer") || strings.TrimSpace(token) == "" {
			writeError(w, 401, "unauthenticated", nil)
			return
		}
		principal, err := auth.Authorize(r.Context(), identity.Secret(token), "", identity.UserOperation)
		if principal.RestrictedMode || errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrRestrictedMode) {
			writeError(w, 403, "forbidden", nil)
			return
		}
		if err != nil || principal.AccountID == "" {
			writeError(w, 401, "unauthenticated", nil)
			return
		}
		fn(w, r, principal)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err == nil {
		var extra any
		if decoder.Decode(&extra) == io.EOF {
			return true
		}
	}
	writeError(w, 400, "invalid_json", nil)
	return false
}

func writeResult(w http.ResponseWriter, status int, result any, err error) {
	var fieldError *FieldError
	switch {
	case err == nil:
		writeJSON(w, status, result)
	case errors.As(err, &fieldError):
		writeError(w, 422, "invalid_identity_profile", fieldError.Fields)
	case errors.Is(err, ErrUsernameTaken):
		writeError(w, 409, "username_taken", map[string]string{"username": "username_taken"})
	case errors.Is(err, ErrRUTLocked):
		writeError(w, 409, "rut_locked", map[string]string{"rut": "rut_locked"})
	case errors.Is(err, ErrNotReady):
		writeError(w, 409, "profile_incomplete", nil)
	case errors.Is(err, ErrConflict):
		writeError(w, 409, "identity_verification_conflict", nil)
	default:
		writeError(w, 503, "identity_verification_unavailable", nil)
	}
}

func writeError(w http.ResponseWriter, status int, code string, fields map[string]string) {
	details := make([]map[string]string, 0, len(fields))
	names := make([]string, 0, len(fields))
	for field := range fields {
		names = append(names, field)
	}
	sort.Strings(names)
	for _, field := range names {
		details = append(details, map[string]string{"field": field, "code": fields[field]})
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": "La operación de identidad no pudo completarse.", "details": details}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
