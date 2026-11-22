-- Immutable commercial proposals acquire consent through separate bilateral
-- evidence. No payment or synthetic booking-trial record is created here.
CREATE TABLE public.contrato_solicitud_comercial (
 id uuid PRIMARY KEY,
 propuesta_id uuid NOT NULL REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT,
 cotizacion_id uuid NOT NULL,
 espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
 propietario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 arrendatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 propuesta_version bigint NOT NULL CHECK(propuesta_version>0),
 documento_sha256 bytea NOT NULL CHECK(octet_length(documento_sha256)=32),
 idempotency_key text NOT NULL CHECK(char_length(idempotency_key) BETWEEN 1 AND 200),
 fingerprint bytea NOT NULL CHECK(octet_length(fingerprint)=32),
 estado text NOT NULL CHECK(estado IN ('pending_landlord','ready_for_payment','rejected','withdrawn','expired')),
 vence_en timestamptz NOT NULL,
 pago_vence_en timestamptz,
 ocupacion_id uuid,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 creada_en timestamptz NOT NULL,
 actualizada_en timestamptz NOT NULL,
 UNIQUE(arrendatario_id,idempotency_key),
 UNIQUE(propuesta_id),
 CHECK(propietario_id<>arrendatario_id),
 CHECK(isfinite(vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en) AND vence_en>creada_en AND actualizada_en>=creada_en),
 CHECK(pago_vence_en IS NULL OR (isfinite(pago_vence_en) AND pago_vence_en>creada_en)),
 CHECK(estado<>'ready_for_payment' OR (pago_vence_en IS NOT NULL AND ocupacion_id IS NOT NULL))
);
CREATE INDEX contrato_solicitud_owner_idx ON public.contrato_solicitud_comercial(propietario_id,creada_en DESC,id);
CREATE INDEX contrato_solicitud_renter_idx ON public.contrato_solicitud_comercial(arrendatario_id,creada_en DESC,id);
CREATE INDEX contrato_solicitud_expira_idx ON public.contrato_solicitud_comercial(vence_en,id) WHERE estado='pending_landlord';
CREATE INDEX contrato_solicitud_pago_expira_idx ON public.contrato_solicitud_comercial(pago_vence_en,id) WHERE estado='ready_for_payment';

CREATE TABLE public.contrato_firma_comercial (
 id uuid PRIMARY KEY,
 solicitud_id uuid NOT NULL REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 -- Historical authentication reference: no bearer and no FK preventing session expiry/purge.
 sesion_id uuid NOT NULL,
 rol text NOT NULL CHECK(rol IN ('arrendatario','arrendador')),
 propuesta_version bigint NOT NULL CHECK(propuesta_version>0),
 documento_sha256 bytea NOT NULL CHECK(octet_length(documento_sha256)=32),
 consentimiento boolean NOT NULL CHECK(consentimiento),
 consentimiento_version text NOT NULL CHECK(consentimiento_version='commercial-agreement-v1'),
 firmada_en timestamptz NOT NULL CHECK(isfinite(firmada_en)),
 UNIQUE(solicitud_id,rol)
);
CREATE TABLE public.contrato_solicitud_evento_comercial (
 id uuid PRIMARY KEY,
 solicitud_id uuid NOT NULL REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 actor_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
 tipo text NOT NULL CHECK(tipo IN ('request_created','landlord_signed','rejected','withdrawn','expired')),
 revision bigint NOT NULL CHECK(revision>0),
 documento_sha256 bytea NOT NULL CHECK(octet_length(documento_sha256)=32),
 ocurrida_en timestamptz NOT NULL CHECK(isfinite(ocurrida_en)),
 UNIQUE(solicitud_id,revision)
);
CREATE TABLE public.contrato_solicitud_notice_outbox (
 id uuid PRIMARY KEY,
 solicitud_id uuid NOT NULL REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 destinatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 tipo text NOT NULL CHECK(tipo IN ('request_created','landlord_signed','rejected','withdrawn','expired')),
 estado text NOT NULL DEFAULT 'pending' CHECK(estado IN ('pending','processing','delivered','cancelled','terminal')),
 intentos integer NOT NULL DEFAULT 0 CHECK(intentos BETWEEN 0 AND 8),
 disponible_en timestamptz NOT NULL,
 reclamada_en timestamptz,
 entregada_en timestamptz,
 ultimo_error text CHECK(ultimo_error IS NULL OR ultimo_error IN ('delivery_failed','recipient_unavailable','attempt_limit_reached')),
 creada_en timestamptz NOT NULL,
 UNIQUE(solicitud_id,destinatario_id,tipo)
);
CREATE INDEX contrato_solicitud_notice_due_idx ON public.contrato_solicitud_notice_outbox(disponible_en,creada_en,id) WHERE estado IN ('pending','processing');

