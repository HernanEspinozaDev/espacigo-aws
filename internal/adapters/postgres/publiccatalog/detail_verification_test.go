package publiccatalogpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
	"github.com/jackc/pgx/v5"
)

type detailVerificationPool struct {
	detailRow       []any
	detailErr       error
	failQueryPrefix string
	rowsErrPrefix   string
	rowsErr         error
	rowsScanErr     error
	attributes      []any
}

func (p *detailVerificationPool) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	if p.failQueryPrefix != "" && strings.HasPrefix(strings.TrimSpace(query), p.failQueryPrefix) {
		return nil, fmt.Errorf("synthetic private query failure")
	}
	switch {
	case strings.HasPrefix(strings.TrimSpace(query), "SELECT f.codigo, a.value->>'label'"):
		return p.rows(query, p.attributes), nil
	case strings.HasPrefix(strings.TrimSpace(query), "SELECT f.codigo,f.etiqueta"):
		return p.rows(query, []any{"wifi", "Wi-Fi", true}), nil
	case strings.HasPrefix(strings.TrimSpace(query), "SELECT image_id,is_cover,source FROM ("):
		return p.rows(query, []any{"22222222-2222-4222-8222-222222222222", true, "media"}), nil
	default:
		return nil, fmt.Errorf("unexpected detail query: %s", query)
	}
}

func (p *detailVerificationPool) rows(query string, row []any) pgx.Rows {
	var err, scanErr error
	if p.rowsErrPrefix != "" && strings.HasPrefix(strings.TrimSpace(query), p.rowsErrPrefix) {
		err = p.rowsErr
		scanErr = p.rowsScanErr
	}
	return &detailVerificationRows{row: row, err: err, scanErr: scanErr}
}

func (p *detailVerificationPool) QueryRow(context.Context, string, ...any) pgx.Row {
	return detailVerificationQueryRow{row: p.detailRow, err: p.detailErr}
}

type detailVerificationQueryRow struct {
	row []any
	err error
}

func (r detailVerificationQueryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanDetailVerificationFixture(r.row, dest...)
}

type detailVerificationRows struct {
	pgx.Rows
	row      []any
	err      error
	scanErr  error
	consumed bool
}

func (r *detailVerificationRows) Close()     {}
func (r *detailVerificationRows) Err() error { return r.err }
func (r *detailVerificationRows) Next() bool {
	if r.consumed {
		return false
	}
	r.consumed = true
	return true
}
func (r *detailVerificationRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return scanDetailVerificationFixture(r.row, dest...)
}

func scanDetailVerificationFixture(row []any, dest ...any) error {
	if len(row) != len(dest) {
		return fmt.Errorf("fixture columns=%d scan destinations=%d", len(row), len(dest))
	}
	for i, destination := range dest {
		value := row[i]
		switch out := destination.(type) {
		case *string:
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("column %d expected string, got %T", i, value)
			}
			*out = v
		case **string:
			if value == nil {
				*out = nil
				continue
			}
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("column %d expected nullable string, got %T", i, value)
			}
			copy := v
			*out = &copy
		case *int:
			v, ok := value.(int)
			if !ok {
				return fmt.Errorf("column %d expected int, got %T", i, value)
			}
			*out = v
		case *int64:
			v, ok := value.(int64)
			if !ok {
				return fmt.Errorf("column %d expected int64, got %T", i, value)
			}
			*out = v
		case *float64:
			v, ok := value.(float64)
			if !ok {
				return fmt.Errorf("column %d expected float64, got %T", i, value)
			}
			*out = v
		case *sql.NullString:
			if value == nil {
				*out = sql.NullString{}
				continue
			}
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("column %d expected nullable string, got %T", i, value)
			}
			*out = sql.NullString{String: v, Valid: true}
		case *sql.NullInt16:
			if value == nil {
				*out = sql.NullInt16{}
				continue
			}
			switch v := value.(type) {
			case int16:
				*out = sql.NullInt16{Int16: v, Valid: true}
			case int:
				*out = sql.NullInt16{Int16: int16(v), Valid: true}
			default:
				return fmt.Errorf("column %d expected nullable int16, got %T", i, value)
			}
		case *time.Time:
			v, ok := value.(time.Time)
			if !ok {
				return fmt.Errorf("column %d expected time.Time, got %T", i, value)
			}
			*out = v
		case *bool:
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("column %d expected bool, got %T", i, value)
			}
			*out = v
		case *[]byte:
			if value == nil {
				*out = nil
				continue
			}
			v, ok := value.([]byte)
			if !ok {
				return fmt.Errorf("column %d expected byte slice, got %T", i, value)
			}
			*out = append([]byte(nil), v...)
		case *json.RawMessage:
			v, ok := value.([]byte)
			if !ok {
				return fmt.Errorf("column %d expected JSON bytes, got %T", i, value)
			}
			*out = append(json.RawMessage(nil), v...)
		default:
			return fmt.Errorf("column %d unsupported destination %T", i, destination)
		}
	}
	return nil
}

