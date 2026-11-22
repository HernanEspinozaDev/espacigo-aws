-- Optional durable agreement binding permits a controlled transition from the
-- legacy checkout. Once bound, its source version and digest cannot change.
ALTER TABLE public.checkout_preferencia_comercial
 ADD COLUMN solicitud_comercial_id uuid UNIQUE REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 ADD COLUMN contrato_version bigint,
 ADD COLUMN contrato_sha256 bytea,
 ADD CONSTRAINT checkout_contract_binding_ck CHECK (
  (solicitud_comercial_id IS NULL AND contrato_version IS NULL AND contrato_sha256 IS NULL)
  OR (solicitud_comercial_id IS NOT NULL AND contrato_version IS NOT NULL AND contrato_sha256 IS NOT NULL AND contrato_version>0 AND octet_length(contrato_sha256)=32)
 );
CREATE FUNCTION public.guard_checkout_commercial_contract_binding() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE; n integer;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.solicitud_comercial_id,NEW.contrato_version,NEW.contrato_sha256)
   IS DISTINCT FROM ROW(OLD.solicitud_comercial_id,OLD.contrato_version,OLD.contrato_sha256)
   OR (OLD.solicitud_comercial_id IS NOT NULL AND ROW(NEW.id,NEW.cotizacion_id,NEW.espacio_id,NEW.host_id,NEW.arrendatario_id,NEW.base_clp,NEW.vence_en,NEW.checkout_method,NEW.test_mode,NEW.fingerprint,NEW.idempotency_key,NEW.creada_en) IS DISTINCT FROM ROW(OLD.id,OLD.cotizacion_id,OLD.espacio_id,OLD.host_id,OLD.arrendatario_id,OLD.base_clp,OLD.vence_en,OLD.checkout_method,OLD.test_mode,OLD.fingerprint,OLD.idempotency_key,OLD.creada_en)) THEN
   RAISE EXCEPTION 'checkout commercial evidence is immutable' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.solicitud_comercial_id IS NULL THEN RETURN NEW; END IF;
 SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.solicitud_comercial_id;
 IF NOT FOUND OR a.estado<>'ready_for_payment' OR a.cotizacion_id<>NEW.cotizacion_id
  OR a.espacio_id<>NEW.espacio_id OR a.propietario_id<>NEW.host_id OR a.arrendatario_id<>NEW.arrendatario_id
  OR a.propuesta_version<>NEW.contrato_version OR a.documento_sha256<>NEW.contrato_sha256
  OR a.pago_vence_en<>NEW.vence_en OR a.pago_vence_en<=NEW.creada_en THEN
  RAISE EXCEPTION 'checkout agreement binding mismatch' USING ERRCODE='23514';
 END IF;
 SELECT count(*) INTO n FROM public.contrato_firma_comercial f WHERE f.solicitud_id=a.id
  AND f.documento_sha256=a.documento_sha256 AND f.propuesta_version=a.propuesta_version
  AND f.consentimiento_version='commercial-agreement-v1' AND f.firmada_en<=NEW.creada_en
  AND ((f.rol='arrendador' AND f.actor_id=a.propietario_id) OR (f.rol='arrendatario' AND f.actor_id=a.arrendatario_id));
 IF n<>2 OR NOT EXISTS(SELECT 1 FROM public.ocupacion o WHERE o.id=a.ocupacion_id
  AND o.solicitud_comercial_id=a.id AND o.espacio_id=a.espacio_id AND o.activo
  AND o.tipo='retencion_contrato_comercial' AND o.expira_en=a.pago_vence_en AND o.expira_en>NEW.creada_en) THEN
  RAISE EXCEPTION 'checkout requires bilateral signature and live hold' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER checkout_commercial_contract_binding_guard BEFORE INSERT OR UPDATE ON public.checkout_preferencia_comercial
 FOR EACH ROW EXECUTE FUNCTION public.guard_checkout_commercial_contract_binding();
COMMENT ON COLUMN public.checkout_preferencia_comercial.solicitud_comercial_id IS
 'Durable signed agreement source; required by application when commercial contracts gate is enabled, immutable once created.';
