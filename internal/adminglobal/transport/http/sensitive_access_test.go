package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type fakeSensitiveAccessStore struct {
	recorded []adminglobal.SensitiveAccessRecord
	grant    adminglobal.SensitiveAccessGrant
	err      error
}

func (f *fakeSensitiveAccessStore) GrantSensitiveAccess(_ context.Context, rec adminglobal.SensitiveAccessRecord) (adminglobal.SensitiveAccessGrant, error) {
	if f.err != nil {
		return adminglobal.SensitiveAccessGrant{}, f.err
	}
	f.recorded = append(f.recorded, rec)
	return f.grant, nil
}

func TestSensitiveAccessValidationCapabilitiesRecentAuthAndSuccess(t *testing.T) {
	now := time.Now().UTC()
	actorID := "00000000-0000-4000-8000-000000000010"
	targetID := "00000000-0000-4000-8000-000000000001"
	sessionID := "00000000-0000-4000-8000-000000000099"
	url := "/api/v1/admin/global/accounts/" + targetID + "/sensitive-access"

	for _, tc := range []struct {
		name   string
		caps   []identity.Capability
		at     time.Time
		body   string
		status int
		code   string
	}{
		{
			name:   "missing email reveal capability",
			caps:   []identity.Capability{identity.CapabilityAccountsRead},
			at:     now,
			body:   `{"field":"email","reason":"Verificacion de soporte","scope":"account_email"}`,
			status: 403,
			code:   "forbidden",
		},
		{
			name:   "missing accounts read capability",
			caps:   []identity.Capability{identity.CapabilityAccountsEmailReveal},
			at:     now,
			body:   `{"field":"email","reason":"Verificacion de soporte","scope":"account_email"}`,
			status: 403,
			code:   "forbidden",
		},
		{
			name:   "stale mfa",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     now.Add(-11 * time.Minute),
			body:   `{"field":"email","reason":"Verificacion de soporte","scope":"account_email"}`,
			status: 428,
			code:   "recent_authentication_required",
		},
		{
			name:   "zero mfa timestamp",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     time.Time{},
			body:   `{"field":"email","reason":"Verificacion de soporte","scope":"account_email"}`,
			status: 428,
			code:   "recent_authentication_required",
		},
		{
			name:   "missing reason",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     now,
			body:   `{"field":"email","reason":"   ","scope":"account_email"}`,
			status: 400,
			code:   "invalid_request",
		},
		{
			name:   "short reason",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     now,
			body:   `{"field":"email","reason":"abc","scope":"account_email"}`,
			status: 400,
			code:   "invalid_request",
		},
		{
			name:   "invalid field",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     now,
			body:   `{"field":"phone","reason":"Verificacion de soporte","scope":"account_email"}`,
			status: 400,
			code:   "invalid_request",
		},
		{
			name:   "invalid scope",
			caps:   []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			at:     now,
			body:   `{"field":"email","reason":"Verificacion de soporte","scope":"unknown_scope"}`,
			status: 400,
			code:   "invalid_request",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := accountEmailAuth{identity.Principal{
				AccountID:       actorID,
				SessionID:       sessionID,
				Capabilities:    tc.caps,
				AuthenticatedAt: tc.at,
			}}
			svc := &adminglobal.Service{}
			store := &fakeSensitiveAccessStore{grant: adminglobal.SensitiveAccessGrant{
				Value:       "secret@espacigo.cl",
				MaskedValue: "s***@espacigo.cl",
			}}
			svc.SetSensitiveAccessStore(store)
			h := NewHandler(auth, svc, nil)

			req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(tc.body))
			reply := httptest.NewRecorder()
			h.ServeHTTP(reply, req)
			if reply.Code != tc.status || !strings.Contains(reply.Body.String(), tc.code) {
				t.Fatalf("got %d %s, expected %d %s", reply.Code, reply.Body.String(), tc.status, tc.code)
			}
			if strings.Contains(reply.Body.String(), "secret@espacigo.cl") {
				t.Fatal("denied sensitive-access must not expose email")
			}
			if len(store.recorded) != 0 {
				t.Fatal("denied sensitive-access must not record grant")
			}
		})
	}

	t.Run("success", func(t *testing.T) {
		auth := accountEmailAuth{identity.Principal{
			AccountID:       actorID,
			SessionID:       sessionID,
			Capabilities:    []identity.Capability{identity.CapabilityAccountsRead, identity.CapabilityAccountsEmailReveal},
			AuthenticatedAt: now,
		}}
		svc := &adminglobal.Service{}
		store := &fakeSensitiveAccessStore{grant: adminglobal.SensitiveAccessGrant{
			Value:       "persona@espacigo.cl",
			MaskedValue: "p***@espacigo.cl",
		}}
		svc.SetSensitiveAccessStore(store)
		h := NewHandler(auth, svc, nil)

		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(`{"field":"email","reason":"Caso de soporte #421","case_id":"SUP-421","scope":"account_summary"}`))
		reply := httptest.NewRecorder()
		h.ServeHTTP(reply, req)
		if reply.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d %s", reply.Code, reply.Body.String())
		}
		if reply.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("expected Cache-Control private, no-store, got %q", reply.Header().Get("Cache-Control"))
		}
		var body struct {
			Data adminglobal.SensitiveAccessResult `json:"data"`
		}
		if err := json.Unmarshal(reply.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.AccountID != targetID || body.Data.Field != "email" || body.Data.Value != "persona@espacigo.cl" ||
			body.Data.MaskedValue != "p***@espacigo.cl" || body.Data.Scope != "account_summary" ||
			body.Data.Reason != "Caso de soporte #421" || body.Data.CaseID != "SUP-421" ||
			body.Data.AuditEventID == "" || body.Data.ExpiresAt.IsZero() {
			t.Fatalf("unexpected sensitive-access response: %+v", body.Data)
		}
		if len(store.recorded) != 1 || store.recorded[0].AdminSessionID != sessionID {
			t.Fatalf("expected recorded grant with sessionID %s, got %+v", sessionID, store.recorded)
		}
	})
}
