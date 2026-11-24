#!/usr/bin/env bash
# Phase 4 benchmark: 10,000 synthetic spaces in a rolled-back transaction on local PostgreSQL.
set -euo pipefail

container="espacigo_localbackend_20261024-database-1"
database="espacigo_aws_local"

docker exec -i "$container" psql -U espacigo_admin -d "$database" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

-- Ensure one synthetic host user exists for FK integrity inside this rolled-back tx.
WITH host_user AS (
  SELECT id FROM public.usuario LIMIT 1
),
cat AS (
  SELECT codigo FROM public.categoria_espacio WHERE activa ORDER BY orden LIMIT 1
),
com AS (
  SELECT c.codigo AS comuna_codigo, c.region_codigo, c.catalogo_version
  FROM public.comuna_publica c
  JOIN public.catalogo_ubicacion_publica v ON v.version = c.catalogo_version AND v.activo
  ORDER BY c.codigo
  LIMIT 1
),
gen AS (
  INSERT INTO public.espacio (
    id, propietario_id, categoria_codigo, titulo, descripcion, superficie_m2,
    capacidad_maxima, reglas_uso, modalidad_tarifa, precio_base_clp, direccion, ubicacion_catalogo_version, region_codigo, comuna_codigo, estado,
    zona_horaria, ubicacion_latitud, ubicacion_longitud, ubicacion_confirmada_en,
    ubicacion_visibilidad
  )
  SELECT
    ('00000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
    (SELECT id FROM host_user),
    (SELECT codigo FROM cat),
    'Espacio Benchmark ' || i,
    'Descripción sintética de benchmark fase 4',
    25 + (i % 150),
    2 + (i % 30),
    'Uso comercial estándar',
    'hora',
    15000 + (i * 100)::bigint,
    'Av. Providencia ' || (100 + i),
    (SELECT catalogo_version FROM com),
    (SELECT region_codigo FROM com),
    (SELECT comuna_codigo FROM com),
    'activa',
    'America/Santiago',
    -33.55 + ((i % 200) * 0.001),
    -70.75 + (((i / 200)::int % 200) * 0.001),
    now(),
    CASE WHEN i % 5 = 0 THEN 'approximate' ELSE 'exact' END
  FROM generate_series(1, 10000) AS s(i)
  RETURNING id
)
INSERT INTO public.tarifa_espacio (espacio_id, version, modalidad, precio_base_clp, moneda)
SELECT id, 1, 'hora', 15000 + ((row_number() OVER ()) * 100)::bigint, 'CLP'
FROM gen;

CREATE TEMP TABLE bench_times (scenario text, ms double precision);

DO $$
DECLARE
  i int;
  plan jsonb;
  exec_ms double precision;
BEGIN
  FOR i IN 1..20 LOOP
    -- 1. bbox + capacity + price (map 250)
    EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
    SELECT e.id, e.titulo, t.precio_base_clp, e.ubicacion_latitud, e.ubicacion_longitud
    FROM public.espacio e
    JOIN public.tarifa_espacio t ON t.espacio_id = e.id
    WHERE e.estado = 'activa'
      AND e.ubicacion_confirmada_en IS NOT NULL
      AND e.capacidad_maxima >= 5
      AND t.precio_base_clp <= 50000
      AND e.ubicacion_longitud BETWEEN -70.70 AND -70.60
      AND e.ubicacion_latitud BETWEEN -33.50 AND -33.40
    ORDER BY e.id
    LIMIT 250
    INTO plan;
    exec_ms := (plan->0->>'Execution Time')::double precision;
    INSERT INTO bench_times VALUES ('bbox_map_250', exec_ms);

    -- 2. Haversine 10km radius + capacity
    EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
    SELECT e.id,
      6371.0088 * 2 * asin(sqrt(LEAST(1.0, GREATEST(0.0,
        power(sin(radians(e.ubicacion_latitud - (-33.45))/2), 2) +
        cos(radians(e.ubicacion_latitud)) * cos(radians(-33.45)) *
        power(sin(radians(e.ubicacion_longitud - (-70.66))/2), 2)
      )))) AS distance_km
    FROM public.espacio e
    JOIN public.tarifa_espacio t ON t.espacio_id = e.id
    WHERE e.estado = 'activa'
      AND e.ubicacion_confirmada_en IS NOT NULL
      AND e.capacidad_maxima >= 5
      AND 6371.0088 * 2 * asin(sqrt(LEAST(1.0, GREATEST(0.0,
        power(sin(radians(e.ubicacion_latitud - (-33.45))/2), 2) +
        cos(radians(e.ubicacion_latitud)) * cos(radians(-33.45)) *
        power(sin(radians(e.ubicacion_longitud - (-70.66))/2), 2)
      )))) <= 10.0
    ORDER BY distance_km, e.id
    LIMIT 20
    INTO plan;
    exec_ms := (plan->0->>'Execution Time')::double precision;
    INSERT INTO bench_times VALUES ('haversine_10km', exec_ms);
  END LOOP;
END $$;

SELECT
  scenario,
  round(percentile_cont(0.50) WITHIN GROUP (ORDER BY ms)::numeric, 2) AS p50_ms,
  round(percentile_cont(0.95) WITHIN GROUP (ORDER BY ms)::numeric, 2) AS p95_ms,
  round(max(ms)::numeric, 2) AS max_ms
FROM bench_times
GROUP BY scenario
ORDER BY scenario;

ROLLBACK;
SQL
