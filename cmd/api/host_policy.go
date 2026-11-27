package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

type hostPolicyDatabase interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type hostPolicyAuthenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}

type hostPolicy struct {
	db       hostPolicyDatabase
	auth     hostPolicyAuthenticator
	document identity.ConsentDocument
}

func (p hostPolicy) status(ctx context.Context, actor string, accept bool) (accepted, available bool, err error) {
	const current = `SELECT id FROM public.version_terminos WHERE id=$2 AND codigo=$3 AND tipo='terminos'
	 AND hash_sha256=$4 AND publicada_en<=now()`
	if accept {
		// ON CONFLICT makes simultaneous confirmations and retries idempotent.
		err = p.db.QueryRow(ctx, `WITH current AS (`+current+`), recorded AS (
		  INSERT INTO public.aceptacion_terminos(id,usuario_id,version_id,canal)
		  SELECT gen_random_uuid(),$1,id,'web' FROM current
		  ON CONFLICT (usuario_id,version_id) DO NOTHING
		) SELECT EXISTS(SELECT 1 FROM current)`,
			actor, p.document.ID, p.document.Code, p.document.SHA256).Scan(&available)
		return available, available, err
	}
	err = p.db.QueryRow(ctx, `WITH current AS (`+current+`)
	 SELECT EXISTS(SELECT 1 FROM public.aceptacion_terminos a JOIN current c ON c.id=a.version_id WHERE a.usuario_id=$1),
	        EXISTS(SELECT 1 FROM current)`,
		actor, p.document.ID, p.document.Code, p.document.SHA256).Scan(&accepted, &available)
	return accepted, available, err
}

func (p hostPolicy) principal(r *http.Request) (identity.Principal, error) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	return p.auth.Authorize(r.Context(), identity.Secret(token), "", identity.UserOperation)
}

func (p hostPolicy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, err := p.principal(r)
	if err != nil {
		writeHostPolicyError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	accept := r.Method == http.MethodPost
	if accept {
		var input struct {
			VersionID string `json:"version_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		var trailing any
		if decoder.Decode(&input) != nil || decoder.Decode(&trailing) != io.EOF {
			writeHostPolicyError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		if input.VersionID != p.document.ID {
			writeHostPolicyError(w, http.StatusConflict, "host_policy_version_changed")
			return
		}
	}
	accepted, available, err := p.status(r.Context(), principal.AccountID, accept)
	if err != nil || !available {
		writeHostPolicyError(w, http.StatusServiceUnavailable, "host_policy_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Accepted bool                     `json:"accepted"`
		Document identity.ConsentDocument `json:"document"`
	}{Accepted: accepted, Document: p.document})
}

func (p hostPolicy) requireForSpaceWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !needsHostPolicy(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		principal, err := p.principal(r)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		accepted, available, err := p.status(r.Context(), principal.AccountID, false)
		if err != nil || !available {
			writeHostPolicyError(w, http.StatusServiceUnavailable, "host_policy_unavailable")
			return
		}
		if !accepted {
			writeHostPolicyError(w, http.StatusForbidden, "host_policy_acceptance_required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func needsHostPolicy(method, path string) bool {
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return false
	}
	const spacesPath = "/api/v1/spaces"
	if path == spacesPath || path == spacesPath+"/" {
		return method == http.MethodPost
	}
	parts := strings.Split(strings.TrimPrefix(path, spacesPath+"/"), "/")
	if !strings.HasPrefix(path, spacesPath+"/") {
		return false
	}
	if len(parts) == 1 {
		return method == http.MethodPut || method == http.MethodPatch
	}
	switch parts[1] {
	case "publication", "publication-content", "gallery", "cover", "location", "contract-template", "availability", "weekly-hours", "tariff":
		return true
	default:
		return false
	}
}

func writeHostPolicyError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
}
