-- Guarded, one-object repair for the historical owner of public.sesion.
-- First obtain expected_owner from the read-only RDS ownership inspector.
-- Run manually with psql -v ON_ERROR_STOP=1 -v expected_owner=<observed-owner>.
-- This file is a template only: do not run it in local or production without
-- the approved deployment runbook and a fresh read-only inspection.
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_catalog.pg_advisory_xact_lock(1163087943, 1296648018);
SELECT pg_catalog.set_config('espacigo.expected_owner', :'expected_owner', true);

DO $prepare$
DECLARE
    observed_owner name;
    expected_owner name := current_setting('espacigo.expected_owner')::name;
    acl_before jsonb;
    acl_after jsonb;
BEGIN
    IF current_database() <> 'espacigo_prod'
       OR session_user <> 'espacigo_admin' OR current_user <> session_user THEN
        RAISE EXCEPTION 'Unexpected RDS session for V104 ownership preparation';
    END IF;
    IF expected_owner IS NULL OR expected_owner = ''
       OR expected_owner IN ('espacigo_schema_owner', 'espacigo_migrator_login') THEN
        RAISE EXCEPTION 'Supply the exact reviewed historical owner from read-only inspection';
    END IF;
    IF (SELECT max(version) FROM public.schema_migrations) IS DISTINCT FROM 103
       OR NOT EXISTS (
           SELECT 1 FROM public.schema_migrations
           WHERE version = 103 AND name = 'V000103__account_phone_verification.sql'
             AND checksum = '64e6221ba83dd77d8a27b48cbeacfd0d19ea540f1ed0d920492918fe189b2f6d'
       ) THEN
        RAISE EXCEPTION 'V104 ownership preparation requires the verified V103 ledger';
    END IF;
    IF NOT pg_catalog.has_schema_privilege('espacigo_schema_owner', 'public', 'CREATE')
       OR EXISTS (
           SELECT 1 FROM pg_catalog.pg_auth_members
           WHERE member = 'espacigo_admin'::regrole
             AND roleid = 'espacigo_schema_owner'::regrole
       ) THEN
        RAISE EXCEPTION 'Unexpected migration authority before ownership preparation';
    END IF;

    LOCK TABLE public.sesion IN ACCESS EXCLUSIVE MODE;
    SELECT pg_catalog.pg_get_userbyid(c.relowner)::name INTO STRICT observed_owner
    FROM pg_catalog.pg_class c
    JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relname = 'sesion' AND c.relkind IN ('r', 'p');
    IF observed_owner IS DISTINCT FROM expected_owner THEN
        RAISE EXCEPTION 'Owner changed since inspection; expected %, found %', expected_owner, observed_owner;
    END IF;

    SELECT COALESCE(jsonb_agg(
        jsonb_build_array(c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable)
        ORDER BY c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable
    ), '[]'::jsonb) INTO acl_before
    FROM pg_catalog.pg_class c
    CROSS JOIN LATERAL (
        SELECT 0::smallint AS column_number, c.relacl AS acl
        UNION ALL
        SELECT attnum, attacl FROM pg_catalog.pg_attribute
        WHERE attrelid = c.oid AND attnum > 0 AND NOT attisdropped
    ) entry
    CROSS JOIN LATERAL pg_catalog.aclexplode(entry.acl) a
    WHERE c.oid = 'public.sesion'::regclass
      AND a.grantee NOT IN ('espacigo_admin'::regrole, 'espacigo_schema_owner'::regrole, expected_owner::regrole);

    GRANT espacigo_schema_owner TO espacigo_admin WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
    ALTER TABLE public.sesion OWNER TO espacigo_schema_owner;
    REVOKE espacigo_schema_owner FROM espacigo_admin;

    SELECT COALESCE(jsonb_agg(
        jsonb_build_array(c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable)
        ORDER BY c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable
    ), '[]'::jsonb) INTO acl_after
    FROM pg_catalog.pg_class c
    CROSS JOIN LATERAL (
        SELECT 0::smallint AS column_number, c.relacl AS acl
        UNION ALL
        SELECT attnum, attacl FROM pg_catalog.pg_attribute
        WHERE attrelid = c.oid AND attnum > 0 AND NOT attisdropped
    ) entry
    CROSS JOIN LATERAL pg_catalog.aclexplode(entry.acl) a
    WHERE c.oid = 'public.sesion'::regclass
      AND a.grantee NOT IN ('espacigo_admin'::regrole, 'espacigo_schema_owner'::regrole, expected_owner::regrole);

    IF acl_after IS DISTINCT FROM acl_before
       OR (SELECT relowner FROM pg_catalog.pg_class WHERE oid = 'public.sesion'::regclass)
          <> 'espacigo_schema_owner'::regrole
       OR EXISTS (
           SELECT 1 FROM pg_catalog.pg_auth_members
           WHERE member = 'espacigo_admin'::regrole
             AND roleid = 'espacigo_schema_owner'::regrole
       ) THEN
        RAISE EXCEPTION 'Ownership transfer changed ACLs or left temporary role membership';
    END IF;
END
$prepare$;
COMMIT;
