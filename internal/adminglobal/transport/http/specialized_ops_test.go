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

type fakeSpecializedOpsStore struct {
	now                  func() time.Time
	payments             []adminglobal.PaymentItem
	reservations         []adminglobal.ReservationItem
	contracts            []adminglobal.ContractItem
	kycCases             []adminglobal.KYCCaseItem
	privacyRequests      []adminglobal.PrivacyRequestItem
	tickets              []adminglobal.SupportTicketItem
	approvals            map[string]adminglobal.ApprovalItem
	idemKeys             map[string]string
	lastPayFilter        adminglobal.PaymentFilter
	lastPrivacyFilter    adminglobal.PrivacyRequestFilter
	lastTransitionInput  adminglobal.TransitionPrivacyRequestInput
	lastExecuteInput     adminglobal.ExecutePrivacyRightInput
	retryPrivacyRequests int
	privacyIncidents     []adminglobal.PrivacyIncidentItem
	lastIncidentFilter   adminglobal.PrivacyIncidentFilter
	lastCreateIncident   adminglobal.CreatePrivacyIncidentInput
	lastIncidentTrans    adminglobal.TransitionPrivacyIncidentInput
	legalHolds           []adminglobal.PrivacyLegalHoldItem
	lastLegalHoldFilter  adminglobal.PrivacyLegalHoldFilter
	lastCreateLegalHold  adminglobal.CreatePrivacyLegalHoldInput
	lastReleaseLegalHold adminglobal.ReleasePrivacyLegalHoldInput
}

func newFakeSpecializedOpsStore(now func() time.Time) *fakeSpecializedOpsStore {
	return &fakeSpecializedOpsStore{
		now:       now,
		approvals: make(map[string]adminglobal.ApprovalItem),
		idemKeys:  make(map[string]string),
	}
}

