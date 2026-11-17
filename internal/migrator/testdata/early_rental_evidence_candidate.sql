DO $migration$
DECLARE
    original_role text := current_user;
BEGIN
    IF current_schema() <> 'public' OR session_user <> original_role
       OR NOT pg_catalog.pg_has_role(session_user, 'espacigo_schema_owner', 'SET') THEN
        RAISE EXCEPTION 'unexpected migration role or schema';
    END IF;
    SET ROLE espacigo_schema_owner;

    CREATE TABLE public.arriendo_temprano_confirmacion (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        arriendo_id uuid NOT NULL REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT,
        actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        clase text NOT NULL CHECK (clase IN ('identidad', 'contrato')),
        contrato_version bigint CHECK (contrato_version > 0),
        contrato_digest bytea CHECK (octet_length(contrato_digest) = 32),
        confirmada_en timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(confirmada_en)),
        CHECK ((clase='identidad' AND contrato_version IS NULL AND contrato_digest IS NULL)
            OR (clase='contrato' AND contrato_version IS NOT NULL AND contrato_digest IS NOT NULL)),
        UNIQUE (arriendo_id, actor_id, clase),
        UNIQUE (id, arriendo_id)
    );

    CREATE FUNCTION public.arriendo_temprano_confirmacion_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $fn$
    DECLARE host_id uuid; renter_id uuid; ver bigint; digest bytea;
    BEGIN
        IF TG_OP <> 'INSERT' THEN RAISE EXCEPTION 'confirmation rows are immutable' USING ERRCODE='55000'; END IF;
        SELECT anfitrion_id, arrendatario_id, contrato_version, contrato_digest
          INTO host_id, renter_id, ver, digest FROM public.arriendo_temprano WHERE id=NEW.arriendo_id;
        IF NOT FOUND OR NEW.actor_id NOT IN (host_id, renter_id) THEN
            RAISE EXCEPTION 'actor is not an arriendo participant' USING ERRCODE='23514';
        END IF;
        IF NEW.clase='contrato' AND (NEW.contrato_version<>ver OR NEW.contrato_digest<>digest) THEN
            RAISE EXCEPTION 'contract snapshot mismatch' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END $fn$;
    CREATE TRIGGER arriendo_temprano_confirmacion_guard BEFORE INSERT OR UPDATE OR DELETE ON public.arriendo_temprano_confirmacion FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_confirmacion_guard();

    CREATE TABLE public.arriendo_temprano_pago (
        arriendo_id uuid PRIMARY KEY REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT,
        pago_id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
        emisor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        receptor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        monto_clp bigint NOT NULL CHECK (monto_clp > 0),
        moneda text NOT NULL CHECK (moneda='CLP'),
        estado text NOT NULL CHECK (estado IN ('pendiente','enviado','recibido','anulado','vencido')),
        enviado_en timestamptz,
        recibido_en timestamptz,
        creado_en timestamptz NOT NULL DEFAULT clock_timestamp(),
        UNIQUE (arriendo_id,pago_id),
        CHECK (emisor_id<>receptor_id),
        CHECK ((estado='pendiente' AND enviado_en IS NULL AND recibido_en IS NULL) OR
               (estado='enviado' AND enviado_en IS NOT NULL AND recibido_en IS NULL) OR
               (estado='recibido' AND enviado_en IS NOT NULL AND recibido_en IS NOT NULL AND recibido_en>=enviado_en) OR
               (estado IN ('anulado','vencido') AND recibido_en IS NULL)),
        CHECK (isfinite(creado_en) AND (enviado_en IS NULL OR isfinite(enviado_en)) AND (recibido_en IS NULL OR isfinite(recibido_en)))
    );
    CREATE FUNCTION public.arriendo_temprano_pago_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $fn$
    DECLARE host_id uuid; renter_id uuid; amount bigint;
    BEGIN
        IF TG_OP='DELETE' THEN RAISE EXCEPTION 'payment rows cannot be deleted' USING ERRCODE='55000'; END IF;
        SELECT anfitrion_id, arrendatario_id, subtotal_clp INTO host_id,renter_id,amount FROM public.arriendo_temprano WHERE id=NEW.arriendo_id;
        IF NOT FOUND OR NEW.emisor_id<>renter_id OR NEW.receptor_id<>host_id OR NEW.monto_clp<>amount OR NEW.moneda<>'CLP' THEN
            RAISE EXCEPTION 'payment snapshot/parties mismatch' USING ERRCODE='23514';
        END IF;
        IF TG_OP='INSERT' AND (NEW.estado<>'pendiente' OR NEW.enviado_en IS NOT NULL OR NEW.recibido_en IS NOT NULL) THEN
            RAISE EXCEPTION 'payment must begin pending' USING ERRCODE='23514';
        END IF;
        IF TG_OP='UPDATE' THEN
            IF NOT ((OLD.estado='pendiente' AND NEW.estado='enviado') OR
                    (OLD.estado='enviado' AND NEW.estado IN ('recibido','anulado','vencido'))) OR
               ROW(NEW.arriendo_id,NEW.pago_id,NEW.emisor_id,NEW.receptor_id,NEW.monto_clp,NEW.moneda,NEW.creado_en)
               IS DISTINCT FROM ROW(OLD.arriendo_id,OLD.pago_id,OLD.emisor_id,OLD.receptor_id,OLD.monto_clp,OLD.moneda,OLD.creado_en)
               OR (OLD.enviado_en IS NOT NULL AND NEW.enviado_en IS DISTINCT FROM OLD.enviado_en)
               OR (OLD.recibido_en IS NOT NULL AND NEW.recibido_en IS DISTINCT FROM OLD.recibido_en)
               OR NEW.estado NOT IN ('enviado','recibido','anulado','vencido') THEN
                RAISE EXCEPTION 'invalid payment transition or immutable snapshot mutation' USING ERRCODE='55000';
            END IF;
        END IF;
        RETURN NEW;
    END $fn$;
    CREATE TRIGGER arriendo_temprano_pago_guard BEFORE INSERT OR UPDATE OR DELETE ON public.arriendo_temprano_pago FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_pago_guard();

    CREATE TABLE public.arriendo_temprano_evento (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        arriendo_id uuid NOT NULL REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT,
        secuencia bigint NOT NULL CHECK (secuencia>0), revision bigint NOT NULL CHECK (revision>0),
        tipo text NOT NULL CHECK (tipo IN ('oferta_creada','identidad_simulada_confirmada','contrato_aceptado','pago_marcado_enviado','pago_marcado_recibido','oferta_rechazada','oferta_cancelada','oferta_vencida')),
        actor_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
        estado_anterior text CHECK (estado_anterior IN ('pendiente_identidad','pendiente_contrato','pendiente_envio_pago','pendiente_recepcion_pago','confirmado','rechazado','cancelado','vencido')),
        estado_nuevo text NOT NULL CHECK (estado_nuevo IN ('pendiente_identidad','pendiente_contrato','pendiente_envio_pago','pendiente_recepcion_pago','confirmado','rechazado','cancelado','vencido')),
        ocurrido_en timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(ocurrido_en)),
        confirmacion_id uuid,
        pago_id uuid,
        UNIQUE(arriendo_id,secuencia), UNIQUE(arriendo_id,revision), UNIQUE(id,arriendo_id),
        FOREIGN KEY(confirmacion_id,arriendo_id) REFERENCES public.arriendo_temprano_confirmacion(id,arriendo_id) ON DELETE RESTRICT,
        FOREIGN KEY(arriendo_id,pago_id) REFERENCES public.arriendo_temprano_pago(arriendo_id,pago_id) ON DELETE RESTRICT,
        CHECK (secuencia=revision),
        CHECK ((tipo='oferta_creada' AND secuencia=1 AND estado_anterior IS NULL AND estado_nuevo='pendiente_identidad')
            OR (tipo<>'oferta_creada' AND secuencia>1 AND estado_anterior IS NOT NULL)),
        CHECK ((tipo='oferta_vencida' AND actor_id IS NULL) OR (tipo<>'oferta_vencida' AND actor_id IS NOT NULL))
    );
    CREATE FUNCTION public.arriendo_temprano_evento_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $fn$
    DECLARE host_id uuid; renter_id uuid; last_seq bigint; last_state text;
        confirmation_actor uuid; confirmation_kind text; other_confirmed boolean; expected_state text;
    BEGIN
        IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'events are append-only' USING ERRCODE='55000'; END IF;
        SELECT anfitrion_id,arrendatario_id INTO host_id,renter_id FROM public.arriendo_temprano WHERE id=NEW.arriendo_id FOR UPDATE;
        IF NOT FOUND OR (NEW.actor_id IS NOT NULL AND NEW.actor_id NOT IN(host_id,renter_id)) THEN
            RAISE EXCEPTION 'event actor is not a participant' USING ERRCODE='23514';
        END IF;
        IF (NEW.tipo IN ('oferta_creada','pago_marcado_enviado') AND NEW.actor_id IS DISTINCT FROM renter_id)
            OR (NEW.tipo='pago_marcado_recibido' AND NEW.actor_id IS DISTINCT FROM host_id) THEN
            RAISE EXCEPTION 'event actor has the wrong participant role' USING ERRCODE='23514';
        END IF;
        IF EXISTS(SELECT 1 FROM public.arriendo_temprano_evento WHERE arriendo_id=NEW.arriendo_id AND secuencia=NEW.secuencia) THEN
            RAISE EXCEPTION 'duplicate event sequence' USING ERRCODE='23505';
        END IF;
        SELECT secuencia,estado_nuevo INTO last_seq,last_state FROM public.arriendo_temprano_evento
            WHERE arriendo_id=NEW.arriendo_id ORDER BY secuencia DESC LIMIT 1;
        IF NEW.secuencia<>COALESCE(last_seq,0)+1 OR NEW.estado_anterior IS DISTINCT FROM last_state THEN
            RAISE EXCEPTION 'event stream gap or prior state mismatch' USING ERRCODE='23514';
        END IF;
        IF NEW.tipo IN ('identidad_simulada_confirmada','contrato_aceptado') THEN
            SELECT actor_id,clase INTO confirmation_actor,confirmation_kind FROM public.arriendo_temprano_confirmacion
                WHERE id=NEW.confirmacion_id AND arriendo_id=NEW.arriendo_id;
            IF NOT FOUND OR confirmation_actor IS DISTINCT FROM NEW.actor_id
                OR confirmation_kind IS DISTINCT FROM (CASE NEW.tipo WHEN 'identidad_simulada_confirmada' THEN 'identidad' ELSE 'contrato' END)
                OR NEW.pago_id IS NOT NULL THEN
                RAISE EXCEPTION 'event confirmation evidence mismatch' USING ERRCODE='23514';
            END IF;
            IF EXISTS(SELECT 1 FROM public.arriendo_temprano_evento WHERE arriendo_id=NEW.arriendo_id AND tipo=NEW.tipo AND actor_id=NEW.actor_id) THEN
                RAISE EXCEPTION 'participant confirmation already recorded' USING ERRCODE='23505';
            END IF;
            SELECT EXISTS(SELECT 1 FROM public.arriendo_temprano_evento WHERE arriendo_id=NEW.arriendo_id
                AND tipo=NEW.tipo AND actor_id<>NEW.actor_id) INTO other_confirmed;
            IF NEW.tipo='identidad_simulada_confirmada' THEN
                expected_state := CASE WHEN other_confirmed THEN 'pendiente_contrato' ELSE 'pendiente_identidad' END;
            ELSE
                expected_state := CASE WHEN other_confirmed THEN 'pendiente_envio_pago' ELSE 'pendiente_contrato' END;
            END IF;
            IF NEW.estado_nuevo IS DISTINCT FROM expected_state THEN
                RAISE EXCEPTION 'phase requires both participant confirmations' USING ERRCODE='23514';
            END IF;
        ELSIF NEW.confirmacion_id IS NOT NULL THEN
            RAISE EXCEPTION 'unexpected confirmation reference' USING ERRCODE='23514';
        END IF;
        IF NEW.tipo IN ('pago_marcado_enviado','pago_marcado_recibido') THEN
            IF NEW.pago_id IS NULL OR NOT EXISTS(SELECT 1 FROM public.arriendo_temprano_pago
                WHERE arriendo_id=NEW.arriendo_id AND pago_id=NEW.pago_id AND enviado_en IS NOT NULL
                  AND (NEW.tipo<>'pago_marcado_recibido' OR recibido_en IS NOT NULL)) THEN
                RAISE EXCEPTION 'event payment evidence missing' USING ERRCODE='23514';
            END IF;
        ELSIF NEW.tipo NOT IN ('oferta_cancelada','oferta_vencida') AND NEW.pago_id IS NOT NULL THEN
            RAISE EXCEPTION 'unexpected payment reference' USING ERRCODE='23514';
        END IF;
        IF (NEW.tipo='identidad_simulada_confirmada' AND NOT (NEW.estado_anterior='pendiente_identidad' AND NEW.estado_nuevo IN ('pendiente_identidad','pendiente_contrato')))
            OR (NEW.tipo='contrato_aceptado' AND NOT (NEW.estado_anterior='pendiente_contrato' AND NEW.estado_nuevo IN ('pendiente_contrato','pendiente_envio_pago')))
            OR (NEW.tipo='pago_marcado_enviado' AND NOT (NEW.estado_anterior='pendiente_envio_pago' AND NEW.estado_nuevo='pendiente_recepcion_pago'))
            OR (NEW.tipo='pago_marcado_recibido' AND NOT (NEW.estado_anterior='pendiente_recepcion_pago' AND NEW.estado_nuevo='confirmado'))
            OR (NEW.tipo='oferta_rechazada' AND NOT (NEW.estado_anterior IN ('pendiente_identidad','pendiente_contrato') AND NEW.estado_nuevo='rechazado'))
            OR (NEW.tipo IN ('oferta_cancelada','oferta_vencida') AND NOT (NEW.estado_anterior IN ('pendiente_identidad','pendiente_contrato','pendiente_envio_pago','pendiente_recepcion_pago')
                AND NEW.estado_nuevo=(CASE NEW.tipo WHEN 'oferta_cancelada' THEN 'cancelado' ELSE 'vencido' END))) THEN
            RAISE EXCEPTION 'event type and transition mismatch' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END $fn$;
    CREATE TRIGGER arriendo_temprano_evento_guard BEFORE INSERT OR UPDATE OR DELETE ON public.arriendo_temprano_evento FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_guard();

    CREATE FUNCTION public.arriendo_temprano_evento_consistency() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $fn$
    DECLARE offer_id uuid; offer_revision bigint; offer_state text; event_count bigint; last_seq bigint; last_state text;
    BEGIN
        IF TG_TABLE_NAME='arriendo_temprano' THEN offer_id := NEW.id; ELSE offer_id := NEW.arriendo_id; END IF;
        SELECT revision,estado INTO offer_revision,offer_state FROM public.arriendo_temprano WHERE id=offer_id FOR UPDATE;
        IF NOT FOUND THEN RETURN NULL; END IF;
        SELECT count(*),COALESCE(max(secuencia),0) INTO event_count,last_seq FROM public.arriendo_temprano_evento WHERE arriendo_id=offer_id;
        SELECT estado_nuevo INTO last_state FROM public.arriendo_temprano_evento WHERE arriendo_id=offer_id ORDER BY secuencia DESC LIMIT 1;
        IF event_count IS DISTINCT FROM offer_revision OR last_seq IS DISTINCT FROM offer_revision OR last_state IS DISTINCT FROM offer_state THEN
            RAISE EXCEPTION 'event stream and aggregate revision/state mismatch'
                USING ERRCODE='23514', CONSTRAINT='arriendo_temprano_evento_consistencia_ck';
        END IF;
        RETURN NULL;
    END $fn$;
    CREATE CONSTRAINT TRIGGER arriendo_temprano_evento_consistency
        AFTER INSERT ON public.arriendo_temprano_evento DEFERRABLE INITIALLY DEFERRED
        FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_consistency();
    CREATE CONSTRAINT TRIGGER arriendo_temprano_evento_revision_consistency
        AFTER INSERT OR UPDATE ON public.arriendo_temprano DEFERRABLE INITIALLY DEFERRED
        FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_consistency();
    REVOKE ALL ON FUNCTION public.arriendo_temprano_evento_consistency() FROM PUBLIC;

    CREATE TABLE public.arriendo_temprano_idempotencia (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        arriendo_id uuid NOT NULL REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT,
        actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        comando text NOT NULL CHECK (comando IN ('crear_oferta','confirmar_identidad_simulada','aceptar_contrato','marcar_pago_enviado','marcar_pago_recibido','rechazar','cancelar')),
        clave text NOT NULL CHECK (char_length(clave) BETWEEN 8 AND 200),
        huella bytea NOT NULL CHECK (octet_length(huella)=32),
        evento_id uuid NOT NULL,
        creado_en timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(creado_en)),
        UNIQUE(actor_id,arriendo_id,comando,clave),
        FOREIGN KEY(evento_id,arriendo_id) REFERENCES public.arriendo_temprano_evento(id,arriendo_id) ON DELETE RESTRICT
    );
    CREATE FUNCTION public.arriendo_temprano_idempotencia_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $fn$
    DECLARE host_id uuid; renter_id uuid; event_kind text; event_actor uuid;
    BEGIN
        IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'idempotency rows are immutable' USING ERRCODE='55000'; END IF;
        SELECT anfitrion_id,arrendatario_id INTO host_id,renter_id FROM public.arriendo_temprano WHERE id=NEW.arriendo_id;
        IF NOT FOUND OR NEW.actor_id NOT IN(host_id,renter_id) THEN RAISE EXCEPTION 'actor is not a participant' USING ERRCODE='23514'; END IF;
        SELECT tipo,actor_id INTO event_kind,event_actor FROM public.arriendo_temprano_evento
            WHERE id=NEW.evento_id AND arriendo_id=NEW.arriendo_id;
        IF NOT FOUND OR event_actor IS DISTINCT FROM NEW.actor_id OR event_kind IS DISTINCT FROM
            (CASE NEW.comando
                WHEN 'crear_oferta' THEN 'oferta_creada'
                WHEN 'confirmar_identidad_simulada' THEN 'identidad_simulada_confirmada'
                WHEN 'aceptar_contrato' THEN 'contrato_aceptado'
                WHEN 'marcar_pago_enviado' THEN 'pago_marcado_enviado'
                WHEN 'marcar_pago_recibido' THEN 'pago_marcado_recibido'
                WHEN 'rechazar' THEN 'oferta_rechazada'
                WHEN 'cancelar' THEN 'oferta_cancelada'
            END) THEN
            RAISE EXCEPTION 'idempotency result actor or command mismatch' USING ERRCODE='23514';
        END IF;
        IF EXISTS(SELECT 1 FROM public.arriendo_temprano_idempotencia i WHERE i.actor_id=NEW.actor_id AND i.arriendo_id=NEW.arriendo_id AND i.comando=NEW.comando AND i.clave=NEW.clave AND i.huella<>NEW.huella) THEN
            RAISE EXCEPTION 'idempotency key payload mismatch' USING ERRCODE='23505';
        END IF;
        RETURN NEW;
    END $fn$;
    CREATE TRIGGER arriendo_temprano_idempotencia_guard BEFORE INSERT OR UPDATE OR DELETE ON public.arriendo_temprano_idempotencia FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_idempotencia_guard();

    REVOKE ALL ON TABLE public.arriendo_temprano_confirmacion, public.arriendo_temprano_pago, public.arriendo_temprano_evento, public.arriendo_temprano_idempotencia FROM PUBLIC;
    GRANT SELECT, INSERT ON TABLE public.arriendo_temprano_confirmacion, public.arriendo_temprano_evento TO espacigo_runtime;
    GRANT SELECT, INSERT, UPDATE ON TABLE public.arriendo_temprano_pago TO espacigo_runtime;
    GRANT SELECT, INSERT ON TABLE public.arriendo_temprano_idempotencia TO espacigo_runtime;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_confirmacion_guard() FROM PUBLIC;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_pago_guard() FROM PUBLIC;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_evento_guard() FROM PUBLIC;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_idempotencia_guard() FROM PUBLIC;
    RESET ROLE;
END
$migration$;