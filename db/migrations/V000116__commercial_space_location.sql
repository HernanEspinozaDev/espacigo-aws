-- Commercial coordinates are explicitly confirmed by the owner. No synthetic
-- coordinate backfill or PostGIS extension is required. Existing listings remain visible.
ALTER TABLE public.espacio
    ADD COLUMN ubicacion_latitud double precision,
    ADD COLUMN ubicacion_longitud double precision,
    ADD COLUMN ubicacion_confirmada_en timestamptz,
    ADD COLUMN ubicacion_revision bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT espacio_commercial_coordinates_ck CHECK (
        (ubicacion_latitud IS NULL AND ubicacion_longitud IS NULL) OR
        (ubicacion_latitud IS NOT NULL AND ubicacion_longitud IS NOT NULL
         AND ubicacion_latitud BETWEEN -90 AND 90
         AND ubicacion_longitud BETWEEN -180 AND 180)),
    ADD CONSTRAINT espacio_commercial_confirmation_ck CHECK (
        ubicacion_confirmada_en IS NULL OR
        (ubicacion_latitud IS NOT NULL AND ubicacion_longitud IS NOT NULL
         AND btrim(direccion) <> '' AND region_codigo IS NOT NULL AND comuna_codigo IS NOT NULL)),
    ADD CONSTRAINT espacio_commercial_revision_ck CHECK (ubicacion_revision >= 0);

-- Legacy writers cannot retain confirmation for a changed address/pin. The
-- dedicated writer supplies a fresh server confirmation timestamp atomically.
CREATE FUNCTION public.guard_commercial_space_location() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF ROW(NEW.direccion,NEW.ubicacion_catalogo_version,NEW.region_codigo,NEW.comuna_codigo,
           NEW.ubicacion_latitud,NEW.ubicacion_longitud,NEW.ubicacion_confirmada_en)
       IS DISTINCT FROM
       ROW(OLD.direccion,OLD.ubicacion_catalogo_version,OLD.region_codigo,OLD.comuna_codigo,
           OLD.ubicacion_latitud,OLD.ubicacion_longitud,OLD.ubicacion_confirmada_en) THEN
        NEW.ubicacion_revision := OLD.ubicacion_revision + 1;
        IF NEW.ubicacion_confirmada_en IS NOT DISTINCT FROM OLD.ubicacion_confirmada_en THEN
            NEW.ubicacion_confirmada_en := NULL;
        END IF;
    ELSE
        NEW.ubicacion_revision := OLD.ubicacion_revision;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER espacio_commercial_location_guard
BEFORE UPDATE OF direccion,ubicacion_catalogo_version,region_codigo,comuna_codigo,
                 ubicacion_latitud,ubicacion_longitud,ubicacion_confirmada_en,ubicacion_revision
ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_space_location();

CREATE INDEX espacio_commercial_location_idx
ON public.espacio (ubicacion_latitud,ubicacion_longitud)
WHERE estado='activa' AND ubicacion_confirmada_en IS NOT NULL;
