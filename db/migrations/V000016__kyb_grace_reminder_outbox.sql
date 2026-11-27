-- KYB-04: idempotent calendar-based reminder scheduling and durable email intents.
DO $guard$
BEGIN
    IF session_user <> 'espacigo_migrator_login'
       OR current_user <> 'espacigo_schema_owner'
       OR current_schema() <> 'public'
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'V000016 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END;
$guard$;

CREATE TABLE public.kyb_compliance_notice_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_type varchar(16) NOT NULL CHECK (profile_type IN ('personal','organization')),
    profile_id uuid NOT NULL,
    recipient_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    reminder_stage varchar(24) NOT NULL CHECK (reminder_stage IN ('day0','month1','day15','day7','day1','due','reactivated')),
    source_transition_id uuid,
    available_at timestamptz NOT NULL,
    state varchar(16) NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending','processing','delivered','terminal','cancelled')),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 8),
    claimed_at timestamptz,
    delivered_at timestamptz,
    terminal_at timestamptz,
    cancelled_at timestamptz,
    last_error_code varchar(32) CHECK (last_error_code IS NULL OR last_error_code IN ('recipient_unavailable','mail_delivery_failed','attempt_limit_reached')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((reminder_stage='reactivated' AND source_transition_id IS NOT NULL) OR (reminder_stage<>'reactivated' AND source_transition_id IS NULL)),
    CHECK (created_at <= available_at),
    CHECK ((state = 'processing') = (claimed_at IS NOT NULL)),
    CHECK ((state = 'delivered') = (delivered_at IS NOT NULL)),
    CHECK ((state = 'terminal') = (terminal_at IS NOT NULL)),
    CHECK ((state = 'cancelled') = (cancelled_at IS NOT NULL)),
    CHECK (num_nonnulls(delivered_at,terminal_at,cancelled_at) <= 1)
);
CREATE UNIQUE INDEX kyb_compliance_notice_stage_uq ON public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage) WHERE reminder_stage<>'reactivated';
CREATE UNIQUE INDEX kyb_compliance_notice_transition_uq ON public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,source_transition_id) WHERE reminder_stage='reactivated';
CREATE INDEX kyb_compliance_notice_pending_idx
    ON public.kyb_compliance_notice_outbox(available_at,created_at,id)
    WHERE state IN ('pending','processing');
CREATE INDEX kyb_compliance_notice_profile_idx
    ON public.kyb_compliance_notice_outbox(profile_type,profile_id,created_at DESC);

CREATE FUNCTION public.kyb_next_reminder(p_started_at timestamptz,p_due_at timestamptz,p_last_stage text,p_at timestamptz)
RETURNS TABLE(reminder_stage text, available_at timestamptz)
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    SELECT candidate.stage,candidate.when_at
    FROM (
        VALUES
            ('day0'::text,0,p_started_at),
            ('month1'::text,1,(((p_started_at AT TIME ZONE 'America/Santiago') + interval '1 month') AT TIME ZONE 'America/Santiago')),
            ('day15'::text,2,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '15 days') AT TIME ZONE 'America/Santiago')),
            ('day7'::text,3,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '7 days') AT TIME ZONE 'America/Santiago')),
            ('day1'::text,4,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '1 day') AT TIME ZONE 'America/Santiago')),
            ('due'::text,5,p_due_at)
    ) AS candidate(stage,ordinal,when_at)
    WHERE candidate.ordinal > CASE
        WHEN p_last_stage IS NULL THEN -1
        WHEN p_last_stage='day0' THEN 0
        WHEN p_last_stage='month1' THEN 1
        WHEN p_last_stage='day15' THEN 2
        WHEN p_last_stage='day7' THEN 3
        WHEN p_last_stage='day1' THEN 4
        WHEN p_last_stage='due' THEN 5
        ELSE 5 END
      AND candidate.when_at<=p_at AND p_started_at IS NOT NULL AND p_due_at IS NOT NULL
    ORDER BY candidate.ordinal DESC LIMIT 1
$$;

CREATE FUNCTION public.kyb_enqueue_due_reminders(p_at timestamptz DEFAULT clock_timestamp(),p_limit integer DEFAULT 100)
RETURNS integer
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE
    profile record;
    recipient record;
    message text;
    rows_enqueued integer := 0;
