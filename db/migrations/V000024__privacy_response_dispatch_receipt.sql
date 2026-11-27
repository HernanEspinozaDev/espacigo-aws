CREATE TABLE public.privacy_response_dispatch_receipt (
    request_id uuid PRIMARY KEY REFERENCES public.solicitud_titular(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    channel text NOT NULL CHECK (channel IN ('email', 'postal')),
    destination text NOT NULL CHECK (length(btrim(destination)) BETWEEN 1 AND 500),
    sent_at timestamptz NOT NULL,
    evidence_ref text NOT NULL CHECK (length(btrim(evidence_ref)) BETWEEN 1 AND 500),
    dispatch_recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT privacy_dispatch_destination_ck CHECK (
        (channel = 'email' AND destination ~* '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$') OR
        (channel = 'postal' AND length(btrim(destination)) >= 5)
    )
);

REVOKE ALL ON TABLE public.privacy_response_dispatch_receipt FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        REVOKE ALL ON TABLE public.privacy_response_dispatch_receipt FROM espacigo_runtime;
        GRANT SELECT, INSERT ON TABLE public.privacy_response_dispatch_receipt TO espacigo_runtime;
    END IF;
END $$;
