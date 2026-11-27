-- Extend canonical privacy case lifecycle with assignment, operational deadlines,
-- optimistic concurrency, and an append-only event/note timeline.
ALTER TABLE public.solicitud_titular
    ADD COLUMN asignado_a_id uuid REFERENCES public.usuario(id) ON DELETE SET NULL,
    ADD COLUMN vence_en timestamptz,
    ADD COLUMN prorrogada_hasta timestamptz,
    ADD COLUMN prorroga_motivo text,
    ADD COLUMN version integer NOT NULL DEFAULT 1;

UPDATE public.solicitud_titular
SET vence_en = CASE
    WHEN tipo = 'bloqueo' THEN solicitada_en + interval '2 days'
    ELSE solicitada_en + interval '30 days'
END
WHERE vence_en IS NULL;

ALTER TABLE public.solicitud_titular
    DROP CONSTRAINT solicitud_titular_motivo_ck,
    ADD CONSTRAINT solicitud_titular_motivo_ck CHECK (
        motivo_resolucion_codigo IS NULL OR motivo_resolucion_codigo IN (
            'baja_local_minimizada',
            'derecho_tramitado',
            'respuesta_acceso_publicada',
            'respuesta_derecho_publicada',
            'denegacion_fundada'
        )
    ),
    DROP CONSTRAINT solicitud_titular_respuesta_ck,
    ADD CONSTRAINT solicitud_titular_respuesta_ck CHECK (
        respuesta_titular IS NULL OR (
            estado IN ('resuelta', 'denegada_fundada')
            AND resuelta_en IS NOT NULL
            AND motivo_resolucion_codigo IN (
                'respuesta_acceso_publicada',
                'respuesta_derecho_publicada',
                'denegacion_fundada',
                'derecho_tramitado',
                'baja_local_minimizada'
            )
            AND length(btrim(respuesta_titular)) BETWEEN 10 AND 4000
        )
    ),
    DROP CONSTRAINT solicitud_titular_detalle_titular_ck,
    ADD CONSTRAINT solicitud_titular_detalle_titular_ck CHECK (
        detalle_titular IS NULL OR (
            length(btrim(detalle_titular)) BETWEEN 1 AND 1000
            AND (tipo NOT IN ('rectificacion', 'oposicion', 'bloqueo') OR length(btrim(detalle_titular)) >= 10)
        )
    ),
    ADD CONSTRAINT solicitud_titular_version_ck CHECK (version >= 1),
    ADD CONSTRAINT solicitud_titular_prorroga_ck CHECK (
        (prorrogada_hasta IS NULL AND prorroga_motivo IS NULL) OR (
            prorrogada_hasta IS NOT NULL
            AND vence_en IS NOT NULL
            AND prorrogada_hasta > vence_en
            AND prorrogada_hasta <= vence_en + interval '31 days'
            AND prorroga_motivo IS NOT NULL
            AND length(btrim(prorroga_motivo)) BETWEEN 10 AND 1000
        )
    );

CREATE INDEX solicitud_titular_asignado_estado_idx
    ON public.solicitud_titular (asignado_a_id, estado, solicitada_en DESC);

CREATE INDEX solicitud_titular_vence_idx
    ON public.solicitud_titular (COALESCE(prorrogada_hasta, vence_en), estado);

CREATE TABLE public.solicitud_titular_evento (
    id uuid PRIMARY KEY,
    solicitud_id uuid NOT NULL REFERENCES public.solicitud_titular(id) ON DELETE CASCADE,
    secuencia integer NOT NULL CHECK (secuencia >= 1),
    tipo_evento text NOT NULL CHECK (tipo_evento IN (
        'recepcion', 'asignacion', 'identidad_verificada', 'en_revision',
        'aclaracion_solicitada', 'nota_interna', 'prorroga', 'resolucion', 'despacho_registrado'
    )),
    visibilidad text NOT NULL CHECK (visibilidad IN ('titular', 'interna')),
    estado_anterior text,
    estado_nuevo text,
    actor_id uuid REFERENCES public.usuario(id) ON DELETE SET NULL,
    detalle text,
    vence_en_nuevo timestamptz,
    creado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (solicitud_id, secuencia),
    CONSTRAINT solicitud_titular_evento_visibilidad_ck CHECK (
        tipo_evento <> 'nota_interna' OR visibilidad = 'interna'
    ),
    CONSTRAINT solicitud_titular_evento_detalle_ck CHECK (
        detalle IS NULL OR length(btrim(detalle)) BETWEEN 1 AND 2000
    )
);

CREATE INDEX solicitud_titular_evento_solicitud_idx
    ON public.solicitud_titular_evento (solicitud_id, secuencia ASC);

REVOKE ALL ON TABLE public.solicitud_titular_evento FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT UPDATE (asignado_a_id, vence_en, prorrogada_hasta, prorroga_motivo, version)
            ON public.solicitud_titular TO espacigo_runtime;
        REVOKE ALL ON TABLE public.solicitud_titular_evento FROM espacigo_runtime;
        GRANT SELECT, INSERT ON TABLE public.solicitud_titular_evento TO espacigo_runtime;
    END IF;
END $$;