CREATE FUNCTION public.guard_commercial_agreement() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE p public.contrato_propuesta_comercial%ROWTYPE;
BEGIN
 IF TG_OP='INSERT' THEN
  SELECT * INTO p FROM public.contrato_propuesta_comercial WHERE id=NEW.propuesta_id;
  IF NOT FOUND OR ROW(NEW.cotizacion_id,NEW.espacio_id,NEW.propietario_id,NEW.arrendatario_id,NEW.propuesta_version,NEW.documento_sha256)
    IS DISTINCT FROM ROW(p.cotizacion_id,p.espacio_id,p.propietario_id,p.arrendatario_id,p.version,p.documento_sha256) THEN
    RAISE EXCEPTION 'agreement proposal binding mismatch' USING ERRCODE='23514';
  END IF;
  IF NEW.estado<>'pending_landlord' OR NEW.revision<>1 OR NEW.vence_en>p.inicio THEN
   RAISE EXCEPTION 'invalid initial agreement state' USING ERRCODE='23514';
  END IF;
 ELSE
  IF ROW(NEW.id,NEW.propuesta_id,NEW.cotizacion_id,NEW.espacio_id,NEW.propietario_id,NEW.arrendatario_id,NEW.propuesta_version,NEW.documento_sha256,NEW.idempotency_key,NEW.fingerprint,NEW.vence_en,NEW.creada_en)
   IS DISTINCT FROM ROW(OLD.id,OLD.propuesta_id,OLD.cotizacion_id,OLD.espacio_id,OLD.propietario_id,OLD.arrendatario_id,OLD.propuesta_version,OLD.documento_sha256,OLD.idempotency_key,OLD.fingerprint,OLD.vence_en,OLD.creada_en) THEN
   RAISE EXCEPTION 'signed agreement source is immutable' USING ERRCODE='23514';
  END IF;
  IF NEW.revision<>OLD.revision+1 OR NEW.actualizada_en<OLD.actualizada_en OR NOT (
   (OLD.estado='pending_landlord' AND NEW.estado IN ('ready_for_payment','rejected','withdrawn','expired')) OR
   (OLD.estado='ready_for_payment' AND NEW.estado IN ('withdrawn','expired'))) THEN
   RAISE EXCEPTION 'invalid agreement transition' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER contrato_solicitud_guard BEFORE INSERT OR UPDATE ON public.contrato_solicitud_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_agreement();

CREATE FUNCTION public.guard_commercial_signature() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'commercial evidence is immutable' USING ERRCODE='23514'; END IF;
 SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.solicitud_id;
 IF NOT FOUND OR NEW.documento_sha256<>a.documento_sha256 OR NEW.propuesta_version<>a.propuesta_version OR
  (NEW.rol='arrendatario' AND NEW.actor_id<>a.arrendatario_id) OR (NEW.rol='arrendador' AND NEW.actor_id<>a.propietario_id) THEN
  RAISE EXCEPTION 'commercial signature binding mismatch' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER contrato_firma_guard BEFORE INSERT OR UPDATE OR DELETE ON public.contrato_firma_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_signature();

CREATE FUNCTION public.require_commercial_agreement_signatures() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE; n integer;
BEGIN
 SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.id;
 SELECT count(*) INTO n FROM public.contrato_firma_comercial WHERE solicitud_id=a.id AND documento_sha256=a.documento_sha256 AND propuesta_version=a.propuesta_version;
 IF n<1 OR (a.estado='ready_for_payment' AND n<>2) THEN RAISE EXCEPTION 'commercial signatures required' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER contrato_solicitud_signatures AFTER INSERT OR UPDATE ON public.contrato_solicitud_comercial DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_commercial_agreement_signatures();