func (f *fakeSpecializedOpsStore) ListPayments(_ context.Context, _, _ string, filter adminglobal.PaymentFilter) (adminglobal.OpsPage[adminglobal.PaymentItem], error) {
	f.lastPayFilter = filter
	return adminglobal.OpsPage[adminglobal.PaymentItem]{
		Items:         f.payments,
		MatchingTotal: int64(len(f.payments)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) ListReservations(_ context.Context, _, _ string, _ adminglobal.ReservationFilter) (adminglobal.OpsPage[adminglobal.ReservationItem], error) {
	return adminglobal.OpsPage[adminglobal.ReservationItem]{
		Items:         f.reservations,
		MatchingTotal: int64(len(f.reservations)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) ListContracts(_ context.Context, _, _ string, _ adminglobal.ContractFilter) (adminglobal.OpsPage[adminglobal.ContractItem], error) {
	return adminglobal.OpsPage[adminglobal.ContractItem]{
		Items:         f.contracts,
		MatchingTotal: int64(len(f.contracts)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) ListKYCCases(_ context.Context, _, _ string, _ adminglobal.KYCCaseFilter) (adminglobal.OpsPage[adminglobal.KYCCaseItem], error) {
	return adminglobal.OpsPage[adminglobal.KYCCaseItem]{
		Items:         f.kycCases,
		MatchingTotal: int64(len(f.kycCases)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) ListPrivacyRequests(_ context.Context, _, _ string, filter adminglobal.PrivacyRequestFilter) (adminglobal.OpsPage[adminglobal.PrivacyRequestItem], error) {
	f.lastPrivacyFilter = filter
	return adminglobal.OpsPage[adminglobal.PrivacyRequestItem]{
		Items:         f.privacyRequests,
		MatchingTotal: int64(len(f.privacyRequests)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) CreatePrivacyRequest(_ context.Context, in adminglobal.CreatePrivacyRequestInput) (adminglobal.PrivacyRequestItem, error) {
	item := adminglobal.PrivacyRequestItem{
		ID:          "00000000-0000-4000-8000-000000000701",
		AccountID:   in.AccountID,
		RightType:   in.RightType,
		Status:      "pendiente",
		Reason:      in.Reason,
		CaseRef:     in.CaseRef,
		RequestedBy: in.ActorID,
		CreatedAt:   f.now(),
		UpdatedAt:   f.now(),
	}
	f.privacyRequests = append(f.privacyRequests, item)
	return item, nil
}

func (f *fakeSpecializedOpsStore) GetPrivacyRequest(_ context.Context, _, _, requestID string) (adminglobal.PrivacyRequestDetail, error) {
	item := adminglobal.PrivacyRequestItem{
		ID:          requestID,
		AccountID:   "00000000-0000-4000-8000-000000000001",
		RightType:   "bloqueo",
		Channel:     "web",
		Status:      "en_revision",
		Version:     1,
		RequestedBy: "00000000-0000-4000-8000-000000000001",
		CreatedAt:   f.now(),
		UpdatedAt:   f.now(),
	}
	detailText := "Solicitud recibida"
	return adminglobal.PrivacyRequestDetail{
		PrivacyRequestItem: item,
		Events: []adminglobal.PrivacyRequestEvent{
			{
				ID:         "00000000-0000-4000-8000-000000000901",
				Sequence:   1,
				EventType:  "recepcion",
				Visibility: "titular",
				Detail:     &detailText,
				CreatedAt:  f.now(),
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) TransitionPrivacyRequest(_ context.Context, in adminglobal.TransitionPrivacyRequestInput) (adminglobal.PrivacyRequestDetail, error) {
	f.lastTransitionInput = in
	item := adminglobal.PrivacyRequestItem{
		ID:          in.RequestID,
		AccountID:   "00000000-0000-4000-8000-000000000001",
		RightType:   "rectificacion",
		Channel:     "web",
		Status:      "en_revision",
		Version:     in.ExpectedVersion + 1,
		RequestedBy: "00000000-0000-4000-8000-000000000001",
		CreatedAt:   f.now(),
		UpdatedAt:   f.now(),
	}
	if in.AccountID != "" {
		item.AccountID = in.AccountID
	}
	noteText := in.Note
	return adminglobal.PrivacyRequestDetail{
		PrivacyRequestItem: item,
		Events: []adminglobal.PrivacyRequestEvent{
			{
				ID:         "00000000-0000-4000-8000-000000000902",
				Sequence:   2,
				EventType:  "nota_interna",
				Visibility: "interna",
				ActorID:    &in.ActorID,
				Detail:     &noteText,
				CreatedAt:  f.now(),
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) RetryPrivacyNotifications(_ context.Context, _, _, requestID string) (adminglobal.PrivacyRequestDetail, error) {
	f.retryPrivacyRequests++
	return adminglobal.PrivacyRequestDetail{
		PrivacyRequestItem: adminglobal.PrivacyRequestItem{
			ID:        requestID,
			Folio:     "PRIV-2026-AABBCCDD",
			RightType: "rectificacion",
			Channel:   "publico",
			Status:    "en_revision",
			Version:   2,
			CreatedAt: f.now(),
			UpdatedAt: f.now(),
		},
		Events: []adminglobal.PrivacyRequestEvent{},
		NotificationSummary: &adminglobal.PrivacyNotificationSummary{
			Pending:   1,
			Delivered: 0,
			Terminal:  0,
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) ExecutePrivacyRight(_ context.Context, in adminglobal.ExecutePrivacyRightInput) (adminglobal.PrivacyRequestDetail, error) {
	f.lastExecuteInput = in
	now := f.now()
	return adminglobal.PrivacyRequestDetail{
		PrivacyRequestItem: adminglobal.PrivacyRequestItem{
			ID:        in.RequestID,
			AccountID: "00000000-0000-4000-8000-000000000001",
			RightType: "bloqueo",
			Channel:   "web",
			Status:    "en_revision",
			Version:   in.ExpectedVersion + 1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Events: []adminglobal.PrivacyRequestEvent{
			{
				ID:         "00000000-0000-4000-8000-000000000903",
				Sequence:   2,
				EventType:  "ejecucion_derecho",
				Visibility: "titular",
				ActorID:    &in.ActorID,
				Detail:     &in.Summary,
				CreatedAt:  now,
			},
		},
		Executions: []adminglobal.PrivacyRightExecution{
			{
				ID:            "00000000-0000-4000-8000-000000000904",
				RequestID:     in.RequestID,
				AccountID:     "00000000-0000-4000-8000-000000000001",
				ExecutionType: "bloqueo_aplicado",
				Status:        "ejecutada",
				Scope:         in.Scope,
				Summary:       in.Summary,
				ExecutedBy:    in.ActorID,
				ExecutedAt:    now,
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) ResolvePrivacyRequest(_ context.Context, in adminglobal.ResolvePrivacyRequestInput) (adminglobal.PrivacyRequestItem, error) {
	return adminglobal.PrivacyRequestItem{ID: in.RequestID, Status: "completada", Reason: "privacy response", CanRecordDispatch: true, CreatedAt: f.now(), UpdatedAt: f.now()}, nil
}

func (f *fakeSpecializedOpsStore) RecordPrivacyDispatch(_ context.Context, in adminglobal.RecordPrivacyDispatchInput) (adminglobal.PrivacyRequestItem, error) {
	recorded := f.now()
	return adminglobal.PrivacyRequestItem{ID: in.RequestID, Status: "completada", Reason: "privacy response", CanRecordDispatch: false, DispatchRecordedAt: &recorded, CreatedAt: f.now(), UpdatedAt: f.now()}, nil
}

func (f *fakeSpecializedOpsStore) ListPrivacyIncidents(_ context.Context, _, _ string, filter adminglobal.PrivacyIncidentFilter) (adminglobal.OpsPage[adminglobal.PrivacyIncidentItem], error) {
	f.lastIncidentFilter = filter
	return adminglobal.OpsPage[adminglobal.PrivacyIncidentItem]{
		Items:         f.privacyIncidents,
		MatchingTotal: int64(len(f.privacyIncidents)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) GetPrivacyIncident(_ context.Context, _, _, incidentID string) (adminglobal.PrivacyIncidentDetail, error) {
	now := f.now()
	return adminglobal.PrivacyIncidentDetail{
		PrivacyIncidentItem: adminglobal.PrivacyIncidentItem{
			ID:              incidentID,
			Folio:           "INC-2026-AABBCCDD",
			Title:           "Incidente de prueba",
			Severity:        "alta",
			Status:          "abierto",
			DataCategories:  []string{"correo"},
			AffectedSystems: []string{"api"},
			Summary:         "Resumen del incidente de prueba",
			LeaderID:        "00000000-0000-4000-8000-000000000010",
			DetectedAt:      now,
			CreatedBy:       "00000000-0000-4000-8000-000000000010",
			CreatedAt:       now,
			UpdatedAt:       now,
			Version:         1,
		},
		Events: []adminglobal.PrivacyIncidentEvent{
			{
				ID:        "00000000-0000-4000-8000-000000000951",
				Sequence:  1,
				EventType: "apertura",
				ActorID:   "00000000-0000-4000-8000-000000000010",
				Detail:    "Apertura de incidente",
				CreatedAt: now,
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) CreatePrivacyIncident(_ context.Context, in adminglobal.CreatePrivacyIncidentInput) (adminglobal.PrivacyIncidentDetail, error) {
	f.lastCreateIncident = in
	now := f.now()
	item := adminglobal.PrivacyIncidentItem{
		ID:                    "00000000-0000-4000-8000-000000000950",
		Folio:                 "INC-2026-11223344",
		Title:                 in.Title,
		Severity:              in.Severity,
		Status:                "abierto",
		DataCategories:        in.DataCategories,
		AffectedSystems:       in.AffectedSystems,
		EstimatedSubjects:     in.EstimatedSubjects,
		Summary:               in.Summary,
		RequiresAgencyNotice:  in.RequiresAgencyNotice,
		RequiresSubjectNotice: in.RequiresSubjectNotice,
		LeaderID:              in.LeaderID,
		DetectedAt:            in.DetectedAt,
		CreatedBy:             in.ActorID,
		CreatedAt:             now,
		UpdatedAt:             now,
		Version:               1,
	}
	f.privacyIncidents = append(f.privacyIncidents, item)
	return adminglobal.PrivacyIncidentDetail{
		PrivacyIncidentItem: item,
		Events: []adminglobal.PrivacyIncidentEvent{
			{
				ID:        "00000000-0000-4000-8000-000000000952",
				Sequence:  1,
				EventType: "apertura",
				ActorID:   in.ActorID,
				Detail:    in.Summary,
				CreatedAt: now,
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) TransitionPrivacyIncident(_ context.Context, in adminglobal.TransitionPrivacyIncidentInput) (adminglobal.PrivacyIncidentDetail, error) {
	f.lastIncidentTrans = in
	now := f.now()
	return adminglobal.PrivacyIncidentDetail{
		PrivacyIncidentItem: adminglobal.PrivacyIncidentItem{
			ID:              in.IncidentID,
			Folio:           "INC-2026-11223344",
			Title:           "Incidente de prueba",
			Severity:        "alta",
			Status:          "contenido",
			DataCategories:  []string{"correo"},
			AffectedSystems: []string{"api"},
			Summary:         "Resumen del incidente",
			LeaderID:        in.ActorID,
			DetectedAt:      now,
			CreatedBy:       in.ActorID,
			CreatedAt:       now,
			UpdatedAt:       now,
			Version:         in.ExpectedVersion + 1,
		},
		Events: []adminglobal.PrivacyIncidentEvent{
			{
				ID:        "00000000-0000-4000-8000-000000000953",
				Sequence:  2,
				EventType: "contencion",
				ActorID:   in.ActorID,
				Detail:    in.Detail,
				CreatedAt: now,
			},
		},
	}, nil
}

func (f *fakeSpecializedOpsStore) ListPrivacyLegalHolds(_ context.Context, _, _ string, filter adminglobal.PrivacyLegalHoldFilter) (adminglobal.OpsPage[adminglobal.PrivacyLegalHoldItem], error) {
	f.lastLegalHoldFilter = filter
	return adminglobal.OpsPage[adminglobal.PrivacyLegalHoldItem]{
		Items:         f.legalHolds,
		MatchingTotal: int64(len(f.legalHolds)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) CreatePrivacyLegalHold(_ context.Context, in adminglobal.CreatePrivacyLegalHoldInput) (adminglobal.PrivacyLegalHoldItem, error) {
	f.lastCreateLegalHold = in
	now := f.now()
	var accPtr *string
	if in.AccountID != "" {
		acc := in.AccountID
		accPtr = &acc
	}
	item := adminglobal.PrivacyLegalHoldItem{
		ID:         "00000000-0000-4000-8000-000000000960",
		Folio:      "HOLD-2026-AABBCCDD",
		AccountID:  accPtr,
		Scope:      in.Scope,
		Authority:  in.Authority,
		LegalBasis: in.LegalBasis,
		Status:     "activa",
		ReviewAt:   in.ReviewAt,
		AppliedBy:  in.ActorID,
		AppliedAt:  now,
	}
	f.legalHolds = append(f.legalHolds, item)
	return item, nil
}

func (f *fakeSpecializedOpsStore) ReleasePrivacyLegalHold(_ context.Context, in adminglobal.ReleasePrivacyLegalHoldInput) (adminglobal.PrivacyLegalHoldItem, error) {
	f.lastReleaseLegalHold = in
	now := f.now()
	relBy := in.ActorID
	relReason := in.ReleaseReason
	return adminglobal.PrivacyLegalHoldItem{
		ID:            in.HoldID,
		Folio:         "HOLD-2026-AABBCCDD",
		Scope:         "expediente_privacidad",
		Authority:     "Agencia de Protección de Datos",
		LegalBasis:    "Orden administrativa vigente",
		Status:        "liberada",
		AppliedBy:     in.ActorID,
		AppliedAt:     now.Add(-time.Hour),
		ReleasedBy:    &relBy,
		ReleasedAt:    &now,
		ReleaseReason: &relReason,
	}, nil
}

func (f *fakeSpecializedOpsStore) ListSupportTickets(_ context.Context, _, _ string, _ adminglobal.SupportTicketFilter) (adminglobal.OpsPage[adminglobal.SupportTicketItem], error) {
	return adminglobal.OpsPage[adminglobal.SupportTicketItem]{
		Items:         f.tickets,
		MatchingTotal: int64(len(f.tickets)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) CreateSupportTicket(_ context.Context, in adminglobal.CreateSupportTicketInput) (adminglobal.SupportTicketItem, error) {
	item := adminglobal.SupportTicketItem{
		ID:           "00000000-0000-4000-8000-000000000801",
		AccountID:    in.AccountID,
		AssignedToID: in.AssignedToID,
		Category:     in.Category,
		Priority:     in.Priority,
		Status:       "abierto",
		Subject:      in.Subject,
		CaseRef:      in.CaseRef,
		CreatedAt:    f.now(),
		UpdatedAt:    f.now(),
	}
	f.tickets = append(f.tickets, item)
	return item, nil
}

func (f *fakeSpecializedOpsStore) ListApprovals(_ context.Context, _, _ string, _ adminglobal.ApprovalFilter) (adminglobal.OpsPage[adminglobal.ApprovalItem], error) {
	items := make([]adminglobal.ApprovalItem, 0, len(f.approvals))
	for _, item := range f.approvals {
		items = append(items, item)
	}
	return adminglobal.OpsPage[adminglobal.ApprovalItem]{
		Items:         items,
		MatchingTotal: int64(len(items)),
		GeneratedAt:   f.now(),
	}, nil
}

func (f *fakeSpecializedOpsStore) CreateApproval(_ context.Context, in adminglobal.CreateApprovalInput) (adminglobal.ApprovalItem, error) {
	if _, exists := f.idemKeys[in.IdempotencyKey]; exists {
		return adminglobal.ApprovalItem{}, adminglobal.ErrConflict
	}
	id := "00000000-0000-4000-8000-000000000901"
	item := adminglobal.ApprovalItem{
		ID:             id,
		ActionType:     in.ActionType,
		ResourceType:   in.ResourceType,
		ResourceID:     in.ResourceID,
		RequesterID:    in.ActorID,
		Status:         "pending",
		Reason:         in.Reason,
		Payload:        in.Payload,
		IdempotencyKey: in.IdempotencyKey,
		Version:        1,
		CreatedAt:      f.now(),
		ExpiresAt:      f.now().Add(24 * time.Hour),
	}
	f.approvals[id] = item
	f.idemKeys[in.IdempotencyKey] = id
	return item, nil
}

func (f *fakeSpecializedOpsStore) DecideApproval(_ context.Context, in adminglobal.DecideApprovalInput) (adminglobal.ApprovalItem, error) {
	item, ok := f.approvals[in.ApprovalID]
	if !ok {
		return adminglobal.ApprovalItem{}, adminglobal.ErrNotFound
	}
	if item.RequesterID == in.ActorID {
		return adminglobal.ApprovalItem{}, adminglobal.ErrSelfApprovalForbidden
	}
	if item.Status != "pending" || item.Version != in.ExpectedVersion || !f.now().Before(item.ExpiresAt) {
		return adminglobal.ApprovalItem{}, adminglobal.ErrConflict
	}
	decidedAt := f.now()
	item.ApproverID = in.ActorID
	item.Status = in.Decision
	item.DecisionReason = in.DecisionReason
	item.DecidedAt = &decidedAt
	item.Version++
	f.approvals[in.ApprovalID] = item
	return item, nil
}

func TestSpecializedOpsReadEndpointsHappyPath(t *testing.T) {
	now := time.Now().UTC()
	actorID := "00000000-0000-4000-8000-000000000010"
	accountID := "00000000-0000-4000-8000-000000000001"
	store := newFakeSpecializedOpsStore(func() time.Time { return now })
	store.payments = []adminglobal.PaymentItem{{
		ID:                    "00000000-0000-4000-8000-000000000101",
		QuoteID:               "00000000-0000-4000-8000-000000000102",
		SpaceID:               "00000000-0000-4000-8000-000000000103",
		SpaceTitle:            "Bodega Norte",
		RenterID:              accountID,
		HostID:                "00000000-0000-4000-8000-000000000002",
		Status:                "approved",
		BaseCLP:               10000,
		CommissionCLP:         300,
		IVACLP:                57,
		BuyerFeeCLP:           357,
		TotalChargedCLP:       10357,
		ProviderPaymentID:     "123456789",
		ProviderPaymentStatus: "approved",
		PaymentVerifiedAt:     &now,
		TestMode:              true,
		CreatedAt:             now,
		UpdatedAt:             now,
	}}
	store.reservations = []adminglobal.ReservationItem{{
		ID:          "00000000-0000-4000-8000-000000000201",
		SpaceID:     "00000000-0000-4000-8000-000000000103",
		SpaceTitle:  "Bodega Norte",
		Status:      "pagada",
		SubtotalCLP: 10000,
		Currency:    "CLP",
		StartAt:     now,
		EndAt:       now.Add(24 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}}
	store.contracts = []adminglobal.ContractItem{{
		ID:             "00000000-0000-4000-8000-000000000301",
		SpaceID:        "00000000-0000-4000-8000-000000000103",
		SpaceTitle:     "Bodega Norte",
		Status:         "ready_for_payment",
		DocumentStatus: "ready",
		SubtotalCLP:    10000,
		Currency:       "CLP",
		CreatedAt:      now,
		UpdatedAt:      now,
	}}
	store.kycCases = []adminglobal.KYCCaseItem{{
		ID:           "00000000-0000-4000-8000-000000000401",
		AccountID:    accountID,
		Status:       "APPROVED",
		Environment:  "SANDBOX",
		IDCountry:    "CHL",
		DocumentType: "ID",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}

	svc := &adminglobal.Service{}
	svc.SetSpecializedOpsStore(store)
	auth := accountEmailAuth{principal: identity.Principal{
		AccountID:       actorID,
		Capabilities:    identity.SpecializedAdminCapabilities(),
		AuthenticatedAt: now,
	}}
	h := NewHandler(auth, svc, nil)

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/api/v1/admin/global/payments?status=approved&test_mode=true&account_id=" + accountID, `"total_charged_clp":10357`},
		{"/api/v1/admin/global/reservations?status=pagada", `"status":"pagada"`},
		{"/api/v1/admin/global/contracts?status=ready_for_payment", `"document_status":"ready"`},
		{"/api/v1/admin/global/kyc-cases?status=APPROVED", `"status":"APPROVED"`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d (%s)", tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: expected %q in %s", tc.path, tc.want, rec.Body.String())
		}
	}
	if store.lastPayFilter.TestMode == nil || !*store.lastPayFilter.TestMode || store.lastPayFilter.Status != "approved" {
		t.Fatalf("expected payment filters to propagate, got %+v", store.lastPayFilter)
	}
	if store.lastPayFilter.PageSize != 25 {
		t.Fatalf("expected default page_size 25, got %d", store.lastPayFilter.PageSize)
	}

	for _, tc := range []struct {
		path     string
		wantCode int
	}{
		{"/api/v1/admin/global/payments?page_size=100", http.StatusOK},
		{"/api/v1/admin/global/payments?page_size=101", http.StatusUnprocessableEntity},
		{"/api/v1/admin/global/payments?page_size=0", http.StatusUnprocessableEntity},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("%s: expected %d, got %d (%s)", tc.path, tc.wantCode, rec.Code, rec.Body.String())
		}
	}
}

func TestPrivacyResolutionRequiresCapabilityAndRecentMFA(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name       string
		capability identity.Capability
		mfaAt      time.Time
		wantStatus int
	}{
		{"missing permission", identity.CapabilityAccountsRead, now, http.StatusForbidden},
		{"missing MFA", identity.CapabilityPrivacyRequestsManage, time.Time{}, http.StatusPreconditionRequired},
		{"stale MFA", identity.CapabilityPrivacyRequestsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := accountEmailAuth{principal: identity.Principal{
				AccountID:       "00000000-0000-4000-8000-000000000010",
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}
			handler := NewHandler(auth, &adminglobal.Service{}, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/privacy-requests/00000000-0000-4000-8000-000000000701/resolution", strings.NewReader(`{"decision":"resuelta","response":"Respuesta de prueba para el titular."}`))
			request.Header.Set("Authorization", "Bearer valid-token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d", recorder.Code, tc.wantStatus)
			}
		})
	}
}

func TestPrivacyDispatchReceiptRequiresCapabilityAndRecentMFA(t *testing.T) {
	now := time.Now().UTC()
	const targetPath = "/api/v1/admin/global/privacy-requests/00000000-0000-4000-8000-000000000701/dispatch-receipt"
	payload := `{"channel":"email","destination":"titular@ejemplo.invalid","sent_at":"` + now.Add(-time.Minute).Format(time.RFC3339) + `","evidence_ref":"MAIL-001"}`
	for _, tc := range []struct {
		name       string
		method     string
		capability identity.Capability
		mfaAt      time.Time
		wantStatus int
	}{
		{"wrong method", http.MethodGet, identity.CapabilityPrivacyRequestsManage, now, http.StatusMethodNotAllowed},
		{"missing permission", http.MethodPost, identity.CapabilityAccountsRead, now, http.StatusForbidden},
		{"missing MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, time.Time{}, http.StatusPreconditionRequired},
		{"stale MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
		{"happy path", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			auth := accountEmailAuth{principal: identity.Principal{
				AccountID:       "00000000-0000-4000-8000-000000000010",
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}
			handler := NewHandler(auth, svc, nil)
			request := httptest.NewRequest(tc.method, targetPath, strings.NewReader(payload))
			request.Header.Set("Authorization", "Bearer valid-token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantStatus == http.StatusOK && !strings.Contains(recorder.Body.String(), `"dispatch_recorded_at"`) {
				t.Fatalf("expected dispatch_recorded_at in response: %s", recorder.Body.String())
			}
		})
	}
}

func TestSpecializedOpsPrivacyAndSupportTickets(t *testing.T) {
	now := time.Now().UTC()
	actorID := "00000000-0000-4000-8000-000000000010"
	accountID := "00000000-0000-4000-8000-000000000001"
	store := newFakeSpecializedOpsStore(func() time.Time { return now })
	svc := &adminglobal.Service{}
	svc.SetSpecializedOpsStore(store)
	auth := accountEmailAuth{principal: identity.Principal{
		AccountID:       actorID,
		Capabilities:    []identity.Capability{identity.CapabilityPrivacyRequestsManage, identity.CapabilitySupportTicketsManage},
		AuthenticatedAt: now,
	}}
	h := NewHandler(auth, svc, nil)

	// Create privacy request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/privacy-requests", bytes.NewBufferString(`{"account_id":"`+accountID+`","right_type":"acceso","reason":"Solicitud formal de acceso","case_ref":"PRIV-101"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create privacy request: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// List privacy requests
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/privacy-requests", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"case_ref":"PRIV-101"`) {
		t.Fatalf("list privacy requests failed: %d (%s)", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/privacy-requests/00000000-0000-4000-8000-000000000701/resolution", bytes.NewBufferString(`{"decision":"resuelta","response":"La información solicitada está disponible."}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"completada"`) {
		t.Fatalf("resolve privacy request failed: %d (%s)", rec.Code, rec.Body.String())
	}

	// Filter privacy requests by right_type
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/privacy-requests?right_type=acceso", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || store.lastPrivacyFilter.RightType != "acceso" {
		t.Fatalf("filter privacy requests failed: status=%d filter=%+v", rec.Code, store.lastPrivacyFilter)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/privacy-requests?right_type=otro", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid right_type status=%d want 422", rec.Code)
	}

	// Create support ticket
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/support-tickets", bytes.NewBufferString(`{"account_id":"`+accountID+`","category":"pagos","priority":"alta","subject":"Pago en conciliacion pendiente","case_ref":"SUP-202"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create support ticket: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// List support tickets
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/support-tickets", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"case_ref":"SUP-202"`) {
		t.Fatalf("list support tickets failed: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestApprovalsDualControlLifecycleAndGuards(t *testing.T) {
	now := time.Now().UTC()
	requesterID := "00000000-0000-4000-8000-000000000010"
	approverID := "00000000-0000-4000-8000-000000000020"
	store := newFakeSpecializedOpsStore(func() time.Time { return now })
	svc := &adminglobal.Service{}
	svc.SetSpecializedOpsStore(store)

	// 1. Stale MFA on POST /approvals -> 428
	staleHandler := NewHandler(accountEmailAuth{principal: identity.Principal{
		AccountID:       requesterID,
		Capabilities:    []identity.Capability{identity.CapabilityApprovalsRequest},
		AuthenticatedAt: now.Add(-15 * time.Minute),
	}}, svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals", bytes.NewBufferString(`{"action_type":"refund_request","resource_type":"payment","resource_id":"pay-1","reason":"Devolucion aprobada por soporte","idempotency_key":"idem-1"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	staleHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("stale MFA create approval: expected 428, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. Create approval happy path -> 201
	reqHandler := NewHandler(accountEmailAuth{principal: identity.Principal{
		AccountID:       requesterID,
		Capabilities:    []identity.Capability{identity.CapabilityApprovalsRequest, identity.CapabilityApprovalsDecide},
		AuthenticatedAt: now,
	}}, svc, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals", bytes.NewBufferString(`{"action_type":"refund_request","resource_type":"payment","resource_id":"pay-1","reason":"Devolucion aprobada por soporte","idempotency_key":"idem-1"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	reqHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create approval: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Data adminglobal.ApprovalItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created approval: %v", err)
	}
	approvalID := created.Data.ID

	// 3. Self-approval rejection -> 403 self_approval_forbidden
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals/"+approvalID+"/decisions", bytes.NewBufferString(`{"decision":"approved","decision_reason":"Intento de auto aprobacion","expected_version":1}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	reqHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"self_approval_forbidden"`) {
		t.Fatalf("self-approval: expected 403 self_approval_forbidden, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 4. Stale MFA on decision -> 428
	staleDeciderHandler := NewHandler(accountEmailAuth{principal: identity.Principal{
		AccountID:       approverID,
		Capabilities:    []identity.Capability{identity.CapabilityApprovalsDecide},
		AuthenticatedAt: now.Add(-15 * time.Minute),
	}}, svc, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals/"+approvalID+"/decisions", bytes.NewBufferString(`{"decision":"approved","decision_reason":"Aprobacion con MFA vencido","expected_version":1}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	staleDeciderHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("stale MFA decision: expected 428, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 5. Version conflict -> 409
	deciderHandler := NewHandler(accountEmailAuth{principal: identity.Principal{
		AccountID:       approverID,
		Capabilities:    []identity.Capability{identity.CapabilityApprovalsDecide},
		AuthenticatedAt: now,
	}}, svc, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals/"+approvalID+"/decisions", bytes.NewBufferString(`{"decision":"approved","decision_reason":"Version incorrecta","expected_version":2}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	deciderHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("version conflict: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 6. Decision happy path -> 200
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/approvals/"+approvalID+"/decisions", bytes.NewBufferString(`{"decision":"approved","decision_reason":"Validado por segundo administrador","expected_version":1}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	deciderHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"approved"`) || !strings.Contains(rec.Body.String(), `"version":2`) {
		t.Fatalf("decision happy path: expected 200 approved v2, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 7. List approvals -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/approvals?status=approved", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec = httptest.NewRecorder()
	deciderHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), approvalID) {
		t.Fatalf("list approvals: expected 200 with approvalID, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPrivacyDetailWorkflowAndQueryFilters(t *testing.T) {
	now := time.Now().UTC()
	const requestID = "00000000-0000-4000-8000-000000000711"
	const actorID = "00000000-0000-4000-8000-000000000010"

	t.Run("detail requires manage capability", func(t *testing.T) {
		store := newFakeSpecializedOpsStore(func() time.Time { return now })
		svc := &adminglobal.Service{}
		svc.SetSpecializedOpsStore(store)
		for _, tc := range []struct {
			name       string
			capability identity.Capability
			wantStatus int
		}{
			{"missing capability", identity.CapabilityAccountsRead, http.StatusForbidden},
			{"privacy manage allowed", identity.CapabilityPrivacyRequestsManage, http.StatusOK},
		} {
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:    actorID,
				Capabilities: []identity.Capability{tc.capability},
			}}, svc, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/privacy-requests/"+requestID, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK && (!strings.Contains(rec.Body.String(), `"events"`) || !strings.Contains(rec.Body.String(), `"bloqueo"`)) {
				t.Fatalf("unexpected detail body: %s", rec.Body.String())
			}
		}
	})

	t.Run("workflow requires manage capability and recent MFA", func(t *testing.T) {
		payload := `{"action":"add_internal_note","expected_version":1,"note":"Verificando respaldo documental del caso"}`
		for _, tc := range []struct {
			name       string
			method     string
			capability identity.Capability
			mfaAt      time.Time
			wantStatus int
		}{
			{"wrong method", http.MethodGet, identity.CapabilityPrivacyRequestsManage, now, http.StatusMethodNotAllowed},
			{"missing permission", http.MethodPost, identity.CapabilityAccountsRead, now, http.StatusForbidden},
			{"missing MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, time.Time{}, http.StatusPreconditionRequired},
			{"stale MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
			{"happy path", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now, http.StatusOK},
		} {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:       actorID,
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}, svc, nil)
			req := httptest.NewRequest(tc.method, "/api/v1/admin/global/privacy-requests/"+requestID+"/workflow", strings.NewReader(payload))
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK && (!strings.Contains(rec.Body.String(), `"version":2`) || !strings.Contains(rec.Body.String(), `"nota_interna"`)) {
				t.Fatalf("unexpected workflow body: %s", rec.Body.String())
			}
		}
	})

	t.Run("list privacy filter parameters validation", func(t *testing.T) {
		store := newFakeSpecializedOpsStore(func() time.Time { return now })
		svc := &adminglobal.Service{}
		svc.SetSpecializedOpsStore(store)
		handler := NewHandler(accountEmailAuth{principal: identity.Principal{
			AccountID:    actorID,
			Capabilities: []identity.Capability{identity.CapabilityPrivacyRequestsManage},
		}}, svc, nil)

		for _, tc := range []struct {
			query      string
			wantStatus int
		}{
			{"?channel=web&overdue=true&assigned_to_id=" + actorID + "&right_type=bloqueo", http.StatusOK},
			{"?channel=publico", http.StatusOK},
			{"?channel=correo", http.StatusOK},
			{"?channel=telegram", http.StatusUnprocessableEntity},
			{"?overdue=maybe", http.StatusUnprocessableEntity},
			{"?right_type=desconocido", http.StatusUnprocessableEntity},
		} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/privacy-requests"+tc.query, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("query %q: status=%d want=%d body=%s", tc.query, rec.Code, tc.wantStatus, rec.Body.String())
			}
		}
	})

	t.Run("workflow forwards account_id for verify_identity", func(t *testing.T) {
		store := newFakeSpecializedOpsStore(func() time.Time { return now })
		svc := &adminglobal.Service{}
		svc.SetSpecializedOpsStore(store)
		handler := NewHandler(accountEmailAuth{principal: identity.Principal{
			AccountID:       actorID,
			Capabilities:    []identity.Capability{identity.CapabilityPrivacyRequestsManage},
			AuthenticatedAt: now,
		}}, svc, nil)
		const targetAccount = "00000000-0000-4000-8000-000000000099"
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/privacy-requests/"+requestID+"/workflow",
			strings.NewReader(`{"action":"verify_identity","expected_version":1,"account_id":"`+targetAccount+`","note":"Vinculado por verificación"}`))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want=200 body=%s", rec.Code, rec.Body.String())
		}
		if store.lastTransitionInput.AccountID != targetAccount {
			t.Fatalf("AccountID=%q want %q", store.lastTransitionInput.AccountID, targetAccount)
		}
	})

	t.Run("notifications retry requires manage capability and recent MFA", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			method     string
			capability identity.Capability
			mfaAt      time.Time
			wantStatus int
		}{
			{"wrong method", http.MethodGet, identity.CapabilityPrivacyRequestsManage, now, http.StatusMethodNotAllowed},
			{"missing permission", http.MethodPost, identity.CapabilityAccountsRead, now, http.StatusForbidden},
			{"missing MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, time.Time{}, http.StatusPreconditionRequired},
			{"stale MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
			{"happy path", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now, http.StatusOK},
		} {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:       actorID,
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}, svc, nil)
			req := httptest.NewRequest(tc.method, "/api/v1/admin/global/privacy-requests/"+requestID+"/notifications/retry", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				if store.retryPrivacyRequests != 1 {
					t.Fatalf("expected RetryPrivacyNotifications to be called once, got %d", store.retryPrivacyRequests)
				}
				if !strings.Contains(rec.Body.String(), `"notification_summary"`) || !strings.Contains(rec.Body.String(), `"pending":1`) {
					t.Fatalf("unexpected retry body: %s", rec.Body.String())
				}
			}
		}
	})

	t.Run("executions requires manage capability and recent MFA", func(t *testing.T) {
		payload := `{"action":"apply_block","expected_version":1,"scope":"analitica_y_perfilado","summary":"Bloqueo temporal aplicado por revisión de exactitud"}`
		for _, tc := range []struct {
			name       string
			method     string
			capability identity.Capability
			mfaAt      time.Time
			wantStatus int
		}{
			{"wrong method", http.MethodGet, identity.CapabilityPrivacyRequestsManage, now, http.StatusMethodNotAllowed},
			{"missing permission", http.MethodPost, identity.CapabilityAccountsRead, now, http.StatusForbidden},
			{"missing MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, time.Time{}, http.StatusPreconditionRequired},
			{"stale MFA", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
			{"happy path", http.MethodPost, identity.CapabilityPrivacyRequestsManage, now, http.StatusOK},
		} {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:       actorID,
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}, svc, nil)
			req := httptest.NewRequest(tc.method, "/api/v1/admin/global/privacy-requests/"+requestID+"/executions", strings.NewReader(payload))
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				if store.lastExecuteInput.Action != "apply_block" || store.lastExecuteInput.Scope != "analitica_y_perfilado" || store.lastExecuteInput.ExpectedVersion != 1 {
					t.Fatalf("unexpected ExecutePrivacyRight input: %+v", store.lastExecuteInput)
				}
				if !strings.Contains(rec.Body.String(), `"executions"`) || !strings.Contains(rec.Body.String(), `"bloqueo_aplicado"`) || !strings.Contains(rec.Body.String(), `"version":2`) {
					t.Fatalf("unexpected execution body: %s", rec.Body.String())
				}
			}
		}
	})
}

func TestPrivacyIncidentsAndLegalHoldsPermissionsAndValidation(t *testing.T) {
	now := time.Now().UTC()
	const actorID = "00000000-0000-4000-8000-000000000010"
	const accountID = "00000000-0000-4000-8000-000000000001"
	const requestID = "00000000-0000-4000-8000-000000000711"
	const incidentID = "00000000-0000-4000-8000-000000000950"
	const holdID = "00000000-0000-4000-8000-000000000960"

	t.Run("privacy.requests.read allows GET list/detail and forbids POST", func(t *testing.T) {
		store := newFakeSpecializedOpsStore(func() time.Time { return now })
		svc := &adminglobal.Service{}
		svc.SetSpecializedOpsStore(store)
		handler := NewHandler(accountEmailAuth{principal: identity.Principal{
			AccountID:       actorID,
			Capabilities:    []identity.Capability{identity.CapabilityPrivacyRequestsRead},
			AuthenticatedAt: now,
		}}, svc, nil)

		for _, path := range []string{
			"/api/v1/admin/global/privacy-requests",
			"/api/v1/admin/global/privacy-requests/" + requestID,
		} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s with privacy.requests.read: status=%d want=200 body=%s", path, rec.Code, rec.Body.String())
			}
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/global/privacy-requests",
			strings.NewReader(`{"account_id":"`+accountID+`","right_type":"acceso","reason":"Solicitud de prueba"}`))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("POST /privacy-requests with privacy.requests.read: status=%d want=403 body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("privacy-incidents list/detail/create/transition permissions and MFA", func(t *testing.T) {
		createPayload := `{"title":"Exposición temporal de respaldos","severity":"alta","data_categories":["correo"],"affected_systems":["s3_backup"],"estimated_subjects":12,"summary":"Acceso anómalo contenido de inmediato.","requires_agency_notice":true,"requires_subject_notice":false,"leader_id":"` + actorID + `","detected_at":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}`
		transPayload := `{"action":"contain","expected_version":1,"detail":"Credenciales rotadas y bucket cerrado en producción."}`

		for _, tc := range []struct {
			name       string
			method     string
			path       string
			body       string
			capability identity.Capability
			mfaAt      time.Time
			wantStatus int
		}{
			{"list forbidden", http.MethodGet, "/api/v1/admin/global/privacy-incidents", "", identity.CapabilityAccountsRead, now, http.StatusForbidden},
			{"list read cap", http.MethodGet, "/api/v1/admin/global/privacy-incidents?status=abierto&severity=alta", "", identity.CapabilityPrivacyIncidentsRead, now, http.StatusOK},
			{"list manage cap", http.MethodGet, "/api/v1/admin/global/privacy-incidents", "", identity.CapabilityPrivacyIncidentsManage, now, http.StatusOK},
			{"list invalid status", http.MethodGet, "/api/v1/admin/global/privacy-incidents?status=invalido", "", identity.CapabilityPrivacyIncidentsRead, now, http.StatusUnprocessableEntity},
			{"detail read cap", http.MethodGet, "/api/v1/admin/global/privacy-incidents/" + incidentID, "", identity.CapabilityPrivacyIncidentsRead, now, http.StatusOK},
			{"detail forbidden", http.MethodGet, "/api/v1/admin/global/privacy-incidents/" + incidentID, "", identity.CapabilityPrivacyRequestsRead, now, http.StatusForbidden},
			{"create read-only forbidden", http.MethodPost, "/api/v1/admin/global/privacy-incidents", createPayload, identity.CapabilityPrivacyIncidentsRead, now, http.StatusForbidden},
			{"create missing MFA", http.MethodPost, "/api/v1/admin/global/privacy-incidents", createPayload, identity.CapabilityPrivacyIncidentsManage, time.Time{}, http.StatusPreconditionRequired},
			{"create stale MFA", http.MethodPost, "/api/v1/admin/global/privacy-incidents", createPayload, identity.CapabilityPrivacyIncidentsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
			{"create happy path", http.MethodPost, "/api/v1/admin/global/privacy-incidents", createPayload, identity.CapabilityPrivacyIncidentsManage, now, http.StatusCreated},
			{"transition read-only forbidden", http.MethodPost, "/api/v1/admin/global/privacy-incidents/" + incidentID + "/transitions", transPayload, identity.CapabilityPrivacyIncidentsRead, now, http.StatusForbidden},
			{"transition missing MFA", http.MethodPost, "/api/v1/admin/global/privacy-incidents/" + incidentID + "/transitions", transPayload, identity.CapabilityPrivacyIncidentsManage, time.Time{}, http.StatusPreconditionRequired},
			{"transition happy path", http.MethodPost, "/api/v1/admin/global/privacy-incidents/" + incidentID + "/transitions", transPayload, identity.CapabilityPrivacyIncidentsManage, now, http.StatusOK},
		} {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:       actorID,
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}, svc, nil)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
		}
	})

	t.Run("privacy-legal-holds list/create/release permissions and MFA", func(t *testing.T) {
		createPayload := `{"account_id":"` + accountID + `","scope":"expediente_privacidad","authority":"Agencia de Protección de Datos","legal_basis":"Conservación preventiva por fiscalización administrativa en curso."}`
		releasePayload := `{"release_reason":"Fiscalización administrativa concluida sin requerir retención adicional."}`

		for _, tc := range []struct {
			name       string
			method     string
			path       string
			body       string
			capability identity.Capability
			mfaAt      time.Time
			wantStatus int
		}{
			{"list forbidden", http.MethodGet, "/api/v1/admin/global/privacy-legal-holds", "", identity.CapabilityAccountsRead, now, http.StatusForbidden},
			{"list incidents.read", http.MethodGet, "/api/v1/admin/global/privacy-legal-holds?status=activa&scope=expediente_privacidad", "", identity.CapabilityPrivacyIncidentsRead, now, http.StatusOK},
			{"list requests.read", http.MethodGet, "/api/v1/admin/global/privacy-legal-holds", "", identity.CapabilityPrivacyRequestsRead, now, http.StatusOK},
			{"list requests.manage", http.MethodGet, "/api/v1/admin/global/privacy-legal-holds", "", identity.CapabilityPrivacyRequestsManage, now, http.StatusOK},
			{"list invalid scope", http.MethodGet, "/api/v1/admin/global/privacy-legal-holds?scope=otro", "", identity.CapabilityPrivacyIncidentsRead, now, http.StatusUnprocessableEntity},
			{"create incidents.read forbidden", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds", createPayload, identity.CapabilityPrivacyIncidentsRead, now, http.StatusForbidden},
			{"create requests.manage forbidden", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds", createPayload, identity.CapabilityPrivacyRequestsManage, now, http.StatusForbidden},
			{"create stale MFA", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds", createPayload, identity.CapabilityPrivacyIncidentsManage, now.Add(-11 * time.Minute), http.StatusPreconditionRequired},
			{"create happy path", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds", createPayload, identity.CapabilityPrivacyIncidentsManage, now, http.StatusCreated},
			{"release incidents.read forbidden", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds/" + holdID + "/release", releasePayload, identity.CapabilityPrivacyIncidentsRead, now, http.StatusForbidden},
			{"release missing MFA", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds/" + holdID + "/release", releasePayload, identity.CapabilityPrivacyIncidentsManage, time.Time{}, http.StatusPreconditionRequired},
			{"release happy path", http.MethodPost, "/api/v1/admin/global/privacy-legal-holds/" + holdID + "/release", releasePayload, identity.CapabilityPrivacyIncidentsManage, now, http.StatusOK},
		} {
			store := newFakeSpecializedOpsStore(func() time.Time { return now })
			svc := &adminglobal.Service{}
			svc.SetSpecializedOpsStore(store)
			handler := NewHandler(accountEmailAuth{principal: identity.Principal{
				AccountID:       actorID,
				Capabilities:    []identity.Capability{tc.capability},
				AuthenticatedAt: tc.mfaAt,
			}}, svc, nil)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status=%d want=%d body=%s", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}
		}
	})
}
