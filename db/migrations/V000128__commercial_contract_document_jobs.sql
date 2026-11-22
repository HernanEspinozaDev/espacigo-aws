-- One recoverable private PDF per paid signed agreement version. Rendering and
-- private object writes run after the payment transaction commits.
CREATE TABLE public.contrato_documento_comercial (
 id uuid PRIMARY KEY,
 operacion_id uuid NOT NULL UNIQUE REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT,
 solicitud_id uuid NOT NULL REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 propuesta_id uuid NOT NULL REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT,
 version bigint NOT NULL CHECK(version>0),
 documento_sha256 bytea NOT NULL CHECK(octet_length(documento_sha256)=32),
 object_id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
 object_key text GENERATED ALWAYS AS ('contracts/' || object_id::text || '.blob') STORED,
 estado text NOT NULL DEFAULT 'pending' CHECK(estado IN ('pending','processing','retry','ready','terminal')),
 intentos integer NOT NULL DEFAULT 0 CHECK(intentos BETWEEN 0 AND 8),
 lease_token uuid,
 lease_vence_en timestamptz,
 disponible_en timestamptz NOT NULL,
 ultimo_error text CHECK(ultimo_error IS NULL OR ultimo_error IN ('evidence_unavailable','render_failed','unsupported_glyph','storage_failed','object_conflict','attempt_limit_reached')),
 bucket_name text,
 object_version text,
 pdf_sha256 bytea CHECK(pdf_sha256 IS NULL OR octet_length(pdf_sha256)=32),
 pdf_bytes bigint CHECK(pdf_bytes IS NULL OR pdf_bytes BETWEEN 1 AND 16777216),
 lista_en timestamptz,
 creada_en timestamptz NOT NULL,
 actualizada_en timestamptz NOT NULL,
 UNIQUE(solicitud_id,version),
 CHECK((estado='processing' AND lease_token IS NOT NULL AND lease_vence_en IS NOT NULL) OR (estado<>'processing' AND lease_token IS NULL AND lease_vence_en IS NULL)),
 CHECK((estado='ready' AND bucket_name IS NOT NULL AND pdf_sha256 IS NOT NULL AND pdf_bytes IS NOT NULL AND lista_en IS NOT NULL) OR (estado<>'ready' AND bucket_name IS NULL AND pdf_sha256 IS NULL AND pdf_bytes IS NULL AND lista_en IS NULL)),
 CHECK(isfinite(disponible_en) AND isfinite(creada_en) AND isfinite(actualizada_en)),
 CHECK(lease_vence_en IS NULL OR isfinite(lease_vence_en))
);
CREATE INDEX contrato_documento_due_idx ON public.contrato_documento_comercial(disponible_en,creada_en,id) WHERE estado IN ('pending','retry','processing');
CREATE FUNCTION public.guard_commercial_contract_document() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NOT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id JOIN public.contrato_solicitud_comercial s ON s.id=c.solicitud_comercial_id WHERE c.id=NEW.operacion_id AND c.estado='approved' AND c.payment_verified_at IS NOT NULL AND r.estado='pagada' AND s.id=NEW.solicitud_id AND s.propuesta_id=NEW.propuesta_id AND s.propuesta_version=NEW.version AND s.documento_sha256=NEW.documento_sha256 AND c.contrato_version=NEW.version AND c.contrato_sha256=NEW.documento_sha256) THEN
   RAISE EXCEPTION 'contract document requires verified paid binding' USING ERRCODE='23514';
  END IF;
 ELSE
  IF ROW(NEW.id,NEW.operacion_id,NEW.solicitud_id,NEW.propuesta_id,NEW.version,NEW.documento_sha256,NEW.object_id,NEW.creada_en) IS DISTINCT FROM ROW(OLD.id,OLD.operacion_id,OLD.solicitud_id,OLD.propuesta_id,OLD.version,OLD.documento_sha256,OLD.object_id,OLD.creada_en) OR OLD.estado='ready' THEN
   RAISE EXCEPTION 'contract document source is immutable' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER contrato_documento_guard BEFORE INSERT OR UPDATE ON public.contrato_documento_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_contract_document();
REVOKE ALL ON public.contrato_documento_comercial FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON public.contrato_documento_comercial TO espacigo_runtime;
