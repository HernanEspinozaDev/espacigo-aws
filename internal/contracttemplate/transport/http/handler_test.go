package contracttemplatehttp

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

const testSpace = "10000000-0000-4000-8000-000000000001"
const testActor = "10000000-0000-4000-8000-000000000002"

type authStub struct{ err error }

func (a authStub) Authorize(_ context.Context, _ identity.Secret, role identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if role != identity.RoleLandlord {
		return identity.Principal{}, errors.New("wrong role")
	}
	return identity.Principal{AccountID: testActor}, a.err
}

type repoStub struct {
	calls int
	err   error
}

func (s *repoStub) Get(context.Context, string, string) (contracttemplate.Draft, error) {
	s.calls++
	return contracttemplate.Draft{SpaceID: testSpace, Sections: []contracttemplate.Section{}}, s.err
}
func (s *repoStub) Save(_ context.Context, actor, space string, revision int64, sections []contracttemplate.Section) (contracttemplate.Draft, error) {
	s.calls++
	return contracttemplate.Draft{SpaceID: space, Revision: revision + 1, Sections: sections}, s.err
}
func (s *repoStub) Publish(context.Context, string, string, int64) (contracttemplate.Version, int64, error) {
	s.calls++
	return contracttemplate.Version{Version: 1}, 2, s.err
}
func (s *repoStub) Versions(context.Context, string, string) ([]contracttemplate.Version, error) {
	s.calls++
	return []contracttemplate.Version{}, s.err
}
func (s *repoStub) Copy(context.Context, string, string, int64, string, int64) (contracttemplate.Draft, error) {
	s.calls++
	return contracttemplate.Draft{Revision: 1}, s.err
}

func TestContractTemplateHTTPPreconditionsAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, method, action, body, etag, origin string
		authErr, repoErr                         error
		status, calls                            int
	}{
		{name: "private draft", method: "GET", action: "draft", status: 200, calls: 1},
		{name: "owner hidden", method: "GET", action: "draft", repoErr: contracttemplate.ErrNotFound, status: 404, calls: 1},
		{name: "landlord role required", method: "GET", action: "draft", authErr: identity.ErrForbidden, status: 403},
		{name: "session required", method: "GET", action: "draft", authErr: errors.New("session expired"), status: 401},
		{name: "origin denied", method: "GET", action: "draft", origin: "https://evil.example", status: 403},
		{name: "precondition required", method: "PUT", action: "draft", body: `{"sections":[]}`, status: 428},
		{name: "malformed precondition", method: "PUT", action: "draft", etag: `*`, body: `{"sections":[]}`, status: 400},
		{name: "duplicate keys", method: "PUT", action: "draft", etag: ETag(0), body: `{"sections":[],"sections":[]}`, status: 422},
		{name: "duplicate nested keys", method: "PUT", action: "draft", etag: ETag(0), body: `{"sections":[{"key":"uso","key":"otra","title":"Uso","body":"Reglas"}]}`, status: 422},
		{name: "unknown identity", method: "PUT", action: "draft", etag: ETag(0), body: `{"sections":[],"rut":"123"}`, status: 422},
		{name: "invalid clauses", method: "PUT", action: "draft", etag: ETag(0), body: `{"sections":[{"key":"uso","title":"Uso","body":"<b>Reglas</b>"}]}`, status: 422},
		{name: "save", method: "PUT", action: "draft", etag: ETag(0), body: `{"sections":[{"key":"uso","title":"Uso","body":"Reglas del espacio"}]}`, status: 200, calls: 1},
		{name: "stale revision", method: "POST", action: "publish", etag: ETag(1), repoErr: contracttemplate.ErrPrecondition, status: 412, calls: 1},
		{name: "publish rejects client snapshot", method: "POST", action: "publish", etag: ETag(1), body: `{"rut":"123"}`, status: 422},
		{name: "publish", method: "POST", action: "publish", etag: ETag(1), body: `{}`, status: 200, calls: 1},
		{name: "copy invalid version", method: "POST", action: "copy", etag: ETag(0), body: `{"source_space_id":"` + testSpace + `","source_version":0}`, status: 422},
		{name: "copy", method: "POST", action: "copy", etag: ETag(0), body: `{"source_space_id":"` + testSpace + `","source_version":1}`, status: 200, calls: 1},
		{name: "wrong method", method: "DELETE", action: "draft", status: 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repoStub{err: tc.repoErr}
			h := New(authStub{err: tc.authErr}, repo, []string{"https://allowed.example"})
			r := httptest.NewRequest(tc.method, "/api/v1/spaces/"+testSpace+"/contract-template/"+tc.action, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("If-Match", tc.etag)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || repo.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, repo.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
			if tc.name == "save" && w.Header().Get("ETag") != ETag(1) {
				t.Fatal("missing updated revision")
			}
		})
	}
}

func TestContractTemplateETagStrict(t *testing.T) {
	for _, v := range []string{`W/"contract-template-1"`, `"contract-template-01"`, `"contract-template--1"`, `"contract-template-1", "contract-template-2"`} {
		if _, ok := parseETag(v); ok {
			t.Fatalf("accepted %s", v)
		}
	}
}
