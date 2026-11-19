-- Location permission and trusted contacts (Fase 7)
CREATE TABLE public.cuenta_permiso_ubicacion (
    account_id uuid NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    consent_updated_at timestamptz NOT NULL DEFAULT now(),
    last_region_label text,
    CONSTRAINT cuenta_permiso_ubicacion_pk PRIMARY KEY (account_id),
    CONSTRAINT cuenta_permiso_ubicacion_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

COMMENT ON TABLE public.cuenta_permiso_ubicacion IS
    'Configuracion de consentimiento y permiso de ubicacion geografica por cuenta.';

CREATE TABLE public.cuenta_persona_confianza (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    full_name text NOT NULL,
    email text NOT NULL,
    phone_e164 text,
    relationship text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_persona_confianza_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_persona_confianza_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE,
    CONSTRAINT cuenta_persona_confianza_status_ck CHECK (status IN ('active', 'revoked'))
);

CREATE INDEX idx_cuenta_persona_confianza_account
    ON public.cuenta_persona_confianza (account_id, created_at DESC);

COMMENT ON TABLE public.cuenta_persona_confianza IS
    'Personas o contactos de confianza designados para recuperacion y seguridad.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_permiso_ubicacion TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_persona_confianza TO espacigo_runtime;
    END IF;
END $grants$;
