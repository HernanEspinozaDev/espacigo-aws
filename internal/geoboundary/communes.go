// Package geoboundary checks WGS84 pins against an explicitly supplied
// commune polygon dataset. It does not ship administrative boundaries.
package geoboundary

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
)

type point [2]float64 // longitude, latitude
type ring []point
type polygon []ring
type feature struct {
	Type       string            `json:"type"`
	Properties map[string]string `json:"properties"`
	Geometry   struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}
type indexedPolygon struct {
	rings                    polygon
	west, south, east, north float64
}

// Boundaries is immutable after loading and safe for concurrent reads.
type Boundaries struct{ byCode map[string][]indexedPolygon }

var cutCode = regexp.MustCompile(`^[0-9]{5}$`)

// Load reads either the lossless compact binary format (.bin.gz) or a local
// GeoJSON FeatureCollection. The caller must verify source rights and CUT version.
func Load(path string) (*Boundaries, error) {
	if strings.HasSuffix(path, ".bin.gz") {
		return LoadBinary(path)
	}
	return loadGeoJSON(path)
}

// loadGeoJSON expects properties.comuna_code as a five digit CUT code and
// Polygon or MultiPolygon geometry in geographic longitude/latitude degrees.
func loadGeoJSON(path string) (*Boundaries, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var decoder *json.Decoder
	if strings.HasSuffix(path, ".gz") {
		compressed, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("open commune boundaries gzip: %w", err)
		}
		defer compressed.Close()
		decoder = json.NewDecoder(compressed)
	} else {
		decoder = json.NewDecoder(file)
	}
	b := &Boundaries{byCode: make(map[string][]indexedPolygon, 345)}
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("commune boundaries require a FeatureCollection object")
	}
	var kind string
	featuresSeen, count := false, 0
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("invalid FeatureCollection key")
		}
		switch key {
		case "type":
			if err := decoder.Decode(&kind); err != nil {
				return nil, err
			}
		case "features":
			if featuresSeen {
				return nil, errors.New("duplicate features array")
			}
			featuresSeen = true
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return nil, errors.New("commune features must be an array")
			}
			for decoder.More() {
				var f feature
				if err := decoder.Decode(&f); err != nil {
					return nil, fmt.Errorf("decode boundary feature %d: %w", count, err)
				}
				if err := b.addFeature(count, f); err != nil {
					return nil, err
				}
				count++
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return nil, errors.New("unclosed features array")
			}
		default:
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return nil, err
			}
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("unclosed FeatureCollection")
	}
	if kind != "FeatureCollection" || !featuresSeen || count == 0 {
		return nil, errors.New("commune boundaries require a nonempty FeatureCollection")
	}
	return b, nil
}

