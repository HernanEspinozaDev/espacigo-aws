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
	now             func() time.Time
	payments        []adminglobal.PaymentItem
	reservations    []adminglobal.ReservationItem
	contracts       []adminglobal.ContractItem
	kycCases        []adminglobal.KYCCaseItem
	privacyRequests []adminglobal.PrivacyRequestItem
	tickets         []adminglobal.SupportTicketItem
	approvals       map[string]adminglobal.ApprovalItem
	idemKeys        map[string]string
	lastPayFilter   adminglobal.PaymentFilter
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

func (f *fakeSpecializedOpsStore) ListPrivacyRequests(_ context.Context, _, _ string, _ adminglobal.PrivacyRequestFilter) (adminglobal.OpsPage[adminglobal.PrivacyRequestItem], error) {
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
