-- Keep the holder's own intake context with the case for authorized review.
ALTER TABLE public.solicitud_titular
    ADD COLUMN detalle_titular text,
    ADD CONSTRAINT solicitud_titular_detalle_titular_ck CHECK (
        detalle_titular IS NULL OR (
            length(btrim(detalle_titular)) BETWEEN 1 AND 1000
            AND (tipo NOT IN ('rectificacion', 'oposicion') OR length(btrim(detalle_titular)) >= 10)
        )
    );
