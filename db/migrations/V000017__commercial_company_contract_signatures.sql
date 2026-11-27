-- Bind company-mode contract signatures to the exact legal representative
-- frozen in the verified proposal, while preserving personal-mode ownership.
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
        RAISE EXCEPTION 'V000017 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

ALTER TABLE public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_company_terms_snapshot_ck CHECK (
        (NOT es_b2b AND NOT (terminos ? 'company_representation'))
        OR ((es_b2b
            AND jsonb_typeof(terminos->'company_representation') = 'object'
            AND (terminos->'company_representation'->>'organization_id')::uuid = organizacion_id
            AND terminos->'company_representation'->>'legal_name' = empresa_razon_social_snapshot
            AND terminos->'company_representation'->>'rut_masked' = empresa_rut_masked_snapshot
            AND (terminos->'company_representation'->>'rut_verified')::boolean = empresa_rut_verificada
            AND (terminos->'company_representation'->>'representative_user_id')::uuid = representante_legal_id
            AND terminos->'company_representation'->>'representative_name' = representante_nombre_snapshot
            AND terminos->'company_representation'->>'representative_rut_masked' = representante_rut_masked_snapshot
            AND (terminos->'company_representation'->>'representative_rut_verified')::boolean = representante_rut_verificada) IS TRUE)
    );

CREATE OR REPLACE FUNCTION public.guard_commercial_agreement() RETURNS trigger
LANGUAGE plpgsql SET search_path TO 'pg_catalog', 'public' AS $$
DECLARE p public.contrato_propuesta_comercial%ROWTYPE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT * INTO p FROM public.contrato_propuesta_comercial WHERE id=NEW.propuesta_id;
        IF NOT FOUND
           OR ROW(NEW.cotizacion_id,NEW.espacio_id,NEW.propietario_id,NEW.arrendatario_id,NEW.propuesta_version,NEW.documento_sha256)
              IS DISTINCT FROM ROW(p.cotizacion_id,p.espacio_id,p.propietario_id,p.arrendatario_id,p.version,p.documento_sha256)
           OR ROW(NEW.es_b2b,NEW.organizacion_id,NEW.empresa_razon_social_snapshot,NEW.empresa_rut_masked_snapshot,
                  NEW.empresa_rut_verificada,NEW.representante_legal_id,NEW.representante_nombre_snapshot,
                  NEW.representante_rut_masked_snapshot,NEW.representante_rut_verificada)
              IS DISTINCT FROM ROW(p.es_b2b,p.organizacion_id,p.empresa_razon_social_snapshot,p.empresa_rut_masked_snapshot,
                                   p.empresa_rut_verificada,p.representante_legal_id,p.representante_nombre_snapshot,
                                   p.representante_rut_masked_snapshot,p.representante_rut_verificada) THEN
            RAISE EXCEPTION 'agreement proposal binding mismatch' USING ERRCODE='23514';
        END IF;
        IF NEW.estado <> 'pending_landlord' OR NEW.revision <> 1 OR NEW.vence_en > p.inicio THEN
            RAISE EXCEPTION 'invalid initial agreement state' USING ERRCODE='23514';
        END IF;
    ELSE
        IF ROW(NEW.id,NEW.propuesta_id,NEW.cotizacion_id,NEW.espacio_id,NEW.propietario_id,NEW.arrendatario_id,
               NEW.propuesta_version,NEW.documento_sha256,NEW.idempotency_key,NEW.fingerprint,NEW.vence_en,NEW.creada_en,
               NEW.es_b2b,NEW.organizacion_id,NEW.empresa_razon_social_snapshot,NEW.empresa_rut_masked_snapshot,
               NEW.empresa_rut_verificada,NEW.representante_legal_id,NEW.representante_nombre_snapshot,
               NEW.representante_rut_masked_snapshot,NEW.representante_rut_verificada)
           IS DISTINCT FROM ROW(OLD.id,OLD.propuesta_id,OLD.cotizacion_id,OLD.espacio_id,OLD.propietario_id,OLD.arrendatario_id,
               OLD.propuesta_version,OLD.documento_sha256,OLD.idempotency_key,OLD.fingerprint,OLD.vence_en,OLD.creada_en,
               OLD.es_b2b,OLD.organizacion_id,OLD.empresa_razon_social_snapshot,OLD.empresa_rut_masked_snapshot,
               OLD.empresa_rut_verificada,OLD.representante_legal_id,OLD.representante_nombre_snapshot,
               OLD.representante_rut_masked_snapshot,OLD.representante_rut_verificada) THEN
            RAISE EXCEPTION 'signed agreement source is immutable' USING ERRCODE='23514';
        END IF;
        IF NEW.revision <> OLD.revision+1 OR NEW.actualizada_en < OLD.actualizada_en OR NOT (
            (OLD.estado='pending_landlord' AND NEW.estado IN ('ready_for_payment','rejected','withdrawn','expired')) OR
            (OLD.estado='ready_for_payment' AND NEW.estado IN ('withdrawn','expired'))
        ) THEN
            RAISE EXCEPTION 'invalid agreement transition' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.guard_commercial_signature() RETURNS trigger
