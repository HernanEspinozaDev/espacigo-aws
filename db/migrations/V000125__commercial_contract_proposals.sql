-- Immutable proposals are separate from requests, signatures, holds and payments.
-- cotizacion_id deliberately has no FK: 15-minute quotes are routinely purged.
CREATE TABLE public.contrato_propuesta_comercial (
    id uuid PRIMARY KEY,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
    propietario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    arrendatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    version bigint NOT NULL CHECK (version > 0),
    plantilla_version_id uuid NOT NULL REFERENCES public.espacio_contrato_version(id) ON DELETE RESTRICT,
    inicio timestamptz NOT NULL,
    termino timestamptz NOT NULL CHECK (termino > inicio),
    zona_horaria text NOT NULL,
    subtotal_clp bigint NOT NULL CHECK (subtotal_clp > 0),
    moneda text NOT NULL CHECK (moneda = 'CLP'),
    terminos jsonb NOT NULL CHECK (jsonb_typeof(terminos) = 'object')
        CHECK (terminos ?& ARRAY['quote_id','space_id','space_title','template_id','template_version','location','sections','rate_unit','rate_version','unit_price_clp','units','subtotal_clp','currency','start_at','end_at','time_zone','cancellation_policy'])
        CHECK (jsonb_typeof(terminos->'location') = 'object' AND (terminos->'location') ?& ARRAY['address','latitude','longitude','revision','region_code','commune_code'])
        CHECK (jsonb_typeof(terminos->'cancellation_policy') = 'object' AND (terminos->'cancellation_policy') ?& ARRAY['version','text'])
        CHECK (NOT (terminos ?| ARRAY['parties','owner_id','renter_id','propietario_id','arrendatario_id','legal_name','rut','email','phone'])),
    terminos_canonicos text NOT NULL,
    terminos_sha256 bytea NOT NULL CHECK (octet_length(terminos_sha256) = 32),
    documento_sha256 bytea NOT NULL CHECK (octet_length(documento_sha256) = 32),
    identidades_cifradas bytea NOT NULL CHECK (octet_length(identidades_cifradas) BETWEEN 29 AND 65564),
    clave_version text NOT NULL CHECK (length(clave_version) BETWEEN 1 AND 64),
    creada_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cotizacion_id, version),
    UNIQUE (cotizacion_id, documento_sha256),
    CHECK (propietario_id <> arrendatario_id),
    CHECK (terminos = terminos_canonicos::jsonb),
    CHECK (terminos_sha256 = pg_catalog.sha256(convert_to(terminos_canonicos, 'UTF8'))),
    CHECK ((terminos->>'space_id' = espacio_id::text) IS TRUE),
    CHECK ((terminos->>'quote_id' = cotizacion_id::text) IS TRUE),
    CHECK ((terminos->>'template_id' = plantilla_version_id::text) IS TRUE),
    CHECK (((terminos->>'subtotal_clp')::bigint = subtotal_clp) IS TRUE),
    CHECK ((terminos->>'currency' = moneda) IS TRUE),
    CHECK ((terminos->>'time_zone' = zona_horaria) IS TRUE),
    CHECK (((terminos->>'start_at')::timestamptz = inicio) IS TRUE),
    CHECK (((terminos->>'end_at')::timestamptz = termino) IS TRUE),
    CHECK (jsonb_typeof(terminos->'sections') = 'array' AND jsonb_array_length(terminos->'sections') BETWEEN 1 AND 12),
    CHECK (jsonb_typeof(terminos->'space_title') = 'string' AND length(terminos->>'space_title') > 0),
    CHECK (jsonb_typeof(terminos->'rate_version') = 'number' AND (terminos->>'rate_version')::bigint > 0),
    CHECK (jsonb_typeof(terminos->'template_version') = 'number' AND (terminos->>'template_version')::bigint > 0),
    CHECK (jsonb_typeof(terminos->'unit_price_clp') = 'number' AND (terminos->>'unit_price_clp')::bigint > 0),
    CHECK (jsonb_typeof(terminos->'units') = 'number' AND (terminos->>'units')::bigint > 0),
    CHECK (jsonb_typeof(terminos->'rate_unit') = 'string' AND terminos->>'rate_unit' IN ('hora','dia','mes')),
    CHECK (jsonb_typeof(terminos->'location'->'latitude') = 'number' AND (terminos->'location'->>'latitude')::double precision BETWEEN -90 AND 90),
    CHECK (jsonb_typeof(terminos->'location'->'longitude') = 'number' AND (terminos->'location'->>'longitude')::double precision BETWEEN -180 AND 180),
    CHECK (jsonb_typeof(terminos->'location'->'revision') = 'number' AND (terminos->'location'->>'revision')::bigint >= 0),
    CHECK (jsonb_typeof(terminos->'location'->'address') = 'string' AND length(terminos->'location'->>'address') > 0),
    CHECK (jsonb_typeof(terminos->'location'->'region_code') = 'string'),
    CHECK (jsonb_typeof(terminos->'location'->'commune_code') = 'string'),
    CHECK (jsonb_typeof(terminos->'cancellation_policy'->'version') = 'string' AND length(terminos->'cancellation_policy'->>'version') > 0),
    CHECK (jsonb_typeof(terminos->'cancellation_policy'->'text') = 'string' AND length(terminos->'cancellation_policy'->>'text') > 0)
);
CREATE INDEX contrato_propuesta_comercial_participantes_idx
    ON public.contrato_propuesta_comercial (arrendatario_id, propietario_id, creada_en DESC);
CREATE TRIGGER commercial_contract_proposal_immutable
    BEFORE UPDATE OR DELETE ON public.contrato_propuesta_comercial
    FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_contract_template_version();
REVOKE ALL ON TABLE public.contrato_propuesta_comercial FROM PUBLIC;
GRANT SELECT, INSERT ON public.contrato_propuesta_comercial TO espacigo_runtime;
COMMENT ON TABLE public.contrato_propuesta_comercial IS
    'Commercial proposal only; terms without legal identities, separate AEAD identity evidence, full canonical document digest. Does not sign, reserve, hold or enable payment.';
