-- Voluntary bounded authorization for automatic rental review. This record is
-- not a party signature; enabling payment remains gated on legal approval.
CREATE TABLE public.autorizacion_arriendo_comercial (
 id uuid PRIMARY KEY,
 espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
 propietario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 plantilla_version bigint NOT NULL CHECK(plantilla_version>0),
 monto_maximo_clp bigint NOT NULL CHECK(monto_maximo_clp BETWEEN 1 AND 100000000),
 vigente_desde timestamptz NOT NULL,
 vence_en timestamptz NOT NULL,
 creada_en timestamptz NOT NULL,
 revocada_en timestamptz,
 revocada_por uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
 motivo_revocacion text,
 CHECK(isfinite(vigente_desde) AND isfinite(vence_en) AND isfinite(creada_en)),
 CHECK(vence_en>vigente_desde AND vence_en-vigente_desde<=interval '90 days'),
 CHECK((revocada_en IS NULL AND revocada_por IS NULL AND motivo_revocacion IS NULL) OR (revocada_en IS NOT NULL AND revocada_por=propietario_id AND motivo_revocacion IS NOT NULL AND motivo_revocacion ~ '^[a-z][a-z0-9_]{2,63}$'))
);
CREATE UNIQUE INDEX autorizacion_arriendo_comercial_activa_idx ON public.autorizacion_arriendo_comercial(espacio_id) WHERE revocada_en IS NULL;
CREATE INDEX autorizacion_arriendo_comercial_history_idx ON public.autorizacion_arriendo_comercial(espacio_id,creada_en DESC,id);

CREATE TABLE public.autorizacion_arriendo_comercial_evento (
 id uuid PRIMARY KEY,
 autorizacion_id uuid NOT NULL REFERENCES public.autorizacion_arriendo_comercial(id) ON DELETE RESTRICT,
 espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
 actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 accion text NOT NULL CHECK(accion IN ('activated','revoked')),
 motivo text NOT NULL CHECK(motivo ~ '^[a-z][a-z0-9_]{2,63}$'),
 ocurrida_en timestamptz NOT NULL CHECK(isfinite(ocurrida_en)),
 UNIQUE(autorizacion_id,accion)
);
CREATE INDEX autorizacion_arriendo_comercial_event_idx ON public.autorizacion_arriendo_comercial_evento(espacio_id,ocurrida_en,id);

CREATE FUNCTION public.guard_commercial_rental_authorization() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NOT EXISTS(SELECT 1 FROM public.espacio e JOIN public.espacio_contrato_borrador d ON d.espacio_id=e.id WHERE e.id=NEW.espacio_id AND e.propietario_id=NEW.propietario_id AND e.estado<>'eliminada' AND d.version_publicada=NEW.plantilla_version AND NEW.vigente_desde>=clock_timestamp()-interval '1 minute') THEN
   RAISE EXCEPTION 'authorization requires owner and current published template' USING ERRCODE='23514';
  END IF;
 ELSE
  IF ROW(NEW.id,NEW.espacio_id,NEW.propietario_id,NEW.plantilla_version,NEW.monto_maximo_clp,NEW.vigente_desde,NEW.vence_en,NEW.creada_en) IS DISTINCT FROM ROW(OLD.id,OLD.espacio_id,OLD.propietario_id,OLD.plantilla_version,OLD.monto_maximo_clp,OLD.vigente_desde,OLD.vence_en,OLD.creada_en) OR OLD.revocada_en IS NOT NULL OR NEW.revocada_en IS NULL OR NEW.revocada_por<>OLD.propietario_id THEN
   RAISE EXCEPTION 'authorization is immutable except owner revocation' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commercial_rental_authorization_guard BEFORE INSERT OR UPDATE ON public.autorizacion_arriendo_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_rental_authorization();
CREATE FUNCTION public.protect_commercial_rental_authorization_event() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$ BEGIN RAISE EXCEPTION 'authorization history is append-only' USING ERRCODE='23514'; END $$;
CREATE TRIGGER commercial_rental_authorization_event_immutable BEFORE UPDATE OR DELETE ON public.autorizacion_arriendo_comercial_evento FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_rental_authorization_event();

-- A new published template invalidates every active opt-in. The owner must
-- authorize the new version explicitly; the old authorization is never copied.
CREATE FUNCTION public.revoke_commercial_rental_authorization_on_template_change() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE changed record;
BEGIN
 IF NEW.version_publicada IS DISTINCT FROM OLD.version_publicada THEN
  FOR changed IN UPDATE public.autorizacion_arriendo_comercial SET revocada_en=clock_timestamp(),revocada_por=propietario_id,motivo_revocacion='template_version_changed' WHERE espacio_id=NEW.espacio_id AND revocada_en IS NULL AND plantilla_version<>NEW.version_publicada RETURNING id,espacio_id,propietario_id LOOP
   INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) VALUES(gen_random_uuid(),changed.id,changed.espacio_id,changed.propietario_id,'revoked','template_version_changed',clock_timestamp());
  END LOOP;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commercial_rental_authorization_template_change AFTER UPDATE OF version_publicada ON public.espacio_contrato_borrador FOR EACH ROW EXECUTE FUNCTION public.revoke_commercial_rental_authorization_on_template_change();
REVOKE ALL ON public.autorizacion_arriendo_comercial,public.autorizacion_arriendo_comercial_evento FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_commercial_rental_authorization() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.protect_commercial_rental_authorization_event() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.revoke_commercial_rental_authorization_on_template_change() FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON public.autorizacion_arriendo_comercial TO espacigo_runtime;
GRANT SELECT,INSERT ON public.autorizacion_arriendo_comercial_evento TO espacigo_runtime;
