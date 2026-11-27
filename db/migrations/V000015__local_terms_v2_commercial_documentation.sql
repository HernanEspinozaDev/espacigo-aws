-- Register the versioned evaluation terms consumed by the local registration flow.
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
        RAISE EXCEPTION 'V000015 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

INSERT INTO public.version_terminos(id,codigo,tipo,hash_sha256,publicada_en)
VALUES('00000000-0000-4000-8000-000000000103','evaluacion-terminos-20261126-v2','terminos',
       'd952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57','2026-11-26 00:00:00+00');
