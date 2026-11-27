-- Rights execution log, purpose preferences (oposicion/retiro), treatment restrictions (bloqueo),
-- and expanded solicitud_titular_evento type constraint.
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
        RAISE EXCEPTION 'V000027 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END;
$guard$;

DO $$
DECLARE
    con_name text;
    tipo_attnum smallint;
BEGIN
    SELECT a.attnum INTO tipo_attnum
    FROM pg_catalog.pg_attribute a
    JOIN pg_catalog.pg_class rel ON rel.oid = a.attrelid
    JOIN pg_catalog.pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = 'public'
      AND rel.relname = 'solicitud_titular_evento'
      AND a.attname = 'tipo_evento'
      AND NOT a.attisdropped;

    FOR con_name IN
        SELECT c.conname
        FROM pg_catalog.pg_constraint c
        JOIN pg_catalog.pg_class rel ON rel.oid = c.conrelid
        JOIN pg_catalog.pg_namespace nsp ON nsp.oid = rel.relnamespace
        WHERE nsp.nspname = 'public'
          AND rel.relname = 'solicitud_titular_evento'
          AND c.contype = 'c'
          AND pg_catalog.pg_get_constraintdef(c.oid) LIKE '%tipo_evento%'
          AND c.conkey = ARRAY[tipo_attnum]
    LOOP
        EXECUTE format('ALTER TABLE public.solicitud_titular_evento DROP CONSTRAINT %I', con_name);
    END LOOP;
END $$;

ALTER TABLE public.solicitud_titular_evento
    DROP CONSTRAINT IF EXISTS solicitud_titular_evento_tipo_evento_ck,
    ADD CONSTRAINT solicitud_titular_evento_tipo_evento_ck CHECK (tipo_evento IN (
        'recepcion', 'asignacion', 'identidad_verificada', 'en_revision',
        'aclaracion_solicitada', 'nota_interna', 'prorroga', 'ejecucion_derecho',
        'resolucion', 'despacho_registrado'
    ));

CREATE TABLE public.privacidad_preferencia_finalidad (
    cuenta_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    finalidad text NOT NULL CHECK (finalidad IN (
        'comunicaciones_comerciales',
        'analitica_producto',
        'personalizacion_recomendaciones',
        'comparticion_encargados_opcionales'
    )),
    estado text NOT NULL CHECK (estado IN ('permitido', 'opuesto', 'retirado')),
    causal text CHECK (causal IS NULL OR char_length(btrim(causal)) BETWEEN 5 AND 500),
    solicitud_id uuid REFERENCES public.solicitud_titular(id) ON DELETE SET NULL,
    actualizado_por uuid NOT NULL REFERENCES public.usuario(id),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cuenta_id, finalidad)
);

CREATE TABLE public.privacidad_bloqueo_tratamiento (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cuenta_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    solicitud_id uuid NOT NULL REFERENCES public.solicitud_titular(id) ON DELETE CASCADE,
    ambito text NOT NULL CHECK (ambito IN (
        'comunicaciones_comerciales',
        'analitica_y_perfilado',
        'publicaciones_y_visibilidad',
        'exportaciones_y_terceros',
        'tratamiento_general'
    )),
    estado text NOT NULL DEFAULT 'activo' CHECK (estado IN ('activo', 'levantado')),
    motivo text NOT NULL CHECK (char_length(btrim(motivo)) BETWEEN 10 AND 1000),
    aplicado_por uuid NOT NULL REFERENCES public.usuario(id),
    aplicado_en timestamptz NOT NULL DEFAULT now(),
    revisar_en timestamptz,
    levantado_por uuid REFERENCES public.usuario(id),
    levantado_en timestamptz,
    motivo_levantamiento text CHECK (motivo_levantamiento IS NULL OR char_length(btrim(motivo_levantamiento)) BETWEEN 10 AND 1000),
    CHECK (
        (estado = 'activo' AND levantado_por IS NULL AND levantado_en IS NULL AND motivo_levantamiento IS NULL)
        OR (estado = 'levantado' AND levantado_por IS NOT NULL AND levantado_en IS NOT NULL AND motivo_levantamiento IS NOT NULL)
    )
);

CREATE UNIQUE INDEX privacidad_bloqueo_activo_uq
    ON public.privacidad_bloqueo_tratamiento (cuenta_id, ambito)
    WHERE estado = 'activo';

CREATE INDEX privacidad_bloqueo_cuenta_idx
    ON public.privacidad_bloqueo_tratamiento (cuenta_id, aplicado_en DESC);

CREATE INDEX privacidad_bloqueo_solicitud_idx
    ON public.privacidad_bloqueo_tratamiento (solicitud_id, aplicado_en DESC);

CREATE TABLE public.privacidad_ejecucion_derecho (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    solicitud_id uuid NOT NULL REFERENCES public.solicitud_titular(id) ON DELETE CASCADE,
    cuenta_id uuid REFERENCES public.usuario(id) ON DELETE SET NULL,
    tipo_ejecucion text NOT NULL CHECK (tipo_ejecucion IN (
        'rectificacion_perfil',
        'oposicion_finalidad',
        'bloqueo_aplicado',
        'bloqueo_levantado',
        'portabilidad_emitida',
        'transferencia_portabilidad',
        'propagacion_encargado'
    )),
    estado text NOT NULL DEFAULT 'ejecutada' CHECK (estado IN ('ejecutada', 'parcial', 'denegada')),
    ambito text NOT NULL CHECK (char_length(btrim(ambito)) BETWEEN 2 AND 120),
    resumen_operativo text NOT NULL CHECK (char_length(btrim(resumen_operativo)) BETWEEN 10 AND 1000),
    destino_referencia text CHECK (destino_referencia IS NULL OR char_length(btrim(destino_referencia)) BETWEEN 2 AND 200),
    sha256_artefacto text CHECK (sha256_artefacto IS NULL OR sha256_artefacto ~ '^[0-9a-f]{64}$'),
    ejecutado_por uuid NOT NULL REFERENCES public.usuario(id),
    ejecutado_en timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX privacidad_ejecucion_solicitud_idx
    ON public.privacidad_ejecucion_derecho (solicitud_id, ejecutado_en ASC);

REVOKE ALL ON TABLE public.privacidad_preferencia_finalidad FROM PUBLIC;
REVOKE ALL ON TABLE public.privacidad_bloqueo_tratamiento FROM PUBLIC;
REVOKE ALL ON TABLE public.privacidad_ejecucion_derecho FROM PUBLIC;

DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        REVOKE ALL ON TABLE public.privacidad_preferencia_finalidad FROM espacigo_runtime;
        REVOKE ALL ON TABLE public.privacidad_bloqueo_tratamiento FROM espacigo_runtime;
        REVOKE ALL ON TABLE public.privacidad_ejecucion_derecho FROM espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.privacidad_preferencia_finalidad TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE (estado, revisar_en, levantado_por, levantado_en, motivo_levantamiento) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;
        GRANT SELECT, INSERT ON TABLE public.privacidad_ejecucion_derecho TO espacigo_runtime;
    END IF;
END
$runtime$;
