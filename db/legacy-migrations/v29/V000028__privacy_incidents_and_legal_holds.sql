-- Privacy data incidents (breaches), append-only incident event timeline, and legal holds.
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
        RAISE EXCEPTION 'V000028 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END;
$guard$;

CREATE TABLE public.privacidad_incidente_datos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    folio_incidente text NOT NULL UNIQUE,
    titulo text NOT NULL CHECK (char_length(btrim(titulo)) BETWEEN 5 AND 200),
    severidad text NOT NULL CHECK (severidad IN ('baja', 'media', 'alta', 'critica')),
    estado text NOT NULL DEFAULT 'abierto' CHECK (estado IN ('abierto', 'contenido', 'evaluado', 'notificado', 'cerrado')),
    categorias_datos text[] NOT NULL CHECK (cardinality(categorias_datos) >= 1),
    sistemas_afectados text[] NOT NULL CHECK (cardinality(sistemas_afectados) >= 1),
    titulares_estimados integer NOT NULL DEFAULT 0 CHECK (titulares_estimados >= 0),
    resumen_hechos text NOT NULL CHECK (char_length(btrim(resumen_hechos)) BETWEEN 10 AND 2000),
    medidas_contencion text CHECK (medidas_contencion IS NULL OR char_length(btrim(medidas_contencion)) BETWEEN 10 AND 2000),
    requiere_notificacion_agencia boolean NOT NULL DEFAULT false,
    notificado_agencia_en timestamptz,
    referencia_agencia text CHECK (referencia_agencia IS NULL OR char_length(btrim(referencia_agencia)) BETWEEN 2 AND 160),
    requiere_notificacion_titulares boolean NOT NULL DEFAULT false,
    notificado_titulares_en timestamptz,
    lider_id uuid NOT NULL REFERENCES public.usuario(id),
    suplente_id uuid REFERENCES public.usuario(id),
    detectado_en timestamptz NOT NULL,
    creado_por uuid NOT NULL REFERENCES public.usuario(id),
    creado_en timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    cerrado_en timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    CHECK ((estado = 'cerrado') = (cerrado_en IS NOT NULL)),
    CHECK (notificado_agencia_en IS NULL OR requiere_notificacion_agencia = true),
    CHECK (notificado_titulares_en IS NULL OR requiere_notificacion_titulares = true)
);

CREATE INDEX privacidad_incidente_estado_idx
    ON public.privacidad_incidente_datos (estado, severidad, creado_en DESC);

CREATE TABLE public.privacidad_incidente_evento (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incidente_id uuid NOT NULL REFERENCES public.privacidad_incidente_datos(id) ON DELETE CASCADE,
    secuencia integer NOT NULL CHECK (secuencia >= 1),
    tipo_evento text NOT NULL CHECK (tipo_evento IN (
        'apertura',
        'contencion',
        'evaluacion',
        'notificacion_agencia',
        'notificacion_titulares',
        'nota',
        'cierre'
    )),
    actor_id uuid NOT NULL REFERENCES public.usuario(id),
    detalle text NOT NULL CHECK (char_length(btrim(detalle)) BETWEEN 5 AND 2000),
    creado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (incidente_id, secuencia)
);

CREATE TABLE public.privacidad_retencion_legal (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    folio_retencion text NOT NULL UNIQUE,
    cuenta_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    solicitud_id uuid REFERENCES public.solicitud_titular(id) ON DELETE SET NULL,
    incidente_id uuid REFERENCES public.privacidad_incidente_datos(id) ON DELETE SET NULL,
    ambito text NOT NULL CHECK (ambito IN (
        'expediente_privacidad',
        'cuenta_completa',
        'evidencia_kyc',
        'contratos_y_pagos',
        'incidente_seguridad'
    )),
    autoridad text NOT NULL CHECK (char_length(btrim(autoridad)) BETWEEN 3 AND 200),
    fundamento text NOT NULL CHECK (char_length(btrim(fundamento)) BETWEEN 10 AND 1000),
    estado text NOT NULL DEFAULT 'activa' CHECK (estado IN ('activa', 'liberada')),
    revisar_en timestamptz,
    aplicada_por uuid NOT NULL REFERENCES public.usuario(id),
    aplicada_en timestamptz NOT NULL DEFAULT now(),
    liberada_por uuid REFERENCES public.usuario(id),
    liberada_en timestamptz,
    motivo_liberacion text CHECK (motivo_liberacion IS NULL OR char_length(btrim(motivo_liberacion)) BETWEEN 10 AND 1000),
    CHECK (
        (estado = 'activa' AND liberada_por IS NULL AND liberada_en IS NULL AND motivo_liberacion IS NULL)
        OR (estado = 'liberada' AND liberada_por IS NOT NULL AND liberada_en IS NOT NULL AND motivo_liberacion IS NOT NULL)
    )
);

CREATE INDEX privacidad_retencion_cuenta_idx
    ON public.privacidad_retencion_legal (cuenta_id, estado, aplicada_en DESC);

CREATE INDEX privacidad_retencion_estado_idx
    ON public.privacidad_retencion_legal (estado, aplicada_en DESC);

REVOKE ALL ON TABLE public.privacidad_incidente_datos FROM PUBLIC;
REVOKE ALL ON TABLE public.privacidad_incidente_evento FROM PUBLIC;
REVOKE ALL ON TABLE public.privacidad_retencion_legal FROM PUBLIC;

DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        REVOKE ALL ON TABLE public.privacidad_incidente_datos FROM espacigo_runtime;
        REVOKE ALL ON TABLE public.privacidad_incidente_evento FROM espacigo_runtime;
        REVOKE ALL ON TABLE public.privacidad_retencion_legal FROM espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.privacidad_incidente_datos TO espacigo_runtime;
        GRANT SELECT, INSERT ON TABLE public.privacidad_incidente_evento TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE (estado, revisar_en, liberada_por, liberada_en, motivo_liberacion) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;
    END IF;
END
$runtime$;