// LoadBinary reads the lossless compact format emitted by
// scripts/build-commune-boundaries.py. It avoids GeoJSON decoding peaks for
// the full-resolution official shapefile.
func LoadBinary(path string) (*Boundaries, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	var header [8]byte
	if _, err := io.ReadFull(gz, header[:]); err != nil {
		return nil, err
	}
	if string(header[:]) != "EGCUT001" {
		return nil, errors.New("invalid commune boundary binary header")
	}
	var count uint32
	if err := binary.Read(gz, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	if count == 0 || count > 1000 {
		return nil, errors.New("invalid commune boundary count")
	}
	b := &Boundaries{byCode: make(map[string][]indexedPolygon, count)}
	for i := uint32(0); i < count; i++ {
		var codeBytes [5]byte
		if _, err := io.ReadFull(gz, codeBytes[:]); err != nil {
			return nil, err
		}
		code := string(codeBytes[:])
		if !cutCode.MatchString(code) {
			return nil, errors.New("invalid binary commune code")
		}
		var polygonCount uint32
		if err := binary.Read(gz, binary.LittleEndian, &polygonCount); err != nil {
			return nil, err
		}
		if polygonCount == 0 || polygonCount > 100000 {
			return nil, errors.New("invalid binary polygon count")
		}
		for p := uint32(0); p < polygonCount; p++ {
			var ringCount uint32
			if err := binary.Read(gz, binary.LittleEndian, &ringCount); err != nil {
				return nil, err
			}
			if ringCount == 0 || ringCount > 100000 {
				return nil, errors.New("invalid binary ring count")
			}
			poly := make(polygon, ringCount)
			for r := range poly {
				var pointCount uint32
				if err := binary.Read(gz, binary.LittleEndian, &pointCount); err != nil {
					return nil, err
				}
				if pointCount < 4 || pointCount > 10000000 {
					return nil, errors.New("invalid binary point count")
				}
				poly[r] = make(ring, pointCount)
				if err := binary.Read(gz, binary.LittleEndian, poly[r]); err != nil {
					return nil, err
				}
			}
			if err := b.addPolygon(code, poly); err != nil {
				return nil, err
			}
		}
	}
	var tail [1]byte
	if n, err := gz.Read(tail[:]); n != 0 || err != io.EOF {
		return nil, errors.New("trailing or invalid binary boundary data")
	}
	return b, nil
}

func (b *Boundaries) addFeature(n int, f feature) error {
	code := f.Properties["comuna_code"]
	if f.Type != "Feature" || !cutCode.MatchString(code) {
		return fmt.Errorf("boundary feature %d has invalid comuna_code", n)
	}
	var polygons []polygon
	switch f.Geometry.Type {
	case "Polygon":
		var p polygon
		if err := json.Unmarshal(f.Geometry.Coordinates, &p); err != nil {
			return fmt.Errorf("boundary %s: %w", code, err)
		}
		polygons = []polygon{p}
	case "MultiPolygon":
		if err := json.Unmarshal(f.Geometry.Coordinates, &polygons); err != nil {
			return fmt.Errorf("boundary %s: %w", code, err)
		}
	default:
		return fmt.Errorf("boundary %s requires polygon geometry", code)
	}
	for _, p := range polygons {
		if err := b.addPolygon(code, p); err != nil {
			return err
		}
	}
	return nil
}

func (b *Boundaries) addPolygon(code string, p polygon) error {
	if len(p) == 0 {
		return fmt.Errorf("boundary %s has empty polygon", code)
	}
	entry := indexedPolygon{rings: p, west: 180, south: 90, east: -180, north: -90}
	for _, r := range p {
		if len(r) < 4 || r[0] != r[len(r)-1] {
			return fmt.Errorf("boundary %s has unclosed ring", code)
		}
		for _, v := range r {
			if !valid(v) {
				return fmt.Errorf("boundary %s has invalid coordinate", code)
			}
			entry.west = math.Min(entry.west, v[0])
			entry.east = math.Max(entry.east, v[0])
			entry.south = math.Min(entry.south, v[1])
			entry.north = math.Max(entry.north, v[1])
		}
	}
	b.byCode[code] = append(b.byCode[code], entry)
	return nil
}

func valid(p point) bool {
	return !math.IsNaN(p[0]) && !math.IsNaN(p[1]) && !math.IsInf(p[0], 0) && !math.IsInf(p[1], 0) && math.Abs(p[0]) <= 180 && math.Abs(p[1]) <= 90
}

// Contains returns (inside, known). Unknown codes cannot safely prove a
// mismatch. A pin exactly on an edge is accepted because boundary precision
// cannot distinguish adjacent communes there.
func (b *Boundaries) Contains(code string, latitude, longitude float64) (bool, bool) {
	if b == nil || !valid(point{longitude, latitude}) {
		return false, false
	}
	polygons, known := b.byCode[code]
	for _, p := range polygons {
		if longitude < p.west || longitude > p.east || latitude < p.south || latitude > p.north {
			continue
		}
		outer, edge := inRing(p.rings[0], point{longitude, latitude})
		if edge {
			return true, true
		}
		if !outer {
			continue
		}
		inside := true
		for _, hole := range p.rings[1:] {
			within, onEdge := inRing(hole, point{longitude, latitude})
			if onEdge {
				return true, true
			}
			if within {
				inside = false
				break
			}
		}
		if inside {
			return true, true
		}
	}
	return false, known
}

// Find returns the CUT commune containing a WGS84 point. If a point falls on
// a shared boundary, the lexicographically first code provides a stable result.
func (b *Boundaries) Find(latitude, longitude float64) (string, bool) {
	if b == nil || !valid(point{longitude, latitude}) {
		return "", false
	}
	codes := make([]string, 0, len(b.byCode))
	for code := range b.byCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		if inside, _ := b.Contains(code, latitude, longitude); inside {
			return code, true
		}
	}
	return "", false
}

// Bounds returns the WGS84 extent as west, south, east, north. It is useful
// for fitting a selected commune in a map; an absent code returns known=false.
func (b *Boundaries) Bounds(code string) (west, south, east, north float64, known bool) {
	if b == nil {
		return 0, 0, 0, 0, false
	}
	polygons, known := b.byCode[code]
	if !known {
		return 0, 0, 0, 0, false
	}
	west, south, east, north = 180, 90, -180, -90
	for _, p := range polygons {
		west = math.Min(west, p.west)
		east = math.Max(east, p.east)
		south = math.Min(south, p.south)
		north = math.Max(north, p.north)
	}
	return west, south, east, north, true
}

func inRing(r ring, p point) (inside, edge bool) {
	for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
		a, z := r[j], r[i]
		cross := (p[0]-a[0])*(z[1]-a[1]) - (p[1]-a[1])*(z[0]-a[0])
		if math.Abs(cross) <= 1e-12 && p[0] >= math.Min(a[0], z[0]) && p[0] <= math.Max(a[0], z[0]) && p[1] >= math.Min(a[1], z[1]) && p[1] <= math.Max(a[1], z[1]) {
			return false, true
		}
		if (a[1] > p[1]) != (z[1] > p[1]) && p[0] < (z[0]-a[0])*(p[1]-a[1])/(z[1]-a[1])+a[0] {
			inside = !inside
		}
	}
	return inside, false
}