BEGIN
    IF p_at IS NULL OR p_limit < 1 OR p_limit > 500 THEN
        RAISE EXCEPTION 'invalid KYB reminder batch arguments' USING ERRCODE = '22023';
    END IF;

    FOR profile IN
        SELECT p.usuario_id AS profile_id,p.grace_started_at,p.grace_due_at,p.compliance_status,n.reminder_stage,n.available_at
        FROM public.anfitrion_cumplimiento_personal p
        CROSS JOIN LATERAL public.kyb_next_reminder(p.grace_started_at,p.grace_due_at,p.last_reminder_stage,p_at) n
        WHERE (p.compliance_status IN ('grace_active','in_review')
               OR (p.compliance_status='disabled_missing_docs' AND p.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND p.last_reminder_stage IS DISTINCT FROM 'due'))
          AND EXISTS (SELECT 1 FROM public.usuario u WHERE u.id=p.usuario_id AND u.estado='activo')
          AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(p.usuario_id,p_at) k WHERE k.verified)
          AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d
              WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW') AND d.submitted_at<=p.grace_due_at)
        ORDER BY n.available_at,p.usuario_id
        LIMIT p_limit FOR UPDATE OF p SKIP LOCKED
    LOOP
        message := CASE profile.reminder_stage
            WHEN 'day0' THEN 'Comenzó tu plazo documental de dos meses. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Adjunta tu certificado de Inicio de Actividades SII en EspaciGo.'
            WHEN 'month1' THEN 'Tu plazo documental continúa vigente. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Revisa tu expediente SII en EspaciGo.'
            WHEN 'day15' THEN 'Quedan 15 días para completar la documentación SII. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day7' THEN 'Quedan 7 días para completar la documentación SII. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day1' THEN 'Tu plazo documental vence mañana. Completa la documentación SII para evitar la inhabilitación para publicar y administrar espacios.'
            WHEN 'reactivated' THEN 'Tu perfil comercial volvió a estar habilitado. Ya puedes administrar y publicar tus espacios en EspaciGo.'
            ELSE CASE WHEN profile.compliance_status='disabled_missing_docs' THEN 'El plazo documental venció y tu perfil quedó inhabilitado para publicar y administrar espacios. Regulariza el certificado SII en EspaciGo.' ELSE 'Hoy vence tu plazo documental de dos meses. Regulariza el certificado SII en EspaciGo para mantener habilitado tu perfil comercial.' END
        END;
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(),profile.profile_id,'kyb_grace_'||profile.reminder_stage,'Documentación comercial pendiente',message,'/anfitrion/kyb',profile.profile_id,p_at)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
        INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,available_at,created_at)
        VALUES('personal',profile.profile_id,profile.profile_id,profile.reminder_stage,p_at,p_at)
        ON CONFLICT(profile_type,profile_id,recipient_id,reminder_stage) WHERE reminder_stage<>'reactivated' DO NOTHING;
        INSERT INTO public.host_compliance_event(usuario_id,event_type,reminder_stage,occurred_at)
        VALUES(profile.profile_id,'reminder_sent',profile.reminder_stage,p_at);
        UPDATE public.anfitrion_cumplimiento_personal SET last_reminder_stage=profile.reminder_stage,updated_at=p_at
        WHERE usuario_id=profile.profile_id;
        rows_enqueued := rows_enqueued + 1;
    END LOOP;

    FOR profile IN
        SELECT o.id AS profile_id,o.grace_started_at,o.grace_due_at,o.compliance_status,n.reminder_stage,n.available_at
        FROM public.organizacion o
        CROSS JOIN LATERAL public.kyb_next_reminder(o.grace_started_at,o.grace_due_at,o.last_reminder_stage,p_at) n
        WHERE (o.compliance_status IN ('grace_active','in_review')
               OR (o.compliance_status='disabled_missing_docs' AND o.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND o.last_reminder_stage IS DISTINCT FROM 'due'))
          AND EXISTS (SELECT 1 FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
              WHERE m.organizacion_id=o.id AND m.estado='activo' AND m.rol IN ('representante_legal','administrador') AND u.estado='activo'
                AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,p_at) k WHERE k.verified))
          AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v
              WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW') AND v.submitted_at<=o.grace_due_at
                AND (SELECT count(DISTINCT d.document_kind) FROM public.kyb_verification_document d
                     WHERE d.verification_id=v.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3)
        ORDER BY n.available_at,o.id
        LIMIT p_limit FOR UPDATE OF o SKIP LOCKED
    LOOP
        message := CASE profile.reminder_stage
            WHEN 'day0' THEN 'Comenzó el plazo documental de dos meses de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Completa SII y KYB en EspaciGo.'
            WHEN 'month1' THEN 'El plazo documental de tu empresa continúa vigente. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Revisa el expediente SII y KYB en EspaciGo.'
            WHEN 'day15' THEN 'Quedan 15 días para completar el expediente SII y KYB de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day7' THEN 'Quedan 7 días para completar el expediente SII y KYB de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day1' THEN 'El plazo documental de tu empresa vence mañana. Completa e-RUT, SII y poderes para evitar la inhabilitación comercial.'
            WHEN 'reactivated' THEN 'El perfil comercial de tu empresa volvió a estar habilitado para administrar y publicar espacios en EspaciGo.'
            ELSE CASE WHEN profile.compliance_status='disabled_missing_docs' THEN 'El plazo documental venció y el perfil comercial de tu empresa quedó inhabilitado. Regulariza SII y KYB en EspaciGo.' ELSE 'Hoy vence el plazo documental de dos meses de tu empresa. Regulariza SII y KYB en EspaciGo para mantener habilitado el perfil comercial.' END
        END;
        FOR recipient IN
            SELECT m.usuario_id FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
            WHERE m.organizacion_id=profile.profile_id AND m.estado='activo' AND u.estado='activo' AND m.rol IN ('representante_legal','administrador')
              AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,p_at) k WHERE k.verified)
            ORDER BY m.usuario_id
        LOOP
            INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
            VALUES(gen_random_uuid(),recipient.usuario_id,'kyb_grace_'||profile.reminder_stage,'Documentación comercial pendiente',message,'/anfitrion/kyb',profile.profile_id,p_at)
            ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
            INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,available_at,created_at)
            VALUES('organization',profile.profile_id,recipient.usuario_id,profile.reminder_stage,p_at,p_at)
            ON CONFLICT(profile_type,profile_id,recipient_id,reminder_stage) WHERE reminder_stage<>'reactivated' DO NOTHING;
        END LOOP;
        INSERT INTO public.host_compliance_event(organizacion_id,event_type,reminder_stage,occurred_at)
        VALUES(profile.profile_id,'reminder_sent',profile.reminder_stage,p_at);
        UPDATE public.organizacion SET last_reminder_stage=profile.reminder_stage,updated_at=p_at
        WHERE id=profile.profile_id;
        rows_enqueued := rows_enqueued + 1;
    END LOOP;
    RETURN rows_enqueued;
