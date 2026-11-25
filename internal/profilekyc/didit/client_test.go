package didit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestNewClientRestrictsAPIHostAndRequiresKey(t *testing.T) {
	for _, cfg := range []Config{
		{BaseURL: "https://evil.example", APIKey: "test"},
		{BaseURL: "http://verification.didit.me", APIKey: "test"},
		{BaseURL: "https://user@verification.didit.me", APIKey: "test"},
		{BaseURL: "https://verification.didit.me:444", APIKey: "test"},
		{BaseURL: APIBaseURL},
	} {
		if _, err := NewClient(cfg); err == nil {
			t.Fatalf("NewClient(%+v) unexpectedly succeeded", cfg)
		}
	}
	if _, err := NewClient(Config{APIKey: "test"}); err != nil {
		t.Fatalf("default config: %v", err)
	}
}

func TestCreateSessionUsesV3AndValidatesHostedHost(t *testing.T) {
	calls := 0
	client, err := NewClient(Config{APIKey: "sandbox-key", HTTPClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.Path != "/v3/session/" || req.Header.Get("x-api-key") != "sandbox-key" {
			t.Fatalf("unexpected request: %s %s headers=%v", req.Method, req.URL, req.Header)
		}
		b, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(b), `"vendor_data":"attempt-opaque-uuid"`) || !strings.Contains(string(b), `"identification_number":"12345678-5"`) || !strings.Contains(string(b), `"expected_document_types":["ID"]`) {
			t.Fatalf("request omitted opaque reference or expected details: %s", b)
		}
		return testResponse(http.StatusCreated, `{"session_id":"s-1","url":"https://verify.didit.me/session/s-1","status":"Not Started"}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CreateSession(context.Background(), CreateSessionRequest{WorkflowID: "wf-sandbox", VendorData: "attempt-opaque-uuid", ExpectedDetails: ExpectedDetails{FirstName: "Ana", LastName: "Pérez", DateOfBirth: "1990-01-02", IdentificationNumber: "12345678-5", ExpectedDocumentTypes: []string{"ID"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "s-1" || got.SessionURL != "https://verify.didit.me/session/s-1" || calls != 1 {
		t.Fatalf("unexpected response/call count: %+v calls=%d", got, calls)
	}

	bad, err := NewClient(Config{APIKey: "sandbox-key", HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(http.StatusCreated, `{"session_id":"s-2","url":"https://attacker.example/steal"}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bad.CreateSession(context.Background(), CreateSessionRequest{WorkflowID: "wf", VendorData: "opaque"}); err != ErrInvalidURL {
		t.Fatalf("unsafe host error = %v", err)
	}
}

func TestGetDecisionDecodesV3PluralFeatureReports(t *testing.T) {
	client, err := NewClient(Config{APIKey: "key", HTTPClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v3/session/session-id/decision/" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		return testResponse(http.StatusOK, `{"session_id":"session-id","workflow_id":"wf","vendor_data":"attempt-id","status":"Approved","id_verifications":[{"status":"Approved","first_name":"Ana","last_name":"Pérez","date_of_birth":"1990-01-02","identification_number":"12.345.678-5","personal_number":"12.345.678-5","document_number":"A123456","issuing_state":"CHL","document_type":"ID","expiration_date":"2030-01-01"}],"liveness_checks":[{"status":"Approved","score":0.98}],"face_matches":[{"status":"Approved","score":0.94}]}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetDecision(context.Background(), "session-id")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-id" || got.WorkflowID != "wf" || got.VendorData != "attempt-id" || got.Status != "Approved" || len(got.IDVerifications) != 1 {
		t.Fatalf("decision envelope not decoded: %+v", got)
	}
	id := got.IDVerifications[0]
	if id.FirstName != "Ana" || id.IdentificationNumber != "12.345.678-5" || id.PersonalNumber != "12.345.678-5" || id.DocumentNumber != "A123456" || id.IssuingState != "CHL" || id.ExpirationDate != "2030-01-01" {
		t.Fatalf("identity fields not decoded: %+v", id)
	}
	if !got.LivenessPassed() || !got.FaceMatchPassed() {
		t.Fatal("approved feature results should pass")
	}
	got.LivenessChecks[0].Status = "In Review"
	if got.LivenessPassed() {
		t.Fatal("in-review liveness should not pass")
	}
	got.LivenessChecks = append(got.LivenessChecks, CheckResult{Status: "Approved"})
	if got.LivenessPassed() {
		t.Fatal("a mixed result set should not pass")
	}
	if (Decision{}).LivenessPassed() || (Decision{}).FaceMatchPassed() {
		t.Fatal("missing feature result must not pass")
	}
}

func TestGetDecisionRejectsPathInjectionAndMapsRateLimit(t *testing.T) {
	client, err := NewClient(Config{APIKey: "key", HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return testResponse(http.StatusTooManyRequests, ""), nil })}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetDecision(context.Background(), "../escape"); err == nil {
		t.Fatal("path injection accepted")
	}
	if _, err := client.GetDecision(context.Background(), "session"); err != ErrRateLimited {
		t.Fatalf("rate limit error = %v", err)
	}
}

func TestClientHonorsContextAndTimeout(t *testing.T) {
	client, err := NewClient(Config{APIKey: "key", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetDecision(ctx, "session"); err == nil {
		t.Fatal("expected canceled context")
	}
}
