package adminglobal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResolveAssistedViewKYCStatus(t *testing.T) {
	cases := []struct {
		name              string
		eligible          bool
		attemptStatus     string
		decisionStatus    string
		rejectionReason   string
		verificationState string
		wantStatus        string
		wantReason        string
	}{
		{
			name:              "eligible overrides rejected attempt",
			eligible:          true,
			attemptStatus:     "REJECTED",
			rejectionReason:   "blurry document",
			verificationState: "aprobada",
			wantStatus:        "verified",
			wantReason:        "",
		},
		{
			name:            "rejected attempt preserves reason",
			attemptStatus:   "REJECTED",
			rejectionReason: "RUT mismatch",
			wantStatus:      "rejected",
			wantReason:      "RUT mismatch",
		},
		{
			name:          "in review attempt is pending",
			attemptStatus: "IN_REVIEW",
			wantStatus:    "pending",
		},
		{
			name:              "verification fallback pending",
			verificationState: "en_revision",
			wantStatus:        "pending",
		},
		{
			name:       "no records returns not_started",
			wantStatus: "not_started",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := resolveAssistedViewKYCStatus(tc.eligible, tc.attemptStatus, tc.decisionStatus, tc.rejectionReason, tc.verificationState)
			if status != tc.wantStatus || reason != tc.wantReason {
				t.Fatalf("got (%q, %q), want (%q, %q)", status, reason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}

func TestComputeSpacePublishBlockers(t *testing.T) {
	readySummary := AssistedViewAccountSummary{
		AccountID:     "acc-1",
		State:         "activo",
		EmailVerified: true,
		PhoneVerified: true,
		KYCStatus:     "verified",
	}
	readyMP := &AssistedViewMercadoPago{
		Connected:       true,
		ConnectionState: "active",
	}
	readySpace := AssistedViewSpace{
		ID:                       "sp-1",
		Title:                    "Bodega",
		State:                    "activa",
		PriceCLP:                 15000,
		TimeZone:                 "America/Santiago",
		LocationConfirmed:        true,
		ContractPublishedVersion: 2,
	}

	if got := computeSpacePublishBlockers(readySpace, readySummary, readyMP); len(got) != 0 {
		t.Fatalf("expected no blockers, got %v", got)
	}

	blockedSummary := AssistedViewAccountSummary{
		AccountID:     "acc-1",
		State:         "pendiente",
		EmailVerified: false,
		KYCStatus:     "rejected",
		AdminBlocked:  true,
	}
	reauthMP := &AssistedViewMercadoPago{
		Connected:       false,
		ConnectionState: "reauthorization_required",
	}
	incompleteSpace := AssistedViewSpace{
		ID:    "sp-2",
		Title: "Oficina",
	}

	got := computeSpacePublishBlockers(incompleteSpace, blockedSummary, reauthMP)
	want := []string{
		"account_admin_blocked",
		"email_not_verified",
		"kyc_not_verified",
		"location_not_confirmed",
		"time_zone_missing",
		"price_not_configured",
		"contract_template_not_published",
		"mercadopago_reauthorization_required",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("blocker[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildAssistedViewPublishGuide(t *testing.T) {
	summary := AssistedViewAccountSummary{
		AccountID:     "acc-1",
		EmailVerified: true,
		KYCStatus:     "verified",
	}
	mp := &AssistedViewMercadoPago{
		Connected:       true,
		ConnectionState: "active",
	}
	spaces := []AssistedViewSpace{
		{
			ID:                       "sp-1",
			Title:                    "Sala",
			State:                    "activa",
			PriceCLP:                 10000,
			TimeZone:                 "America/Santiago",
			LocationConfirmed:        true,
			ContractPublishedVersion: 1,
		},
	}

	if guide := buildAssistedViewPublishGuide("arrendatario", summary, nil, nil); guide != nil {
		t.Fatalf("expected nil publish guide for arrendatario, got %+v", guide)
	}

	guide := buildAssistedViewPublishGuide("arrendador", summary, mp, spaces)
	if guide == nil {
		t.Fatal("expected publish guide for arrendador")
	}
	if guide.KYCStatus != "verified" || guide.DraftStatus != "published" || guide.MercadoPagoStatus != "connected" || guide.ContractStatus != "published" {
		t.Fatalf("unexpected guide summary: %+v", guide)
	}
	if len(guide.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(guide.Steps))
	}
	wantKeys := []string{"kyc", "draft", "mercadopago", "contract"}
	for i, key := range wantKeys {
		if guide.Steps[i].Step != i+1 || guide.Steps[i].Key != key || guide.Steps[i].Status != "completed" {
			t.Fatalf("step %d = %+v, want step=%d key=%s status=completed", i, guide.Steps[i], i+1, key)
		}
	}
}

func TestBuildAssistedViewDiagnosticsAndNoSecrets(t *testing.T) {
	now := time.Now().UTC()
	summary := AssistedViewAccountSummary{
		AccountID:          "acc-1",
		State:              "pendiente",
		EmailVerified:      false,
		PhoneVerified:      false,
		KYCStatus:          "rejected",
		KYCRejectionReason: "Documento vencido",
		AdminBlocked:       true,
	}
	mp := &AssistedViewMercadoPago{
		Connected:       false,
		ConnectionState: "not_connected",
		ConnectedAt:     &now,
		UpdatedAt:       &now,
	}
	spaces := []AssistedViewSpace{
		{
			ID:              "sp-1",
			Title:           "Estudio",
			PublishBlockers: []string{"kyc_not_verified", "mercadopago_not_connected"},
		},
	}
	contracts := []AssistedViewContract{
		{ID: "c-1", State: "pending_landlord", DocumentStatus: "failed"},
	}
	payments := []AssistedViewPayment{
		{ID: "p-1", Status: "reconciliation_required"},
		{ID: "p-2", Status: "rejected"},
	}

	diags := buildAssistedViewDiagnostics("arrendador", summary, mp, spaces, contracts, payments)
	codes := make(map[string]bool, len(diags))
	for _, d := range diags {
		codes[d.Code] = true
	}
	for _, expected := range []string{
		"account_admin_blocked",
		"email_not_verified",
		"phone_not_verified",
		"kyc_rejected",
		"mercadopago_not_connected",
		"space_publish_blocked",
		"contracts_pending_landlord",
		"contract_document_failed",
		"payments_reconciliation_required",
		"payments_rejected",
	} {
		if !codes[expected] {
			t.Fatalf("expected diagnostic %q in %+v", expected, diags)
		}
	}

	proj := AssistedViewProjection{
		GrantID:               "g-1",
		AccountID:             "acc-1",
		ActorAccountID:        "adm-1",
		Role:                  "arrendador",
		CaseID:                "SUP-1",
		ExpiresAt:             now.Add(15 * time.Minute),
		ReadOnly:              true,
		AccountSummary:        summary,
		Diagnostics:           diags,
		PublishGuide:          buildAssistedViewPublishGuide("arrendador", summary, mp, spaces),
		MercadoPagoConnection: mp,
		Spaces:                spaces,
		Reservations:          []AssistedViewReservation{},
		Contracts:             contracts,
		Payments:              payments,
	}
	raw, err := json.Marshal(proj)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(raw)
	for _, secret := range []string{"encrypted_access_token", "encrypted_refresh_token", "seller_id"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("projection JSON must never contain %q: %s", secret, payload)
		}
	}
}
