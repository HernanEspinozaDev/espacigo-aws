DO $migration$
BEGIN
DECLARE
    original_role text := current_user;
BEGIN
    IF session_user <> current_user AND current_user <> 'espacigo_schema_owner' THEN
        RAISE EXCEPTION 'candidate requires session/effective role equality before owner switching';
    END IF;
    IF NOT pg_catalog.pg_has_role(session_user, 'espacigo_schema_owner', 'SET') THEN
        RAISE EXCEPTION 'session user cannot SET ROLE espacigo_schema_owner';
    END IF;

    SET LOCAL ROLE espacigo_schema_owner;
    CREATE TABLE public.arriendo_temprano (
        id uuid PRIMARY KEY,
        flow_version text NOT NULL DEFAULT 'manual_early_v1' CHECK (flow_version = 'manual_early_v1'),
        espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
        anfitrion_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        arrendatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
        titulo_snapshot text NOT NULL CHECK (length(btrim(titulo_snapshot)) BETWEEN 1 AND 70),
        modalidad text NOT NULL CHECK (modalidad = 'hora'),
        precio_unitario_clp bigint NOT NULL CHECK (precio_unitario_clp >= 5001),
        moneda char(3) NOT NULL CHECK (moneda = 'CLP'),
        unidades bigint NOT NULL CHECK (unidades IN (1,2,4)),
        subtotal_clp bigint NOT NULL CHECK (subtotal_clp > 0 AND subtotal_clp::numeric = precio_unitario_clp::numeric * unidades),
        inicio timestamptz NOT NULL,
        termino timestamptz NOT NULL,
        intervalo tstzrange GENERATED ALWAYS AS (tstzrange(inicio, termino, '[)')) STORED,
        zona_horaria text NOT NULL CHECK (length(btrim(zona_horaria)) BETWEEN 1 AND 100 AND zona_horaria <> 'Local'),
        contrato_version bigint NOT NULL CHECK (contrato_version > 0),
        contrato_texto text NOT NULL CHECK (length(contrato_texto) BETWEEN 1 AND 20000),
        contrato_digest bytea NOT NULL CHECK (octet_length(contrato_digest) = 32),
        estado text NOT NULL CHECK (estado IN ('pendiente_identidad','pendiente_contrato','pendiente_envio_pago','pendiente_recepcion_pago','confirmado','rechazado','cancelado','vencido')),
        revision bigint NOT NULL CHECK (revision >= 1),
        creada_en timestamptz NOT NULL,
        actualizada_en timestamptz NOT NULL,
        vence_en timestamptz NOT NULL,
        ocupacion_id uuid NOT NULL UNIQUE,
        CONSTRAINT arriendo_temprano_participantes_ck CHECK (anfitrion_id <> arrendatario_id),
        CONSTRAINT arriendo_temprano_intervalo_ck CHECK (
            isfinite(inicio) AND isfinite(termino) AND termino > inicio
            AND termino - inicio = unidades * interval '1 hour'
        ),
        CONSTRAINT arriendo_temprano_vencimiento_ck CHECK (
            isfinite(creada_en) AND isfinite(actualizada_en) AND isfinite(vence_en)
            AND vence_en > creada_en AND vence_en <= inicio
            AND vence_en <= creada_en + interval '30 minutes' AND actualizada_en >= creada_en
        ),
        CONSTRAINT arriendo_temprano_digest_ck CHECK (contrato_digest = pg_catalog.sha256(convert_to(contrato_texto, 'UTF8'))),
        CONSTRAINT arriendo_temprano_id_space_interval_uk UNIQUE (id, espacio_id, intervalo, ocupacion_id)
    );
    EXECUTE pg_catalog.format('SET LOCAL ROLE %I', original_role);

    ALTER TABLE public.ocupacion ADD COLUMN arriendo_temprano_id uuid;
    ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_check;
    ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_check CHECK (
        tipo IN ('bloqueo_manual','retencion','reserva','retencion_temprana','reserva_temprana')
    );
    ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_reserva_ck;
    ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_reserva_ck CHECK (
        (tipo = 'bloqueo_manual' AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND expira_en IS NULL
            AND motivo IS NOT NULL AND length(btrim(motivo)) BETWEEN 1 AND 500)
        OR (tipo IN ('retencion', 'reserva') AND reserva_id IS NOT NULL AND arriendo_temprano_id IS NULL
            AND (tipo <> 'retencion' OR expira_en IS NOT NULL))
        OR (tipo IN ('retencion_temprana', 'reserva_temprana') AND reserva_id IS NULL AND arriendo_temprano_id IS NOT NULL
            AND motivo IS NULL AND (tipo <> 'retencion_temprana' OR expira_en IS NOT NULL))
    );
    ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_temprana_tipo_expiracion_ck CHECK (
        tipo NOT IN ('retencion_temprana','reserva_temprana')
        OR (tipo = 'retencion_temprana' AND expira_en IS NOT NULL
            AND isfinite(expira_en) AND expira_en <= lower(intervalo))
        OR (tipo = 'reserva_temprana' AND expira_en IS NULL)
    );
    ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_id_arriendo_space_interval_uk
        UNIQUE (arriendo_temprano_id, espacio_id, intervalo, id);
    ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_temprana_agregado_fk
        FOREIGN KEY (arriendo_temprano_id, espacio_id, intervalo, id)
        REFERENCES public.arriendo_temprano (id, espacio_id, intervalo, ocupacion_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
    ALTER TABLE public.arriendo_temprano ADD CONSTRAINT arriendo_temprano_ocupacion_fk
        FOREIGN KEY (id, espacio_id, intervalo, ocupacion_id)
        REFERENCES public.ocupacion (arriendo_temprano_id, espacio_id, intervalo, id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

    SET LOCAL ROLE espacigo_schema_owner;
    CREATE FUNCTION public.arriendo_temprano_immutable_update()
    RETURNS trigger
    LANGUAGE plpgsql
    SET search_path = pg_catalog, public
    AS $function$
    BEGIN
        IF (NEW.id, NEW.flow_version, NEW.espacio_id, NEW.anfitrion_id, NEW.arrendatario_id,
            NEW.titulo_snapshot, NEW.modalidad, NEW.precio_unitario_clp, NEW.moneda, NEW.unidades,
            NEW.subtotal_clp, NEW.inicio, NEW.termino, NEW.zona_horaria, NEW.contrato_version,
            NEW.contrato_texto, NEW.contrato_digest, NEW.creada_en, NEW.vence_en, NEW.ocupacion_id)
           IS DISTINCT FROM
           (OLD.id, OLD.flow_version, OLD.espacio_id, OLD.anfitrion_id, OLD.arrendatario_id,
            OLD.titulo_snapshot, OLD.modalidad, OLD.precio_unitario_clp, OLD.moneda, OLD.unidades,
            OLD.subtotal_clp, OLD.inicio, OLD.termino, OLD.zona_horaria, OLD.contrato_version,
            OLD.contrato_texto, OLD.contrato_digest, OLD.creada_en, OLD.vence_en, OLD.ocupacion_id) THEN
            RAISE EXCEPTION 'early rental snapshot, parts, interval and deadline are immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'arriendo_temprano_inmutable_ck';
        END IF;
        IF NEW.estado IS DISTINCT FROM OLD.estado THEN
            IF NEW.revision <> OLD.revision + 1 THEN
                RAISE EXCEPTION 'state change must increment revision exactly once'
                    USING ERRCODE = '23514', CONSTRAINT = 'arriendo_temprano_revision_ck';
            END IF;
        ELSIF NEW.revision < OLD.revision THEN
            RAISE EXCEPTION 'revision cannot decrease'
                USING ERRCODE = '23514', CONSTRAINT = 'arriendo_temprano_revision_ck';
        END IF;
        RETURN NEW;
    END
    $function$;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_immutable_update() FROM PUBLIC;
    CREATE TRIGGER arriendo_temprano_immutable_update_trg
        BEFORE UPDATE ON public.arriendo_temprano
        FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_immutable_update();

    CREATE FUNCTION public.arriendo_temprano_occupancy_consistency()
    RETURNS trigger
    LANGUAGE plpgsql
    SET search_path = pg_catalog, public
    AS $function$
    DECLARE
        offer_id uuid;
        occupancy_id uuid;
        offer_state text;
        occupancy_type text;
        occupancy_expiry timestamptz;
        occupancy_active boolean;
        offer_deadline timestamptz;
    BEGIN
    IF TG_TABLE_NAME = 'arriendo_temprano' THEN
            IF TG_OP = 'DELETE' THEN
                offer_id := OLD.id;
                occupancy_id := OLD.ocupacion_id;
            ELSE
                offer_id := NEW.id;
                occupancy_id := NEW.ocupacion_id;
            END IF;
        ELSE
            IF TG_OP = 'DELETE' THEN
                offer_id := OLD.arriendo_temprano_id;
                occupancy_id := OLD.id;
            ELSE
                offer_id := NEW.arriendo_temprano_id;
                occupancy_id := NEW.id;
            END IF;
        END IF;
        SELECT a.estado, a.vence_en, o.tipo, o.expira_en, o.activo
          INTO offer_state, offer_deadline, occupancy_type, occupancy_expiry, occupancy_active
          FROM public.arriendo_temprano AS a
          JOIN public.ocupacion AS o ON o.arriendo_temprano_id = a.id
             AND o.id = a.ocupacion_id AND o.espacio_id = a.espacio_id AND o.intervalo = a.intervalo
         WHERE a.id = offer_id AND o.id = occupancy_id;
        IF NOT FOUND THEN
            RETURN NULL;
        END IF;
        IF (offer_state IN ('pendiente_identidad','pendiente_contrato','pendiente_envio_pago','pendiente_recepcion_pago')
                AND NOT (occupancy_type = 'retencion_temprana' AND occupancy_active AND occupancy_expiry = offer_deadline))
           OR (offer_state = 'confirmado'
                AND NOT (occupancy_type = 'reserva_temprana' AND occupancy_active AND occupancy_expiry IS NULL))
           OR (offer_state IN ('rechazado','cancelado','vencido')
                AND NOT (occupancy_type = 'retencion_temprana' AND NOT occupancy_active AND occupancy_expiry = offer_deadline)) THEN
            RAISE EXCEPTION 'early rental state and occupancy are inconsistent'
                USING ERRCODE = '23514', CONSTRAINT = 'arriendo_temprano_ocupacion_estado_ck';
        END IF;
        RETURN NULL;
    END
    $function$;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_occupancy_consistency() FROM PUBLIC;
    EXECUTE pg_catalog.format('SET LOCAL ROLE %I', original_role);
    CREATE CONSTRAINT TRIGGER arriendo_temprano_occupancy_consistency_trg
        AFTER INSERT OR UPDATE ON public.arriendo_temprano
        DEFERRABLE INITIALLY DEFERRED
        FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_occupancy_consistency();
    CREATE CONSTRAINT TRIGGER ocupacion_arriendo_temprano_consistency_trg
        AFTER INSERT OR UPDATE ON public.ocupacion
        DEFERRABLE INITIALLY DEFERRED
        FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_occupancy_consistency();

    GRANT SELECT, INSERT, UPDATE ON TABLE public.arriendo_temprano TO espacigo_runtime;
    IF current_user <> original_role THEN
        RAISE EXCEPTION 'candidate failed to restore original role';
    END IF;
END;
DECLARE
    original_role text := current_user;
BEGIN
    IF current_schema() <> 'public' OR (session_user <> original_role AND original_role <> 'espacigo_schema_owner')
       OR NOT pg_catalog.pg_has_role(session_user, 'espacigo_schema_owner', 'SET') THEN
        RAISE EXCEPTION 'unexpected migration role or schema';
    END IF;
    SET LOCAL ROLE espacigo_schema_owner;

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
    DECLARE host_id uuid; renter_id uuid; last_seq bigint; last_state text; offer_deadline timestamptz;
        confirmation_actor uuid; confirmation_kind text; other_confirmed boolean; expected_state text;
    BEGIN
        IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'events are append-only' USING ERRCODE='55000'; END IF;
        SELECT anfitrion_id,arrendatario_id,vence_en INTO host_id,renter_id,offer_deadline FROM public.arriendo_temprano WHERE id=NEW.arriendo_id FOR UPDATE;
        IF NOT FOUND OR (NEW.actor_id IS NOT NULL AND NEW.actor_id NOT IN(host_id,renter_id)) THEN
            RAISE EXCEPTION 'event actor is not a participant' USING ERRCODE='23514';
        END IF;
        IF NEW.tipo='oferta_vencida' AND NEW.ocurrido_en<offer_deadline THEN
            RAISE EXCEPTION 'expiration event precedes immutable deadline' USING ERRCODE='23514', CONSTRAINT='arriendo_temprano_evento_vencimiento_ck';
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
    EXECUTE pg_catalog.format('SET LOCAL ROLE %I', original_role);
END;

    ALTER TABLE public.arriendo_temprano_evento ADD COLUMN motivo_codigo text;
    ALTER TABLE public.arriendo_temprano_evento ADD CONSTRAINT arriendo_temprano_evento_motivo_ck CHECK (
        (tipo IN ('oferta_rechazada','oferta_cancelada') AND motivo_codigo IS NOT NULL
            AND motivo_codigo IN ('condiciones_no_aceptadas','cambio_de_planes','error_en_la_oferta','otro'))
        OR (tipo NOT IN ('oferta_rechazada','oferta_cancelada') AND motivo_codigo IS NULL)
    );
    REVOKE ALL ON TABLE public.arriendo_temprano, public.arriendo_temprano_confirmacion,
        public.arriendo_temprano_pago, public.arriendo_temprano_evento,
        public.arriendo_temprano_idempotencia FROM PUBLIC, espacigo_runtime;
    GRANT SELECT, INSERT, UPDATE ON TABLE public.arriendo_temprano, public.arriendo_temprano_pago TO espacigo_runtime;
    GRANT SELECT, INSERT ON TABLE public.arriendo_temprano_confirmacion, public.arriendo_temprano_evento,
        public.arriendo_temprano_idempotencia TO espacigo_runtime;
    REVOKE ALL ON FUNCTION public.arriendo_temprano_immutable_update(), public.arriendo_temprano_occupancy_consistency(),
        public.arriendo_temprano_confirmacion_guard(), public.arriendo_temprano_pago_guard(),
        public.arriendo_temprano_evento_guard(), public.arriendo_temprano_evento_consistency(),
        public.arriendo_temprano_idempotencia_guard() FROM PUBLIC, espacigo_runtime;
END
$migration$;
