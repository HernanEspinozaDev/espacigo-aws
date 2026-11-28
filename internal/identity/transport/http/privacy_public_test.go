package identityhttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type memoryPublicPrivacyRepo struct {
	stored map[string]storedPublicPrivacyCase
}

type storedPublicPrivacyCase struct {
	trackingHash []byte
	status       privacy.PublicRightsStatus
}

func (m *memoryPublicPrivacyRepo) GetProfile(context.Context, string) (privacy.Profile, error) {
	return privacy.Profile{}, privacy.ErrNotFound
}

func (m *memoryPublicPrivacyRepo) SaveProfile(context.Context, string, string, *string) (privacy.Profile, error) {
	return privacy.Profile{}, privacy.ErrInvalid
}

func (m *memoryPublicPrivacyRepo) CreateRightsRequest(context.Context, string, string, string) (privacy.RightsRequest, error) {
	return privacy.RightsRequest{}, privacy.ErrInvalid
}

func (m *memoryPublicPrivacyRepo) ListOwnRightsRequests(context.Context, string) ([]privacy.RightsRequest, error) {
	return nil, nil
}

func (m *memoryPublicPrivacyRepo) ExportOwnData(context.Context, string) (privacy.OwnData, error) {
	return privacy.OwnData{}, nil
}

func (m *memoryPublicPrivacyRepo) CreatePublicRightsRequest(_ context.Context, in privacy.PublicRightsRequestInput, folio string, trackingHash []byte, now, dueAt time.Time) (privacy.PublicRightsReceipt, error) {
	if m.stored == nil {
		m.stored = make(map[string]storedPublicPrivacyCase)
	}
	hashCopy := append([]byte(nil), trackingHash...)
	dueCopy := dueAt.UTC()
	createdAt := now.UTC()
	state := "identidad_pendiente"
	m.stored[folio] = storedPublicPrivacyCase{
		trackingHash: hashCopy,
		status: privacy.PublicRightsStatus{
			Folio:         folio,
			RightType:     in.RightType,
			RequesterType: in.RequesterType,
			State:         state,
			CreatedAt:     createdAt,
			DueAt:         &dueCopy,
		},
	}
	return privacy.PublicRightsReceipt{
		ID:            "req-public-1",
		Folio:         folio,
		RightType:     in.RightType,
		RequesterType: in.RequesterType,
		State:         state,
		Channel:       "publico",
		DueAt:         dueCopy,
		CreatedAt:     createdAt,
	}, nil
}

func (m *memoryPublicPrivacyRepo) LookupPublicRightsRequest(_ context.Context, folio string, trackingHash []byte) (privacy.PublicRightsStatus, error) {
	item, ok := m.stored[folio]
	if !ok {
		return privacy.PublicRightsStatus{}, privacy.ErrNotFound
	}
	if len(item.trackingHash) != 32 || len(trackingHash) != 32 || subtle.ConstantTimeCompare(item.trackingHash, trackingHash) != 1 {
		return privacy.PublicRightsStatus{}, privacy.ErrNotFound
	}
	return item.status, nil
}

func newPublicPrivacyTestHandler(t *testing.T, withPrivacy bool) http.Handler {
	t.Helper()
	var svc *privacy.Service
	if withPrivacy {
		var err error
		svc, err = privacy.NewService(&memoryPublicPrivacyRepo{})
		if err != nil {
			t.Fatalf("privacy.NewService: %v", err)
		}
	}
	return NewHandler(nil, nil, []string{"http://localhost:3000"}, svc)
}

