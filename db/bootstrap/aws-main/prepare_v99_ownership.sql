BEGIN;
SELECT pg_catalog.pg_advisory_xact_lock(1163087943, 1296648018);

DO $target$
BEGIN
    IF current_database() <> 'espacigo_prod'
       OR session_user <> 'espacigo_admin' OR session_user <> current_user THEN
        RAISE EXCEPTION 'Unexpected RDS ownership preparation target';
    END IF;
    IF (SELECT count(*) FROM public.schema_migrations) <> 98
       OR NOT EXISTS (
           SELECT 1 FROM public.schema_migrations
           WHERE version = 98 AND name = 'V000098__space_inventory_and_calendar_rules.sql'
             AND checksum = 'a12e56098d68a4d683ff2bca5fb975d96fa6ec6853675336a8bd318c595339f2'
       ) THEN
        RAISE EXCEPTION 'Ownership preparation requires the canonical V98 boundary';
    END IF;
    IF (SELECT count(*) FROM pg_catalog.pg_class
        WHERE oid IN ('public.ocupacion'::regclass, 'public.schema_migrations'::regclass,
                      'public.usuario'::regclass, 'public.espacio'::regclass)
          AND relowner IN ('espacigo_admin'::regrole, 'espacigo_schema_owner'::regrole)) <> 4 THEN
        RAISE EXCEPTION 'Unexpected owner of the approved V99 relations';
    END IF;
    IF pg_catalog.has_schema_privilege('espacigo_schema_owner', 'public', 'CREATE')
       OR EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members
                  WHERE member = 'espacigo_admin'::regrole
                    AND roleid = 'espacigo_schema_owner'::regrole) THEN
        RAISE EXCEPTION 'Close previous schema-owner preparation before continuing';
    END IF;
END
$target$;

GRANT espacigo_schema_owner TO espacigo_admin WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
ALTER TABLE public.usuario OWNER TO espacigo_schema_owner;
ALTER TABLE public.espacio OWNER TO espacigo_schema_owner;
ALTER TABLE public.ocupacion OWNER TO espacigo_schema_owner;
ALTER TABLE public.schema_migrations OWNER TO espacigo_schema_owner;
REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
REVOKE espacigo_schema_owner FROM espacigo_admin;

DO $verify$
BEGIN
    IF (SELECT count(*) FROM pg_catalog.pg_class
        WHERE oid IN ('public.ocupacion'::regclass, 'public.schema_migrations'::regclass,
                      'public.usuario'::regclass, 'public.espacio'::regclass)
          AND relowner = 'espacigo_schema_owner'::regrole) <> 4
       OR NOT pg_catalog.has_table_privilege('espacigo_runtime', 'public.ocupacion', 'SELECT')
       OR NOT pg_catalog.has_table_privilege('espacigo_runtime', 'public.usuario', 'SELECT')
       OR NOT pg_catalog.has_table_privilege('espacigo_runtime', 'public.espacio', 'SELECT')
       OR NOT pg_catalog.has_column_privilege('espacigo_document_verification_fn', 'public.usuario', 'estado', 'SELECT')
       OR NOT pg_catalog.has_column_privilege('espacigo_document_verification_fn', 'public.espacio', 'propietario_id', 'SELECT')
       OR pg_catalog.has_schema_privilege('espacigo_schema_owner', 'public', 'CREATE') THEN
        RAISE EXCEPTION 'RDS ownership or existing access changed unexpectedly';
    END IF;
END
$verify$;
COMMIT;
