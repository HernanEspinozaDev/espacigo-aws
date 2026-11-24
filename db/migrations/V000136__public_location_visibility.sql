-- Adds optional public location visibility ('exact' | 'approximate') and deterministic public coordinates.
ALTER TABLE public.espacio
    ADD COLUMN ubicacion_visibilidad text NOT NULL DEFAULT 'exact',
    ADD COLUMN ubicacion_publica_latitud double precision,
    ADD COLUMN ubicacion_publica_longitud double precision,
    ADD CONSTRAINT espacio_ubicacion_visibilidad_ck CHECK (
        ubicacion_visibilidad IN ('exact', 'approximate')
    ),
    ADD CONSTRAINT espacio_ubicacion_publica_coords_ck CHECK (
        (ubicacion_publica_latitud IS NULL AND ubicacion_publica_longitud IS NULL)
        OR (
            ubicacion_publica_latitud IS NOT NULL
            AND ubicacion_publica_longitud IS NOT NULL
            AND ubicacion_publica_latitud BETWEEN -90 AND 90
            AND ubicacion_publica_longitud BETWEEN -180 AND 180
        )
    );