REVOKE ALL ON public.contrato_solicitud_comercial,public.contrato_firma_comercial,public.contrato_solicitud_evento_comercial,public.contrato_solicitud_notice_outbox FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON public.contrato_solicitud_comercial,public.contrato_solicitud_notice_outbox TO espacigo_runtime;
GRANT SELECT,INSERT ON public.contrato_firma_comercial,public.contrato_solicitud_evento_comercial TO espacigo_runtime;
ALTER TABLE public.ocupacion ADD COLUMN solicitud_comercial_id uuid;
ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_check;
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_check CHECK (
 tipo IN ('bloqueo_manual','retencion','reserva','retencion_temprana','reserva_temprana','retencion_comercial','reserva_comercial','retencion_contrato_comercial')
);
ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_reserva_ck;
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_reserva_ck CHECK (
 (tipo='bloqueo_manual' AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NULL AND solicitud_comercial_id IS NULL AND expira_en IS NULL AND motivo IS NOT NULL AND length(btrim(motivo)) BETWEEN 1 AND 500)
 OR (tipo IN ('retencion','reserva') AND reserva_id IS NOT NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NULL AND solicitud_comercial_id IS NULL AND (tipo<>'retencion' OR expira_en IS NOT NULL))
 OR (tipo IN ('retencion_temprana','reserva_temprana') AND reserva_id IS NULL AND arriendo_temprano_id IS NOT NULL AND reserva_arriendo_comercial_id IS NULL AND solicitud_comercial_id IS NULL AND motivo IS NULL AND (tipo<>'retencion_temprana' OR expira_en IS NOT NULL))
 OR (tipo IN ('retencion_comercial','reserva_comercial') AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NOT NULL AND motivo IS NULL AND (tipo<>'retencion_comercial' OR expira_en IS NOT NULL))
 OR (tipo='retencion_contrato_comercial' AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NULL AND solicitud_comercial_id IS NOT NULL AND motivo IS NULL AND expira_en IS NOT NULL AND isfinite(expira_en) AND expira_en<=lower(intervalo))
);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_solicitud_comercial_uk UNIQUE(solicitud_comercial_id);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_solicitud_comercial_ref_uk UNIQUE(solicitud_comercial_id,espacio_id,id);
-- Add matching unique request triple before these deferred cyclic FKs.
ALTER TABLE public.contrato_solicitud_comercial ADD CONSTRAINT contrato_solicitud_ocupacion_ref_uk UNIQUE(id,espacio_id,ocupacion_id);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_solicitud_comercial_fk
 FOREIGN KEY(solicitud_comercial_id,espacio_id,id)
 REFERENCES public.contrato_solicitud_comercial(id,espacio_id,ocupacion_id)
 ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE public.contrato_solicitud_comercial ADD CONSTRAINT contrato_solicitud_ocupacion_fk
 FOREIGN KEY(id,espacio_id,ocupacion_id)
 REFERENCES public.ocupacion(solicitud_comercial_id,espacio_id,id)
 ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

-- A short-lived FK anchor serializes signature submission with concurrent quote
-- purge without granting UPDATE on immutable quotes. Terminal requests retain
-- only the historical quote ID; reservation-owned quotes keep their own FK.
CREATE TABLE public.contrato_solicitud_quote_activa (
 solicitud_id uuid PRIMARY KEY REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT,
 cotizacion_id uuid NOT NULL REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE RESTRICT
);
REVOKE ALL ON public.contrato_solicitud_quote_activa FROM PUBLIC;
GRANT SELECT,INSERT,DELETE ON public.contrato_solicitud_quote_activa TO espacigo_runtime;
CREATE FUNCTION public.maintain_commercial_agreement_quote_anchor() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF NEW.estado IN ('pending_landlord','ready_for_payment') THEN
  INSERT INTO public.contrato_solicitud_quote_activa(solicitud_id,cotizacion_id) VALUES(NEW.id,NEW.cotizacion_id) ON CONFLICT(solicitud_id) DO NOTHING;
 ELSE
  IF EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial WHERE cotizacion_id=NEW.cotizacion_id AND estado IN ('pending','approved','reconciliation_required')) THEN
   RAISE EXCEPTION 'commercial checkout owns the request interval' USING ERRCODE='23514';
  END IF;
  DELETE FROM public.contrato_solicitud_quote_activa WHERE solicitud_id=NEW.id;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER contrato_solicitud_quote_anchor AFTER INSERT OR UPDATE ON public.contrato_solicitud_comercial FOR EACH ROW EXECUTE FUNCTION public.maintain_commercial_agreement_quote_anchor();

CREATE FUNCTION public.protect_commercial_agreement_event() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 RAISE EXCEPTION 'commercial agreement events are immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER contrato_solicitud_evento_immutable BEFORE UPDATE OR DELETE ON public.contrato_solicitud_evento_comercial FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_agreement_event();
