package publiccatalogpg

import (
	"context"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
)

func TestConfirmedLocationProjectionAndGeoFilteringPrecedeLimit(t *testing.T) {
	const location = `{"address":"Calle 123","latitude":-33.45,"longitude":-70.66}`
	row := listCursorFixtureRow()
	row[len(row)-1] = []byte(location)
	for _, f := range []publiccatalog.Filters{
		{Bounds: &publiccatalog.Bounds{West: -71, South: -34, East: -70, North: -33}},
		{Proximity: &publiccatalog.Proximity{Latitude: -33.45, Longitude: -70.66, RadiusKM: 10}},
		{MapOnly: true},
	} {
		fixture := append([]any(nil), row...)
		if f.Proximity != nil {
			fixture = append(fixture, float64(1.25))
		}
		pool := &coverSourcePool{row: fixture}
		items, err := (&Repository{pool: pool}).List(context.Background(), f, nil, 4)
		if err != nil || len(items) != 1 || items[0].Location == nil || items[0].Location.Address != "Calle 123" {
			t.Fatalf("items=%+v err=%v", items, err)
		}
		condition := strings.Index(pool.query, " AND public_location IS NOT NULL")
		limit := strings.LastIndex(pool.query, " LIMIT $")
		if condition < 0 || condition >= limit || !strings.Contains(pool.query, "CASE WHEN e.ubicacion_confirmada_en IS NOT NULL") {
			t.Fatal("confirmation/geo filter missing before limit")
		}
		if f.Bounds != nil && !strings.Contains(pool.query, "longitude >= $") {
			t.Fatal("bbox filter missing")
		}
		if f.Proximity != nil && !strings.Contains(pool.query, "6371.0088 * 2 * asin") {
			t.Fatal("distance filter missing")
		}
	}
	pool := &coverSourcePool{row: row}
	_, err := (&Repository{pool: pool}).List(context.Background(), publiccatalog.Filters{}, nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pool.query, " AND public_location IS NOT NULL") {
		t.Fatal("legacy unconfirmed listings excluded from grid")
	}
}

func TestDetailProjectsOnlyConfirmedAddressAndPin(t *testing.T) {
	row := detailVerificationRow("11111111-1111-4111-8111-111111111111")
	row[23] = []byte(`{"address":"Calle 123","latitude":-33.45,"longitude":-70.66}`)
	item, err := (&Repository{pool: &detailVerificationPool{detailRow: row}}).Detail(context.Background(), row[0].(string))
	if err != nil || item.Location == nil || item.Location.Longitude != -70.66 {
		t.Fatalf("item=%+v err=%v", item, err)
	}
}

func TestDistanceOrderingUsesExactSQLDistanceAndStableTieBreaker(t *testing.T) {
	row := listCursorFixtureRow()
	row[len(row)-1] = []byte(`{"address":"Calle","latitude":-33.45,"longitude":-70.66}`)
	row = append(row, float64(0))
	pool := &coverSourcePool{row: row}
	distance := 0.0
	f := publiccatalog.Filters{Sort: "distance_asc", Proximity: &publiccatalog.Proximity{Latitude: -33.45, Longitude: -70.66, RadiusKM: 10}}
	items, err := (&Repository{pool: pool}).List(context.Background(), f, &publiccatalog.Cursor{LastDistanceKM: &distance, LastSpaceID: "11111111-1111-4111-8111-111111111111"}, 2)
	if err != nil || len(items) != 1 || items[0].DistanceKM == nil || *items[0].DistanceKM != 0 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if !strings.Contains(pool.query, "ORDER BY distance_km,space_id::uuid") || !strings.Contains(pool.query, "AND (distance_km,space_id::uuid) >") {
		t.Fatal("unstable distance pagination")
	}
	if strings.Contains(pool.query, "category_order,title_key COLLATE \"C\",space_id::uuid) >") {
		t.Fatal("default cursor mixed into distance cursor")
	}
}
