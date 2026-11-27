package commercialagreementhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

const ownerID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const renterID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
const agreementID = "11111111-1111-4111-8111-111111111111"
const proposalID = "22222222-2222-4222-8222-222222222222"
const sessionID = "33333333-3333-4333-8333-333333333333"

type auth struct{}

func (auth) Authorize(_ context.Context, token identity.Secret, role identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if token != "renter" && token != "owner" {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	id := renterID
	if token == "owner" {
		id = ownerID
	}
	if role == identity.RoleTenant && token != "renter" || role == identity.RoleLandlord && token != "owner" {
		return identity.Principal{}, identity.ErrForbidden
	}
	return identity.Principal{AccountID: id, SessionID: sessionID}, nil
}

type repository struct {
	domain.Repository
	calls        int
	err          error
	agreement    domain.Agreement
	lastActor    domain.Actor
	lastKey      string
	lastRevision int64
	role         string
}

func (r *repository) Create(_ context.Context, a domain.Actor, _ domain.CreateInput, key string) (domain.Agreement, error) {
	r.calls++
	r.lastActor = a
	r.lastKey = key
	return r.agreement, r.err
}
func (r *repository) Accept(_ context.Context, a domain.Actor, _ string, revision int64, _ domain.SignInput) (domain.Agreement, error) {
	r.calls++
	r.lastActor = a
	r.lastRevision = revision
	if a.ID != ownerID {
		return domain.Agreement{}, domain.ErrNotFound
	}
	return r.agreement, r.err
}
func (r *repository) Decide(_ context.Context, a domain.Actor, _ string, revision int64, _ domain.DecisionInput) (domain.Agreement, error) {
	r.calls++
	r.lastActor = a
	r.lastRevision = revision
	return r.agreement, r.err
}
func (r *repository) Get(context.Context, string, string) (domain.Agreement, error) {
	r.calls++
	return r.agreement, r.err
}
func (r *repository) List(_ context.Context, _ string, role string) ([]domain.Agreement, error) {
	r.calls++
	r.role = role
	return []domain.Agreement{r.agreement}, r.err
}
func fixture() (*repository, http.Handler) {
	at := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	repo := &repository{agreement: domain.Agreement{ID: agreementID, ProposalID: proposalID, SHA256: strings.Repeat("a", 64), OwnerID: ownerID, RenterID: renterID, State: domain.Pending, Revision: 1, CreatedAt: at, UpdatedAt: at, ExpiresAt: at.Add(time.Hour), Signatures: []domain.Signature{}}}
	service, _ := domain.New(repo)
	return repo, New(auth{}, service, []string{"https://app.example.test"})
}
func invoke(h http.Handler, method, path, token, body, etag, key, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if etag != "" {
		r.Header.Set("If-Match", etag)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestCommercialAgreementCreateBindsActorConsentAndKey(t *testing.T) {
	repo, h := fixture()
	body := `{"proposal_id":"` + proposalID + `","snapshot_sha256":"` + strings.Repeat("a", 64) + `","consent":true,"consent_version":"commercial-agreement-v1"}`
	w := invoke(h, "POST", "/api/v1/commercial-agreements", "renter", body, "", "intent-1", "https://app.example.test")
	if w.Code != 201 || repo.calls != 1 || repo.lastKey != "intent-1" || repo.lastActor.ID != renterID || repo.lastActor.SessionID != sessionID {
		t.Fatalf("status%d actor%+v key%s body%s", w.Code, repo.lastActor, repo.lastKey, w.Body.String())
	}
	if w.Header().Get("ETag") != ETag(agreementID, 1) || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "ETag") {
		t.Fatal("revision/privacy headers missing")
	}
	var dto map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto["id"] != agreementID || dto["data"] != nil || dto["owner_id"] != nil || strings.Contains(w.Body.String(), sessionID) || strings.Contains(w.Body.String(), ownerID) {
		t.Fatal("response shape/private evidence", w.Body.String())
	}
	repo.agreement.Replayed = true
	w = invoke(h, "POST", "/api/v1/commercial-agreements", "renter", body, "", "intent-1", "")
	if w.Code != 200 {
		t.Fatalf("replay status%d", w.Code)
	}
}
func TestCommercialAgreementRejectsUnsafeWritesBeforeRepository(t *testing.T) {
	good := `{"proposal_id":"` + proposalID + `","snapshot_sha256":"` + strings.Repeat("a", 64) + `","consent":true,"consent_version":"commercial-agreement-v1"}`
	for _, tc := range []struct {
		name, method, path, token, body, etag, key, origin string
		status                                             int
	}{
		{name: "missing key", method: "POST", path: "/api/v1/commercial-agreements", token: "renter", body: good, status: 422},
		{name: "false consent", method: "POST", path: "/api/v1/commercial-agreements", token: "renter", body: strings.Replace(good, `"consent":true`, `"consent":false`, 1), key: "key", status: 422},
		{name: "unknown field", method: "POST", path: "/api/v1/commercial-agreements", token: "renter", body: strings.TrimSuffix(good, "}") + `,"actor_id":"evil"}`, key: "key", status: 400},
		{name: "trailing JSON", method: "POST", path: "/api/v1/commercial-agreements", token: "renter", body: good + ` {}`, key: "key", status: 400},
		{name: "missing session", method: "POST", path: "/api/v1/commercial-agreements", body: good, key: "key", status: 401},
		{name: "wrong role", method: "POST", path: "/api/v1/commercial-agreements", token: "owner", body: good, key: "key", status: 403},
		{name: "cross origin", method: "POST", path: "/api/v1/commercial-agreements", token: "renter", body: good, key: "key", origin: "https://attacker.test", status: 403},
		{name: "missing CAS", method: "PUT", path: "/api/v1/commercial-agreements/" + agreementID + "/landlord-signature", token: "owner", body: `{}`, status: 428},
		{name: "weak CAS", method: "PUT", path: "/api/v1/commercial-agreements/" + agreementID + "/landlord-signature", token: "owner", body: `{}`, etag: `W/` + ETag(agreementID, 1), status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, h := fixture()
			w := invoke(h, tc.method, tc.path, tc.token, tc.body, tc.etag, tc.key, tc.origin)
			if w.Code != tc.status || repo.calls != 0 {
				t.Fatalf("status%d want%d calls%d body%s", w.Code, tc.status, repo.calls, w.Body.String())
			}
		})
	}
}
func TestCommercialAgreementSignatureAuthorizationIsEnforcedByRepository(t *testing.T) {
	repo, h := fixture()
	body := `{"snapshot_sha256":"` + strings.Repeat("a", 64) + `","consent":true,"consent_version":"commercial-agreement-v1"}`
	w := invoke(h, "PUT", "/api/v1/commercial-agreements/"+agreementID+"/landlord-signature", "renter", body, ETag(agreementID, 1), "", "")
	if w.Code != 404 || repo.calls != 1 {
		t.Fatalf("repository did not reject non-owner/non-representative signer: status%d calls%d body%s", w.Code, repo.calls, w.Body.String())
	}
}
func TestCommercialAgreementStableConflictCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{domain.ErrPrecondition, 412, "precondition_failed"}, {domain.ErrHash, 409, "proposal_changed"}, {domain.ErrExpired, 409, "request_expired"}, {domain.ErrIdentity, 409, "identity_verification_required"}, {domain.ErrIdempotency, 409, "idempotency_conflict"}, {domain.ErrConflict, 409, "agreement_conflict"}, {domain.ErrNotFound, 404, "not_found"}, {errors.New("secret database details"), 500, "internal_error"}} {
		t.Run(tc.code, func(t *testing.T) {
			repo, h := fixture()
			repo.err = tc.err
			body := `{"snapshot_sha256":"` + strings.Repeat("a", 64) + `","consent":true,"consent_version":"commercial-agreement-v1"}`
			w := invoke(h, "PUT", "/api/v1/commercial-agreements/"+agreementID+"/landlord-signature", "owner", body, ETag(agreementID, 1), "", "")
			if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("status%d body%s", w.Code, w.Body.String())
			}
		})
	}
}
func TestCommercialAgreementParticipantListHasStrictRoleFilter(t *testing.T) {
	repo, h := fixture()
	w := invoke(h, "GET", "/api/v1/commercial-agreements?role=landlord", "owner", "", "", "", "")
	if w.Code != 200 || repo.role != "landlord" || !strings.Contains(w.Body.String(), `"items":[`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"?role=landlord&role=renter", "?private=true", "?role=unknown", "?role="} {
		repo, h = fixture()
		w = invoke(h, "GET", "/api/v1/commercial-agreements"+query, "renter", "", "", "", "")
		if w.Code < 400 || repo.calls != 0 {
			t.Fatal("ambiguous role query reached repository", query, w.Code)
		}
	}
}
