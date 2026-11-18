-- Execute before bootstrap using the administrator over PostgreSQL verify-full.
SELECT json_build_object(
    'database', pg_catalog.current_database(),
    'session_user', session_user,
    'server_version', pg_catalog.current_setting('server_version'),
    'database_oid', (SELECT oid FROM pg_catalog.pg_database WHERE datname = pg_catalog.current_database()),
    'transaction_read_only', pg_catalog.current_setting('transaction_read_only') = 'on',
    'tls', COALESCE((SELECT ssl FROM pg_catalog.pg_stat_ssl WHERE pid = pg_catalog.pg_backend_pid()), false),
    'tls_version', (SELECT version FROM pg_catalog.pg_stat_ssl WHERE pid = pg_catalog.pg_backend_pid()),
    'force_tls', pg_catalog.current_setting('rds.force_ssl', true),
    'can_read_control_identity', pg_catalog.has_function_privilege(
        current_user, 'pg_catalog.pg_control_system()', 'EXECUTE'
    ),
    'extensions', (
        SELECT json_object_agg(name, default_version ORDER BY name)
        FROM pg_catalog.pg_available_extensions
        WHERE name IN ('btree_gist', 'postgis', 'unaccent')
    ),
    'public_relations', (
        SELECT count(*) FROM pg_catalog.pg_class WHERE relnamespace = 'public'::pg_catalog.regnamespace
          AND relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
    ),
    'has_migration_ledger', pg_catalog.to_regclass('public.schema_migrations') IS NOT NULL,
    'application_roles', (
        SELECT count(*) FROM pg_catalog.pg_roles
        WHERE rolname IN ('espacigo_api_login', 'espacigo_migrator_login', 'espacigo_schema_owner', 'espacigo_runtime')
    )
);
