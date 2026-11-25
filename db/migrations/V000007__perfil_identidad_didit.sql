CREATE TABLE public.account_identity_profile_draft (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE CASCADE,
    key_version text NOT NULL,
    encrypted_data bytea NOT NULL CHECK (octet_length(encrypted_data) > 28),
    updated_at timestamptz NOT NULL
);

CREATE TABLE public.account_identity_profile_revision (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    key_version text NOT NULL,
    encrypted_data bytea NOT NULL CHECK (octet_length(encrypted_data) > 28),
    masked_rut text NOT NULL,
    masked_phone text NOT NULL,
    saved_at timestamptz NOT NULL,
    UNIQUE (account_id, id)
);

CREATE TABLE public.account_identity_profile (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE CASCADE,
    username text NOT NULL CHECK (username ~ '^[A-Za-z0-9_][A-Za-z0-9_.-]{2,29}$'),
    current_revision uuid NOT NULL,
    FOREIGN KEY (account_id, current_revision) REFERENCES public.account_identity_profile_revision(account_id, id)
);
CREATE UNIQUE INDEX account_identity_profile_username_unique ON public.account_identity_profile (lower(username));

CREATE TABLE public.account_identity_rut_claim (
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id),
    identity_revision uuid NOT NULL,
    claimed_at timestamptz NOT NULL,
    FOREIGN KEY (account_id, identity_revision) REFERENCES public.account_identity_profile_revision(account_id, id)
);

CREATE TABLE public.account_identity_rut_fingerprint (
    fingerprint text PRIMARY KEY CHECK (fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    account_id uuid NOT NULL REFERENCES public.account_identity_rut_claim(account_id),
    UNIQUE (account_id, fingerprint)
);

CREATE TABLE public.account_identity_verification (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL,
    profile_revision uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('in_progress','in_review','needs_correction','verified','expired','revoked')),
    provider_session uuid UNIQUE,
    started_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    decision_at timestamptz,
    last_refresh_at timestamptz,
    verified_at timestamptz,
    verified_until timestamptz,
    FOREIGN KEY (account_id, profile_revision) REFERENCES public.account_identity_profile_revision(account_id, id) ON DELETE CASCADE,
    CHECK (state <> 'verified' OR (verified_at IS NOT NULL AND verified_until > verified_at))
);
CREATE INDEX account_identity_verification_latest ON public.account_identity_verification (account_id, profile_revision, started_at DESC, id DESC);
CREATE UNIQUE INDEX account_identity_verification_pending ON public.account_identity_verification (account_id, profile_revision)
    WHERE state IN ('in_progress','in_review');

CREATE TABLE public.account_identity_webhook_receipt (
    event_id uuid PRIMARY KEY,
    attempt_id uuid NOT NULL REFERENCES public.account_identity_verification(id) ON DELETE CASCADE,
    received_at timestamptz NOT NULL
);

CREATE FUNCTION public.account_profile_identity_status(subject uuid, as_of timestamptz)
RETURNS TABLE (complete boolean, verified boolean)
LANGUAGE sql STABLE SET search_path = pg_catalog
AS $$
    SELECT p.account_id IS NOT NULL,
           COALESCE(v.state = 'verified' AND v.verified_at <= as_of AND v.verified_until > as_of, false)
    FROM public.usuario AS u
    LEFT JOIN public.account_identity_profile AS p ON p.account_id = u.id
    LEFT JOIN LATERAL (
        SELECT a.state,a.verified_at,a.verified_until
        FROM public.account_identity_verification AS a
        WHERE a.account_id = u.id AND a.profile_revision = p.current_revision
        ORDER BY a.started_at DESC,a.id DESC LIMIT 1
    ) AS v ON true
    WHERE u.id = subject AND u.estado = 'activo';
$$;

REVOKE ALL ON public.account_identity_profile_draft, public.account_identity_profile_revision,
    public.account_identity_profile, public.account_identity_rut_claim, public.account_identity_rut_fingerprint,
    public.account_identity_verification, public.account_identity_webhook_receipt FROM PUBLIC, espacigo_runtime;
REVOKE ALL ON FUNCTION public.account_profile_identity_status(uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.account_profile_identity_status(uuid,timestamptz) TO espacigo_runtime;
GRANT SELECT, INSERT ON public.account_identity_profile_revision TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE ON public.account_identity_profile TO espacigo_runtime;
GRANT SELECT, INSERT ON public.account_identity_rut_claim, public.account_identity_rut_fingerprint TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE ON public.account_identity_verification TO espacigo_runtime;
GRANT SELECT, INSERT ON public.account_identity_webhook_receipt TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.account_identity_profile_draft TO espacigo_runtime;
