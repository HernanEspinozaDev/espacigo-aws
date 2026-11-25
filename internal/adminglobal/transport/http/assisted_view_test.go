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

type fakeAssistedViewStore struct {
	grants map[string]*fakeAssistedViewEntry
	now    func() time.Time
}

type fakeAssistedViewEntry struct {
	record  adminglobal.AssistedViewRecord
	revoked bool
}

func newFakeAssistedViewStore(now func() time.Time) *fakeAssistedViewStore {
	return &fakeAssistedViewStore{
		grants: make(map[string]*fakeAssistedViewEntry),
		now:    now,
	}
}

func (f *fakeAssistedViewStore) StartAssistedView(_ context.Context, rec adminglobal.AssistedViewRecord) (adminglobal.AssistedViewGrant, error) {
	f.grants[rec.ID] = &fakeAssistedViewEntry{record: rec}
	return adminglobal.AssistedViewGrant{
		GrantID:        rec.ID,
		AccountID:      rec.TargetID,
		ActorAccountID: rec.ActorID,
		Role:           rec.Role,
		CaseID:         rec.CaseID,
		Reason:         rec.Reason,
		ExpiresAt:      rec.ExpiresAt,
	}, nil
}

func (f *fakeAssistedViewStore) GetAssistedViewProjection(_ context.Context, in adminglobal.AssistedViewProjectionInput) (adminglobal.AssistedViewProjection, error) {
	entry, ok := f.grants[in.GrantID]
	if !ok {
		return adminglobal.AssistedViewProjection{}, adminglobal.ErrNotFound
	}
	if entry.record.ActorID != in.ActorID || entry.record.AdminSessionID != in.AdminSessionID {
		return adminglobal.AssistedViewProjection{}, adminglobal.ErrForbidden
	}
	if entry.revoked || !f.now().Before(entry.record.ExpiresAt) {
		return adminglobal.AssistedViewProjection{}, adminglobal.ErrForbidden
	}
	now := f.now()
	summary := adminglobal.AssistedViewAccountSummary{
		AccountID:     entry.record.TargetID,
		State:         "activo",
		EmailVerified: true,
		PhoneVerified: true,
		KYCStatus:     "verified",
		Roles:         []string{entry.record.Role},
	}
	var mp *adminglobal.AssistedViewMercadoPago
	var publishGuide *adminglobal.AssistedViewPublishGuide
	spaces := make([]adminglobal.AssistedViewSpace, 0)
	if entry.record.Role == "arrendador" {
		mp = &adminglobal.AssistedViewMercadoPago{
			Connected:       true,
			ConnectionState: "active",
			LiveMode:        false,
			ConnectedAt:     &now,
			UpdatedAt:       &now,
		}
		spaces = append(spaces, adminglobal.AssistedViewSpace{
			ID:                       "00000000-0000-4000-8000-000000000201",
			Title:                    "Bodega Central",
			State:                    "publicado",
			CategoryCode:             "bodega",
			Modality:                 "hora",
			PriceCLP:                 15000,
			MaxCapacity:              10,
			AreaM2:                   45,
			TimeZone:                 "America/Santiago",
			LocationConfirmed:        true,
			ContractPublishedVersion: 1,
			PublishBlockers:          []string{},
			UpdatedAt:                now,
		})
		publishGuide = &adminglobal.AssistedViewPublishGuide{
			KYCStatus:         "verified",
			DraftStatus:       "published",
			MercadoPagoStatus: "connected",
			ContractStatus:    "published",
			Steps: []adminglobal.AssistedViewPublishStep{
				{Step: 1, Key: "kyc", Title: "KYC", Status: "completed", Detail: "Identidad verificada."},
				{Step: 2, Key: "draft", Title: "Borrador del espacio", Status: "completed", Detail: "Espacio publicado y visible."},
				{Step: 3, Key: "mercadopago", Title: "Sincronizar Mercado Pago", Status: "completed", Detail: "Cuenta de Mercado Pago conectada."},
				{Step: 4, Key: "contract", Title: "Contrato listo", Status: "completed", Detail: "Plantilla de contrato publicada."},
			},
		}
	}
	payExpires := now.Add(30 * time.Minute)
	return adminglobal.AssistedViewProjection{
		GrantID:               entry.record.ID,
		AccountID:             entry.record.TargetID,
		ActorAccountID:        entry.record.ActorID,
		Role:                  entry.record.Role,
		CaseID:                entry.record.CaseID,
		ExpiresAt:             entry.record.ExpiresAt,
		ReadOnly:              true,
		AccountSummary:        summary,
		Diagnostics:           []adminglobal.AssistedViewDiagnostic{},
		PublishGuide:          publishGuide,
		MercadoPagoConnection: mp,
		Spaces:                spaces,
		Reservations: []adminglobal.AssistedViewReservation{
			{
				ID:               "00000000-0000-4000-8000-000000000301",
				SpaceID:          "00000000-0000-4000-8000-000000000201",
				SpaceTitle:       "Bodega Central",
				State:            "confirmada",
				Modality:         "hora",
				SubtotalCLP:      45000,
				Currency:         "CLP",
				StartAt:          now.Add(24 * time.Hour),
				EndAt:            now.Add(48 * time.Hour),
				PaymentExpiresAt: &payExpires,
				CreatedAt:        now,
			},
		},
		Contracts: []adminglobal.AssistedViewContract{
			{
				ID:             "00000000-0000-4000-8000-000000000401",
				ProposalID:     "00000000-0000-4000-8000-000000000402",
				SpaceID:        "00000000-0000-4000-8000-000000000201",
				SpaceTitle:     "Bodega Central",
				State:          "ready_for_payment",
				SigningMode:    "digital",
				DocumentStatus: "ready",
				SubtotalCLP:    45000,
				Currency:       "CLP",
				CreatedAt:      now,
			},
		},
		Payments: []adminglobal.AssistedViewPayment{
			{
				ID:                    "00000000-0000-4000-8000-000000000501",
				ReservationID:         "00000000-0000-4000-8000-000000000301",
				SpaceID:               "00000000-0000-4000-8000-000000000201",
				SpaceTitle:            "Bodega Central",
				Status:                "approved",
				CheckoutMethod:        "checkout_pro",
				TestMode:              true,
				TotalChargedCLP:       46350,
				ProviderPaymentStatus: "approved",
				CreatedAt:             now,
			},
		},
	}, nil
}

