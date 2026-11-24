-- Reinforced Simple Electronic Signature (FES - Ley 19.799) with OTP challenge,
-- contextual evidence, PDF seal metadata, and extensible provider modality.
CREATE TABLE public.contrato_firma_desafio_otp (
    id uuid PRIMARY KEY,
    propuesta_id uuid NOT NULL REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT,
    solicitud_id uuid REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
    actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    rol text NOT NULL CHECK (rol IN ('arrendatario', 'arrendador')),
    documento_sha256 bytea NOT NULL CHECK (octet_length(documento_sha256) = 32),
    canal text NOT NULL CHECK (canal IN ('email_otp', 'totp')),
    codigo_hmac bytea NOT NULL CHECK (octet_length(codigo_hmac) = 32),
    intentos integer NOT NULL DEFAULT 0 CHECK (intentos BETWEEN 0 AND 5),
    expira_en timestamptz NOT NULL CHECK (isfinite(expira_en)),
    verificado_en timestamptz,
    creada_en timestamptz NOT NULL DEFAULT now() CHECK (isfinite(creada_en))
);

CREATE INDEX contrato_firma_desafio_lookup_idx
    ON public.contrato_firma_desafio_otp (actor_id, propuesta_id, rol, creada_en DESC);

REVOKE ALL ON TABLE public.contrato_firma_desafio_otp FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON TABLE public.contrato_firma_desafio_otp TO espacigo_runtime;

COMMENT ON TABLE public.contrato_firma_desafio_otp IS
    'Desafios OTP de firma electronica simple (FES) ligados al actor, rol y SHA-256 del documento. Nunca almacena codigos en texto plano.';

ALTER TABLE public.contrato_solicitud_comercial
    ADD COLUMN modalidad_firma text NOT NULL DEFAULT 'fes_interna'
        CHECK (modalidad_firma IN ('fes_interna', 'firmavirtual_notarial', 'fea_externa'));

ALTER TABLE public.contrato_firma_comercial
    ADD COLUMN tipo_firma text NOT NULL DEFAULT 'FES'
        CHECK (tipo_firma IN ('FES', 'FEA', 'NOTARIAL')),
    ADD COLUMN metodo_autenticacion text NOT NULL DEFAULT 'session_consent'
        CHECK (metodo_autenticacion IN ('session_consent', 'email_otp', 'totp', 'external_provider')),
    ADD COLUMN desafio_otp_id uuid REFERENCES public.contrato_firma_desafio_otp(id) ON DELETE RESTRICT,
    ADD COLUMN ip_masked text,
    ADD COLUMN user_agent_hash bytea CHECK (user_agent_hash IS NULL OR octet_length(user_agent_hash) = 32),
    ADD COLUMN evidencia_sha256 bytea CHECK (evidencia_sha256 IS NULL OR octet_length(evidencia_sha256) = 32),
    ADD COLUMN proveedor_ref text;

ALTER TABLE public.contrato_documento_comercial
    ADD COLUMN evidencia_object_key text,
    ADD COLUMN evidencia_sha256 bytea CHECK (evidencia_sha256 IS NULL OR octet_length(evidencia_sha256) = 32),
    ADD COLUMN sello_cert_fingerprint text;
