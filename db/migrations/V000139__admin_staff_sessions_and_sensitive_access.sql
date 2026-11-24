-- Dedicated administrative staff sessions, MFA challenges, and audited sensitive-access grants.
CREATE TABLE public.sesion_administrativa (
    id uuid PRIMARY KEY,
    cuenta_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    audiencia text NOT NULL DEFAULT 'admin' CHECK (audiencia = 'admin'),
    token_hash char(64) NOT NULL UNIQUE,
    creada_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creada_en)),
    ultima_actividad_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(ultima_actividad_en)),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    revocada_en timestamptz,
    mfa_verificado_en timestamptz,
    mfa_metodo text NOT NULL DEFAULT '',
    etiqueta_dispositivo text NOT NULL DEFAULT '',
    ip_enmascarada text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT ''
);

CREATE INDEX sesion_administrativa_cuenta_idx
    ON public.sesion_administrativa (cuenta_id, creada_en DESC);

CREATE TABLE public.desafio_mfa_administrativo (
    id uuid PRIMARY KEY,
    cuenta_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    sesion_administrativa_id uuid REFERENCES public.sesion_administrativa(id) ON DELETE CASCADE,
    proposito text NOT NULL CHECK (proposito IN ('login', 'step_up')),
    intentos integer NOT NULL DEFAULT 0 CHECK (intentos >= 0),
    max_intentos integer NOT NULL DEFAULT 5 CHECK (max_intentos > 0),
    creado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creado_en)),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    consumido_en timestamptz,
    ip_enmascarada text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT ''
);

CREATE INDEX desafio_mfa_administrativo_cuenta_idx
    ON public.desafio_mfa_administrativo (cuenta_id, creado_en DESC);

CREATE TABLE public.acceso_sensible_administrativo (
    id uuid PRIMARY KEY,
    actor_cuenta_id uuid NOT NULL REFERENCES public.usuario(id),
    objetivo_cuenta_id uuid NOT NULL REFERENCES public.usuario(id),
    sesion_administrativa_id uuid REFERENCES public.sesion_administrativa(id),
    campo text NOT NULL,
    alcance text NOT NULL,
    motivo text NOT NULL,
    caso_id text NOT NULL DEFAULT '',
    otorgado_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(otorgado_en)),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    evento_auditoria_id uuid
);

CREATE INDEX acceso_sensible_administrativo_objetivo_idx
    ON public.acceso_sensible_administrativo (objetivo_cuenta_id, otorgado_en DESC);

CREATE INDEX acceso_sensible_administrativo_actor_idx
    ON public.acceso_sensible_administrativo (actor_cuenta_id, otorgado_en DESC);

REVOKE ALL ON TABLE
    public.sesion_administrativa,
    public.desafio_mfa_administrativo,
    public.acceso_sensible_administrativo
FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE ON TABLE public.sesion_administrativa TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.desafio_mfa_administrativo TO espacigo_runtime;
        GRANT SELECT, INSERT ON TABLE public.acceso_sensible_administrativo TO espacigo_runtime;
    END IF;
END $grants$;
