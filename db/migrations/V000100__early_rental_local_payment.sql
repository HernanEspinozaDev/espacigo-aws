-- Preserve manual receipts and admit a receipt from the local payment processor.
SET LOCAL ROLE espacigo_schema_owner;

ALTER TABLE public.arriendo_temprano_evento
    ADD COLUMN procesador_referencia text,
    DROP CONSTRAINT arriendo_temprano_evento_check2,
    ADD CONSTRAINT arriendo_temprano_evento_actor_ck CHECK (
        (procesador_referencia IS NULL AND ((tipo = 'oferta_vencida') = (actor_id IS NULL)))
        OR (procesador_referencia IS NOT NULL AND tipo = 'pago_marcado_recibido'
            AND actor_id IS NULL AND pago_id IS NOT NULL
            AND procesador_referencia = 'localfake:' || pago_id::text || ':exito_simulado')
    );

CREATE OR REPLACE FUNCTION public.arriendo_temprano_evento_guard() RETURNS trigger
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
            OR (NEW.tipo='pago_marcado_recibido' AND NEW.procesador_referencia IS NULL
                AND NEW.actor_id IS DISTINCT FROM host_id) THEN
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
