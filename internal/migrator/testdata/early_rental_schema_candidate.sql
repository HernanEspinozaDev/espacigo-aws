DO $migration$
DECLARE
    original_role text := current_user;
BEGIN
    IF session_user <> current_user THEN
        RAISE EXCEPTION 'candidate requires session/effective role equality before owner switching';
    END IF;
    IF NOT pg_catalog.pg_has_role(session_user, 'espacigo_schema_owner', 'SET') THEN
        RAISE EXCEPTION 'session user cannot SET ROLE espacigo_schema_owner';
    END IF;

    SET ROLE espacigo_schema_owner;
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
    RESET ROLE;

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

    SET ROLE espacigo_schema_owner;
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
    RESET ROLE;
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
END
$migration$;