func doPublicPrivacyJSONRequest(h http.Handler, method, path, body, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestPublicPrivacyRequestsCreateAndLookupAnonymous(t *testing.T) {
	handler := newPublicPrivacyTestHandler(t, true)

	createBody := `{
		"right_type": "acceso",
		"requester_type": "titular",
		"requester_name": "Camila Rojas",
		"requester_email": "camila@example.com",
		"detail": "Solicito copia de los datos personales asociados a mi historial."
	}`
	createRes := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests", createBody, "http://localhost:3000")
	if createRes.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/privacy/public-requests status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	if createRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing Cache-Control: no-store, got %q", createRes.Header().Get("Cache-Control"))
	}
	if createRes.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("missing CORS allow-origin header, got %q", createRes.Header().Get("Access-Control-Allow-Origin"))
	}

	var created struct {
		Data privacy.PublicRightsReceipt `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data.Folio == "" || created.Data.TrackingCode == "" {
		t.Fatalf("expected non-empty folio and tracking_code, got %+v", created.Data)
	}

	lookupBody, err := json.Marshal(privacy.PublicRightsLookupInput{
		Folio:        created.Data.Folio,
		TrackingCode: created.Data.TrackingCode,
	})
	if err != nil {
		t.Fatalf("marshal lookup body: %v", err)
	}
	lookupRes := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests/lookup", string(lookupBody), "http://localhost:3000")
	if lookupRes.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/privacy/public-requests/lookup status = %d, body = %s", lookupRes.Code, lookupRes.Body.String())
	}
	if lookupRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing Cache-Control: no-store on lookup, got %q", lookupRes.Header().Get("Cache-Control"))
	}

	var lookedUp struct {
		Data privacy.PublicRightsStatus `json:"data"`
	}
	if err := json.Unmarshal(lookupRes.Body.Bytes(), &lookedUp); err != nil {
		t.Fatalf("decode lookup response: %v", err)
	}
	if lookedUp.Data.Folio != created.Data.Folio || lookedUp.Data.RightType != "acceso" || lookedUp.Data.State != "identidad_pendiente" {
		t.Fatalf("unexpected lookup payload: %+v", lookedUp.Data)
	}

	// Uniform 404 for wrong tracking code and unknown folio.
	wrongCodeBody, _ := json.Marshal(privacy.PublicRightsLookupInput{
		Folio:        created.Data.Folio,
		TrackingCode: strings.Repeat("A", 32),
	})
	wrongCodeRes := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests/lookup", string(wrongCodeBody), "")
	unknownFolioBody, _ := json.Marshal(privacy.PublicRightsLookupInput{
		Folio:        "PRIV-2026-DEADBEEF",
		TrackingCode: created.Data.TrackingCode,
	})
	unknownFolioRes := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests/lookup", string(unknownFolioBody), "")
	if wrongCodeRes.Code != http.StatusNotFound || unknownFolioRes.Code != http.StatusNotFound {
		t.Fatalf("expected uniform 404, got wrongCode=%d unknownFolio=%d", wrongCodeRes.Code, unknownFolioRes.Code)
	}
	var errWrong, errUnknown struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(wrongCodeRes.Body.Bytes(), &errWrong)
	_ = json.Unmarshal(unknownFolioRes.Body.Bytes(), &errUnknown)
	if errWrong.Error.Code != "not_found" || errWrong.Error != errUnknown.Error {
		t.Fatalf("404 error responses must be uniform: wrong=%+v unknown=%+v", errWrong, errUnknown)
	}
}

func TestPublicPrivacyRequestsErrorsAndMethods(t *testing.T) {
	handler := newPublicPrivacyTestHandler(t, true)

	for _, path := range []string{"/api/v1/privacy/public-requests", "/api/v1/privacy/public-requests/lookup"} {
		res := doPublicPrivacyJSONRequest(handler, http.MethodGet, path, "", "")
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want 405", path, res.Code)
		}
		if res.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("GET %s Allow = %q, want POST", path, res.Header().Get("Allow"))
		}
		if !strings.Contains(res.Body.String(), `"method_not_allowed"`) {
			t.Fatalf("GET %s expected method_not_allowed, got %s", path, res.Body.String())
		}

		denied := doPublicPrivacyJSONRequest(handler, http.MethodPost, path, `{}`, "http://evil.invalid")
		if denied.Code != http.StatusForbidden {
			t.Fatalf("disallowed origin on %s status = %d, want 403", path, denied.Code)
		}
	}

	invalidCreate := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests", `{
		"right_type": "desconocido",
		"requester_type": "titular",
		"requester_name": "Camila",
		"requester_email": "camila@example.com",
		"detail": "Detalle suficientemente largo para validar."
	}`, "")
	if invalidCreate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create status = %d, want 422, body = %s", invalidCreate.Code, invalidCreate.Body.String())
	}

	invalidLookup := doPublicPrivacyJSONRequest(handler, http.MethodPost, "/api/v1/privacy/public-requests/lookup", `{"folio":""}`, "")
	if invalidLookup.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid lookup status = %d, want 422, body = %s", invalidLookup.Code, invalidLookup.Body.String())
	}

	unavailable := newPublicPrivacyTestHandler(t, false)
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/v1/privacy/public-requests", `{"right_type":"acceso"}`},
		{"/api/v1/privacy/public-requests/lookup", `{"folio":"PRIV-2026-DEADBEEF","tracking_code":"0123456789ABCDEF"}`},
	} {
		res := doPublicPrivacyJSONRequest(unavailable, http.MethodPost, tc.path, tc.body, "")
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil privacy on %s status = %d, want 503, body = %s", tc.path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), `"privacy_unavailable"`) {
			t.Fatalf("nil privacy on %s expected privacy_unavailable, got %s", tc.path, res.Body.String())
		}
	}
}

func TestPublicPrivacyRequestsRateLimitByRequesterAndEndpoint(t *testing.T) {
	h := newHandler(nil, nil, nil, nil, mustPublicPrivacyService(t)).(*Handler)
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	var body = `{"right_type":"acceso","requester_type":"titular","requester_name":"Camila Rojas","requester_email":"camila@example.com","detail":"Solicito copia de mis datos personales."}`
	for request := 0; request < 5; request++ {
		req := httptest.NewRequest(http.MethodPost, privacyPublicRequestsPath, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusCreated {
			t.Fatalf("request %d status = %d, want 201, body=%s", request+1, res.Code, res.Body.String())
		}
	}
	blocked := httptest.NewRequest(http.MethodPost, privacyPublicRequestsPath, strings.NewReader(body))
	blocked.Header.Set("Content-Type", "application/json")
	blockedRes := httptest.NewRecorder()
	h.ServeHTTP(blockedRes, blocked)
	if blockedRes.Code != http.StatusTooManyRequests || blockedRes.Header().Get("Retry-After") != "3600" {
		t.Fatalf("sixth submission status/retry-after = %d/%q, want 429/3600", blockedRes.Code, blockedRes.Header().Get("Retry-After"))
	}

	otherRequesterBody := strings.Replace(body, "camila@example.com", "ines@example.com", 1)
	otherRequester := httptest.NewRequest(http.MethodPost, privacyPublicRequestsPath, strings.NewReader(otherRequesterBody))
	otherRequester.Header.Set("Content-Type", "application/json")
	otherRequesterRes := httptest.NewRecorder()
	h.ServeHTTP(otherRequesterRes, otherRequester)
	if otherRequesterRes.Code != http.StatusCreated {
		t.Fatalf("separate requester status = %d, want 201, body=%s", otherRequesterRes.Code, otherRequesterRes.Body.String())
	}

	now = now.Add(time.Hour)
	retried := httptest.NewRequest(http.MethodPost, privacyPublicRequestsPath, strings.NewReader(body))
	retried.Header.Set("Content-Type", "application/json")
	retriedRes := httptest.NewRecorder()
	h.ServeHTTP(retriedRes, retried)
	if retriedRes.Code != http.StatusCreated {
		t.Fatalf("request after window status = %d, want 201, body=%s", retriedRes.Code, retriedRes.Body.String())
	}
}

func mustPublicPrivacyService(t *testing.T) *privacy.Service {
	t.Helper()
	svc, err := privacy.NewService(&memoryPublicPrivacyRepo{})
	if err != nil {
		t.Fatalf("privacy.NewService: %v", err)
	}
	return svc
}

func TestPublicPrivacyRateLimitsLookupSeparatelyAndBoundKeys(t *testing.T) {
	limiter := newPublicPrivacyRateLimiter()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	for request := 0; request < 20; request++ {
		allowed, _ := limiter.allow(privacyPublicRequestsLookupPath, "PRIV-2026-ABCDEF12", now)
		if !allowed {
			t.Fatalf("lookup request %d was blocked before the limit", request+1)
		}
	}
	allowed, retryAfter := limiter.allow(privacyPublicRequestsLookupPath, "PRIV-2026-ABCDEF12", now)
	if allowed || retryAfter != 15*time.Minute {
		t.Fatalf("lookup limit result = %t, %s, want blocked for 15 minutes", allowed, retryAfter)
	}
	allowed, _ = limiter.allow(privacyPublicRequestsPath, "camila@example.com", now)
	if !allowed {
		t.Fatal("lookup limit unexpectedly blocked a new submission bucket")
	}

	for request := 0; request < maxPublicPrivacyRateLimitKeys; request++ {
		limiter.allow(privacyPublicRequestsPath, "requester"+strconv.Itoa(request)+"@example.com", now)
	}
	if len(limiter.buckets) > maxPublicPrivacyRateLimitKeys {
		t.Fatalf("limiter retained %d keys, maximum is %d", len(limiter.buckets), maxPublicPrivacyRateLimitKeys)
	}
}

func TestPublicPrivacyRateLimiterCleansExpiredKeysAndClampsClock(t *testing.T) {
	limiter := newPublicPrivacyRateLimiter()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	for request := 0; request < 5; request++ {
		allowed, _ := limiter.allow(privacyPublicRequestsPath, "camila@example.com", now)
		if !allowed {
			t.Fatalf("request %d was unexpectedly blocked", request+1)
		}
	}
	allowed, retryAfter := limiter.allow(privacyPublicRequestsPath, "camila@example.com", now.Add(-2*time.Hour))
	if allowed || retryAfter != time.Hour {
		t.Fatalf("backward-clock request result = %t, %s, want blocked for an hour", allowed, retryAfter)
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("backward clock changed limiter state: %d buckets", len(limiter.buckets))
	}

	now = now.Add(2 * time.Hour)
	allowed, _ = limiter.allow(privacyPublicRequestsPath, "ines@example.com", now)
	if !allowed {
		t.Fatal("request after expiry was unexpectedly blocked")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("expired-key cleanup retained %d buckets, want only the new identity and aggregate buckets", len(limiter.buckets))
	}
}
