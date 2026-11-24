package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type accountEmailAuth struct{ principal identity.Principal }

func (a accountEmailAuth) AuthorizeCapability(_ context.Context, _ identity.Secret, cap identity.Capability, _ identity.ActivityKind) (identity.Principal, error) {
	for _, granted := range a.principal.Capabilities {
		if cap == granted {
			return a.principal, nil
		}
	}
	return identity.Principal{}, identity.ErrForbidden
}

func TestAccountEmailRevealRequiresReadPermissionSpecificPermissionAndRecentAuth(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		caps   []identity.Capability
		at     time.Time
		status int
		code   string
	}{
		{"reveal without account read", []identity.Capability{identity.CapabilityAccountsEmailReveal}, now, 403, "forbidden"},
		{"read without reveal", []identity.Capability{identity.CapabilityAccountsRead}, now, 403, "forbidden"},
		{"stale authentication", []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal}, now.Add(-11 * time.Minute), 428, "recent_authentication_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := accountEmailAuth{identity.Principal{Capabilities: tc.caps, AuthenticatedAt: tc.at}}
			h := NewHandler(auth, &adminglobal.Service{}, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/accounts/00000000-0000-4000-8000-000000000001?reveal_email=true", nil)
			reply := httptest.NewRecorder()
			h.ServeHTTP(reply, req)
			if reply.Code != tc.status || !strings.Contains(reply.Body.String(), tc.code) {
				t.Fatalf("got %d %s, expected %d %s", reply.Code, reply.Body, tc.status, tc.code)
			}
			if reply.Header().Get("Cache-Control") != "no-store" || strings.Contains(reply.Body.String(), "@") {
				t.Fatal("denied lookup must not expose an email or allow caching")
			}
		})
	}
}
