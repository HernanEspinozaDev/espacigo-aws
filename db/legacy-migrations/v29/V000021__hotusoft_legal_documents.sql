-- Register the Hotusoft public terms and privacy notice without changing historical versions.
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
        RAISE EXCEPTION 'V000021 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

-- Existing templates keep their 12-section limit. A new proposal may also
-- capture the platform's payment clause in its immutable signed snapshot.
ALTER TABLE public.contrato_propuesta_comercial
    DROP CONSTRAINT contrato_propuesta_comercial_terminos_check5,
    ADD CONSTRAINT contrato_propuesta_comercial_terminos_secciones_check
        CHECK (jsonb_typeof(terminos -> 'sections') = 'array'
           AND jsonb_array_length(terminos -> 'sections') BETWEEN 1 AND 13);

INSERT INTO public.version_terminos (id, codigo, tipo, hash_sha256, publicada_en)
VALUES
  ('00000000-0000-4000-8000-000000000105', 'espacigo-terminos-hotusoft-v1-2', 'terminos',
   '1c17baa937446e5136faa232ae3aaabda8c9579fd4f824791f9ea073cf787d78', CURRENT_TIMESTAMP),
  ('00000000-0000-4000-8000-000000000106', 'espacigo-privacidad-hotusoft-v1-2', 'privacidad',
   '35f430d2463b6036337caa3baf6520d3a1a19d26262dd8339dcf19ccf0cb5eae', CURRENT_TIMESTAMP);
