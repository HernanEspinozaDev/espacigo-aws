package spaceshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

type locationMemoryRepo struct {
	*memoryRepo
	location spaces.CommercialLocation
	writes   int
}

type testLocationBounds struct{}
type testLocationCatalog struct{}

func (testLocationCatalog) ResolveLocation(_ context.Context, region, commune string) (string, string, bool, error) {
	return "Región Metropolitana", "Santiago", region == "13" && commune == "13101", nil
}

func (testLocationBounds) Contains(code string, latitude, longitude float64) (bool, bool) {
	return longitude < -70, true
}
func (testLocationBounds) Bounds(string) (float64, float64, float64, float64, bool) {
	return -71, -34, -70, -33, true
}
func (testLocationBounds) Find(latitude, longitude float64) (string, bool) {
	if longitude < -70 {
		return "13101", true
	}
	return "", false
}

func TestCommercialLocationRejectsConfirmedOutsideCommuneButSavesDraft(t *testing.T) {
	repo := &locationMemoryRepo{memoryRepo: &memoryRepo{}, location: spaces.CommercialLocation{SpaceID: draftID, UpdatedAt: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)}}
	svc, err := spaces.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetLocationBoundaryChecker(testLocationBounds{})
	svc.SetLocationCatalog(testLocationCatalog{})
	handler := NewHandler(fakeAuth{}, svc, nil)
	path := "/api/v1/spaces/" + draftID + "/location"
	put := func(confirmed bool) *httptest.ResponseRecorder {
		body := `{"address":"Av. Prueba 123","region_code":"13","comuna_code":"13101","latitude":-33.45,"longitude":-69.66,"public_confirmed":false}`
		if confirmed {
			body = strings.Replace(body, `"public_confirmed":false`, `"public_confirmed":true`, 1)
		}
		request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer landlord")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("If-Match", draftETag(spaces.Draft{ID: draftID, UpdatedAt: repo.location.UpdatedAt}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	confirmed := put(true)
	if confirmed.Code != 422 || !strings.Contains(confirmed.Body.String(), `"field":"location.latitude","code":"no_commune_match"`) || repo.writes != 0 {
		t.Fatalf("confirmed mismatch: %d %s writes=%d", confirmed.Code, confirmed.Body.String(), repo.writes)
	}
	draft := put(false)
	if draft.Code != 422 || repo.writes != 0 {
		t.Fatalf("mismatched draft must be rejected: %d %s writes=%d", draft.Code, draft.Body.String(), repo.writes)
	}
}

func (m *locationMemoryRepo) GetOwnLocation(_ context.Context, owner, id string) (spaces.CommercialLocation, error) {
	if owner != ownerA || id != draftID {
		return spaces.CommercialLocation{}, spaces.ErrNotFound
	}
	return m.location, nil
}
func (m *locationMemoryRepo) SetOwnLocation(ctx context.Context, owner, id string, expected time.Time, in spaces.LocationInput) (spaces.CommercialLocation, error) {
	old, err := m.GetOwnLocation(ctx, owner, id)
	if err != nil {
		return old, err
	}
	if !expected.Equal(old.UpdatedAt) {
		return old, spaces.ErrPreconditionFailed
	}
	m.writes++
	m.location.LocationInput = in
	m.location.Revision++
	m.location.UpdatedAt = m.location.UpdatedAt.Add(time.Microsecond)
	if in.PublicConfirmed {
		now := m.location.UpdatedAt
		m.location.ConfirmedAt = &now
	} else {
		m.location.ConfirmedAt = nil
	}
	return m.location, nil
}
func TestCommercialLocationHTTPAuthorizationAndCAS(t *testing.T) {
	repo := &locationMemoryRepo{memoryRepo: &memoryRepo{}, location: spaces.CommercialLocation{SpaceID: draftID, UpdatedAt: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)}}
	svc, err := spaces.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetLocationBoundaryChecker(testLocationBounds{})
	svc.SetLocationCatalog(testLocationCatalog{})
	handler := NewHandler(fakeAuth{}, svc, nil)
	path := "/api/v1/spaces/" + draftID + "/location"
	body := `{"address":"Av. Prueba 123","region_code":"13","comuna_code":"13101","latitude":-33.45,"longitude":-70.66,"public_confirmed":true}`
	get := invoke(handler, http.MethodGet, path, "landlord", "")
	if get.Code != 200 || get.Header().Get("ETag") == "" {
		t.Fatalf("GET: %d %s", get.Code, get.Body.String())
	}
	etag := get.Header().Get("ETag")
	for _, tc := range []struct {
		name, token, tag string
		status           int
	}{
		{"missing auth", "", etag, 401}, {"wrong role", "tenant", etag, 403}, {"another owner", "other", etag, 404},
		{"missing precondition", "landlord", "", 428}, {"bad precondition", "landlord", "bad", 400},
		{"write", "landlord", etag, 200}, {"stale precondition", "landlord", etag, 412},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			request.Header.Set("Content-Type", "application/json")
			if tc.tag != "" {
				request.Header.Set("If-Match", tc.tag)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d want %d: %s", response.Code, tc.status, response.Body.String())
			}
			if tc.status == 200 {
				var result spaces.CommercialLocation
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.PublicConfirmed || result.Revision != 1 || response.Header().Get("ETag") == etag {
					t.Fatalf("invalid updated location: %s", response.Body.String())
				}
			}
		})
	}
	if repo.writes != 1 {
		t.Fatalf("writes %d want 1", repo.writes)
	}
}

func TestCommercialLocationRequiredHasActionableErrorCode(t *testing.T) {
	response := httptest.NewRecorder()
	serviceError(response, spaces.ErrLocationRequired)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"location_confirmation_required"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
