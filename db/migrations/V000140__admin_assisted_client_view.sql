-- Audited, time-bounded, read-only assisted client view grants for global administrators.
CREATE TABLE public.vista_asistida_administrativa (
    id uuid PRIMARY KEY,
    sesion_administrativa_id uuid NOT NULL REFERENCES public.sesion_administrativa(id) ON DELETE CASCADE,
    actor_cuenta_id uuid NOT NULL REFERENCES public.usuario(id),
    objetivo_cuenta_id uuid NOT NULL REFERENCES public.usuario(id),
    rol_vista text NOT NULL CHECK (rol_vista IN ('arrendador', 'arrendatario')),
    caso_id text NOT NULL,
    motivo text NOT NULL,
    creada_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creada_en)),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    revocada_en timestamptz,
    CONSTRAINT vista_asistida_actor_distinto_objetivo CHECK (actor_cuenta_id <> objetivo_cuenta_id)
);

CREATE INDEX vista_asistida_administrativa_actor_idx
    ON public.vista_asistida_administrativa (actor_cuenta_id, revocada_en, expira_en);

REVOKE ALL ON TABLE public.vista_asistida_administrativa FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE ON TABLE public.vista_asistida_administrativa TO espacigo_runtime;
    END IF;
END $grants$;
