package geoboundary

import (
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

const fixture = `{"type":"FeatureCollection","features":[
{"type":"Feature","properties":{"comuna_code":"13101"},"geometry":{"type":"Polygon","coordinates":[[[-71,-34],[-70,-34],[-70,-33],[-71,-33],[-71,-34]],[[-70.8,-33.8],[-70.7,-33.8],[-70.7,-33.7],[-70.8,-33.7],[-70.8,-33.8]]]}},
{"type":"Feature","properties":{"comuna_code":"13102"},"geometry":{"type":"MultiPolygon","coordinates":[[[[-72,-34],[-71,-34],[-71,-33],[-72,-33],[-72,-34]]],[[[-70,-34],[-69,-34],[-69,-33],[-70,-33],[-70,-34]]]]}}
]}`

func writeFixture(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "communes.geojson")
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestContainsAndBounds(t *testing.T) {
	b, err := Load(writeFixture(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		code          string
		lat, lon      float64
		inside, known bool
	}{
		{"13101", -33.5, -70.5, true, true},
		{"13101", -33.75, -70.75, false, true}, // hole
		{"13101", -33.5, -71, true, true},      // shared edge accepted
		{"13101", -33.5, -71.5, false, true},
		{"13102", -33.5, -69.5, true, true}, // second island
		{"13103", -33.5, -70.5, false, false},
	} {
		inside, known := b.Contains(tc.code, tc.lat, tc.lon)
		if inside != tc.inside || known != tc.known {
			t.Errorf("%s %g %g: got (%v,%v)", tc.code, tc.lat, tc.lon, inside, known)
		}
	}
	w, s, e, n, known := b.Bounds("13102")
	if !known || w != -72 || s != -34 || e != -69 || n != -33 {
		t.Fatalf("unexpected bounds: %g %g %g %g %v", w, s, e, n, known)
	}
}

func TestFindReturnsOfficialCodeForPoint(t *testing.T) {
	b, err := Load(writeFixture(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	if code, ok := b.Find(-33.5, -70.5); !ok || code != "13101" {
		t.Fatalf("Find=%q,%v", code, ok)
	}
	if _, ok := b.Find(-30, -70); ok {
		t.Fatal("found code outside all polygons")
	}
}

func TestRejectMalformedDataset(t *testing.T) {
	for _, tc := range []string{
		`{"type":"FeatureCollection","features":[]}`,
		`{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"comuna_code":"13101"},"geometry":{"type":"Polygon","coordinates":[[[-71,-34],[-70,-34],[-70,-33]]]}}]}`,
		`{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"comuna_code":"x"},"geometry":{"type":"Polygon","coordinates":[[[-71,-34],[-70,-34],[-70,-33],[-71,-34]]]}}]}`,
	} {
		if _, err := Load(writeFixture(t, tc)); err == nil {
			t.Errorf("accepted malformed dataset %s", tc)
		}
	}
}

func TestLoadBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "communes.bin.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	g := gzip.NewWriter(f)
	_, _ = g.Write([]byte("EGCUT001"))
	_ = binary.Write(g, binary.LittleEndian, uint32(1))
	_, _ = g.Write([]byte("13101"))
	for _, n := range []uint32{1, 1, 5} {
		_ = binary.Write(g, binary.LittleEndian, n)
	}
	for _, p := range []point{{-71, -34}, {-70, -34}, {-70, -33}, {-71, -33}, {-71, -34}} {
		_ = binary.Write(g, binary.LittleEndian, p)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if inside, known := b.Contains("13101", -33.5, -70.5); !inside || !known {
		t.Fatalf("binary lookup: %v %v", inside, known)
	}
}
