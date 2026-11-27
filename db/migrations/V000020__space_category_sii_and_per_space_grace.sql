-- Per-category SII verification for multi-category companies and per-space 2-month KYB grace.
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
        RAISE EXCEPTION 'V000020 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

-- A. Category SII in public.space_kyb_verification.
ALTER TABLE public.space_kyb_verification
    ADD COLUMN document_kind varchar(40) NOT NULL DEFAULT 'property_or_sublease_authorization'
        CHECK (document_kind IN ('property_or_sublease_authorization','sii_start_activities')),
    ADD COLUMN sii_giro_code varchar(6)
        CHECK (sii_giro_code IS NULL OR sii_giro_code ~ '^[0-9]{6}$'),
    ADD COLUMN categoria_codigo text
        REFERENCES public.categoria_espacio(codigo) ON DELETE RESTRICT,
    ADD CONSTRAINT space_kyb_verification_sii_fields_ck CHECK (
        (document_kind = 'sii_start_activities'
         AND host_mode = 'empresa'
         AND organizacion_id IS NOT NULL
         AND sii_giro_code IS NOT NULL
         AND categoria_codigo IS NOT NULL)
        OR (document_kind = 'property_or_sublease_authorization'
            AND sii_giro_code IS NULL)
    );

UPDATE public.space_kyb_verification sv
SET categoria_codigo = e.categoria_codigo
FROM public.espacio e
WHERE e.id = sv.espacio_id AND sv.categoria_codigo IS NULL;

DROP INDEX public.space_kyb_verification_one_open_uq;
CREATE UNIQUE INDEX space_kyb_verification_one_open_uq
    ON public.space_kyb_verification(espacio_id, document_kind)
    WHERE status IN ('PENDING','IN_REVIEW');
CREATE INDEX space_kyb_verification_org_cat_sii_idx
    ON public.space_kyb_verification(organizacion_id, categoria_codigo)
    WHERE host_mode = 'empresa' AND document_kind = 'sii_start_activities' AND status = 'APPROVED';

CREATE OR REPLACE FUNCTION public.space_kyb_owner_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE space_mode text; space_owner uuid; space_org uuid; space_category text;
BEGIN
    SELECT modo_anfitrion, propietario_id, organizacion_id, categoria_codigo
      INTO space_mode, space_owner, space_org, space_category
      FROM public.espacio WHERE id=NEW.espacio_id;
    IF space_mode IS NULL OR space_mode <> NEW.host_mode OR space_owner <> NEW.owner_usuario_id OR
       space_org IS DISTINCT FROM NEW.organizacion_id THEN
        RAISE EXCEPTION 'space KYB scope must match the space owner and mode' USING ERRCODE = '23514';
    END IF;
    IF NEW.categoria_codigo IS NULL THEN
        NEW.categoria_codigo := space_category;
    ELSIF NEW.categoria_codigo IS DISTINCT FROM space_category THEN
        RAISE EXCEPTION 'space KYB category must match space category' USING ERRCODE = '23514';
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

-- B. Per-space 2-month grace period on public.espacio.
ALTER TABLE public.espacio
    ADD COLUMN kyb_grace_started_at timestamptz,
    ADD COLUMN kyb_grace_due_at timestamptz,
    ADD CONSTRAINT espacio_kyb_grace_dates_ck CHECK (
        kyb_grace_due_at IS NULL OR (kyb_grace_started_at IS NOT NULL AND kyb_grace_due_at > kyb_grace_started_at)
    );

UPDATE public.espacio e
SET kyb_grace_started_at = o.grace_started_at,
    kyb_grace_due_at = o.grace_due_at
FROM public.organizacion o
WHERE e.estado = 'activa' AND e.kyb_grace_started_at IS NULL
  AND e.modo_anfitrion = 'empresa' AND e.organizacion_id = o.id
  AND o.grace_started_at IS NOT NULL AND o.grace_due_at IS NOT NULL;

UPDATE public.espacio e
SET kyb_grace_started_at = p.grace_started_at,
    kyb_grace_due_at = p.grace_due_at
