package commercialproposalhttp

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
)

type authStub struct{ err error }

func (a authStub) Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error) {
	return identity.Principal{AccountID: "10000000-0000-4000-8000-000000000001"}, a.err
}

type serviceStub struct {
	err   error
	calls int
}

func (s *serviceStub) Create(context.Context, string, string) (commercialproposal.View, error) {
	s.calls++
	return commercialproposal.View{}, s.err
}
func (s *serviceStub) Get(context.Context, string, string) (commercialproposal.View, error) {
	s.calls++
	return commercialproposal.View{}, s.err
}
func TestCommercialProposalHTTPDependenciesAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, origin string
		serviceErr, authErr              error
		status                           int
		code                             string
		calls                            int
	}{
		{name: "create", method: "POST", status: 200, calls: 1},
		{name: "location", method: "POST", serviceErr: commercialproposal.ErrLocationRequired, status: 409, code: "space_location_confirmation_required", calls: 1},
		{name: "template", method: "POST", serviceErr: commercialproposal.ErrTemplateRequired, status: 409, code: "contract_template_required", calls: 1},
		{name: "keys", method: "POST", serviceErr: commercialproposal.ErrEvidenceUnavailable, status: 503, code: "contract_evidence_unavailable", calls: 1},
		{name: "policy", method: "POST", serviceErr: commercialproposal.ErrTermsUnavailable, status: 503, code: "contract_terms_unavailable", calls: 1},
		{name: "identity", method: "POST", serviceErr: rentalflow.ErrIdentityRequired, status: 409, code: "identity_required", calls: 1},
		{name: "private get", method: "GET", path: "/api/v1/rental-contract-proposals/proposal", serviceErr: commercialproposal.ErrNotFound, status: 404, code: "not_found", calls: 1},
		{name: "body rejected", method: "POST", body: `{"rut":"private"}`, status: 422, code: "invalid_request"},
		{name: "authentication", method: "POST", authErr: errors.New("unavailable session"), status: 401, code: "unauthenticated"},
		{name: "foreign origin", method: "POST", origin: "https://foreign.example", status: 403, code: "origin_denied"},
		{name: "wrong method", method: "PUT", status: 405, code: "method_not_allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &serviceStub{err: tc.serviceErr}
			h := New(authStub{tc.authErr}, s, nil)
			path := tc.path
			if path == "" {
				path = "/api/v1/rental-quotes/quote/contract-proposal"
			}
			r := httptest.NewRequest(tc.method, path, strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || s.calls != tc.calls || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private proposal is cacheable")
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Fatal("input leaked in error")
			}
		})
	}
}
