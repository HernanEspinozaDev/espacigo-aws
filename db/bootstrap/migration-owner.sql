-- Run once at the canonical V100 baseline, before enabling routine delivery.
-- Run as the database administrator. This transaction also works on later baselines.
BEGIN;
SELECT pg_catalog.pg_advisory_xact_lock(1163087943, 1296648018);

DO $owner$
BEGIN
    IF current_database() NOT IN ('espacigo_prod', 'espacigo_aws_local')
       OR session_user <> 'espacigo_admin' OR current_user <> session_user THEN
        RAISE EXCEPTION 'Unexpected target or migration-owner administrator';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = 100 AND name = 'V000100__early_rental_local_payment.sql'
          AND checksum = '24554d7716ce5bcc8d5e3f50cbce2d2a39efaf3314132eccdad311d683644ea6'
    ) THEN
        RAISE EXCEPTION 'Routine migration ownership requires the canonical V100 baseline';
    END IF;

    IF (SELECT count(*) FROM pg_catalog.pg_roles
        WHERE rolname IN ('espacigo_migrator_login', 'espacigo_schema_owner')
          AND rolcanlogin = (rolname = 'espacigo_migrator_login')
          AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication
                   OR rolbypassrls OR rolinherit)) <> 2 THEN
        RAISE EXCEPTION 'Unexpected migrator or schema-owner attributes';
    END IF;

    IF (SELECT count(*) FROM pg_catalog.pg_auth_members
        WHERE member = 'espacigo_migrator_login'::regrole) <> 1
       OR NOT EXISTS (
           SELECT 1 FROM pg_catalog.pg_auth_members
           WHERE member = 'espacigo_migrator_login'::regrole
             AND roleid = 'espacigo_schema_owner'::regrole
             AND set_option AND NOT inherit_option AND NOT admin_option
       ) THEN
        RAISE EXCEPTION 'Unexpected migration role membership';
    END IF;

    IF (SELECT count(*) FROM pg_catalog.pg_class c
        WHERE c.oid IN ('public.schema_migrations'::regclass, 'public.ocupacion'::regclass)
          AND c.relowner = 'espacigo_schema_owner'::regrole) <> 2 THEN
        RAISE EXCEPTION 'The migration owner must own the ledger and occupancy table';
    END IF;

    IF pg_catalog.has_schema_privilege('espacigo_api_login', 'public', 'CREATE')
       OR pg_catalog.has_schema_privilege('espacigo_runtime', 'public', 'CREATE')
       OR pg_catalog.has_schema_privilege('espacigo_migrator_login', 'public', 'CREATE') THEN
        RAISE EXCEPTION 'Unexpected schema CREATE outside the migration owner';
    END IF;
END
$owner$;

GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner;
COMMIT;
