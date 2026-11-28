-- Preparación puntual de RDS. Ejecutar como el administrador de la base con V28 aplicada.
-- El dueño de objetos que ejecuta V29 carece de opción de delegar USAGE en el esquema.
BEGIN;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '10s';

DO $guard$
BEGIN
    IF pg_catalog.current_database() <> 'espacigo_prod'
       OR session_user <> 'espacigo_admin'
       OR current_user <> session_user
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'USAGE WITH GRANT OPTION') THEN
        RAISE EXCEPTION 'Expected the EspaciGo RDS database administrator with permission to grant schema USAGE';
    END IF;
    IF (SELECT count(*) FROM public.schema_migrations) <> 28
       OR (SELECT max(version) FROM public.schema_migrations) <> 28
       OR NOT EXISTS (
           SELECT 1 FROM public.schema_migrations
           WHERE version = 28 AND name = 'V000028__privacy_incidents_and_legal_holds.sql'
             AND checksum = 'd21142c19be3e0cf0f32169cd5f7b384f18130c4151a65cc216138080569be59'
       ) THEN
        RAISE EXCEPTION 'Expected the verified V28 migration history';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles
        WHERE rolname = 'espacigo_privacy_cleanup_worker' AND NOT
            (rolcanlogin OR rolsuper OR rolcreaterole OR rolcreatedb OR rolinherit OR rolreplication OR rolbypassrls)
    ) OR pg_catalog.has_schema_privilege('espacigo_privacy_cleanup_worker', 'public', 'CREATE') THEN
        RAISE EXCEPTION 'Expected a restricted privacy cleanup worker role';
    END IF;
END
$guard$;

GRANT USAGE ON SCHEMA public TO espacigo_privacy_cleanup_worker;

DO $readback$
BEGIN
    IF NOT pg_catalog.has_schema_privilege('espacigo_privacy_cleanup_worker', 'public', 'USAGE')
       OR pg_catalog.has_schema_privilege('espacigo_privacy_cleanup_worker', 'public', 'USAGE WITH GRANT OPTION') THEN
        RAISE EXCEPTION 'Expected non-delegable USAGE for the privacy cleanup worker';
    END IF;
END
$readback$;
COMMIT;