FROM public.anfitrion_cumplimiento_personal p
WHERE e.estado = 'activa' AND e.kyb_grace_started_at IS NULL
  AND e.modo_anfitrion = 'personal' AND e.propietario_id = p.usuario_id
  AND p.grace_started_at IS NOT NULL AND p.grace_due_at IS NOT NULL;

CREATE OR REPLACE FUNCTION public.kyb_start_space_grace() RETURNS trigger
SECURITY DEFINER
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE
    started timestamptz := clock_timestamp();
    due timestamptz;
    existing_started timestamptz;
    existing_due timestamptz;
    holder_status text;
    space_started timestamptz;
    space_due timestamptz;
    started_user uuid;
    published_by uuid;
BEGIN
    SELECT h.actor_id INTO published_by
    FROM public.espacio_publicacion_historial_local h
    WHERE h.espacio_id=NEW.id AND h.estado_nuevo='activa'
      AND h.ocurrida_en=transaction_timestamp()
    ORDER BY h.id DESC LIMIT 1;
    published_by := COALESCE(published_by, NEW.propietario_id);
    due := ((started AT TIME ZONE 'America/Santiago' + interval '2 months') AT TIME ZONE 'America/Santiago');
    IF NEW.modo_anfitrion = 'empresa' THEN
        SELECT grace_started_at, grace_due_at, compliance_status
          INTO existing_started, existing_due, holder_status
          FROM public.organizacion WHERE id=NEW.organizacion_id FOR UPDATE;
        IF existing_started IS NULL THEN
            UPDATE public.organizacion SET grace_started_at=started, grace_due_at=due, updated_at=clock_timestamp() WHERE id=NEW.organizacion_id;
            INSERT INTO public.host_compliance_event(organizacion_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.organizacion_id, published_by, 'grace_started', started);
            space_started := started;
            space_due := due;
        ELSIF holder_status = 'compliant' THEN
            space_started := started;
            space_due := due;
        ELSE
            space_started := existing_started;
            space_due := existing_due;
        END IF;
    ELSE
        INSERT INTO public.anfitrion_cumplimiento_personal(usuario_id, grace_started_at, grace_due_at)
          VALUES(NEW.propietario_id, started, due) ON CONFLICT(usuario_id) DO UPDATE
          SET grace_started_at=EXCLUDED.grace_started_at, grace_due_at=EXCLUDED.grace_due_at, updated_at=clock_timestamp()
          WHERE anfitrion_cumplimiento_personal.grace_started_at IS NULL
          RETURNING usuario_id INTO started_user;
        IF started_user IS NOT NULL THEN
            INSERT INTO public.host_compliance_event(usuario_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.propietario_id, published_by, 'grace_started', started);
            space_started := started;
            space_due := due;
        ELSE
            SELECT grace_started_at, grace_due_at, compliance_status
              INTO existing_started, existing_due, holder_status
              FROM public.anfitrion_cumplimiento_personal WHERE usuario_id=NEW.propietario_id;
            IF holder_status = 'compliant' OR existing_started IS NULL THEN
                space_started := started;
                space_due := due;
            ELSE
                space_started := existing_started;
                space_due := existing_due;
            END IF;
        END IF;
    END IF;
    IF NEW.kyb_grace_started_at IS NULL THEN
        UPDATE public.espacio
        SET kyb_grace_started_at = space_started, kyb_grace_due_at = space_due
        WHERE id = NEW.id AND kyb_grace_started_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$;

