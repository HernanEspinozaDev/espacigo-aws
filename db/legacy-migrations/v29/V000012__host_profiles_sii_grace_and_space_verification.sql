-- KYB-02: multi-user organizations, personal/SII compliance, per-space verification and grace.
DO $guard$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles
        WHERE rolname = session_user AND rolsuper
    ) AND (
        session_user <> 'espacigo_migrator_login'
        OR current_user <> 'espacigo_schema_owner'
        OR current_schema() <> 'public'
        OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE')
    ) THEN
        RAISE EXCEPTION 'V000012 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

-- A company RUT belongs to the organization, even when an individual submission is rejected.
ALTER TABLE public.organizacion
    ADD COLUMN created_by_usuario_id uuid,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    ADD COLUMN company_rut_fingerprint varchar(97),
    ADD COLUMN sii_giro_code varchar(6) CHECK (sii_giro_code IS NULL OR sii_giro_code ~ '^[0-9]{6}$'),
    ADD COLUMN legal_name varchar(200),
    ADD COLUMN trade_name varchar(200),
    ADD COLUMN public_summary varchar(2000),
    ADD COLUMN logo_media_id uuid,
    ADD COLUMN commune_code varchar(32),
    ADD COLUMN region_code varchar(32),
    ADD COLUMN compliance_status varchar(32) NOT NULL DEFAULT 'grace_active'
        CHECK (compliance_status IN ('grace_active','in_review','compliant','disabled_missing_docs')),
    ADD COLUMN grace_started_at timestamptz,
    ADD COLUMN grace_due_at timestamptz,
    ADD COLUMN disabled_at timestamptz,
    ADD COLUMN disabled_reason varchar(500),
    ADD COLUMN disabled_by uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    ADD COLUMN last_reminder_stage varchar(24),
    ADD CONSTRAINT organizacion_rut_fingerprint_ck CHECK (company_rut_fingerprint IS NULL OR
        company_rut_fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    ADD CONSTRAINT organizacion_grace_dates_ck CHECK (grace_due_at IS NULL OR
        (grace_started_at IS NOT NULL AND grace_due_at > grace_started_at)),
    ADD CONSTRAINT organizacion_disabled_state_ck CHECK (
        (compliance_status = 'disabled_missing_docs' AND disabled_at IS NOT NULL AND disabled_reason IS NOT NULL)
        OR (compliance_status <> 'disabled_missing_docs' AND disabled_at IS NULL));
UPDATE public.organizacion SET created_by_usuario_id = usuario_id;
ALTER TABLE public.organizacion ALTER COLUMN created_by_usuario_id SET NOT NULL;
ALTER TABLE public.organizacion ADD CONSTRAINT organizacion_created_by_fk
    FOREIGN KEY (created_by_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;
ALTER TABLE public.organizacion DROP CONSTRAINT organizacion_usuario_id_key;
ALTER TABLE public.kyb_verifications DROP CONSTRAINT kyb_verifications_organization_id_submitted_by_fkey;
ALTER TABLE public.kyb_verifications ADD CONSTRAINT kyb_verifications_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX organizacion_company_rut_fingerprint_uq
    ON public.organizacion(company_rut_fingerprint) WHERE company_rut_fingerprint IS NOT NULL;
DO $legacy_rut_claims$
BEGIN
    IF EXISTS (SELECT 1 FROM public.kyb_rut_claim WHERE rut_kind='company'
        GROUP BY fingerprint HAVING count(DISTINCT organization_id) > 1) THEN
        RAISE EXCEPTION 'V000012 found company RUT claims reserved by multiple organizations; resolve claims before migration';
    END IF;
END
$legacy_rut_claims$;
CREATE TABLE public.organizacion_rut_fingerprint (
    organizacion_id uuid NOT NULL REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    fingerprint varchar(97) NOT NULL CHECK (fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (organizacion_id, fingerprint),
    UNIQUE (fingerprint)
);
INSERT INTO public.organizacion_rut_fingerprint(organizacion_id, fingerprint)
SELECT organization_id, fingerprint FROM public.kyb_rut_claim WHERE rut_kind='company'
GROUP BY organization_id, fingerprint;
UPDATE public.organizacion o SET company_rut_fingerprint = claims.fingerprint
FROM (SELECT DISTINCT ON (organizacion_id) organizacion_id, fingerprint
      FROM public.organizacion_rut_fingerprint ORDER BY organizacion_id, created_at, fingerprint) claims
WHERE o.id=claims.organizacion_id AND o.company_rut_fingerprint IS NULL;

-- Company ownership now lives in the unique organization fingerprint registry.
-- Keep prior approved evidence while a new representative's case is reviewed.
DROP INDEX public.kyb_verifications_active_company_rut_uq;
DROP INDEX public.kyb_company_rut_claim_unique_idx;

CREATE TABLE public.organizacion_miembro (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organizacion_id uuid NOT NULL REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    usuario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    rol varchar(32) NOT NULL CHECK (rol IN ('representante_legal','administrador','operador')),
    estado varchar(16) NOT NULL DEFAULT 'activo' CHECK (estado IN ('activo','suspendido','retirado')),
    invited_by uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    joined_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    removed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (id, organizacion_id),
    CHECK ((estado = 'retirado' AND removed_at IS NOT NULL) OR (estado <> 'retirado' AND removed_at IS NULL))
);
INSERT INTO public.organizacion_miembro(organizacion_id, usuario_id, rol, estado, joined_at)
SELECT id, usuario_id, 'representante_legal', 'activo', created_at FROM public.organizacion;
CREATE UNIQUE INDEX organizacion_miembro_active_user_uq
    ON public.organizacion_miembro(organizacion_id, usuario_id) WHERE estado = 'activo';
CREATE INDEX organizacion_miembro_user_idx ON public.organizacion_miembro(usuario_id, estado, organizacion_id);
CREATE INDEX organizacion_miembro_role_idx ON public.organizacion_miembro(organizacion_id, rol) WHERE estado = 'activo';

CREATE TABLE public.organizacion_invitacion (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organizacion_id uuid NOT NULL REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    tipo varchar(24) NOT NULL CHECK (tipo IN ('invitacion','solicitud_adhesion')),
    target_email_normalized varchar(320),
    target_usuario_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    rol_propuesto varchar(32) NOT NULL CHECK (rol_propuesto IN ('representante_legal','administrador','operador')),
    token_sha256 bytea UNIQUE CHECK (token_sha256 IS NULL OR octet_length(token_sha256) = 32),
    estado varchar(16) NOT NULL DEFAULT 'pendiente'
        CHECK (estado IN ('pendiente','aceptada','rechazada','expirada','revocada')),
    expires_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    resolved_by uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((target_email_normalized IS NOT NULL) OR (target_usuario_id IS NOT NULL)),
    CHECK (target_email_normalized IS NULL OR target_email_normalized = lower(btrim(target_email_normalized))),
    CHECK ((estado = 'pendiente' AND resolved_at IS NULL) OR (estado <> 'pendiente' AND resolved_at IS NOT NULL))
);
CREATE UNIQUE INDEX organizacion_invitacion_pending_email_uq
    ON public.organizacion_invitacion(organizacion_id, target_email_normalized)
    WHERE estado = 'pendiente' AND target_email_normalized IS NOT NULL;
CREATE UNIQUE INDEX organizacion_invitacion_pending_user_uq
    ON public.organizacion_invitacion(organizacion_id, target_usuario_id)
    WHERE estado = 'pendiente' AND target_usuario_id IS NOT NULL;
CREATE INDEX organizacion_invitacion_org_status_idx
    ON public.organizacion_invitacion(organizacion_id, estado, created_at DESC);

CREATE FUNCTION public.organizacion_miembro_requires_legal_rep() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE org_id uuid; org_status text; rep_count bigint;
BEGIN
    org_id := COALESCE(NEW.organizacion_id, OLD.organizacion_id);
    SELECT compliance_status INTO org_status FROM public.organizacion WHERE id = org_id FOR UPDATE;
    IF org_status IS NOT NULL AND org_status <> 'disabled_missing_docs' THEN
        SELECT count(*) INTO rep_count FROM public.organizacion_miembro
        WHERE organizacion_id = org_id AND estado = 'activo' AND rol = 'representante_legal';
        IF rep_count = 0 THEN
            RAISE EXCEPTION 'an active organization must retain an active legal representative' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER organizacion_miembro_legal_rep_ck
    AFTER INSERT OR UPDATE OR DELETE ON public.organizacion_miembro
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION public.organizacion_miembro_requires_legal_rep();

CREATE FUNCTION public.kyb_bind_company_rut() RETURNS trigger
SECURITY DEFINER
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE claim_count bigint; owned_by uuid;
BEGIN
    PERFORM 1 FROM public.organizacion WHERE id = NEW.organization_id FOR UPDATE;
    SELECT organizacion_id INTO owned_by FROM public.organizacion_rut_fingerprint
      WHERE fingerprint=NEW.company_rut_fingerprint;
    IF owned_by IS NOT NULL AND owned_by <> NEW.organization_id THEN
        RAISE EXCEPTION 'company RUT fingerprint is reserved by another organization' USING ERRCODE = '23505';
    END IF;
    SELECT count(*) INTO claim_count FROM public.organizacion_rut_fingerprint
      WHERE organizacion_id=NEW.organization_id;
    IF owned_by IS NULL THEN
        IF claim_count > 0 THEN
            RAISE EXCEPTION 'company RUT fingerprint is not one of this organization keyring candidates' USING ERRCODE = '23505';
        END IF;
        INSERT INTO public.organizacion_rut_fingerprint(organizacion_id, fingerprint)
          VALUES(NEW.organization_id, NEW.company_rut_fingerprint);
    END IF;
    UPDATE public.organizacion SET company_rut_fingerprint = COALESCE(company_rut_fingerprint, NEW.company_rut_fingerprint),
      legal_name = COALESCE(legal_name, NEW.legal_name), updated_at = clock_timestamp()
      WHERE id = NEW.organization_id;
    RETURN NEW;
END;
$$;
CREATE TRIGGER kyb_bind_company_rut_before_insert
    BEFORE INSERT ON public.kyb_verifications FOR EACH ROW EXECUTE FUNCTION public.kyb_bind_company_rut();
CREATE FUNCTION public.kyb_submission_requires_active_member() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=NEW.organization_id
        AND m.usuario_id=NEW.submitted_by AND m.estado='activo') THEN
        RAISE EXCEPTION 'KYB submitter must be an active organization member' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER kyb_submission_member_before_insert BEFORE INSERT ON public.kyb_verifications
    FOR EACH ROW EXECUTE FUNCTION public.kyb_submission_requires_active_member();

-- V000011 stores property documents at organization scope. Preserve the schema, but require
-- new per-space documents to be recorded against their own space below.
ALTER TABLE public.kyb_verification_document DROP CONSTRAINT kyb_verification_document_document_kind_check;
ALTER TABLE public.kyb_verification_document ADD CONSTRAINT kyb_verification_document_document_kind_check
    CHECK (document_kind IN ('erut','sii_start_activities','power_certificate','property_or_sublease_authorization'));

CREATE TABLE public.anfitrion_cumplimiento_personal (
    usuario_id uuid PRIMARY KEY REFERENCES public.usuario(id) ON DELETE RESTRICT,
    sii_giro_code varchar(6) CHECK (sii_giro_code IS NULL OR sii_giro_code ~ '^[0-9]{6}$'),
    compliance_status varchar(32) NOT NULL DEFAULT 'grace_active'
        CHECK (compliance_status IN ('grace_active','in_review','compliant','disabled_missing_docs')),
    sii_status varchar(16) NOT NULL DEFAULT 'MISSING' CHECK (sii_status IN ('MISSING','PENDING','IN_REVIEW','APPROVED','REJECTED')),
    grace_started_at timestamptz,
    grace_due_at timestamptz,
    disabled_at timestamptz,
    disabled_reason varchar(500),
    disabled_by uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    last_reminder_stage varchar(24),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (grace_due_at IS NULL OR (grace_started_at IS NOT NULL AND grace_due_at > grace_started_at)),
    CHECK ((compliance_status = 'disabled_missing_docs' AND disabled_at IS NOT NULL AND disabled_reason IS NOT NULL)
       OR (compliance_status <> 'disabled_missing_docs' AND disabled_at IS NULL))
);
CREATE TABLE public.anfitrion_cumplimiento_documento (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL REFERENCES public.anfitrion_cumplimiento_personal(usuario_id) ON DELETE RESTRICT,
    document_kind varchar(32) NOT NULL CHECK (document_kind = 'sii_start_activities'),
    storage_ref uuid NOT NULL UNIQUE,
    mime_type varchar(32) NOT NULL CHECK (mime_type IN ('application/pdf','image/jpeg','image/png')),
    size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    content_sha256 bytea NOT NULL CHECK (octet_length(content_sha256) = 32),
    status varchar(16) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    rejection_reason varchar(500),
    submitted_by uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    reviewer_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((status = 'REJECTED' AND rejection_reason IS NOT NULL AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
       OR (status <> 'REJECTED' AND rejection_reason IS NULL)),
    CHECK ((status IN ('APPROVED','REJECTED') AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
       OR (status IN ('PENDING','IN_REVIEW') AND reviewer_id IS NULL AND reviewed_at IS NULL))
);
CREATE UNIQUE INDEX anfitrion_cumplimiento_documento_one_open_uq
    ON public.anfitrion_cumplimiento_documento(usuario_id) WHERE status IN ('PENDING','IN_REVIEW');

ALTER TABLE public.espacio ADD COLUMN modo_anfitrion varchar(16) NOT NULL DEFAULT 'personal'
    CHECK (modo_anfitrion IN ('personal','empresa'));
UPDATE public.espacio SET modo_anfitrion='empresa' WHERE es_b2b;
ALTER TABLE public.espacio ADD CONSTRAINT espacio_host_mode_consistency_ck CHECK (
    (modo_anfitrion = 'empresa' AND es_b2b AND organizacion_id IS NOT NULL)
    OR (modo_anfitrion = 'personal' AND NOT es_b2b AND organizacion_id IS NULL));
CREATE FUNCTION public.kyb_start_space_grace() RETURNS trigger
SECURITY DEFINER
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE started timestamptz := clock_timestamp(); due timestamptz; existing_started timestamptz; started_user uuid;
BEGIN
    due := ((started AT TIME ZONE 'America/Santiago' + interval '2 months') AT TIME ZONE 'America/Santiago');
    IF NEW.modo_anfitrion = 'empresa' THEN
        SELECT grace_started_at INTO existing_started FROM public.organizacion WHERE id=NEW.organizacion_id FOR UPDATE;
        IF existing_started IS NULL THEN
            UPDATE public.organizacion SET grace_started_at=started, grace_due_at=due, updated_at=clock_timestamp() WHERE id=NEW.organizacion_id;
            INSERT INTO public.host_compliance_event(organizacion_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.organizacion_id, NEW.propietario_id, 'grace_started', started);
        END IF;
    ELSE
        INSERT INTO public.anfitrion_cumplimiento_personal(usuario_id, grace_started_at, grace_due_at)
          VALUES(NEW.propietario_id, started, due) ON CONFLICT(usuario_id) DO UPDATE
          SET grace_started_at=EXCLUDED.grace_started_at, grace_due_at=EXCLUDED.grace_due_at, updated_at=clock_timestamp()
          WHERE anfitrion_cumplimiento_personal.grace_started_at IS NULL
          RETURNING usuario_id INTO started_user;
        IF started_user IS NOT NULL THEN
            INSERT INTO public.host_compliance_event(usuario_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.propietario_id, NEW.propietario_id, 'grace_started', started);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER espacio_start_kyb_grace_insert AFTER INSERT ON public.espacio
    FOR EACH ROW WHEN (NEW.estado='activa') EXECUTE FUNCTION public.kyb_start_space_grace();
CREATE TRIGGER espacio_start_kyb_grace_update AFTER UPDATE OF estado ON public.espacio
    FOR EACH ROW WHEN (NEW.estado='activa' AND OLD.estado IS DISTINCT FROM 'activa')
    EXECUTE FUNCTION public.kyb_start_space_grace();

CREATE TABLE public.space_kyb_verification (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
    host_mode varchar(16) NOT NULL CHECK (host_mode IN ('personal','empresa')),
    owner_usuario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    organizacion_id uuid REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    kyb_verification_id uuid,
    document_storage_ref uuid NOT NULL UNIQUE,
    mime_type varchar(32) NOT NULL CHECK (mime_type IN ('application/pdf','image/jpeg','image/png')),
    size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    content_sha256 bytea NOT NULL CHECK (octet_length(content_sha256) = 32),
    status varchar(16) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    rejection_reason varchar(500),
    submitted_by uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    reviewer_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (kyb_verification_id, organizacion_id)
       REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT,
    CHECK ((host_mode = 'empresa' AND organizacion_id IS NOT NULL) OR (host_mode = 'personal' AND organizacion_id IS NULL)),
    CHECK ((status = 'REJECTED' AND rejection_reason IS NOT NULL AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
       OR (status <> 'REJECTED' AND rejection_reason IS NULL)),
    CHECK ((status IN ('APPROVED','REJECTED') AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
       OR (status IN ('PENDING','IN_REVIEW') AND reviewer_id IS NULL AND reviewed_at IS NULL))
);
CREATE UNIQUE INDEX space_kyb_verification_one_open_uq
    ON public.space_kyb_verification(espacio_id) WHERE status IN ('PENDING','IN_REVIEW');
CREATE INDEX space_kyb_verification_space_history_idx
    ON public.space_kyb_verification(espacio_id, submitted_at DESC, id);

ALTER TABLE public.contrato_propuesta_comercial
    ADD COLUMN es_b2b boolean NOT NULL DEFAULT false,
    ADD COLUMN organizacion_id uuid REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    ADD COLUMN empresa_razon_social_snapshot varchar(200),
    ADD COLUMN empresa_rut_masked_snapshot varchar(16),
    ADD COLUMN empresa_rut_verificada boolean NOT NULL DEFAULT false,
    ADD COLUMN representante_legal_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    ADD COLUMN representante_nombre_snapshot varchar(200),
    ADD COLUMN representante_rut_masked_snapshot varchar(16),
    ADD COLUMN representante_rut_verificada boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT contrato_propuesta_b2b_snapshot_ck CHECK (
      (NOT es_b2b AND organizacion_id IS NULL) OR (es_b2b AND organizacion_id IS NOT NULL
       AND empresa_razon_social_snapshot IS NOT NULL AND empresa_rut_masked_snapshot IS NOT NULL
       AND empresa_rut_verificada AND representante_legal_id IS NOT NULL
       AND representante_nombre_snapshot IS NOT NULL AND representante_rut_masked_snapshot IS NOT NULL
       AND representante_rut_verificada));
ALTER TABLE public.contrato_solicitud_comercial
    ADD COLUMN es_b2b boolean NOT NULL DEFAULT false,
    ADD COLUMN organizacion_id uuid REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    ADD COLUMN empresa_razon_social_snapshot varchar(200),
    ADD COLUMN empresa_rut_masked_snapshot varchar(16),
    ADD COLUMN empresa_rut_verificada boolean NOT NULL DEFAULT false,
    ADD COLUMN representante_legal_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    ADD COLUMN representante_nombre_snapshot varchar(200),
    ADD COLUMN representante_rut_masked_snapshot varchar(16),
    ADD COLUMN representante_rut_verificada boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT contrato_solicitud_b2b_snapshot_ck CHECK (
      (NOT es_b2b AND organizacion_id IS NULL) OR (es_b2b AND organizacion_id IS NOT NULL
       AND empresa_razon_social_snapshot IS NOT NULL AND empresa_rut_masked_snapshot IS NOT NULL
       AND empresa_rut_verificada AND representante_legal_id IS NOT NULL
       AND representante_nombre_snapshot IS NOT NULL AND representante_rut_masked_snapshot IS NOT NULL
       AND representante_rut_verificada));

CREATE FUNCTION public.space_kyb_owner_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE space_mode text; space_owner uuid; space_org uuid;
BEGIN
    SELECT modo_anfitrion, propietario_id, organizacion_id INTO space_mode, space_owner, space_org
      FROM public.espacio WHERE id=NEW.espacio_id;
    IF space_mode IS NULL OR space_mode <> NEW.host_mode OR space_owner <> NEW.owner_usuario_id OR
       space_org IS DISTINCT FROM NEW.organizacion_id THEN
        RAISE EXCEPTION 'space KYB scope must match the space owner and mode' USING ERRCODE = '23514';
    END IF;
    IF NEW.host_mode='personal' AND NEW.submitted_by <> NEW.owner_usuario_id THEN
        RAISE EXCEPTION 'personal space document must be submitted by its owner' USING ERRCODE = '23514';
    ELSIF NEW.host_mode='empresa' AND NOT EXISTS (SELECT 1 FROM public.organizacion_miembro m
        WHERE m.organizacion_id=NEW.organizacion_id AND m.usuario_id=NEW.submitted_by AND m.estado='activo') THEN
        RAISE EXCEPTION 'company space document submitter must be an active member' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER space_kyb_owner_guard_before_insert_update
    BEFORE INSERT OR UPDATE ON public.space_kyb_verification FOR EACH ROW
    EXECUTE FUNCTION public.space_kyb_owner_guard();

CREATE TABLE public.host_compliance_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    usuario_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    organizacion_id uuid REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    espacio_id uuid REFERENCES public.espacio(id) ON DELETE RESTRICT,
    actor_usuario_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    event_type varchar(32) NOT NULL CHECK (event_type IN ('grace_started','reminder_sent','disabled_auto','disabled_manual','re_enabled','member_changed','personal_sii_submitted','space_property_submitted','personal_sii_reviewed','space_reviewed','space_created','space_edited')),
    reason varchar(500),
    reminder_stage varchar(24),
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (num_nonnulls(usuario_id, organizacion_id, espacio_id) = 1)
);
CREATE INDEX host_compliance_event_profile_idx
    ON public.host_compliance_event(usuario_id, organizacion_id, occurred_at DESC);
CREATE FUNCTION public.host_compliance_event_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN RAISE EXCEPTION 'host compliance events are append-only' USING ERRCODE = '23514'; END;
$$;
CREATE TRIGGER host_compliance_event_no_mutation BEFORE UPDATE OR DELETE ON public.host_compliance_event
    FOR EACH ROW EXECUTE FUNCTION public.host_compliance_event_immutable();
CREATE TRIGGER host_compliance_event_no_truncate BEFORE TRUNCATE ON public.host_compliance_event
    FOR EACH STATEMENT EXECUTE FUNCTION public.host_compliance_event_immutable();

CREATE FUNCTION public.kyb_space_eligibility(p_espacio_id uuid, p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(can_publish boolean, can_book boolean, space_verified boolean, compliance_status text)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    SELECT eligible, eligible, verified, host_status FROM (
      SELECT
        (host_status <> 'disabled_missing_docs') AS eligible,
        (host_status = 'compliant' AND property_approved AND titular_approved) AS verified,
        host_status
      FROM (
        SELECT e.id,
          CASE WHEN e.modo_anfitrion = 'empresa' THEN
                 CASE WHEN o.grace_due_at <= p_at AND o.compliance_status IN ('grace_active','in_review')
                   AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW')
              AND (SELECT count(*) FROM public.kyb_verification_document d WHERE d.verification_id=v.id
                   AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3)
                      THEN 'disabled_missing_docs' ELSE COALESCE(o.compliance_status, 'disabled_missing_docs') END
               ELSE CASE WHEN p.grace_due_at <= p_at AND p.compliance_status IN ('grace_active','in_review')
                   AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW'))
                      THEN 'disabled_missing_docs' ELSE COALESCE(p.compliance_status, 'grace_active') END END AS host_status,
          EXISTS (SELECT 1 FROM public.space_kyb_verification sv WHERE sv.espacio_id=e.id AND sv.status='APPROVED') AS property_approved,
          CASE WHEN e.modo_anfitrion = 'empresa' THEN
            EXISTS (
              SELECT 1 FROM (
                SELECT v.submitted_by FROM public.kyb_verifications v
                WHERE v.organization_id=e.organizacion_id AND v.status='APPROVED' AND v.reviewed_at<=p_at
                  AND (SELECT count(*) FROM public.kyb_verification_document d WHERE d.verification_id=v.id
                       AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
                ORDER BY v.reviewed_at DESC,v.id DESC LIMIT 1
              ) verified_company
              JOIN public.organizacion_miembro m ON m.organizacion_id=e.organizacion_id
                AND m.usuario_id=verified_company.submitted_by AND m.estado='activo' AND m.rol='representante_legal'
              CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,p_at) k
              WHERE k.verified
            )
          ELSE COALESCE(p.sii_status='APPROVED', false)
            AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(e.propietario_id,p_at) k WHERE k.verified) END AS titular_approved
        FROM public.espacio e
        LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
        LEFT JOIN public.anfitrion_cumplimiento_personal p ON p.usuario_id=e.propietario_id
        WHERE e.id=p_espacio_id
      ) profile
    ) eligibility
$$;

CREATE FUNCTION public.kyb_expire_noncompliant_profiles(p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(usuario_id uuid, organizacion_id uuid)
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    RETURN QUERY
      WITH expired_org AS (
        UPDATE public.organizacion o SET compliance_status='disabled_missing_docs', disabled_at=p_at,
          disabled_reason='Plazo de gracia vencido sin documentación aprobada', updated_at=p_at
        WHERE o.compliance_status IN ('grace_active','in_review') AND o.grace_due_at <= p_at
          AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW')
              AND (SELECT count(*) FROM public.kyb_verification_document d WHERE d.verification_id=v.id
                   AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3)
        RETURNING o.id
      ), org_events AS (
        INSERT INTO public.host_compliance_event AS event(organizacion_id,event_type,reason,occurred_at)
        SELECT id,'disabled_auto','Plazo de gracia vencido sin documentación aprobada',p_at FROM expired_org
        RETURNING event.organizacion_id
      ), expired_person AS (
        UPDATE public.anfitrion_cumplimiento_personal p SET compliance_status='disabled_missing_docs', disabled_at=p_at,
          disabled_reason='Plazo de gracia vencido sin documentación aprobada', updated_at=p_at
        WHERE p.compliance_status IN ('grace_active','in_review') AND p.grace_due_at <= p_at
          AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW'))
        RETURNING p.usuario_id
      ), person_events AS (
        INSERT INTO public.host_compliance_event AS event(usuario_id,event_type,reason,occurred_at)
        SELECT person.usuario_id,'disabled_auto','Plazo de gracia vencido sin documentación aprobada',p_at FROM expired_person person
        RETURNING event.usuario_id
      ) SELECT NULL::uuid, id FROM expired_org UNION ALL SELECT person.usuario_id, NULL::uuid FROM expired_person person;
END;
$$;

REVOKE ALL ON TABLE public.organizacion_rut_fingerprint, public.organizacion_miembro, public.organizacion_invitacion,
    public.anfitrion_cumplimiento_personal, public.anfitrion_cumplimiento_documento,
    public.space_kyb_verification, public.host_compliance_event FROM PUBLIC;
REVOKE ALL ON FUNCTION public.organizacion_miembro_requires_legal_rep(), public.kyb_bind_company_rut(), public.kyb_submission_requires_active_member(),
    public.kyb_start_space_grace(), public.space_kyb_owner_guard(), public.host_compliance_event_immutable(), public.kyb_space_eligibility(uuid,timestamptz),
    public.kyb_expire_noncompliant_profiles(timestamptz) FROM PUBLIC;

DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        REVOKE ALL ON SEQUENCE public.host_compliance_event_id_seq FROM espacigo_runtime;
        REVOKE ALL ON public.organizacion_rut_fingerprint, public.organizacion_miembro, public.organizacion_invitacion,
            public.anfitrion_cumplimiento_personal, public.anfitrion_cumplimiento_documento,
            public.space_kyb_verification, public.host_compliance_event FROM espacigo_runtime;
        GRANT SELECT, INSERT ON public.organizacion_rut_fingerprint TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON public.organizacion_miembro, public.organizacion_invitacion TO espacigo_runtime;
        GRANT SELECT, UPDATE (trade_name, public_summary, logo_media_id, commune_code, region_code, sii_giro_code, updated_at, grace_started_at, grace_due_at,
            compliance_status, disabled_at, disabled_reason, disabled_by, last_reminder_stage) ON public.organizacion TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON public.anfitrion_cumplimiento_personal, public.anfitrion_cumplimiento_documento,
            public.space_kyb_verification TO espacigo_runtime;
        GRANT SELECT, INSERT ON public.host_compliance_event TO espacigo_runtime;
        GRANT USAGE ON SEQUENCE public.host_compliance_event_id_seq TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.kyb_space_eligibility(uuid,timestamptz),
            public.kyb_expire_noncompliant_profiles(timestamptz) TO espacigo_runtime;
        GRANT INSERT, UPDATE (modo_anfitrion) ON public.espacio TO espacigo_runtime;
    END IF;
END
$runtime$;
