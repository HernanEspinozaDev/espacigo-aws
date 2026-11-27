-- KYB-01: owner-scoped business identity, encrypted RUTs and private document refs.
DO $guard$
BEGIN
    IF session_user <> 'espacigo_migrator_login'
       OR current_user <> 'espacigo_schema_owner'
       OR current_schema() <> 'public'
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'V000011 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

CREATE TABLE public.organizacion (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL UNIQUE REFERENCES public.usuario(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (id, usuario_id)
);

CREATE TABLE public.kyb_verifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    submitted_by uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    idempotency_key varchar(120) NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 120),
    legal_name varchar(200) NOT NULL CHECK (length(btrim(legal_name)) BETWEEN 2 AND 200),
    company_rut_fingerprint varchar(97) NOT NULL
        CHECK (company_rut_fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    company_rut_key_version varchar(64) NOT NULL
        CHECK (company_rut_key_version ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    company_rut_ciphertext bytea NOT NULL
        CHECK (octet_length(company_rut_ciphertext) BETWEEN 29 AND 65564),
    representative_name varchar(200) NOT NULL CHECK (length(btrim(representative_name)) BETWEEN 2 AND 200),
    representative_rut_fingerprint varchar(97) NOT NULL
        CHECK (representative_rut_fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    representative_rut_key_version varchar(64) NOT NULL
        CHECK (representative_rut_key_version ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    representative_rut_ciphertext bytea NOT NULL
        CHECK (octet_length(representative_rut_ciphertext) BETWEEN 29 AND 65564),
    sii_giro_code varchar(6) NOT NULL CHECK (sii_giro_code ~ '^[0-9]{6}$'),
    status varchar(16) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    rejection_reason varchar(500),
    submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    reviewed_at timestamptz,
    reviewer_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (updated_at >= created_at AND submitted_at >= created_at),
    CHECK ((status = 'REJECTED' AND rejection_reason IS NOT NULL AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
        OR (status <> 'REJECTED' AND rejection_reason IS NULL)),
    CHECK ((status IN ('APPROVED','REJECTED') AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
        OR (status IN ('PENDING','IN_REVIEW') AND reviewer_id IS NULL AND reviewed_at IS NULL)),
    FOREIGN KEY (organization_id, submitted_by)
        REFERENCES public.organizacion(id, usuario_id) ON DELETE RESTRICT,
    UNIQUE (id, organization_id),
    UNIQUE (submitted_by, idempotency_key)
);
CREATE UNIQUE INDEX kyb_verifications_one_open_per_organization_uq
    ON public.kyb_verifications(organization_id)
    WHERE status IN ('PENDING','IN_REVIEW');
CREATE UNIQUE INDEX kyb_verifications_active_company_rut_uq
    ON public.kyb_verifications(company_rut_fingerprint)
    WHERE status IN ('PENDING','IN_REVIEW','APPROVED');
CREATE INDEX kyb_verifications_status_submitted_idx
    ON public.kyb_verifications(status, submitted_at, id);

CREATE TABLE public.kyb_verification_document (
    id uuid PRIMARY KEY,
    verification_id uuid NOT NULL REFERENCES public.kyb_verifications(id) ON DELETE RESTRICT,
    document_kind varchar(40) NOT NULL
        CHECK (document_kind IN ('erut','power_certificate','property_or_sublease_authorization')),
    storage_ref uuid NOT NULL UNIQUE,
    mime_type varchar(32) NOT NULL CHECK (mime_type IN ('application/pdf','image/jpeg','image/png')),
    size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    content_sha256 bytea NOT NULL CHECK (octet_length(content_sha256) = 32),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (verification_id, document_kind)
);
CREATE INDEX kyb_verification_document_verification_idx
    ON public.kyb_verification_document(verification_id, document_kind);

CREATE TABLE public.kyb_rut_claim (
    rut_kind varchar(20) NOT NULL CHECK (rut_kind IN ('company','representative')),
    fingerprint varchar(97) NOT NULL
        CHECK (fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    verification_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (verification_id, organization_id)
        REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT,
    PRIMARY KEY (verification_id, rut_kind, fingerprint)
);
CREATE UNIQUE INDEX kyb_company_rut_claim_unique_idx
    ON public.kyb_rut_claim(fingerprint) WHERE rut_kind = 'company' AND active;
CREATE INDEX kyb_rut_claim_verification_idx
    ON public.kyb_rut_claim(verification_id, rut_kind);

CREATE TABLE public.kyb_verification_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verification_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    action varchar(24) NOT NULL CHECK (action IN ('submitted','review_started','approved','rejected')),
    from_status varchar(16) CHECK (from_status IS NULL OR from_status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    to_status varchar(16) NOT NULL CHECK (to_status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    reason varchar(500),
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (verification_id, organization_id)
        REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT
);
CREATE INDEX kyb_verification_event_verification_idx
    ON public.kyb_verification_event(verification_id, occurred_at, id);

CREATE FUNCTION public.kyb_verification_event_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION 'KYB verification events are append-only' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER kyb_verification_event_no_mutation
    BEFORE UPDATE OR DELETE ON public.kyb_verification_event
    FOR EACH ROW EXECUTE FUNCTION public.kyb_verification_event_immutable();
CREATE TRIGGER kyb_verification_event_no_truncate
    BEFORE TRUNCATE ON public.kyb_verification_event
    FOR EACH STATEMENT EXECUTE FUNCTION public.kyb_verification_event_immutable();
REVOKE ALL ON FUNCTION public.kyb_verification_event_immutable() FROM PUBLIC;

ALTER TABLE public.espacio
    ADD COLUMN es_b2b boolean NOT NULL DEFAULT false,
    ADD COLUMN organizacion_id uuid,
    ADD CONSTRAINT espacio_b2b_organization_ck
        CHECK ((es_b2b AND organizacion_id IS NOT NULL) OR (NOT es_b2b AND organizacion_id IS NULL)),
    ADD CONSTRAINT espacio_b2b_owner_fk
        FOREIGN KEY (organizacion_id, propietario_id)
        REFERENCES public.organizacion(id, usuario_id) ON DELETE RESTRICT;
CREATE INDEX espacio_b2b_organization_idx ON public.espacio(organizacion_id) WHERE es_b2b;

REVOKE ALL ON TABLE public.organizacion, public.kyb_verifications,
    public.kyb_verification_document, public.kyb_rut_claim, public.kyb_verification_event FROM PUBLIC;
REVOKE ALL ON FUNCTION public.kyb_verification_event_immutable() FROM PUBLIC;

DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE (created_at) ON public.organizacion TO espacigo_runtime;
        GRANT SELECT ON public.kyb_verifications, public.kyb_verification_document TO espacigo_runtime;
        GRANT INSERT (id, organization_id, submitted_by, idempotency_key, legal_name, company_rut_fingerprint,
            company_rut_key_version, company_rut_ciphertext, representative_name,
            representative_rut_fingerprint, representative_rut_key_version,
            representative_rut_ciphertext, sii_giro_code, status, submitted_at,
            created_at, updated_at) ON public.kyb_verifications TO espacigo_runtime;
        GRANT UPDATE (status, rejection_reason, reviewer_id, reviewed_at, updated_at)
            ON public.kyb_verifications TO espacigo_runtime;
        GRANT INSERT (id, verification_id, document_kind, storage_ref, mime_type, size_bytes, content_sha256)
            ON public.kyb_verification_document TO espacigo_runtime;
        GRANT INSERT (rut_kind, fingerprint, verification_id, organization_id)
            ON public.kyb_rut_claim TO espacigo_runtime;
        GRANT SELECT (verification_id), UPDATE (active)
            ON public.kyb_rut_claim TO espacigo_runtime;
        GRANT SELECT, INSERT ON public.kyb_verification_event TO espacigo_runtime;
        GRANT USAGE ON SEQUENCE public.kyb_verification_event_id_seq TO espacigo_runtime;
        GRANT INSERT (es_b2b, organizacion_id), UPDATE (es_b2b, organizacion_id)
            ON public.espacio TO espacigo_runtime;
    END IF;
END
$runtime$;
