package publiccataloghttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
)

type verifiedFilterCaptureRepo struct {
	lastFilters publiccatalog.Filters
	listCalls   int
}

func (r *verifiedFilterCaptureRepo) Categories(context.Context) ([]publiccatalog.Category, error) {
	return nil, nil
}
func (r *verifiedFilterCaptureRepo) HasActiveCategory(context.Context, string) (bool, error) {
	return true, nil
}
func (r *verifiedFilterCaptureRepo) AttributeFilters(context.Context, string) ([]publiccatalog.AttributeFilter, error) {
	return nil, nil
}
func (r *verifiedFilterCaptureRepo) HasRegion(context.Context, string) (bool, error) { return true, nil }
func (r *verifiedFilterCaptureRepo) HasCommune(context.Context, string, string) (bool, error) {
	return true, nil
}
func (r *verifiedFilterCaptureRepo) Locations(context.Context, string, string, int) ([]publiccatalog.Location, error) {
	return nil, nil
}
func (r *verifiedFilterCaptureRepo) List(_ context.Context, filters publiccatalog.Filters, _ *publiccatalog.Cursor, _ int) ([]publiccatalog.Space, error) {
	r.lastFilters = filters
	r.listCalls++
	return []publiccatalog.Space{}, nil
}
func (r *verifiedFilterCaptureRepo) Detail(context.Context, string) (publiccatalog.Space, error) {
	return publiccatalog.Space{}, publiccatalog.ErrNotFound
}

func TestPublicFiltersParseVerifiedOnlyAndRejectInvalidValues(t *testing.T) {
	repo := &verifiedFilterCaptureRepo{}
	svc, err := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	h := New(svc)

	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "/api/v1/public/spaces?verified_only=true", want: true},
		{path: "/api/v1/public/spaces?verified_only=false", want: false},
		{path: "/api/v1/public/spaces?verified_only=", want: false},
		{path: "/api/v1/public/spaces/map?verified_only=true", want: true},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK || repo.lastFilters.VerifiedOnly != tc.want {
			t.Fatalf("%s status=%d verified_only=%t want=%t body=%s", tc.path, rec.Code, repo.lastFilters.VerifiedOnly, tc.want, rec.Body.String())
		}
	}

	callsBefore := repo.listCalls
	for _, invalid := range []string{
		"/api/v1/public/spaces?verified_only=abc",
		"/api/v1/public/spaces/map?verified_only=1",
		"/api/v1/public/spaces?verified_only=true&verified_only=false",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, invalid, nil))
		if rec.Code != http.StatusUnprocessableEntity || repo.listCalls != callsBefore {
			t.Fatalf("%s status=%d calls=%d want 422 and no repo call", invalid, rec.Code, repo.listCalls)
		}
	}
}
