-- V000029: Endurecimiento no invasivo de privilegios en funciones SECURITY DEFINER,
-- ledger de migraciones y tablas append-only de auditoría.

DO $guard$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = session_user AND rolsuper
    ) AND (
        session_user <> 'espacigo_migrator_login'
        OR current_user <> 'espacigo_schema_owner'
        OR current_schema() <> 'public'
    ) THEN
        RAISE EXCEPTION 'V000029 requires the pinned migrator/schema-owner identity';
    END IF;
END
$guard$;

-- A. Revocar EXECUTE público sobre las 23 funciones SECURITY DEFINER de V000001.
REVOKE ALL ON FUNCTION public.assign_local_space_history_retention() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.claim_expired_local_verification_files(timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_operation_exists(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_owner_ready() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.gcp06_test_access_target_ids() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.img01b_enforce_gallery_slot() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.local_contract_effective_retention_deadline(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.local_retention_reservation_has_open_obligation(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_delivered_identity_session_outbox() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_identity_declaration_revisions() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_communication(timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_credential_notices(timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_expired_local_privacy_rows(timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.set_verification_evidence_retention_deadline() FROM PUBLIC;

-- B. Evitar concesión implícita a PUBLIC en futuras funciones y proteger ledger y bitácoras para PUBLIC.
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;
REVOKE ALL ON TABLE public.schema_migrations FROM PUBLIC;
REVOKE UPDATE, DELETE ON TABLE public.aviso_local_recuperacion_auditoria FROM PUBLIC;
REVOKE UPDATE, DELETE ON TABLE public.rol_usuario_evento FROM PUBLIC;
REVOKE UPDATE, DELETE ON TABLE public.document_verification_reviewer_grant_event FROM PUBLIC;

-- C. Endurecimiento de espacigo_runtime y protección de ledger / bitácoras append-only.
DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        REVOKE EXECUTE ON FUNCTION public.assign_local_space_history_retention() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.img01b_enforce_gallery_slot() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.set_verification_evidence_retention_deadline() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.local_contract_effective_retention_deadline(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.local_retention_reservation_has_open_obligation(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_operation_exists(uuid) FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_owner_ready() FROM espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.gcp06_test_access_target_ids() FROM espacigo_runtime;

        GRANT EXECUTE ON FUNCTION public.claim_expired_local_verification_files(timestamptz, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_delivered_identity_session_outbox() TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_identity_declaration_revisions() TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_communication(timestamptz, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamptz, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_credential_notices(timestamptz, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_expired_local_privacy_rows(timestamptz, integer) TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) TO espacigo_runtime;

        REVOKE ALL ON TABLE public.schema_migrations FROM espacigo_runtime;
        REVOKE UPDATE, DELETE ON TABLE public.aviso_local_recuperacion_auditoria FROM espacigo_runtime;
        REVOKE UPDATE, DELETE ON TABLE public.rol_usuario_evento FROM espacigo_runtime;
        REVOKE UPDATE, DELETE ON TABLE public.document_verification_reviewer_grant_event FROM espacigo_runtime;
    END IF;
END
$runtime$;

-- D. El worker de limpieza utiliza un rol efectivo dedicado y sólo los permisos
-- de columna y las seis funciones que exige su ciclo actual.
DO $cleanup_worker$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_privacy_cleanup_worker') THEN
        IF NOT pg_catalog.has_schema_privilege('espacigo_privacy_cleanup_worker', 'public', 'USAGE') THEN
            RAISE EXCEPTION 'V000029 requires USAGE on schema public for espacigo_privacy_cleanup_worker';
        END IF;

        GRANT SELECT (id, solicitud_id, estado, iniciada_en)
            ON public.ejecucion_baja_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (ejecucion_id, evidencia_id, completada_en, disponible_en, intentos,
                      ultimo_codigo_error, claim_owner, claim_hasta, claim_generacion)
            ON public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE (completada_en, disponible_en, intentos, ultimo_codigo_error,
                      claim_owner, claim_hasta, claim_generacion)
            ON public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (id) ON public.verificacion_evidencia_sintetica TO espacigo_privacy_cleanup_worker;
        GRANT DELETE ON public.verificacion_evidencia_sintetica TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (archivo_id) ON public.operacion_arriendo_archivo_candidato_local TO espacigo_privacy_cleanup_worker;
        GRANT DELETE ON public.operacion_arriendo_archivo_candidato_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (id, retirar_archivo_en, archivo_retirado_en)
            ON public.operacion_arriendo_evidencia_ensayo_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE (retirar_archivo_en, archivo_retirado_en)
            ON public.operacion_arriendo_evidencia_ensayo_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (id, estado) ON public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE (archivo_id, mime_type, sha256, size_bytes, limpia_en, proximo_intento_en, ultimo_codigo_error)
            ON public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (id, estado) ON public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE (archivo_id, limpia_en, proximo_intento_en, ultimo_codigo_error)
            ON public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;

        GRANT SELECT (id, propietario_id, estado, fencing_token, lease_until, updated_at)
            ON public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (media_id, variant, provider, bucket_name, object_key, generation, mime_type, size_bytes, sha256, width, height)
            ON public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT (media_id, variant, object_key, generation, write_complete)
            ON public.media_variant_intent TO espacigo_privacy_cleanup_worker;

        GRANT EXECUTE ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT EXECUTE ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT EXECUTE ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT EXECUTE ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT EXECUTE ON FUNCTION public.purge_expired_identity_declaration_revisions() TO espacigo_privacy_cleanup_worker;
        GRANT EXECUTE ON FUNCTION public.purge_delivered_identity_session_outbox() TO espacigo_privacy_cleanup_worker;
    END IF;
END
$cleanup_worker$;
