-- Public ARCO+ intake (with anonymous tracking folio/hash) and durable privacy notification outbox.
DO $guard$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles
        WHERE rolname = session_user AND rolsuper
    ) AND (
        session_user <> 'espacigo_migrator_login'
        OR current_user <> 'espacigo_schema_owner'
        OR current_schema() <> 'public'
        OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE')
    ) THEN
        RAISE EXCEPTION 'V000026 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END;
$guard$;

ALTER TABLE public.solicitud_titular
    ALTER COLUMN usuario_id DROP NOT NULL,
    ADD COLUMN folio_publico text,
    ADD COLUMN solicitante_tipo text NOT NULL DEFAULT 'titular'
        CHECK (solicitante_tipo IN ('titular', 'representante')),
    ADD COLUMN solicitante_nombre text
        CHECK (solicitante_nombre IS NULL OR char_length(btrim(solicitante_nombre)) BETWEEN 2 AND 160),
    ADD COLUMN solicitante_correo text
        CHECK (solicitante_correo IS NULL OR char_length(btrim(solicitante_correo)) BETWEEN 5 AND 254),
    ADD COLUMN representante_nombre text
        CHECK (representante_nombre IS NULL OR char_length(btrim(representante_nombre)) BETWEEN 2 AND 160),
    ADD COLUMN representante_relacion text
        CHECK (representante_relacion IS NULL OR representante_relacion IN ('mandatario', 'representante_legal', 'tutor_nna')),
    ADD COLUMN titular_referencia text
        CHECK (titular_referencia IS NULL OR char_length(btrim(titular_referencia)) BETWEEN 2 AND 200),
    ADD COLUMN consulta_token_hash bytea
        CHECK (consulta_token_hash IS NULL OR octet_length(consulta_token_hash) = 32);

UPDATE public.solicitud_titular
SET folio_publico = 'PRIV-' || upper(substr(replace(id::text, '-', ''), 1, 10))
WHERE folio_publico IS NULL;

ALTER TABLE public.solicitud_titular
    ALTER COLUMN folio_publico SET DEFAULT ('PRIV-' || upper(substr(replace(gen_random_uuid()::text, '-', ''), 1, 10))),
    ALTER COLUMN folio_publico SET NOT NULL,
    DROP CONSTRAINT solicitud_titular_canal_ck,
    ADD CONSTRAINT solicitud_titular_canal_ck
        CHECK (canal IN ('web', 'api', 'publico', 'correo')),
    ADD CONSTRAINT solicitud_titular_identificacion_ck CHECK (
        (usuario_id IS NOT NULL)
        OR (solicitante_nombre IS NOT NULL AND solicitante_correo IS NOT NULL)
    ),
    ADD CONSTRAINT solicitud_titular_representante_ck CHECK (
        (solicitante_tipo = 'titular' AND representante_nombre IS NULL AND representante_relacion IS NULL)
        OR (solicitante_tipo = 'representante' AND representante_nombre IS NOT NULL AND representante_relacion IS NOT NULL)
    );

CREATE UNIQUE INDEX solicitud_titular_folio_publico_uq
    ON public.solicitud_titular (folio_publico);

CREATE TABLE public.privacidad_notificacion_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    solicitud_id uuid NOT NULL REFERENCES public.solicitud_titular(id) ON DELETE CASCADE,
    tipo_aviso text NOT NULL CHECK (tipo_aviso IN (
        'acuse_recepcion',
        'aclaracion_solicitada',
        'prorroga_plazo',
        'resolucion_publicada'
    )),
    clave_idempotencia text NOT NULL UNIQUE,
    destinatario_correo text NOT NULL CHECK (char_length(destinatario_correo) BETWEEN 5 AND 254),
    asunto text NOT NULL CHECK (char_length(asunto) BETWEEN 3 AND 200),
    cuerpo_texto text NOT NULL CHECK (char_length(cuerpo_texto) BETWEEN 10 AND 4000),
    estado text NOT NULL DEFAULT 'pending' CHECK (estado IN ('pending', 'processing', 'delivered', 'terminal')),
    intentos smallint NOT NULL DEFAULT 0 CHECK (intentos BETWEEN 0 AND 6),
    disponible_en timestamptz NOT NULL DEFAULT now(),
    reclamado_en timestamptz,
    entregado_en timestamptz,
    terminal_en timestamptz,
    ultimo_error_codigo text CHECK (ultimo_error_codigo IS NULL OR ultimo_error_codigo IN ('mail_delivery_failed', 'attempt_limit_reached')),
    creado_en timestamptz NOT NULL DEFAULT now(),
    CHECK ((estado = 'processing') = (reclamado_en IS NOT NULL)),
    CHECK ((estado = 'delivered') = (entregado_en IS NOT NULL)),
    CHECK ((estado = 'terminal') = (terminal_en IS NOT NULL)),
    CHECK (num_nonnulls(entregado_en, terminal_en) <= 1)
);

CREATE INDEX privacidad_notificacion_outbox_pending_idx
    ON public.privacidad_notificacion_outbox (disponible_en, creado_en, id)
    WHERE estado IN ('pending', 'processing');

CREATE INDEX privacidad_notificacion_outbox_solicitud_idx
    ON public.privacidad_notificacion_outbox (solicitud_id, creado_en DESC);

REVOKE ALL ON TABLE public.privacidad_notificacion_outbox FROM PUBLIC;
DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT UPDATE (usuario_id, folio_publico) ON public.solicitud_titular TO espacigo_runtime;
        REVOKE ALL ON TABLE public.privacidad_notificacion_outbox FROM espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE (estado, intentos, disponible_en, reclamado_en, entregado_en, terminal_en, ultimo_error_codigo) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;
    END IF;
END
$runtime$;
