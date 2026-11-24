package publiccatalog

import (
	"context"
	"errors"
	"testing"
)

func TestMinCapacityValidationAndCursorBinding(t *testing.T) {
	for _, value := range []int{0, -1} {
		repo := &cursorCountingRepository{memoryRepository: memoryRepository{rows: []Space{}}}
		service, err := New(repo, []byte("01234567890123456789012345678901"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.List(context.Background(), Filters{MinCapacity: &value})
		if !errors.Is(err, ErrInvalid) || repo.listCalls != 0 {
			t.Fatalf("capacity %d accepted: err=%v calls=%d", value, err, repo.listCalls)
		}
	}
	repo := &cursorCountingRepository{memoryRepository: memoryRepository{rows: []Space{
		{SpaceID: "00000000-0000-4000-8000-000000000001", TitleKey: "a"},
		{SpaceID: "00000000-0000-4000-8000-000000000002", TitleKey: "b"},
	}}}
	service, _ := New(repo, []byte("01234567890123456789012345678901"))
	capacity := 3
	first, err := service.List(context.Background(), Filters{PageSize: 1, PageSizeSet: true, MinCapacity: &capacity})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	changed := 4
	_, err = service.List(context.Background(), Filters{PageSize: 1, PageSizeSet: true, MinCapacity: &changed, Cursor: first.NextCursor})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("cursor accepted changed min_capacity: %v", err)
	}
}

func TestSpatialModesRejectAdministrativeFilters(t *testing.T) {
	service, err := New(memoryRepository{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	region := "13"
	for _, filters := range []Filters{
		{RegionCode: region, Bounds: &Bounds{West: -71, South: -34, East: -70, North: -33}},
		{CommuneCode: "13101", Proximity: &Proximity{Latitude: -33, Longitude: -70, RadiusKM: 10}},
	} {
		if _, err := service.List(context.Background(), filters); !errors.Is(err, ErrInvalid) {
			t.Fatalf("mixed spatial modes accepted: %+v err=%v", filters, err)
		}
	}
}
