BEGIN;
INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado)
VALUES ('10000000-0000-4000-8000-000000000001','mail-kyb@example.test','mail-kyb@example.test','fixture','activo');
INSERT INTO public.account_identity_profile_revision(id,account_id,key_version,encrypted_data,masked_rut,masked_phone,saved_at)
VALUES ('10000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001','fixture',decode(repeat('ab',40),'hex'),'•••56-0','•••1234','2025-12-01T12:00:00Z');
INSERT INTO public.account_identity_profile(account_id,username,current_revision)
VALUES ('10000000-0000-4000-8000-000000000001','kyb_mail_fixture','10000000-0000-4000-8000-000000000002');
INSERT INTO public.account_identity_verification(id,account_id,profile_revision,state,started_at,updated_at,verified_at,verified_until)
VALUES (gen_random_uuid(),'10000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000002','verified','2025-12-01T12:00:00Z','2025-12-01T12:00:00Z','2025-12-01T12:00:00Z','2027-01-01T12:00:00Z');
INSERT INTO public.anfitrion_cumplimiento_personal(usuario_id,grace_started_at,grace_due_at,compliance_status)
VALUES ('10000000-0000-4000-8000-000000000001','2025-12-31T12:00:00Z','2026-02-28T12:00:00Z','grace_active');
SET ROLE espacigo_runtime;
DO $test$
DECLARE
    fixture_user uuid := '10000000-0000-4000-8000-000000000001';
    started_at timestamptz := '2025-12-31T12:00:00Z';
    due_at timestamptz := '2026-02-28T12:00:00Z';
    queued integer;
BEGIN
    SELECT public.kyb_enqueue_due_reminders(started_at,100) INTO queued;
    IF queued <> 1 THEN RAISE EXCEPTION 'Day zero not queued'; END IF;
    SELECT public.kyb_enqueue_due_reminders(started_at,100) INTO queued;
    IF queued <> 0 THEN RAISE EXCEPTION 'Duplicate day zero notice'; END IF;
    SELECT public.kyb_enqueue_due_reminders('2026-01-30T12:00:00Z',100) INTO queued;
    IF queued <> 0 THEN RAISE EXCEPTION 'First month reminded early'; END IF;
    SELECT public.kyb_enqueue_due_reminders('2026-01-31T12:00:00Z',100) INTO queued;
    IF queued <> 1 THEN RAISE EXCEPTION 'First calendar month not queued'; END IF;
    PERFORM public.kyb_expire_noncompliant_profiles(due_at);
    SELECT public.kyb_enqueue_due_reminders(due_at,100) INTO queued;
    IF queued <> 1 THEN RAISE EXCEPTION 'Deadline notice lost when expiry ran first'; END IF;
    IF (SELECT count(*) FROM public.kyb_compliance_notice_outbox WHERE recipient_id=fixture_user) <> 3 THEN
        RAISE EXCEPTION 'Catch-up did not skip stale intermediate notices';
    END IF;
    UPDATE public.anfitrion_cumplimiento_personal
       SET compliance_status='compliant',sii_status='APPROVED',disabled_at=NULL,disabled_reason=NULL
       WHERE usuario_id=fixture_user;
    UPDATE public.anfitrion_cumplimiento_personal
       SET compliance_status='compliant' WHERE usuario_id=fixture_user;
    IF (SELECT count(*) FROM public.kyb_compliance_notice_outbox WHERE recipient_id=fixture_user AND reminder_stage='reactivated') <> 1 THEN
        RAISE EXCEPTION 'Reactivation must enqueue exactly once per transition';
    END IF;
    IF has_table_privilege(current_user,'public.kyb_compliance_notice_outbox','DELETE') THEN
        RAISE EXCEPTION 'Runtime role can delete outbox evidence';
    END IF;
END;
$test$;
RESET ROLE;
DO $test$
BEGIN
    IF (SELECT count(*) FROM public.notificacion_usuario WHERE usuario_id='10000000-0000-4000-8000-000000000001') <> 4 THEN
        RAISE EXCEPTION 'Missing in-app notices';
    END IF;
END;
$test$;
ROLLBACK;
