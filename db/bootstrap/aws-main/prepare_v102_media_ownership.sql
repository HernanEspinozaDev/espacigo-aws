BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_catalog.pg_advisory_xact_lock(1163087943, 1296648018);

DO $prepare$
DECLARE
    relation regclass;
    owner_name name;
    transfer_needed boolean := false;
    existing_acl jsonb;
    observed_acl jsonb;
BEGIN
    IF current_database() <> 'espacigo_prod'
       OR session_user <> 'espacigo_admin' OR current_user <> session_user THEN
        RAISE EXCEPTION 'Unexpected RDS media ownership target';
    END IF;
    IF (SELECT max(version) FROM public.schema_migrations) IS DISTINCT FROM 101
       OR NOT EXISTS (
           SELECT 1 FROM public.schema_migrations
           WHERE version = 101 AND name = 'V000101__publish_production_consent_documents.sql'
             AND checksum = '5f970f3830f701409db3b133aa4231e59fd1dce846754888adc2a2961ac64109'
       ) THEN
        RAISE EXCEPTION 'Media ownership preparation requires V101';
    END IF;
    IF NOT pg_catalog.has_schema_privilege('espacigo_schema_owner', 'public', 'CREATE')
       OR EXISTS (
           SELECT 1 FROM pg_catalog.pg_auth_members
           WHERE member = 'espacigo_admin'::regrole
             AND roleid = 'espacigo_schema_owner'::regrole
       ) THEN
        RAISE EXCEPTION 'Unexpected migration authority before media ownership preparation';
    END IF;

    FOREACH relation IN ARRAY ARRAY['public.media_variant'::regclass, 'public.media_variant_intent'::regclass]
    LOOP
        SELECT pg_catalog.pg_get_userbyid(relowner) INTO STRICT owner_name
        FROM pg_catalog.pg_class WHERE oid = relation;
        IF owner_name NOT IN ('espacigo_admin', 'espacigo_schema_owner') THEN
            RAISE EXCEPTION 'Unexpected owner for %', relation;
        END IF;
        transfer_needed := transfer_needed OR owner_name = 'espacigo_admin';
    END LOOP;

    LOCK TABLE public.media_variant, public.media_variant_intent IN ACCESS EXCLUSIVE MODE;
    FOR phase IN 1..2 LOOP
        SELECT COALESCE(jsonb_agg(
            jsonb_build_array(c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable)
            ORDER BY c.oid, entry.column_number, a.grantee, a.privilege_type, a.is_grantable
        ), '[]'::jsonb) INTO observed_acl
        FROM pg_catalog.pg_class c
        CROSS JOIN LATERAL (
            SELECT 0::smallint AS column_number, c.relacl AS acl
            UNION ALL
            SELECT attnum, attacl FROM pg_catalog.pg_attribute
            WHERE attrelid = c.oid AND attnum > 0 AND NOT attisdropped
        ) entry
        CROSS JOIN LATERAL pg_catalog.aclexplode(entry.acl) a
        WHERE c.oid IN ('public.media_variant'::regclass, 'public.media_variant_intent'::regclass)
          AND a.grantee NOT IN ('espacigo_admin'::regrole, 'espacigo_schema_owner'::regrole);

        IF phase = 1 THEN
            existing_acl := observed_acl;
            IF transfer_needed THEN
                GRANT espacigo_schema_owner TO espacigo_admin WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
                ALTER TABLE public.media_variant OWNER TO espacigo_schema_owner;
                ALTER TABLE public.media_variant_intent OWNER TO espacigo_schema_owner;
                REVOKE espacigo_schema_owner FROM espacigo_admin;
            END IF;
        END IF;
    END LOOP;

    IF observed_acl IS DISTINCT FROM existing_acl OR EXISTS (
        SELECT 1 FROM pg_catalog.pg_class
        WHERE oid IN ('public.media_variant'::regclass, 'public.media_variant_intent'::regclass)
          AND relowner <> 'espacigo_schema_owner'::regrole
    ) OR EXISTS (
        SELECT 1 FROM pg_catalog.pg_auth_members
        WHERE member = 'espacigo_admin'::regrole
          AND roleid = 'espacigo_schema_owner'::regrole
    ) THEN
        RAISE EXCEPTION 'Media ownership preparation changed existing access';
    END IF;
END
$prepare$;
COMMIT;
