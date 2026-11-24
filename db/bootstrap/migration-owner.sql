-- Grant schema public USAGE and CREATE to espacigo_schema_owner when the role exists.
-- Run as the database administrator.
BEGIN;
SELECT pg_catalog.pg_advisory_xact_lock(1163087943, 1296648018);

DO $owner$
BEGIN
    IF session_user <> 'espacigo_admin' OR current_user <> session_user THEN
        RAISE EXCEPTION 'Unexpected migration-owner administrator';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_schema_owner'
    ) THEN
        RAISE EXCEPTION 'Role espacigo_schema_owner does not exist';
    END IF;
END
$owner$;

GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner;
COMMIT;
