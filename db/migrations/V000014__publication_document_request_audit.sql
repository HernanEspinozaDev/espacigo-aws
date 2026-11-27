-- Record the documentation requirement in the same transaction as publication.
DO $guard$
BEGIN
    IF session_user <> 'espacigo_migrator_login'
       OR current_user <> 'espacigo_schema_owner'
       OR current_schema() <> 'public'
       OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'V000014 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

CREATE TABLE public.espacio_solicitud_documental (
    espacio_id uuid PRIMARY KEY REFERENCES public.espacio(id) ON DELETE RESTRICT,
    actor_usuario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    titular_usuario_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    organizacion_id uuid REFERENCES public.organizacion(id) ON DELETE RESTRICT,
    solicitado_en timestamptz NOT NULL,
    vence_en timestamptz NOT NULL,
    version_politica text NOT NULL CHECK (version_politica = 'documentacion-comercial-v1'),
    documentos_titular text[] NOT NULL,
    documento_espacio text NOT NULL CHECK (documento_espacio = 'property_or_sublease_authorization'),
    CHECK (num_nonnulls(titular_usuario_id, organizacion_id) = 1),
    CHECK ((titular_usuario_id IS NOT NULL AND documentos_titular = ARRAY['sii_start_activities'])
        OR (organizacion_id IS NOT NULL AND documentos_titular = ARRAY['erut','sii_start_activities','power_certificate']))
);

CREATE TRIGGER espacio_solicitud_documental_no_mutation
    BEFORE UPDATE OR DELETE ON public.espacio_solicitud_documental
    FOR EACH ROW EXECUTE FUNCTION public.host_compliance_event_immutable();
CREATE TRIGGER espacio_solicitud_documental_no_truncate
    BEFORE TRUNCATE ON public.espacio_solicitud_documental
    FOR EACH STATEMENT EXECUTE FUNCTION public.host_compliance_event_immutable();

REVOKE ALL ON TABLE public.espacio_solicitud_documental FROM PUBLIC, espacigo_runtime;
GRANT SELECT ON TABLE public.espacio_solicitud_documental TO espacigo_runtime;

CREATE FUNCTION public.kyb_record_publication_document_request(p_space uuid, p_actor uuid)
RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $$
    INSERT INTO public.espacio_solicitud_documental(
        espacio_id, actor_usuario_id, titular_usuario_id, organizacion_id,
        solicitado_en, vence_en, version_politica, documentos_titular, documento_espacio
    )
    SELECT e.id, h.actor_id,
           CASE WHEN e.organizacion_id IS NULL THEN e.propietario_id END,
           e.organizacion_id, h.ocurrida_en,
           COALESCE(o.grace_due_at, p.grace_due_at), 'documentacion-comercial-v1',
           CASE WHEN e.organizacion_id IS NULL THEN ARRAY['sii_start_activities']
                ELSE ARRAY['erut','sii_start_activities','power_certificate'] END,
           'property_or_sublease_authorization'
    FROM public.espacio e
    JOIN LATERAL (
        SELECT actor_id, ocurrida_en
        FROM public.espacio_publicacion_historial_local
        WHERE espacio_id=e.id AND estado_nuevo='activa' AND actor_id=p_actor
        ORDER BY ocurrida_en DESC, id DESC LIMIT 1
    ) h ON true
    LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
    LEFT JOIN public.anfitrion_cumplimiento_personal p ON p.usuario_id=e.propietario_id
    WHERE e.id=p_space AND e.estado='activa'
    ON CONFLICT (espacio_id) DO NOTHING
$$;

REVOKE ALL ON FUNCTION public.kyb_record_publication_document_request(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.kyb_record_publication_document_request(uuid,uuid) TO espacigo_runtime;
