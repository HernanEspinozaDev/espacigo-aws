package spaceshttp

import (
	"context"
	"errors"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeGeocoder struct {
	query string
	err   error
}
type fakeReverseGeocoder struct {
	address string
	err     error
}

func (f fakeReverseGeocoder) Reverse(context.Context, float64, float64) (string, error) {
	return f.address, f.err
}

type reverseTestBounds struct{}

func (reverseTestBounds) Contains(string, float64, float64) (bool, bool) { return false, true }
func (reverseTestBounds) Bounds(string) (float64, float64, float64, float64, bool) {
	return 0, 0, 0, 0, true
}
func (reverseTestBounds) Find(lat, lon float64) (string, bool) {
	if lat < -33 && lon < -70 {
		return "13101", true
	}
	return "", false
}

func (f *fakeGeocoder) Search(_ context.Context, query string) ([]GeocodeCandidate, error) {
	f.query = query
	if f.err != nil {
		return nil, f.err
	}
	return []GeocodeCandidate{{Label: "Prueba 123", Latitude: -33.45, Longitude: -70.66, Precision: "address"}}, nil
}

type fakeLocationCatalog struct{ found bool }

func (f fakeLocationCatalog) ResolveLocation(context.Context, string, string) (string, string, bool, error) {
	return "Metropolitana de Santiago", "Santiago", f.found, nil
}

func TestGeocodeRequiresSessionAndCatalog(t *testing.T) {
	geo := &fakeGeocoder{}
	h := NewHandler(fakeAuth{}, nil, nil).(*Handler)
	h.SetGeocoder(geo, fakeLocationCatalog{found: true})
	request := func(token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/spaces/location/geocode", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	valid := `{"address":"Alameda 123","region_code":"13","comuna_code":"13101"}`
	if got := request("", valid); got.Code != 401 {
		t.Fatalf("unauthorized=%d", got.Code)
	}
	if got := request("tenant", valid); got.Code != 200 || !strings.Contains(got.Body.String(), `"precision":"address"`) {
		t.Fatalf("valid=%d %s", got.Code, got.Body.String())
	}
	if geo.query != "Alameda 123, Santiago, Metropolitana de Santiago, Chile" {
		t.Fatalf("query=%q", geo.query)
	}
	if got := request("tenant", `{"address":"Alameda 123"}`); got.Code != 200 || geo.query != "Alameda 123, Chile" {
		t.Fatalf("address-only search=%d query=%q body=%s", got.Code, geo.query, got.Body.String())
	}
	if got := request("tenant", `{"address":"Alameda 123","region_code":"13"}`); got.Code != 422 {
		t.Fatalf("unpaired codes=%d %s", got.Code, got.Body.String())
	}
	geo.query = ""
	if got := request("tenant", `{"address":"contacto@example.com","region_code":"13","comuna_code":"13101"}`); got.Code != 422 || geo.query != "" {
		t.Fatalf("contact=%d", got.Code)
	}
	h.SetGeocoder(geo, fakeLocationCatalog{found: false})
	if got := request("tenant", valid); got.Code != 422 || geo.query != "" {
		t.Fatalf("catalog=%d", got.Code)
	}
}

func TestGeocodeUnavailableAndRateLimited(t *testing.T) {
	h := NewHandler(fakeAuth{}, nil, nil).(*Handler)
	path := "/api/v1/spaces/location/geocode"
	valid := `{"address":"Alameda 123","region_code":"13","comuna_code":"13101"}`
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(valid))
		r.Header.Set("Authorization", "Bearer tenant")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := call(); got.Code != 503 {
		t.Fatalf("disabled=%d", got.Code)
	}
	geo := &fakeGeocoder{err: errors.New("provider credentials")}
	h.SetGeocoder(geo, fakeLocationCatalog{found: true})
	for i := 0; i < 10; i++ {
		if got := call(); got.Code != 503 || strings.Contains(got.Body.String(), "provider credentials") {
			t.Fatalf("provider=%d %s", got.Code, got.Body.String())
		}
	}
	if got := call(); got.Code != 429 {
		t.Fatalf("limit=%d", got.Code)
	}
}

func TestReverseGeocodeUsesPolygonFallbackWithoutProvider(t *testing.T) {
	svc, err := spaces.NewService(&memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetLocationBoundaryChecker(reverseTestBounds{})
	h := NewHandler(fakeAuth{}, svc, nil).(*Handler)
	h.SetReverseGeocoder(nil, fakeLocationCatalog{found: true})
	call := func(token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/spaces/location/reverse-geocode", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := call("", `{"latitude":-33.45,"longitude":-70.66}`); got.Code != 401 {
		t.Fatalf("auth=%d", got.Code)
	}
	got := call("tenant", `{"latitude":-33.45,"longitude":-70.66}`)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"region_code":"13"`) || !strings.Contains(got.Body.String(), `"comuna_code":"13101"`) || strings.Contains(got.Body.String(), `"address"`) {
		t.Fatalf("fallback=%d %s", got.Code, got.Body.String())
	}
	if got = call("tenant", `{"latitude":-91,"longitude":0}`); got.Code != 422 {
		t.Fatalf("range=%d %s", got.Code, got.Body.String())
	}
}

func TestReverseGeocodeAddsAddressWhenProviderResponds(t *testing.T) {
	svc, _ := spaces.NewService(&memoryRepo{})
	svc.SetLocationBoundaryChecker(reverseTestBounds{})
	h := NewHandler(fakeAuth{}, svc, nil).(*Handler)
	h.SetReverseGeocoder(fakeReverseGeocoder{address: "Alameda 123"}, fakeLocationCatalog{found: true})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/spaces/location/reverse-geocode", strings.NewReader(`{"latitude":-33.45,"longitude":-70.66}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer tenant")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"address":"Alameda 123"`) {
		t.Fatalf("response=%d %s", w.Code, w.Body.String())
	}
}
