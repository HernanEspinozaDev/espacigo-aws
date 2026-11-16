package publiccatalogpg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"testing"
)

type publicAttributesPool struct {
	queryErr error
	rows     pgx.Rows
}

func (p *publicAttributesPool) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	if len(args) != 1 || args[0] != "space-id" {
		return nil, fmt.Errorf("unexpected args")
	}
	if p.queryErr != nil {
		return nil, p.queryErr
	}
	return p.rows, nil
}
func (*publicAttributesPool) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

type publicAttributesRows struct {
	pgx.Rows
	data               [][]any
	i                  int
	scanErr, streamErr error
}

func (r *publicAttributesRows) Close()     {}
func (r *publicAttributesRows) Err() error { return r.streamErr }
func (r *publicAttributesRows) Next() bool {
	if r.i >= len(r.data) {
		return false
	}
	r.i++
	return true
}
func (r *publicAttributesRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.data[r.i-1]
	if len(row) != len(dest) {
		return fmt.Errorf("width")
	}
	for i, v := range row {
		switch d := dest[i].(type) {
		case *string:
			*d = v.(string)
		case *int:
			*d = v.(int)
		case *[]byte:
			*d = append([]byte(nil), v.([]byte)...)
		case *json.RawMessage:
			*d = append([]byte(nil), v.([]byte)...)
		default:
			return fmt.Errorf("dest %T", d)
		}
	}
	return nil
}
func fixtureRows() *publicAttributesRows {
	return &publicAttributesRows{data: [][]any{{"enabled", "Enabled", "boolean", 1, "", []byte(`false`), []byte(`[]`)}, {"count", "Count", "integer", 2, "", []byte(`0`), []byte(`[]`)}, {"size", "Size", "number", 3, "m", []byte(`12.3400`), []byte(`[]`)}, {"kind", "Kind", "enum", 4, "", []byte(`"wood"`), []byte(`["wood","gas"]`)}, {"features", "Features", "enum_list", 5, "", []byte(`["wifi"]`), []byte(`["wifi","screen"]`)}}}
}
func TestPublicAttributesProjectsTypedPublishedValues(t *testing.T) {
	rows := fixtureRows()
	got, err := (&Repository{pool: &publicAttributesPool{rows: rows}}).PublicAttributes(context.Background(), "space-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d", len(got))
	}
	for i := range got {
		if string(got[i].Value) != string(rows.data[i][5].([]byte)) {
			t.Fatalf("value %s", got[i].Value)
		}
	}
	var n json.Number
	if err := json.Unmarshal(got[2].Value, &n); err != nil || n.String() != "12.3400" {
		t.Fatalf("precision lost: %q %v", n, err)
	}
}
func TestPublicAttributesQueryAndStreamErrors(t *testing.T) {
	boom := fmt.Errorf("boom")
	cases := []struct {
		name string
		pool *publicAttributesPool
	}{
		{name: "query", pool: &publicAttributesPool{queryErr: boom}},
		{name: "stream", pool: &publicAttributesPool{rows: &publicAttributesRows{data: [][]any{{"x", "X", "boolean", 1, "", []byte(`true`), []byte(`[]`)}}, streamErr: boom}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Repository{pool: tc.pool}).PublicAttributes(context.Background(), "space-id")
			if !errors.Is(err, boom) || got != nil {
				t.Fatalf("partial/error contract: %#v %v", got, err)
			}
		})
	}
}
func TestPublicAttributesScanErrorHasNoPartialResult(t *testing.T) {
	rows := fixtureRows()
	boom := fmt.Errorf("scan")
	rows.scanErr = boom
	got, err := (&Repository{pool: &publicAttributesPool{rows: rows}}).PublicAttributes(context.Background(), "space-id")
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("got %#v err %v, want scan error and no partial result", got, err)
	}
}
