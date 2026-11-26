CREATE TABLE public.account_pinpass_email_challenge (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE CASCADE,
    session_id uuid NOT NULL,
    challenge_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('pinpass', 'password', 'authenticator', 'new_email')),
    destination text,
    CHECK ((purpose = 'new_email' AND consumed_at IS NULL) = (destination IS NOT NULL)),
    code_hmac bytea NOT NULL CHECK (octet_length(code_hmac) = 32),
    sent_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > sent_at),
    consumed_at timestamptz,
    FOREIGN KEY (account_id, session_id) REFERENCES public.sesion (usuario_id, id) ON DELETE CASCADE
);

REVOKE ALL ON public.account_pinpass_email_challenge FROM PUBLIC, espacigo_runtime;
GRANT SELECT, INSERT, UPDATE ON public.account_pinpass_email_challenge TO espacigo_runtime;

-- Contact updates do not change the identity revision linked to Didit.
CREATE TABLE public.account_identity_profile_phone (
    account_id uuid PRIMARY KEY REFERENCES public.account_identity_profile(account_id) ON DELETE CASCADE,
    key_version text NOT NULL,
    encrypted_phone bytea NOT NULL,
    masked_phone text NOT NULL
);
REVOKE ALL ON public.account_identity_profile_phone FROM PUBLIC, espacigo_runtime;
GRANT SELECT, INSERT, UPDATE ON public.account_identity_profile_phone TO espacigo_runtime;
