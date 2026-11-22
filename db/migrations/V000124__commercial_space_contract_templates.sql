-- Commercial owner-managed clauses; independent of contrato_ensayo_*.
CREATE TABLE public.espacio_contrato_borrador (
    espacio_id uuid PRIMARY KEY REFERENCES public.espacio(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision > 0),
    secciones jsonb NOT NULL CHECK (jsonb_typeof(secciones) = 'array' AND jsonb_array_length(secciones) BETWEEN 1 AND 12),
    version_publicada bigint NOT NULL DEFAULT 0 CHECK (version_publicada >= 0),
    actualizada_en timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.espacio_contrato_version (
    id uuid PRIMARY KEY,
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
    version bigint NOT NULL CHECK (version > 0),
    revision_origen bigint NOT NULL CHECK (revision_origen > 0),
    texto_canonico text NOT NULL CHECK (octet_length(texto_canonico) BETWEEN 2 AND 160000),
    secciones jsonb NOT NULL CHECK (jsonb_typeof(secciones) = 'array' AND jsonb_array_length(secciones) BETWEEN 1 AND 12),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    publicada_por uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    publicada_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (espacio_id, version),
    UNIQUE (espacio_id, revision_origen),
    CHECK (secciones = texto_canonico::jsonb),
    CHECK (sha256 = pg_catalog.sha256(convert_to(texto_canonico, 'UTF8')))
);

CREATE FUNCTION public.protect_commercial_contract_template_version()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    RAISE EXCEPTION 'published commercial contract templates are immutable' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER commercial_contract_template_immutable
    BEFORE UPDATE OR DELETE ON public.espacio_contrato_version
    FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_contract_template_version();

REVOKE ALL ON TABLE public.espacio_contrato_borrador, public.espacio_contrato_version FROM PUBLIC;
REVOKE ALL ON FUNCTION public.protect_commercial_contract_template_version() FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON public.espacio_contrato_borrador TO espacigo_runtime;
GRANT SELECT, INSERT ON public.espacio_contrato_version TO espacigo_runtime;

COMMENT ON TABLE public.espacio_contrato_version IS 'Immutable commercial clause versions. New requests select the current published version and retain their own immutable agreement snapshot.';