func TestDetailVerificationFlagsReachPublicHTTPResponse(t *testing.T) {
	const spaceID = "11111111-1111-4111-8111-111111111111"
	pool := &detailVerificationPool{detailRow: detailVerificationRow(spaceID), attributes: []any{"capacity", "Capacity", "integer", 1, "people", []byte(`8`), []byte(`[]`)}}
	repo := &Repository{pool: pool}
	service, err := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	publiccataloghttp.New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+spaceID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Verification map[string]json.RawMessage      `json:"verification"`
			Attributes   []publiccatalog.PublicAttribute `json:"attributes"`
			Images       []publiccatalog.PublicImageRef  `json:"images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode detail response: %v body=%s", err, response.Body.String())
	}
	if len(envelope.Data.Verification) != 2 || string(envelope.Data.Verification["identity_verified"]) != "true" || string(envelope.Data.Verification["space_verified"]) != "false" {
		t.Fatalf("verification=%v, want only identity_verified=true and space_verified=false", envelope.Data.Verification)
	}
	if len(envelope.Data.Attributes) != 1 || envelope.Data.Attributes[0].Code != "capacity" || string(envelope.Data.Attributes[0].Value) != "8" {
		t.Fatalf("attributes=%+v, want the published capacity value", envelope.Data.Attributes)
	}
	if len(envelope.Data.Images) != 1 || envelope.Data.Images[0].ImageID != "22222222-2222-4222-8222-222222222222" || !envelope.Data.Images[0].IsCover || envelope.Data.Images[0].Source != "media" {
		t.Fatalf("images=%+v, want the media cover", envelope.Data.Images)
	}
}

func TestDetailQueryFailureDoesNotLeakPartialPublicDTO(t *testing.T) {
	const spaceID = "11111111-1111-4111-8111-111111111111"
	cases := []struct {
		name       string
		prefix     string
		rowsPrefix string
		queryErr   error
		rowsErr    error
		scanErr    error
	}{
		{name: "space query", queryErr: errors.New("synthetic private query failure")},
		{name: "features query", prefix: "SELECT f.codigo,f.etiqueta"},
		{name: "images query", prefix: "SELECT image_id,is_cover,source FROM ("},
		{name: "features stream", rowsPrefix: "SELECT f.codigo,f.etiqueta", rowsErr: errors.New("synthetic private row stream failure")},
		{name: "attributes query", prefix: "SELECT f.codigo, a.value->>'label'"},
		{name: "attributes stream", rowsPrefix: "SELECT f.codigo, a.value->>'label'", rowsErr: errors.New("synthetic private row stream failure")},
		{name: "attributes scan", rowsPrefix: "SELECT f.codigo, a.value->>'label'", scanErr: errors.New("synthetic private scan failure")},
		{name: "images stream", rowsPrefix: "SELECT image_id,is_cover,source FROM (", rowsErr: errors.New("synthetic private row stream failure")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &detailVerificationPool{detailRow: detailVerificationRow(spaceID), attributes: []any{"capacity", "Capacity", "integer", 1, "people", []byte(`8`), []byte(`[]`)}, detailErr: tc.queryErr, failQueryPrefix: tc.prefix, rowsErrPrefix: tc.rowsPrefix, rowsErr: tc.rowsErr, rowsScanErr: tc.scanErr}
			service, err := publiccatalog.New(&Repository{pool: pool}, []byte("01234567890123456789012345678901"))
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			publiccataloghttp.New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+spaceID, nil))
			body := response.Body.String()
			if response.Code != http.StatusInternalServerError || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(body, `"data"`) {
				t.Fatalf("status=%d cache=%q partial_data=%v body=%s", response.Code, response.Header().Get("Cache-Control"), strings.Contains(body, `"data"`), body)
			}
			for _, private := range []string{"identity_verified", "space_verified", "Reglas sintéticas", "synthetic private query failure", "synthetic private row stream failure"} {
				if strings.Contains(body, private) {
					t.Errorf("error response leaked %q: %s", private, body)
				}
			}
		})
	}
}

func TestDetailMissingRowMapsToPublicNotFound(t *testing.T) {
	const spaceID = "11111111-1111-4111-8111-111111111111"
	pool := &detailVerificationPool{detailErr: pgx.ErrNoRows}
	service, err := publiccatalog.New(&Repository{pool: pool}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	publiccataloghttp.New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+spaceID, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want public not-found", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("not-found response includes data: %s", response.Body.String())
	}
}

func detailVerificationRow(spaceID string) []any {
	return []any{
		spaceID, "oficina", "Oficina", "Sala sintética", "Descripción sintética", float64(20), 8, "dia", int64(5000), "CLP", 1, "sala",
		"Reglas sintéticas", "13", "Metropolitana", "13101", "Santiago", true, false, nil, nil, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), int64(3), nil, nil, nil,
	}
}
