-- Register production terms v2 covering differentiated KYC/KYB/SII
-- accreditation, the 2-month America/Santiago deadline, per-space verification,
-- automatic disablement on expiry, company mandates, and host tax duties.
DO $guard$
BEGIN
    IF session_user <> 'espacigo_migrator_login'
       OR current_user <> 'espacigo_schema_owner'
       OR current_schema() <> 'public'
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'V000019 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

INSERT INTO public.version_terminos(id,codigo,tipo,hash_sha256,publicada_en)
VALUES('00000000-0000-4000-8000-000000000104','espacigo-terminos-20261126-v2','terminos',
       'b5dc085757725d8bc60a545f98b9b544a1b1cb07c16355df7bb56bac3e274d3c','2026-11-26 00:00:00+00');
