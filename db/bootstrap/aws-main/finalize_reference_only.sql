-- Run once, after the approved V1..V100 bootstrap and ledger verification.
-- Use psql -X -v ON_ERROR_STOP=1 against the new espacigo_prod instance.
BEGIN;
SET LOCAL row_security = off;
SET LOCAL search_path = pg_catalog, public;
SELECT pg_advisory_xact_lock(1163087943, 1296648018); -- ESPG / MIGR

DO $bootstrap$
DECLARE
    relation record;
    has_rows boolean;
BEGIN
    IF current_database() <> 'espacigo_prod' THEN
        RAISE EXCEPTION 'Reference-only bootstrap requires espacigo_prod';
    END IF;
    IF (SELECT count(*) = 100 AND min(version) = 1 AND max(version) = 100
        FROM public.schema_migrations) IS DISTINCT FROM true THEN
        RAISE EXCEPTION 'Reference-only bootstrap requires the initial V1..V100 ledger';
    END IF;

    FOR relation IN
        SELECT n.nspname, c.relname
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname IN ('public', 'document_verification_private')
          AND c.relkind IN ('r', 'p', 'm')
          AND NOT EXISTS (
              SELECT 1 FROM pg_catalog.pg_depend d
              WHERE d.classid = 'pg_catalog.pg_class'::regclass
                AND d.objid = c.oid AND d.deptype = 'e'
          )
          AND NOT (n.nspname = 'public' AND c.relname IN (
              'schema_migrations', 'categoria_espacio', 'categoria_perfil_atributos',
              'catalogo_ubicacion_publica', 'region_publica', 'comuna_publica',
              'atributo_filtro_publico', 'version_terminos'
          ))
    LOOP
        EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I)', relation.nspname, relation.relname)
            INTO has_rows;
        IF has_rows THEN
            RAISE EXCEPTION 'Unexpected rows in %.%; stopping reference-only bootstrap',
                relation.nspname, relation.relname;
        END IF;
    END LOOP;

    -- These exact seed codes came from immutable V1 and V75.
    DELETE FROM public.version_terminos WHERE codigo IN (
        'sintetico-terminos-v0.1', 'sintetico-privacidad-v0.1',
        'sintetico-politica-arrendador-v0.1',
        'evaluacion-terminos-20261023-v1', 'evaluacion-privacidad-20261023-v1'
    );
    IF EXISTS (SELECT 1 FROM public.version_terminos) THEN
        RAISE EXCEPTION 'Unexpected terms exist; stopping reference-only bootstrap';
    END IF;
    IF (SELECT count(*) FROM public.categoria_espacio WHERE activa) <> 9
        OR (SELECT count(*) FROM public.region_publica WHERE catalogo_version = 'SUBDERE_DPA_2018') <> 16
        OR (SELECT count(*) FROM public.comuna_publica WHERE catalogo_version = 'SUBDERE_DPA_2018') <> 345
        OR NOT EXISTS (
            SELECT 1 FROM public.catalogo_ubicacion_publica
            WHERE version = 'SUBDERE_DPA_2018' AND activo
        )
        OR NOT EXISTS (SELECT 1 FROM public.categoria_perfil_atributos)
        OR NOT EXISTS (SELECT 1 FROM public.atributo_filtro_publico)
    THEN
        RAISE EXCEPTION 'Initial reference catalog is incomplete';
    END IF;
END
$bootstrap$;

COMMIT;
