package spaceshttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

func TestCommuneBoundsAvailableBeforeDraftCreation(t *testing.T) {
	service, err := spaces.NewService(&memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(fakeAuth{}, service, nil)
	const path = "/api/v1/spaces/location/communes/13101"

	if got := invoke(handler, http.MethodGet, path, "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("expected session required, got %d", got.Code)
	}
	if got := invoke(handler, http.MethodGet, path, "tenant", ""); got.Code != http.StatusNotFound {
		t.Fatalf("expected no configured dataset, got %d", got.Code)
	}

	service.SetLocationBoundaryChecker(testLocationBounds{})
	got := invoke(handler, http.MethodGet, path, "tenant", "")
	var bounds communeBounds
	if got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &bounds) != nil ||
		bounds != (communeBounds{West: -71, South: -34, East: -70, North: -33}) {
		t.Fatalf("unexpected viewport response: %d %s", got.Code, got.Body.String())
	}
}
