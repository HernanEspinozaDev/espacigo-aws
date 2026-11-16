package publiccatalogpg

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
)

func (r *Repository) PublicAttributes(ctx context.Context, id string) ([]publiccatalog.PublicAttribute, error) {
	return r.publicAttributes(ctx, id)
}

// publicAttributes reads only profile fields explicitly exposed by the V57 policy.
func (r *Repository) publicAttributes(ctx context.Context, id string) ([]publiccatalog.PublicAttribute, error) {
	rows, err := r.pool.Query(ctx, `SELECT f.codigo, a.value->>'label', f.tipo, (a.value->>'order')::integer,
       COALESCE(a.value->>'unit',''), ec.valores->f.codigo, a.value->'options'
FROM public.espacio_caracteristicas ec
JOIN public.espacio e ON e.id=ec.espacio_id AND e.categoria_codigo=ec.categoria_codigo
JOIN public.atributo_filtro_publico f ON f.categoria_codigo=ec.categoria_codigo AND f.perfil_version=ec.perfil_version
JOIN public.categoria_perfil_atributos p ON p.categoria_codigo=f.categoria_codigo AND p.version=f.perfil_version
CROSS JOIN LATERAL jsonb_array_elements(p.perfil->'attributes') a(value)
WHERE ec.espacio_id=$1 AND a.value->>'code'=f.codigo AND a.value->>'type'=f.tipo
  AND f.tipo IN ('boolean','integer','number','enum','enum_list')
  AND ec.valores ? f.codigo AND ec.valores->f.codigo <> 'null'::jsonb
ORDER BY (a.value->>'order')::integer, f.codigo`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]publiccatalog.PublicAttribute, 0, 8)
	for rows.Next() {
		var v publiccatalog.PublicAttribute
		var raw, opts []byte
		if err := rows.Scan(&v.Code, &v.Label, &v.Type, &v.Order, &v.Unit, &raw, &opts); err != nil {
			return nil, err
		}
		if !json.Valid(raw) {
			continue
		}
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&value) != nil {
			continue
		}
		if !validPublicAttributeValue(v.Type, value, opts) {
			continue
		}
		v.Value = append(json.RawMessage(nil), raw...)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func validPublicAttributeValue(kind string, v any, optionsJSON []byte) bool {
	switch kind {
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		_, err := n.Int64()
		return err == nil
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "enum", "enum_list":
		var options []string
		if json.Unmarshal(optionsJSON, &options) != nil || len(options) == 0 {
			return false
		}
		allowed := map[string]bool{}
		for _, s := range options {
			allowed[s] = true
		}
		if kind == "enum" {
			s, ok := v.(string)
			return ok && allowed[s]
		}
		a, ok := v.([]any)
		if !ok {
			return false
		}
		for _, x := range a {
			s, ok := x.(string)
			if !ok || !allowed[s] {
				return false
			}
		}
		return true
	}
	return false
}
