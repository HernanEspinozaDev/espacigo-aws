-- Specialized administrative operations (support tickets, privacy requests) and dual-control approvals.
CREATE TABLE public.ticket_soporte_administrativo (
    id uuid PRIMARY KEY,
    cuenta_id uuid REFERENCES public.usuario(id),
    asignado_a_id uuid REFERENCES public.usuario(id),
    categoria text NOT NULL,
    prioridad text NOT NULL DEFAULT 'media' CHECK (prioridad IN ('baja', 'media', 'alta', 'critica')),
    estado text NOT NULL DEFAULT 'abierto' CHECK (estado IN ('abierto', 'en_progreso', 'resuelto', 'cerrado')),
    asunto text NOT NULL,
    caso_ref text NOT NULL DEFAULT '',
    creado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creado_en)),
    actualizado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(actualizado_en))
);

CREATE INDEX ticket_soporte_administrativo_estado_idx
    ON public.ticket_soporte_administrativo (estado, creado_en DESC, id DESC);

CREATE INDEX ticket_soporte_administrativo_cuenta_idx
    ON public.ticket_soporte_administrativo (cuenta_id, creado_en DESC)
    WHERE cuenta_id IS NOT NULL;

CREATE TABLE public.solicitud_privacidad_administrativa (
    id uuid PRIMARY KEY,
    cuenta_id uuid NOT NULL REFERENCES public.usuario(id),
    tipo_derecho text NOT NULL CHECK (tipo_derecho IN ('acceso', 'rectificacion', 'supresion', 'oposicion', 'portabilidad', 'bloqueo')),
    estado text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente', 'en_revision', 'aprobada', 'rechazada', 'completada')),
    motivo text NOT NULL,
    caso_ref text NOT NULL DEFAULT '',
    solicitado_por uuid REFERENCES public.usuario(id),
    creado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creado_en)),
    actualizado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(actualizado_en)),
    resuelto_en timestamptz
);

CREATE INDEX solicitud_privacidad_administrativa_estado_idx
    ON public.solicitud_privacidad_administrativa (estado, creado_en DESC, id DESC);

CREATE INDEX solicitud_privacidad_administrativa_cuenta_idx
    ON public.solicitud_privacidad_administrativa (cuenta_id, creado_en DESC);

CREATE TABLE public.aprobacion_administrativa (
    id uuid PRIMARY KEY,
    tipo_accion text NOT NULL CHECK (tipo_accion IN ('refund_request', 'privacy_export', 'staff_role_grant')),
    recurso_tipo text NOT NULL,
    recurso_id text NOT NULL,
    solicitante_id uuid NOT NULL REFERENCES public.usuario(id),
    aprobador_id uuid REFERENCES public.usuario(id),
    estado text NOT NULL DEFAULT 'pending' CHECK (estado IN ('pending', 'approved', 'rejected', 'cancelled')),
    motivo text NOT NULL,
    decision_motivo text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key text NOT NULL UNIQUE,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    creada_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creada_en)),
    decidida_en timestamptz,
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    CONSTRAINT aprobacion_solicitante_distinto_aprobador CHECK (aprobador_id IS NULL OR aprobador_id <> solicitante_id)
);

CREATE INDEX aprobacion_administrativa_estado_idx
    ON public.aprobacion_administrativa (estado, creada_en DESC, id DESC);

CREATE INDEX aprobacion_administrativa_solicitante_idx
    ON public.aprobacion_administrativa (solicitante_id, creada_en DESC);

REVOKE ALL ON TABLE
    public.ticket_soporte_administrativo,
    public.solicitud_privacidad_administrativa,
    public.aprobacion_administrativa
FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE ON TABLE public.ticket_soporte_administrativo TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.solicitud_privacidad_administrativa TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.aprobacion_administrativa TO espacigo_runtime;
    END IF;
END $grants$;
