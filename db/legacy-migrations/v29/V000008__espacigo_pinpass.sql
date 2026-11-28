CREATE TABLE public.account_pinpass (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE CASCADE,
    verifier_salt bytea NOT NULL CHECK (octet_length(verifier_salt) = 16),
    verifier_hash bytea NOT NULL CHECK (octet_length(verifier_hash) = 32),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX sesion_usuario_id_pinpass_uk ON public.sesion (usuario_id, id);

-- A NULL session_id is the account-wide limiter bucket. The second bucket is
-- scoped to an authenticated session. Both are updated under accountlock.
CREATE TABLE public.account_pinpass_attempt (
    account_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    session_id uuid,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    blocked_until timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT account_pinpass_attempt_scope_uk UNIQUE NULLS NOT DISTINCT (account_id, session_id),
    CONSTRAINT account_pinpass_attempt_session_fk FOREIGN KEY (account_id, session_id)
        REFERENCES public.sesion (usuario_id, id) ON DELETE CASCADE
);

CREATE TABLE public.account_pinpass_access (
    account_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    session_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (account_id, session_id),
    FOREIGN KEY (account_id, session_id) REFERENCES public.sesion (usuario_id, id) ON DELETE CASCADE,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes')
);

CREATE INDEX account_pinpass_access_expiry_idx ON public.account_pinpass_access (expires_at);

CREATE TABLE public.account_pinpass_audit (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    session_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN (
        'pinpass.changed',
        'pinpass.reset',
        'pinpass.unlock_denied',
        'pinpass.unlocked',
        'pinpass.sensitive_data_viewed'
    )),
    occurred_at timestamptz NOT NULL
);

CREATE INDEX account_pinpass_audit_account_time_idx
    ON public.account_pinpass_audit (account_id, occurred_at DESC, id DESC);

-- Account-scoped password reauthentication throttling, shared by all sessions.
CREATE TABLE public.profile_pinpass_reauth (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE CASCADE,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    blocked_until timestamptz,
    updated_at timestamptz NOT NULL
);

REVOKE ALL ON public.account_pinpass, public.account_pinpass_attempt,
    public.account_pinpass_access, public.account_pinpass_audit,
    public.profile_pinpass_reauth FROM PUBLIC;
REVOKE ALL ON public.account_pinpass, public.account_pinpass_attempt,
    public.account_pinpass_access, public.account_pinpass_audit,
    public.profile_pinpass_reauth FROM espacigo_runtime;

GRANT SELECT, INSERT, UPDATE, DELETE ON public.account_pinpass TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.account_pinpass_attempt TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.account_pinpass_access TO espacigo_runtime;
GRANT INSERT ON public.account_pinpass_audit TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.profile_pinpass_reauth TO espacigo_runtime;
