ALTER TABLE public.espacio
    ADD COLUMN politica_cancelacion_version text
        CHECK (politica_cancelacion_version IN ('flexible_v1', 'moderada_v1', 'estricta_v1'));

ALTER TABLE public.cotizacion_arriendo_comercial
    ADD COLUMN politica_cancelacion_version text,
    ADD COLUMN politica_cancelacion_texto text,
    ADD CONSTRAINT cotizacion_comercial_cancelacion_snapshot_ck CHECK (
        (politica_cancelacion_version IS NULL AND politica_cancelacion_texto IS NULL)
        OR (
            politica_cancelacion_version IN ('flexible_v1', 'moderada_v1', 'estricta_v1')
            AND politica_cancelacion_version IS NOT NULL
            AND politica_cancelacion_texto IS NOT NULL
            AND length(btrim(politica_cancelacion_texto)) BETWEEN 1 AND 500
        )
    );