END;
$$;


CREATE FUNCTION public.kyb_personal_reactivation_notice() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE
    transition_id uuid := gen_random_uuid();
    at_time timestamptz := clock_timestamp();
BEGIN
    INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
    VALUES(gen_random_uuid(),NEW.usuario_id,'kyb_reactivated_'||transition_id::text,'Perfil comercial habilitado',
        'Tu perfil comercial volvió a estar habilitado. Ya puedes administrar y publicar tus espacios en EspaciGo.',
        '/anfitrion/kyb',NEW.usuario_id,at_time)
    ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
    INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,source_transition_id,available_at,created_at)
    VALUES('personal',NEW.usuario_id,NEW.usuario_id,'reactivated',transition_id,at_time,at_time)
    ON CONFLICT(profile_type,profile_id,recipient_id,source_transition_id) WHERE reminder_stage='reactivated' DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER anfitrion_personal_reactivation_notice
    AFTER UPDATE OF compliance_status ON public.anfitrion_cumplimiento_personal
    FOR EACH ROW WHEN (OLD.compliance_status='disabled_missing_docs' AND NEW.compliance_status='compliant')
    EXECUTE FUNCTION public.kyb_personal_reactivation_notice();

CREATE FUNCTION public.kyb_organization_reactivation_notice() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE
    member record;
    transition_id uuid := gen_random_uuid();
    at_time timestamptz := clock_timestamp();
