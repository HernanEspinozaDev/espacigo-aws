-- Email OTP challenges for landlord commercial auto-authorization activation.
CREATE TABLE public.autorizacion_firma_desafio_otp (
    id uuid PRIMARY KEY,
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    plantilla_version bigint NOT NULL CHECK (plantilla_version > 0),
    canal text NOT NULL DEFAULT 'email_otp' CHECK (canal IN ('email_otp')),
    codigo_hmac bytea NOT NULL CHECK (octet_length(codigo_hmac) = 32),
    intentos integer NOT NULL DEFAULT 0 CHECK (intentos BETWEEN 0 AND 5),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    verificado_en timestamptz,
    creada_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creada_en))
);

CREATE INDEX autorizacion_firma_desafio_otp_lookup_idx
    ON public.autorizacion_firma_desafio_otp (actor_id, espacio_id, plantilla_version, creada_en DESC);

REVOKE ALL ON TABLE public.autorizacion_firma_desafio_otp FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE ON TABLE public.autorizacion_firma_desafio_otp TO espacigo_runtime;
    END IF;
END $grants$;
