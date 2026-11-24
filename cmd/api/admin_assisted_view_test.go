package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devauth"
	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/zohoauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	identityhttp "github.com/HernanEspinozaDev/espaciGo/internal/identity/transport/http"
	spaceshttp "github.com/HernanEspinozaDev/espaciGo/internal/spaces/transport/http"
)

type commercialOnlyAuthRepo struct {
	identity.AuthenticationRepository
}

func (r commercialOnlyAuthRepo) WithLockedAccount(_ context.Context, _ identity.AccountLookup, _ func(identity.Account, identity.AuthenticationTransaction) error) error {
	// Neither admin session tokens nor assisted-view grant IDs exist in commercial sesion storage.
	return identity.ErrUnauthorized
}

func TestCommercialAndMutatingRoutesRejectAdminSessionTokensAndAssistedViewGrants(t *testing.T) {
	authService, err := identity.NewAuthenticationService(
		commercialOnlyAuthRepo{},
		password.Hasher{},
		zohoauth.UnavailableMailer{},
		devauth.NewIPLimiter(20),
		credentials.Generator{},
		time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}

	identityHandler := identityhttp.NewHandler(authService, commercialOnlyAuthRepo{}, nil)
	spacesHandler := spaceshttp.NewHandler(authService, nil, nil)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/", identityHandler)
	mux.Handle("/api/v1/account/", identityHandler)
	mux.Handle("/api/v1/spaces", spacesHandler)
	mux.Handle("/api/v1/spaces/", spacesHandler)

	adminSessionToken := "adm_tok_00000000000000000000000000000001"
	assistedViewGrantID := "00000000-0000-4000-8000-000000000555"

	tokens := []struct {
		name  string
		token string
	}{
		{name: "admin_session_token", token: adminSessionToken},
		{name: "assisted_view_grant_id", token: assistedViewGrantID},
	}

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/auth/session"},
		{method: http.MethodGet, path: "/api/v1/account/security/methods"},
		{method: http.MethodPost, path: "/api/v1/account/security/totp/setup", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/account/security/devices/revoke-others", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/spaces"},
		{method: http.MethodPost, path: "/api/v1/spaces", body: `{"title":"No permitido"}`},
		{method: http.MethodPatch, path: "/api/v1/spaces/00000000-0000-4000-8000-000000000201", body: `{"title":"No permitido"}`},
	}

	for _, tok := range tokens {
		for _, rt := range routes {
			t.Run(tok.name+"_"+rt.method+"_"+rt.path, func(t *testing.T) {
				var body *bytes.Buffer
				if rt.body != "" {
					body = bytes.NewBufferString(rt.body)
				} else {
					body = &bytes.Buffer{}
				}
				req := httptest.NewRequest(rt.method, rt.path, body)
				req.Header.Set("Authorization", "Bearer "+tok.token)
				req.Header.Set("X-Assisted-View-Grant", assistedViewGrantID)
				if rt.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				reply := httptest.NewRecorder()
				mux.ServeHTTP(reply, req)
				if reply.Code != http.StatusUnauthorized {
					t.Fatalf("%s %s with %s got status %d (%s), want 401", rt.method, rt.path, tok.name, reply.Code, reply.Body.String())
				}
			})
		}
	}
}
