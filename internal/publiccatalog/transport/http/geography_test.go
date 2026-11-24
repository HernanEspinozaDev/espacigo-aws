package publiccataloghttp

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
)

func TestMapReturnsOnlyMarkerProjectionAndSharedFilters(t *testing.T) {
	name := "Nombre Apellido"
	repo := &pageSizeRepository{spaces: []publiccatalog.Space{{SpaceID: "11111111-1111-4111-8111-111111111111", Title: "Sala", Description: "Privada", UsageRules: "Reglas", Host: &publiccatalog.PublicHost{Name: &name}, RateUnit: "hora", BasePriceCLP: 5000, Currency: "CLP", Location: &publiccatalog.ConfirmedLocation{Address: "Calle 123", Latitude: -33.45, Longitude: -70.66}}}}
	service, _ := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
	response := httptest.NewRecorder()
	New(service).ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/public/spaces/map?bbox=-71,-34,-70,-33&category_code=oficina&rate_unit=hora&min_unit_price_clp=1000", nil))
	if response.Code != 200 {
		t.Fatalf("status=%d %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || len(body.Items[0]) != 6 {
		t.Fatalf("marker=%s", response.Body.String())
	}
	for _, key := range []string{"space_id", "title", "base_price_clp", "currency", "rate_unit", "location"} {
		if _, ok := body.Items[0][key]; !ok {
			t.Fatal(key)
		}
	}
	if !repo.filters.MapOnly || repo.filters.Bounds == nil || repo.filters.CategoryCode != "oficina" || *repo.filters.MinUnitPriceCLP != 1000 {
		t.Fatalf("filters=%+v", repo.filters)
	}
}

func TestGeoHTTPRejectsMalformedFiltersBeforeRepository(t *testing.T) {
	for _, query := range []string{"bbox=1,2,3", "bbox=-71,-34,-70,-33&latitude=-33&longitude=-70&radius_km=10", "latitude=-33", "latitude=NaN&longitude=-70&radius_km=10", "latitude=-33&longitude=-70&radius_km=201", "bbox=-71,-34,-70,-33&bbox=-71,-34,-70,-33", "bbox=Infinity,-34,-70,-33"} {
		repo := &pageSizeRepository{}
		service, _ := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
		response := httptest.NewRecorder()
		New(service).ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/public/spaces/map?"+query, nil))
		if response.Code != 422 || repo.listCalls != 0 {
			t.Fatalf("query=%s status=%d calls=%d", query, response.Code, repo.listCalls)
		}
	}
}