-- C. Compliance detail and eligibility functions.
CREATE FUNCTION public.kyb_space_compliance_detail(p_espacio_id uuid, p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(
    space_state text,
    host_status text,
    grace_due_at timestamptz,
    property_approved boolean,
    property_submitted_in_time boolean,
    titular_approved boolean,
    holder_submitted_in_time boolean,
    category_sii_required boolean,
    category_sii_approved boolean,
    category_sii_submitted_in_time boolean,
    categoria_codigo text
)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    WITH holder AS (
        SELECT e.id, e.estado, e.propietario_id, e.organizacion_id, e.modo_anfitrion, e.categoria_codigo,
               e.kyb_grace_due_at, p.sii_status,
               CASE WHEN e.modo_anfitrion='empresa' THEN o.grace_due_at ELSE p.grace_due_at END AS holder_due_at,
               CASE WHEN e.modo_anfitrion='empresa' THEN COALESCE(o.compliance_status,'disabled_missing_docs')
                    ELSE COALESCE(p.compliance_status,'grace_active') END AS recorded_status
        FROM public.espacio e
        LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
        LEFT JOIN public.anfitrion_cumplimiento_personal p ON e.organizacion_id IS NULL AND p.usuario_id=e.propietario_id
        WHERE e.id=p_espacio_id
    ), holder_state AS (
        SELECT h.*,
               CASE WHEN h.modo_anfitrion='empresa' THEN EXISTS (
                   SELECT 1 FROM public.kyb_verifications v
                   WHERE v.organization_id=h.organizacion_id AND v.status IN ('PENDING','IN_REVIEW')
                     AND v.submitted_at<=LEAST(h.holder_due_at, p_at)
                     AND (SELECT count(*) FROM public.kyb_verification_document d
                          WHERE d.verification_id=v.id
                            AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
               ) ELSE EXISTS (
                   SELECT 1 FROM public.anfitrion_cumplimiento_documento d
                   WHERE d.usuario_id=h.propietario_id AND d.status IN ('PENDING','IN_REVIEW')
                     AND d.submitted_at<=LEAST(h.holder_due_at, p_at)
               ) END AS holder_submitted_in_time
        FROM holder h
    ), evaluated AS (
        SELECT hs.*,
               CASE
                   WHEN hs.recorded_status IN ('grace_active','in_review') AND hs.holder_due_at<=p_at
                        AND NOT hs.holder_submitted_in_time THEN 'disabled_missing_docs'
                   ELSE hs.recorded_status
               END AS effective_host_status,
               EXISTS (
                   SELECT 1 FROM public.space_kyb_verification sv
                   WHERE sv.espacio_id=hs.id AND sv.document_kind='property_or_sublease_authorization'
                     AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
               ) AS property_approved,
               (
                   hs.modo_anfitrion='empresa' AND hs.organizacion_id IS NOT NULL
                   AND EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       JOIN public.espacio other_e ON other_e.id=sv.espacio_id
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='property_or_sublease_authorization'
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                         AND other_e.categoria_codigo <> hs.categoria_codigo
                   )
                   AND NOT EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       JOIN public.espacio same_e ON same_e.id=sv.espacio_id
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='property_or_sublease_authorization'
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                         AND same_e.categoria_codigo = hs.categoria_codigo
                   )
                   AND NOT EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='sii_start_activities'
                         AND sv.categoria_codigo=hs.categoria_codigo
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                   )
               ) AS category_sii_required,
               EXISTS (
                   SELECT 1 FROM public.space_kyb_verification sv
                   WHERE sv.organizacion_id=hs.organizacion_id
                     AND sv.document_kind='sii_start_activities'
                     AND sv.categoria_codigo=hs.categoria_codigo
                     AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
               ) AS category_sii_doc_approved
        FROM holder_state hs
    ), deadlines AS (
        SELECT ev.*,
               CASE
                   WHEN ev.effective_host_status='compliant' THEN
                       CASE
                           WHEN ev.estado='borrador' THEN ev.kyb_grace_due_at
                           ELSE COALESCE(ev.kyb_grace_due_at, ev.holder_due_at)
                       END
                   ELSE ev.holder_due_at
               END AS effective_due_at
        FROM evaluated ev
    )
    SELECT d.estado,
           d.effective_host_status,
           d.effective_due_at,
           d.property_approved,
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.espacio_id=d.id AND sv.document_kind='property_or_sublease_authorization'
                 AND sv.status IN ('PENDING','IN_REVIEW') AND sv.submitted_at<=LEAST(d.effective_due_at, p_at)
           ),
           CASE WHEN d.modo_anfitrion='empresa' THEN EXISTS (
               SELECT 1
               FROM (
                   SELECT v.submitted_by
                   FROM public.kyb_verifications v
                   WHERE v.organization_id=d.organizacion_id AND v.status='APPROVED' AND v.reviewed_at<=p_at
                     AND (SELECT count(*) FROM public.kyb_verification_document d2
                          WHERE d2.verification_id=v.id
                            AND d2.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
                   ORDER BY v.reviewed_at DESC,v.id DESC LIMIT 1
               ) verified_company
               JOIN public.organizacion_miembro m ON m.organizacion_id=d.organizacion_id
                 AND m.usuario_id=verified_company.submitted_by AND m.estado='activo' AND m.rol='representante_legal'
               CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,p_at) k
               WHERE k.verified
           ) ELSE COALESCE(d.sii_status='APPROVED', false)
               AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(d.propietario_id,p_at) k WHERE k.verified)
           END,
           d.holder_submitted_in_time,
           d.category_sii_required,
           (NOT d.category_sii_required OR d.category_sii_doc_approved),
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.organizacion_id=d.organizacion_id
                 AND sv.categoria_codigo=d.categoria_codigo
                 AND sv.document_kind='sii_start_activities'
                 AND sv.status IN ('PENDING','IN_REVIEW')
                 AND sv.submitted_at<=LEAST(d.effective_due_at, p_at)
           ),
           d.categoria_codigo
    FROM deadlines d