LANGUAGE plpgsql SET search_path TO 'pg_catalog', 'public' AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'commercial evidence is immutable' USING ERRCODE='23514';
    END IF;
    SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.solicitud_id;
    IF NOT FOUND OR NEW.documento_sha256 <> a.documento_sha256 OR NEW.propuesta_version <> a.propuesta_version
       OR (NEW.rol='arrendatario' AND NEW.actor_id <> a.arrendatario_id)
       OR (NEW.rol='arrendador' AND NOT (
            (NOT a.es_b2b AND NEW.actor_id=a.propietario_id)
            OR (a.es_b2b AND NEW.actor_id=a.representante_legal_id
                AND EXISTS (
                    SELECT 1
                    FROM public.organizacion_miembro m
                    JOIN public.usuario u ON u.id=m.usuario_id AND u.estado='activo'
                    JOIN public.espacio e ON e.id=a.espacio_id AND e.es_b2b AND e.organizacion_id=a.organizacion_id AND e.propietario_id=a.propietario_id
                    CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,NEW.firmada_en) identity_status
                    WHERE m.organizacion_id=a.organizacion_id AND m.usuario_id=NEW.actor_id
                      AND m.rol='representante_legal' AND m.estado='activo' AND identity_status.verified
                      AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=m.usuario_id)
                )
       ))) THEN
        RAISE EXCEPTION 'commercial signature binding mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.guard_checkout_commercial_contract_binding() RETURNS trigger
LANGUAGE plpgsql SET search_path TO 'pg_catalog', 'public' AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE; p public.contrato_propuesta_comercial%ROWTYPE; n integer;
BEGIN
    IF TG_OP='UPDATE' THEN
        IF ROW(NEW.solicitud_comercial_id,NEW.contrato_version,NEW.contrato_sha256)
           IS DISTINCT FROM ROW(OLD.solicitud_comercial_id,OLD.contrato_version,OLD.contrato_sha256)
           OR (OLD.solicitud_comercial_id IS NOT NULL AND ROW(NEW.id,NEW.cotizacion_id,NEW.espacio_id,NEW.host_id,
               NEW.arrendatario_id,NEW.base_clp,NEW.vence_en,NEW.checkout_method,NEW.test_mode,NEW.fingerprint,
               NEW.idempotency_key,NEW.creada_en)
              IS DISTINCT FROM ROW(OLD.id,OLD.cotizacion_id,OLD.espacio_id,OLD.host_id,OLD.arrendatario_id,
               OLD.base_clp,OLD.vence_en,OLD.checkout_method,OLD.test_mode,OLD.fingerprint,OLD.idempotency_key,OLD.creada_en)) THEN
            RAISE EXCEPTION 'checkout commercial evidence is immutable' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.solicitud_comercial_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.solicitud_comercial_id;
    IF NOT FOUND OR a.estado <> 'ready_for_payment' OR a.cotizacion_id <> NEW.cotizacion_id
       OR a.espacio_id <> NEW.espacio_id OR a.propietario_id <> NEW.host_id OR a.arrendatario_id <> NEW.arrendatario_id
       OR a.propuesta_version <> NEW.contrato_version OR a.documento_sha256 <> NEW.contrato_sha256
       OR a.pago_vence_en <> NEW.vence_en OR a.pago_vence_en <= NEW.creada_en THEN
        RAISE EXCEPTION 'checkout agreement binding mismatch' USING ERRCODE='23514';
    END IF;
    SELECT * INTO p FROM public.contrato_propuesta_comercial WHERE id=a.propuesta_id;
    IF NOT FOUND OR ROW(p.es_b2b,p.organizacion_id,p.empresa_razon_social_snapshot,p.empresa_rut_masked_snapshot,
                        p.empresa_rut_verificada,p.representante_legal_id,p.representante_nombre_snapshot,
                        p.representante_rut_masked_snapshot,p.representante_rut_verificada)
       IS DISTINCT FROM ROW(a.es_b2b,a.organizacion_id,a.empresa_razon_social_snapshot,a.empresa_rut_masked_snapshot,
                            a.empresa_rut_verificada,a.representante_legal_id,a.representante_nombre_snapshot,
                            a.representante_rut_masked_snapshot,a.representante_rut_verificada) THEN
        RAISE EXCEPTION 'checkout company contract snapshot mismatch' USING ERRCODE='23514';
    END IF;
    SELECT count(*) INTO n FROM public.contrato_firma_comercial f WHERE f.solicitud_id=a.id
      AND f.documento_sha256=a.documento_sha256 AND f.propuesta_version=a.propuesta_version
      AND f.consentimiento_version='commercial-agreement-v1' AND f.firmada_en <= NEW.creada_en
      AND (
          (f.rol='arrendador' AND (
              (a.es_b2b AND f.actor_id=a.representante_legal_id)
              OR (NOT a.es_b2b AND f.actor_id=a.propietario_id)
          ))
          OR (f.rol='arrendatario' AND f.actor_id=a.arrendatario_id)
      );
    IF n<>2 OR NOT EXISTS(SELECT 1 FROM public.ocupacion o WHERE o.id=a.ocupacion_id
        AND o.solicitud_comercial_id=a.id AND o.espacio_id=a.espacio_id AND o.activo
        AND o.tipo='retencion_contrato_comercial' AND o.expira_en=a.pago_vence_en AND o.expira_en>NEW.creada_en) THEN
        RAISE EXCEPTION 'checkout requires bilateral signature and live hold' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
