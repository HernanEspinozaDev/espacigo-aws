package publiccatalog

import (
	"context"
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

func TestPublicProjectionRemovesContactAndOnlyKeepsFirstName(t *testing.T) {
	name := "María José Pérez"
	source := Space{Title: "Sala", Description: "Buena sala. correo@ejemplo.cl +56 9 1234 5678 RUT 12.345.678-9 https://wa.me/56912345678", UsageRules: "Preguntar a @privado", Host: &PublicHost{Name: &name}}
	got := publicProjection(source)
	if got.Host.Name == nil || *got.Host.Name != "María" {
		t.Fatalf("name=%v", got.Host.Name)
	}
	for _, private := range []string{"correo@", "1234", "12.345.678-9", "wa.me", "@privado"} {
		if strings.Contains(got.Description+got.UsageRules, private) {
			t.Fatalf("exposed %s: %+v", private, got)
		}
	}
	if *source.Host.Name != name || !strings.Contains(source.Description, "correo@") {
		t.Fatal("changed private input")
	}
	for _, bad := range []string{"test@example.cl", "+56912345678", "@usuario", "https://contacto.cl"} {
		if got := publicProjection(Space{Host: &PublicHost{Name: &bad}}); got.Host.Name != nil {
			t.Fatalf("unsafe name %q", bad)
		}
	}
}

func TestGeoValidationRejectsPartialInvalidAndMixedSearch(t *testing.T) {
	for _, f := range []Filters{
		{Bounds: &Bounds{West: -71, South: -34, East: -70, North: -35}},
		{Bounds: &Bounds{West: 170, South: -34, East: -170, North: -33}},
		{Bounds: &Bounds{West: -71, South: -34, East: math.NaN(), North: -33}},
		{Proximity: &Proximity{Latitude: 91, RadiusKM: 10}},
		{Proximity: &Proximity{Latitude: -33, Longitude: -70, RadiusKM: 0}},
		{Proximity: &Proximity{Latitude: -33, Longitude: -70, RadiusKM: 201}},
		{Bounds: &Bounds{West: -71, South: -34, East: -70, North: -33}, Proximity: &Proximity{RadiusKM: 10}},
	} {
		if validateGeo(f) != ErrInvalid {
			t.Fatalf("accepted %+v", f)
		}
	}
}

func TestMapAndGeographicCursorsAreBoundToAreaAndMode(t *testing.T) {
	rows := []Space{{SpaceID: "11111111-1111-4111-8111-111111111111", TitleKey: "a"}, {SpaceID: "22222222-2222-4222-8222-222222222222", TitleKey: "b"}}
	s, err := New(memoryRepository{rows}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	f := Filters{PageSize: 1, Bounds: &Bounds{West: -71, South: -34, East: -70, North: -33}}
	page, err := s.Map(context.Background(), f)
	if err != nil || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	f.Cursor = page.NextCursor
	if _, err = s.List(context.Background(), f); err != ErrInvalid {
		t.Fatalf("map cursor used for grid: %v", err)
	}
	if _, err = s.Map(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	f.Bounds.East = -69
	if _, err = s.Map(context.Background(), f); err != ErrInvalid {
		t.Fatalf("area changed: %v", err)
	}
}

func TestCursorDoesNotExposeRedactedTitleButRetainsSortKey(t *testing.T) {
	s, _ := New(memoryRepository{}, []byte("01234567890123456789012345678901"))
	want := Cursor{Version: 3, LastTitleKey: "reserva test@example.cl +56 9 1234 5678", LastSpaceID: "11111111-1111-4111-8111-111111111111"}
	token, err := s.encodeCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "example.cl") || strings.Contains(string(raw), "1234") {
		t.Fatalf("cursor leaks private sort key: %s", raw)
	}
	got, err := s.decodeCursor(token)
	if err != nil || got != want {
		t.Fatalf("round trip got=%+v err=%v", got, err)
	}
}
