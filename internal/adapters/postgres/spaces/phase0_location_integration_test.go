package spacespg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	occupancypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/occupancy"
	publiccatalogpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/publiccatalog"
	"github.com/HernanEspinozaDev/espaciGo/internal/geoboundary"
	"github.com/HernanEspinozaDev/espaciGo/internal/occupancy"
	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	spaceshttp "github.com/HernanEspinozaDev/espaciGo/internal/spaces/transport/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

type phase0LocationCatalog struct{ pool *pgxpool.Pool }

func (c phase0LocationCatalog) ResolveLocation(ctx context.Context, region, commune string) (string, string, bool, error) {
	var found bool
	err := c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.comuna_publica c
		JOIN public.catalogo_ubicacion_publica v ON v.version=c.catalogo_version AND v.activo
		WHERE c.codigo=$1 AND c.region_codigo=$2)`, commune, region).Scan(&found)
	return region, commune, found, err
}

func TestPhase0LocationFromDraftToPublicMap(t *testing.T) {
	testURL := os.Getenv("PHASE0_ISOLATED_DATABASE_URL")
	boundsPath := os.Getenv("ESPACIGO_COMMUNE_BOUNDARIES_FILE")
	if testURL == "" || boundsPath == "" {
		t.Skip("requires disposable PostgreSQL and the official commune geometry")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	config, err := pgxpool.ParseConfig(testURL)
	if err != nil || config.ConnConfig.Database != "phase0_location_test" ||
		!filepath.IsAbs(config.ConnConfig.Host) {
		t.Fatal("Phase 0 requires an isolated Unix socket database named phase0_location_test")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var database string
	var users int
	if err := pool.QueryRow(ctx, "SELECT current_database(), (SELECT count(*) FROM public.usuario)").Scan(&database, &users); err != nil ||
		database != "phase0_location_test" || users != 0 {
		t.Fatalf("Phase 0 requires the disposable schema with no copied accounts: %v", err)
	}

	const owner = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado)
		VALUES($1,'phase0-pin@example.test','phase0-pin@example.test','synthetic','activo')`, owner); err != nil {
		t.Fatal(err)
	}
	spaceService, err := spaces.NewService(New(pool, credentials.Generator{}))
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := geoboundary.Load(boundsPath)
	if err != nil {
		t.Fatal(err)
	}
	spaceService.SetLocationBoundaryChecker(bounds)
	spaceService.SetLocationCatalog(phase0LocationCatalog{pool})
	calendar, err := occupancy.NewService(occupancypg.New(pool), credentials.Generator{})
	if err != nil {
		t.Fatal(err)
	}
	spaceAPI := spaceshttp.NewHandlerWithPricing(publicationFlowAuth{ownerID: owner}, spaceService, nil, calendar, nil)
	catalog, err := publiccatalog.New(publiccatalogpg.New(pool), []byte("phase-zero-public-catalog-synthetic-key"))
	if err != nil {
		t.Fatal(err)
	}
	publicAPI := publiccataloghttp.New(catalog)
	call := func(handler http.Handler, method, path, etag string, input any, status int) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if input != nil {
			var err error
			body, err = json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer owner-token")
		if input != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s: status = %d, want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response
	}

	lat, lng := -33.45226, -70.65857
	address := "Dirección sintética 100, Santiago"
	draft := spaces.CreateInput{
		CategoryCode: "oficina", Title: "Espacio sintético Phase 0", AreaM2: 30,
		Capacity: 5, UsageRules: "Sin fumar", RateUnit: "hora", BasePriceCLP: 9000,
		Location: spaces.LocationInput{Address: address, RegionCode: "05", CommuneCode: "05101", Latitude: &lat, Longitude: &lng, Source: "manual"},
	}
	const createPath = "/api/v1/spaces"
	call(spaceAPI, http.MethodPost, createPath, "", draft, http.StatusUnprocessableEntity)
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM public.espacio WHERE propietario_id=$1", owner).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("invalid pin left %d drafts: %v", rows, err)
	}
	draft.Location.RegionCode, draft.Location.CommuneCode = "13", "13101"
	draft.Location.PublicConfirmed = true
	created := call(spaceAPI, http.MethodPost, createPath, "", draft, http.StatusCreated)
	var item spaces.Draft
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil || item.ID == "" {
		t.Fatalf("decode draft id: %v", err)
	}
	path := createPath + "/" + item.ID
	private := call(spaceAPI, http.MethodGet, path+"/location", "", nil, http.StatusOK)
	expectedLocation := publiccatalog.ConfirmedLocation{Address: address, Latitude: lat, Longitude: lng}
	var own spaces.CommercialLocation
	if err := json.Unmarshal(private.Body.Bytes(), &own); err != nil || own.Latitude == nil || own.Longitude == nil || own.RegionCode != "13" || own.CommuneCode != "13101" || *own.Latitude != lat || *own.Longitude != lng {
		t.Fatalf("location was not written atomically: %v", err)
	}

	movedLat, movedLng := -33.11533, -71.56463
	movedAddress := "Dirección sintética 250, Valparaíso"
	update := spaces.LocationInput{
		Address: movedAddress, RegionCode: "05", CommuneCode: "05101",
		Latitude: &movedLat, Longitude: &movedLng, Source: "manual", PublicConfirmed: true,
	}
	// Use an interior point from the confirmed local DPA snapshot for Valparaíso.
	if code, ok := bounds.Find(movedLat, movedLng); !ok || code != update.CommuneCode {
		t.Fatalf("invalid Phase 0 Valparaíso fixture: code %s", code)
	}
	tag := private.Header().Get("ETag")
	update.CommuneCode = "13101"
	update.RegionCode = "13"
	update.PublicConfirmed = false
	call(spaceAPI, http.MethodPut, path+"/location", tag, update, http.StatusUnprocessableEntity)
	update.CommuneCode, update.RegionCode, update.PublicConfirmed = "05101", "05", true
	updated := call(spaceAPI, http.MethodPut, path+"/location", tag, update, http.StatusOK)
	call(spaceAPI, http.MethodPut, path+"/location", tag, update, http.StatusPreconditionFailed)
	expectedLocation = publiccatalog.ConfirmedLocation{Address: movedAddress, Latitude: movedLat, Longitude: movedLng}

	content := map[string]any{
		"category_code": "oficina", "title": "Espacio sintético Phase 0 editado",
		"area_m2": 30, "capacity": 5, "usage_rules": "Sin fumar",
		"rate_unit": "hora", "base_price_clp": 9000,
		"region_code": "13", "comuna_code": "13101",
	}
	call(spaceAPI, http.MethodPut, path, updated.Header().Get("ETag"), content, http.StatusUnprocessableEntity)
	delete(content, "region_code")
	delete(content, "comuna_code")
	details := call(spaceAPI, http.MethodPut, path, updated.Header().Get("ETag"), content, http.StatusOK)
	ownerLocation := call(spaceAPI, http.MethodGet, path+"/location", "", nil, http.StatusOK)
	if err := json.Unmarshal(ownerLocation.Body.Bytes(), &own); err != nil ||
		own.Latitude == nil || own.Longitude == nil || *own.Latitude != movedLat ||
		*own.Longitude != movedLng || own.Address != movedAddress ||
		own.CommuneCode != "05101" || !own.PublicConfirmed {
		t.Fatalf("editing details changed the map location: %v", err)
	}
	configured := call(spaceAPI, http.MethodPut, path+"/availability", details.Header().Get("ETag"), map[string]string{"time_zone": "America/Santiago"}, http.StatusOK)
	call(spaceAPI, http.MethodPut, path+"/publication", configured.Header().Get("ETag"), map[string]string{"state": "activa"}, http.StatusOK)

	detail := call(publicAPI, http.MethodGet, "/api/v1/public/spaces/"+item.ID, "", nil, http.StatusOK)
	var publicDetail struct {
		Data publiccatalog.Space `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &publicDetail); err != nil || publicDetail.Data.Location == nil || *publicDetail.Data.Location != expectedLocation {
		t.Fatalf("public detail location = %+v, expected %+v, error %v", publicDetail.Data.Location, expectedLocation, err)
	}
	markers := call(publicAPI, http.MethodGet, fmt.Sprintf("/api/v1/public/spaces/map?latitude=%f&longitude=%f&radius_km=2", movedLat, movedLng), "", nil, http.StatusOK)
	var mapPage publiccatalog.MapPage
	if err := json.Unmarshal(markers.Body.Bytes(), &mapPage); err != nil || len(mapPage.Items) != 1 || mapPage.Items[0].SpaceID != item.ID || mapPage.Items[0].Location != expectedLocation {
		t.Fatalf("public marker did not match stored location: %v", err)
	}
	grid := call(publicAPI, http.MethodGet, "/api/v1/public/spaces?region_code=05&comuna_code=05101", "", nil, http.StatusOK)
	var gridPage publiccatalog.Page
	if err := json.Unmarshal(grid.Body.Bytes(), &gridPage); err != nil || len(gridPage.Items) != 1 || gridPage.Items[0].SpaceID != item.ID {
		t.Fatalf("public grid did not find the edited commune: %v", err)
	}
}
