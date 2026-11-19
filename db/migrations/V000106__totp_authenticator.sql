-- Google Authenticator TOTP and backup codes (Fase 4)
CREATE TABLE public.cuenta_totp (
    account_id uuid NOT NULL,
    secret_ciphertext bytea,
    key_version text,
    pending_secret_ciphertext bytea,
    pending_key_version text,
    enabled_at timestamptz,
    last_verified_step bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_totp_pk PRIMARY KEY (account_id),
    CONSTRAINT cuenta_totp_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

COMMENT ON TABLE public.cuenta_totp IS
    'Estado y secretos cifrados de autenticador TOTP (Google Authenticator) por cuenta.';

CREATE TABLE public.cuenta_totp_backup_code (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    code_hash text NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_totp_backup_code_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_totp_backup_code_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

CREATE INDEX idx_cuenta_totp_backup_account
    ON public.cuenta_totp_backup_code (account_id, used_at);

COMMENT ON TABLE public.cuenta_totp_backup_code IS
    'Codigos de respaldo de recuperacion TOTP con hash SHA-256 por cuenta.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_totp TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_totp_backup_code TO espacigo_runtime;
    END IF;
END $grants$;
