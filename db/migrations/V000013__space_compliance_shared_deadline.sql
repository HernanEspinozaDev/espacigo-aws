-- KYB-03: enforce the original holder deadline for each published space.
DO $guard$
BEGIN
    IF session_user <> 'espacigo_migrator_login'
       OR current_user <> 'espacigo_schema_owner'
       OR current_schema() <> 'public'
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'V000013 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

-- The deadline still belongs to the holder. Keep the space documents
-- independent and use the actual submission time to protect only on-time reviews.
CREATE FUNCTION public.kyb_space_document_state(p_espacio_id uuid, p_at timestamptz)
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
    WITH holder AS (
        SELECT e.id, e.estado, e.propietario_id, e.organizacion_id, e.modo_anfitrion, p.sii_status,
               CASE WHEN e.modo_anfitrion='empresa' THEN o.grace_due_at ELSE p.grace_due_at END AS due_at,
               CASE WHEN e.modo_anfitrion='empresa' THEN COALESCE(o.compliance_status,'disabled_missing_docs')
                    ELSE COALESCE(p.compliance_status,'grace_active') END AS recorded_status
        FROM public.espacio e
        LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
        LEFT JOIN public.anfitrion_cumplimiento_personal p ON p.usuario_id=e.propietario_id
        WHERE e.id=p_espacio_id
    ), submissions AS (
        SELECT e.*,
               CASE WHEN e.modo_anfitrion='empresa' THEN EXISTS (
                   SELECT 1 FROM public.kyb_verifications v
                   WHERE v.organization_id=e.organizacion_id AND v.status IN ('PENDING','IN_REVIEW')
                     AND v.submitted_at<=LEAST(e.due_at,p_at)
                     AND (SELECT count(*) FROM public.kyb_verification_document d
                          WHERE d.verification_id=v.id
                            AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
               ) ELSE EXISTS (
                   SELECT 1 FROM public.anfitrion_cumplimiento_documento d
                   WHERE d.usuario_id=e.propietario_id AND d.status IN ('PENDING','IN_REVIEW')
                     AND d.submitted_at<=LEAST(e.due_at,p_at)
               ) END AS submitted_in_time
        FROM holder e
    )
    SELECT e.estado,
           CASE
               WHEN e.recorded_status IN ('grace_active','in_review') AND e.due_at<=p_at
                    AND NOT e.submitted_in_time THEN 'disabled_missing_docs'
               ELSE e.recorded_status
           END,
           e.due_at,
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.espacio_id=e.id AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
           ),
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.espacio_id=e.id AND sv.status IN ('PENDING','IN_REVIEW') AND sv.submitted_at<=LEAST(e.due_at,p_at)
           ),
           CASE WHEN e.modo_anfitrion='empresa' THEN EXISTS (
               SELECT 1
               FROM (
                   SELECT v.submitted_by
                   FROM public.kyb_verifications v
                   WHERE v.organization_id=e.organizacion_id AND v.status='APPROVED' AND v.reviewed_at<=p_at
                     AND (SELECT count(*) FROM public.kyb_verification_document d
                          WHERE d.verification_id=v.id
                            AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
                   ORDER BY v.reviewed_at DESC,v.id DESC LIMIT 1
               ) verified_company
               JOIN public.organizacion_miembro m ON m.organizacion_id=e.organizacion_id
                 AND m.usuario_id=verified_company.submitted_by AND m.estado='activo' AND m.rol='representante_legal'
               CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,p_at) k
               WHERE k.verified
           ) ELSE COALESCE(e.sii_status='APPROVED', false)
               AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(e.propietario_id,p_at) k WHERE k.verified)
           END,
           e.submitted_in_time
    FROM submissions e
$$;

CREATE OR REPLACE FUNCTION public.kyb_space_eligibility(p_espacio_id uuid, p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(can_publish boolean, can_book boolean, space_verified boolean, compliance_status text)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    WITH documents AS (
        SELECT *,
               host_status='compliant' AND property_approved AS commercially_complete,
               grace_due_at IS NULL OR p_at<grace_due_at AS within_grace
        FROM public.kyb_space_document_state(p_espacio_id,p_at)
    ), eligibility AS (
        SELECT *,
               host_status<>'disabled_missing_docs' AND space_state<>'eliminada'
                   AND (within_grace OR commercially_complete OR
                        (space_state='activa'
                         AND (host_status='compliant' OR holder_submitted_in_time)
                         AND (property_approved OR property_submitted_in_time))) AS allowed
        FROM documents
    )
    SELECT allowed, allowed AND space_state='activa', allowed AND commercially_complete AND titular_approved,
           CASE WHEN NOT allowed THEN 'disabled_missing_docs'
                WHEN commercially_complete THEN 'compliant'
                WHEN host_status='in_review' OR property_submitted_in_time THEN 'in_review'
                ELSE 'grace_active' END
    FROM eligibility
$$;

CREATE OR REPLACE FUNCTION public.kyb_expire_noncompliant_profiles(p_at timestamptz DEFAULT clock_timestamp())
RETURNS TABLE(usuario_id uuid, organizacion_id uuid)
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    RETURN QUERY
      WITH expired_org AS (
        UPDATE public.organizacion o SET compliance_status='disabled_missing_docs', disabled_at=p_at,
          disabled_reason='Plazo de gracia vencido sin documentación aprobada', updated_at=p_at
        WHERE o.compliance_status IN ('grace_active','in_review') AND o.grace_due_at <= p_at
          AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW')
              AND v.submitted_at <= o.grace_due_at
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
          AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW')
                AND d.submitted_at <= p.grace_due_at)
        RETURNING p.usuario_id
      ), person_events AS (
        INSERT INTO public.host_compliance_event AS event(usuario_id,event_type,reason,occurred_at)
        SELECT person.usuario_id,'disabled_auto','Plazo de gracia vencido sin documentación aprobada',p_at FROM expired_person person
        RETURNING event.usuario_id
      ) SELECT NULL::uuid, id FROM expired_org UNION ALL SELECT person.usuario_id, NULL::uuid FROM expired_person person;
END;
$$;

REVOKE ALL ON FUNCTION public.kyb_space_document_state(uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.kyb_space_document_state(uuid,timestamptz) TO espacigo_runtime;