BEGIN
    FOR member IN
        SELECT m.usuario_id FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
        WHERE m.organizacion_id=NEW.id AND m.estado='activo' AND u.estado='activo'
          AND m.rol IN ('representante_legal','administrador')
          AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,at_time) k WHERE k.verified)
        ORDER BY m.usuario_id
    LOOP
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(),member.usuario_id,'kyb_reactivated_'||transition_id::text,'Perfil comercial habilitado',
            'El perfil comercial de tu empresa volvió a estar habilitado para administrar y publicar espacios en EspaciGo.',
            '/anfitrion/kyb',NEW.id,at_time)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
        INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,source_transition_id,available_at,created_at)
        VALUES('organization',NEW.id,member.usuario_id,'reactivated',transition_id,at_time,at_time)
        ON CONFLICT(profile_type,profile_id,recipient_id,source_transition_id) WHERE reminder_stage='reactivated' DO NOTHING;
    END LOOP;
    RETURN NEW;
END;
$$;
CREATE TRIGGER organizacion_reactivation_notice
    AFTER UPDATE OF compliance_status ON public.organizacion
    FOR EACH ROW WHEN (OLD.compliance_status='disabled_missing_docs' AND NEW.compliance_status='compliant')
    EXECUTE FUNCTION public.kyb_organization_reactivation_notice();

CREATE OR REPLACE FUNCTION public.kyb_start_space_grace() RETURNS trigger
SECURITY DEFINER
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE started timestamptz := clock_timestamp(); due timestamptz; existing_started timestamptz; started_user uuid; published_by uuid;
BEGIN
    SELECT h.actor_id INTO published_by
    FROM public.espacio_publicacion_historial_local h
    WHERE h.espacio_id=NEW.id AND h.estado_nuevo='activa'
      AND h.ocurrida_en=transaction_timestamp()
    ORDER BY h.id DESC LIMIT 1;
    published_by := COALESCE(published_by, NEW.propietario_id);
    due := ((started AT TIME ZONE 'America/Santiago' + interval '2 months') AT TIME ZONE 'America/Santiago');
    IF NEW.modo_anfitrion = 'empresa' THEN
        SELECT grace_started_at INTO existing_started FROM public.organizacion WHERE id=NEW.organizacion_id FOR UPDATE;
        IF existing_started IS NULL THEN
            UPDATE public.organizacion SET grace_started_at=started, grace_due_at=due, updated_at=clock_timestamp() WHERE id=NEW.organizacion_id;
            INSERT INTO public.host_compliance_event(organizacion_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.organizacion_id, published_by, 'grace_started', started);
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
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION public.kyb_space_document_request_notice() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
BEGIN
    IF EXISTS (
        SELECT FROM public.kyb_space_eligibility(NEW.espacio_id, NEW.solicitado_en) e
        WHERE NOT e.space_verified
    ) THEN
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(), NEW.actor_usuario_id, 'kyb_space_document_request', 'Documentación de la publicación',
            'Revisa el respaldo de este espacio y la documentación de su titular. Fecha límite compartida: '
            || to_char(NEW.vence_en AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.',
            '/anfitrion/kyb', NEW.espacio_id, NEW.solicitado_en)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER espacio_document_request_notice AFTER INSERT ON public.espacio_solicitud_documental
    FOR EACH ROW EXECUTE FUNCTION public.kyb_space_document_request_notice();
REVOKE ALL ON FUNCTION public.kyb_space_document_request_notice() FROM PUBLIC;

REVOKE ALL ON TABLE public.kyb_compliance_notice_outbox FROM PUBLIC;
REVOKE ALL ON FUNCTION public.kyb_next_reminder(timestamptz,timestamptz,text,timestamptz),
    public.kyb_enqueue_due_reminders(timestamptz,integer),
    public.kyb_personal_reactivation_notice(), public.kyb_organization_reactivation_notice() FROM PUBLIC;
DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        REVOKE ALL ON public.kyb_compliance_notice_outbox FROM espacigo_runtime;
        GRANT SELECT,INSERT,UPDATE ON public.kyb_compliance_notice_outbox TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.kyb_next_reminder(timestamptz,timestamptz,text,timestamptz),
            public.kyb_enqueue_due_reminders(timestamptz,integer),
            public.kyb_personal_reactivation_notice(), public.kyb_organization_reactivation_notice() TO espacigo_runtime;
        GRANT INSERT ON public.notificacion_usuario TO espacigo_runtime;
        GRANT UPDATE (last_reminder_stage,updated_at) ON public.anfitrion_cumplimiento_personal TO espacigo_runtime;
        GRANT UPDATE (last_reminder_stage,updated_at) ON public.organizacion TO espacigo_runtime;
    END IF;
END;
$runtime$;