func (f *fakeAssistedViewStore) CloseAssistedView(_ context.Context, in adminglobal.AssistedViewCloseInput) error {
	entry, ok := f.grants[in.GrantID]
	if !ok {
		return adminglobal.ErrNotFound
	}
	if entry.record.ActorID != in.ActorID || entry.record.AdminSessionID != in.AdminSessionID {
		return adminglobal.ErrForbidden
	}
	entry.revoked = true
	return nil
}

func TestAssistedViewStartNegativeMatrix(t *testing.T) {
	now := time.Now().UTC()
	actorID := "00000000-0000-4000-8000-000000000010"
	targetID := "00000000-0000-4000-8000-000000000001"
	sessionID := "00000000-0000-4000-8000-000000000099"

	for _, tc := range []struct {
		name     string
		target   string
		caps     []identity.Capability
		mfaAt    time.Time
		body     string
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing accounts.read capability",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsList},
			mfaAt:    now,
			body:     `{"role":"arrendador","case_id":"SUP-101","reason":"Revision de contrato"}`,
			wantCode: http.StatusForbidden,
			wantErr:  "forbidden",
		},
		{
			name:     "stale mfa",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    now.Add(-11 * time.Minute),
			body:     `{"role":"arrendador","case_id":"SUP-101","reason":"Revision de contrato"}`,
			wantCode: http.StatusPreconditionRequired,
			wantErr:  "recent_authentication_required",
		},
		{
			name:     "zero mfa",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    time.Time{},
			body:     `{"role":"arrendador","case_id":"SUP-101","reason":"Revision de contrato"}`,
			wantCode: http.StatusPreconditionRequired,
			wantErr:  "recent_authentication_required",
		},
		{
			name:     "self impersonation rejected",
			target:   actorID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    now,
			body:     `{"role":"arrendador","case_id":"SUP-101","reason":"Revision de contrato"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid_request",
		},
		{
			name:     "missing case_id rejected",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    now,
			body:     `{"role":"arrendador","case_id":"  ","reason":"Revision de contrato"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid_request",
		},
		{
			name:     "short reason rejected",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    now,
			body:     `{"role":"arrendatario","case_id":"SUP-101","reason":"abc"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid_request",
		},
		{
			name:     "invalid role rejected",
			target:   targetID,
			caps:     []identity.Capability{identity.CapabilityAccountsRead},
			mfaAt:    now,
			body:     `{"role":"admin","case_id":"SUP-101","reason":"Revision de contrato"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid_request",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := accountEmailAuth{identity.Principal{
				AccountID:       actorID,
				SessionID:       sessionID,
				Capabilities:    tc.caps,
				AuthenticatedAt: tc.mfaAt,
			}}
			store := newFakeAssistedViewStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetAssistedViewStore(store)
			h := NewHandler(auth, svc, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/accounts/"+tc.target+"/assisted-view", bytes.NewBufferString(tc.body))
			reply := httptest.NewRecorder()
			h.ServeHTTP(reply, req)
			if reply.Code != tc.wantCode || !strings.Contains(reply.Body.String(), tc.wantErr) {
				t.Fatalf("got %d %s, want %d %s", reply.Code, reply.Body.String(), tc.wantCode, tc.wantErr)
			}
			if len(store.grants) != 0 {
				t.Fatal("denied assisted-view start must not create grant")
			}
		})
	}
}

