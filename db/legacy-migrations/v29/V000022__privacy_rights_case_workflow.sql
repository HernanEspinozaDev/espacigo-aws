-- Preserve existing titular folios and extend them for the shared privacy queue.
ALTER TABLE public.solicitud_titular
    DROP CONSTRAINT solicitud_titular_tipo_ck,
    ADD CONSTRAINT solicitud_titular_tipo_ck
        CHECK (tipo IN ('acceso', 'rectificacion', 'supresion', 'oposicion', 'portabilidad', 'bloqueo')),
    DROP CONSTRAINT solicitud_titular_motivo_ck,
    ADD CONSTRAINT solicitud_titular_motivo_ck CHECK (
        motivo_resolucion_codigo IS NULL OR motivo_resolucion_codigo IN (
            'baja_local_minimizada', 'derecho_tramitado', 'respuesta_acceso_publicada'
        )
    ),
    ADD COLUMN solicitado_por uuid REFERENCES public.usuario(id) ON DELETE SET NULL,
    ADD COLUMN motivo_administrativo text,
    ADD COLUMN caso_ref_administrativo text,
    ADD COLUMN respuesta_titular text,
    ADD CONSTRAINT solicitud_titular_respuesta_ck CHECK (
        respuesta_titular IS NULL OR (
            tipo = 'acceso'
            AND estado IN ('resuelta', 'denegada_fundada')
            AND resuelta_en IS NOT NULL
            AND motivo_resolucion_codigo = 'respuesta_acceso_publicada'
            AND length(btrim(respuesta_titular)) BETWEEN 10 AND 4000
        )
    );

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT UPDATE (respuesta_titular) ON public.solicitud_titular TO espacigo_runtime;
    END IF;
END $$;
