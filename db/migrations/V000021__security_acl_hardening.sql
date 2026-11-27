-- V000021: Endurecimiento no invasivo de privilegios en funciones SECURITY DEFINER,
-- ledger de migraciones y tablas append-only de auditoría.

-- A. Revocar EXECUTE público sobre las 23 funciones SECURITY DEFINER de V000001.
REVOKE ALL ON FUNCTION public.assign_local_space_history_retention() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.claim_expired_local_verification_files(timestamp with time zone, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.claim_private_media_cleanup(integer, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamp with time zone) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_operation_exists(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_owner_ready() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_target_ids() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.img01b_enforce_gallery_slot() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.local_contract_effective_retention_deadline(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.local_retention_reservation_has_open_obligation(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_delivered_identity_session_outbox() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_identity_declaration_revisions() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_communication(timestamp with time zone, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamp with time zone, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_credential_notices(timestamp with time zone, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_privacy_rows(timestamp with time zone, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.set_verification_evidence_retention_deadline() FROM PUBLIC;

-- B. Evitar concesión implícita a PUBLIC en futuras funciones creadas por el esquema/dueño.
DO $defaults$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_schema_owner')
       AND pg_catalog.pg_has_role(current_user, 'espacigo_schema_owner', 'MEMBER') THEN
        ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;
    ELSE
        ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;
    END IF;
END
$defaults$;

-- C. Endurecimiento de espacigo_runtime y protección de ledger / bitácoras append-only.
DO $acl$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        -- Revocar EXECUTE al rol runtime en triggers, helpers internos y utilidades de ensayo.
        REVOKE EXECUTE ON FUNCTION public.assign_local_space_history_retention() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.img01b_enforce_gallery_slot() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.set_verification_evidence_retention_deadline() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.local_contract_effective_retention_deadline(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.local_retention_reservation_has_open_obligation(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_operation_exists(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_owner_ready() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_target_ids() FROM espacigo_runtime;

        -- Conservar explícitamente EXECUTE en las 15 funciones invocadas por los servicios Go.
        GRANT EXECUTE ON FUNCTION public.claim_expired_local_verification_files(timestamp with time zone, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.claim_private_media_cleanup(integer, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamp with time zone) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_delivered_identity_session_outbox() TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_identity_declaration_revisions() TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_communication(timestamp with time zone, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamp with time zone, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_credential_notices(timestamp with time zone, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_privacy_rows(timestamp with time zone, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) TO espacigo_runtime;

        -- Proteger el ledger de migraciones contra lectura/escritura desde el rol runtime.
        IF to_regclass('public.schema_migrations') IS NOT NULL THEN
            REVOKE ALL ON TABLE public.schema_migrations FROM PUBLIC, espacigo_runtime;
        END IF;

        -- Proteger bitácoras append-only contra UPDATE/DELETE (conservando SELECT/INSERT).
        REVOKE UPDATE, DELETE ON TABLE public.aviso_local_recuperacion_auditoria FROM PUBLIC, espacigo_runtime;
        REVOKE UPDATE, DELETE ON TABLE public.rol_usuario_evento FROM PUBLIC, espacigo_runtime;
        REVOKE UPDATE, DELETE ON TABLE public.document_verification_reviewer_grant_event FROM PUBLIC, espacigo_runtime;
    ELSE
        IF to_regclass('public.schema_migrations') IS NOT NULL THEN
            REVOKE ALL ON TABLE public.schema_migrations FROM PUBLIC;
        END IF;
        REVOKE UPDATE, DELETE ON TABLE public.aviso_local_recuperacion_auditoria FROM PUBLIC;
        REVOKE UPDATE, DELETE ON TABLE public.rol_usuario_evento FROM PUBLIC;
        REVOKE UPDATE, DELETE ON TABLE public.document_verification_reviewer_grant_event FROM PUBLIC;
    END IF;
END
$acl$;