func TestAssistedViewLifecycleAndNegativeProjectionMatrix(t *testing.T) {
	current := time.Now().UTC()
	actorID := "00000000-0000-4000-8000-000000000010"
	otherActorID := "00000000-0000-4000-8000-000000000011"
	targetID := "00000000-0000-4000-8000-000000000001"
	sessionID := "00000000-0000-4000-8000-000000000099"

	store := newFakeAssistedViewStore(func() time.Time { return current })
	svc := &adminglobal.Service{}
	svc.SetAssistedViewStore(store)

	actorAuth := accountEmailAuth{identity.Principal{
		AccountID:       actorID,
		SessionID:       sessionID,
		Capabilities:    []identity.Capability{identity.CapabilityAccountsRead},
		AuthenticatedAt: current,
	}}
	h := NewHandler(actorAuth, svc, nil)

	// 1. Start grant (happy path)
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/accounts/"+targetID+"/assisted-view", bytes.NewBufferString(`{"role":"arrendador","case_id":"SUP-777","reason":"Asistencia por reserva pendiente"}`))
	startReply := httptest.NewRecorder()
	h.ServeHTTP(startReply, startReq)
	if startReply.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", startReply.Code, startReply.Body.String())
	}
	var startBody struct {
		Data adminglobal.AssistedViewGrant `json:"data"`
	}
	if err := json.Unmarshal(startReply.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	grantID := startBody.Data.GrantID
	if grantID == "" || startBody.Data.AccountID != targetID || startBody.Data.ActorAccountID != actorID || startBody.Data.Role != "arrendador" {
		t.Fatalf("unexpected start payload: %+v", startBody.Data)
	}

	// 2. Non-GET methods on /projection must be rejected with 405
	projectionURL := "/api/v1/admin/global/assisted-view/" + grantID + "/projection"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, projectionURL, nil)
		reply := httptest.NewRecorder()
		h.ServeHTTP(reply, req)
		if reply.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /projection got %d, want 405", method, reply.Code)
		}
	}

	// 3. Projection happy path
	projReq := httptest.NewRequest(http.MethodGet, projectionURL, nil)
	projReply := httptest.NewRecorder()
	h.ServeHTTP(projReply, projReq)
	if projReply.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", projReply.Code, projReply.Body.String())
	}
	if projReply.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("expected Cache-Control private, no-store, got %q", projReply.Header().Get("Cache-Control"))
	}
	var projBody struct {
		Data adminglobal.AssistedViewProjection `json:"data"`
	}
	if err := json.Unmarshal(projReply.Body.Bytes(), &projBody); err != nil {
		t.Fatal(err)
	}
	if !projBody.Data.ReadOnly || projBody.Data.GrantID != grantID || len(projBody.Data.Spaces) != 1 || len(projBody.Data.Reservations) != 1 || len(projBody.Data.Contracts) != 1 || len(projBody.Data.Payments) != 1 {
		t.Fatalf("unexpected projection payload: %+v", projBody.Data)
	}
	if projBody.Data.AccountSummary.KYCStatus != "verified" || !projBody.Data.AccountSummary.EmailVerified || !projBody.Data.AccountSummary.PhoneVerified {
		t.Fatalf("unexpected account summary: %+v", projBody.Data.AccountSummary)
	}
	if projBody.Data.PublishGuide == nil || len(projBody.Data.PublishGuide.Steps) != 4 {
		t.Fatalf("expected 4-step publish guide, got %+v", projBody.Data.PublishGuide)
	}
	if projBody.Data.MercadoPagoConnection == nil || !projBody.Data.MercadoPagoConnection.Connected || projBody.Data.MercadoPagoConnection.ConnectionState != "active" {
		t.Fatalf("unexpected mercadopago connection: %+v", projBody.Data.MercadoPagoConnection)
	}
	if projBody.Data.Spaces[0].Modality != "hora" || !projBody.Data.Spaces[0].LocationConfirmed || projBody.Data.Spaces[0].ContractPublishedVersion != 1 {
		t.Fatalf("unexpected space fields: %+v", projBody.Data.Spaces[0])
	}
	if projBody.Data.Contracts[0].ProposalID == "" || projBody.Data.Contracts[0].SigningMode != "digital" {
		t.Fatalf("unexpected contract fields: %+v", projBody.Data.Contracts[0])
	}
	if projBody.Data.Reservations[0].Modality != "hora" || projBody.Data.Reservations[0].PaymentExpiresAt == nil {
		t.Fatalf("unexpected reservation fields: %+v", projBody.Data.Reservations[0])
	}
	for _, secret := range []string{"encrypted_access_token", "encrypted_refresh_token", "seller_id"} {
		if strings.Contains(projReply.Body.String(), secret) {
			t.Fatalf("HTTP projection must never expose %q", secret)
		}
	}

	// 4. Wrong actor rejected (403)
	otherHandler := NewHandler(accountEmailAuth{identity.Principal{
		AccountID:       otherActorID,
		SessionID:       sessionID,
		Capabilities:    []identity.Capability{identity.CapabilityAccountsRead},
		AuthenticatedAt: current,
	}}, svc, nil)
	wrongActorReply := httptest.NewRecorder()
	otherHandler.ServeHTTP(wrongActorReply, httptest.NewRequest(http.MethodGet, projectionURL, nil))
	if wrongActorReply.Code != http.StatusForbidden {
		t.Fatalf("wrong actor got %d, want 403", wrongActorReply.Code)
	}

	// 5. Unknown grant rejected (404)
	unknownReply := httptest.NewRecorder()
	h.ServeHTTP(unknownReply, httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/assisted-view/00000000-0000-4000-8000-000000000999/projection", nil))
	if unknownReply.Code != http.StatusNotFound {
		t.Fatalf("unknown grant got %d, want 404", unknownReply.Code)
	}

	// 6. Expired grant rejected (403)
	current = current.Add(16 * time.Minute)
	expiredReply := httptest.NewRecorder()
	h.ServeHTTP(expiredReply, httptest.NewRequest(http.MethodGet, projectionURL, nil))
	if expiredReply.Code != http.StatusForbidden {
		t.Fatalf("expired grant got %d, want 403", expiredReply.Code)
	}

	// 7. Reset clock, close grant (204), and verify revoked grant rejected (403)
	current = current.Add(-15 * time.Minute)
	closeReply := httptest.NewRecorder()
	h.ServeHTTP(closeReply, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/global/assisted-view/"+grantID, nil))
	if closeReply.Code != http.StatusNoContent {
		t.Fatalf("close grant got %d, want 204", closeReply.Code)
	}
	revokedReply := httptest.NewRecorder()
	h.ServeHTTP(revokedReply, httptest.NewRequest(http.MethodGet, projectionURL, nil))
	if revokedReply.Code != http.StatusForbidden {
		t.Fatalf("revoked grant got %d, want 403", revokedReply.Code)
	}
}