$$;

CREATE OR REPLACE FUNCTION public.kyb_space_document_state(p_espacio_id uuid, p_at timestamptz)
RETURNS TABLE(
    space_state text,
    host_status text,
    grace_due_at timestamptz,
    property_approved boolean,
    property_submitted_in_time boolean,
    titular_approved boolean,
    holder_submitted_in_time boolean
)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    SELECT space_state, host_status, grace_due_at, property_approved,
           property_submitted_in_time, titular_approved, holder_submitted_in_time
    FROM public.kyb_space_compliance_detail(p_espacio_id, p_at)
$$;

CREATE OR REPLACE FUNCTION public.kyb_space_eligibility(p_espacio_id uuid, p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(can_publish boolean, can_book boolean, space_verified boolean, compliance_status text)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    WITH documents AS (
        SELECT *,
               (host_status='compliant' AND property_approved AND category_sii_approved) AS commercially_complete,
               (grace_due_at IS NULL OR p_at<grace_due_at) AS within_grace
        FROM public.kyb_space_compliance_detail(p_espacio_id, p_at)
    ), eligibility AS (
        SELECT *,
               (host_status<>'disabled_missing_docs' AND space_state<>'eliminada'
                   AND (commercially_complete OR within_grace OR
                        (space_state='activa'
                         AND (host_status='compliant' OR holder_submitted_in_time)
                         AND (property_approved OR property_submitted_in_time)
                         AND (category_sii_approved OR category_sii_submitted_in_time)))) AS allowed
        FROM documents
    )
    SELECT allowed, allowed AND space_state='activa', allowed AND commercially_complete AND titular_approved,
           CASE WHEN NOT allowed THEN 'disabled_missing_docs'
                WHEN commercially_complete THEN 'compliant'
                WHEN host_status='in_review' OR property_submitted_in_time OR category_sii_submitted_in_time THEN 'in_review'
                ELSE 'grace_active' END
    FROM eligibility
$$;

-- D. Runtime grants.
REVOKE ALL ON FUNCTION public.kyb_space_compliance_detail(uuid,timestamptz) FROM PUBLIC;
DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT EXECUTE ON FUNCTION public.kyb_space_compliance_detail(uuid,timestamptz) TO espacigo_runtime;
        GRANT UPDATE (kyb_grace_started_at, kyb_grace_due_at) ON public.espacio TO espacigo_runtime;
    END IF;
END
$runtime$;
