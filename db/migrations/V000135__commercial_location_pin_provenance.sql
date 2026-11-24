-- Optional pin provenance. Historical coordinates remain unclassified.
ALTER TABLE public.espacio
    ADD COLUMN ubicacion_pin_origen text,
    ADD COLUMN ubicacion_pin_precision text,
    ADD CONSTRAINT espacio_pin_provenance_ck CHECK (
        (ubicacion_pin_origen IS NULL AND ubicacion_pin_precision IS NULL)
        OR (ubicacion_latitud IS NOT NULL AND ubicacion_longitud IS NOT NULL AND ubicacion_pin_origen IS NOT NULL AND (
            (ubicacion_pin_origen = 'manual' AND ubicacion_pin_precision IS NULL)
            OR (ubicacion_pin_origen = 'geocoded' AND ubicacion_pin_precision IS NOT NULL AND ubicacion_pin_precision IN ('address','approximate'))
        ))
    );

-- Any older writer that changes the address, administrative selection or pin
-- invalidates its provenance. The dedicated location writer sets new metadata
-- in a second statement after the guarded location update.
CREATE FUNCTION public.clear_commercial_space_pin_provenance() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF ROW(NEW.direccion,NEW.ubicacion_catalogo_version,NEW.region_codigo,NEW.comuna_codigo,
           NEW.ubicacion_latitud,NEW.ubicacion_longitud)
       IS DISTINCT FROM
       ROW(OLD.direccion,OLD.ubicacion_catalogo_version,OLD.region_codigo,OLD.comuna_codigo,
           OLD.ubicacion_latitud,OLD.ubicacion_longitud) THEN
        NEW.ubicacion_pin_origen := NULL;
        NEW.ubicacion_pin_precision := NULL;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER espacio_commercial_pin_provenance_clear
BEFORE UPDATE OF direccion,ubicacion_catalogo_version,region_codigo,comuna_codigo,
                 ubicacion_latitud,ubicacion_longitud
ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.clear_commercial_space_pin_provenance();

REVOKE ALL ON FUNCTION public.clear_commercial_space_pin_provenance() FROM PUBLIC;
