package kybhttp

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
)

const handlerOwnerID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

type handlerAuth struct{}

func (handlerAuth) Authorize(_ context.Context, token identity.Secret, role identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if token == "landlord-token" && (role == "" || role == identity.RoleLandlord) {
		return identity.Principal{AccountID: handlerOwnerID, Roles: []identity.Role{identity.RoleLandlord}}, nil
	}
	if token == "admin-token" && (role == "" || role == identity.RoleAdministrator) {
		return identity.Principal{AccountID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}, nil
	}
	return identity.Principal{}, identity.ErrForbidden
}

type handlerService struct {
	submitted bool
	listed    bool
	read      bool
	granular  bool
}

func (s *handlerService) Submit(_ context.Context, owner string, input kyb.SubmissionInput) (kyb.Verification, error) {
	s.submitted = owner == handlerOwnerID && len(input.Documents) == 3 && input.IdempotencyKey == "handler-key"
	return kyb.Verification{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Status: kyb.StatusPending}, nil
}
func (s *handlerService) Own(context.Context, string) (kyb.Verification, error) {
	return kyb.Verification{Status: kyb.StatusPending}, nil
}
func (s *handlerService) OwnForOrganization(context.Context, string, string) (kyb.Verification, error) {
	return kyb.Verification{Status: kyb.StatusPending}, nil
}
func (s *handlerService) Pending(context.Context) ([]kyb.Verification, error) {
	s.listed = true
	return []kyb.Verification{{Status: kyb.StatusInReview}}, nil
}
func (*handlerService) StartReview(context.Context, string, string) (kyb.Verification, error) {
	return kyb.Verification{Status: kyb.StatusInReview}, nil
}
func (*handlerService) ForReview(context.Context, string) (kyb.Verification, error) {
	return kyb.Verification{Status: kyb.StatusPending, CompanyRUT: "76123456-0"}, nil
}
func (*handlerService) Decide(context.Context, string, string, bool, string) (kyb.Verification, error) {
	return kyb.Verification{Status: kyb.StatusApproved}, nil
}
func (s *handlerService) DecideWithDocuments(context.Context,string,string,bool,[]kyb.DocumentDecision)(kyb.Verification,error){s.granular=true;return kyb.Verification{Status:kyb.StatusApproved},nil}
func (*handlerService) IssueReviewURL(context.Context, string, string, time.Time) (string, time.Time, error) {
	return "/api/v1/kyb/documents/eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee?expires=1", time.Now().Add(time.Minute), nil
}
func (s *handlerService) ReadSignedDocument(_ context.Context, storageRef, keyVersion, signature string, _ int64, _ time.Time) ([]byte, string, error) {
	s.read = storageRef == "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee" && keyVersion == "v1" && signature == "signed"
	return []byte("%PDF-1.4\nprivate"), "application/pdf", nil
}
func (*handlerService) SubmitPersonalSII(context.Context, string, string, kyb.Upload) (kyb.ComplianceDocument, error) {
	return kyb.ComplianceDocument{}, nil
}
func (*handlerService) PersonalCompliance(context.Context, string) (kyb.ComplianceProfile, error) {
	return kyb.ComplianceProfile{}, nil
}
func (*handlerService) SubmitSpaceProperty(context.Context, string, string, kyb.Upload) (kyb.ComplianceDocument, error) {
	return kyb.ComplianceDocument{}, nil
}
func (*handlerService) SpaceEligibility(context.Context, string, string) (kyb.SpaceEligibility, error) {
	return kyb.SpaceEligibility{}, nil
}
func (*handlerService) ComplianceProfiles(context.Context, string) ([]kyb.ComplianceProfile, error) {
	return nil, nil
}
func (*handlerService) SetComplianceDisabled(context.Context, string, string, string, bool) error {
	return nil
}
func (*handlerService) SetOrganizationDisabled(context.Context, string, string, string, bool) error {
	return nil
}
func (*handlerService) PendingComplianceDocuments(context.Context) ([]kyb.ComplianceDocument, error) {
	return nil, nil
}
func (*handlerService) StartComplianceReview(context.Context, string, string) (kyb.ComplianceDocument, error) {
	return kyb.ComplianceDocument{}, nil
}
func (*handlerService) DecideComplianceDocument(context.Context, string, string, bool, string) (kyb.ComplianceDocument, error) {
	return kyb.ComplianceDocument{}, nil
}
func (*handlerService) IssueComplianceReviewURL(context.Context, string, time.Time) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (*handlerService) ReadSignedComplianceDocument(context.Context, string, string, string, int64, time.Time) ([]byte, string, error) {
	return nil, "", nil
}

func TestOwnerSubmitRequiresLandlordAndThreeDocuments(t *testing.T) {
	service := &handlerService{}
	handler := NewHandler(handlerAuth{}, service, nil)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"legal_name":          "Empresa de prueba",
		"company_rut":         "76.123.456-0",
		"representative_name": "Representante de prueba",
		"representative_rut":  "12.345.678-5",
		"sii_giro_code":       "620100",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, field := range []string{"erut", "power_certificate", "property_or_sublease_authorization"} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="document.pdf"`)
		header.Set("Content-Type", "application/pdf")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, "%PDF-1.4\nsynthetic"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/kyb/verifications", &body)
	request.Header.Set("Authorization", "Bearer landlord-token")
	request.Header.Set("Idempotency-Key", "handler-key")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !service.submitted {
		t.Fatalf("submit status=%d submitted=%t body=%s", response.Code, service.submitted, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/kyb/verifications", strings.NewReader(""))
	request.Header.Set("Authorization", "Bearer admin-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-landlord submit status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminListAndTemporaryDocumentAccess(t *testing.T) {
	service := &handlerService{}
	handler := NewHandler(handlerAuth{}, service, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/kyb/verifications", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !service.listed {
		t.Fatalf("admin list status=%d listed=%t body=%s", response.Code, service.listed, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/kyb/documents/eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee?expires=1999999999&key_version=v1&signature=signed", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !service.read || response.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/pdf") {
		t.Fatalf("signed document status=%d read=%t headers=%v body=%s", response.Code, service.read, response.Header(), response.Body.String())
	}
}

func TestAdminDecisionAcceptsPerDocumentPayload(t *testing.T) {
	service := &handlerService{}
	handler := NewHandler(handlerAuth{}, service, nil)
	body := `{"decision":"REJECT","documents":[{"document_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd","decision":"APPROVE"},{"document_id":"ffffffff-ffff-4fff-8fff-ffffffffffff","decision":"REJECT","rejection_reason":"Vigencia vencida"}]}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/kyb/verifications/cccccccc-cccc-4ccc-8ccc-cccccccccccc/decision", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !service.granular {
		t.Fatalf("decision status=%d granular=%t body=%s", response.Code, service.granular, response.Body.String())
	}
}
