-- Passkeys (WebAuthn/FIDO2) credentials and challenges (Fase 5)
CREATE TABLE public.cuenta_passkey (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    credential_id text NOT NULL,
    public_key bytea NOT NULL,
    algorithm integer NOT NULL DEFAULT -7,
    sign_count bigint NOT NULL DEFAULT 0,
    label text NOT NULL,
    transports text[] NOT NULL DEFAULT '{}',
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_passkey_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_passkey_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE,
    CONSTRAINT cuenta_passkey_credential_id_key UNIQUE (credential_id)
);

CREATE INDEX idx_cuenta_passkey_account_created
    ON public.cuenta_passkey (account_id, created_at DESC);

COMMENT ON TABLE public.cuenta_passkey IS
    'Credenciales WebAuthn / FIDO2 (Passkeys) asociadas a cuentas de usuario.';

CREATE TABLE public.cuenta_webauthn_challenge (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    challenge text NOT NULL,
    purpose text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_webauthn_challenge_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_webauthn_challenge_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE,
    CONSTRAINT cuenta_webauthn_challenge_purpose_ck CHECK (purpose IN ('register', 'authenticate'))
);

CREATE INDEX idx_cuenta_webauthn_challenge_lookup
    ON public.cuenta_webauthn_challenge (account_id, purpose, expires_at);

COMMENT ON TABLE public.cuenta_webauthn_challenge IS
    'Desafios (challenges) temporales emitidos para registro o autenticacion WebAuthn.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_passkey TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_webauthn_challenge TO espacigo_runtime;
    END IF;
END $grants$;
