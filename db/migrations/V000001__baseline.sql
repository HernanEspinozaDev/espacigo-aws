-- Esquema consolidado de las migraciones V1 a V29 y sus catálogos de referencia.
--
-- PostgreSQL database dump
--


-- Dumped from database version 18.6 (Debian 18.6-1.pgdg13+2)
-- Dumped by pg_dump version 18.6 (Debian 18.6-1.pgdg13+2)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--



--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--



--
-- Name: account_profile_identity_status(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.account_profile_identity_status(subject uuid, as_of timestamp with time zone) RETURNS TABLE(complete boolean, verified boolean)
    LANGUAGE sql STABLE
    SET search_path TO 'pg_catalog'
    AS $$
    SELECT p.account_id IS NOT NULL,
           COALESCE(v.state = 'verified' AND v.verified_at <= as_of AND v.verified_until > as_of, false)
    FROM public.usuario AS u
    LEFT JOIN public.account_identity_profile AS p ON p.account_id = u.id
    LEFT JOIN LATERAL (
        SELECT a.state,a.verified_at,a.verified_until
        FROM public.account_identity_verification AS a
        WHERE a.account_id = u.id AND a.profile_revision = p.current_revision
        ORDER BY a.started_at DESC,a.id DESC LIMIT 1
    ) AS v ON true
    WHERE u.id = subject AND u.estado = 'activo';
$$;


--
-- Name: arriendo_temprano_confirmacion_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_confirmacion_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    END $$;


--
-- Name: arriendo_temprano_evento_consistency(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_evento_consistency() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    END $$;


--
-- Name: arriendo_temprano_evento_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_evento_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    END $$;


--
-- Name: arriendo_temprano_idempotencia_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_idempotencia_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    END $$;


--
-- Name: arriendo_temprano_immutable_update(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_immutable_update() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    $$;


--
-- Name: arriendo_temprano_occupancy_consistency(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_occupancy_consistency() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    $$;


--
-- Name: arriendo_temprano_pago_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.arriendo_temprano_pago_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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
    END $$;


--
-- Name: assign_local_space_history_retention(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.assign_local_space_history_retention() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
  NEW.retirar_en := NEW.ocurrida_en + interval '24 months';
  PERFORM set_config('app.local_retention_purge','on',true);
  UPDATE public.espacio_publicacion_historial_local
     SET retirar_en=NEW.retirar_en WHERE espacio_id=NEW.espacio_id;
  PERFORM set_config('app.local_retention_purge','off',true);
  RETURN NEW;
END;
$$;


--
-- Name: build_public_search_document(text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.build_public_search_document(p_status text, p_category text, p_title text, p_description text, p_location_version text, p_region_code text, p_commune_code text) RETURNS tsvector
    LANGUAGE plpgsql STABLE PARALLEL SAFE
    AS $$
DECLARE
    category_name text;
    region_name text;
    commune_name text;
    text_value text;
BEGIN
    IF p_status <> 'activa' THEN RETURN ''::tsvector; END IF;
    SELECT c.nombre INTO category_name FROM public.categoria_espacio c WHERE c.codigo=p_category;
    IF p_location_version IS NOT NULL AND p_region_code IS NOT NULL AND p_commune_code IS NOT NULL THEN
        SELECT r.nombre, m.nombre INTO region_name, commune_name
        FROM public.region_publica r
        JOIN public.comuna_publica m
          ON m.catalogo_version=r.catalogo_version AND m.region_codigo=r.codigo
        WHERE r.catalogo_version=p_location_version
          AND r.codigo=p_region_code AND m.codigo=p_commune_code;
    END IF;
    text_value := normalize(regexp_replace(concat_ws(' ',p_title,p_description,category_name,p_category,region_name,commune_name), '[[:space:]]+', ' ', 'g'), NFC);
    RETURN to_tsvector('public.espacigo_es_unaccent'::regconfig, text_value);
END;
$$;


--
-- Name: claim_expired_local_verification_files(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.claim_expired_local_verification_files(p_at timestamp with time zone, p_limit integer) RETURNS TABLE(evidencia_id uuid, archivo_id uuid)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
  IF p_limit<1 OR p_limit>500 THEN RAISE EXCEPTION 'invalid batch'; END IF;
  RETURN QUERY
  WITH due AS (
    SELECT j.evidencia_id FROM public.verificacion_archivo_retiro_pendiente_local j
    WHERE j.disponible_en<=p_at AND (j.lease_hasta IS NULL OR j.lease_hasta<=p_at)
    ORDER BY j.disponible_en,j.evidencia_id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ), claimed AS (
    UPDATE public.verificacion_archivo_retiro_pendiente_local j
       SET lease_hasta=p_at+interval '5 minutes',intentos=intentos+1
      FROM due WHERE j.evidencia_id=due.evidencia_id
    RETURNING j.evidencia_id,j.archivo_id
  ) SELECT claimed.evidencia_id,claimed.archivo_id FROM claimed;
END;
$$;


--
-- Name: claim_private_media_cleanup(integer, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.claim_private_media_cleanup(p_limit integer, p_at timestamp with time zone) RETURNS TABLE(id text, fencing_token bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    SET "TimeZone" TO 'UTC'
    AS $$
DECLARE
    v_now timestamptz := pg_catalog.clock_timestamp();
BEGIN
    IF pg_catalog.current_setting('role', true) IS DISTINCT FROM 'espacigo_privacy_cleanup_worker' THEN
        RAISE EXCEPTION 'privacy cleanup worker role is required' USING ERRCODE = '42501';
    END IF;
    IF p_limit IS NULL OR p_limit < 1 OR p_limit > 100 OR p_at IS NULL OR p_at > v_now + interval '1 minute' THEN
        RAISE EXCEPTION 'valid cleanup limit and timestamp are required' USING ERRCODE = '22023';
    END IF;
    RETURN QUERY
    WITH claimed AS (
        SELECT a.id FROM public.media_asset a
         WHERE (a.estado='deleting' AND (a.lease_until IS NULL OR a.lease_until <= v_now))
            OR (a.estado='pending' AND a.lease_until <= v_now)
         ORDER BY a.updated_at,a.id FOR UPDATE SKIP LOCKED LIMIT p_limit
    ), changed AS (
        UPDATE public.media_asset a
           SET estado='deleting',fencing_token=a.fencing_token+1,
               lease_until=v_now+interval '5 minutes',updated_at=v_now
          FROM claimed c WHERE a.id=c.id
        RETURNING a.id::text,a.fencing_token
    ) SELECT changed.id,changed.fencing_token FROM changed;
END
$$;


--
-- Name: clear_commercial_space_pin_provenance(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.clear_commercial_space_pin_provenance() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
    IF ROW(NEW.direccion,NEW.ubicacion_catalogo_version,NEW.region_codigo,NEW.comuna_codigo,
           NEW.ubicacion_latitud,NEW.ubicacion_longitud)
       IS DISTINCT FROM
       ROW(OLD.direccion,OLD.ubicacion_catalogo_version,OLD.region_codigo,OLD.comuna_codigo,
           OLD.ubicacion_latitud,OLD.ubicacion_longitud) THEN
        NEW.ubicacion_pin_origen := NULL;
        NEW.ubicacion_pin_precision := NULL;
    END IF;
    RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: identity_provider_attempt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_provider_attempt (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    declaration_id uuid,
    provider character varying(16) NOT NULL,
    environment character varying(8) NOT NULL,
    application_id character varying(128) NOT NULL,
    workflow_id character varying(128) NOT NULL,
    workflow_version character varying(64) NOT NULL,
    idempotency_key uuid NOT NULL,
    correlation_id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_session_id character varying(128),
    status character varying(24) DEFAULT 'CREATED'::character varying NOT NULL,
    decision_key_version character varying(64),
    decision_algorithm character varying(16),
    encrypted_decision bytea,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    retention_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_provider_attempt_application_id_check CHECK (((length((application_id)::text) >= 1) AND (length((application_id)::text) <= 128))),
    CONSTRAINT identity_provider_attempt_check CHECK ((((encrypted_decision IS NULL) AND (decision_key_version IS NULL) AND (decision_algorithm IS NULL)) OR ((encrypted_decision IS NOT NULL) AND (decision_key_version IS NOT NULL) AND (decision_algorithm IS NOT NULL) AND ((decision_algorithm)::text = 'AES-256-GCM/v1'::text) AND ((decision_key_version)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'::text) AND ((octet_length(encrypted_decision) >= 29) AND (octet_length(encrypted_decision) <= 65564))))),
    CONSTRAINT identity_provider_attempt_check1 CHECK ((retention_at > created_at)),
    CONSTRAINT identity_provider_attempt_check2 CHECK ((updated_at >= created_at)),
    CONSTRAINT identity_provider_attempt_detached_approved_ck CHECK (((declaration_id IS NOT NULL) OR (((status)::text = ANY (ARRAY[('APPROVED'::character varying)::text, ('REJECTED'::character varying)::text, ('EXPIRED'::character varying)::text])) AND (decision_key_version IS NULL) AND (decision_algorithm IS NULL) AND (encrypted_decision IS NULL)))),
    CONSTRAINT identity_provider_attempt_environment_check CHECK (((environment)::text = ANY (ARRAY[('SANDBOX'::character varying)::text, ('LIVE'::character varying)::text]))),
    CONSTRAINT identity_provider_attempt_provider_check CHECK (((provider)::text = 'didit'::text)),
    CONSTRAINT identity_provider_attempt_provider_session_id_check CHECK (((provider_session_id IS NULL) OR (length(btrim((provider_session_id)::text)) > 0))),
    CONSTRAINT identity_provider_attempt_status_check CHECK (((status)::text = ANY (ARRAY[('CREATED'::character varying)::text, ('PENDING_USER_INPUT'::character varying)::text, ('PROCESSING'::character varying)::text, ('APPROVED'::character varying)::text, ('IN_REVIEW'::character varying)::text, ('REJECTED'::character varying)::text, ('EXPIRED'::character varying)::text, ('FAILED'::character varying)::text]))),
    CONSTRAINT identity_provider_attempt_workflow_id_check CHECK (((length((workflow_id)::text) >= 1) AND (length((workflow_id)::text) <= 128))),
    CONSTRAINT identity_provider_attempt_workflow_version_check CHECK (((length((workflow_version)::text) >= 1) AND (length((workflow_version)::text) <= 64)))
);


--
-- Name: commit_verified_identity_decision(uuid, uuid, text, text, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.commit_verified_identity_decision(p_attempt_id uuid, p_event_row_id uuid, p_rut_fingerprint text, p_session_id text, p_processed_at timestamp with time zone) RETURNS public.identity_provider_attempt
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $_$
DECLARE
    v_attempt public.identity_provider_attempt%ROWTYPE;
    v_event public.identity_provider_event%ROWTYPE;
    v_claim public.identity_rut_claim%ROWTYPE;
    v_declaration_id uuid;
    v_account_id uuid;
    v_processed_at timestamptz;
    v_dedup_key text;
    v_rows bigint;
    v_related record;
BEGIN
    IF p_attempt_id IS NULL OR p_event_row_id IS NULL
       OR p_rut_fingerprint IS NULL
       OR p_rut_fingerprint !~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'
       OR p_session_id IS NULL OR length(btrim(p_session_id)) = 0
       OR p_processed_at IS NULL THEN
        RAISE EXCEPTION 'invalid verified identity decision input' USING ERRCODE = '22023';
    END IF;

    SELECT * INTO v_attempt
      FROM public.identity_provider_attempt
     WHERE id = p_attempt_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'provider attempt not found' USING ERRCODE = 'P0002';
    END IF;
    IF v_attempt.status NOT IN ('CREATED', 'PENDING_USER_INPUT', 'PROCESSING', 'IN_REVIEW')
       OR v_attempt.declaration_id IS NULL
       OR v_attempt.provider_session_id IS DISTINCT FROM btrim(p_session_id) THEN
        RAISE EXCEPTION 'provider attempt is not eligible for verified commit' USING ERRCODE = '23505';
    END IF;
    v_declaration_id := v_attempt.declaration_id;
    v_account_id := v_attempt.account_id;

    -- PrepareAttempt uses this account namespace. Acquire it before row locks,
    -- then lock every attempt/event of the snapshot in deterministic order.
    -- Otherwise a prior TTL retry can retain the FK and prevent snapshot purge,
    -- or competing approvals can each hold one attempt while waiting for another.
    PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended(v_account_id::text, 1));
    PERFORM a.id FROM public.identity_provider_attempt a
     WHERE a.declaration_id=v_declaration_id AND a.account_id=v_account_id
     ORDER BY a.id FOR UPDATE OF a;
    SELECT * INTO v_attempt FROM public.identity_provider_attempt
     WHERE id=p_attempt_id FOR UPDATE;
    IF NOT FOUND OR v_attempt.account_id IS DISTINCT FROM v_account_id
       OR v_attempt.declaration_id IS DISTINCT FROM v_declaration_id
       OR v_attempt.status NOT IN ('CREATED', 'PENDING_USER_INPUT', 'PROCESSING', 'IN_REVIEW')
       OR v_attempt.provider_session_id IS DISTINCT FROM btrim(p_session_id) THEN
        RAISE EXCEPTION 'provider attempt changed before verified commit' USING ERRCODE='23505';
    END IF;
    PERFORM e.id FROM public.identity_provider_event e
     JOIN public.identity_provider_attempt a ON a.id=e.attempt_id
     WHERE a.declaration_id=v_declaration_id AND a.account_id=v_account_id
     ORDER BY e.id FOR UPDATE OF e;

    SELECT * INTO v_event
      FROM public.identity_provider_event
     WHERE id = p_event_row_id AND attempt_id = p_attempt_id
     FOR UPDATE;
    IF NOT FOUND OR v_event.processing_status NOT IN ('RECEIVED', 'FAILED') THEN
        RAISE EXCEPTION 'webhook event is not eligible for verified commit' USING ERRCODE = '23505';
    END IF;

    SELECT * INTO v_claim
      FROM public.identity_rut_claim
     WHERE fingerprint = p_rut_fingerprint
       AND account_id = v_attempt.account_id
       AND declaration_id = v_attempt.declaration_id
     FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'verified identity claim conflict' USING ERRCODE = '23505';
    END IF;
    v_processed_at := GREATEST(p_processed_at, v_attempt.created_at,
                               v_event.received_at, v_claim.created_at);
    IF NOT ((v_claim.verified_at IS NULL AND v_claim.expires_at > v_processed_at)
                        OR (v_claim.verified_at IS NOT NULL AND v_claim.expires_at IS NULL)) THEN
        RAISE EXCEPTION 'verified identity claim conflict' USING ERRCODE = '23505';
    END IF;

    -- Queue all known sessions before removing any obsolete retry metadata.
    -- Remote calls remain outside this transaction. Every existing key must be
    -- bound to its exact attempt, including sessions from earlier failed retries.
    FOR v_related IN SELECT a.id,a.provider_session_id
      FROM public.identity_provider_attempt a
     WHERE a.declaration_id=v_declaration_id AND a.account_id=v_account_id
     ORDER BY a.id
    LOOP
        IF v_related.provider_session_id IS NULL OR btrim(v_related.provider_session_id)='' THEN
            CONTINUE;
        END IF;
        v_dedup_key := 'didit_session_deletion:' || btrim(v_related.provider_session_id);
        IF length(v_dedup_key)>120 THEN
            RAISE EXCEPTION 'provider session identifier exceeds outbox limit' USING ERRCODE='22023';
        END IF;
        INSERT INTO public.outbox_evento_local (
            id,agregado_tipo,agregado_id,tipo,clave_deduplicacion,version,creada_en,disponible_en
        ) VALUES (
            gen_random_uuid(),'didit_session',v_related.id,'didit.session_remote_deletion',
            v_dedup_key,1,v_processed_at,v_processed_at
        ) ON CONFLICT (clave_deduplicacion) DO NOTHING;
        IF NOT EXISTS (SELECT 1 FROM public.outbox_evento_local
            WHERE clave_deduplicacion=v_dedup_key AND agregado_tipo='didit_session'
              AND agregado_id=v_related.id AND tipo='didit.session_remote_deletion') THEN
            RAISE EXCEPTION 'session deletion outbox key is bound to another record' USING ERRCODE='23505';
        END IF;
    END LOOP;

    UPDATE public.identity_rut_claim
       SET declaration_id = NULL,
           verified_at = COALESCE(v_claim.verified_at, v_processed_at),
           expires_at = NULL,
           updated_at = v_processed_at
     WHERE fingerprint = p_rut_fingerprint
       AND account_id = v_attempt.account_id
       AND declaration_id = v_attempt.declaration_id
       AND ((verified_at IS NULL AND expires_at > v_processed_at)
            OR (verified_at IS NOT NULL AND expires_at IS NULL));
    GET DIAGNOSTICS v_rows = ROW_COUNT;
    IF v_rows <> 1 THEN
        RAISE EXCEPTION 'verified identity claim expired or changed' USING ERRCODE = '23505';
    END IF;

    UPDATE public.identity_provider_attempt
       SET status = 'APPROVED',
           declaration_id = NULL,
           decision_key_version = NULL,
           decision_algorithm = NULL,
           encrypted_decision = NULL,
           updated_at = v_processed_at
     WHERE id = p_attempt_id
       AND status IN ('CREATED', 'PENDING_USER_INPUT', 'PROCESSING', 'IN_REVIEW')
       AND declaration_id = v_attempt.declaration_id
       AND provider_session_id = btrim(p_session_id)
     RETURNING * INTO v_attempt;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'provider attempt changed during verified commit' USING ERRCODE = '23505';
    END IF;

    UPDATE public.identity_provider_event
       SET processing_status = 'PROCESSED',
           processed_at = v_processed_at
     WHERE id = p_event_row_id
       AND attempt_id = p_attempt_id
       AND processing_status IN ('RECEIVED', 'FAILED');
    GET DIAGNOSTICS v_rows = ROW_COUNT;
    IF v_rows <> 1 THEN
        RAISE EXCEPTION 'webhook event changed during verified commit' USING ERRCODE = '23505';
    END IF;

    -- Only the new approved attempt remains bound to the current result. Known
    -- obsolete sessions have durable deletion intents; remove their dependent
    -- events explicitly. For an uncertain creation without a known session,
    -- retain minimal correlation metadata, expire it and remove all ciphertext.
    -- This is not confirmation that a remote unknown session was deleted.
    DELETE FROM public.identity_provider_event e USING public.identity_provider_attempt a
     WHERE e.attempt_id=a.id AND a.declaration_id=v_declaration_id
       AND a.account_id=v_account_id AND a.provider_session_id IS NOT NULL
       AND btrim(a.provider_session_id)<>'';
    DELETE FROM public.identity_provider_attempt a
     WHERE a.declaration_id=v_declaration_id AND a.account_id=v_account_id
       AND a.provider_session_id IS NOT NULL AND btrim(a.provider_session_id)<>'';
    UPDATE public.identity_provider_attempt a SET declaration_id=NULL,status='EXPIRED',
        decision_key_version=NULL,decision_algorithm=NULL,encrypted_decision=NULL,
        updated_at=GREATEST(a.updated_at,v_processed_at)
     WHERE a.declaration_id=v_declaration_id AND a.account_id=v_account_id
       AND (a.provider_session_id IS NULL OR btrim(a.provider_session_id)='');

    DELETE FROM public.identity_declaration_revision
     WHERE id = v_declaration_id
       AND account_id = v_account_id;
    GET DIAGNOSTICS v_rows = ROW_COUNT;
    IF v_rows <> 1 THEN
        RAISE EXCEPTION 'identity declaration snapshot could not be purged' USING ERRCODE = '23503';
    END IF;

    RETURN v_attempt;
END;
$_$;


--
-- Name: complete_expired_local_verification_file(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.complete_expired_local_verification_file(p_evidencia uuid, p_at timestamp with time zone) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE file_id uuid;
BEGIN
  SELECT archivo_id INTO file_id FROM public.verificacion_archivo_retiro_pendiente_local
   WHERE evidencia_id=p_evidencia FOR UPDATE;
  IF file_id IS NULL THEN RETURN false; END IF;
  UPDATE public.verificacion_evidencia_sintetica
     SET archivo_id=NULL,archivo_retirado_en=p_at
   WHERE id=p_evidencia AND archivo_id=file_id;
  DELETE FROM public.verificacion_archivo_retiro_pendiente_local WHERE evidencia_id=p_evidencia;
  RETURN true;
END;
$$;


--
-- Name: complete_private_media_cleanup(uuid, bigint, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.complete_private_media_cleanup(p_id uuid, p_token bigint, p_at timestamp with time zone) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    SET "TimeZone" TO 'UTC'
    AS $$
DECLARE
    v_now timestamptz := pg_catalog.clock_timestamp();
BEGIN
    IF pg_catalog.current_setting('role', true) IS DISTINCT FROM 'espacigo_privacy_cleanup_worker' THEN
        RAISE EXCEPTION 'privacy cleanup worker role is required' USING ERRCODE = '42501';
    END IF;
    IF p_at IS NULL OR p_at > v_now + interval '1 minute' THEN
        RAISE EXCEPTION 'cleanup completion time exceeds allowed clock skew' USING ERRCODE = '22023';
    END IF;
    PERFORM 1 FROM public.media_asset
     WHERE id=p_id AND estado='deleting' AND fencing_token=p_token FOR UPDATE;
    IF NOT FOUND THEN RETURN false; END IF;
    DELETE FROM public.media_variant WHERE media_id=p_id;
    DELETE FROM public.media_variant_intent WHERE media_id=p_id;
    UPDATE public.media_asset SET estado='deleted',deleted_at=v_now,lease_until=NULL,updated_at=v_now
     WHERE id=p_id AND estado='deleting' AND fencing_token=p_token;
    RETURN FOUND;
END
$$;


--
-- Name: document_verification_event_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_event_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RAISE EXCEPTION 'document verification events are append-only' USING ERRCODE='23514'; END; $$;


--
-- Name: document_verification_identity_case_insert_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_identity_case_insert_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    IF NEW.purpose='identity'
       AND (NEW.identity_declaration_revision_id IS NULL
            OR NEW.document_format_declared IS NULL
            OR NEW.capture_consent_version IS NULL
            OR NEW.capture_consented_at IS NULL) THEN
        RAISE EXCEPTION 'new identity case requires saved declaration and capture consent'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: document_verification_identity_case_link_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_identity_case_link_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    IF OLD.purpose='identity' AND NEW.purpose='identity'
       AND NEW.identity_declaration_revision_id IS DISTINCT FROM OLD.identity_declaration_revision_id
       AND (NEW.identity_declaration_revision_id IS NOT NULL
            OR NEW.state IS NULL
            OR NEW.state NOT IN ('rechazada','vencida','revocada')) THEN
        RAISE EXCEPTION 'identity declaration link can only be cleared on a terminal case transition'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: document_verification_identity_review_audit_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_identity_review_audit_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
DECLARE
    v_state text;
    v_revision bigint;
    v_reviewer uuid;
    v_format text;
    v_expiry timestamptz;
BEGIN
    IF NEW.purpose='identity' AND NEW.action='review_decision' THEN
        IF NEW.identity_review_observations IS NULL THEN
            RAISE EXCEPTION 'identity review decision requires observed metadata and checklist'
                USING ERRCODE='23514';
        END IF;
        SELECT c.state,c.revision,c.reviewer_id,c.document_format_observed,c.document_expires_at
          INTO v_state,v_revision,v_reviewer,v_format,v_expiry
          FROM public.document_verification_case c
         WHERE c.id=NEW.case_id AND c.purpose=NEW.purpose AND c.subject_id=NEW.subject_id;
        IF NOT FOUND
           OR v_state IS DISTINCT FROM NEW.to_state
           OR v_state IS DISTINCT FROM NEW.result_code
           OR v_revision IS DISTINCT FROM NEW.revision
           OR v_reviewer IS DISTINCT FROM NEW.actor_id
           OR v_format IS DISTINCT FROM NEW.identity_review_observations->>'document_format_observed'
           OR v_expiry IS DISTINCT FROM NULLIF(NEW.identity_review_observations->>'document_expires_at_observed','')::timestamptz THEN
            RAISE EXCEPTION 'identity review event does not match the case transition'
                USING ERRCODE='23514';
        END IF;
    ELSIF NEW.identity_review_observations IS NOT NULL THEN
        RAISE EXCEPTION 'identity review observations are only valid on identity review decisions'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: document_verification_reviewer_grant_audit(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_reviewer_grant_audit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  INSERT INTO public.document_verification_reviewer_grant_event(grant_id,reviewer_id,capability,scope_kind,scope_id,actor_id,action,reason_code)
  VALUES(NEW.id,NEW.reviewer_id,NEW.capability,NEW.scope_kind,NEW.scope_id,NEW.granted_by,'granted',NEW.granted_reason_code);
 ELSE
  INSERT INTO public.document_verification_reviewer_grant_event(grant_id,reviewer_id,capability,scope_kind,scope_id,actor_id,action,reason_code)
  VALUES(NEW.id,NEW.reviewer_id,NEW.capability,NEW.scope_kind,NEW.scope_id,NEW.revoked_by,'revoked',NEW.revocation_reason_code);
 END IF;
 RETURN NEW;
END; $$;


--
-- Name: document_verification_reviewer_grant_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_reviewer_grant_guard() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'reviewer grants must be revoked, not deleted' USING ERRCODE='23514';
 END IF;
 PERFORM 1 FROM public.usuario WHERE id=NEW.reviewer_id AND estado='activo' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'reviewer must be active' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM public.usuario WHERE id=NEW.granted_by AND estado='activo';
  IF NOT FOUND THEN RAISE EXCEPTION 'grantor must be active' USING ERRCODE='23514'; END IF;
  IF NEW.scope_kind='identity_subject' THEN
   PERFORM 1 FROM public.usuario WHERE id=NEW.scope_id AND estado='activo';
  ELSE
   PERFORM 1 FROM public.espacio WHERE id=NEW.scope_id;
  END IF;
  IF NOT FOUND THEN RAISE EXCEPTION 'review grant scope does not exist' USING ERRCODE='23514'; END IF;
 ELSIF ROW(NEW.id,NEW.reviewer_id,NEW.capability,NEW.scope_kind,NEW.scope_id,NEW.granted_by,NEW.granted_at,NEW.granted_reason_code,NEW.expires_at)
    IS DISTINCT FROM ROW(OLD.id,OLD.reviewer_id,OLD.capability,OLD.scope_kind,OLD.scope_id,OLD.granted_by,OLD.granted_at,OLD.granted_reason_code,OLD.expires_at)
    OR OLD.revoked_at IS NOT NULL OR NEW.revoked_at IS NULL THEN
  RAISE EXCEPTION 'reviewer grant may only transition once to revoked' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;


--
-- Name: document_verification_space_owner_changed(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.document_verification_space_owner_changed() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
    changed_at timestamptz := clock_timestamp();
    replacement_deadline timestamptz;
    file_ids uuid[];
    retention_deadlines timestamptz[];
BEGIN
    IF NEW.propietario_id IS NOT DISTINCT FROM OLD.propietario_id THEN
        RETURN NEW;
    END IF;
    replacement_deadline := changed_at + interval '30 days';

    WITH superseded AS (
        UPDATE public.document_verification_case
        SET superseded_at=changed_at, revision=revision+1, updated_at=changed_at
        WHERE purpose='space'
          AND space_id=NEW.id
          AND subject_id<>NEW.propietario_id
          AND superseded_at IS NULL
        RETURNING id,purpose,subject_id,state,revision
    ), audit_events AS (
        INSERT INTO public.document_verification_event(
            case_id,purpose,subject_id,actor_kind,action,from_state,to_state,
            result_code,reason_code,revision
        )
        SELECT id,purpose,subject_id,'system','case_superseded',state,state,
               state,'cambio_titularidad',revision
        FROM superseded
        RETURNING case_id
    ), file_policy AS (
        -- Keep the updated table out of UPDATE ... FROM's lateral scope: derive
        -- deadlines in a separate read joined to the rows superseded above.
        SELECT f.id AS file_id,
               CASE
                   WHEN f.inspected_at IS NULL OR f.inspected_at > changed_at THEN changed_at
                   ELSE f.inspected_at + interval '24 hours'
               END AS malware_deadline
        FROM public.document_verification_file f
        JOIN superseded s ON s.id=f.case_id
    ), updated_files AS (
        UPDATE public.document_verification_file f
        SET retention_at = CASE
                WHEN f.last_error_code='malware_detected'
                  OR f.retention_reason_code='malicious_file'
                    THEN LEAST(COALESCE(f.retention_at,policy.malware_deadline),policy.malware_deadline)
                WHEN f.retention_at IS NULL OR f.retention_at > replacement_deadline
                    THEN replacement_deadline
                ELSE f.retention_at
            END,
            retention_reason_code = CASE
                WHEN f.last_error_code='malware_detected'
                  OR f.retention_reason_code='malicious_file'
                    THEN CASE
                        WHEN f.retention_at IS NOT NULL
                          AND f.retention_at < policy.malware_deadline
                            THEN f.retention_reason_code
                        ELSE 'malicious_file'
                    END
                WHEN f.retention_at IS NOT NULL AND f.retention_at <= replacement_deadline
                    THEN f.retention_reason_code
                ELSE 'rejected_or_replaced'
            END,
            updated_at=changed_at
        FROM file_policy policy
        WHERE f.id=policy.file_id
        RETURNING f.id,f.case_id,f.retention_at
    ), cancelled_scan_jobs AS (
        UPDATE public.document_verification_job j
        SET state='completado',lease_until=NULL,updated_at=changed_at
        WHERE j.kind='scan'
          AND j.state IN ('pendiente','procesando','reintentar')
          AND j.file_id IN (SELECT id FROM updated_files)
        RETURNING j.file_id
    )
    SELECT array_agg(uf.id ORDER BY uf.id) FILTER (WHERE uf.id IS NOT NULL),
           array_agg(uf.retention_at ORDER BY uf.id) FILTER (WHERE uf.id IS NOT NULL)
    INTO file_ids,retention_deadlines
    FROM superseded s
    LEFT JOIN updated_files uf ON uf.case_id=s.id;

    -- Require the atomic deletion audit event: scan cancellation also marks a
    -- retention job completed, but does not mean the object was deleted.
    INSERT INTO public.document_verification_job(file_id,kind)
    SELECT f.file_id,'retention_delete'
    FROM unnest(file_ids) AS f(file_id)
    WHERE NOT EXISTS (
        SELECT 1
        FROM public.document_verification_job AS done
        WHERE done.file_id=f.file_id
          AND done.kind='retention_delete'
          AND done.state='completado'
          AND EXISTS (
              SELECT 1
              FROM public.document_verification_event e
              WHERE e.file_id=done.file_id
                AND e.action='retention_deleted'
          )
    )
    ON CONFLICT DO NOTHING;

    UPDATE public.document_verification_job j
    SET state='pendiente',attempts=0,available_at=f.retention_at,
        lease_until=NULL,last_error_code=NULL,updated_at=changed_at
    FROM unnest(file_ids,retention_deadlines) AS f(file_id,retention_at)
    WHERE j.file_id=f.file_id
      AND j.kind='retention_delete'
      AND j.state IN ('pendiente','procesando','reintentar');

    -- If a successful deletion and an active duplicate coexist, preserve the
    -- deletion result and retire that duplicate.
    UPDATE public.document_verification_job j
    SET state='completado',lease_until=NULL,last_error_code=NULL,updated_at=changed_at
    WHERE j.file_id = ANY(file_ids)
      AND j.kind='retention_delete'
      AND j.state IN ('pendiente','procesando','reintentar')
      AND EXISTS (
          SELECT 1
          FROM public.document_verification_job AS done
          WHERE done.file_id=j.file_id
            AND done.kind='retention_delete'
            AND done.state='completado'
            AND EXISTS (
                SELECT 1
                FROM public.document_verification_event e
                WHERE e.file_id=done.file_id
                  AND e.action='retention_deleted'
            )
      );

    RETURN NEW;
END;
$$;


--
-- Name: FUNCTION document_verification_space_owner_changed(); Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON FUNCTION public.document_verification_space_owner_changed() IS 'Closes prior-owner space cases atomically; preserves earlier mandatory file-retention deadlines, caps malware retention at 24 hours after inspection (immediate if its timestamp is absent or future), and schedules other replacement retention no later than 30 days after transfer.';


--
-- Name: enqueue_booking_payment_notices(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_booking_payment_notices() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF OLD.estado IS DISTINCT FROM NEW.estado AND NEW.estado='pagada' THEN
    PERFORM public.enqueue_local_notice('pago_arriendo_confirmado','reserva',NEW.id,NEW.arrendatario_id,
      'rental-payment:'||NEW.id::text||':confirmed',NEW.actualizada_en);
    PERFORM public.enqueue_local_notice('nueva_reserva_pagada','reserva',NEW.id,NEW.anfitrion_id,
      'paid-booking:'||NEW.id::text||':host',NEW.actualizada_en);
  ELSIF OLD.estado IS DISTINCT FROM NEW.estado AND NEW.estado='cancelada_por_pago' THEN
    -- Guarantee rejection/expiry also cancels a previously paid rental. Only
    -- emit RQF-201 when the rental payment operation itself was rejected.
    IF EXISTS (SELECT 1 FROM public.reserva_pago_ensayo p
      WHERE p.reserva_id=NEW.id AND p.resultado='rechazo_simulado') THEN
      PERFORM public.enqueue_local_notice('pago_arriendo_rechazado','reserva',NEW.id,NEW.arrendatario_id,
        'rental-payment:'||NEW.id::text||':rejected',NEW.actualizada_en);
    END IF;
  END IF;
  RETURN NEW;
END $$;


--
-- Name: enqueue_checkout_payment_notices(uuid, text, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_checkout_payment_notices(p_operation_id uuid, p_transition text, p_at timestamp with time zone) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE reservation public.reserva_arriendo_comercial%ROWTYPE;
BEGIN
    IF p_transition NOT IN ('approved','rejected','expired') THEN
        RAISE EXCEPTION 'unsupported commercial payment notice transition' USING ERRCODE='22023';
    END IF;
    SELECT * INTO reservation FROM public.reserva_arriendo_comercial
      WHERE operacion_checkout_id=p_operation_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'commercial reservation missing for payment transition' USING ERRCODE='23503';
    END IF;
    IF (p_transition='approved' AND reservation.estado <> 'pagada') OR
       (p_transition='rejected' AND reservation.estado <> 'cancelada_por_pago') OR
       (p_transition='expired' AND reservation.estado <> 'vencida_pago') THEN
        RAISE EXCEPTION 'reservation state does not match payment notice transition' USING ERRCODE='23514';
    END IF;

    IF p_transition='approved' THEN
        INSERT INTO public.checkout_payment_notice_outbox(
            id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
        VALUES
            (gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,
             'checkout-payment:'||p_operation_id::text||':approved:'||reservation.arrendatario_id::text,p_at,p_at),
            (gen_random_uuid(),reservation.id,p_operation_id,reservation.anfitrion_id,'anfitrion',p_transition,
             'checkout-payment:'||p_operation_id::text||':approved:'||reservation.anfitrion_id::text,p_at,p_at)
        ON CONFLICT (dedupe_key) DO NOTHING;
    ELSE
        INSERT INTO public.checkout_payment_notice_outbox(
            id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
        VALUES (gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,
                'checkout-payment:'||p_operation_id::text||':'||p_transition||':'||reservation.arrendatario_id::text,p_at,p_at)
        ON CONFLICT (dedupe_key) DO NOTHING;
    END IF;
END $$;


--
-- Name: enqueue_contract_complete_notices(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_contract_complete_notices() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE reservation_row public.reserva_ensayo_local%ROWTYPE;
BEGIN
  IF OLD.estado IS DISTINCT FROM NEW.estado AND NEW.estado='firmado' THEN
    SELECT * INTO reservation_row FROM public.reserva_ensayo_local WHERE id=NEW.reserva_id;
    PERFORM public.enqueue_local_notice('contrato_firmado','contrato',NEW.id,reservation_row.anfitrion_id,
      'contract-signed:'||NEW.id::text||':'||reservation_row.anfitrion_id::text,NEW.firmado_en);
    PERFORM public.enqueue_local_notice('contrato_firmado','contrato',NEW.id,reservation_row.arrendatario_id,
      'contract-signed:'||NEW.id::text||':'||reservation_row.arrendatario_id::text,NEW.firmado_en);
  END IF;
  RETURN NEW;
END $$;


--
-- Name: enqueue_contract_signature_notices(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_contract_signature_notices() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE contract_row public.contrato_ensayo_local%ROWTYPE;
BEGIN
  SELECT * INTO contract_row FROM public.contrato_ensayo_local WHERE id=NEW.contrato_id;
  IF NEW.estado='pendiente' THEN
    PERFORM public.enqueue_local_notice('invitacion_firma','contrato',NEW.contrato_id,NEW.firmante_id,
      'contract-invite:'||NEW.contrato_id::text||':'||NEW.firmante_id::text,NEW.actualizada_en);
  END IF;
  RETURN NEW;
END $$;


--
-- Name: FUNCTION enqueue_contract_signature_notices(); Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON FUNCTION public.enqueue_contract_signature_notices() IS 'Authenticated-link invitation event; no bearer secret is placed in the URL.';


--
-- Name: enqueue_local_cancel_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_cancel_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF OLD.estado NOT IN ('cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma') AND NEW.estado IN ('cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma') THEN
    PERFORM public.enqueue_local_notice('reserva_cancelada','reserva',NEW.id,NEW.anfitrion_id,'cancel:'||NEW.id::text||':'||NEW.estado||':host',NEW.actualizada_en);
    PERFORM public.enqueue_local_notice('reserva_cancelada','reserva',NEW.id,NEW.arrendatario_id,'cancel:'||NEW.id::text||':'||NEW.estado||':renter',NEW.actualizada_en);
  END IF; RETURN NEW;
END $$;


--
-- Name: enqueue_local_checkin_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_checkin_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE host uuid; BEGIN
  IF NEW.tipo='checkin' THEN
    SELECT anfitrion_id INTO host FROM public.reserva_ensayo_local WHERE id=NEW.reserva_id;
    PERFORM public.enqueue_local_notice('checkin_registrado','reserva',NEW.reserva_id,host,'checkin:'||NEW.id::text,NEW.ocurrio_en);
  END IF; RETURN NEW;
END $$;


--
-- Name: enqueue_local_claim_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_claim_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM public.enqueue_local_notice('reclamo_abierto','reserva',NEW.reserva_id,NEW.arrendatario_id,'claim:'||NEW.id::text,NEW.abierto_en);
  RETURN NEW;
END $$;


--
-- Name: enqueue_local_damage_claim_resolution_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_damage_claim_resolution_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE claim public.reclamo_dano_ensayo_local%ROWTYPE;
BEGIN
  SELECT * INTO claim FROM public.reclamo_dano_ensayo_local WHERE id=NEW.reclamo_id;
  PERFORM public.enqueue_local_notice('reclamo_resuelto','reserva',claim.reserva_id,claim.anfitrion_id,
    'claim-resolved:'||claim.id::text||':'||claim.anfitrion_id::text,NEW.resuelta_en);
  PERFORM public.enqueue_local_notice('reclamo_resuelto','reserva',claim.reserva_id,claim.arrendatario_id,
    'claim-resolved:'||claim.id::text||':'||claim.arrendatario_id::text,NEW.resuelta_en);
  RETURN NEW;
END $$;


--
-- Name: enqueue_local_notice(text, text, uuid, uuid, text, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_notice(p_type text, p_aggregate_type text, p_aggregate uuid, p_recipient uuid, p_key text, p_at timestamp with time zone) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE notice_id uuid := gen_random_uuid();
BEGIN
    IF p_recipient IS NULL THEN RETURN; END IF;
    INSERT INTO public.aviso_local(id,tipo_evento,agregado_tipo,agregado_id,destinatario_id,clave_deduplicacion,creada_en,proximo_intento_en)
    VALUES(notice_id,p_type,p_aggregate_type,p_aggregate,p_recipient,p_key,p_at,p_at)
    ON CONFLICT(clave_deduplicacion) DO NOTHING;
    IF EXISTS (SELECT 1 FROM public.aviso_local WHERE clave_deduplicacion=p_key) THEN
      INSERT INTO public.aviso_local_ciclo(aviso_id,ciclo,estado,iniciada_en)
      SELECT id,ciclo,'pendiente',creada_en FROM public.aviso_local WHERE clave_deduplicacion=p_key
      ON CONFLICT DO NOTHING;
    END IF;
END $$;


--
-- Name: enqueue_local_review_report_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_local_review_report_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE admin record; BEGIN
  FOR admin IN SELECT u.id AS usuario_id FROM public.usuario u
    WHERE u.estado='activo' AND (
      EXISTS (SELECT 1 FROM public.rol_usuario ru WHERE ru.usuario_id=u.id AND ru.rol='administrador')
      OR EXISTS (SELECT 1 FROM public.administrador_global ag WHERE ag.usuario_id=u.id)
    ) LOOP
    PERFORM public.enqueue_local_notice('resena_reportada','reporte_resena',NEW.id,admin.usuario_id,'review-report:'||NEW.id::text||':'||admin.usuario_id::text,NEW.creada_en);
  END LOOP; RETURN NEW;
END $$;


--
-- Name: enqueue_login_lock_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_login_lock_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.bloqueado_hasta IS NOT NULL AND NEW.bloqueado_hasta IS DISTINCT FROM OLD.bloqueado_hasta THEN
    PERFORM public.enqueue_local_notice(
      'cuenta_bloqueada_login','usuario',NEW.id,NEW.id,
      'login-lock:'||NEW.id::text||':'||extract(epoch FROM NEW.bloqueado_hasta)::text,
      COALESCE(NEW.actualizado_en,NEW.bloqueado_hasta));
  END IF;
  RETURN NEW;
END $$;


--
-- Name: FUNCTION enqueue_login_lock_notice(); Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON FUNCTION public.enqueue_login_lock_notice() IS 'Durable synthetic login lockout notice. Never stores a password, token or email.';


--
-- Name: enqueue_verification_result_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enqueue_verification_result_notice() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF OLD.estado='en_revision' AND NEW.estado IN ('aprobada','rechazada') THEN
    PERFORM public.enqueue_local_notice('verificacion_'||NEW.estado,'verificacion',NEW.id,NEW.usuario_id,
      'verification-result:'||NEW.id::text||':'||NEW.estado,NEW.resuelta_en);
  END IF;
  RETURN NEW;
END $$;


--
-- Name: fail_expired_local_verification_file(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fail_expired_local_verification_file(p_evidencia uuid, p_at timestamp with time zone) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
  UPDATE public.verificacion_archivo_retiro_pendiente_local
     SET intentos=intentos+0,lease_hasta=NULL,
         disponible_en=p_at+LEAST(interval '1 hour',interval '5 seconds' * (1 << LEAST(intentos,9))),
         ultimo_codigo_error='archivo_sintetico_no_retirado'
   WHERE evidencia_id=p_evidencia;
  RETURN FOUND;
END;
$$;


--
-- Name: fail_private_media_cleanup(uuid, bigint, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fail_private_media_cleanup(p_id uuid, p_token bigint, p_at timestamp with time zone) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    SET "TimeZone" TO 'UTC'
    AS $$
DECLARE
    v_now timestamptz := pg_catalog.clock_timestamp();
BEGIN
    IF pg_catalog.current_setting('role', true) IS DISTINCT FROM 'espacigo_privacy_cleanup_worker' THEN
        RAISE EXCEPTION 'privacy cleanup worker role is required' USING ERRCODE = '42501';
    END IF;
    IF p_at IS NULL OR p_at > v_now + interval '1 minute' THEN
        RAISE EXCEPTION 'cleanup retry time exceeds allowed clock skew' USING ERRCODE = '22023';
    END IF;
    UPDATE public.media_asset SET lease_until=v_now+interval '1 minute',updated_at=v_now
     WHERE id=p_id AND estado='deleting' AND fencing_token=p_token;
    RETURN FOUND;
END
$$;


--
-- Name: finalize_pending_suppression_cleanup(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.finalize_pending_suppression_cleanup(p_request_id uuid, p_at timestamp with time zone) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    SET "TimeZone" TO 'UTC'
    AS $$
DECLARE
    v_execution_id uuid;
    v_subject_id uuid;
    v_request_state text;
    v_execution_state text;
    v_completed_at timestamptz;
    v_requested_at timestamptz;
    v_pending integer;
    v_retire_at timestamptz;
BEGIN
    IF pg_catalog.current_setting('role', true) IS DISTINCT FROM 'espacigo_privacy_cleanup_worker' THEN
        RAISE EXCEPTION 'privacy cleanup worker role is required' USING ERRCODE = '42501';
    END IF;

    SELECT x.id, x.usuario_id, x.estado, x.completada_en, s.estado, s.solicitada_en
      INTO v_execution_id, v_subject_id, v_execution_state, v_completed_at, v_request_state, v_requested_at
      FROM public.ejecucion_baja_local x
      JOIN public.solicitud_titular s ON s.id = x.solicitud_id
     WHERE x.solicitud_id = p_request_id
       AND s.tipo = 'supresion'
       AND x.estado IN ('limpieza_pendiente', 'completada')
     ORDER BY x.iniciada_en DESC
     LIMIT 1
     FOR UPDATE OF x, s;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    IF v_request_state NOT IN ('en_revision', 'resuelta') THEN
        RETURN false;
    END IF;

    SELECT count(*) INTO v_pending
      FROM public.baja_archivo_pendiente_local
     WHERE ejecucion_id = v_execution_id AND completada_en IS NULL;
    SELECT v_pending + count(*) INTO v_pending
      FROM public.media_asset
     WHERE propietario_id=v_subject_id AND estado NOT IN ('deleted','failed');
    IF v_pending > 0 THEN
        RETURN false;
    END IF;

    IF p_at IS NULL THEN
        RAISE EXCEPTION 'cleanup completion time is required' USING ERRCODE = '22004';
    END IF;
    v_retire_at := p_at + interval '60 months';

    IF v_execution_state = 'limpieza_pendiente' THEN
        UPDATE public.solicitud_titular
           SET estado = 'resuelta', resuelta_en = p_at,
               motivo_resolucion_codigo = 'baja_local_minimizada', retirar_en = v_retire_at
         WHERE id = p_request_id AND estado = 'en_revision';
        IF NOT FOUND AND v_request_state <> 'resuelta' THEN
            RETURN false;
        END IF;
        UPDATE public.ejecucion_baja_local
           SET estado = 'completada', completada_en = p_at, retirar_en = v_retire_at
         WHERE id = v_execution_id AND estado = 'limpieza_pendiente';
        v_completed_at := p_at;
    END IF;

    IF v_completed_at IS NULL THEN
        RETURN false;
    END IF;

    INSERT INTO public.registro_replay_baja_local
        (ejecucion_origen_id, solicitud_id, usuario_id, solicitada_en, completada_en)
    SELECT v_execution_id, p_request_id, v_subject_id, v_requested_at, v_completed_at
    ON CONFLICT (ejecucion_origen_id) DO NOTHING;
    IF NOT FOUND AND NOT EXISTS (
        SELECT 1
          FROM public.registro_replay_baja_local r
         WHERE r.ejecucion_origen_id = v_execution_id
           AND r.solicitud_id = p_request_id
           AND r.usuario_id = v_subject_id
           AND r.solicitada_en = v_requested_at
           AND r.completada_en = v_completed_at
    ) THEN
        RAISE EXCEPTION 'privacy replay registry conflicts with completed cleanup' USING ERRCODE = '23514';
    END IF;
    RETURN true;
END
$$;


--
-- Name: gcp06_test_access_operation_exists(uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gcp06_test_access_operation_exists(p_id uuid) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    SELECT EXISTS (SELECT 1 FROM public.gcp06_test_access_batch_audit WHERE id=p_id)
$$;


--
-- Name: gcp06_test_access_owner_ready(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gcp06_test_access_owner_ready() RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    SELECT count(*)=1
    FROM public.usuario u
    JOIN public.administrador_global g ON g.usuario_id=u.id
    WHERE u.correo_normalizado='hernan.espinoza.dev@gmail.com'
      AND u.estado='activo'
      AND (u.bloqueado_hasta IS NULL OR u.bloqueado_hasta<=now())
      AND NOT EXISTS (SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id)
$$;


--
-- Name: gcp06_test_access_target_ids(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gcp06_test_access_target_ids() RETURNS SETOF uuid
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    SELECT u.id FROM public.usuario u
    WHERE u.estado <> 'desidentificado'
    ORDER BY u.id
$$;


--
-- Name: guard_checkout_commercial_contract_binding(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_checkout_commercial_contract_binding() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: guard_commercial_agreement(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_agreement() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: guard_commercial_contract_document(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_contract_document() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: guard_commercial_rental_authorization(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_rental_authorization() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: guard_commercial_rental_tax_document(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_rental_tax_document() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
 IF TG_OP='UPDATE' OR TG_OP='DELETE' THEN
  RAISE EXCEPTION 'commercial rental tax document is immutable' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS (
  SELECT 1 FROM public.reserva_arriendo_comercial r
  JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
  WHERE r.id=NEW.reserva_id AND r.anfitrion_id=NEW.anfitrion_id
   AND r.estado='pagada' AND c.estado='approved'
   AND c.provider_payment_status='approved' AND c.payment_verified_at IS NOT NULL
   AND c.provider_payment_id IS NOT NULL AND c.base_clp=r.subtotal_clp
   AND r.moneda='CLP'
 ) THEN
  RAISE EXCEPTION 'commercial tax document requires host and verified paid reservation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_commercial_signature(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_signature() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: guard_commercial_space_location(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_commercial_space_location() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
    IF ROW(NEW.direccion,NEW.ubicacion_catalogo_version,NEW.region_codigo,NEW.comuna_codigo,
           NEW.ubicacion_latitud,NEW.ubicacion_longitud,NEW.ubicacion_confirmada_en)
       IS DISTINCT FROM
       ROW(OLD.direccion,OLD.ubicacion_catalogo_version,OLD.region_codigo,OLD.comuna_codigo,
           OLD.ubicacion_latitud,OLD.ubicacion_longitud,OLD.ubicacion_confirmada_en) THEN
        NEW.ubicacion_revision := OLD.ubicacion_revision + 1;
        IF NEW.ubicacion_confirmada_en IS NOT DISTINCT FROM OLD.ubicacion_confirmada_en THEN
            NEW.ubicacion_confirmada_en := NULL;
        END IF;
    ELSE
        NEW.ubicacion_revision := OLD.ubicacion_revision;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: guard_local_reservation_close_after_settlement(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_local_reservation_close_after_settlement() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.estado IS DISTINCT FROM 'cerrada' AND NEW.estado='cerrada' THEN
        IF OLD.estado IS DISTINCT FROM 'finalizada' OR NOT EXISTS (
            SELECT 1
            FROM public.reserva_liquidacion_ensayo_local s
            JOIN public.reserva_liquidacion_operacion_ensayo_local o ON o.liquidacion_id=s.id
            JOIN public.reserva_liquidacion_resultado_fake_ensayo_local f ON f.operacion_id=o.id
            WHERE s.reserva_id=NEW.id
              AND s.estado='liquidada'
              AND s.ultimo_resultado='exito_simulado'
              AND f.resultado='exito_simulado'
        ) THEN
            RAISE EXCEPTION 'only a finalized reservation with a confirmed fake settlement can be closed'
                USING ERRCODE='23514', CONSTRAINT='reserva_cerrada_liquidacion_fake_ck';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: guardar_modalidad_horario_activo(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guardar_modalidad_horario_activo() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.modalidad_tarifa IS DISTINCT FROM OLD.modalidad_tarifa
       AND NEW.modalidad_tarifa <> 'hora'
       AND EXISTS (SELECT 1 FROM public.espacio_horario_semanal h
                   WHERE h.espacio_id=OLD.id AND h.activo) THEN
        RAISE EXCEPTION 'desactive el horario semanal antes de cambiar la modalidad tarifaria'
            USING ERRCODE='23514', CONSTRAINT='modalidad_horario_semanal_activo_ck';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: guardar_zona_sin_horario_activo(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guardar_zona_sin_horario_activo() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.zona_horaria IS DISTINCT FROM OLD.zona_horaria
       AND EXISTS (SELECT 1 FROM public.espacio_horario_semanal h
                   WHERE h.espacio_id=OLD.id AND h.activo) THEN
        RAISE EXCEPTION 'desactive el horario semanal antes de cambiar la zona horaria'
            USING ERRCODE='23514', CONSTRAINT='zona_horaria_horario_semanal_activo';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: host_compliance_event_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.host_compliance_event_immutable() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN RAISE EXCEPTION 'host compliance events are append-only' USING ERRCODE = '23514'; END;
$$;


--
-- Name: identity_declaration_case_retention_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.identity_declaration_case_retention_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.document_verification_case c
         WHERE c.identity_declaration_revision_id=OLD.id
           AND c.subject_id=OLD.account_id
    ) THEN
        RAISE lock_not_available;
    END IF;
    RETURN OLD;
END;
$$;


--
-- Name: img01b_check_media_relation_scope(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.img01b_check_media_relation_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE asset public.media_asset%ROWTYPE;
BEGIN
    SELECT * INTO asset FROM public.media_asset WHERE id=NEW.media_id;
    IF NOT FOUND OR asset.propietario_id IS DISTINCT FROM NEW.propietario_id THEN
        RAISE EXCEPTION 'media owner mismatch' USING ERRCODE='23503';
    END IF;

    IF TG_TABLE_NAME='profile_media' THEN
        IF asset.proposito<>'perfil' THEN
            RAISE EXCEPTION 'profile media purpose mismatch' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    ELSIF TG_TABLE_NAME='space_media' THEN
        IF asset.proposito<>'espacio' OR asset.espacio_id IS DISTINCT FROM NEW.espacio_id THEN
            RAISE EXCEPTION 'space media scope mismatch' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'unsupported media relation table: %', TG_TABLE_NAME USING ERRCODE='23514';
END
$$;


--
-- Name: img01b_enforce_gallery_slot(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.img01b_enforce_gallery_slot() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    target_space uuid;
    target_owner uuid;
    candidate_exclusion uuid;
    gallery_exclusion uuid;
    asset_exclusion uuid;
    old_asset_exclusion uuid;
    occupied bigint;
    claims_slot boolean;
    validate_target boolean := false;
BEGIN
    candidate_exclusion := NULL;
    gallery_exclusion := NULL;
    asset_exclusion := NULL;
    old_asset_exclusion := NULL;

    IF TG_TABLE_NAME='media_asset' THEN
        IF TG_OP='UPDATE' AND (NEW.id,NEW.propietario_id,NEW.espacio_id,NEW.proposito,
                               NEW.replacement_target_space_media_id)
             IS DISTINCT FROM (OLD.id,OLD.propietario_id,OLD.espacio_id,OLD.proposito,
                               OLD.replacement_target_space_media_id) THEN
            RAISE EXCEPTION 'media asset scope is immutable' USING ERRCODE='23514';
        END IF;
        IF TG_OP='INSERT' AND NEW.replacement_target_space_media_id IS NOT NULL
           AND (NEW.proposito IS DISTINCT FROM 'espacio' OR NEW.estado IS DISTINCT FROM 'pending') THEN
            RAISE EXCEPTION 'replacement must start as a pending space asset' USING ERRCODE='23514';
        END IF;
        IF TG_OP='UPDATE' AND OLD.estado='failed' AND NEW.estado='ready'
           AND NEW.replacement_target_space_media_id IS NOT NULL THEN
            RAISE EXCEPTION 'failed replacement must re-enter pending before ready' USING ERRCODE='23514';
        END IF;
        validate_target := NEW.replacement_target_space_media_id IS NOT NULL
            AND ((TG_OP='INSERT' AND NEW.estado='pending')
                 OR (TG_OP='UPDATE' AND NEW.estado='pending' AND OLD.estado<>'pending')
                 OR (TG_OP='UPDATE' AND OLD.estado='pending' AND NEW.estado='ready'));
        IF NEW.proposito<>'espacio' OR NEW.estado NOT IN ('pending','ready') THEN RETURN NEW; END IF;
        target_space := NEW.espacio_id;
        target_owner := NEW.propietario_id;
        asset_exclusion := NEW.id;
        claims_slot := NOT (TG_OP='UPDATE' AND OLD.estado IN ('pending','ready'));
    ELSIF TG_TABLE_NAME='espacio_galeria_archivo_candidato_local' THEN
        IF TG_OP='UPDATE' AND (NEW.archivo_id,NEW.propietario_id,NEW.espacio_id)
             IS DISTINCT FROM (OLD.archivo_id,OLD.propietario_id,OLD.espacio_id) THEN
            RAISE EXCEPTION 'synthetic candidate scope is immutable' USING ERRCODE='23514';
        END IF;
        IF NEW.estado<>'reservado' THEN RETURN NEW; END IF;
        IF TG_OP='UPDATE' AND OLD.estado='reservado' THEN RETURN NEW; END IF;
        target_space := NEW.espacio_id;
        target_owner := NEW.propietario_id;
        candidate_exclusion := NEW.archivo_id;
        claims_slot := true;
    ELSE
        IF TG_OP='UPDATE' AND (NEW.id,NEW.espacio_id,NEW.propietario_id)
             IS DISTINCT FROM (OLD.id,OLD.espacio_id,OLD.propietario_id) THEN
            RAISE EXCEPTION 'synthetic gallery scope is immutable' USING ERRCODE='23514';
        END IF;
        IF NEW.estado<>'activa' THEN RETURN NEW; END IF;
        IF TG_OP='UPDATE' AND OLD.estado='activa' THEN RETURN NEW; END IF;
        target_space := NEW.espacio_id;
        target_owner := NEW.propietario_id;
        candidate_exclusion := NEW.archivo_id;
        gallery_exclusion := NEW.id;
        claims_slot := true;
    END IF;

    IF NOT claims_slot AND NOT validate_target THEN RETURN NEW; END IF;
    PERFORM 1 FROM public.espacio WHERE id=target_space AND propietario_id=target_owner FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'space owner mismatch' USING ERRCODE='23503'; END IF;

    -- Validate and lock the active slot and its real ready asset only after the
    -- owning space lock, serializing replacement checks with slot retirement.
    IF validate_target THEN
        PERFORM 1
        FROM public.space_media sm
        JOIN public.media_asset prior ON prior.id=sm.media_id
        WHERE sm.id=NEW.replacement_target_space_media_id
          AND sm.espacio_id=target_space
          AND sm.propietario_id=target_owner
          AND sm.retired_at IS NULL
          AND prior.proposito='espacio'
          AND prior.espacio_id=target_space
          AND prior.propietario_id=target_owner
          AND prior.estado='ready'
        FOR UPDATE OF sm,prior;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'invalid replacement target' USING ERRCODE='23514';
        END IF;
    END IF;
    IF NOT claims_slot THEN RETURN NEW; END IF;

    -- Exclude only the ready asset behind this validated active slot while its
    -- pending replacement exists; all other pending/ready assets remain counted.
    IF TG_TABLE_NAME='media_asset' AND (to_jsonb(NEW)->>'replacement_target_space_media_id') IS NOT NULL THEN
        SELECT sm.media_id INTO old_asset_exclusion
        FROM public.space_media sm
        WHERE sm.id=NEW.replacement_target_space_media_id
          AND sm.espacio_id=target_space AND sm.propietario_id=target_owner
          AND sm.retired_at IS NULL;
    END IF;
    SELECT
      (SELECT count(*) FROM public.espacio_galeria_sintetica_local g
       WHERE g.espacio_id=target_space AND g.estado='activa' AND g.id IS DISTINCT FROM gallery_exclusion) +
      (SELECT count(*) FROM public.espacio_galeria_archivo_candidato_local c
       WHERE c.espacio_id=target_space AND c.estado='reservado' AND c.archivo_id IS DISTINCT FROM candidate_exclusion) +
      (SELECT count(*) FROM public.media_asset a
       WHERE a.espacio_id=target_space AND a.proposito='espacio' AND a.estado IN ('pending','ready')
         AND a.id IS DISTINCT FROM asset_exclusion
         AND a.id IS DISTINCT FROM old_asset_exclusion
         AND NOT EXISTS (
             SELECT 1 FROM public.media_asset replacement
             WHERE replacement.espacio_id=target_space
               AND replacement.propietario_id=target_owner
               AND replacement.estado='pending'
               AND replacement.replacement_target_space_media_id IS NOT NULL
               AND EXISTS (SELECT 1 FROM public.space_media old_slot
                           WHERE old_slot.id=replacement.replacement_target_space_media_id
                             AND old_slot.media_id=a.id
                             AND old_slot.retired_at IS NULL)
         ))
    INTO occupied;
    IF occupied >= 30 THEN RAISE EXCEPTION 'space media limit of 30 reached' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END
$$;


--
-- Name: img01b_guard_replacement_request_revision(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.img01b_guard_replacement_request_revision() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF (NEW.replacement_target_space_media_id IS NULL)
           <> (NEW.replacement_expected_revision IS NULL) THEN
            RAISE EXCEPTION 'replacement target and expected revision must be supplied together'
                USING ERRCODE='23514';
        END IF;
    ELSIF (NEW.replacement_target_space_media_id,NEW.replacement_expected_revision)
          IS DISTINCT FROM
          (OLD.replacement_target_space_media_id,OLD.replacement_expected_revision) THEN
        RAISE EXCEPTION 'replacement request identity is immutable'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;


--
-- Name: kyb_bind_company_rut(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_bind_company_rut() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
DECLARE claim_count bigint; owned_by uuid;
BEGIN
    PERFORM 1 FROM public.organizacion WHERE id = NEW.organization_id FOR UPDATE;
    SELECT organizacion_id INTO owned_by FROM public.organizacion_rut_fingerprint
      WHERE fingerprint=NEW.company_rut_fingerprint;
    IF owned_by IS NOT NULL AND owned_by <> NEW.organization_id THEN
        RAISE EXCEPTION 'company RUT fingerprint is reserved by another organization' USING ERRCODE = '23505';
    END IF;
    SELECT count(*) INTO claim_count FROM public.organizacion_rut_fingerprint
      WHERE organizacion_id=NEW.organization_id;
    IF owned_by IS NULL THEN
        IF claim_count > 0 THEN
            RAISE EXCEPTION 'company RUT fingerprint is not one of this organization keyring candidates' USING ERRCODE = '23505';
        END IF;
        INSERT INTO public.organizacion_rut_fingerprint(organizacion_id, fingerprint)
          VALUES(NEW.organization_id, NEW.company_rut_fingerprint);
    END IF;
    UPDATE public.organizacion SET company_rut_fingerprint = COALESCE(company_rut_fingerprint, NEW.company_rut_fingerprint),
      legal_name = COALESCE(legal_name, NEW.legal_name), updated_at = clock_timestamp()
      WHERE id = NEW.organization_id;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_enqueue_due_reminders(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_enqueue_due_reminders(p_at timestamp with time zone DEFAULT clock_timestamp(), p_limit integer DEFAULT 100) RETURNS integer
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    profile record;
    recipient record;
    message text;
    rows_enqueued integer := 0;
BEGIN
    IF p_at IS NULL OR p_limit < 1 OR p_limit > 500 THEN
        RAISE EXCEPTION 'invalid KYB reminder batch arguments' USING ERRCODE = '22023';
    END IF;

    FOR profile IN
        SELECT p.usuario_id AS profile_id,p.grace_started_at,p.grace_due_at,p.compliance_status,n.reminder_stage,n.available_at
        FROM public.anfitrion_cumplimiento_personal p
        CROSS JOIN LATERAL public.kyb_next_reminder(p.grace_started_at,p.grace_due_at,p.last_reminder_stage,p_at) n
        WHERE (p.compliance_status IN ('grace_active','in_review')
               OR (p.compliance_status='disabled_missing_docs' AND p.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND p.last_reminder_stage IS DISTINCT FROM 'due'))
          AND EXISTS (SELECT 1 FROM public.usuario u WHERE u.id=p.usuario_id AND u.estado='activo')
          AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(p.usuario_id,p_at) k WHERE k.verified)
          AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d
              WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW') AND d.submitted_at<=p.grace_due_at)
        ORDER BY n.available_at,p.usuario_id
        LIMIT p_limit FOR UPDATE OF p SKIP LOCKED
    LOOP
        message := CASE profile.reminder_stage
            WHEN 'day0' THEN 'Comenzó tu plazo documental de dos meses. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Adjunta tu certificado de Inicio de Actividades SII en EspaciGo.'
            WHEN 'month1' THEN 'Tu plazo documental continúa vigente. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Revisa tu expediente SII en EspaciGo.'
            WHEN 'day15' THEN 'Quedan 15 días para completar la documentación SII. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day7' THEN 'Quedan 7 días para completar la documentación SII. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day1' THEN 'Tu plazo documental vence mañana. Completa la documentación SII para evitar la inhabilitación para publicar y administrar espacios.'
            WHEN 'reactivated' THEN 'Tu perfil comercial volvió a estar habilitado. Ya puedes administrar y publicar tus espacios en EspaciGo.'
            ELSE CASE WHEN profile.compliance_status='disabled_missing_docs' THEN 'El plazo documental venció y tu perfil quedó inhabilitado para publicar y administrar espacios. Regulariza el certificado SII en EspaciGo.' ELSE 'Hoy vence tu plazo documental de dos meses. Regulariza el certificado SII en EspaciGo para mantener habilitado tu perfil comercial.' END
        END;
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(),profile.profile_id,'kyb_grace_'||profile.reminder_stage,'Documentación comercial pendiente',message,'/anfitrion/kyb',profile.profile_id,p_at)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
        INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,available_at,created_at)
        VALUES('personal',profile.profile_id,profile.profile_id,profile.reminder_stage,p_at,p_at)
        ON CONFLICT(profile_type,profile_id,recipient_id,reminder_stage) WHERE reminder_stage<>'reactivated' DO NOTHING;
        INSERT INTO public.host_compliance_event(usuario_id,event_type,reminder_stage,occurred_at)
        VALUES(profile.profile_id,'reminder_sent',profile.reminder_stage,p_at);
        UPDATE public.anfitrion_cumplimiento_personal SET last_reminder_stage=profile.reminder_stage,updated_at=p_at
        WHERE usuario_id=profile.profile_id;
        rows_enqueued := rows_enqueued + 1;
    END LOOP;

    FOR profile IN
        SELECT o.id AS profile_id,o.grace_started_at,o.grace_due_at,o.compliance_status,n.reminder_stage,n.available_at
        FROM public.organizacion o
        CROSS JOIN LATERAL public.kyb_next_reminder(o.grace_started_at,o.grace_due_at,o.last_reminder_stage,p_at) n
        WHERE (o.compliance_status IN ('grace_active','in_review')
               OR (o.compliance_status='disabled_missing_docs' AND o.disabled_reason='Plazo de gracia vencido sin documentación aprobada' AND o.last_reminder_stage IS DISTINCT FROM 'due'))
          AND EXISTS (SELECT 1 FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
              WHERE m.organizacion_id=o.id AND m.estado='activo' AND m.rol IN ('representante_legal','administrador') AND u.estado='activo'
                AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,p_at) k WHERE k.verified))
          AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v
              WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW') AND v.submitted_at<=o.grace_due_at
                AND (SELECT count(DISTINCT d.document_kind) FROM public.kyb_verification_document d
                     WHERE d.verification_id=v.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3)
        ORDER BY n.available_at,o.id
        LIMIT p_limit FOR UPDATE OF o SKIP LOCKED
    LOOP
        message := CASE profile.reminder_stage
            WHEN 'day0' THEN 'Comenzó el plazo documental de dos meses de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Completa SII y KYB en EspaciGo.'
            WHEN 'month1' THEN 'El plazo documental de tu empresa continúa vigente. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '. Revisa el expediente SII y KYB en EspaciGo.'
            WHEN 'day15' THEN 'Quedan 15 días para completar el expediente SII y KYB de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day7' THEN 'Quedan 7 días para completar el expediente SII y KYB de tu empresa. Fecha límite: ' || to_char(profile.grace_due_at AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.'
            WHEN 'day1' THEN 'El plazo documental de tu empresa vence mañana. Completa e-RUT, SII y poderes para evitar la inhabilitación comercial.'
            WHEN 'reactivated' THEN 'El perfil comercial de tu empresa volvió a estar habilitado para administrar y publicar espacios en EspaciGo.'
            ELSE CASE WHEN profile.compliance_status='disabled_missing_docs' THEN 'El plazo documental venció y el perfil comercial de tu empresa quedó inhabilitado. Regulariza SII y KYB en EspaciGo.' ELSE 'Hoy vence el plazo documental de dos meses de tu empresa. Regulariza SII y KYB en EspaciGo para mantener habilitado el perfil comercial.' END
        END;
        FOR recipient IN
            SELECT m.usuario_id FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
            WHERE m.organizacion_id=profile.profile_id AND m.estado='activo' AND u.estado='activo' AND m.rol IN ('representante_legal','administrador')
              AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,p_at) k WHERE k.verified)
            ORDER BY m.usuario_id
        LOOP
            INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
            VALUES(gen_random_uuid(),recipient.usuario_id,'kyb_grace_'||profile.reminder_stage,'Documentación comercial pendiente',message,'/anfitrion/kyb',profile.profile_id,p_at)
            ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
            INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,available_at,created_at)
            VALUES('organization',profile.profile_id,recipient.usuario_id,profile.reminder_stage,p_at,p_at)
            ON CONFLICT(profile_type,profile_id,recipient_id,reminder_stage) WHERE reminder_stage<>'reactivated' DO NOTHING;
        END LOOP;
        INSERT INTO public.host_compliance_event(organizacion_id,event_type,reminder_stage,occurred_at)
        VALUES(profile.profile_id,'reminder_sent',profile.reminder_stage,p_at);
        UPDATE public.organizacion SET last_reminder_stage=profile.reminder_stage,updated_at=p_at
        WHERE id=profile.profile_id;
        rows_enqueued := rows_enqueued + 1;
    END LOOP;
    RETURN rows_enqueued;
END;
$$;


--
-- Name: kyb_expire_noncompliant_profiles(timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_expire_noncompliant_profiles(p_at timestamp with time zone DEFAULT clock_timestamp()) RETURNS TABLE(usuario_id uuid, organizacion_id uuid)
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
    RETURN QUERY
      WITH expired_org AS (
        UPDATE public.organizacion o SET compliance_status='disabled_missing_docs', disabled_at=p_at,
          disabled_reason='Plazo de gracia vencido sin documentación aprobada', updated_at=p_at
        WHERE o.compliance_status IN ('grace_active','in_review') AND o.grace_due_at <= p_at
          AND NOT EXISTS (SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.status IN ('PENDING','IN_REVIEW')
              AND v.submitted_at <= o.grace_due_at
              AND (SELECT count(*) FROM public.kyb_verification_document d WHERE d.verification_id=v.id
                   AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3)
        RETURNING o.id
      ), org_events AS (
        INSERT INTO public.host_compliance_event AS event(organizacion_id,event_type,reason,occurred_at)
        SELECT id,'disabled_auto','Plazo de gracia vencido sin documentación aprobada',p_at FROM expired_org
        RETURNING event.organizacion_id
      ), expired_person AS (
        UPDATE public.anfitrion_cumplimiento_personal p SET compliance_status='disabled_missing_docs', disabled_at=p_at,
          disabled_reason='Plazo de gracia vencido sin documentación aprobada', updated_at=p_at
        WHERE p.compliance_status IN ('grace_active','in_review') AND p.grace_due_at <= p_at
          AND NOT EXISTS (SELECT 1 FROM public.anfitrion_cumplimiento_documento d WHERE d.usuario_id=p.usuario_id AND d.status IN ('PENDING','IN_REVIEW')
                AND d.submitted_at <= p.grace_due_at)
        RETURNING p.usuario_id
      ), person_events AS (
        INSERT INTO public.host_compliance_event AS event(usuario_id,event_type,reason,occurred_at)
        SELECT person.usuario_id,'disabled_auto','Plazo de gracia vencido sin documentación aprobada',p_at FROM expired_person person
        RETURNING event.usuario_id
      ) SELECT NULL::uuid, id FROM expired_org UNION ALL SELECT person.usuario_id, NULL::uuid FROM expired_person person;
END;
$$;


--
-- Name: kyb_next_reminder(timestamp with time zone, timestamp with time zone, text, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_next_reminder(p_started_at timestamp with time zone, p_due_at timestamp with time zone, p_last_stage text, p_at timestamp with time zone) RETURNS TABLE(reminder_stage text, available_at timestamp with time zone)
    LANGUAGE sql STABLE
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    SELECT candidate.stage,candidate.when_at
    FROM (
        VALUES
            ('day0'::text,0,p_started_at),
            ('month1'::text,1,(((p_started_at AT TIME ZONE 'America/Santiago') + interval '1 month') AT TIME ZONE 'America/Santiago')),
            ('day15'::text,2,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '15 days') AT TIME ZONE 'America/Santiago')),
            ('day7'::text,3,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '7 days') AT TIME ZONE 'America/Santiago')),
            ('day1'::text,4,(((p_due_at AT TIME ZONE 'America/Santiago') - interval '1 day') AT TIME ZONE 'America/Santiago')),
            ('due'::text,5,p_due_at)
    ) AS candidate(stage,ordinal,when_at)
    WHERE candidate.ordinal > CASE
        WHEN p_last_stage IS NULL THEN -1
        WHEN p_last_stage='day0' THEN 0
        WHEN p_last_stage='month1' THEN 1
        WHEN p_last_stage='day15' THEN 2
        WHEN p_last_stage='day7' THEN 3
        WHEN p_last_stage='day1' THEN 4
        WHEN p_last_stage='due' THEN 5
        ELSE 5 END
      AND candidate.when_at<=p_at AND p_started_at IS NOT NULL AND p_due_at IS NOT NULL
    ORDER BY candidate.ordinal DESC LIMIT 1
$$;


--
-- Name: kyb_organization_reactivation_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_organization_reactivation_notice() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    member record;
    transition_id uuid := gen_random_uuid();
    at_time timestamptz := clock_timestamp();
BEGIN
    FOR member IN
        SELECT m.usuario_id FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id
        WHERE m.organizacion_id=NEW.id AND m.estado='activo' AND u.estado='activo'
          AND m.rol IN ('representante_legal','administrador')
          AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,at_time) k WHERE k.verified)
        ORDER BY m.usuario_id
    LOOP
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(),member.usuario_id,'kyb_reactivated_'||transition_id::text,'Perfil comercial habilitado',
            'El perfil comercial de tu empresa volvió a estar habilitado para administrar y publicar espacios en EspaciGo.',
            '/anfitrion/kyb',NEW.id,at_time)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
        INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,source_transition_id,available_at,created_at)
        VALUES('organization',NEW.id,member.usuario_id,'reactivated',transition_id,at_time,at_time)
        ON CONFLICT(profile_type,profile_id,recipient_id,source_transition_id) WHERE reminder_stage='reactivated' DO NOTHING;
    END LOOP;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_personal_reactivation_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_personal_reactivation_notice() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    transition_id uuid := gen_random_uuid();
    at_time timestamptz := clock_timestamp();
BEGIN
    INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
    VALUES(gen_random_uuid(),NEW.usuario_id,'kyb_reactivated_'||transition_id::text,'Perfil comercial habilitado',
        'Tu perfil comercial volvió a estar habilitado. Ya puedes administrar y publicar tus espacios en EspaciGo.',
        '/anfitrion/kyb',NEW.usuario_id,at_time)
    ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
    INSERT INTO public.kyb_compliance_notice_outbox(profile_type,profile_id,recipient_id,reminder_stage,source_transition_id,available_at,created_at)
    VALUES('personal',NEW.usuario_id,NEW.usuario_id,'reactivated',transition_id,at_time,at_time)
    ON CONFLICT(profile_type,profile_id,recipient_id,source_transition_id) WHERE reminder_stage='reactivated' DO NOTHING;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_record_publication_document_request(uuid, uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_record_publication_document_request(p_space uuid, p_actor uuid) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
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


--
-- Name: kyb_space_compliance_detail(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_space_compliance_detail(p_espacio_id uuid, p_at timestamp with time zone DEFAULT clock_timestamp()) RETURNS TABLE(space_state text, host_status text, grace_due_at timestamp with time zone, property_approved boolean, property_submitted_in_time boolean, titular_approved boolean, holder_submitted_in_time boolean, category_sii_required boolean, category_sii_approved boolean, category_sii_submitted_in_time boolean, categoria_codigo text)
    LANGUAGE sql STABLE
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    WITH holder AS (
        SELECT e.id, e.estado, e.propietario_id, e.organizacion_id, e.modo_anfitrion, e.categoria_codigo,
               e.kyb_grace_due_at, p.sii_status,
               CASE WHEN e.modo_anfitrion='empresa' THEN o.grace_due_at ELSE p.grace_due_at END AS holder_due_at,
               CASE WHEN e.modo_anfitrion='empresa' THEN COALESCE(o.compliance_status,'disabled_missing_docs')
                    ELSE COALESCE(p.compliance_status,'grace_active') END AS recorded_status
        FROM public.espacio e
        LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
        LEFT JOIN public.anfitrion_cumplimiento_personal p ON e.organizacion_id IS NULL AND p.usuario_id=e.propietario_id
        WHERE e.id=p_espacio_id
    ), holder_state AS (
        SELECT h.*,
               CASE WHEN h.modo_anfitrion='empresa' THEN EXISTS (
                   SELECT 1 FROM public.kyb_verifications v
                   WHERE v.organization_id=h.organizacion_id AND v.status IN ('PENDING','IN_REVIEW')
                     AND v.submitted_at<=LEAST(h.holder_due_at, p_at)
                     AND (SELECT count(*) FROM public.kyb_verification_document d
                          WHERE d.verification_id=v.id
                            AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
               ) ELSE EXISTS (
                   SELECT 1 FROM public.anfitrion_cumplimiento_documento d
                   WHERE d.usuario_id=h.propietario_id AND d.status IN ('PENDING','IN_REVIEW')
                     AND d.submitted_at<=LEAST(h.holder_due_at, p_at)
               ) END AS holder_submitted_in_time
        FROM holder h
    ), evaluated AS (
        SELECT hs.*,
               CASE
                   WHEN hs.recorded_status IN ('grace_active','in_review') AND hs.holder_due_at<=p_at
                        AND NOT hs.holder_submitted_in_time THEN 'disabled_missing_docs'
                   ELSE hs.recorded_status
               END AS effective_host_status,
               EXISTS (
                   SELECT 1 FROM public.space_kyb_verification sv
                   WHERE sv.espacio_id=hs.id AND sv.document_kind='property_or_sublease_authorization'
                     AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
               ) AS property_approved,
               (
                   hs.modo_anfitrion='empresa' AND hs.organizacion_id IS NOT NULL
                   AND EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       JOIN public.espacio other_e ON other_e.id=sv.espacio_id
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='property_or_sublease_authorization'
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                         AND other_e.categoria_codigo <> hs.categoria_codigo
                   )
                   AND NOT EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       JOIN public.espacio same_e ON same_e.id=sv.espacio_id
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='property_or_sublease_authorization'
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                         AND same_e.categoria_codigo = hs.categoria_codigo
                   )
                   AND NOT EXISTS (
                       SELECT 1 FROM public.space_kyb_verification sv
                       WHERE sv.organizacion_id=hs.organizacion_id
                         AND sv.document_kind='sii_start_activities'
                         AND sv.categoria_codigo=hs.categoria_codigo
                         AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
                   )
               ) AS category_sii_required,
               EXISTS (
                   SELECT 1 FROM public.space_kyb_verification sv
                   WHERE sv.organizacion_id=hs.organizacion_id
                     AND sv.document_kind='sii_start_activities'
                     AND sv.categoria_codigo=hs.categoria_codigo
                     AND sv.status='APPROVED' AND sv.reviewed_at<=p_at
               ) AS category_sii_doc_approved
        FROM holder_state hs
    ), deadlines AS (
        SELECT ev.*,
               CASE
                   WHEN ev.effective_host_status='compliant' THEN
                       CASE
                           WHEN ev.estado='borrador' THEN ev.kyb_grace_due_at
                           ELSE COALESCE(ev.kyb_grace_due_at, ev.holder_due_at)
                       END
                   ELSE ev.holder_due_at
               END AS effective_due_at
        FROM evaluated ev
    )
    SELECT d.estado,
           d.effective_host_status,
           d.effective_due_at,
           d.property_approved,
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.espacio_id=d.id AND sv.document_kind='property_or_sublease_authorization'
                 AND sv.status IN ('PENDING','IN_REVIEW') AND sv.submitted_at<=LEAST(d.effective_due_at, p_at)
           ),
           CASE WHEN d.modo_anfitrion='empresa' THEN EXISTS (
               SELECT 1
               FROM (
                   SELECT v.submitted_by
                   FROM public.kyb_verifications v
                   WHERE v.organization_id=d.organizacion_id AND v.status='APPROVED' AND v.reviewed_at<=p_at
                     AND (SELECT count(*) FROM public.kyb_verification_document d2
                          WHERE d2.verification_id=v.id
                            AND d2.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
                   ORDER BY v.reviewed_at DESC,v.id DESC LIMIT 1
               ) verified_company
               JOIN public.organizacion_miembro m ON m.organizacion_id=d.organizacion_id
                 AND m.usuario_id=verified_company.submitted_by AND m.estado='activo' AND m.rol='representante_legal'
               CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,p_at) k
               WHERE k.verified
           ) ELSE COALESCE(d.sii_status='APPROVED', false)
               AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(d.propietario_id,p_at) k WHERE k.verified)
           END,
           d.holder_submitted_in_time,
           d.category_sii_required,
           (NOT d.category_sii_required OR d.category_sii_doc_approved),
           EXISTS (
               SELECT 1 FROM public.space_kyb_verification sv
               WHERE sv.organizacion_id=d.organizacion_id
                 AND sv.categoria_codigo=d.categoria_codigo
                 AND sv.document_kind='sii_start_activities'
                 AND sv.status IN ('PENDING','IN_REVIEW')
                 AND sv.submitted_at<=LEAST(d.effective_due_at, p_at)
           ),
           d.categoria_codigo
    FROM deadlines d
$$;


--
-- Name: kyb_space_document_request_notice(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_space_document_request_notice() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    IF EXISTS (
        SELECT FROM public.kyb_space_eligibility(NEW.espacio_id, NEW.solicitado_en) e
        WHERE NOT e.space_verified
    ) THEN
        INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
        VALUES(gen_random_uuid(), NEW.actor_usuario_id, 'kyb_space_document_request', 'Documentación de la publicación',
            'Revisa el respaldo de este espacio y la documentación de su titular. Fecha límite compartida: '
            || to_char(NEW.vence_en AT TIME ZONE 'America/Santiago','DD/MM/YYYY') || '.',
            '/anfitrion/kyb', NEW.espacio_id, NEW.solicitado_en)
        ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_space_document_state(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_space_document_state(p_espacio_id uuid, p_at timestamp with time zone) RETURNS TABLE(space_state text, host_status text, grace_due_at timestamp with time zone, property_approved boolean, property_submitted_in_time boolean, titular_approved boolean, holder_submitted_in_time boolean)
    LANGUAGE sql STABLE
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    SELECT space_state, host_status, grace_due_at, property_approved,
           property_submitted_in_time, titular_approved, holder_submitted_in_time
    FROM public.kyb_space_compliance_detail(p_espacio_id, p_at)
$$;


--
-- Name: kyb_space_eligibility(uuid, timestamp with time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_space_eligibility(p_espacio_id uuid, p_at timestamp with time zone DEFAULT clock_timestamp()) RETURNS TABLE(can_publish boolean, can_book boolean, space_verified boolean, compliance_status text)
    LANGUAGE sql STABLE
    SET search_path TO 'pg_catalog', 'public'
    AS $$
    WITH documents AS (
        SELECT *,
               (host_status='compliant' AND property_approved AND category_sii_approved) AS commercially_complete,
               (grace_due_at IS NULL OR p_at<grace_due_at) AS within_grace
        FROM public.kyb_space_compliance_detail(p_espacio_id, p_at)
    ), eligibility AS (
        SELECT *,
               (host_status<>'disabled_missing_docs' AND space_state<>'eliminada'
                   AND (commercially_complete OR within_grace OR
                        (space_state='activa'
                         AND (host_status='compliant' OR holder_submitted_in_time)
                         AND (property_approved OR property_submitted_in_time)
                         AND (category_sii_approved OR category_sii_submitted_in_time)))) AS allowed
        FROM documents
    )
    SELECT allowed, allowed AND space_state='activa', allowed AND commercially_complete AND titular_approved,
           CASE WHEN NOT allowed THEN 'disabled_missing_docs'
                WHEN commercially_complete THEN 'compliant'
                WHEN host_status='in_review' OR property_submitted_in_time OR category_sii_submitted_in_time THEN 'in_review'
                ELSE 'grace_active' END
    FROM eligibility
$$;


--
-- Name: kyb_start_space_grace(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_start_space_grace() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    started timestamptz := clock_timestamp();
    due timestamptz;
    existing_started timestamptz;
    existing_due timestamptz;
    holder_status text;
    space_started timestamptz;
    space_due timestamptz;
    started_user uuid;
    published_by uuid;
BEGIN
    SELECT h.actor_id INTO published_by
    FROM public.espacio_publicacion_historial_local h
    WHERE h.espacio_id=NEW.id AND h.estado_nuevo='activa'
      AND h.ocurrida_en=transaction_timestamp()
    ORDER BY h.id DESC LIMIT 1;
    published_by := COALESCE(published_by, NEW.propietario_id);
    due := ((started AT TIME ZONE 'America/Santiago' + interval '2 months') AT TIME ZONE 'America/Santiago');
    IF NEW.modo_anfitrion = 'empresa' THEN
        SELECT grace_started_at, grace_due_at, compliance_status
          INTO existing_started, existing_due, holder_status
          FROM public.organizacion WHERE id=NEW.organizacion_id FOR UPDATE;
        IF existing_started IS NULL THEN
            UPDATE public.organizacion SET grace_started_at=started, grace_due_at=due, updated_at=clock_timestamp() WHERE id=NEW.organizacion_id;
            INSERT INTO public.host_compliance_event(organizacion_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.organizacion_id, published_by, 'grace_started', started);
            space_started := started;
            space_due := due;
        ELSIF holder_status = 'compliant' THEN
            space_started := started;
            space_due := due;
        ELSE
            space_started := existing_started;
            space_due := existing_due;
        END IF;
    ELSE
        INSERT INTO public.anfitrion_cumplimiento_personal(usuario_id, grace_started_at, grace_due_at)
          VALUES(NEW.propietario_id, started, due) ON CONFLICT(usuario_id) DO UPDATE
          SET grace_started_at=EXCLUDED.grace_started_at, grace_due_at=EXCLUDED.grace_due_at, updated_at=clock_timestamp()
          WHERE anfitrion_cumplimiento_personal.grace_started_at IS NULL
          RETURNING usuario_id INTO started_user;
        IF started_user IS NOT NULL THEN
            INSERT INTO public.host_compliance_event(usuario_id, actor_usuario_id, event_type, occurred_at)
              VALUES(NEW.propietario_id, published_by, 'grace_started', started);
            space_started := started;
            space_due := due;
        ELSE
            SELECT grace_started_at, grace_due_at, compliance_status
              INTO existing_started, existing_due, holder_status
              FROM public.anfitrion_cumplimiento_personal WHERE usuario_id=NEW.propietario_id;
            IF holder_status = 'compliant' OR existing_started IS NULL THEN
                space_started := started;
                space_due := due;
            ELSE
                space_started := existing_started;
                space_due := existing_due;
            END IF;
        END IF;
    END IF;
    IF NEW.kyb_grace_started_at IS NULL THEN
        UPDATE public.espacio
        SET kyb_grace_started_at = space_started, kyb_grace_due_at = space_due
        WHERE id = NEW.id AND kyb_grace_started_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_submission_requires_active_member(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_submission_requires_active_member() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=NEW.organization_id
        AND m.usuario_id=NEW.submitted_by AND m.estado='activo') THEN
        RAISE EXCEPTION 'KYB submitter must be an active organization member' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: kyb_verification_event_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.kyb_verification_event_immutable() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
    RAISE EXCEPTION 'KYB verification events are append-only' USING ERRCODE = '23514';
END;
$$;


--
-- Name: local_contract_effective_retention_deadline(uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.local_contract_effective_retention_deadline(p_reservation uuid) RETURNS timestamp with time zone
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
SELECT GREATEST(r.actualizada_en,
  COALESCE((SELECT max(p.actualizada_en) FROM public.reserva_pago_ensayo_operacion p WHERE p.reserva_id=r.id),r.actualizada_en),
  COALESCE((SELECT max(d.actualizada_en) FROM public.reserva_devolucion_ensayo d WHERE d.reserva_id=r.id),r.actualizada_en),
  COALESCE((SELECT max(d.cerrada_en) FROM public.disputa_ensayo_local d WHERE d.reserva_id=r.id),r.actualizada_en),
  COALESCE((SELECT max(x.resuelta_en) FROM public.reclamo_dano_resolucion_ensayo_local x JOIN public.reclamo_dano_ensayo_local c ON c.id=x.reclamo_id WHERE c.reserva_id=r.id),r.actualizada_en),
  COALESCE((SELECT max(s.actualizada_en) FROM public.reserva_liquidacion_ensayo_local s WHERE s.reserva_id=r.id),r.actualizada_en)
)+interval '24 months'
FROM public.reserva_ensayo_local r WHERE r.id=p_reservation;
$$;


--
-- Name: local_retention_reservation_has_open_obligation(uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.local_retention_reservation_has_open_obligation(p_reservation uuid) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
SELECT
  EXISTS(SELECT 1 FROM public.reserva_pago_ensayo_operacion p WHERE p.reserva_id=p_reservation AND p.estado='pendiente')
  OR EXISTS(SELECT 1 FROM public.reserva_pago_fake_resultado_ensayo f JOIN public.reserva_pago_ensayo_operacion p ON p.id=f.operacion_id WHERE p.reserva_id=p_reservation AND f.estado='resultado' AND NOT EXISTS(SELECT 1 FROM public.reserva_pago_evento_ensayo e WHERE e.operacion_id=p.id AND e.proveedor_evento_id=f.proveedor_evento_id))
  OR EXISTS(SELECT 1 FROM public.reserva_pago_evento_ensayo e JOIN public.reserva_pago_ensayo_operacion p ON p.id=e.operacion_id WHERE p.reserva_id=p_reservation AND (NOT EXISTS(SELECT 1 FROM public.reserva_pago_evento_aplicacion_ensayo a WHERE a.evento_id=e.id) OR EXISTS(SELECT 1 FROM public.reserva_pago_evento_aplicacion_ensayo a WHERE a.evento_id=e.id AND a.estado IN ('pendiente','pendiente_conciliacion'))))
  OR EXISTS(SELECT 1 FROM public.reserva_devolucion_ensayo d WHERE d.reserva_id=p_reservation AND d.estado='pendiente')
  OR EXISTS(SELECT 1 FROM public.reserva_garantia_ensayo_local g WHERE g.reserva_id=p_reservation AND (g.estado IN ('pendiente_pago','pendiente_autorizacion','por_conciliar','captura_pendiente','captura_por_conciliar','liberacion_pendiente','liberacion_por_conciliar') OR g.autorizado_clp>g.capturado_clp+g.liberado_clp))
  OR EXISTS(SELECT 1 FROM public.reserva_garantia_operacion_ensayo_local o JOIN public.reserva_garantia_ensayo_local g ON g.id=o.garantia_id WHERE g.reserva_id=p_reservation AND o.estado IN ('pendiente','por_conciliar'))
  OR EXISTS(SELECT 1 FROM public.reserva_garantia_evento_aplicacion_ensayo_local a JOIN public.reserva_garantia_evento_ensayo_local e ON e.id=a.evento_id JOIN public.reserva_garantia_operacion_ensayo_local o ON o.id=e.operacion_id JOIN public.reserva_garantia_ensayo_local g ON g.id=o.garantia_id WHERE g.reserva_id=p_reservation AND a.estado IN ('pendiente','por_conciliar'))
  OR EXISTS(SELECT 1 FROM public.reserva_decision_financiera_ensayo_local d WHERE d.reserva_id=p_reservation AND d.estado IN ('pendiente','por_conciliar'))
  OR EXISTS(SELECT 1 FROM public.reserva_liquidacion_ensayo_local s WHERE s.reserva_id=p_reservation AND s.estado IN ('pendiente','por_conciliar'))
  OR EXISTS(SELECT 1 FROM public.reserva_ensayo_local r WHERE r.id=p_reservation AND r.estado='finalizada' AND r.liquidacion_politica_version='liquidacion_local_0pct_v1'
       AND EXISTS(SELECT 1 FROM public.operacion_arriendo_ensayo_local checkout WHERE checkout.reserva_id=r.id AND checkout.tipo='checkout')
       AND COALESCE((SELECT sum(p.importe_clp) FROM public.reserva_pago_ensayo p WHERE p.reserva_id=r.id AND p.resultado='exito_simulado'),0) > COALESCE((SELECT sum(d.importe_clp) FROM public.reserva_devolucion_ensayo d WHERE d.reserva_id=r.id AND d.estado='completada'),0)
       AND NOT EXISTS(SELECT 1 FROM public.reserva_liquidacion_ensayo_local s WHERE s.reserva_id=r.id))
  OR EXISTS(SELECT 1 FROM public.disputa_ensayo_local d WHERE d.reserva_id=p_reservation AND d.estado='abierta')
  OR EXISTS(SELECT 1 FROM public.reclamo_dano_ensayo_local c WHERE c.reserva_id=p_reservation AND c.estado='abierto')
  OR EXISTS(SELECT 1 FROM public.operacion_arriendo_archivo_candidato_local f WHERE f.reserva_id=p_reservation AND f.estado IN ('reservado','pendiente_limpieza','limpiando'));
$$;


--
-- Name: maintain_commercial_agreement_quote_anchor(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.maintain_commercial_agreement_quote_anchor() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
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


--
-- Name: organizacion_miembro_requires_legal_rep(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.organizacion_miembro_requires_legal_rep() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
DECLARE org_id uuid; org_status text; rep_count bigint;
BEGIN
    org_id := COALESCE(NEW.organizacion_id, OLD.organizacion_id);
    SELECT compliance_status INTO org_status FROM public.organizacion WHERE id = org_id FOR UPDATE;
    IF org_status IS NOT NULL AND org_status <> 'disabled_missing_docs' THEN
        SELECT count(*) INTO rep_count FROM public.organizacion_miembro
        WHERE organizacion_id = org_id AND estado = 'activo' AND rol = 'representante_legal';
        IF rep_count = 0 THEN
            RAISE EXCEPTION 'an active organization must retain an active legal representative' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: prevent_local_publication_history_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.prevent_local_publication_history_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF current_setting('app.local_retention_purge',true)='on' THEN
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    IF (to_jsonb(OLD)-'retirar_en') IS DISTINCT FROM (to_jsonb(NEW)-'retirar_en') THEN
      RAISE EXCEPTION 'only retention metadata may change' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'local publication history is append-only' USING ERRCODE='55000';
END;
$$;


--
-- Name: prevent_local_space_deletion_history_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.prevent_local_space_deletion_history_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF current_setting('app.local_retention_purge',true)='on' AND TG_OP='DELETE' THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'local space deletion history is append-only' USING ERRCODE='55000';
END;
$$;


--
-- Name: protect_commercial_agreement_event(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_commercial_agreement_event() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
 RAISE EXCEPTION 'commercial agreement events are immutable' USING ERRCODE='23514';
END $$;


--
-- Name: protect_commercial_contract_template_version(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_commercial_contract_template_version() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
    RAISE EXCEPTION 'published commercial contract templates are immutable' USING ERRCODE = '23514';
END;
$$;


--
-- Name: protect_commercial_rental_authorization_event(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.protect_commercial_rental_authorization_event() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$ BEGIN RAISE EXCEPTION 'authorization history is append-only' USING ERRCODE='23514'; END $$;


--
-- Name: public_catalog_billed_units(text, timestamp with time zone, timestamp with time zone, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.public_catalog_billed_units(p_unit text, p_start timestamp with time zone, p_end timestamp with time zone, p_zone text) RETURNS bigint
    LANGUAGE plpgsql STABLE STRICT PARALLEL SAFE
    AS $$
DECLARE
    local_start timestamp;
    local_end timestamp;
    month_delta integer;
    target_month date;
    anniversary_day integer;
    target_day integer;
    anniversary_local timestamp;
    end_boundary timestamptz;
    anniversary_instant timestamptz;
    end_date date;
BEGIN
    IF p_end <= p_start OR p_zone = '' THEN RETURN NULL; END IF;
    local_start := p_start AT TIME ZONE p_zone;
    local_end := p_end AT TIME ZONE p_zone;
    CASE p_unit
    WHEN 'hora' THEN RETURN GREATEST(1,ceil(extract(epoch FROM (p_end-p_start))/3600.0)::bigint);
    WHEN 'dia' THEN
        end_date := local_end::date;
        end_boundary := public.public_catalog_resolve_wall(end_date::timestamp,p_zone);
        IF p_end=end_boundary THEN end_date := end_date-1; END IF;
        RETURN end_date-local_start::date+1;
    WHEN 'mes' THEN
        month_delta := (extract(year FROM local_end)::integer-extract(year FROM local_start)::integer)*12
                     + extract(month FROM local_end)::integer-extract(month FROM local_start)::integer;
        IF month_delta < 1 THEN RETURN 1; END IF;
        target_month := (date_trunc('month',local_start)::date + make_interval(months=>month_delta))::date;
        anniversary_day := extract(day FROM local_start)::integer;
        target_day := LEAST(anniversary_day,extract(day FROM (target_month + interval '1 month - 1 day'))::integer);
        anniversary_local := target_month + (target_day-1) + local_start::time;
        anniversary_instant := public.public_catalog_resolve_wall(anniversary_local,p_zone);
        IF anniversary_instant IS NULL THEN RETURN NULL; END IF;
        IF p_end > anniversary_instant THEN month_delta := month_delta+1; END IF;
        RETURN month_delta;
    ELSE RETURN NULL;
    END CASE;
EXCEPTION WHEN invalid_parameter_value OR datetime_field_overflow THEN
    RETURN NULL;
END;
$$;


--
-- Name: public_catalog_resolve_wall(timestamp without time zone, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.public_catalog_resolve_wall(p_wall timestamp without time zone, p_zone text) RETURNS timestamp with time zone
    LANGUAGE plpgsql STABLE PARALLEL SAFE
    AS $$
DECLARE
    guess timestamptz;
    offset_values integer[];
    offset_value integer;
    candidate timestamptz;
    resolved timestamptz;
    low_probe timestamptz;
    high_probe timestamptz;
    middle_probe timestamptz;
    old_offset integer;
    new_offset integer;
    before_wall timestamp;
    after_wall timestamp;
    step_index integer;
BEGIN
    guess := p_wall AT TIME ZONE p_zone;
    SELECT array_agg(DISTINCT extract(epoch FROM ((probe AT TIME ZONE p_zone) - (probe AT TIME ZONE 'UTC')))::integer)
      INTO offset_values
      FROM generate_series(guess - interval '48 hours', guess + interval '48 hours', interval '6 hours') AS probes(probe);

    FOREACH offset_value IN ARRAY offset_values LOOP
        candidate := (p_wall - make_interval(secs => offset_value)) AT TIME ZONE 'UTC';
        IF candidate AT TIME ZONE p_zone = p_wall AND (resolved IS NULL OR candidate < resolved) THEN
            resolved := candidate;
        END IF;
    END LOOP;
    IF resolved IS NOT NULL THEN RETURN resolved; END IF;

    -- No exact wall-time candidate means p_wall lies in a forward gap. Find
    -- that transition and use its first valid instant, matching calendar.go.
    low_probe := guess - interval '48 hours';
    FOR step_index IN 1..16 LOOP
        high_probe := low_probe + interval '6 hours';
        SELECT extract(epoch FROM ((low_probe AT TIME ZONE p_zone) - (low_probe AT TIME ZONE 'UTC')))::integer,
               extract(epoch FROM ((high_probe AT TIME ZONE p_zone) - (high_probe AT TIME ZONE 'UTC')))::integer
          INTO old_offset,new_offset;
        IF new_offset > old_offset THEN
            WHILE high_probe - low_probe > interval '1 second' LOOP
                middle_probe := low_probe + (high_probe - low_probe) / 2;
                IF extract(epoch FROM ((middle_probe AT TIME ZONE p_zone) - (middle_probe AT TIME ZONE 'UTC')))::integer = old_offset THEN
                    low_probe := middle_probe;
                ELSE
                    high_probe := middle_probe;
                END IF;
            END LOOP;
            before_wall := (high_probe AT TIME ZONE 'UTC') + make_interval(secs => old_offset);
            after_wall := (high_probe AT TIME ZONE 'UTC') + make_interval(secs => new_offset);
            IF p_wall >= before_wall AND p_wall < after_wall THEN RETURN high_probe; END IF;
        END IF;
        low_probe := high_probe;
    END LOOP;
    RETURN NULL;
EXCEPTION WHEN invalid_parameter_value OR datetime_field_overflow THEN
    RETURN NULL;
END;
$$;


--
-- Name: purge_delivered_identity_session_outbox(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_delivered_identity_session_outbox() RETURNS bigint
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
DECLARE
    v_now timestamptz := pg_catalog.clock_timestamp();
    v_deleted bigint;
BEGIN
    WITH candidate AS MATERIALIZED (
        SELECT o.id
          FROM public.outbox_evento_local AS o
         WHERE o.agregado_tipo = 'didit_session'
           AND o.tipo = 'didit.session_remote_deletion'
           AND pg_catalog.starts_with(o.clave_deduplicacion, 'didit_session_deletion:')
           AND pg_catalog.char_length(o.clave_deduplicacion) > pg_catalog.char_length('didit_session_deletion:')
           AND pg_catalog.char_length(o.clave_deduplicacion) <= 120
           AND pg_catalog.substr(o.clave_deduplicacion,
                 pg_catalog.char_length('didit_session_deletion:') + 1)
               = pg_catalog.btrim(pg_catalog.substr(o.clave_deduplicacion,
                 pg_catalog.char_length('didit_session_deletion:') + 1))
           AND o.entregada_en IS NOT NULL
           AND o.retirar_en IS NOT NULL
           AND o.retirar_en <= v_now
           AND o.retirar_en >= o.entregada_en + interval '30 days'
           AND o.cancelada_en IS NULL
           AND o.fallo_terminal_en IS NULL
         ORDER BY o.retirar_en, o.id
         LIMIT 100
         FOR UPDATE OF o SKIP LOCKED
    )
    DELETE FROM public.outbox_evento_local AS o
     USING candidate AS c
     WHERE o.id = c.id
       AND o.agregado_tipo = 'didit_session'
       AND o.tipo = 'didit.session_remote_deletion'
       AND pg_catalog.starts_with(o.clave_deduplicacion, 'didit_session_deletion:')
       AND pg_catalog.char_length(o.clave_deduplicacion) > pg_catalog.char_length('didit_session_deletion:')
       AND pg_catalog.char_length(o.clave_deduplicacion) <= 120
       AND pg_catalog.substr(o.clave_deduplicacion,
             pg_catalog.char_length('didit_session_deletion:') + 1)
           = pg_catalog.btrim(pg_catalog.substr(o.clave_deduplicacion,
             pg_catalog.char_length('didit_session_deletion:') + 1))
       AND o.entregada_en IS NOT NULL
       AND o.retirar_en IS NOT NULL
       AND o.retirar_en <= v_now
       AND o.retirar_en >= o.entregada_en + interval '30 days'
       AND o.cancelada_en IS NULL
       AND o.fallo_terminal_en IS NULL;
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    IF v_deleted < 0 OR v_deleted > 100 THEN
        RAISE EXCEPTION 'V94 retirement exceeded its batch bound';
    END IF;
    RETURN v_deleted;
END;
$$;


--
-- Name: purge_expired_identity_declaration_revisions(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_expired_identity_declaration_revisions() RETURNS bigint
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
DECLARE
    purged bigint := 0;
    d record;
    a record;
    session_key text;
    dedup_key text;
    deleted_rows bigint;
    locked_revision uuid;
BEGIN
    -- This scan takes no revision lock: VERIFIED writers acquire attempt, event,
    -- claim, revision, in that order. Every potentially contended row lock below
    -- is NOWAIT, inside a per-group subtransaction. A busy group rolls back all
    -- its locks and mutations rather than deleting only some of its children.
    FOR d IN SELECT r.id, r.account_id, r.retention_at
               FROM public.identity_declaration_revision r
              WHERE r.retention_at <= pg_catalog.clock_timestamp()
              ORDER BY r.retention_at, r.id
              LIMIT 100
    LOOP
        BEGIN
            -- Save and Prepare use independent account namespaces 0 and 1.
            IF NOT pg_catalog.pg_try_advisory_xact_lock(pg_catalog.hashtextextended(d.account_id::text, 0))
               OR NOT pg_catalog.pg_try_advisory_xact_lock(pg_catalog.hashtextextended(d.account_id::text, 1)) THEN
                -- Unwind any first-namespace lock acquired by this group.
                -- CONTINUE would retain it until the outer transaction ends.
                RAISE lock_not_available;
            END IF;
            PERFORM x.id FROM public.identity_provider_attempt x
             WHERE x.declaration_id=d.id AND x.account_id=d.account_id
             ORDER BY x.id FOR UPDATE OF x NOWAIT;
            PERFORM e.id FROM public.identity_provider_event e
             JOIN public.identity_provider_attempt x ON x.id=e.attempt_id
             WHERE x.declaration_id=d.id AND x.account_id=d.account_id
             ORDER BY e.id FOR UPDATE OF e NOWAIT;
            PERFORM c.fingerprint FROM public.identity_rut_claim c
             WHERE c.declaration_id=d.id AND c.account_id=d.account_id
             ORDER BY c.fingerprint FOR UPDATE OF c NOWAIT;
            locked_revision := NULL;
            SELECT r.id INTO locked_revision FROM public.identity_declaration_revision r
             WHERE r.id=d.id AND r.account_id=d.account_id
               AND r.retention_at<=pg_catalog.clock_timestamp()
             FOR UPDATE OF r NOWAIT;
            IF locked_revision IS NULL THEN RAISE lock_not_available; END IF;

        -- Durable remote-deletion intent precedes all local deletion; no network call occurs here.
        FOR a IN SELECT x.id, x.provider_session_id, x.created_at
                   FROM public.identity_provider_attempt x
                  WHERE x.declaration_id=d.id AND x.account_id=d.account_id
                  ORDER BY x.id
        LOOP
            session_key := pg_catalog.btrim(a.provider_session_id);
            IF session_key IS NOT NULL AND session_key <> '' THEN
                dedup_key := 'didit_session_deletion:' || session_key;
                IF pg_catalog.char_length(dedup_key) > 120 THEN
                    RAISE EXCEPTION 'provider session identifier exceeds outbox limit' USING ERRCODE='22023';
                END IF;

                INSERT INTO public.outbox_evento_local(id, agregado_tipo, agregado_id, tipo, clave_deduplicacion, version, creada_en, disponible_en)
                VALUES (pg_catalog.gen_random_uuid(), 'didit_session', a.id, 'didit.session_remote_deletion', dedup_key, 1, pg_catalog.clock_timestamp(), pg_catalog.clock_timestamp())
                ON CONFLICT (clave_deduplicacion) DO NOTHING;
                IF NOT EXISTS (SELECT 1 FROM public.outbox_evento_local o WHERE o.clave_deduplicacion=dedup_key AND o.agregado_tipo='didit_session' AND o.agregado_id=a.id AND o.tipo='didit.session_remote_deletion') THEN
                    RAISE EXCEPTION 'session deletion outbox key is bound to another record' USING ERRCODE='23505';
                END IF;
            END IF;
        END LOOP;

        -- All group locks are already held. Keep exact FK deletion order;
        -- verified anti-reuse claims survive without a reference to ciphertext.
        DELETE FROM public.identity_provider_event e USING public.identity_provider_attempt x
         WHERE e.attempt_id=x.id AND x.declaration_id=d.id AND x.account_id=d.account_id;
        DELETE FROM public.identity_provider_attempt x WHERE x.declaration_id=d.id AND x.account_id=d.account_id;
        DELETE FROM public.identity_rut_claim c WHERE c.declaration_id=d.id AND c.account_id=d.account_id AND c.verified_at IS NULL;
        UPDATE public.identity_rut_claim c SET declaration_id=NULL,
            updated_at=GREATEST(c.updated_at,pg_catalog.clock_timestamp())
         WHERE c.declaration_id=d.id AND c.account_id=d.account_id AND c.verified_at IS NOT NULL;
        DELETE FROM public.identity_declaration_revision r WHERE r.id=d.id AND r.account_id=d.account_id AND r.retention_at <= pg_catalog.clock_timestamp();
        GET DIAGNOSTICS deleted_rows = ROW_COUNT;
        purged := purged + deleted_rows;
        EXCEPTION WHEN lock_not_available THEN
            -- No partial outbox or deletion survives this group's subtransaction.
            NULL;
        END;
    END LOOP;
    RETURN purged;
END;
$$;


--
-- Name: purge_expired_local_communication(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_expired_local_communication(p_at timestamp with time zone, p_limit integer) RETURNS TABLE(reviews_purged bigint, reports_purged bigint, notices_purged bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE ids uuid[];
BEGIN
  IF p_limit<1 OR p_limit>500 THEN RAISE EXCEPTION 'invalid batch'; END IF;
  SELECT array_agg(id) INTO ids FROM (
    SELECT r.id FROM public.reporte_resena_ensayo_local r
    WHERE r.estado IN ('desestimada','oculta') AND r.retirar_en<=p_at
    ORDER BY r.retirar_en,r.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) due;
  IF cardinality(ids)>0 THEN
    DELETE FROM public.reporte_resena_historial_local WHERE reporte_id=ANY(ids);
    DELETE FROM public.reporte_resena_ensayo_local WHERE id=ANY(ids);
    GET DIAGNOSTICS reports_purged = ROW_COUNT;
  ELSE reports_purged := 0; END IF;

  SELECT array_agg(id) INTO ids FROM (
    SELECT r.id FROM public.resena_ensayo_local r
    WHERE r.retirar_en<=p_at AND NOT EXISTS(SELECT 1 FROM public.reporte_resena_ensayo_local q WHERE q.resena_id=r.id)
    ORDER BY r.retirar_en,r.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) due;
  IF cardinality(ids)>0 THEN
    DELETE FROM public.resena_ensayo_local WHERE id=ANY(ids);
    GET DIAGNOSTICS reviews_purged = ROW_COUNT;
  ELSE reviews_purged := 0; END IF;

  SELECT array_agg(id) INTO ids FROM (
    SELECT a.id FROM public.aviso_local a
    WHERE a.retirar_en<=p_at AND a.estado IN ('entregada','fallo_terminal','cancelada') AND a.lease_hasta IS NULL
    ORDER BY a.retirar_en,a.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) due;
  IF cardinality(ids)>0 THEN
    DELETE FROM public.aviso_local_ciclo WHERE aviso_id=ANY(ids);
    DELETE FROM public.aviso_local WHERE id=ANY(ids);
    GET DIAGNOSTICS notices_purged = ROW_COUNT;
  ELSE notices_purged := 0; END IF;

  DELETE FROM public.resena_autoria_marca_local WHERE retirar_en<=p_at;
  DELETE FROM public.aviso_local_recuperacion_auditoria
  WHERE id IN (SELECT id FROM public.aviso_local_recuperacion_auditoria ORDER BY retirar_en,id LIMIT p_limit)
    AND retirar_en<=p_at;
  RETURN NEXT;
END $$;


--
-- Name: purge_expired_local_contracts_and_publication_history(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_expired_local_contracts_and_publication_history(p_at timestamp with time zone, p_limit integer) RETURNS TABLE(contracts_purged bigint, publication_history_purged bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE candidate record; space_candidate record; host_id uuid; renter_id uuid; document_id uuid;
        effective_deadline timestamptz; stored_deadline timestamptz;
        selected_space_ids uuid[]:=ARRAY[]::uuid[];
        selected_deletion_ids bigint[]:=ARRAY[]::bigint[]; locked_deletion_id bigint;
BEGIN
  IF p_limit<1 OR p_limit>500 THEN RAISE EXCEPTION 'invalid batch'; END IF;
  contracts_purged:=0; publication_history_purged:=0;
  PERFORM set_config('app.local_retention_purge','on',true);

  -- Lock participants in stable order, then the reservation. Re-read the
  -- current stored deadline and derive the latest effective close while that
  -- lock is held; never trust a stale/NULL value observed by the batch scan.
  FOR candidate IN
    SELECT r.id,r.anfitrion_id,r.arrendatario_id,r.vinculos_retirar_en
      FROM public.reserva_ensayo_local r
      JOIN public.contrato_ensayo_local c ON c.reserva_id=r.id
     WHERE COALESCE(r.vinculos_retirar_en,public.local_contract_effective_retention_deadline(r.id))<=p_at
       AND r.estado IN ('finalizada','cerrada','cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma','en_disputa')
     ORDER BY COALESCE(r.vinculos_retirar_en,public.local_contract_effective_retention_deadline(r.id)),r.id
     LIMIT p_limit
  LOOP
    host_id:=candidate.anfitrion_id; renter_id:=candidate.arrendatario_id;
    PERFORM u.id FROM public.usuario u
     WHERE u.id=ANY(ARRAY[host_id,renter_id]::uuid[])
     ORDER BY u.id FOR UPDATE;
    PERFORM r.id FROM public.reserva_ensayo_local r WHERE r.id=candidate.id FOR UPDATE;
    IF NOT EXISTS(SELECT 1 FROM public.reserva_ensayo_local r WHERE r.id=candidate.id
      AND r.estado IN ('finalizada','cerrada','cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma','en_disputa')) THEN
      CONTINUE;
    END IF;
    IF NOT EXISTS(SELECT 1 FROM public.contrato_ensayo_local c WHERE c.reserva_id=candidate.id) THEN
      CONTINUE;
    END IF;
    SELECT r.vinculos_retirar_en INTO stored_deadline FROM public.reserva_ensayo_local r WHERE r.id=candidate.id;
    SELECT public.local_contract_effective_retention_deadline(candidate.id) INTO effective_deadline;
    IF effective_deadline IS NULL THEN CONTINUE; END IF;
    IF stored_deadline IS NOT NULL AND stored_deadline>effective_deadline THEN
      effective_deadline:=stored_deadline;
    END IF;
    IF stored_deadline IS NULL OR stored_deadline<effective_deadline THEN
      UPDATE public.reserva_ensayo_local SET vinculos_retirar_en=effective_deadline WHERE id=candidate.id;
    END IF;
    IF effective_deadline>p_at THEN CONTINUE; END IF;
    IF public.local_retention_reservation_has_open_obligation(candidate.id) THEN
      CONTINUE;
    END IF;
    SELECT c.documento_id INTO document_id FROM public.contrato_ensayo_local c WHERE c.reserva_id=candidate.id FOR UPDATE;
    DELETE FROM public.contrato_ensayo_firma WHERE contrato_id IN (SELECT id FROM public.contrato_ensayo_local WHERE reserva_id=candidate.id);
    DELETE FROM public.contrato_ensayo_historial WHERE contrato_id IN (SELECT id FROM public.contrato_ensayo_local WHERE reserva_id=candidate.id);
    DELETE FROM public.contrato_ensayo_local WHERE reserva_id=candidate.id;
    IF document_id IS NOT NULL THEN DELETE FROM public.documento_privado_sintetico_local WHERE id=document_id; END IF;
    contracts_purged:=contracts_purged+1;
  END LOOP;

  -- Keep V53's single eligible-space set for both related history ledgers.
  FOR space_candidate IN
    SELECT h.id,h.espacio_id
      FROM public.espacio_eliminacion_historial_local h
     WHERE h.retirar_en<=p_at
       AND NOT EXISTS(SELECT 1 FROM public.reserva_ensayo_local r WHERE r.espacio_id=h.espacio_id AND (
         r.estado NOT IN ('finalizada','cerrada','cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma')
         OR GREATEST(COALESCE(r.vinculos_retirar_en,'epoch'::timestamptz),public.local_contract_effective_retention_deadline(r.id))>p_at
         OR public.local_retention_reservation_has_open_obligation(r.id)))
     ORDER BY h.retirar_en,h.id
     LIMIT p_limit
  LOOP
    PERFORM u.id FROM public.usuario u
     WHERE u.id IN (SELECT r.anfitrion_id FROM public.reserva_ensayo_local r WHERE r.espacio_id=space_candidate.espacio_id
                    UNION SELECT r.arrendatario_id FROM public.reserva_ensayo_local r WHERE r.espacio_id=space_candidate.espacio_id)
     ORDER BY u.id FOR UPDATE;
    PERFORM r.id FROM public.reserva_ensayo_local r WHERE r.espacio_id=space_candidate.espacio_id ORDER BY r.id FOR UPDATE;
    SELECT h.id INTO locked_deletion_id FROM public.espacio_eliminacion_historial_local h
     WHERE h.id=space_candidate.id AND h.retirar_en<=p_at FOR UPDATE SKIP LOCKED;
    IF locked_deletion_id IS NULL THEN CONTINUE; END IF;
    IF EXISTS(SELECT 1 FROM public.reserva_ensayo_local r WHERE r.espacio_id=space_candidate.espacio_id AND (
         r.estado NOT IN ('finalizada','cerrada','cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma')
         OR GREATEST(COALESCE(r.vinculos_retirar_en,'epoch'::timestamptz),public.local_contract_effective_retention_deadline(r.id))>p_at
         OR public.local_retention_reservation_has_open_obligation(r.id))) THEN
      CONTINUE;
    END IF;
    selected_space_ids:=array_append(selected_space_ids,space_candidate.espacio_id);
    selected_deletion_ids:=array_append(selected_deletion_ids,space_candidate.id);
  END LOOP;
  IF cardinality(selected_space_ids)>0 THEN
    DELETE FROM public.espacio_publicacion_historial_local p WHERE p.espacio_id=ANY(selected_space_ids);
    GET DIAGNOSTICS publication_history_purged=ROW_COUNT;
    DELETE FROM public.espacio_eliminacion_historial_local h WHERE h.id=ANY(selected_deletion_ids);
  END IF;
  PERFORM set_config('app.local_retention_purge','off',true);
  RETURN NEXT;
END;
$$;


--
-- Name: purge_expired_local_credential_notices(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_expired_local_credential_notices(p_at timestamp with time zone, p_limit integer) RETURNS bigint
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE removed bigint;
BEGIN
    WITH candidate AS (
        SELECT event.id FROM public.outbox_evento_local AS event
        WHERE event.tipo='identidad.credencial_cambiada' AND event.retirar_en<=p_at
          AND event.lease_hasta IS NULL AND (event.entregada_en IS NOT NULL OR event.cancelada_en IS NOT NULL OR event.fallo_terminal_en IS NOT NULL)
        ORDER BY event.retirar_en,event.id FOR UPDATE SKIP LOCKED LIMIT p_limit
    )
    DELETE FROM public.outbox_evento_local AS event USING candidate
    WHERE event.id=candidate.id;
    GET DIAGNOSTICS removed = ROW_COUNT;
    RETURN removed;
END;
$$;


--
-- Name: purge_expired_local_privacy_rows(timestamp with time zone, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_expired_local_privacy_rows(p_at timestamp with time zone, p_limit integer) RETURNS TABLE(terms_purged bigint, rights_purged bigint, quotes_purged bigint, verifications_purged bigint, files_queued bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE ids uuid[]; owner_ids uuid[]; verification_ids uuid[];
BEGIN
  IF p_limit<1 OR p_limit>500 THEN RAISE EXCEPTION 'invalid batch'; END IF;
  terms_purged:=0; rights_purged:=0; quotes_purged:=0; verifications_purged:=0; files_queued:=0;

  INSERT INTO public.verificacion_archivo_retiro_pendiente_local(evidencia_id,archivo_id,disponible_en)
  SELECT e.id,e.archivo_id,p_at
    FROM public.verificacion_evidencia_sintetica e JOIN public.verificacion v ON v.id=e.verificacion_id
   WHERE e.archivo_id IS NOT NULL
     AND v.estado<>'retirada_privacidad'
     AND (e.retirar_archivo_en<=p_at OR v.retirar_en<=p_at)
     AND NOT EXISTS(SELECT 1 FROM public.verificacion_archivo_retiro_pendiente_local j WHERE j.evidencia_id=e.id)
   ORDER BY COALESCE(e.retirar_archivo_en,v.retirar_en),e.id
   LIMIT p_limit
  ON CONFLICT(evidencia_id) DO NOTHING;
  GET DIAGNOSTICS files_queued=ROW_COUNT;

  WITH due AS (
    SELECT id FROM public.aceptacion_terminos WHERE retirar_en<=p_at
    ORDER BY retirar_en,id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) DELETE FROM public.aceptacion_terminos a USING due WHERE a.id=due.id;
  GET DIAGNOSTICS terms_purged=ROW_COUNT;

  -- A completed suppression's operation record shares its five-year period.
  -- Remove only completed filesystem jobs first; pending work has no deadline.
  SELECT array_agg(id) INTO ids FROM (
    SELECT id FROM public.ejecucion_baja_local WHERE retirar_en<=p_at
    ORDER BY retirar_en,id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) due;
  IF cardinality(ids)>0 THEN
    DELETE FROM public.baja_archivo_pendiente_local WHERE ejecucion_id=ANY(ids) AND completada_en IS NOT NULL;
    DELETE FROM public.ejecucion_baja_local WHERE id=ANY(ids);
  END IF;
  WITH due AS (
    SELECT q.id FROM public.solicitud_titular q
    WHERE q.retirar_en<=p_at
      AND NOT EXISTS(SELECT 1 FROM public.ejecucion_baja_local x WHERE x.solicitud_id=q.id)
    ORDER BY q.retirar_en,q.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) DELETE FROM public.solicitud_titular q USING due WHERE q.id=due.id;
  GET DIAGNOSTICS rights_purged=ROW_COUNT;

  WITH due AS (
    SELECT q.id FROM public.cotizacion_reserva_ensayo q
    WHERE q.retirar_en<=p_at
      AND NOT EXISTS(SELECT 1 FROM public.reserva_ensayo_local r WHERE r.cotizacion_id=q.id)
    ORDER BY q.retirar_en,q.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) DELETE FROM public.cotizacion_reserva_ensayo q USING due WHERE q.id=due.id;
  GET DIAGNOSTICS quotes_purged=ROW_COUNT;

  -- Follow the account -> verification lock order used by verification and
  -- suppression writers. SKIP LOCKED makes concurrent passes retryable.
  SELECT array_agg(id) INTO owner_ids FROM (
    SELECT u.id FROM public.usuario u
    WHERE EXISTS(SELECT 1 FROM public.verificacion v WHERE v.usuario_id=u.id
      AND v.retirar_en<=p_at AND v.estado IN ('aprobada','rechazada','revocada','retirada_privacidad'))
    ORDER BY u.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) owners;
  IF cardinality(owner_ids)>0 THEN
    SELECT array_agg(id) INTO verification_ids FROM (
      SELECT v.id FROM public.verificacion v
      WHERE v.usuario_id=ANY(owner_ids) AND v.retirar_en<=p_at
        AND v.estado IN ('aprobada','rechazada','revocada','retirada_privacidad')
        AND NOT EXISTS(SELECT 1 FROM public.verificacion_evidencia_sintetica e
                       WHERE e.verificacion_id=v.id AND e.archivo_id IS NOT NULL)
        AND NOT EXISTS(SELECT 1 FROM public.ejecucion_baja_local b
                       WHERE b.usuario_id=v.usuario_id AND b.estado='limpieza_pendiente')
      ORDER BY v.usuario_id,v.retirar_en,v.id FOR UPDATE SKIP LOCKED LIMIT p_limit
    ) due_verifications;
  END IF;
  IF cardinality(verification_ids)>0 THEN
    UPDATE public.verificacion SET reintento_de=NULL WHERE reintento_de=ANY(verification_ids);
    DELETE FROM public.elegibilidad_verificacion_local WHERE verificacion_id=ANY(verification_ids);
    DELETE FROM public.verificacion_historial_local WHERE verificacion_id=ANY(verification_ids);
    DELETE FROM public.verificacion_evidencia_sintetica WHERE verificacion_id=ANY(verification_ids);
    DELETE FROM public.verificacion WHERE id=ANY(verification_ids);
    GET DIAGNOSTICS verifications_purged=ROW_COUNT;
  END IF;
  RETURN NEXT;
END;
$$;


--
-- Name: purge_unreferenced_local_publication_rates(uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.purge_unreferenced_local_publication_rates(p_space_id uuid) RETURNS integer
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE
    removed_count integer;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.espacio
        WHERE id = p_space_id AND estado = 'eliminada'
    ) THEN
        RAISE EXCEPTION 'publication must be retired before rate cleanup'
            USING ERRCODE = '42501';
    END IF;

    DELETE FROM public.tarifa_espacio AS rate
    WHERE rate.espacio_id = p_space_id
      AND NOT EXISTS (
          SELECT 1 FROM public.cotizacion_reserva_ensayo AS quote
          WHERE quote.espacio_id = rate.espacio_id
            AND quote.tarifa_version = rate.version
      );
    GET DIAGNOSTICS removed_count = ROW_COUNT;
    RETURN removed_count;
END;
$$;


--
-- Name: rechazar_cambio_tarifa_con_horario_activo(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.rechazar_cambio_tarifa_con_horario_activo() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.modalidad <> 'hora'
       AND EXISTS (SELECT 1 FROM public.espacio_horario_semanal h
                   WHERE h.espacio_id=NEW.espacio_id AND h.activo) THEN
        RAISE EXCEPTION 'desactive el horario semanal antes de cambiar la modalidad tarifaria'
            USING ERRCODE='23514', CONSTRAINT='tarifa_horario_semanal_activo_ck';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: refresh_category_public_search_document(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.refresh_category_public_search_document() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE public.espacio SET descripcion=descripcion WHERE categoria_codigo=NEW.codigo;
    RETURN NEW;
END;
$$;


--
-- Name: refresh_espacio_public_search_document(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.refresh_espacio_public_search_document() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.busqueda_publica := public.build_public_search_document(
        NEW.estado,NEW.categoria_codigo,NEW.titulo,NEW.descripcion,
        NEW.ubicacion_catalogo_version,NEW.region_codigo,NEW.comuna_codigo
    );
    RETURN NEW;
END;
$$;


--
-- Name: reject_gadm_bootstrap_identity_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_gadm_bootstrap_identity_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF current_user = 'espacigo_gadm_bootstrap' AND NEW.id IS DISTINCT FROM OLD.id THEN
        RAISE EXCEPTION 'GADM bootstrap lock role cannot change account identity'
            USING ERRCODE = '42501';
    END IF;
    RETURN NEW;
END
$$;


--
-- Name: require_commercial_agreement_signatures(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.require_commercial_agreement_signatures() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE a public.contrato_solicitud_comercial%ROWTYPE; n integer;
BEGIN
 SELECT * INTO a FROM public.contrato_solicitud_comercial WHERE id=NEW.id;
 SELECT count(*) INTO n FROM public.contrato_firma_comercial WHERE solicitud_id=a.id AND documento_sha256=a.documento_sha256 AND propuesta_version=a.propuesta_version;
 IF n<1 OR (a.estado='ready_for_payment' AND n<>2) THEN RAISE EXCEPTION 'commercial signatures required' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;


--
-- Name: revoke_commercial_rental_authorization_on_template_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.revoke_commercial_rental_authorization_on_template_change() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE changed record;
BEGIN
 IF NEW.version_publicada IS DISTINCT FROM OLD.version_publicada THEN
  FOR changed IN UPDATE public.autorizacion_arriendo_comercial SET revocada_en=clock_timestamp(),revocada_por=propietario_id,motivo_revocacion='template_version_changed' WHERE espacio_id=NEW.espacio_id AND revocada_en IS NULL AND plantilla_version<>NEW.version_publicada RETURNING id,espacio_id,propietario_id LOOP
   INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) VALUES(gen_random_uuid(),changed.id,changed.espacio_id,changed.propietario_id,'revoked','template_version_changed',clock_timestamp());
  END LOOP;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: schedule_local_reservation_link_retention(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.schedule_local_reservation_link_retention() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.estado IN ('finalizada','cerrada','cancelada_por_pago','rechazada_arrendador','vencida_pago','vencida_host','cancelada_arrendatario','cancelada_por_firma')
       AND (TG_OP='INSERT' OR OLD.estado IS DISTINCT FROM NEW.estado) THEN
        NEW.vinculos_retirar_en := GREATEST(COALESCE(NEW.vinculos_retirar_en,'epoch'::timestamptz),NEW.actualizada_en + interval '24 months');
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: set_local_verification_retention_deadline(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.set_local_verification_retention_deadline() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP='UPDATE' AND NEW.estado IN ('retirada_privacidad','revocada') AND OLD.resuelta_en IS NOT NULL THEN
        NEW.retirar_en := OLD.retirar_en;
    ELSIF NEW.estado IN ('aprobada','rechazada','retirada_privacidad') AND NEW.resuelta_en IS NOT NULL THEN
        NEW.retirar_en := NEW.resuelta_en + interval '2 years';
    ELSE
        NEW.retirar_en := NULL;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: set_verification_evidence_retention_deadline(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.set_verification_evidence_retention_deadline() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
  IF NEW.estado='retirada_privacidad' AND NEW.retirada_privacidad_en IS NOT NULL THEN
    UPDATE public.verificacion_evidencia_sintetica
       SET retirar_archivo_en=LEAST(COALESCE(retirar_archivo_en,NEW.retirada_privacidad_en),NEW.retirada_privacidad_en)
     WHERE verificacion_id=NEW.id AND archivo_id IS NOT NULL;
  ELSIF NEW.estado IN ('aprobada','rechazada','revocada') AND NEW.resuelta_en IS NOT NULL THEN
    UPDATE public.verificacion_evidencia_sintetica
       SET retirar_archivo_en=NEW.resuelta_en + interval '90 days'
     WHERE verificacion_id=NEW.id AND archivo_id IS NOT NULL;
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: space_kyb_owner_guard(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.space_kyb_owner_guard() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE space_mode text; space_owner uuid; space_org uuid; space_category text;
BEGIN
    SELECT modo_anfitrion, propietario_id, organizacion_id, categoria_codigo
      INTO space_mode, space_owner, space_org, space_category
      FROM public.espacio WHERE id=NEW.espacio_id;
    IF space_mode IS NULL OR space_mode <> NEW.host_mode OR space_owner <> NEW.owner_usuario_id OR
       space_org IS DISTINCT FROM NEW.organizacion_id THEN
        RAISE EXCEPTION 'space KYB scope must match the space owner and mode' USING ERRCODE = '23514';
    END IF;
    IF NEW.categoria_codigo IS NULL THEN
        NEW.categoria_codigo := space_category;
    ELSIF NEW.categoria_codigo IS DISTINCT FROM space_category THEN
        RAISE EXCEPTION 'space KYB category must match space category' USING ERRCODE = '23514';
    END IF;
    IF NEW.host_mode='personal' AND NEW.submitted_by <> NEW.owner_usuario_id THEN
        RAISE EXCEPTION 'personal space document must be submitted by its owner' USING ERRCODE = '23514';
    ELSIF NEW.host_mode='empresa' AND NOT EXISTS (SELECT 1 FROM public.organizacion_miembro m
        WHERE m.organizacion_id=NEW.organizacion_id AND m.usuario_id=NEW.submitted_by AND m.estado='activo') THEN
        RAISE EXCEPTION 'company space document submitter must be an active member' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validar_horario_semanal_fixture_hora(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.validar_horario_semanal_fixture_hora() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.activo AND NOT EXISTS (
        SELECT 1
        FROM public.espacio e
        JOIN LATERAL (
            SELECT modalidad
            FROM public.tarifa_espacio
            WHERE espacio_id=e.id
            ORDER BY version DESC
            LIMIT 1
        ) t ON true
        WHERE e.id=NEW.espacio_id
          AND e.estado <> 'eliminada'
          AND t.modalidad='hora'
    ) THEN
        RAISE EXCEPTION 'el horario semanal activo requiere una publicacion vigente con tarifa por hora'
            USING ERRCODE='23514', CONSTRAINT='horario_semanal_publicacion_hora_ck';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: espacigo_es_unaccent_dict; Type: TEXT SEARCH DICTIONARY; Schema: public; Owner: -
--

CREATE TEXT SEARCH DICTIONARY public.espacigo_es_unaccent_dict (
    TEMPLATE = public.unaccent,
    rules = 'unaccent' );


--
-- Name: espacigo_es_unaccent; Type: TEXT SEARCH CONFIGURATION; Schema: public; Owner: -
--

CREATE TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent (
    PARSER = pg_catalog."default" );

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR asciiword WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR word WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR numword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR email WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR url WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR host WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR sfloat WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR version WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR hword_numpart WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR hword_part WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR hword_asciipart WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR numhword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR asciihword WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR hword WITH public.espacigo_es_unaccent_dict, spanish_stem;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR url_path WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR file WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR "float" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR "int" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.espacigo_es_unaccent
    ADD MAPPING FOR uint WITH simple;


--
-- Name: acceso_sensible_administrativo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acceso_sensible_administrativo (
    id uuid NOT NULL,
    actor_cuenta_id uuid NOT NULL,
    objetivo_cuenta_id uuid NOT NULL,
    sesion_administrativa_id uuid,
    campo text NOT NULL,
    alcance text NOT NULL,
    motivo text NOT NULL,
    caso_id text DEFAULT ''::text NOT NULL,
    otorgado_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    evento_auditoria_id uuid,
    CONSTRAINT acceso_sensible_administrativo_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT acceso_sensible_administrativo_otorgado_en_check CHECK (isfinite(otorgado_en))
);


--
-- Name: account_identity_profile; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_profile (
    account_id uuid NOT NULL,
    username text NOT NULL,
    current_revision uuid NOT NULL,
    CONSTRAINT account_identity_profile_username_check CHECK ((username ~ '^[A-Za-z0-9_][A-Za-z0-9_.-]{2,29}$'::text))
);


--
-- Name: account_identity_profile_draft; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_profile_draft (
    account_id uuid NOT NULL,
    key_version text NOT NULL,
    encrypted_data bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT account_identity_profile_draft_encrypted_data_check CHECK ((octet_length(encrypted_data) > 28))
);


--
-- Name: account_identity_profile_phone; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_profile_phone (
    account_id uuid NOT NULL,
    key_version text NOT NULL,
    encrypted_phone bytea NOT NULL,
    masked_phone text NOT NULL
);


--
-- Name: account_identity_profile_revision; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_profile_revision (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    key_version text NOT NULL,
    encrypted_data bytea NOT NULL,
    masked_rut text NOT NULL,
    masked_phone text NOT NULL,
    saved_at timestamp with time zone NOT NULL,
    CONSTRAINT account_identity_profile_revision_encrypted_data_check CHECK ((octet_length(encrypted_data) > 28))
);


--
-- Name: account_identity_rut_claim; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_rut_claim (
    account_id uuid NOT NULL,
    identity_revision uuid NOT NULL,
    claimed_at timestamp with time zone NOT NULL
);


--
-- Name: account_identity_rut_fingerprint; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_rut_fingerprint (
    fingerprint text NOT NULL,
    account_id uuid NOT NULL,
    CONSTRAINT account_identity_rut_fingerprint_fingerprint_check CHECK ((fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text))
);


--
-- Name: account_identity_verification; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_verification (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    profile_revision uuid NOT NULL,
    state text NOT NULL,
    provider_session uuid,
    started_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    decision_at timestamp with time zone,
    last_refresh_at timestamp with time zone,
    verified_at timestamp with time zone,
    verified_until timestamp with time zone,
    CONSTRAINT account_identity_verification_check CHECK (((state <> 'verified'::text) OR ((verified_at IS NOT NULL) AND (verified_until > verified_at)))),
    CONSTRAINT account_identity_verification_state_check CHECK ((state = ANY (ARRAY['in_progress'::text, 'in_review'::text, 'needs_correction'::text, 'verified'::text, 'expired'::text, 'revoked'::text])))
);


--
-- Name: account_identity_webhook_receipt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_identity_webhook_receipt (
    event_id uuid NOT NULL,
    attempt_id uuid NOT NULL,
    received_at timestamp with time zone NOT NULL
);


--
-- Name: account_pinpass; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_pinpass (
    account_id uuid NOT NULL,
    verifier_salt bytea NOT NULL,
    verifier_hash bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT account_pinpass_check CHECK ((updated_at >= created_at)),
    CONSTRAINT account_pinpass_verifier_hash_check CHECK ((octet_length(verifier_hash) = 32)),
    CONSTRAINT account_pinpass_verifier_salt_check CHECK ((octet_length(verifier_salt) = 16))
);


--
-- Name: account_pinpass_access; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_pinpass_access (
    account_id uuid NOT NULL,
    session_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT account_pinpass_access_check CHECK (((expires_at > created_at) AND (expires_at <= (created_at + '00:15:00'::interval))))
);


--
-- Name: account_pinpass_attempt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_pinpass_attempt (
    account_id uuid NOT NULL,
    session_id uuid,
    failed_attempts integer DEFAULT 0 NOT NULL,
    blocked_until timestamp with time zone,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT account_pinpass_attempt_failed_attempts_check CHECK ((failed_attempts >= 0))
);


--
-- Name: account_pinpass_audit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_pinpass_audit (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    session_id uuid NOT NULL,
    action text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    CONSTRAINT account_pinpass_audit_action_check CHECK ((action = ANY (ARRAY['pinpass.changed'::text, 'pinpass.reset'::text, 'pinpass.unlock_denied'::text, 'pinpass.unlocked'::text, 'pinpass.sensitive_data_viewed'::text])))
);


--
-- Name: account_pinpass_email_challenge; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_pinpass_email_challenge (
    account_id uuid NOT NULL,
    session_id uuid NOT NULL,
    challenge_id uuid NOT NULL,
    purpose text NOT NULL,
    destination text,
    code_hmac bytea NOT NULL,
    sent_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    CONSTRAINT account_pinpass_email_challenge_check CHECK ((((purpose = 'new_email'::text) AND (consumed_at IS NULL)) = (destination IS NOT NULL))),
    CONSTRAINT account_pinpass_email_challenge_check1 CHECK ((expires_at > sent_at)),
    CONSTRAINT account_pinpass_email_challenge_code_hmac_check CHECK ((octet_length(code_hmac) = 32)),
    CONSTRAINT account_pinpass_email_challenge_purpose_check CHECK ((purpose = ANY (ARRAY['pinpass'::text, 'password'::text, 'authenticator'::text, 'new_email'::text])))
);


--
-- Name: aceptacion_terminos; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aceptacion_terminos (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    version_id uuid NOT NULL,
    aceptada_en timestamp with time zone DEFAULT now() NOT NULL,
    canal text NOT NULL,
    retirar_en timestamp with time zone,
    CONSTRAINT aceptacion_terminos_canal_ck CHECK ((canal = ANY (ARRAY['web'::text, 'api'::text, 'administrado'::text])))
);


--
-- Name: administrador_global; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.administrador_global (
    usuario_id uuid NOT NULL,
    concedido_en timestamp with time zone NOT NULL,
    concedido_por uuid,
    motivo_codigo text NOT NULL,
    CONSTRAINT administrador_global_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['bootstrap_propietario'::text, 'recuperacion_local'::text, 'concesion_administrativa'::text])))
);


--
-- Name: TABLE administrador_global; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.administrador_global IS 'Membresía global de aplicación. Se concede por herramienta local auditada; el runtime solo puede leerla.';


--
-- Name: administrador_global_bootstrap; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.administrador_global_bootstrap (
    singleton boolean DEFAULT true NOT NULL,
    usuario_id uuid NOT NULL,
    creado_en timestamp with time zone NOT NULL,
    CONSTRAINT administrador_global_bootstrap_singleton_check CHECK (singleton)
);


--
-- Name: TABLE administrador_global_bootstrap; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.administrador_global_bootstrap IS 'Vincula el bootstrap inicial y permite recuperación local únicamente de la misma cuenta registrada/verificada.';


--
-- Name: administrador_global_bootstrap_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.administrador_global_bootstrap_evento (
    id uuid NOT NULL,
    usuario_objetivo_id uuid CONSTRAINT administrador_global_bootstrap_eve_usuario_objetivo_id_not_null NOT NULL,
    actor_tipo text NOT NULL,
    accion text NOT NULL,
    resultado text NOT NULL,
    motivo_codigo text NOT NULL,
    correlacion_id uuid NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT administrador_global_bootstrap_evento_accion_check CHECK ((accion = ANY (ARRAY['bootstrap'::text, 'recuperacion'::text]))),
    CONSTRAINT administrador_global_bootstrap_evento_actor_tipo_check CHECK ((actor_tipo = ANY (ARRAY['cli_local'::text, 'cli_gcp06'::text]))),
    CONSTRAINT administrador_global_bootstrap_evento_motivo_codigo_check CHECK (((length(btrim(motivo_codigo)) >= 1) AND (length(btrim(motivo_codigo)) <= 60))),
    CONSTRAINT administrador_global_bootstrap_evento_resultado_check CHECK ((resultado = ANY (ARRAY['exito'::text, 'rechazo'::text])))
);


--
-- Name: TABLE administrador_global_bootstrap_evento; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.administrador_global_bootstrap_evento IS 'Auditoría mínima append-only de bootstrap/recuperación local, sin credenciales ni datos de contacto.';


--
-- Name: anfitrion_cumplimiento_documento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anfitrion_cumplimiento_documento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    usuario_id uuid NOT NULL,
    document_kind character varying(32) NOT NULL,
    storage_ref uuid NOT NULL,
    mime_type character varying(32) NOT NULL,
    size_bytes bigint NOT NULL,
    content_sha256 bytea NOT NULL,
    status character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    rejection_reason character varying(500),
    submitted_by uuid NOT NULL,
    submitted_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    reviewer_id uuid,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT anfitrion_cumplimiento_documento_check CHECK (((((status)::text = 'REJECTED'::text) AND (rejection_reason IS NOT NULL) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text <> 'REJECTED'::text) AND (rejection_reason IS NULL)))),
    CONSTRAINT anfitrion_cumplimiento_documento_check1 CHECK (((((status)::text = ANY ((ARRAY['APPROVED'::character varying, 'REJECTED'::character varying])::text[])) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[])) AND (reviewer_id IS NULL) AND (reviewed_at IS NULL)))),
    CONSTRAINT anfitrion_cumplimiento_documento_content_sha256_check CHECK ((octet_length(content_sha256) = 32)),
    CONSTRAINT anfitrion_cumplimiento_documento_document_kind_check CHECK (((document_kind)::text = 'sii_start_activities'::text)),
    CONSTRAINT anfitrion_cumplimiento_documento_mime_type_check CHECK (((mime_type)::text = ANY ((ARRAY['application/pdf'::character varying, 'image/jpeg'::character varying, 'image/png'::character varying])::text[]))),
    CONSTRAINT anfitrion_cumplimiento_documento_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760))),
    CONSTRAINT anfitrion_cumplimiento_documento_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))
);


--
-- Name: anfitrion_cumplimiento_personal; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anfitrion_cumplimiento_personal (
    usuario_id uuid NOT NULL,
    sii_giro_code character varying(6),
    compliance_status character varying(32) DEFAULT 'grace_active'::character varying NOT NULL,
    sii_status character varying(16) DEFAULT 'MISSING'::character varying NOT NULL,
    grace_started_at timestamp with time zone,
    grace_due_at timestamp with time zone,
    disabled_at timestamp with time zone,
    disabled_reason character varying(500),
    disabled_by uuid,
    last_reminder_stage character varying(24),
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT anfitrion_cumplimiento_personal_check CHECK (((grace_due_at IS NULL) OR ((grace_started_at IS NOT NULL) AND (grace_due_at > grace_started_at)))),
    CONSTRAINT anfitrion_cumplimiento_personal_check1 CHECK (((((compliance_status)::text = 'disabled_missing_docs'::text) AND (disabled_at IS NOT NULL) AND (disabled_reason IS NOT NULL)) OR (((compliance_status)::text <> 'disabled_missing_docs'::text) AND (disabled_at IS NULL)))),
    CONSTRAINT anfitrion_cumplimiento_personal_compliance_status_check CHECK (((compliance_status)::text = ANY ((ARRAY['grace_active'::character varying, 'in_review'::character varying, 'compliant'::character varying, 'disabled_missing_docs'::character varying])::text[]))),
    CONSTRAINT anfitrion_cumplimiento_personal_sii_giro_code_check CHECK (((sii_giro_code IS NULL) OR ((sii_giro_code)::text ~ '^[0-9]{6}$'::text))),
    CONSTRAINT anfitrion_cumplimiento_personal_sii_status_check CHECK (((sii_status)::text = ANY ((ARRAY['MISSING'::character varying, 'PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))
);


--
-- Name: aprobacion_administrativa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aprobacion_administrativa (
    id uuid NOT NULL,
    tipo_accion text NOT NULL,
    recurso_tipo text NOT NULL,
    recurso_id text NOT NULL,
    solicitante_id uuid NOT NULL,
    aprobador_id uuid,
    estado text DEFAULT 'pending'::text NOT NULL,
    motivo text NOT NULL,
    decision_motivo text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    idempotency_key text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    decidida_en timestamp with time zone,
    expira_en timestamp with time zone NOT NULL,
    CONSTRAINT aprobacion_administrativa_creada_en_check CHECK (isfinite(creada_en)),
    CONSTRAINT aprobacion_administrativa_estado_check CHECK ((estado = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'cancelled'::text]))),
    CONSTRAINT aprobacion_administrativa_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT aprobacion_administrativa_tipo_accion_check CHECK ((tipo_accion = ANY (ARRAY['refund_request'::text, 'privacy_export'::text, 'staff_role_grant'::text]))),
    CONSTRAINT aprobacion_administrativa_version_check CHECK ((version > 0)),
    CONSTRAINT aprobacion_solicitante_distinto_aprobador CHECK (((aprobador_id IS NULL) OR (aprobador_id <> solicitante_id)))
);


--
-- Name: arriendo_temprano; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arriendo_temprano (
    id uuid NOT NULL,
    flow_version text DEFAULT 'manual_early_v1'::text NOT NULL,
    espacio_id uuid NOT NULL,
    anfitrion_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    titulo_snapshot text NOT NULL,
    modalidad text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    moneda character(3) NOT NULL,
    unidades bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    intervalo tstzrange GENERATED ALWAYS AS (tstzrange(inicio, termino, '[)'::text)) STORED,
    zona_horaria text NOT NULL,
    contrato_version bigint NOT NULL,
    contrato_texto text NOT NULL,
    contrato_digest bytea NOT NULL,
    estado text NOT NULL,
    revision bigint NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    ocupacion_id uuid NOT NULL,
    CONSTRAINT arriendo_temprano_check CHECK (((subtotal_clp > 0) AND ((subtotal_clp)::numeric = ((precio_unitario_clp)::numeric * (unidades)::numeric)))),
    CONSTRAINT arriendo_temprano_contrato_digest_check CHECK ((octet_length(contrato_digest) = 32)),
    CONSTRAINT arriendo_temprano_contrato_texto_check CHECK (((length(contrato_texto) >= 1) AND (length(contrato_texto) <= 20000))),
    CONSTRAINT arriendo_temprano_contrato_version_check CHECK ((contrato_version > 0)),
    CONSTRAINT arriendo_temprano_digest_ck CHECK ((contrato_digest = sha256(convert_to(contrato_texto, 'UTF8'::name)))),
    CONSTRAINT arriendo_temprano_estado_check CHECK ((estado = ANY (ARRAY['pendiente_identidad'::text, 'pendiente_contrato'::text, 'pendiente_envio_pago'::text, 'pendiente_recepcion_pago'::text, 'confirmado'::text, 'rechazado'::text, 'cancelado'::text, 'vencido'::text]))),
    CONSTRAINT arriendo_temprano_flow_version_check CHECK ((flow_version = 'manual_early_v1'::text)),
    CONSTRAINT arriendo_temprano_intervalo_ck CHECK ((isfinite(inicio) AND isfinite(termino) AND (termino > inicio) AND ((termino - inicio) = ((unidades)::double precision * '01:00:00'::interval)))),
    CONSTRAINT arriendo_temprano_modalidad_check CHECK ((modalidad = 'hora'::text)),
    CONSTRAINT arriendo_temprano_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT arriendo_temprano_participantes_ck CHECK ((anfitrion_id <> arrendatario_id)),
    CONSTRAINT arriendo_temprano_precio_unitario_clp_min_1000_ck CHECK ((precio_unitario_clp >= 1000)),
    CONSTRAINT arriendo_temprano_revision_check CHECK ((revision >= 1)),
    CONSTRAINT arriendo_temprano_titulo_snapshot_check CHECK (((length(btrim(titulo_snapshot)) >= 1) AND (length(btrim(titulo_snapshot)) <= 70))),
    CONSTRAINT arriendo_temprano_unidades_check CHECK ((unidades = ANY (ARRAY[(1)::bigint, (2)::bigint, (4)::bigint]))),
    CONSTRAINT arriendo_temprano_vencimiento_ck CHECK ((isfinite(creada_en) AND isfinite(actualizada_en) AND isfinite(vence_en) AND (vence_en > creada_en) AND (vence_en <= inicio) AND (vence_en <= (creada_en + '00:30:00'::interval)) AND (actualizada_en >= creada_en))),
    CONSTRAINT arriendo_temprano_zona_horaria_check CHECK (((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100) AND (zona_horaria <> 'Local'::text)))
);


--
-- Name: arriendo_temprano_confirmacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arriendo_temprano_confirmacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    arriendo_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    clase text NOT NULL,
    contrato_version bigint,
    contrato_digest bytea,
    confirmada_en timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT arriendo_temprano_confirmacion_check CHECK ((((clase = 'identidad'::text) AND (contrato_version IS NULL) AND (contrato_digest IS NULL)) OR ((clase = 'contrato'::text) AND (contrato_version IS NOT NULL) AND (contrato_digest IS NOT NULL)))),
    CONSTRAINT arriendo_temprano_confirmacion_clase_check CHECK ((clase = ANY (ARRAY['identidad'::text, 'contrato'::text]))),
    CONSTRAINT arriendo_temprano_confirmacion_confirmada_en_check CHECK (isfinite(confirmada_en)),
    CONSTRAINT arriendo_temprano_confirmacion_contrato_digest_check CHECK ((octet_length(contrato_digest) = 32)),
    CONSTRAINT arriendo_temprano_confirmacion_contrato_version_check CHECK ((contrato_version > 0))
);


--
-- Name: arriendo_temprano_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arriendo_temprano_evento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    arriendo_id uuid NOT NULL,
    secuencia bigint NOT NULL,
    revision bigint NOT NULL,
    tipo text NOT NULL,
    actor_id uuid,
    estado_anterior text,
    estado_nuevo text NOT NULL,
    ocurrido_en timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    confirmacion_id uuid,
    pago_id uuid,
    motivo_codigo text,
    procesador_referencia text,
    CONSTRAINT arriendo_temprano_evento_actor_ck CHECK ((((procesador_referencia IS NULL) AND ((tipo = 'oferta_vencida'::text) = (actor_id IS NULL))) OR ((procesador_referencia IS NOT NULL) AND (tipo = 'pago_marcado_recibido'::text) AND (actor_id IS NULL) AND (pago_id IS NOT NULL) AND (procesador_referencia = (('localfake:'::text || (pago_id)::text) || ':exito_simulado'::text))))),
    CONSTRAINT arriendo_temprano_evento_check CHECK ((secuencia = revision)),
    CONSTRAINT arriendo_temprano_evento_check1 CHECK ((((tipo = 'oferta_creada'::text) AND (secuencia = 1) AND (estado_anterior IS NULL) AND (estado_nuevo = 'pendiente_identidad'::text)) OR ((tipo <> 'oferta_creada'::text) AND (secuencia > 1) AND (estado_anterior IS NOT NULL)))),
    CONSTRAINT arriendo_temprano_evento_estado_anterior_check CHECK ((estado_anterior = ANY (ARRAY['pendiente_identidad'::text, 'pendiente_contrato'::text, 'pendiente_envio_pago'::text, 'pendiente_recepcion_pago'::text, 'confirmado'::text, 'rechazado'::text, 'cancelado'::text, 'vencido'::text]))),
    CONSTRAINT arriendo_temprano_evento_estado_nuevo_check CHECK ((estado_nuevo = ANY (ARRAY['pendiente_identidad'::text, 'pendiente_contrato'::text, 'pendiente_envio_pago'::text, 'pendiente_recepcion_pago'::text, 'confirmado'::text, 'rechazado'::text, 'cancelado'::text, 'vencido'::text]))),
    CONSTRAINT arriendo_temprano_evento_motivo_ck CHECK ((((tipo = ANY (ARRAY['oferta_rechazada'::text, 'oferta_cancelada'::text])) AND (motivo_codigo IS NOT NULL) AND (motivo_codigo = ANY (ARRAY['condiciones_no_aceptadas'::text, 'cambio_de_planes'::text, 'error_en_la_oferta'::text, 'otro'::text]))) OR ((tipo <> ALL (ARRAY['oferta_rechazada'::text, 'oferta_cancelada'::text])) AND (motivo_codigo IS NULL)))),
    CONSTRAINT arriendo_temprano_evento_ocurrido_en_check CHECK (isfinite(ocurrido_en)),
    CONSTRAINT arriendo_temprano_evento_revision_check CHECK ((revision > 0)),
    CONSTRAINT arriendo_temprano_evento_secuencia_check CHECK ((secuencia > 0)),
    CONSTRAINT arriendo_temprano_evento_tipo_check CHECK ((tipo = ANY (ARRAY['oferta_creada'::text, 'identidad_simulada_confirmada'::text, 'contrato_aceptado'::text, 'pago_marcado_enviado'::text, 'pago_marcado_recibido'::text, 'oferta_rechazada'::text, 'oferta_cancelada'::text, 'oferta_vencida'::text])))
);


--
-- Name: arriendo_temprano_idempotencia; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arriendo_temprano_idempotencia (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    arriendo_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    comando text NOT NULL,
    clave text NOT NULL,
    huella bytea NOT NULL,
    evento_id uuid NOT NULL,
    creado_en timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT arriendo_temprano_idempotencia_clave_check CHECK (((char_length(clave) >= 8) AND (char_length(clave) <= 200))),
    CONSTRAINT arriendo_temprano_idempotencia_comando_check CHECK ((comando = ANY (ARRAY['crear_oferta'::text, 'confirmar_identidad_simulada'::text, 'aceptar_contrato'::text, 'marcar_pago_enviado'::text, 'marcar_pago_recibido'::text, 'rechazar'::text, 'cancelar'::text]))),
    CONSTRAINT arriendo_temprano_idempotencia_creado_en_check CHECK (isfinite(creado_en)),
    CONSTRAINT arriendo_temprano_idempotencia_huella_check CHECK ((octet_length(huella) = 32))
);


--
-- Name: arriendo_temprano_pago; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arriendo_temprano_pago (
    arriendo_id uuid NOT NULL,
    pago_id uuid DEFAULT gen_random_uuid() NOT NULL,
    emisor_id uuid NOT NULL,
    receptor_id uuid NOT NULL,
    monto_clp bigint NOT NULL,
    moneda text NOT NULL,
    estado text NOT NULL,
    enviado_en timestamp with time zone,
    recibido_en timestamp with time zone,
    creado_en timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT arriendo_temprano_pago_check CHECK ((emisor_id <> receptor_id)),
    CONSTRAINT arriendo_temprano_pago_check1 CHECK ((((estado = 'pendiente'::text) AND (enviado_en IS NULL) AND (recibido_en IS NULL)) OR ((estado = 'enviado'::text) AND (enviado_en IS NOT NULL) AND (recibido_en IS NULL)) OR ((estado = 'recibido'::text) AND (enviado_en IS NOT NULL) AND (recibido_en IS NOT NULL) AND (recibido_en >= enviado_en)) OR ((estado = ANY (ARRAY['anulado'::text, 'vencido'::text])) AND (recibido_en IS NULL)))),
    CONSTRAINT arriendo_temprano_pago_check2 CHECK ((isfinite(creado_en) AND ((enviado_en IS NULL) OR isfinite(enviado_en)) AND ((recibido_en IS NULL) OR isfinite(recibido_en)))),
    CONSTRAINT arriendo_temprano_pago_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'enviado'::text, 'recibido'::text, 'anulado'::text, 'vencido'::text]))),
    CONSTRAINT arriendo_temprano_pago_moneda_check CHECK ((moneda = 'CLP'::text)),
    CONSTRAINT arriendo_temprano_pago_monto_clp_check CHECK ((monto_clp > 0))
);


--
-- Name: atributo_filtro_publico; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.atributo_filtro_publico (
    categoria_codigo text NOT NULL,
    perfil_version integer NOT NULL,
    codigo text NOT NULL,
    etiqueta text NOT NULL,
    tipo text NOT NULL,
    orden smallint NOT NULL,
    CONSTRAINT atributo_filtro_publico_codigo_check CHECK ((codigo ~ '^[a-z][a-z0-9_]{0,59}$'::text)),
    CONSTRAINT atributo_filtro_publico_etiqueta_check CHECK (((length(btrim(etiqueta)) >= 1) AND (length(btrim(etiqueta)) <= 80))),
    CONSTRAINT atributo_filtro_publico_orden_check CHECK ((orden > 0)),
    CONSTRAINT atributo_filtro_publico_perfil_version_check CHECK ((perfil_version > 0)),
    CONSTRAINT atributo_filtro_publico_tipo_check CHECK ((tipo = ANY (ARRAY['boolean'::text, 'integer'::text, 'number'::text, 'enum'::text, 'enum_list'::text])))
);


--
-- Name: autorizacion_arriendo_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.autorizacion_arriendo_comercial (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    plantilla_version bigint NOT NULL,
    monto_maximo_clp bigint NOT NULL,
    vigente_desde timestamp with time zone NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    revocada_por uuid,
    motivo_revocacion text,
    CONSTRAINT autorizacion_arriendo_comercial_check CHECK ((isfinite(vigente_desde) AND isfinite(vence_en) AND isfinite(creada_en))),
    CONSTRAINT autorizacion_arriendo_comercial_check1 CHECK (((vence_en > vigente_desde) AND ((vence_en - vigente_desde) <= '90 days'::interval))),
    CONSTRAINT autorizacion_arriendo_comercial_check2 CHECK ((((revocada_en IS NULL) AND (revocada_por IS NULL) AND (motivo_revocacion IS NULL)) OR ((revocada_en IS NOT NULL) AND (revocada_por = propietario_id) AND (motivo_revocacion IS NOT NULL) AND (motivo_revocacion ~ '^[a-z][a-z0-9_]{2,63}$'::text)))),
    CONSTRAINT autorizacion_arriendo_comercial_monto_maximo_clp_check CHECK (((monto_maximo_clp >= 1) AND (monto_maximo_clp <= 100000000))),
    CONSTRAINT autorizacion_arriendo_comercial_plantilla_version_check CHECK ((plantilla_version > 0))
);


--
-- Name: autorizacion_arriendo_comercial_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.autorizacion_arriendo_comercial_evento (
    id uuid NOT NULL,
    autorizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    accion text NOT NULL,
    motivo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT autorizacion_arriendo_comercial_evento_accion_check CHECK ((accion = ANY (ARRAY['activated'::text, 'revoked'::text, 'auto_confirmed'::text]))),
    CONSTRAINT autorizacion_arriendo_comercial_evento_motivo_check CHECK ((motivo ~ '^[a-z][a-z0-9_]{2,63}$'::text)),
    CONSTRAINT autorizacion_arriendo_comercial_evento_ocurrida_en_check CHECK (isfinite(ocurrida_en))
);


--
-- Name: autorizacion_firma_desafio_otp; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.autorizacion_firma_desafio_otp (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    plantilla_version bigint NOT NULL,
    canal text DEFAULT 'email_otp'::text NOT NULL,
    codigo_hmac bytea NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    verificado_en timestamp with time zone,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT autorizacion_firma_desafio_otp_canal_check CHECK ((canal = 'email_otp'::text)),
    CONSTRAINT autorizacion_firma_desafio_otp_codigo_hmac_check CHECK ((octet_length(codigo_hmac) = 32)),
    CONSTRAINT autorizacion_firma_desafio_otp_creada_en_check CHECK (isfinite(creada_en)),
    CONSTRAINT autorizacion_firma_desafio_otp_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT autorizacion_firma_desafio_otp_intentos_check CHECK (((intentos >= 0) AND (intentos <= 5))),
    CONSTRAINT autorizacion_firma_desafio_otp_plantilla_version_check CHECK ((plantilla_version > 0))
);


--
-- Name: aviso_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aviso_local (
    id uuid NOT NULL,
    tipo_evento text NOT NULL,
    agregado_tipo text NOT NULL,
    agregado_id uuid NOT NULL,
    destinatario_id uuid NOT NULL,
    clave_deduplicacion text NOT NULL,
    estado text DEFAULT 'pendiente'::text NOT NULL,
    ciclo integer DEFAULT 1 NOT NULL,
    intentos_total integer DEFAULT 0 NOT NULL,
    intentos_ciclo integer DEFAULT 0 NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    proximo_intento_en timestamp with time zone NOT NULL,
    lease_hasta timestamp with time zone,
    entregada_en timestamp with time zone,
    fallo_terminal_en timestamp with time zone,
    cancelada_en timestamp with time zone,
    codigo_error text,
    retirar_en timestamp with time zone,
    CONSTRAINT aviso_local_agregado_tipo_check CHECK ((agregado_tipo = ANY (ARRAY['reserva'::text, 'reporte_resena'::text, 'usuario'::text, 'verificacion'::text, 'contrato'::text]))),
    CONSTRAINT aviso_local_check CHECK ((((estado = 'entregada'::text) AND (entregada_en IS NOT NULL) AND (retirar_en = (entregada_en + '30 days'::interval))) OR ((estado = 'fallo_terminal'::text) AND (fallo_terminal_en IS NOT NULL) AND (retirar_en = (fallo_terminal_en + '30 days'::interval))) OR ((estado = 'cancelada'::text) AND (cancelada_en IS NOT NULL) AND (retirar_en = (cancelada_en + '30 days'::interval))) OR ((estado = ANY (ARRAY['pendiente'::text, 'procesando'::text])) AND (retirar_en IS NULL)))),
    CONSTRAINT aviso_local_ciclo_check CHECK ((ciclo > 0)),
    CONSTRAINT aviso_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'procesando'::text, 'entregada'::text, 'fallo_terminal'::text, 'cancelada'::text]))),
    CONSTRAINT aviso_local_intentos_ciclo_check CHECK (((intentos_ciclo >= 0) AND (intentos_ciclo <= 8))),
    CONSTRAINT aviso_local_intentos_total_check CHECK ((intentos_total >= 0)),
    CONSTRAINT aviso_local_tipo_evento_check CHECK ((tipo_evento = ANY (ARRAY['checkin_registrado'::text, 'resena_reportada'::text, 'reclamo_abierto'::text, 'reclamo_resuelto'::text, 'reserva_cancelada'::text, 'cuenta_bloqueada_login'::text, 'verificacion_aprobada'::text, 'verificacion_rechazada'::text, 'pago_arriendo_confirmado'::text, 'pago_arriendo_rechazado'::text, 'nueva_reserva_pagada'::text, 'invitacion_firma'::text, 'contrato_firmado'::text])))
);


--
-- Name: TABLE aviso_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.aviso_local IS 'Durable local Mailpit notice intent. SMTP may duplicate after uncertain acceptance; no exactly-once guarantee.';


--
-- Name: aviso_local_ciclo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aviso_local_ciclo (
    aviso_id uuid NOT NULL,
    ciclo integer NOT NULL,
    estado text NOT NULL,
    iniciada_en timestamp with time zone NOT NULL,
    finalizada_en timestamp with time zone,
    intentos integer DEFAULT 0 NOT NULL,
    codigo_resultado text,
    actor_reapertura_id uuid,
    motivo_reapertura_codigo text,
    correlacion_id text,
    clave_idempotencia text,
    CONSTRAINT aviso_local_ciclo_check1 CHECK (((estado = 'pendiente'::text) = (finalizada_en IS NULL))),
    CONSTRAINT aviso_local_ciclo_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'entregada'::text, 'fallo_terminal'::text, 'cancelada'::text]))),
    CONSTRAINT aviso_local_ciclo_intentos_check CHECK (((intentos >= 0) AND (intentos <= 8))),
    CONSTRAINT aviso_local_ciclo_motivo_reapertura_codigo_check CHECK ((motivo_reapertura_codigo = ANY (ARRAY['smtp_restaurado'::text, 'reintento_operativo'::text])))
);


--
-- Name: aviso_local_recuperacion_auditoria; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aviso_local_recuperacion_auditoria (
    id uuid NOT NULL,
    aviso_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    motivo_codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    clave_idempotencia text NOT NULL,
    retirar_en timestamp with time zone NOT NULL,
    CONSTRAINT aviso_local_recuperacion_auditoria_check CHECK ((retirar_en = (ocurrida_en + '5 years'::interval))),
    CONSTRAINT aviso_local_recuperacion_auditoria_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['smtp_restaurado'::text, 'reintento_operativo'::text])))
);


--
-- Name: baja_archivo_pendiente_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.baja_archivo_pendiente_local (
    ejecucion_id uuid NOT NULL,
    evidencia_id uuid NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    disponible_en timestamp with time zone NOT NULL,
    ultimo_codigo_error text,
    completada_en timestamp with time zone,
    claim_owner uuid,
    claim_hasta timestamp with time zone,
    claim_generacion bigint DEFAULT 0 NOT NULL,
    CONSTRAINT baja_archivo_claim_generacion_ck CHECK ((claim_generacion >= 0)),
    CONSTRAINT baja_archivo_claim_lease_pair_ck CHECK (((claim_owner IS NULL) = (claim_hasta IS NULL))),
    CONSTRAINT baja_archivo_pendiente_local_intentos_check CHECK ((intentos >= 0))
);


--
-- Name: COLUMN baja_archivo_pendiente_local.claim_owner; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.baja_archivo_pendiente_local.claim_owner IS 'Unique worker-process UUID for the current privacy evidence deletion lease.';


--
-- Name: COLUMN baja_archivo_pendiente_local.claim_hasta; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.baja_archivo_pendiente_local.claim_hasta IS 'Lease deadline; expired claims may be atomically taken over.';


--
-- Name: COLUMN baja_archivo_pendiente_local.claim_generacion; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.baja_archivo_pendiente_local.claim_generacion IS 'Monotonic per-row fencing generation; stale workers cannot acknowledge a takeover.';


--
-- Name: bloqueo_cuenta_administrativo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bloqueo_cuenta_administrativo_local (
    cuenta_id uuid NOT NULL,
    bloqueada_por uuid,
    motivo_codigo text NOT NULL,
    bloqueada_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    CONSTRAINT bloqueo_cuenta_administrativo_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT bloqueo_cuenta_administrativo_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['riesgo_seguridad'::text, 'uso_indebido'::text, 'revision_administrativa'::text])))
);


--
-- Name: TABLE bloqueo_cuenta_administrativo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.bloqueo_cuenta_administrativo_local IS 'Bloqueo sintético separado de usuario.estado, KYC y baja; conserva reservas y obligaciones.';


--
-- Name: COLUMN bloqueo_cuenta_administrativo_local.bloqueada_por; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bloqueo_cuenta_administrativo_local.bloqueada_por IS 'Nullable when the administrator actor is retired; the active block remains effective without retaining that personal link.';


--
-- Name: bloqueo_cuenta_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bloqueo_cuenta_historial_local (
    id uuid NOT NULL,
    cuenta_id uuid NOT NULL,
    actor_id uuid,
    accion text NOT NULL,
    motivo_codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    CONSTRAINT bloqueo_cuenta_historial_local_accion_check CHECK ((accion = ANY (ARRAY['bloquear'::text, 'desbloquear'::text]))),
    CONSTRAINT bloqueo_cuenta_historial_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT bloqueo_cuenta_historial_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['riesgo_seguridad'::text, 'uso_indebido'::text, 'revision_administrativa'::text, 'revision_concluida'::text, 'error_bloqueo'::text, 'riesgo_resuelto'::text])))
);


--
-- Name: TABLE bloqueo_cuenta_historial_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.bloqueo_cuenta_historial_local IS 'Historial append-only de gobierno local de cuentas; auditoría de cinco años también queda en evento_auditoria_local.';


--
-- Name: COLUMN bloqueo_cuenta_historial_local.actor_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bloqueo_cuenta_historial_local.actor_id IS 'Nullable when the actor account is retired; privacy execution may remove the personal link while preserving the structured event.';


--
-- Name: caso_soporte_reserva_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.caso_soporte_reserva_comercial (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reserva_id uuid NOT NULL,
    iniciador_id uuid NOT NULL,
    rol_iniciador text NOT NULL,
    categoria text NOT NULL,
    enrutado_a text NOT NULL,
    estado text NOT NULL,
    descripcion text NOT NULL,
    clave_idempotencia text NOT NULL,
    plazo_descargo timestamp with time zone,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT caso_soporte_reserva_comercial_categoria_check CHECK ((categoria = ANY (ARRAY['problemas_acceso'::text, 'condiciones_diferentes'::text, 'incumplimiento_normas'::text, 'espacio_no_disponible'::text, 'problema_pago_reembolso'::text, 'mediacion_directa'::text, 'dano_inmueble'::text]))),
    CONSTRAINT caso_soporte_reserva_comercial_check CHECK (((categoria = 'dano_inmueble'::text) = (plazo_descargo IS NOT NULL))),
    CONSTRAINT caso_soporte_reserva_comercial_check1 CHECK (((categoria <> 'dano_inmueble'::text) OR ((rol_iniciador = 'anfitrion'::text) AND (enrutado_a = 'espacigo'::text)))),
    CONSTRAINT caso_soporte_reserva_comercial_clave_idempotencia_check CHECK (((char_length(clave_idempotencia) >= 1) AND (char_length(clave_idempotencia) <= 128))),
    CONSTRAINT caso_soporte_reserva_comercial_descripcion_check CHECK (((char_length(descripcion) >= 1) AND (char_length(descripcion) <= 3000))),
    CONSTRAINT caso_soporte_reserva_comercial_enrutado_a_check CHECK ((enrutado_a = ANY (ARRAY['contraparte'::text, 'espacigo'::text]))),
    CONSTRAINT caso_soporte_reserva_comercial_estado_check CHECK ((estado = ANY (ARRAY['esperando_contraparte'::text, 'en_mediacion_espacigo'::text, 'resuelto'::text, 'cerrado'::text]))),
    CONSTRAINT caso_soporte_reserva_comercial_rol_iniciador_check CHECK ((rol_iniciador = ANY (ARRAY['anfitrion'::text, 'arrendatario'::text])))
);


--
-- Name: catalogo_ubicacion_publica; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalogo_ubicacion_publica (
    version text NOT NULL,
    fuente text NOT NULL,
    publicado_en date NOT NULL,
    activo boolean DEFAULT false NOT NULL,
    CONSTRAINT catalogo_ubicacion_publica_version_check CHECK ((version ~ '^[A-Z0-9_]{1,40}$'::text))
);


--
-- Name: categoria_espacio; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.categoria_espacio (
    codigo text NOT NULL,
    nombre text NOT NULL,
    orden smallint NOT NULL,
    activa boolean DEFAULT true NOT NULL,
    CONSTRAINT categoria_espacio_orden_check CHECK ((orden > 0))
);


--
-- Name: categoria_perfil_atributos; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.categoria_perfil_atributos (
    categoria_codigo text NOT NULL,
    version integer NOT NULL,
    perfil jsonb NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT categoria_perfil_atributos_perfil_check CHECK ((jsonb_typeof(perfil) = 'object'::text)),
    CONSTRAINT categoria_perfil_atributos_version_check CHECK ((version > 0))
);


--
-- Name: checkout_cancelacion_pendiente; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checkout_cancelacion_pendiente (
    operacion_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    seller_id text NOT NULL,
    provider_payment_id text,
    clave_idempotencia uuid NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    ultimo_intento_en timestamp with time zone,
    resultado text,
    CONSTRAINT checkout_cancelacion_pendiente_provider_payment_id_check CHECK ((provider_payment_id ~ '^[0-9]{1,64}$'::text)),
    CONSTRAINT checkout_cancelacion_pendiente_resultado_check CHECK ((resultado = ANY (ARRAY['approved'::text, 'cancelled'::text]))),
    CONSTRAINT checkout_cancelacion_pendiente_seller_id_check CHECK ((seller_id ~ '^[0-9]{1,64}$'::text))
);


--
-- Name: TABLE checkout_cancelacion_pendiente; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.checkout_cancelacion_pendiente IS 'Durable Checkout Pro cancellation intent. No provider mutation without a bound payment ID.';


--
-- Name: checkout_payment_notice_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checkout_payment_notice_outbox (
    id uuid NOT NULL,
    reservation_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    recipient_id uuid NOT NULL,
    recipient_role text NOT NULL,
    transition text NOT NULL,
    dedupe_key text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    available_at timestamp with time zone NOT NULL,
    claimed_at timestamp with time zone,
    delivered_at timestamp with time zone,
    terminal_at timestamp with time zone,
    last_error_code text,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT checkout_payment_notice_attempts_ck CHECK (((attempts >= 0) AND (attempts <= 8))),
    CONSTRAINT checkout_payment_notice_dedupe_ck CHECK (((char_length(dedupe_key) >= 1) AND (char_length(dedupe_key) <= 200))),
    CONSTRAINT checkout_payment_notice_error_ck CHECK (((last_error_code IS NULL) OR (last_error_code = ANY (ARRAY['mail_delivery_failed'::text, 'recipient_unavailable'::text, 'attempt_limit_reached'::text])))),
    CONSTRAINT checkout_payment_notice_event_role_ck CHECK ((((transition = 'approved'::text) AND (recipient_role = ANY (ARRAY['arrendatario'::text, 'anfitrion'::text]))) OR ((transition = ANY (ARRAY['rejected'::text, 'expired'::text])) AND (recipient_role = 'arrendatario'::text)))),
    CONSTRAINT checkout_payment_notice_role_ck CHECK ((recipient_role = ANY (ARRAY['arrendatario'::text, 'anfitrion'::text]))),
    CONSTRAINT checkout_payment_notice_state_ck CHECK ((state = ANY (ARRAY['pending'::text, 'processing'::text, 'delivered'::text, 'terminal'::text, 'cancelled'::text]))),
    CONSTRAINT checkout_payment_notice_state_time_ck CHECK ((((state = 'delivered'::text) AND (delivered_at IS NOT NULL) AND (terminal_at IS NULL)) OR ((state = 'terminal'::text) AND (terminal_at IS NOT NULL) AND (delivered_at IS NULL)) OR ((state = ANY (ARRAY['pending'::text, 'processing'::text, 'cancelled'::text])) AND (delivered_at IS NULL) AND (terminal_at IS NULL)))),
    CONSTRAINT checkout_payment_notice_timestamps_ck CHECK (((created_at <= available_at) AND ((claimed_at IS NULL) OR (claimed_at >= created_at)) AND ((delivered_at IS NULL) OR (delivered_at >= created_at)) AND ((terminal_at IS NULL) OR (terminal_at >= created_at)))),
    CONSTRAINT checkout_payment_notice_transition_ck CHECK ((transition = ANY (ARRAY['approved'::text, 'rejected'::text, 'expired'::text])))
);


--
-- Name: TABLE checkout_payment_notice_outbox; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.checkout_payment_notice_outbox IS 'Transactional email intent created only for backend-verified commercial checkout transitions; recipient email is resolved at delivery time.';


--
-- Name: checkout_preferencia_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checkout_preferencia_comercial (
    id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    host_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    fingerprint bytea NOT NULL,
    estado text DEFAULT 'pending'::text NOT NULL,
    base_clp bigint NOT NULL,
    commission_clp bigint NOT NULL,
    iva_clp bigint NOT NULL,
    marketplace_fee_clp bigint NOT NULL,
    test_mode boolean NOT NULL,
    preference_id text,
    init_point text,
    preference_claimed_at timestamp with time zone,
    vence_en timestamp with time zone NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    provider_payment_id text,
    provider_payment_status text,
    provider_payment_updated_at timestamp with time zone,
    reconciliation_reason text,
    payment_verified_at timestamp with time zone,
    checkout_method text DEFAULT 'checkout_pro'::text NOT NULL,
    payment_create_claimed_at timestamp with time zone,
    buyer_commission_clp bigint DEFAULT 0 NOT NULL,
    buyer_iva_clp bigint DEFAULT 0 NOT NULL,
    buyer_fee_clp bigint DEFAULT 0 NOT NULL,
    total_charged_clp bigint DEFAULT 0 NOT NULL,
    solicitud_comercial_id uuid,
    contrato_version bigint,
    contrato_sha256 bytea,
    provider_settlement jsonb,
    provider_settlement_attempted_at timestamp with time zone,
    CONSTRAINT checkout_contract_binding_ck CHECK ((((solicitud_comercial_id IS NULL) AND (contrato_version IS NULL) AND (contrato_sha256 IS NULL)) OR ((solicitud_comercial_id IS NOT NULL) AND (contrato_version IS NOT NULL) AND (contrato_sha256 IS NOT NULL) AND (contrato_version > 0) AND (octet_length(contrato_sha256) = 32)))),
    CONSTRAINT checkout_preferencia_comercial_amounts_ck CHECK (((base_clp > 0) AND (commission_clp >= 0) AND (iva_clp >= 0) AND (buyer_commission_clp >= 0) AND (buyer_iva_clp >= 0) AND (buyer_fee_clp = (buyer_commission_clp + buyer_iva_clp)) AND (total_charged_clp = (base_clp + buyer_fee_clp)) AND (marketplace_fee_clp = ((commission_clp + iva_clp) + buyer_fee_clp)) AND (marketplace_fee_clp < total_charged_clp))),
    CONSTRAINT checkout_preferencia_comercial_dates_ck CHECK ((isfinite(vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en) AND (vence_en > creada_en) AND (actualizada_en >= creada_en))),
    CONSTRAINT checkout_preferencia_comercial_estado_ck CHECK ((estado = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text, 'cancelled'::text, 'reconciliation_required'::text]))),
    CONSTRAINT checkout_preferencia_comercial_fingerprint_ck CHECK ((octet_length(fingerprint) = 32)),
    CONSTRAINT checkout_preferencia_comercial_key_ck CHECK (((char_length(idempotency_key) >= 1) AND (char_length(idempotency_key) <= 200))),
    CONSTRAINT checkout_preferencia_comercial_method_ck CHECK ((checkout_method = ANY (ARRAY['checkout_pro'::text, 'payments_api'::text]))),
    CONSTRAINT checkout_preferencia_comercial_payment_claim_ck CHECK (((payment_create_claimed_at IS NULL) OR ((checkout_method = 'payments_api'::text) AND isfinite(payment_create_claimed_at)))),
    CONSTRAINT checkout_preferencia_comercial_payment_fields_ck CHECK ((((provider_payment_id IS NULL) AND (payment_verified_at IS NULL)) OR ((provider_payment_id IS NOT NULL) AND (provider_payment_status IS NOT NULL) AND (provider_payment_updated_at IS NOT NULL)))),
    CONSTRAINT checkout_preferencia_comercial_payment_id_ck CHECK (((provider_payment_id IS NULL) OR (provider_payment_id ~ '^[0-9]{1,64}$'::text))),
    CONSTRAINT checkout_preferencia_comercial_payment_method_mode_ck CHECK (((checkout_method <> 'payments_api'::text) OR test_mode)),
    CONSTRAINT checkout_preferencia_comercial_payment_no_preference_ck CHECK (((checkout_method <> 'payments_api'::text) OR ((preference_id IS NULL) AND (init_point IS NULL) AND (preference_claimed_at IS NULL)))),
    CONSTRAINT checkout_preferencia_comercial_payment_status_ck CHECK (((provider_payment_status IS NULL) OR (provider_payment_status ~ '^[a-z_]{1,40}$'::text))),
    CONSTRAINT checkout_preferencia_comercial_provider_ck CHECK ((((preference_id IS NULL) AND (init_point IS NULL)) OR ((preference_id IS NOT NULL) AND (init_point IS NOT NULL)))),
    CONSTRAINT checkout_preferencia_comercial_provider_settlement_check CHECK ((jsonb_typeof(provider_settlement) = 'object'::text)),
    CONSTRAINT checkout_preferencia_comercial_reconciliation_reason_ck CHECK (((reconciliation_reason IS NULL) OR (reconciliation_reason ~ '^[a-z_]{1,64}$'::text)))
);


--
-- Name: TABLE checkout_preferencia_comercial; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.checkout_preferencia_comercial IS 'Durable Mercado Pago Checkout Pro preference, immutable price reference and pending interval hold for commercial quotes.';


--
-- Name: COLUMN checkout_preferencia_comercial.checkout_method; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.checkout_preferencia_comercial.checkout_method IS 'Durable provider path; existing rows default to checkout_pro and Payments API is test-mode-only.';


--
-- Name: COLUMN checkout_preferencia_comercial.payment_create_claimed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.checkout_preferencia_comercial.payment_create_claimed_at IS 'One-shot durable claim before Payments API creation; never reset after an uncertain provider outcome.';


--
-- Name: COLUMN checkout_preferencia_comercial.solicitud_comercial_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.checkout_preferencia_comercial.solicitud_comercial_id IS 'Durable signed agreement source; required by application when commercial contracts gate is enabled, immutable once created.';


--
-- Name: comuna_publica; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comuna_publica (
    catalogo_version text NOT NULL,
    codigo text NOT NULL,
    region_codigo text NOT NULL,
    nombre text NOT NULL,
    CONSTRAINT comuna_publica_codigo_check CHECK ((codigo ~ '^[0-9]{5}$'::text)),
    CONSTRAINT comuna_publica_nombre_check CHECK (((length(btrim(nombre)) >= 1) AND (length(btrim(nombre)) <= 100)))
);


--
-- Name: contrato_documento_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_documento_comercial (
    id uuid NOT NULL,
    operacion_id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    propuesta_id uuid NOT NULL,
    version bigint NOT NULL,
    documento_sha256 bytea NOT NULL,
    object_id uuid DEFAULT gen_random_uuid() NOT NULL,
    object_key text GENERATED ALWAYS AS ((('contracts/'::text || (object_id)::text) || '.blob'::text)) STORED,
    estado text DEFAULT 'pending'::text NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    lease_token uuid,
    lease_vence_en timestamp with time zone,
    disponible_en timestamp with time zone NOT NULL,
    ultimo_error text,
    bucket_name text,
    object_version text,
    pdf_sha256 bytea,
    pdf_bytes bigint,
    lista_en timestamp with time zone,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    evidencia_object_key text,
    evidencia_sha256 bytea,
    sello_cert_fingerprint text,
    CONSTRAINT contrato_documento_comercial_check CHECK ((((estado = 'processing'::text) AND (lease_token IS NOT NULL) AND (lease_vence_en IS NOT NULL)) OR ((estado <> 'processing'::text) AND (lease_token IS NULL) AND (lease_vence_en IS NULL)))),
    CONSTRAINT contrato_documento_comercial_check1 CHECK ((((estado = 'ready'::text) AND (bucket_name IS NOT NULL) AND (pdf_sha256 IS NOT NULL) AND (pdf_bytes IS NOT NULL) AND (lista_en IS NOT NULL)) OR ((estado <> 'ready'::text) AND (bucket_name IS NULL) AND (pdf_sha256 IS NULL) AND (pdf_bytes IS NULL) AND (lista_en IS NULL)))),
    CONSTRAINT contrato_documento_comercial_check2 CHECK ((isfinite(disponible_en) AND isfinite(creada_en) AND isfinite(actualizada_en))),
    CONSTRAINT contrato_documento_comercial_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_documento_comercial_estado_check CHECK ((estado = ANY (ARRAY['pending'::text, 'processing'::text, 'retry'::text, 'ready'::text, 'terminal'::text]))),
    CONSTRAINT contrato_documento_comercial_evidencia_sha256_check CHECK (((evidencia_sha256 IS NULL) OR (octet_length(evidencia_sha256) = 32))),
    CONSTRAINT contrato_documento_comercial_intentos_check CHECK (((intentos >= 0) AND (intentos <= 8))),
    CONSTRAINT contrato_documento_comercial_lease_vence_en_check CHECK (((lease_vence_en IS NULL) OR isfinite(lease_vence_en))),
    CONSTRAINT contrato_documento_comercial_pdf_bytes_check CHECK (((pdf_bytes IS NULL) OR ((pdf_bytes >= 1) AND (pdf_bytes <= 16777216)))),
    CONSTRAINT contrato_documento_comercial_pdf_sha256_check CHECK (((pdf_sha256 IS NULL) OR (octet_length(pdf_sha256) = 32))),
    CONSTRAINT contrato_documento_comercial_ultimo_error_check CHECK (((ultimo_error IS NULL) OR (ultimo_error = ANY (ARRAY['evidence_unavailable'::text, 'render_failed'::text, 'unsupported_glyph'::text, 'storage_failed'::text, 'object_conflict'::text, 'attempt_limit_reached'::text])))),
    CONSTRAINT contrato_documento_comercial_version_check CHECK ((version > 0))
);


--
-- Name: contrato_ensayo_firma; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_ensayo_firma (
    contrato_id uuid NOT NULL,
    firmante_id uuid NOT NULL,
    rol text NOT NULL,
    estado text NOT NULL,
    motivo text DEFAULT ''::text NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT contrato_ensayo_firma_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'firmada'::text, 'rechazada'::text]))),
    CONSTRAINT contrato_ensayo_firma_rol_check CHECK ((rol = ANY (ARRAY['anfitrion'::text, 'arrendatario'::text])))
);


--
-- Name: contrato_ensayo_historial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_ensayo_historial (
    id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    secuencia bigint NOT NULL,
    actor_id uuid,
    accion text NOT NULL,
    motivo text DEFAULT ''::text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT contrato_ensayo_historial_accion_check CHECK ((accion = ANY (ARRAY['generado'::text, 'firmado'::text, 'rechazado'::text, 'firmado_completo'::text, 'vencido'::text, 'reserva_terminal'::text, 'reserva_cancelada'::text]))),
    CONSTRAINT contrato_ensayo_historial_secuencia_check CHECK ((secuencia > 0))
);


--
-- Name: contrato_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    version integer NOT NULL,
    estado text NOT NULL,
    snapshot jsonb NOT NULL,
    documento_id uuid,
    sha256 bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    firmado_en timestamp with time zone,
    pdf_retirado_en timestamp with time zone,
    CONSTRAINT contrato_documento_retirado_ck CHECK ((((documento_id IS NOT NULL) AND (pdf_retirado_en IS NULL)) OR ((documento_id IS NULL) AND (pdf_retirado_en IS NOT NULL)))),
    CONSTRAINT contrato_ensayo_local_check CHECK (((estado = 'firmado'::text) = (firmado_en IS NOT NULL))),
    CONSTRAINT contrato_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['generado'::text, 'firma_parcial'::text, 'firmado'::text, 'anulado'::text]))),
    CONSTRAINT contrato_ensayo_local_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT contrato_ensayo_local_version_check CHECK ((version > 0))
);


--
-- Name: contrato_firma_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_firma_comercial (
    id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    sesion_id uuid NOT NULL,
    rol text NOT NULL,
    propuesta_version bigint NOT NULL,
    documento_sha256 bytea NOT NULL,
    consentimiento boolean NOT NULL,
    consentimiento_version text NOT NULL,
    firmada_en timestamp with time zone NOT NULL,
    tipo_firma text DEFAULT 'FES'::text NOT NULL,
    metodo_autenticacion text DEFAULT 'session_consent'::text NOT NULL,
    desafio_otp_id uuid,
    ip_masked text,
    user_agent_hash bytea,
    evidencia_sha256 bytea,
    proveedor_ref text,
    CONSTRAINT contrato_firma_comercial_consentimiento_check CHECK (consentimiento),
    CONSTRAINT contrato_firma_comercial_consentimiento_version_check CHECK ((consentimiento_version = 'commercial-agreement-v1'::text)),
    CONSTRAINT contrato_firma_comercial_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_firma_comercial_evidencia_sha256_check CHECK (((evidencia_sha256 IS NULL) OR (octet_length(evidencia_sha256) = 32))),
    CONSTRAINT contrato_firma_comercial_firmada_en_check CHECK (isfinite(firmada_en)),
    CONSTRAINT contrato_firma_comercial_metodo_autenticacion_check CHECK ((metodo_autenticacion = ANY (ARRAY['session_consent'::text, 'email_otp'::text, 'totp'::text, 'external_provider'::text, 'auto_authorization'::text]))),
    CONSTRAINT contrato_firma_comercial_propuesta_version_check CHECK ((propuesta_version > 0)),
    CONSTRAINT contrato_firma_comercial_rol_check CHECK ((rol = ANY (ARRAY['arrendatario'::text, 'arrendador'::text]))),
    CONSTRAINT contrato_firma_comercial_tipo_firma_check CHECK ((tipo_firma = ANY (ARRAY['FES'::text, 'FEA'::text, 'NOTARIAL'::text]))),
    CONSTRAINT contrato_firma_comercial_user_agent_hash_check CHECK (((user_agent_hash IS NULL) OR (octet_length(user_agent_hash) = 32)))
);


--
-- Name: contrato_firma_desafio_otp; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_firma_desafio_otp (
    id uuid NOT NULL,
    propuesta_id uuid NOT NULL,
    solicitud_id uuid,
    actor_id uuid NOT NULL,
    rol text NOT NULL,
    documento_sha256 bytea NOT NULL,
    canal text NOT NULL,
    codigo_hmac bytea NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    verificado_en timestamp with time zone,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT contrato_firma_desafio_otp_canal_check CHECK ((canal = ANY (ARRAY['email_otp'::text, 'totp'::text]))),
    CONSTRAINT contrato_firma_desafio_otp_codigo_hmac_check CHECK ((octet_length(codigo_hmac) = 32)),
    CONSTRAINT contrato_firma_desafio_otp_creada_en_check CHECK (isfinite(creada_en)),
    CONSTRAINT contrato_firma_desafio_otp_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_firma_desafio_otp_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT contrato_firma_desafio_otp_intentos_check CHECK (((intentos >= 0) AND (intentos <= 5))),
    CONSTRAINT contrato_firma_desafio_otp_rol_check CHECK ((rol = ANY (ARRAY['arrendatario'::text, 'arrendador'::text])))
);


--
-- Name: TABLE contrato_firma_desafio_otp; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.contrato_firma_desafio_otp IS 'Desafios OTP de firma electronica simple (FES) ligados al actor, rol y SHA-256 del documento. Nunca almacena codigos en texto plano.';


--
-- Name: contrato_propuesta_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_propuesta_comercial (
    id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    version bigint NOT NULL,
    plantilla_version_id uuid NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    subtotal_clp bigint NOT NULL,
    moneda text NOT NULL,
    terminos jsonb NOT NULL,
    terminos_canonicos text NOT NULL,
    terminos_sha256 bytea NOT NULL,
    documento_sha256 bytea NOT NULL,
    identidades_cifradas bytea NOT NULL,
    clave_version text NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    es_b2b boolean DEFAULT false NOT NULL,
    organizacion_id uuid,
    empresa_razon_social_snapshot character varying(200),
    empresa_rut_masked_snapshot character varying(16),
    empresa_rut_verificada boolean DEFAULT false NOT NULL,
    representante_legal_id uuid,
    representante_nombre_snapshot character varying(200),
    representante_rut_masked_snapshot character varying(16),
    representante_rut_verificada boolean DEFAULT false CONSTRAINT contrato_propuesta_comercia_representante_rut_verifica_not_null NOT NULL,
    CONSTRAINT contrato_propuesta_b2b_snapshot_ck CHECK ((((NOT es_b2b) AND (organizacion_id IS NULL)) OR (es_b2b AND (organizacion_id IS NOT NULL) AND (empresa_razon_social_snapshot IS NOT NULL) AND (empresa_rut_masked_snapshot IS NOT NULL) AND empresa_rut_verificada AND (representante_legal_id IS NOT NULL) AND (representante_nombre_snapshot IS NOT NULL) AND (representante_rut_masked_snapshot IS NOT NULL) AND representante_rut_verificada))),
    CONSTRAINT contrato_propuesta_comercial_check CHECK ((termino > inicio)),
    CONSTRAINT contrato_propuesta_comercial_check1 CHECK ((propietario_id <> arrendatario_id)),
    CONSTRAINT contrato_propuesta_comercial_check10 CHECK (((((terminos ->> 'start_at'::text))::timestamp with time zone = inicio) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check11 CHECK (((((terminos ->> 'end_at'::text))::timestamp with time zone = termino) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check2 CHECK ((terminos = (terminos_canonicos)::jsonb)),
    CONSTRAINT contrato_propuesta_comercial_check3 CHECK ((terminos_sha256 = sha256(convert_to(terminos_canonicos, 'UTF8'::name)))),
    CONSTRAINT contrato_propuesta_comercial_check4 CHECK ((((terminos ->> 'space_id'::text) = (espacio_id)::text) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check5 CHECK ((((terminos ->> 'quote_id'::text) = (cotizacion_id)::text) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check6 CHECK ((((terminos ->> 'template_id'::text) = (plantilla_version_id)::text) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check7 CHECK (((((terminos ->> 'subtotal_clp'::text))::bigint = subtotal_clp) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check8 CHECK ((((terminos ->> 'currency'::text) = moneda) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_check9 CHECK ((((terminos ->> 'time_zone'::text) = zona_horaria) IS TRUE)),
    CONSTRAINT contrato_propuesta_comercial_clave_version_check CHECK (((length(clave_version) >= 1) AND (length(clave_version) <= 64))),
    CONSTRAINT contrato_propuesta_comercial_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_propuesta_comercial_identidades_cifradas_check CHECK (((octet_length(identidades_cifradas) >= 29) AND (octet_length(identidades_cifradas) <= 65564))),
    CONSTRAINT contrato_propuesta_comercial_moneda_check CHECK ((moneda = 'CLP'::text)),
    CONSTRAINT contrato_propuesta_comercial_subtotal_clp_check CHECK ((subtotal_clp > 0)),
    CONSTRAINT contrato_propuesta_comercial_terminos_check CHECK ((jsonb_typeof(terminos) = 'object'::text)),
    CONSTRAINT contrato_propuesta_comercial_terminos_check1 CHECK ((terminos ?& ARRAY['quote_id'::text, 'space_id'::text, 'space_title'::text, 'template_id'::text, 'template_version'::text, 'location'::text, 'sections'::text, 'rate_unit'::text, 'rate_version'::text, 'unit_price_clp'::text, 'units'::text, 'subtotal_clp'::text, 'currency'::text, 'start_at'::text, 'end_at'::text, 'time_zone'::text, 'cancellation_policy'::text])),
    CONSTRAINT contrato_propuesta_comercial_terminos_check10 CHECK (((jsonb_typeof((terminos -> 'units'::text)) = 'number'::text) AND (((terminos ->> 'units'::text))::bigint > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check11 CHECK (((jsonb_typeof((terminos -> 'rate_unit'::text)) = 'string'::text) AND ((terminos ->> 'rate_unit'::text) = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text])))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check12 CHECK (((jsonb_typeof(((terminos -> 'location'::text) -> 'latitude'::text)) = 'number'::text) AND (((((terminos -> 'location'::text) ->> 'latitude'::text))::double precision >= ('-90'::integer)::double precision) AND ((((terminos -> 'location'::text) ->> 'latitude'::text))::double precision <= (90)::double precision)))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check13 CHECK (((jsonb_typeof(((terminos -> 'location'::text) -> 'longitude'::text)) = 'number'::text) AND (((((terminos -> 'location'::text) ->> 'longitude'::text))::double precision >= ('-180'::integer)::double precision) AND ((((terminos -> 'location'::text) ->> 'longitude'::text))::double precision <= (180)::double precision)))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check14 CHECK (((jsonb_typeof(((terminos -> 'location'::text) -> 'revision'::text)) = 'number'::text) AND ((((terminos -> 'location'::text) ->> 'revision'::text))::bigint >= 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check15 CHECK (((jsonb_typeof(((terminos -> 'location'::text) -> 'address'::text)) = 'string'::text) AND (length(((terminos -> 'location'::text) ->> 'address'::text)) > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check16 CHECK ((jsonb_typeof(((terminos -> 'location'::text) -> 'region_code'::text)) = 'string'::text)),
    CONSTRAINT contrato_propuesta_comercial_terminos_check17 CHECK ((jsonb_typeof(((terminos -> 'location'::text) -> 'commune_code'::text)) = 'string'::text)),
    CONSTRAINT contrato_propuesta_comercial_terminos_check18 CHECK (((jsonb_typeof(((terminos -> 'cancellation_policy'::text) -> 'version'::text)) = 'string'::text) AND (length(((terminos -> 'cancellation_policy'::text) ->> 'version'::text)) > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check19 CHECK (((jsonb_typeof(((terminos -> 'cancellation_policy'::text) -> 'text'::text)) = 'string'::text) AND (length(((terminos -> 'cancellation_policy'::text) ->> 'text'::text)) > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check2 CHECK (((jsonb_typeof((terminos -> 'location'::text)) = 'object'::text) AND ((terminos -> 'location'::text) ?& ARRAY['address'::text, 'latitude'::text, 'longitude'::text, 'revision'::text, 'region_code'::text, 'commune_code'::text]))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check3 CHECK (((jsonb_typeof((terminos -> 'cancellation_policy'::text)) = 'object'::text) AND ((terminos -> 'cancellation_policy'::text) ?& ARRAY['version'::text, 'text'::text]))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check4 CHECK ((NOT (terminos ?| ARRAY['parties'::text, 'owner_id'::text, 'renter_id'::text, 'propietario_id'::text, 'arrendatario_id'::text, 'legal_name'::text, 'rut'::text, 'email'::text, 'phone'::text]))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check6 CHECK (((jsonb_typeof((terminos -> 'space_title'::text)) = 'string'::text) AND (length((terminos ->> 'space_title'::text)) > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check7 CHECK (((jsonb_typeof((terminos -> 'rate_version'::text)) = 'number'::text) AND (((terminos ->> 'rate_version'::text))::bigint > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check8 CHECK (((jsonb_typeof((terminos -> 'template_version'::text)) = 'number'::text) AND (((terminos ->> 'template_version'::text))::bigint > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_check9 CHECK (((jsonb_typeof((terminos -> 'unit_price_clp'::text)) = 'number'::text) AND (((terminos ->> 'unit_price_clp'::text))::bigint > 0))),
    CONSTRAINT contrato_propuesta_comercial_terminos_secciones_check CHECK (((jsonb_typeof((terminos -> 'sections'::text)) = 'array'::text) AND ((jsonb_array_length((terminos -> 'sections'::text)) >= 1) AND (jsonb_array_length((terminos -> 'sections'::text)) <= 13)))),
    CONSTRAINT contrato_propuesta_comercial_terminos_sha256_check CHECK ((octet_length(terminos_sha256) = 32)),
    CONSTRAINT contrato_propuesta_comercial_version_check CHECK ((version > 0)),
    CONSTRAINT contrato_propuesta_company_terms_snapshot_ck CHECK ((((NOT es_b2b) AND (NOT (terminos ? 'company_representation'::text))) OR ((es_b2b AND (jsonb_typeof((terminos -> 'company_representation'::text)) = 'object'::text) AND ((((terminos -> 'company_representation'::text) ->> 'organization_id'::text))::uuid = organizacion_id) AND (((terminos -> 'company_representation'::text) ->> 'legal_name'::text) = (empresa_razon_social_snapshot)::text) AND (((terminos -> 'company_representation'::text) ->> 'rut_masked'::text) = (empresa_rut_masked_snapshot)::text) AND ((((terminos -> 'company_representation'::text) ->> 'rut_verified'::text))::boolean = empresa_rut_verificada) AND ((((terminos -> 'company_representation'::text) ->> 'representative_user_id'::text))::uuid = representante_legal_id) AND (((terminos -> 'company_representation'::text) ->> 'representative_name'::text) = (representante_nombre_snapshot)::text) AND (((terminos -> 'company_representation'::text) ->> 'representative_rut_masked'::text) = (representante_rut_masked_snapshot)::text) AND ((((terminos -> 'company_representation'::text) ->> 'representative_rut_verified'::text))::boolean = representante_rut_verificada)) IS TRUE)))
);


--
-- Name: TABLE contrato_propuesta_comercial; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.contrato_propuesta_comercial IS 'Commercial proposal only; terms without legal identities, separate AEAD identity evidence, full canonical document digest. Does not sign, reserve, hold or enable payment.';


--
-- Name: contrato_solicitud_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_solicitud_comercial (
    id uuid NOT NULL,
    propuesta_id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    propuesta_version bigint NOT NULL,
    documento_sha256 bytea NOT NULL,
    idempotency_key text NOT NULL,
    fingerprint bytea NOT NULL,
    estado text NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    pago_vence_en timestamp with time zone,
    ocupacion_id uuid,
    revision bigint DEFAULT 1 NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    modalidad_firma text DEFAULT 'fes_interna'::text NOT NULL,
    es_b2b boolean DEFAULT false NOT NULL,
    organizacion_id uuid,
    empresa_razon_social_snapshot character varying(200),
    empresa_rut_masked_snapshot character varying(16),
    empresa_rut_verificada boolean DEFAULT false NOT NULL,
    representante_legal_id uuid,
    representante_nombre_snapshot character varying(200),
    representante_rut_masked_snapshot character varying(16),
    representante_rut_verificada boolean DEFAULT false CONSTRAINT contrato_solicitud_comercia_representante_rut_verifica_not_null NOT NULL,
    CONSTRAINT contrato_solicitud_b2b_snapshot_ck CHECK ((((NOT es_b2b) AND (organizacion_id IS NULL)) OR (es_b2b AND (organizacion_id IS NOT NULL) AND (empresa_razon_social_snapshot IS NOT NULL) AND (empresa_rut_masked_snapshot IS NOT NULL) AND empresa_rut_verificada AND (representante_legal_id IS NOT NULL) AND (representante_nombre_snapshot IS NOT NULL) AND (representante_rut_masked_snapshot IS NOT NULL) AND representante_rut_verificada))),
    CONSTRAINT contrato_solicitud_comercial_check CHECK ((propietario_id <> arrendatario_id)),
    CONSTRAINT contrato_solicitud_comercial_check1 CHECK ((isfinite(vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en) AND (vence_en > creada_en) AND (actualizada_en >= creada_en))),
    CONSTRAINT contrato_solicitud_comercial_check2 CHECK (((pago_vence_en IS NULL) OR (isfinite(pago_vence_en) AND (pago_vence_en > creada_en)))),
    CONSTRAINT contrato_solicitud_comercial_check3 CHECK (((estado <> 'ready_for_payment'::text) OR ((pago_vence_en IS NOT NULL) AND (ocupacion_id IS NOT NULL)))),
    CONSTRAINT contrato_solicitud_comercial_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_solicitud_comercial_estado_check CHECK ((estado = ANY (ARRAY['pending_landlord'::text, 'ready_for_payment'::text, 'rejected'::text, 'withdrawn'::text, 'expired'::text]))),
    CONSTRAINT contrato_solicitud_comercial_fingerprint_check CHECK ((octet_length(fingerprint) = 32)),
    CONSTRAINT contrato_solicitud_comercial_idempotency_key_check CHECK (((char_length(idempotency_key) >= 1) AND (char_length(idempotency_key) <= 200))),
    CONSTRAINT contrato_solicitud_comercial_modalidad_firma_check CHECK ((modalidad_firma = ANY (ARRAY['fes_interna'::text, 'firmavirtual_notarial'::text, 'fea_externa'::text]))),
    CONSTRAINT contrato_solicitud_comercial_propuesta_version_check CHECK ((propuesta_version > 0)),
    CONSTRAINT contrato_solicitud_comercial_revision_check CHECK ((revision > 0))
);


--
-- Name: contrato_solicitud_evento_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_solicitud_evento_comercial (
    id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    actor_id uuid,
    tipo text NOT NULL,
    revision bigint NOT NULL,
    documento_sha256 bytea NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT contrato_solicitud_evento_comercial_documento_sha256_check CHECK ((octet_length(documento_sha256) = 32)),
    CONSTRAINT contrato_solicitud_evento_comercial_ocurrida_en_check CHECK (isfinite(ocurrida_en)),
    CONSTRAINT contrato_solicitud_evento_comercial_revision_check CHECK ((revision > 0)),
    CONSTRAINT contrato_solicitud_evento_comercial_tipo_check CHECK ((tipo = ANY (ARRAY['request_created'::text, 'landlord_signed'::text, 'rejected'::text, 'withdrawn'::text, 'expired'::text])))
);


--
-- Name: contrato_solicitud_notice_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_solicitud_notice_outbox (
    id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    destinatario_id uuid NOT NULL,
    tipo text NOT NULL,
    estado text DEFAULT 'pending'::text NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    disponible_en timestamp with time zone NOT NULL,
    reclamada_en timestamp with time zone,
    entregada_en timestamp with time zone,
    ultimo_error text,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT contrato_solicitud_notice_outbox_estado_check CHECK ((estado = ANY (ARRAY['pending'::text, 'processing'::text, 'delivered'::text, 'cancelled'::text, 'terminal'::text]))),
    CONSTRAINT contrato_solicitud_notice_outbox_intentos_check CHECK (((intentos >= 0) AND (intentos <= 8))),
    CONSTRAINT contrato_solicitud_notice_outbox_tipo_check CHECK ((tipo = ANY (ARRAY['request_created'::text, 'landlord_signed'::text, 'rejected'::text, 'withdrawn'::text, 'expired'::text]))),
    CONSTRAINT contrato_solicitud_notice_outbox_ultimo_error_check CHECK (((ultimo_error IS NULL) OR (ultimo_error = ANY (ARRAY['delivery_failed'::text, 'recipient_unavailable'::text, 'attempt_limit_reached'::text]))))
);


--
-- Name: contrato_solicitud_quote_activa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_solicitud_quote_activa (
    solicitud_id uuid NOT NULL,
    cotizacion_id uuid NOT NULL
);


--
-- Name: cotizacion_arriendo_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cotizacion_arriendo_comercial (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    titulo_espacio text NOT NULL,
    tarifa_version integer NOT NULL,
    modalidad text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    moneda text NOT NULL,
    unidades bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    politica_cancelacion_version text,
    politica_cancelacion_texto text,
    CONSTRAINT cotizacion_arriendo_comercial_intervalo_ck CHECK (((termino > inicio) AND (zona_horaria <> ''::text))),
    CONSTRAINT cotizacion_arriendo_comercial_participantes_ck CHECK ((propietario_id <> arrendatario_id)),
    CONSTRAINT cotizacion_arriendo_comercial_subtotal_ck CHECK ((subtotal_clp = (precio_unitario_clp * unidades))),
    CONSTRAINT cotizacion_arriendo_comercial_tarifa_ck CHECK (((tarifa_version > 0) AND (modalidad = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text])) AND (precio_unitario_clp > 0) AND (moneda = 'CLP'::text))),
    CONSTRAINT cotizacion_arriendo_comercial_titulo_ck CHECK (((char_length(btrim(titulo_espacio)) >= 1) AND (char_length(btrim(titulo_espacio)) <= 70))),
    CONSTRAINT cotizacion_arriendo_comercial_unidades_ck CHECK ((((modalidad = 'hora'::text) AND ((unidades >= 1) AND (unidades <= 168))) OR ((modalidad = 'dia'::text) AND ((unidades >= 1) AND (unidades <= 64))) OR ((modalidad = 'mes'::text) AND ((unidades >= 1) AND (unidades <= 12))))),
    CONSTRAINT cotizacion_comercial_cancelacion_snapshot_ck CHECK ((((politica_cancelacion_version IS NULL) AND (politica_cancelacion_texto IS NULL)) OR ((politica_cancelacion_version IS NOT NULL) AND (politica_cancelacion_version = ANY (ARRAY['facil_v1'::text, 'flexible_v1'::text, 'moderada_v1'::text, 'estricta_v1'::text])) AND (politica_cancelacion_texto IS NOT NULL) AND ((length(btrim(politica_cancelacion_texto)) >= 1) AND (length(btrim(politica_cancelacion_texto)) <= 500)))))
);


--
-- Name: TABLE cotizacion_arriendo_comercial; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cotizacion_arriendo_comercial IS 'Commercial 15-minute quote snapshot; excludes legal identity and is purged at expiry or participant/space removal.';


--
-- Name: cotizacion_arriendo_tramo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cotizacion_arriendo_tramo (
    id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    unidades bigint NOT NULL,
    orden integer NOT NULL,
    CONSTRAINT cotizacion_arriendo_tramo_check CHECK ((termino > inicio)),
    CONSTRAINT cotizacion_arriendo_tramo_check1 CHECK ((isfinite(inicio) AND isfinite(termino))),
    CONSTRAINT cotizacion_arriendo_tramo_orden_check CHECK ((orden >= 1)),
    CONSTRAINT cotizacion_arriendo_tramo_unidades_check CHECK ((unidades > 0))
);


--
-- Name: cotizacion_modalidad_cobro; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cotizacion_modalidad_cobro (
    cotizacion_id uuid NOT NULL,
    modalidad_cobro text DEFAULT 'full'::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cotizacion_modalidad_cobro_modalidad_cobro_check CHECK ((modalidad_cobro = ANY (ARRAY['full'::text, 'monthly_installments'::text])))
);


--
-- Name: cotizacion_reserva_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cotizacion_reserva_ensayo (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    anfitrion_id uuid,
    arrendatario_id uuid,
    tarifa_version integer NOT NULL,
    modalidad text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    moneda character(3) NOT NULL,
    unidades bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    condiciones_snapshot text NOT NULL,
    categoria_codigo text NOT NULL,
    perfil_version integer NOT NULL,
    perfil_valores_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    politica_cancelacion_version text DEFAULT 'local_flexible_v1'::text NOT NULL,
    retirar_en timestamp with time zone,
    garantia_politica_version text,
    garantia_moneda character(3),
    garantia_prevista_clp numeric(14,0),
    liquidacion_politica_version text,
    liquidacion_comision_bps integer,
    CONSTRAINT cotizacion_ensayo_snapshot_object_ck CHECK (((jsonb_typeof(perfil_valores_snapshot) = 'object'::text) AND (octet_length((perfil_valores_snapshot)::text) <= 16384))),
    CONSTRAINT cotizacion_reserva_ensayo_check CHECK ((termino > inicio)),
    CONSTRAINT cotizacion_reserva_ensayo_check1 CHECK ((vence_en > creada_en)),
    CONSTRAINT cotizacion_reserva_ensayo_modalidad_check CHECK ((modalidad = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text]))),
    CONSTRAINT cotizacion_reserva_ensayo_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT cotizacion_reserva_ensayo_precio_unitario_clp_min_1000_ck CHECK ((precio_unitario_clp >= 1000)),
    CONSTRAINT cotizacion_reserva_ensayo_subtotal_clp_check CHECK ((subtotal_clp > 0)),
    CONSTRAINT cotizacion_reserva_ensayo_unidades_check CHECK ((unidades > 0)),
    CONSTRAINT cotizacion_reserva_ensayo_zona_horaria_check CHECK (((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100))),
    CONSTRAINT cotizacion_reserva_liquidacion_snapshot_ck CHECK ((((liquidacion_politica_version IS NULL) AND (liquidacion_comision_bps IS NULL)) OR ((liquidacion_politica_version = 'liquidacion_local_0pct_v1'::text) AND (liquidacion_comision_bps = 0))))
);


--
-- Name: cuenta_alerta_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_alerta_evento (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    event_code text NOT NULL,
    title text NOT NULL,
    device_label text,
    ip_masked text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_alerta_evento; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_alerta_evento IS 'Historial de eventos y alertas criticas de seguridad por cuenta.';


--
-- Name: cuenta_alerta_preferencia; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_alerta_preferencia (
    account_id uuid NOT NULL,
    app_notifications_enabled boolean DEFAULT true NOT NULL,
    email_alerts_enabled boolean DEFAULT true NOT NULL,
    whatsapp_alerts_enabled boolean DEFAULT false NOT NULL,
    extra_email text,
    extra_email_verified_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_alerta_preferencia; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_alerta_preferencia IS 'Preferencias de canales de alerta de seguridad por cuenta.';


--
-- Name: cuenta_cobro_sintetica_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_cobro_sintetica_historial_local (
    id bigint NOT NULL,
    cuenta_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    accion text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    clave_idempotencia text CONSTRAINT cuenta_cobro_sintetica_historial_lo_clave_idempotencia_not_null NOT NULL,
    CONSTRAINT cuenta_cobro_sintetica_historial_local_accion_check CHECK ((accion = ANY (ARRAY['creada'::text, 'actualizada'::text, 'revocada'::text, 'retirada_baja'::text]))),
    CONSTRAINT cuenta_cobro_sintetica_historial_local_clave_idempotencia_check CHECK (((length(clave_idempotencia) >= 8) AND (length(clave_idempotencia) <= 128)))
);


--
-- Name: cuenta_cobro_sintetica_historial_local_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cuenta_cobro_sintetica_historial_local_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cuenta_cobro_sintetica_historial_local_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cuenta_cobro_sintetica_historial_local_id_seq OWNED BY public.cuenta_cobro_sintetica_historial_local.id;


--
-- Name: cuenta_cobro_sintetica_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_cobro_sintetica_local (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    adaptador text NOT NULL,
    referencia_ficticia text,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    CONSTRAINT cuenta_cobro_sintetica_local_adaptador_check CHECK ((adaptador = 'fake-local-v1'::text)),
    CONSTRAINT cuenta_cobro_sintetica_local_estado_check CHECK ((estado = ANY (ARRAY['activa'::text, 'reemplazada'::text, 'revocada'::text, 'retirada_baja'::text]))),
    CONSTRAINT cuenta_cobro_sintetica_reference_ck CHECK ((((estado = 'retirada_baja'::text) AND (referencia_ficticia IS NULL)) OR ((estado <> 'retirada_baja'::text) AND (referencia_ficticia ~ '^demo_[a-f0-9-]{36}$'::text)))),
    CONSTRAINT cuenta_cobro_sintetica_revocation_ck CHECK ((((estado = ANY (ARRAY['revocada'::text, 'retirada_baja'::text])) AND (revocada_en IS NOT NULL)) OR ((estado = ANY (ARRAY['activa'::text, 'reemplazada'::text])) AND (revocada_en IS NULL))))
);


--
-- Name: cuenta_passkey; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_passkey (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    credential_id text NOT NULL,
    public_key bytea NOT NULL,
    algorithm integer DEFAULT '-7'::integer NOT NULL,
    sign_count bigint DEFAULT 0 NOT NULL,
    label text NOT NULL,
    transports text[] DEFAULT '{}'::text[] NOT NULL,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_passkey; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_passkey IS 'Credenciales WebAuthn / FIDO2 (Passkeys) asociadas a cuentas de usuario.';


--
-- Name: cuenta_permiso_ubicacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_permiso_ubicacion (
    account_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    consent_updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_region_label text
);


--
-- Name: TABLE cuenta_permiso_ubicacion; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_permiso_ubicacion IS 'Configuracion de consentimiento y permiso de ubicacion geografica por cuenta.';


--
-- Name: cuenta_persona_confianza; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_persona_confianza (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    full_name text NOT NULL,
    email text NOT NULL,
    phone_e164 text,
    relationship text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cuenta_persona_confianza_status_ck CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text])))
);


--
-- Name: TABLE cuenta_persona_confianza; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_persona_confianza IS 'Personas o contactos de confianza designados para recuperacion y seguridad.';


--
-- Name: cuenta_qr_dispositivo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_qr_dispositivo (
    account_id uuid NOT NULL,
    enabled_at timestamp with time zone,
    trusted_device_label text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_qr_dispositivo; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_qr_dispositivo IS 'Estado y dispositivo de confianza para verificacion mediante codigo QR.';


--
-- Name: cuenta_qr_ticket; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_qr_ticket (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    token_hash text NOT NULL,
    status text NOT NULL,
    device_label text,
    ip_masked text,
    expires_at timestamp with time zone NOT NULL,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cuenta_qr_ticket_status_ck CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text])))
);


--
-- Name: TABLE cuenta_qr_ticket; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_qr_ticket IS 'Tickets y desafios de verificacion de inicio de sesion mediante codigo QR.';


--
-- Name: cuenta_totp; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_totp (
    account_id uuid NOT NULL,
    secret_ciphertext bytea,
    key_version text,
    pending_secret_ciphertext bytea,
    pending_key_version text,
    enabled_at timestamp with time zone,
    last_verified_step bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_totp; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_totp IS 'Estado y secretos cifrados de autenticador TOTP (Google Authenticator) por cuenta.';


--
-- Name: cuenta_totp_backup_code; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_totp_backup_code (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    code_hash text NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE cuenta_totp_backup_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_totp_backup_code IS 'Codigos de respaldo de recuperacion TOTP con hash SHA-256 por cuenta.';


--
-- Name: cuenta_verificacion_telefono; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_verificacion_telefono (
    account_id uuid NOT NULL,
    phone_e164 text NOT NULL,
    verified_at timestamp with time zone,
    otp_hash text,
    otp_expires_at timestamp with time zone,
    otp_attempts integer DEFAULT 0 NOT NULL,
    resend_available_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cuenta_verificacion_telefono_otp_attempts_ck CHECK ((otp_attempts >= 0))
);


--
-- Name: TABLE cuenta_verificacion_telefono; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_verificacion_telefono IS 'Registro y verificacion por SMS/OTP del numero telefonico de la cuenta.';


--
-- Name: cuenta_webauthn_challenge; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_webauthn_challenge (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    challenge text NOT NULL,
    purpose text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cuenta_webauthn_challenge_purpose_ck CHECK ((purpose = ANY (ARRAY['register'::text, 'authenticate'::text])))
);


--
-- Name: TABLE cuenta_webauthn_challenge; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cuenta_webauthn_challenge IS 'Desafios (challenges) temporales emitidos para registro o autenticacion WebAuthn.';


--
-- Name: desafio_mfa_administrativo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.desafio_mfa_administrativo (
    id uuid NOT NULL,
    cuenta_id uuid NOT NULL,
    sesion_administrativa_id uuid,
    proposito text NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    max_intentos integer DEFAULT 5 NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    consumido_en timestamp with time zone,
    ip_enmascarada text DEFAULT ''::text NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL,
    CONSTRAINT desafio_mfa_administrativo_creado_en_check CHECK (isfinite(creado_en)),
    CONSTRAINT desafio_mfa_administrativo_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT desafio_mfa_administrativo_intentos_check CHECK ((intentos >= 0)),
    CONSTRAINT desafio_mfa_administrativo_max_intentos_check CHECK ((max_intentos > 0)),
    CONSTRAINT desafio_mfa_administrativo_proposito_check CHECK ((proposito = ANY (ARRAY['login'::text, 'step_up'::text])))
);


--
-- Name: didit_webhook_inbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.didit_webhook_inbox (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_id character varying(120) NOT NULL,
    session_id character varying(100) NOT NULL,
    event_type character varying(80) NOT NULL,
    payload jsonb NOT NULL,
    signature_header character varying(255),
    status character varying(30) DEFAULT 'PENDING'::character varying NOT NULL,
    error_message text,
    received_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    processed_at timestamp with time zone,
    CONSTRAINT didit_webhook_inbox_status_check CHECK (((status)::text = ANY (ARRAY[('PENDING'::character varying)::text, ('PROCESSED'::character varying)::text, ('FAILED'::character varying)::text, ('IGNORED'::character varying)::text])))
);


--
-- Name: TABLE didit_webhook_inbox; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.didit_webhook_inbox IS 'Durable, deduplicated inbox for authenticated Didit webhook events.';


--
-- Name: disputa_ensayo_historial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.disputa_ensayo_historial (
    secuencia bigint NOT NULL,
    disputa_id uuid NOT NULL,
    estado_anterior text,
    estado_nuevo text NOT NULL,
    actor_id uuid,
    motivo_codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT disputa_ensayo_historial_estado_anterior_check CHECK ((estado_anterior = ANY (ARRAY['abierta'::text, 'cerrada'::text]))),
    CONSTRAINT disputa_ensayo_historial_estado_nuevo_check CHECK ((estado_nuevo = ANY (ARRAY['abierta'::text, 'cerrada'::text]))),
    CONSTRAINT disputa_ensayo_historial_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['ensayo_privacidad'::text, 'ensayo_finalizado'::text, 'registro_erroneo'::text, 'duplicada'::text]))),
    CONSTRAINT disputa_ensayo_historial_transicion_ck CHECK ((((estado_anterior IS NULL) AND (estado_nuevo = 'abierta'::text) AND (motivo_codigo = 'ensayo_privacidad'::text)) OR ((estado_anterior = 'abierta'::text) AND (estado_nuevo = 'cerrada'::text) AND (motivo_codigo = ANY (ARRAY['ensayo_finalizado'::text, 'registro_erroneo'::text, 'duplicada'::text])))))
);


--
-- Name: disputa_ensayo_historial_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.disputa_ensayo_historial ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.disputa_ensayo_historial_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: disputa_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.disputa_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    anfitrion_id uuid,
    arrendatario_id uuid,
    abierta_por uuid,
    motivo_codigo text NOT NULL,
    estado text NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    abierta_en timestamp with time zone NOT NULL,
    cerrada_por uuid,
    motivo_cierre_codigo text,
    cerrada_en timestamp with time zone,
    CONSTRAINT disputa_ensayo_apertura_ck CHECK (((abierta_por = anfitrion_id) OR ((abierta_por IS NULL) AND (anfitrion_id IS NULL)))),
    CONSTRAINT disputa_ensayo_cierre_ck CHECK ((((estado = 'abierta'::text) AND (cerrada_por IS NULL) AND (motivo_cierre_codigo IS NULL) AND (cerrada_en IS NULL)) OR ((estado = 'cerrada'::text) AND (motivo_cierre_codigo IS NOT NULL) AND (cerrada_en IS NOT NULL) AND ((cerrada_por IS NOT NULL) OR ((anfitrion_id IS NULL) AND (arrendatario_id IS NULL)))))),
    CONSTRAINT disputa_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT disputa_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['abierta'::text, 'cerrada'::text]))),
    CONSTRAINT disputa_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT disputa_ensayo_local_motivo_cierre_codigo_check CHECK ((motivo_cierre_codigo = ANY (ARRAY['ensayo_finalizado'::text, 'registro_erroneo'::text, 'duplicada'::text]))),
    CONSTRAINT disputa_ensayo_local_motivo_codigo_check CHECK ((motivo_codigo = 'ensayo_privacidad'::text)),
    CONSTRAINT disputa_ensayo_participantes_ck CHECK ((anfitrion_id <> arrendatario_id))
);


--
-- Name: document_verification_case; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_case (
    id uuid NOT NULL,
    purpose text NOT NULL,
    subject_id uuid NOT NULL,
    space_id uuid,
    state text DEFAULT 'borrador'::text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    reviewer_id uuid,
    verified_at timestamp with time zone,
    verified_until timestamp with time zone,
    document_expires_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    superseded_at timestamp with time zone,
    identity_declaration_revision_id uuid,
    document_format_declared text,
    capture_consent_version text,
    capture_consented_at timestamp with time zone,
    document_format_observed text,
    CONSTRAINT document_verification_case_identity_metadata_ck CHECK ((((purpose = 'space'::text) AND (identity_declaration_revision_id IS NULL) AND (document_format_declared IS NULL) AND (capture_consent_version IS NULL) AND (capture_consented_at IS NULL)) OR ((purpose = 'identity'::text) AND (((identity_declaration_revision_id IS NULL) AND (document_format_declared IS NULL) AND (capture_consent_version IS NULL) AND (capture_consented_at IS NULL)) OR (((identity_declaration_revision_id IS NOT NULL) OR (state = ANY (ARRAY['rechazada'::text, 'vencida'::text, 'revocada'::text]))) AND (document_format_declared IS NOT NULL) AND (capture_consent_version IS NOT NULL) AND (capture_consented_at IS NOT NULL) AND (document_format_declared = ANY (ARRAY['legacy'::text, 'new'::text, 'unsure'::text])) AND (capture_consent_version = 'identity-capture-local-v1'::text)))))),
    CONSTRAINT document_verification_case_observed_format_ck CHECK (((document_format_observed IS NULL) OR ((purpose = 'identity'::text) AND (document_format_observed = ANY (ARRAY['legacy'::text, 'new'::text, 'undetermined'::text]))))),
    CONSTRAINT document_verification_case_purpose_check CHECK ((purpose = ANY (ARRAY['identity'::text, 'space'::text]))),
    CONSTRAINT document_verification_case_reviewer_ck CHECK ((((state = ANY (ARRAY['borrador'::text, 'enviada'::text])) AND (reviewer_id IS NULL)) OR ((state <> ALL (ARRAY['borrador'::text, 'enviada'::text])) AND (reviewer_id IS NOT NULL)))),
    CONSTRAINT document_verification_case_revision_check CHECK ((revision > 0)),
    CONSTRAINT document_verification_case_revoked_ck CHECK ((((state = 'revocada'::text) AND (revoked_at IS NOT NULL) AND (revoked_at >= verified_at)) OR ((state <> 'revocada'::text) AND (revoked_at IS NULL)))),
    CONSTRAINT document_verification_case_scope_ck CHECK ((((purpose = 'identity'::text) AND (space_id IS NULL)) OR ((purpose = 'space'::text) AND (space_id IS NOT NULL)))),
    CONSTRAINT document_verification_case_state_check CHECK ((state = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text, 'aprobada'::text, 'rechazada'::text, 'vencida'::text, 'revocada'::text]))),
    CONSTRAINT document_verification_case_superseded_scope_ck CHECK (((superseded_at IS NULL) OR (purpose = 'space'::text))),
    CONSTRAINT document_verification_case_verified_ck CHECK ((((state = ANY (ARRAY['aprobada'::text, 'vencida'::text, 'revocada'::text])) AND (verified_at IS NOT NULL) AND (verified_until > verified_at)) OR ((state <> ALL (ARRAY['aprobada'::text, 'vencida'::text, 'revocada'::text])) AND (verified_at IS NULL) AND (verified_until IS NULL))))
);


--
-- Name: COLUMN document_verification_case.superseded_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.document_verification_case.superseded_at IS 'Internal terminal marker for an ownership-change case replacement; does not add a user-facing case state.';


--
-- Name: document_verification_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_event (
    id bigint NOT NULL,
    case_id uuid NOT NULL,
    purpose text NOT NULL,
    subject_id uuid NOT NULL,
    file_id uuid,
    actor_kind text DEFAULT 'user'::text NOT NULL,
    actor_id uuid,
    action text NOT NULL,
    from_state text,
    to_state text NOT NULL,
    result_code text NOT NULL,
    reason_code text,
    revision bigint NOT NULL,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    identity_review_observations jsonb,
    CONSTRAINT document_verification_event_action_check CHECK ((action = ANY (ARRAY['case_created'::text, 'case_transitioned'::text, 'case_superseded'::text, 'file_added'::text, 'scan_result'::text, 'review_decision'::text, 'revoked'::text, 'expired'::text, 'retention_deleted'::text]))),
    CONSTRAINT document_verification_event_actor_ck CHECK ((((actor_kind = 'user'::text) AND (actor_id IS NOT NULL)) OR ((actor_kind = ANY (ARRAY['scanner'::text, 'system'::text])) AND (actor_id IS NULL)))),
    CONSTRAINT document_verification_event_actor_kind_check CHECK ((actor_kind = ANY (ARRAY['user'::text, 'scanner'::text, 'system'::text]))),
    CONSTRAINT document_verification_event_file_action_ck CHECK ((((action = ANY (ARRAY['file_added'::text, 'scan_result'::text, 'retention_deleted'::text])) AND (file_id IS NOT NULL)) OR ((action <> ALL (ARRAY['file_added'::text, 'scan_result'::text, 'retention_deleted'::text])) AND (file_id IS NULL)))),
    CONSTRAINT document_verification_event_from_state_check CHECK (((from_state IS NULL) OR (from_state = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text, 'aprobada'::text, 'rechazada'::text, 'vencida'::text, 'revocada'::text])))),
    CONSTRAINT document_verification_event_identity_observations_ck CHECK (((identity_review_observations IS NULL) OR ((purpose = 'identity'::text) AND (action = 'review_decision'::text) AND (jsonb_typeof(identity_review_observations) = 'object'::text) AND (identity_review_observations ?& ARRAY['document_format_observed'::text, 'document_expires_at_observed'::text, 'identity_review_checklist'::text]) AND (identity_review_observations = jsonb_build_object('document_format_observed', (identity_review_observations -> 'document_format_observed'::text), 'document_expires_at_observed', (identity_review_observations -> 'document_expires_at_observed'::text), 'identity_review_checklist', (identity_review_observations -> 'identity_review_checklist'::text))) AND ((identity_review_observations ->> 'document_format_observed'::text) IS NOT NULL) AND ((identity_review_observations ->> 'document_format_observed'::text) = ANY (ARRAY['legacy'::text, 'new'::text, 'undetermined'::text])) AND
CASE
    WHEN (jsonb_typeof((identity_review_observations -> 'document_expires_at_observed'::text)) = 'null'::text) THEN true
    WHEN (jsonb_typeof((identity_review_observations -> 'document_expires_at_observed'::text)) = 'string'::text) THEN ((identity_review_observations ->> 'document_expires_at_observed'::text) ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?Z$'::text)
    ELSE false
END AND (jsonb_typeof((identity_review_observations -> 'identity_review_checklist'::text)) = 'object'::text) AND ((identity_review_observations -> 'identity_review_checklist'::text) ?& ARRAY['names_match'::text, 'rut_matches'::text, 'birth_date_and_majority_match'::text, 'portrait_matches_selfie'::text, 'evidence_legible'::text, 'document_unexpired'::text, 'issue_date_and_format_verified'::text]) AND ((identity_review_observations -> 'identity_review_checklist'::text) = jsonb_build_object('names_match', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'names_match'::text), 'rut_matches', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'rut_matches'::text), 'birth_date_and_majority_match', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'birth_date_and_majority_match'::text), 'portrait_matches_selfie', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'portrait_matches_selfie'::text), 'evidence_legible', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'evidence_legible'::text), 'document_unexpired', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'document_unexpired'::text), 'issue_date_and_format_verified', ((identity_review_observations -> 'identity_review_checklist'::text) -> 'issue_date_and_format_verified'::text))) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'names_match'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'rut_matches'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'birth_date_and_majority_match'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'portrait_matches_selfie'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'evidence_legible'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'document_unexpired'::text)) = 'boolean'::text) AND (jsonb_typeof(((identity_review_observations -> 'identity_review_checklist'::text) -> 'issue_date_and_format_verified'::text)) = 'boolean'::text)))),
    CONSTRAINT document_verification_event_purpose_check CHECK ((purpose = ANY (ARRAY['identity'::text, 'space'::text]))),
    CONSTRAINT document_verification_event_reason_code_check CHECK (((reason_code IS NULL) OR (reason_code = ANY (ARRAY['documento_ilegible'::text, 'documento_invalido'::text, 'documento_vencido'::text, 'antecedentes_incompletos'::text, 'cambio_titularidad'::text, 'conflicto_interes'::text, 'malware_detectado'::text, 'scanner_no_disponible'::text, 'firmas_desactualizadas'::text, 'solicitud_titular'::text, 'retencion_vencida'::text])))),
    CONSTRAINT document_verification_event_result_code_check CHECK ((result_code = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text, 'aprobada'::text, 'rechazada'::text, 'vencida'::text, 'revocada'::text, 'pendiente'::text, 'limpio'::text, 'bloqueado'::text, 'error'::text, 'eliminado'::text]))),
    CONSTRAINT document_verification_event_revision_check CHECK ((revision > 0)),
    CONSTRAINT document_verification_event_to_state_check CHECK ((to_state = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text, 'aprobada'::text, 'rechazada'::text, 'vencida'::text, 'revocada'::text])))
);


--
-- Name: document_verification_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.document_verification_event ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.document_verification_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: document_verification_file; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_file (
    id uuid NOT NULL,
    case_id uuid NOT NULL,
    state text DEFAULT 'pendiente'::text NOT NULL,
    size_bytes bigint NOT NULL,
    mime_type text NOT NULL,
    pages integer NOT NULL,
    content_sha256 bytea NOT NULL,
    scan_attempts smallint DEFAULT 0 NOT NULL,
    scanner_version text,
    signatures_at timestamp with time zone,
    inspected_at timestamp with time zone,
    last_error_code text,
    retention_at timestamp with time zone,
    retention_reason_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT document_verification_file_clean_ck CHECK (((state <> 'limpio'::text) OR ((inspected_at IS NOT NULL) AND (scanner_version IS NOT NULL) AND (signatures_at IS NOT NULL)))),
    CONSTRAINT document_verification_file_content_sha256_check CHECK ((octet_length(content_sha256) = 32)),
    CONSTRAINT document_verification_file_last_error_code_check CHECK (((last_error_code IS NULL) OR (last_error_code = ANY (ARRAY['scanner_unavailable'::text, 'scanner_malformed'::text, 'signatures_stale'::text, 'malware_detected'::text, 'inspection_rejected'::text, 'retention_store_error'::text])))),
    CONSTRAINT document_verification_file_mime_type_check CHECK ((mime_type = ANY (ARRAY['image/jpeg'::text, 'image/png'::text, 'application/pdf'::text]))),
    CONSTRAINT document_verification_file_pages_check CHECK (((pages >= 0) AND (pages <= 20))),
    CONSTRAINT document_verification_file_retention_reason_code_check CHECK (((retention_reason_code IS NULL) OR (retention_reason_code = ANY (ARRAY['abandoned_draft'::text, 'malicious_file'::text, 'inspection_error'::text, 'identity_decision'::text, 'space_expiry'::text, 'rejected_or_replaced'::text, 'revoked'::text])))),
    CONSTRAINT document_verification_file_scan_attempts_check CHECK (((scan_attempts >= 0) AND (scan_attempts <= 5))),
    CONSTRAINT document_verification_file_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760))),
    CONSTRAINT document_verification_file_state_check CHECK ((state = ANY (ARRAY['pendiente'::text, 'limpio'::text, 'bloqueado'::text, 'error'::text])))
);


--
-- Name: document_verification_job; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_job (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    file_id uuid NOT NULL,
    kind text NOT NULL,
    state text DEFAULT 'pendiente'::text NOT NULL,
    attempts smallint DEFAULT 0 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    lease_until timestamp with time zone,
    last_error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT document_verification_job_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT document_verification_job_kind_check CHECK ((kind = ANY (ARRAY['scan'::text, 'retention_delete'::text]))),
    CONSTRAINT document_verification_job_last_error_code_check CHECK (((last_error_code IS NULL) OR (last_error_code = ANY (ARRAY['scanner_unavailable'::text, 'scanner_malformed'::text, 'signatures_stale'::text, 'malware_detected'::text, 'inspection_rejected'::text, 'retention_store_error'::text])))),
    CONSTRAINT document_verification_job_lease_ck CHECK (((state = 'procesando'::text) = (lease_until IS NOT NULL))),
    CONSTRAINT document_verification_job_state_check CHECK ((state = ANY (ARRAY['pendiente'::text, 'procesando'::text, 'reintentar'::text, 'completado'::text])))
);


--
-- Name: document_verification_reviewer_grant; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_reviewer_grant (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reviewer_id uuid NOT NULL,
    capability text NOT NULL,
    scope_kind text NOT NULL,
    scope_id uuid NOT NULL,
    granted_by uuid NOT NULL,
    granted_at timestamp with time zone DEFAULT now() NOT NULL,
    granted_reason_code text CONSTRAINT document_verification_reviewer_gra_granted_reason_code_not_null NOT NULL,
    expires_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revoked_by uuid,
    revocation_reason_code text,
    CONSTRAINT document_verification_reviewer_gra_revocation_reason_code_check CHECK (((revocation_reason_code IS NULL) OR (revocation_reason_code = ANY (ARRAY['role_assignment_ended'::text, 'temporary_coverage_ended'::text, 'reviewer_rotation'::text, 'access_review'::text])))),
    CONSTRAINT document_verification_reviewer_grant_capability_check CHECK ((capability = ANY (ARRAY['review_identity'::text, 'review_space'::text]))),
    CONSTRAINT document_verification_reviewer_grant_expiry_ck CHECK (((expires_at IS NULL) OR (expires_at > granted_at))),
    CONSTRAINT document_verification_reviewer_grant_granted_reason_code_check CHECK ((granted_reason_code = ANY (ARRAY['role_assignment'::text, 'temporary_coverage'::text, 'reviewer_rotation'::text]))),
    CONSTRAINT document_verification_reviewer_grant_revocation_ck CHECK ((((revoked_at IS NULL) AND (revoked_by IS NULL) AND (revocation_reason_code IS NULL)) OR ((revoked_at IS NOT NULL) AND (revoked_by IS NOT NULL) AND (revocation_reason_code IS NOT NULL) AND (revoked_at >= granted_at)))),
    CONSTRAINT document_verification_reviewer_grant_scope_ck CHECK ((((capability = 'review_identity'::text) AND (scope_kind = 'identity_subject'::text)) OR ((capability = 'review_space'::text) AND (scope_kind = 'space'::text)))),
    CONSTRAINT document_verification_reviewer_grant_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['identity_subject'::text, 'space'::text])))
);


--
-- Name: document_verification_reviewer_grant_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_verification_reviewer_grant_event (
    id bigint NOT NULL,
    grant_id uuid NOT NULL,
    reviewer_id uuid NOT NULL,
    capability text NOT NULL,
    scope_kind text NOT NULL,
    scope_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    action text NOT NULL,
    reason_code text NOT NULL,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT document_verification_reviewer_grant_event_action_check CHECK ((action = ANY (ARRAY['granted'::text, 'revoked'::text]))),
    CONSTRAINT document_verification_reviewer_grant_event_capability_check CHECK ((capability = ANY (ARRAY['review_identity'::text, 'review_space'::text]))),
    CONSTRAINT document_verification_reviewer_grant_event_reason_code_check CHECK ((reason_code = ANY (ARRAY['role_assignment'::text, 'temporary_coverage'::text, 'reviewer_rotation'::text, 'role_assignment_ended'::text, 'temporary_coverage_ended'::text, 'access_review'::text]))),
    CONSTRAINT document_verification_reviewer_grant_event_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['identity_subject'::text, 'space'::text])))
);


--
-- Name: document_verification_reviewer_grant_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.document_verification_reviewer_grant_event ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.document_verification_reviewer_grant_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: documento_privado_sintetico_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documento_privado_sintetico_local (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    mime_type text NOT NULL,
    encryption_algorithm text NOT NULL,
    sha256 bytea NOT NULL,
    size_bytes integer NOT NULL,
    contenido bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT documento_privado_sintetico_local_encryption_algorithm_check CHECK ((encryption_algorithm = 'aes-256-gcm'::text)),
    CONSTRAINT documento_privado_sintetico_local_mime_type_check CHECK ((mime_type = 'application/pdf'::text)),
    CONSTRAINT documento_privado_sintetico_local_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT documento_privado_sintetico_local_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 2097152)))
);


--
-- Name: ejecucion_baja_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ejecucion_baja_local (
    id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    clave_idempotencia text NOT NULL,
    estado text NOT NULL,
    motivo_codigo text NOT NULL,
    iniciada_en timestamp with time zone NOT NULL,
    completada_en timestamp with time zone,
    retirar_en timestamp with time zone,
    detalle jsonb NOT NULL,
    CONSTRAINT ejecucion_baja_local_check CHECK (((estado = 'completada'::text) = (completada_en IS NOT NULL))),
    CONSTRAINT ejecucion_baja_local_check1 CHECK (((estado = 'completada'::text) = (retirar_en IS NOT NULL))),
    CONSTRAINT ejecucion_baja_local_check2 CHECK (((retirar_en IS NULL) OR (retirar_en = (completada_en + '5 years'::interval)))),
    CONSTRAINT ejecucion_baja_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT ejecucion_baja_local_detalle_check CHECK ((jsonb_typeof(detalle) = 'object'::text)),
    CONSTRAINT ejecucion_baja_local_estado_check CHECK ((estado = ANY (ARRAY['bloqueada'::text, 'limpieza_pendiente'::text, 'completada'::text]))),
    CONSTRAINT ejecucion_baja_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['supresion_con_obligaciones'::text, 'baja_local_minimizada'::text])))
);


--
-- Name: elegibilidad_verificacion_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.elegibilidad_verificacion_local (
    usuario_id uuid NOT NULL,
    tipo text NOT NULL,
    verificacion_id uuid NOT NULL,
    estado text NOT NULL,
    concedida_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    revocada_por uuid,
    motivo_revocacion_codigo text,
    CONSTRAINT elegibilidad_revocacion_ck CHECK ((((estado = 'elegible'::text) AND (revocada_en IS NULL) AND (revocada_por IS NULL) AND (motivo_revocacion_codigo IS NULL)) OR ((estado = 'revocada'::text) AND (revocada_en IS NOT NULL) AND (revocada_por IS NOT NULL) AND (motivo_revocacion_codigo = ANY (ARRAY['aprobacion_fixture_incorrecta'::text, 'revision_fixture_actualizada'::text]))) OR ((estado = 'retirada_privacidad'::text) AND (revocada_en IS NOT NULL) AND (revocada_por IS NOT NULL) AND (motivo_revocacion_codigo = 'baja_privacidad'::text)))),
    CONSTRAINT elegibilidad_verificacion_local_estado_check CHECK ((estado = ANY (ARRAY['elegible'::text, 'revocada'::text, 'retirada_privacidad'::text]))),
    CONSTRAINT elegibilidad_verificacion_local_tipo_check CHECK ((tipo = ANY (ARRAY['kyc'::text, 'kyb'::text])))
);


--
-- Name: espacio; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio (
    id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    categoria_codigo text NOT NULL,
    titulo character varying(70) NOT NULL,
    descripcion text NOT NULL,
    superficie_m2 numeric(10,2) NOT NULL,
    capacidad_maxima integer NOT NULL,
    reglas_uso character varying(250) NOT NULL,
    modalidad_tarifa text NOT NULL,
    precio_base_clp bigint NOT NULL,
    direccion text NOT NULL,
    estado text DEFAULT 'borrador'::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    zona_horaria text,
    portada_foto_id uuid,
    ubicacion_catalogo_version text,
    region_codigo text,
    comuna_codigo text,
    busqueda_publica tsvector DEFAULT ''::tsvector NOT NULL,
    portada_media_id uuid,
    ubicacion_latitud double precision,
    ubicacion_longitud double precision,
    ubicacion_confirmada_en timestamp with time zone,
    ubicacion_revision bigint DEFAULT 0 NOT NULL,
    politica_cancelacion_version text,
    ubicacion_pin_origen text,
    ubicacion_pin_precision text,
    ubicacion_visibilidad text DEFAULT 'exact'::text NOT NULL,
    ubicacion_publica_latitud double precision,
    ubicacion_publica_longitud double precision,
    es_b2b boolean DEFAULT false NOT NULL,
    organizacion_id uuid,
    modo_anfitrion character varying(16) DEFAULT 'personal'::character varying NOT NULL,
    kyb_grace_started_at timestamp with time zone,
    kyb_grace_due_at timestamp with time zone,
    CONSTRAINT espacio_b2b_organization_ck CHECK (((es_b2b AND (organizacion_id IS NOT NULL)) OR ((NOT es_b2b) AND (organizacion_id IS NULL)))),
    CONSTRAINT espacio_capacidad_maxima_check CHECK ((capacidad_maxima > 0)),
    CONSTRAINT espacio_commercial_confirmation_ck CHECK (((ubicacion_confirmada_en IS NULL) OR ((ubicacion_latitud IS NOT NULL) AND (ubicacion_longitud IS NOT NULL) AND (btrim(direccion) <> ''::text) AND (region_codigo IS NOT NULL) AND (comuna_codigo IS NOT NULL)))),
    CONSTRAINT espacio_commercial_coordinates_ck CHECK ((((ubicacion_latitud IS NULL) AND (ubicacion_longitud IS NULL)) OR ((ubicacion_latitud IS NOT NULL) AND (ubicacion_longitud IS NOT NULL) AND ((ubicacion_latitud >= ('-90'::integer)::double precision) AND (ubicacion_latitud <= (90)::double precision)) AND ((ubicacion_longitud >= ('-180'::integer)::double precision) AND (ubicacion_longitud <= (180)::double precision))))),
    CONSTRAINT espacio_commercial_revision_ck CHECK ((ubicacion_revision >= 0)),
    CONSTRAINT espacio_direccion_check CHECK (((length(btrim(direccion)) >= 1) AND (length(btrim(direccion)) <= 500))),
    CONSTRAINT espacio_estado_local_publication_ck CHECK ((estado = ANY (ARRAY['borrador'::text, 'activa'::text, 'oculta'::text, 'eliminada'::text]))),
    CONSTRAINT espacio_host_mode_consistency_ck CHECK (((((modo_anfitrion)::text = 'empresa'::text) AND es_b2b AND (organizacion_id IS NOT NULL)) OR (((modo_anfitrion)::text = 'personal'::text) AND (NOT es_b2b) AND (organizacion_id IS NULL)))),
    CONSTRAINT espacio_kyb_grace_dates_ck CHECK (((kyb_grace_due_at IS NULL) OR ((kyb_grace_started_at IS NOT NULL) AND (kyb_grace_due_at > kyb_grace_started_at)))),
    CONSTRAINT espacio_modalidad_tarifa_check CHECK ((modalidad_tarifa = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text]))),
    CONSTRAINT espacio_modo_anfitrion_check CHECK (((modo_anfitrion)::text = ANY ((ARRAY['personal'::character varying, 'empresa'::character varying])::text[]))),
    CONSTRAINT espacio_pin_provenance_ck CHECK ((((ubicacion_pin_origen IS NULL) AND (ubicacion_pin_precision IS NULL)) OR ((ubicacion_latitud IS NOT NULL) AND (ubicacion_longitud IS NOT NULL) AND (ubicacion_pin_origen IS NOT NULL) AND (((ubicacion_pin_origen = 'manual'::text) AND (ubicacion_pin_precision IS NULL)) OR ((ubicacion_pin_origen = 'geocoded'::text) AND (ubicacion_pin_precision IS NOT NULL) AND (ubicacion_pin_precision = ANY (ARRAY['address'::text, 'approximate'::text]))))))),
    CONSTRAINT espacio_politica_cancelacion_version_check CHECK ((politica_cancelacion_version = ANY (ARRAY['facil_v1'::text, 'flexible_v1'::text, 'moderada_v1'::text, 'estricta_v1'::text]))),
    CONSTRAINT espacio_precio_base_clp_min_1000_ck CHECK ((precio_base_clp >= 1000)),
    CONSTRAINT espacio_public_location_all_or_none_ck CHECK ((((ubicacion_catalogo_version IS NULL) AND (region_codigo IS NULL) AND (comuna_codigo IS NULL)) OR ((ubicacion_catalogo_version IS NOT NULL) AND (region_codigo IS NOT NULL) AND (comuna_codigo IS NOT NULL)))),
    CONSTRAINT espacio_reglas_uso_check CHECK (((length(btrim((reglas_uso)::text)) >= 1) AND (length(btrim((reglas_uso)::text)) <= 250))),
    CONSTRAINT espacio_single_cover_ck CHECK (((portada_foto_id IS NULL) OR (portada_media_id IS NULL))),
    CONSTRAINT espacio_superficie_m2_check CHECK ((superficie_m2 > (0)::numeric)),
    CONSTRAINT espacio_titulo_check CHECK (((length(btrim((titulo)::text)) >= 1) AND (length(btrim((titulo)::text)) <= 70))),
    CONSTRAINT espacio_ubicacion_publica_coords_ck CHECK ((((ubicacion_publica_latitud IS NULL) AND (ubicacion_publica_longitud IS NULL)) OR ((ubicacion_publica_latitud IS NOT NULL) AND (ubicacion_publica_longitud IS NOT NULL) AND ((ubicacion_publica_latitud >= ('-90'::integer)::double precision) AND (ubicacion_publica_latitud <= (90)::double precision)) AND ((ubicacion_publica_longitud >= ('-180'::integer)::double precision) AND (ubicacion_publica_longitud <= (180)::double precision))))),
    CONSTRAINT espacio_ubicacion_visibilidad_ck CHECK ((ubicacion_visibilidad = ANY (ARRAY['exact'::text, 'approximate'::text]))),
    CONSTRAINT espacio_zona_horaria_ck CHECK (((zona_horaria IS NULL) OR ((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100))))
);


--
-- Name: COLUMN espacio.portada_foto_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.espacio.portada_foto_id IS 'Selección de portada sintética propia; NULL cuando no hay selección vigente.';


--
-- Name: COLUMN espacio.portada_media_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.espacio.portada_media_id IS 'Selección de portada real propia; mutuamente excluyente con portada_foto_id.';


--
-- Name: espacio_calendario_override; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_calendario_override (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    espacio_id uuid NOT NULL,
    fecha date NOT NULL,
    estado text NOT NULL,
    tarifa_personalizada_clp integer,
    motivo text,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_calendario_override_estado_check CHECK ((estado = ANY (ARRAY['disponible'::text, 'bloqueado'::text, 'mantenimiento'::text]))),
    CONSTRAINT espacio_calendario_override_tarifa_personalizada_clp_check CHECK ((tarifa_personalizada_clp > 0))
);


--
-- Name: TABLE espacio_calendario_override; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.espacio_calendario_override IS 'Excepciones y tarifas personalizadas por fecha y espacio para el calendario.';


--
-- Name: espacio_caracteristicas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_caracteristicas (
    espacio_id uuid NOT NULL,
    categoria_codigo text NOT NULL,
    perfil_version integer NOT NULL,
    valores jsonb DEFAULT '{}'::jsonb NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_caracteristicas_valores_check CHECK (((jsonb_typeof(valores) = 'object'::text) AND (octet_length((valores)::text) <= 16384)))
);


--
-- Name: espacio_contrato_borrador; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_contrato_borrador (
    espacio_id uuid NOT NULL,
    revision bigint NOT NULL,
    secciones jsonb NOT NULL,
    version_publicada bigint DEFAULT 0 NOT NULL,
    actualizada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_contrato_borrador_revision_check CHECK ((revision > 0)),
    CONSTRAINT espacio_contrato_borrador_secciones_check CHECK (((jsonb_typeof(secciones) = 'array'::text) AND ((jsonb_array_length(secciones) >= 1) AND (jsonb_array_length(secciones) <= 12)))),
    CONSTRAINT espacio_contrato_borrador_version_publicada_check CHECK ((version_publicada >= 0))
);


--
-- Name: espacio_contrato_version; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_contrato_version (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    version bigint NOT NULL,
    revision_origen bigint NOT NULL,
    texto_canonico text NOT NULL,
    secciones jsonb NOT NULL,
    sha256 bytea NOT NULL,
    publicada_por uuid NOT NULL,
    publicada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_contrato_version_check CHECK ((secciones = (texto_canonico)::jsonb)),
    CONSTRAINT espacio_contrato_version_check1 CHECK ((sha256 = sha256(convert_to(texto_canonico, 'UTF8'::name)))),
    CONSTRAINT espacio_contrato_version_revision_origen_check CHECK ((revision_origen > 0)),
    CONSTRAINT espacio_contrato_version_secciones_check CHECK (((jsonb_typeof(secciones) = 'array'::text) AND ((jsonb_array_length(secciones) >= 1) AND (jsonb_array_length(secciones) <= 12)))),
    CONSTRAINT espacio_contrato_version_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT espacio_contrato_version_texto_canonico_check CHECK (((octet_length(texto_canonico) >= 2) AND (octet_length(texto_canonico) <= 160000))),
    CONSTRAINT espacio_contrato_version_version_check CHECK ((version > 0))
);


--
-- Name: TABLE espacio_contrato_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.espacio_contrato_version IS 'Immutable commercial clause versions. New requests select the current published version and retain their own immutable agreement snapshot.';


--
-- Name: espacio_eliminacion_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_eliminacion_historial_local (
    id bigint NOT NULL,
    espacio_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    archivos_pendientes integer CONSTRAINT espacio_eliminacion_historial_loca_archivos_pendientes_not_null NOT NULL,
    resumen jsonb NOT NULL,
    retirar_en timestamp with time zone,
    CONSTRAINT espacio_eliminacion_historial_local_archivos_pendientes_check CHECK ((archivos_pendientes >= 0)),
    CONSTRAINT espacio_eliminacion_historial_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT espacio_eliminacion_historial_local_resumen_check CHECK (((jsonb_typeof(resumen) = 'object'::text) AND (resumen ? 'removed'::text) AND (resumen ? 'preserved'::text)))
);


--
-- Name: espacio_eliminacion_historial_local_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.espacio_eliminacion_historial_local ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.espacio_eliminacion_historial_local_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: espacio_galeria_archivo_candidato_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_galeria_archivo_candidato_local (
    archivo_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    proximo_intento_en timestamp with time zone,
    intentos_limpieza integer DEFAULT 0 CONSTRAINT espacio_galeria_archivo_candidato_lo_intentos_limpieza_not_null NOT NULL,
    ultimo_codigo_error text,
    CONSTRAINT espacio_galeria_archivo_candidato_local_estado_check CHECK ((estado = ANY (ARRAY['reservado'::text, 'pendiente_limpieza'::text, 'limpiando'::text]))),
    CONSTRAINT espacio_galeria_archivo_candidato_local_intentos_limpieza_check CHECK ((intentos_limpieza >= 0)),
    CONSTRAINT espacio_galeria_candidato_state_ck CHECK ((((estado = 'reservado'::text) AND (proximo_intento_en IS NULL)) OR ((estado = ANY (ARRAY['pendiente_limpieza'::text, 'limpiando'::text])) AND (proximo_intento_en IS NOT NULL))))
);


--
-- Name: espacio_galeria_sintetica_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_galeria_sintetica_local (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    archivo_id uuid,
    fixture_code text NOT NULL,
    mime_type text NOT NULL,
    sha256 character(64) NOT NULL,
    size_bytes bigint NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirada_en timestamp with time zone,
    proximo_intento_en timestamp with time zone,
    limpia_en timestamp with time zone,
    intentos_limpieza integer DEFAULT 0 NOT NULL,
    ultimo_codigo_error text,
    clave_idempotencia text NOT NULL,
    publicada_en_catalogo boolean DEFAULT false NOT NULL,
    CONSTRAINT espacio_galeria_sintetica_cleanup_ck CHECK ((((estado = 'activa'::text) AND (proximo_intento_en IS NULL)) OR ((estado <> 'activa'::text) AND ((archivo_id IS NULL) OR (proximo_intento_en IS NOT NULL))))),
    CONSTRAINT espacio_galeria_sintetica_file_ck CHECK (((archivo_id IS NULL) = (limpia_en IS NOT NULL))),
    CONSTRAINT espacio_galeria_sintetica_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 128))),
    CONSTRAINT espacio_galeria_sintetica_local_estado_check CHECK ((estado = ANY (ARRAY['activa'::text, 'retirada'::text, 'retirada_baja'::text]))),
    CONSTRAINT espacio_galeria_sintetica_local_fixture_code_check CHECK ((fixture_code = 'synthetic-png-v1'::text)),
    CONSTRAINT espacio_galeria_sintetica_local_intentos_limpieza_check CHECK ((intentos_limpieza >= 0)),
    CONSTRAINT espacio_galeria_sintetica_local_mime_type_check CHECK ((mime_type = 'image/png'::text)),
    CONSTRAINT espacio_galeria_sintetica_local_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT espacio_galeria_sintetica_local_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 1048576))),
    CONSTRAINT espacio_galeria_sintetica_state_ck CHECK ((((estado = 'activa'::text) AND (retirada_en IS NULL)) OR ((estado = ANY (ARRAY['retirada'::text, 'retirada_baja'::text])) AND (retirada_en IS NOT NULL))))
);


--
-- Name: espacio_horario_semanal; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_horario_semanal (
    espacio_id uuid NOT NULL,
    activo boolean DEFAULT false NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: espacio_horario_semanal_tramo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_horario_semanal_tramo (
    espacio_id uuid NOT NULL,
    dia_iso smallint NOT NULL,
    orden smallint NOT NULL,
    apertura_minuto smallint NOT NULL,
    cierre_minuto smallint NOT NULL,
    CONSTRAINT espacio_horario_semanal_tramo_apertura_minuto_check CHECK (((apertura_minuto >= 0) AND (apertura_minuto <= 1439))),
    CONSTRAINT espacio_horario_semanal_tramo_check CHECK ((cierre_minuto > apertura_minuto)),
    CONSTRAINT espacio_horario_semanal_tramo_cierre_minuto_check CHECK (((cierre_minuto >= 1) AND (cierre_minuto <= 1440))),
    CONSTRAINT espacio_horario_semanal_tramo_dia_iso_check CHECK (((dia_iso >= 1) AND (dia_iso <= 7))),
    CONSTRAINT espacio_horario_semanal_tramo_orden_check CHECK (((orden >= 1) AND (orden <= 2)))
);


--
-- Name: espacio_publicacion_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_publicacion_historial_local (
    id bigint NOT NULL,
    espacio_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    estado_anterior text NOT NULL,
    estado_nuevo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    correlacion_id text NOT NULL,
    retirar_en timestamp with time zone,
    CONSTRAINT espacio_publicacion_historial_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT espacio_publicacion_historial_local_estado_anterior_check CHECK ((estado_anterior = ANY (ARRAY['borrador'::text, 'activa'::text, 'oculta'::text]))),
    CONSTRAINT espacio_publicacion_historial_local_estado_nuevo_check CHECK ((estado_nuevo = ANY (ARRAY['activa'::text, 'oculta'::text]))),
    CONSTRAINT espacio_publicacion_transicion_ck CHECK ((((estado_anterior = ANY (ARRAY['borrador'::text, 'oculta'::text])) AND (estado_nuevo = 'activa'::text)) OR ((estado_anterior = 'activa'::text) AND (estado_nuevo = 'oculta'::text))))
);


--
-- Name: espacio_publicacion_historial_local_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.espacio_publicacion_historial_local ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.espacio_publicacion_historial_local_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: espacio_regla_disponibilidad; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_regla_disponibilidad (
    espacio_id uuid NOT NULL,
    dias_operacion smallint[] DEFAULT '{1,2,3,4,5,6,7}'::smallint[] NOT NULL,
    hora_apertura time without time zone DEFAULT '09:00:00'::time without time zone NOT NULL,
    hora_cierre time without time zone DEFAULT '20:00:00'::time without time zone NOT NULL,
    check_in_hora time without time zone DEFAULT '15:00:00'::time without time zone,
    check_out_hora time without time zone DEFAULT '12:00:00'::time without time zone,
    tarifa_base_dia integer NOT NULL,
    estadia_minima smallint DEFAULT 1 NOT NULL,
    estadia_maxima smallint DEFAULT 30 NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_regla_disponibilidad_tarifa_base_dia_check CHECK ((tarifa_base_dia > 0))
);


--
-- Name: TABLE espacio_regla_disponibilidad; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.espacio_regla_disponibilidad IS 'Reglas base de disponibilidad, horario de operacion y tarifas diarias por espacio.';


--
-- Name: espacio_regla_mensual; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_regla_mensual (
    espacio_id uuid NOT NULL,
    disponible_desde date,
    estadia_minima_meses smallint DEFAULT 1 NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT espacio_regla_mensual_estadia_minima_meses_check CHECK (((estadia_minima_meses >= 1) AND (estadia_minima_meses <= 24)))
);


--
-- Name: espacio_solicitud_documental; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.espacio_solicitud_documental (
    espacio_id uuid NOT NULL,
    actor_usuario_id uuid NOT NULL,
    titular_usuario_id uuid,
    organizacion_id uuid,
    solicitado_en timestamp with time zone NOT NULL,
    vence_en timestamp with time zone NOT NULL,
    version_politica text NOT NULL,
    documentos_titular text[] NOT NULL,
    documento_espacio text NOT NULL,
    CONSTRAINT espacio_solicitud_documental_check CHECK ((num_nonnulls(titular_usuario_id, organizacion_id) = 1)),
    CONSTRAINT espacio_solicitud_documental_check1 CHECK ((((titular_usuario_id IS NOT NULL) AND (documentos_titular = ARRAY['sii_start_activities'::text])) OR ((organizacion_id IS NOT NULL) AND (documentos_titular = ARRAY['erut'::text, 'sii_start_activities'::text, 'power_certificate'::text])))),
    CONSTRAINT espacio_solicitud_documental_documento_espacio_check CHECK ((documento_espacio = 'property_or_sublease_authorization'::text)),
    CONSTRAINT espacio_solicitud_documental_version_politica_check CHECK ((version_politica = 'documentacion-comercial-v1'::text))
);


--
-- Name: evento_auditoria_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.evento_auditoria_local (
    id uuid NOT NULL,
    actor_id uuid NOT NULL,
    recurso_tipo text NOT NULL,
    recurso_id uuid NOT NULL,
    accion text NOT NULL,
    resultado text NOT NULL,
    motivo_codigo text NOT NULL,
    correlacion_id text NOT NULL,
    ocurrido_en timestamp with time zone NOT NULL,
    retirar_en timestamp with time zone NOT NULL,
    clave_idempotencia text,
    detalle_codigos jsonb DEFAULT '{"pending_checks": [], "obligations_detected": []}'::jsonb NOT NULL,
    cuenta_objetivo_id uuid,
    registrado_en timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT evento_auditoria_codigos_ck CHECK (((jsonb_typeof(detalle_codigos) = 'object'::text) AND (detalle_codigos ? 'obligations_detected'::text) AND (detalle_codigos ? 'pending_checks'::text) AND ((detalle_codigos - ARRAY['obligations_detected'::text, 'pending_checks'::text]) = '{}'::jsonb) AND (jsonb_typeof((detalle_codigos -> 'obligations_detected'::text)) = 'array'::text) AND (jsonb_typeof((detalle_codigos -> 'pending_checks'::text)) = 'array'::text) AND ((detalle_codigos -> 'obligations_detected'::text) <@ '["reserva_activa", "pago_o_devolucion_pendiente", "disputa_abierta", "liquidacion_pendiente"]'::jsonb) AND ((detalle_codigos -> 'pending_checks'::text) <@ '["fuente_disputas_no_modelada", "matriz_retencion_historicos_incompleta"]'::jsonb))),
    CONSTRAINT evento_auditoria_five_years_ck CHECK ((retirar_en = (ocurrido_en + '5 years'::interval))),
    CONSTRAINT evento_auditoria_idempotencia_ck CHECK (((clave_idempotencia IS NULL) OR ((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200) AND (((accion = ANY (ARRAY['privacy.suppression.review'::text, 'privacy.suppression.execute'::text])) AND (recurso_tipo = 'solicitud_titular'::text)) OR ((accion = 'identity.credential_notice.reopen'::text) AND (recurso_tipo = 'outbox_evento'::text)) OR ((accion = 'reputation.review.moderate'::text) AND (recurso_tipo = 'reporte_resena'::text)) OR ((accion = 'local.damage_claim.resolve'::text) AND (recurso_tipo = 'reclamo_dano'::text)) OR ((accion = ANY (ARRAY['local.finance.decision'::text, 'local.finance.guarantee.operation'::text, 'local.finance.guarantee.reconcile'::text, 'local.finance.settlement'::text])) AND (recurso_tipo = 'reserva'::text)))))),
    CONSTRAINT evento_auditoria_local_accion_check CHECK (((length(btrim(accion)) >= 1) AND (length(btrim(accion)) <= 60))),
    CONSTRAINT evento_auditoria_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT evento_auditoria_local_motivo_codigo_check CHECK (((length(btrim(motivo_codigo)) >= 1) AND (length(btrim(motivo_codigo)) <= 60))),
    CONSTRAINT evento_auditoria_local_recurso_tipo_check CHECK (((length(btrim(recurso_tipo)) >= 1) AND (length(btrim(recurso_tipo)) <= 40))),
    CONSTRAINT evento_auditoria_local_resultado_check CHECK ((resultado = ANY (ARRAY['exito'::text, 'rechazo'::text]))),
    CONSTRAINT evento_auditoria_reapertura_motivo_ck CHECK (((accion <> 'identity.credential_notice.reopen'::text) OR ((motivo_codigo = ANY (ARRAY['smtp_restaurado'::text, 'reintento_operativo'::text])) AND (resultado = 'exito'::text)))),
    CONSTRAINT evento_auditoria_retencion_ck CHECK ((retirar_en > ocurrido_en))
);


--
-- Name: COLUMN evento_auditoria_local.cuenta_objetivo_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.evento_auditoria_local.cuenta_objetivo_id IS 'Cuenta objetivo explícita para auditoría GADM; distinta del actor. NULL en eventos históricos o globales.';


--
-- Name: COLUMN evento_auditoria_local.registrado_en; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.evento_auditoria_local.registrado_en IS 'Instante de inserción para describir el corte de exportaciones de auditoría; ocurrido_en sigue siendo el tiempo del evento.';


--
-- Name: evento_caso_soporte_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.evento_caso_soporte_comercial (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    caso_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    accion text NOT NULL,
    descripcion text NOT NULL,
    clave_idempotencia text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT evento_caso_soporte_comercial_accion_check CHECK ((accion = ANY (ARRAY['abrir'::text, 'responder'::text, 'escalar'::text, 'descargo'::text, 'resolver'::text, 'cerrar'::text]))),
    CONSTRAINT evento_caso_soporte_comercial_clave_idempotencia_check CHECK (((char_length(clave_idempotencia) >= 1) AND (char_length(clave_idempotencia) <= 128))),
    CONSTRAINT evento_caso_soporte_comercial_descripcion_check CHECK ((char_length(descripcion) <= 3000))
);


--
-- Name: evidencia_caso_soporte_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.evidencia_caso_soporte_comercial (
    id uuid NOT NULL,
    caso_id uuid NOT NULL,
    autor_id uuid NOT NULL,
    mime text NOT NULL,
    tamano_bytes integer NOT NULL,
    sha256 bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT evidencia_caso_soporte_comercial_mime_check CHECK ((mime = ANY (ARRAY['image/jpeg'::text, 'image/png'::text, 'application/pdf'::text]))),
    CONSTRAINT evidencia_caso_soporte_comercial_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT evidencia_caso_soporte_comercial_tamano_bytes_check CHECK (((tamano_bytes >= 1) AND (tamano_bytes <= 5242880)))
);


--
-- Name: foto_perfil_sintetica_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.foto_perfil_sintetica_local (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    archivo_id uuid,
    mime_type text,
    fixture_code text NOT NULL,
    sha256 text,
    size_bytes bigint,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirada_en timestamp with time zone,
    limpia_en timestamp with time zone,
    intentos_limpieza integer DEFAULT 0 NOT NULL,
    proximo_intento_en timestamp with time zone,
    ultimo_codigo_error text,
    clave_idempotencia text NOT NULL,
    CONSTRAINT foto_perfil_sintetica_archivo_ck CHECK ((((archivo_id IS NULL) AND (mime_type IS NULL) AND (sha256 IS NULL) AND (size_bytes IS NULL) AND (limpia_en IS NOT NULL)) OR ((archivo_id IS NOT NULL) AND (mime_type = 'image/png'::text) AND (sha256 ~ '^[0-9a-f]{64}$'::text) AND ((size_bytes >= 1) AND (size_bytes <= 4096)) AND (limpia_en IS NULL)))),
    CONSTRAINT foto_perfil_sintetica_idempotencia_ck CHECK (((length(clave_idempotencia) >= 8) AND (length(clave_idempotencia) <= 128))),
    CONSTRAINT foto_perfil_sintetica_local_estado_check CHECK ((estado = ANY (ARRAY['activa'::text, 'reemplazada'::text, 'retirada'::text, 'retirada_baja'::text]))),
    CONSTRAINT foto_perfil_sintetica_local_fixture_code_check CHECK ((fixture_code = 'synthetic-png-v1'::text)),
    CONSTRAINT foto_perfil_sintetica_local_intentos_limpieza_check CHECK ((intentos_limpieza >= 0)),
    CONSTRAINT foto_perfil_sintetica_retirada_ck CHECK ((((estado = 'activa'::text) AND (retirada_en IS NULL)) OR ((estado <> 'activa'::text) AND (retirada_en IS NOT NULL))))
);


--
-- Name: gallery_item_order; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gallery_item_order (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    synthetic_photo_id uuid,
    space_media_id uuid,
    "position" smallint NOT NULL,
    CONSTRAINT gallery_item_order_position_check CHECK ((("position" >= 1) AND ("position" <= 30))),
    CONSTRAINT gallery_order_exactly_one_ck CHECK (((synthetic_photo_id IS NULL) <> (space_media_id IS NULL)))
);


--
-- Name: gcp06_test_access_batch_audit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gcp06_test_access_batch_audit (
    id uuid NOT NULL,
    target_count integer NOT NULL,
    target_set_sha256 character(64) NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    CONSTRAINT gcp06_test_access_batch_audit_target_count_check CHECK (((target_count >= 1) AND (target_count <= 10000))),
    CONSTRAINT gcp06_test_access_batch_audit_target_set_sha256_check CHECK ((target_set_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: TABLE gcp06_test_access_batch_audit; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.gcp06_test_access_batch_audit IS 'Aggregate-only append-only evidence for an explicitly confirmed synthetic GCP-06 test-account credential batch; contains no emails, password material, or account UUIDs.';


--
-- Name: historial_clave_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.historial_clave_local (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    hash_clave text NOT NULL,
    dejo_de_ser_vigente_en timestamp with time zone NOT NULL,
    retirar_en timestamp with time zone NOT NULL,
    creado_en timestamp with time zone NOT NULL,
    CONSTRAINT historial_clave_local_hash_clave_check CHECK (((length(hash_clave) >= 1) AND (length(hash_clave) <= 200))),
    CONSTRAINT historial_clave_ventana_ck CHECK ((retirar_en > dejo_de_ser_vigente_en))
);


--
-- Name: host_compliance_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_compliance_event (
    id bigint NOT NULL,
    usuario_id uuid,
    organizacion_id uuid,
    espacio_id uuid,
    actor_usuario_id uuid,
    event_type character varying(32) NOT NULL,
    reason character varying(500),
    reminder_stage character varying(24),
    occurred_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT host_compliance_event_check CHECK ((num_nonnulls(usuario_id, organizacion_id, espacio_id) = 1)),
    CONSTRAINT host_compliance_event_event_type_check CHECK (((event_type)::text = ANY ((ARRAY['grace_started'::character varying, 'reminder_sent'::character varying, 'disabled_auto'::character varying, 'disabled_manual'::character varying, 're_enabled'::character varying, 'member_changed'::character varying, 'personal_sii_submitted'::character varying, 'space_property_submitted'::character varying, 'personal_sii_reviewed'::character varying, 'space_reviewed'::character varying, 'space_created'::character varying, 'space_edited'::character varying])::text[])))
);


--
-- Name: host_compliance_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_compliance_event ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.host_compliance_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: identity_declaration_revision; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_declaration_revision (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    revision bigint NOT NULL,
    key_version character varying(64) NOT NULL,
    ciphertext bytea NOT NULL,
    schema_version character varying(32) DEFAULT '1'::character varying NOT NULL,
    encryption_algorithm character varying(16) DEFAULT 'AES-256-GCM/v1'::character varying NOT NULL,
    consent_version character varying(32) NOT NULL,
    consent_given_at timestamp with time zone NOT NULL,
    retention_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT identity_declaration_revision_check CHECK ((retention_at > created_at)),
    CONSTRAINT identity_declaration_revision_check1 CHECK ((consent_given_at <= created_at)),
    CONSTRAINT identity_declaration_revision_ciphertext_check CHECK (((octet_length(ciphertext) >= 29) AND (octet_length(ciphertext) <= 65564))),
    CONSTRAINT identity_declaration_revision_consent_version_check CHECK (((length((consent_version)::text) >= 1) AND (length((consent_version)::text) <= 32))),
    CONSTRAINT identity_declaration_revision_encryption_algorithm_check CHECK (((encryption_algorithm)::text = 'AES-256-GCM/v1'::text)),
    CONSTRAINT identity_declaration_revision_key_version_check CHECK (((key_version)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'::text)),
    CONSTRAINT identity_declaration_revision_revision_check CHECK ((revision > 0)),
    CONSTRAINT identity_declaration_revision_schema_version_check CHECK (((schema_version)::text = '1'::text))
);


--
-- Name: identity_provider_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_provider_event (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    attempt_id uuid NOT NULL,
    provider character varying(16) NOT NULL,
    environment character varying(8) NOT NULL,
    application_id character varying(128) NOT NULL,
    event_id character varying(160) NOT NULL,
    event_type character varying(32) NOT NULL,
    body_sha256 bytea NOT NULL,
    processing_status character varying(16) DEFAULT 'RECEIVED'::character varying NOT NULL,
    received_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    processed_at timestamp with time zone,
    retention_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_provider_event_body_sha256_check CHECK ((octet_length(body_sha256) = 32)),
    CONSTRAINT identity_provider_event_check CHECK (((((processing_status)::text = 'RECEIVED'::text) AND (processed_at IS NULL)) OR (((processing_status)::text = ANY (ARRAY[('PROCESSED'::character varying)::text, ('FAILED'::character varying)::text, ('IGNORED'::character varying)::text])) AND (processed_at IS NOT NULL)))),
    CONSTRAINT identity_provider_event_check1 CHECK (((processed_at IS NULL) OR (processed_at >= received_at))),
    CONSTRAINT identity_provider_event_check2 CHECK ((retention_at > received_at)),
    CONSTRAINT identity_provider_event_environment_check CHECK (((environment)::text = ANY (ARRAY[('SANDBOX'::character varying)::text, ('LIVE'::character varying)::text]))),
    CONSTRAINT identity_provider_event_event_id_check CHECK (((length((event_id)::text) >= 1) AND (length((event_id)::text) <= 160))),
    CONSTRAINT identity_provider_event_event_type_check CHECK (((event_type)::text = ANY (ARRAY[('status.updated'::character varying)::text, ('data.updated'::character varying)::text, ('user.status.updated'::character varying)::text]))),
    CONSTRAINT identity_provider_event_processing_status_check CHECK (((processing_status)::text = ANY (ARRAY[('RECEIVED'::character varying)::text, ('PROCESSED'::character varying)::text, ('FAILED'::character varying)::text, ('IGNORED'::character varying)::text]))),
    CONSTRAINT identity_provider_event_provider_check CHECK (((provider)::text = 'didit'::text))
);


--
-- Name: identity_rut_claim; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_rut_claim (
    fingerprint text NOT NULL,
    account_id uuid NOT NULL,
    declaration_id uuid,
    expires_at timestamp with time zone,
    verified_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_rut_claim_declaration_ck CHECK (((verified_at IS NOT NULL) OR (declaration_id IS NOT NULL))),
    CONSTRAINT identity_rut_claim_fingerprint_ck CHECK ((fingerprint ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text)),
    CONSTRAINT identity_rut_claim_state_ck CHECK ((((verified_at IS NULL) AND (expires_at IS NOT NULL)) OR ((verified_at IS NOT NULL) AND (expires_at IS NULL)))),
    CONSTRAINT identity_rut_claim_times_ck CHECK (((updated_at >= created_at) AND ((verified_at IS NULL) OR (verified_at >= created_at))))
);


--
-- Name: identity_verification_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_verification_attempts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    status character varying(30) DEFAULT 'CREATED'::character varying NOT NULL,
    id_country character varying(3) DEFAULT 'CHL'::character varying NOT NULL,
    document_type character varying(20) DEFAULT 'ID'::character varying NOT NULL,
    declared_first_name character varying(150) NOT NULL,
    declared_last_name character varying(150) NOT NULL,
    declared_date_of_birth date NOT NULL,
    declared_rut_normalized character varying(20) NOT NULL,
    didit_session_id character varying(100),
    didit_session_url text,
    didit_workflow_id character varying(100),
    environment character varying(20) DEFAULT 'SANDBOX'::character varying NOT NULL,
    consent_given_at timestamp with time zone NOT NULL,
    consent_version character varying(20) NOT NULL,
    decision_status character varying(50),
    decision_warnings jsonb,
    extracted_data jsonb,
    mismatch_flags jsonb,
    verified_at timestamp with time zone,
    rejection_reason text,
    review_notes text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT identity_verification_attempts_status_check CHECK (((status)::text = ANY (ARRAY[('CREATED'::character varying)::text, ('PENDING_USER_INPUT'::character varying)::text, ('PROCESSING'::character varying)::text, ('APPROVED'::character varying)::text, ('IN_REVIEW'::character varying)::text, ('REJECTED'::character varying)::text, ('EXPIRED'::character varying)::text, ('FAILED'::character varying)::text])))
);


--
-- Name: TABLE identity_verification_attempts; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.identity_verification_attempts IS 'Didit identity-verification attempt and declared-identity snapshot; no captured media is stored.';


--
-- Name: kyb_compliance_notice_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kyb_compliance_notice_outbox (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    profile_type character varying(16) NOT NULL,
    profile_id uuid NOT NULL,
    recipient_id uuid NOT NULL,
    reminder_stage character varying(24) NOT NULL,
    source_transition_id uuid,
    available_at timestamp with time zone NOT NULL,
    state character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    attempts smallint DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    delivered_at timestamp with time zone,
    terminal_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    last_error_code character varying(32),
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kyb_compliance_notice_outbox_attempts_check CHECK (((attempts >= 0) AND (attempts <= 8))),
    CONSTRAINT kyb_compliance_notice_outbox_check CHECK (((((reminder_stage)::text = 'reactivated'::text) AND (source_transition_id IS NOT NULL)) OR (((reminder_stage)::text <> 'reactivated'::text) AND (source_transition_id IS NULL)))),
    CONSTRAINT kyb_compliance_notice_outbox_check1 CHECK ((created_at <= available_at)),
    CONSTRAINT kyb_compliance_notice_outbox_check2 CHECK ((((state)::text = 'processing'::text) = (claimed_at IS NOT NULL))),
    CONSTRAINT kyb_compliance_notice_outbox_check3 CHECK ((((state)::text = 'delivered'::text) = (delivered_at IS NOT NULL))),
    CONSTRAINT kyb_compliance_notice_outbox_check4 CHECK ((((state)::text = 'terminal'::text) = (terminal_at IS NOT NULL))),
    CONSTRAINT kyb_compliance_notice_outbox_check5 CHECK ((((state)::text = 'cancelled'::text) = (cancelled_at IS NOT NULL))),
    CONSTRAINT kyb_compliance_notice_outbox_check6 CHECK ((num_nonnulls(delivered_at, terminal_at, cancelled_at) <= 1)),
    CONSTRAINT kyb_compliance_notice_outbox_last_error_code_check CHECK (((last_error_code IS NULL) OR ((last_error_code)::text = ANY ((ARRAY['recipient_unavailable'::character varying, 'mail_delivery_failed'::character varying, 'attempt_limit_reached'::character varying])::text[])))),
    CONSTRAINT kyb_compliance_notice_outbox_profile_type_check CHECK (((profile_type)::text = ANY ((ARRAY['personal'::character varying, 'organization'::character varying])::text[]))),
    CONSTRAINT kyb_compliance_notice_outbox_reminder_stage_check CHECK (((reminder_stage)::text = ANY ((ARRAY['day0'::character varying, 'month1'::character varying, 'day15'::character varying, 'day7'::character varying, 'day1'::character varying, 'due'::character varying, 'reactivated'::character varying])::text[]))),
    CONSTRAINT kyb_compliance_notice_outbox_state_check CHECK (((state)::text = ANY ((ARRAY['pending'::character varying, 'processing'::character varying, 'delivered'::character varying, 'terminal'::character varying, 'cancelled'::character varying])::text[])))
);


--
-- Name: kyb_rut_claim; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kyb_rut_claim (
    rut_kind character varying(20) NOT NULL,
    fingerprint character varying(97) NOT NULL,
    verification_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kyb_rut_claim_fingerprint_check CHECK (((fingerprint)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text)),
    CONSTRAINT kyb_rut_claim_rut_kind_check CHECK (((rut_kind)::text = ANY ((ARRAY['company'::character varying, 'representative'::character varying])::text[])))
);


--
-- Name: kyb_verification_document; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kyb_verification_document (
    id uuid NOT NULL,
    verification_id uuid NOT NULL,
    document_kind character varying(40) NOT NULL,
    storage_ref uuid NOT NULL,
    mime_type character varying(32) NOT NULL,
    size_bytes bigint NOT NULL,
    content_sha256 bytea NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    review_status character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    rejection_reason character varying(500),
    reviewer_id uuid,
    reviewed_at timestamp with time zone,
    CONSTRAINT kyb_verification_document_content_sha256_check CHECK ((octet_length(content_sha256) = 32)),
    CONSTRAINT kyb_verification_document_document_kind_check CHECK (((document_kind)::text = ANY ((ARRAY['erut'::character varying, 'sii_start_activities'::character varying, 'power_certificate'::character varying, 'property_or_sublease_authorization'::character varying])::text[]))),
    CONSTRAINT kyb_verification_document_mime_type_check CHECK (((mime_type)::text = ANY ((ARRAY['application/pdf'::character varying, 'image/jpeg'::character varying, 'image/png'::character varying])::text[]))),
    CONSTRAINT kyb_verification_document_rejection_reason_ck CHECK (((((review_status)::text = 'REJECTED'::text) AND (rejection_reason IS NOT NULL) AND ((length(btrim((rejection_reason)::text)) >= 5) AND (length(btrim((rejection_reason)::text)) <= 500))) OR (((review_status)::text <> 'REJECTED'::text) AND (rejection_reason IS NULL)))),
    CONSTRAINT kyb_verification_document_review_consistency_ck CHECK (((((review_status)::text = ANY ((ARRAY['APPROVED'::character varying, 'REJECTED'::character varying])::text[])) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((review_status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[])) AND (reviewer_id IS NULL) AND (reviewed_at IS NULL)))),
    CONSTRAINT kyb_verification_document_review_status_check CHECK (((review_status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[]))),
    CONSTRAINT kyb_verification_document_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760)))
);


--
-- Name: kyb_verification_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kyb_verification_event (
    id bigint NOT NULL,
    verification_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    action character varying(24) NOT NULL,
    from_status character varying(16),
    to_status character varying(16) NOT NULL,
    reason character varying(500),
    occurred_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kyb_verification_event_action_check CHECK (((action)::text = ANY ((ARRAY['submitted'::character varying, 'review_started'::character varying, 'approved'::character varying, 'rejected'::character varying])::text[]))),
    CONSTRAINT kyb_verification_event_from_status_check CHECK (((from_status IS NULL) OR ((from_status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))),
    CONSTRAINT kyb_verification_event_to_status_check CHECK (((to_status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))
);


--
-- Name: kyb_verification_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.kyb_verification_event ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.kyb_verification_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: kyb_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kyb_verifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    submitted_by uuid NOT NULL,
    idempotency_key character varying(120) NOT NULL,
    legal_name character varying(200) NOT NULL,
    company_rut_fingerprint character varying(97) NOT NULL,
    company_rut_key_version character varying(64) NOT NULL,
    company_rut_ciphertext bytea NOT NULL,
    representative_name character varying(200) NOT NULL,
    representative_rut_fingerprint character varying(97) NOT NULL,
    representative_rut_key_version character varying(64) NOT NULL,
    representative_rut_ciphertext bytea NOT NULL,
    sii_giro_code character varying(6) NOT NULL,
    status character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    rejection_reason character varying(500),
    submitted_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    reviewed_at timestamp with time zone,
    reviewer_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kyb_verifications_check CHECK (((updated_at >= created_at) AND (submitted_at >= created_at))),
    CONSTRAINT kyb_verifications_check1 CHECK (((((status)::text = 'REJECTED'::text) AND (rejection_reason IS NOT NULL) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text <> 'REJECTED'::text) AND (rejection_reason IS NULL)))),
    CONSTRAINT kyb_verifications_check2 CHECK (((((status)::text = ANY ((ARRAY['APPROVED'::character varying, 'REJECTED'::character varying])::text[])) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[])) AND (reviewer_id IS NULL) AND (reviewed_at IS NULL)))),
    CONSTRAINT kyb_verifications_company_rut_ciphertext_check CHECK (((octet_length(company_rut_ciphertext) >= 29) AND (octet_length(company_rut_ciphertext) <= 65564))),
    CONSTRAINT kyb_verifications_company_rut_fingerprint_check CHECK (((company_rut_fingerprint)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text)),
    CONSTRAINT kyb_verifications_company_rut_key_version_check CHECK (((company_rut_key_version)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'::text)),
    CONSTRAINT kyb_verifications_idempotency_key_check CHECK (((length(btrim((idempotency_key)::text)) >= 1) AND (length(btrim((idempotency_key)::text)) <= 120))),
    CONSTRAINT kyb_verifications_legal_name_check CHECK (((length(btrim((legal_name)::text)) >= 2) AND (length(btrim((legal_name)::text)) <= 200))),
    CONSTRAINT kyb_verifications_representative_name_check CHECK (((length(btrim((representative_name)::text)) >= 2) AND (length(btrim((representative_name)::text)) <= 200))),
    CONSTRAINT kyb_verifications_representative_rut_ciphertext_check CHECK (((octet_length(representative_rut_ciphertext) >= 29) AND (octet_length(representative_rut_ciphertext) <= 65564))),
    CONSTRAINT kyb_verifications_representative_rut_fingerprint_check CHECK (((representative_rut_fingerprint)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text)),
    CONSTRAINT kyb_verifications_representative_rut_key_version_check CHECK (((representative_rut_key_version)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'::text)),
    CONSTRAINT kyb_verifications_sii_giro_code_check CHECK (((sii_giro_code)::text ~ '^[0-9]{6}$'::text)),
    CONSTRAINT kyb_verifications_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))
);


--
-- Name: m02_operacion_idempotente_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.m02_operacion_idempotente_local (
    usuario_id uuid NOT NULL,
    recurso text NOT NULL,
    clave_idempotencia text NOT NULL,
    solicitud_sha256 text NOT NULL,
    respuesta jsonb NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT m02_operacion_idempotente_local_clave_idempotencia_check CHECK (((length(clave_idempotencia) >= 8) AND (length(clave_idempotencia) <= 128))),
    CONSTRAINT m02_operacion_idempotente_local_recurso_check CHECK ((recurso = ANY (ARRAY['foto_perfil'::text, 'cuenta_cobro'::text]))),
    CONSTRAINT m02_operacion_idempotente_local_solicitud_sha256_check CHECK ((solicitud_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: media_asset; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media_asset (
    id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    espacio_id uuid,
    proposito text NOT NULL,
    estado text NOT NULL,
    idempotency_key text NOT NULL,
    input_sha256 character(64) NOT NULL,
    fencing_token bigint DEFAULT 1 NOT NULL,
    lease_until timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    replacement_target_space_media_id uuid,
    replacement_expected_revision timestamp with time zone,
    CONSTRAINT media_asset_estado_check CHECK ((estado = ANY (ARRAY['pending'::text, 'ready'::text, 'deleting'::text, 'deleted'::text, 'failed'::text]))),
    CONSTRAINT media_asset_fencing_token_check CHECK ((fencing_token > 0)),
    CONSTRAINT media_asset_idempotency_key_check CHECK (((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 128))),
    CONSTRAINT media_asset_input_sha256_check CHECK ((input_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT media_asset_proposito_check CHECK ((proposito = ANY (ARRAY['espacio'::text, 'perfil'::text]))),
    CONSTRAINT media_asset_purpose_scope_ck CHECK ((((proposito = 'espacio'::text) AND (espacio_id IS NOT NULL)) OR ((proposito = 'perfil'::text) AND (espacio_id IS NULL)))),
    CONSTRAINT media_asset_terminal_ck CHECK ((((estado = 'deleted'::text) AND (deleted_at IS NOT NULL)) OR ((estado <> 'deleted'::text) AND (deleted_at IS NULL))))
);


--
-- Name: TABLE media_asset; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.media_asset IS 'IMG-01 private media lifecycle; the privacy cleanup worker can only claim and finalize deleting assets.';


--
-- Name: media_variant; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media_variant (
    media_id uuid NOT NULL,
    variant text NOT NULL,
    provider text NOT NULL,
    bucket_name text NOT NULL,
    object_key text NOT NULL,
    generation bigint NOT NULL,
    mime_type text NOT NULL,
    size_bytes bigint NOT NULL,
    sha256 character(64) NOT NULL,
    width integer NOT NULL,
    height integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT media_variant_bucket_name_check CHECK (((length(btrim(bucket_name)) >= 1) AND (length(btrim(bucket_name)) <= 222))),
    CONSTRAINT media_variant_dims_ck CHECK ((((width)::bigint * (height)::bigint) <= 25000000)),
    CONSTRAINT media_variant_generation_check CHECK ((generation > 0)),
    CONSTRAINT media_variant_height_check CHECK (((height >= 1) AND (height <= 1600))),
    CONSTRAINT media_variant_mime_type_check CHECK ((mime_type = 'image/webp'::text)),
    CONSTRAINT media_variant_object_key_check CHECK (((length(btrim(object_key)) >= 1) AND (length(btrim(object_key)) <= 1024))),
    CONSTRAINT media_variant_provider_check CHECK ((provider = ANY (ARRAY['gcs'::text, 'filesystem'::text, 's3'::text]))),
    CONSTRAINT media_variant_purpose_ck CHECK ((((variant = 'avatar'::text) AND (width <= 512) AND (height <= 512)) OR ((variant = 'thumbnail'::text) AND (width <= 480) AND (height <= 480)) OR ((variant = 'display'::text) AND (width <= 1600) AND (height <= 1600)))),
    CONSTRAINT media_variant_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT media_variant_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760))),
    CONSTRAINT media_variant_variant_check CHECK ((variant = ANY (ARRAY['display'::text, 'thumbnail'::text, 'avatar'::text]))),
    CONSTRAINT media_variant_width_check CHECK (((width >= 1) AND (width <= 1600)))
);


--
-- Name: media_variant_intent; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.media_variant_intent (
    media_id uuid NOT NULL,
    variant text NOT NULL,
    object_key text NOT NULL,
    provider text,
    bucket_name text,
    generation bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    write_complete boolean DEFAULT false NOT NULL,
    CONSTRAINT media_variant_intent_generation_check CHECK (((generation IS NULL) OR (generation > 0))),
    CONSTRAINT media_variant_intent_object_key_check CHECK (((length(object_key) >= 1) AND (length(object_key) <= 1024) AND ((object_key ~~ 'media/v1/%'::text) OR (object_key ~ '^users/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/spaces/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/images/v2/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/[0-9]{8}T[0-9]{6}[.][0-9]{9}Z_[1-9][0-9]*_(display|thumbnail)[.]webp$'::text)))),
    CONSTRAINT media_variant_intent_provider_check CHECK ((provider = ANY (ARRAY['gcs'::text, 'filesystem'::text, 's3'::text]))),
    CONSTRAINT media_variant_intent_storage_ck CHECK ((((provider IS NULL) AND (bucket_name IS NULL) AND (generation IS NULL)) OR ((provider IS NOT NULL) AND (bucket_name IS NOT NULL) AND (generation IS NOT NULL)))),
    CONSTRAINT media_variant_intent_variant_check CHECK ((variant = ANY (ARRAY['display'::text, 'thumbnail'::text, 'avatar'::text])))
);


--
-- Name: mensaje_reserva_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mensaje_reserva_comercial (
    secuencia bigint NOT NULL,
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    autor_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    cuerpo text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT mensaje_reserva_comercial_body_ck CHECK ((((char_length(cuerpo) >= 1) AND (char_length(cuerpo) <= 2000)) AND (length(btrim(cuerpo)) > 0))),
    CONSTRAINT mensaje_reserva_comercial_hash_ck CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT mensaje_reserva_comercial_key_ck CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200)))
);


--
-- Name: mensaje_reserva_comercial_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mensaje_reserva_comercial ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.mensaje_reserva_comercial_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mensaje_reserva_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mensaje_reserva_ensayo (
    secuencia bigint NOT NULL,
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    autor_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    cuerpo text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT mensaje_reserva_ensayo_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT mensaje_reserva_ensayo_cuerpo_check CHECK (((char_length(cuerpo) >= 1) AND (char_length(cuerpo) <= 2000) AND (length(btrim(cuerpo)) > 0))),
    CONSTRAINT mensaje_reserva_ensayo_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32))
);


--
-- Name: mensaje_reserva_ensayo_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mensaje_reserva_ensayo ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.mensaje_reserva_ensayo_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mercadopago_oauth_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mercadopago_oauth_state (
    state_hash bytea NOT NULL,
    anfitrion_id uuid NOT NULL,
    encrypted_code_verifier bytea NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    consumida_en timestamp with time zone,
    CONSTRAINT mercadopago_oauth_state_consumed_ck CHECK (((consumida_en IS NULL) OR (consumida_en >= creada_en))),
    CONSTRAINT mercadopago_oauth_state_hash_ck CHECK ((octet_length(state_hash) = 32)),
    CONSTRAINT mercadopago_oauth_state_lifetime_ck CHECK (((expira_en > creada_en) AND (expira_en <= (creada_en + '00:15:00'::interval)))),
    CONSTRAINT mercadopago_oauth_state_verifier_ck CHECK ((octet_length(encrypted_code_verifier) > 28))
);


--
-- Name: mercadopago_payment_webhook_receipt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mercadopago_payment_webhook_receipt (
    id uuid NOT NULL,
    topic text NOT NULL,
    action text DEFAULT ''::text NOT NULL,
    provider_payment_id text CONSTRAINT mercadopago_payment_webhook_receip_provider_payment_id_not_null NOT NULL,
    provider_seller_hint text,
    provider_request_id text CONSTRAINT mercadopago_payment_webhook_receip_provider_request_id_not_null NOT NULL,
    signature_timestamp text CONSTRAINT mercadopago_payment_webhook_receip_signature_timestamp_not_null NOT NULL,
    estado text DEFAULT 'received'::text NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    reintentar_en timestamp with time zone NOT NULL,
    claimed_at timestamp with time zone,
    last_error_code text,
    provider_status text,
    recibida_en timestamp with time zone NOT NULL,
    procesada_en timestamp with time zone,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT mercadopago_payment_webhook_attempts_ck CHECK (((intentos >= 0) AND (intentos <= 1000000))),
    CONSTRAINT mercadopago_payment_webhook_error_ck CHECK (((last_error_code IS NULL) OR (last_error_code = ANY (ARRAY['seller_hint_missing'::text, 'provider_identity_mismatch'::text, 'payment_requires_reconciliation'::text, 'seller_token_unavailable'::text, 'provider_lookup_failed'::text, 'payment_outcome_unavailable'::text])))),
    CONSTRAINT mercadopago_payment_webhook_id_ck CHECK ((provider_payment_id ~ '^[0-9]{1,64}$'::text)),
    CONSTRAINT mercadopago_payment_webhook_processed_ck CHECK (((estado <> 'processed'::text) OR (procesada_en IS NOT NULL))),
    CONSTRAINT mercadopago_payment_webhook_provider_status_ck CHECK (((provider_status IS NULL) OR (provider_status ~ '^[a-z_]{1,40}$'::text))),
    CONSTRAINT mercadopago_payment_webhook_request_ck CHECK (((char_length(provider_request_id) >= 1) AND (char_length(provider_request_id) <= 256))),
    CONSTRAINT mercadopago_payment_webhook_seller_ck CHECK (((provider_seller_hint IS NULL) OR (provider_seller_hint ~ '^[0-9]{1,64}$'::text))),
    CONSTRAINT mercadopago_payment_webhook_state_ck CHECK ((estado = ANY (ARRAY['received'::text, 'processing'::text, 'retry'::text, 'processed'::text, 'reconciliation_required'::text]))),
    CONSTRAINT mercadopago_payment_webhook_timestamp_ck CHECK ((signature_timestamp ~ '^[0-9]{1,20}$'::text)),
    CONSTRAINT mercadopago_payment_webhook_topic_ck CHECK ((topic = 'payment'::text))
);


--
-- Name: TABLE mercadopago_payment_webhook_receipt; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.mercadopago_payment_webhook_receipt IS 'Minimal durable receipt for signed Mercado Pago payment notification delivery and idempotent reconciliation; excludes raw payloads and credentials.';


--
-- Name: mercadopago_seller_connection; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mercadopago_seller_connection (
    anfitrion_id uuid NOT NULL,
    seller_id text NOT NULL,
    encrypted_access_token bytea NOT NULL,
    encrypted_refresh_token bytea NOT NULL,
    token_expires_at timestamp with time zone NOT NULL,
    scopes text[] DEFAULT '{}'::text[] NOT NULL,
    live_mode boolean NOT NULL,
    connection_state text DEFAULT 'active'::text NOT NULL,
    connected_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    public_key text,
    CONSTRAINT mercadopago_seller_connection_access_ck CHECK ((octet_length(encrypted_access_token) > 28)),
    CONSTRAINT mercadopago_seller_connection_public_key_ck CHECK (((public_key IS NULL) OR (public_key ~ '^(TEST-|APP_USR-)[A-Za-z0-9_-]{16,120}$'::text))),
    CONSTRAINT mercadopago_seller_connection_refresh_ck CHECK ((octet_length(encrypted_refresh_token) > 28)),
    CONSTRAINT mercadopago_seller_connection_seller_ck CHECK ((seller_id ~ '^[0-9]+$'::text)),
    CONSTRAINT mercadopago_seller_connection_state_ck CHECK ((connection_state = ANY (ARRAY['active'::text, 'reauthorization_required'::text])))
);


--
-- Name: notificacion_usuario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notificacion_usuario (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    tipo text NOT NULL,
    titulo text NOT NULL,
    mensaje text NOT NULL,
    href text NOT NULL,
    referencia_id uuid,
    leida_en timestamp with time zone,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notificacion_usuario_href_check CHECK (((char_length(href) >= 1) AND (char_length(href) <= 300))),
    CONSTRAINT notificacion_usuario_mensaje_check CHECK (((char_length(mensaje) >= 1) AND (char_length(mensaje) <= 500))),
    CONSTRAINT notificacion_usuario_tipo_check CHECK (((char_length(tipo) >= 1) AND (char_length(tipo) <= 80))),
    CONSTRAINT notificacion_usuario_titulo_check CHECK (((char_length(titulo) >= 1) AND (char_length(titulo) <= 160)))
);


--
-- Name: TABLE notificacion_usuario; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.notificacion_usuario IS 'Notificaciones in-app por cuenta para solicitudes de contrato, firmas y eventos de arriendo.';


--
-- Name: ocupacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ocupacion (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    reserva_id uuid,
    intervalo tstzrange NOT NULL,
    tipo text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    expira_en timestamp with time zone,
    motivo text,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    desactivada_en timestamp with time zone,
    arriendo_temprano_id uuid,
    reserva_arriendo_comercial_id uuid,
    solicitud_comercial_id uuid,
    CONSTRAINT ocupacion_desactivacion_ck CHECK (((activo AND (desactivada_en IS NULL)) OR ((NOT activo) AND (desactivada_en IS NOT NULL)))),
    CONSTRAINT ocupacion_intervalo_ck CHECK (((NOT isempty(intervalo)) AND (NOT lower_inf(intervalo)) AND (NOT upper_inf(intervalo)) AND lower_inc(intervalo) AND (NOT upper_inc(intervalo)) AND (lower(intervalo) < upper(intervalo)))),
    CONSTRAINT ocupacion_reserva_comercial_expira_ck CHECK (((tipo <> ALL (ARRAY['retencion_comercial'::text, 'reserva_comercial'::text])) OR ((tipo = 'retencion_comercial'::text) AND (expira_en IS NOT NULL) AND isfinite(expira_en) AND (expira_en <= lower(intervalo))) OR ((tipo = 'reserva_comercial'::text) AND (expira_en IS NULL)))),
    CONSTRAINT ocupacion_temprana_tipo_expiracion_ck CHECK (((tipo <> ALL (ARRAY['retencion_temprana'::text, 'reserva_temprana'::text])) OR ((tipo = 'retencion_temprana'::text) AND (expira_en IS NOT NULL) AND isfinite(expira_en) AND (expira_en <= lower(intervalo))) OR ((tipo = 'reserva_temprana'::text) AND (expira_en IS NULL)))),
    CONSTRAINT ocupacion_tipo_check CHECK ((tipo = ANY (ARRAY['bloqueo_manual'::text, 'retencion'::text, 'reserva'::text, 'retencion_temprana'::text, 'reserva_temprana'::text, 'retencion_comercial'::text, 'reserva_comercial'::text, 'retencion_contrato_comercial'::text]))),
    CONSTRAINT ocupacion_tipo_reserva_ck CHECK ((((tipo = 'bloqueo_manual'::text) AND (reserva_id IS NULL) AND (arriendo_temprano_id IS NULL) AND (reserva_arriendo_comercial_id IS NULL) AND (solicitud_comercial_id IS NULL) AND (expira_en IS NULL) AND (motivo IS NOT NULL) AND ((length(btrim(motivo)) >= 1) AND (length(btrim(motivo)) <= 500))) OR ((tipo = ANY (ARRAY['retencion'::text, 'reserva'::text])) AND (reserva_id IS NOT NULL) AND (arriendo_temprano_id IS NULL) AND (reserva_arriendo_comercial_id IS NULL) AND (solicitud_comercial_id IS NULL) AND ((tipo <> 'retencion'::text) OR (expira_en IS NOT NULL))) OR ((tipo = ANY (ARRAY['retencion_temprana'::text, 'reserva_temprana'::text])) AND (reserva_id IS NULL) AND (arriendo_temprano_id IS NOT NULL) AND (reserva_arriendo_comercial_id IS NULL) AND (solicitud_comercial_id IS NULL) AND (motivo IS NULL) AND ((tipo <> 'retencion_temprana'::text) OR (expira_en IS NOT NULL))) OR ((tipo = ANY (ARRAY['retencion_comercial'::text, 'reserva_comercial'::text])) AND (reserva_id IS NULL) AND (arriendo_temprano_id IS NULL) AND (reserva_arriendo_comercial_id IS NOT NULL) AND (motivo IS NULL) AND ((tipo <> 'retencion_comercial'::text) OR (expira_en IS NOT NULL))) OR ((tipo = 'retencion_contrato_comercial'::text) AND (reserva_id IS NULL) AND (arriendo_temprano_id IS NULL) AND (reserva_arriendo_comercial_id IS NULL) AND (solicitud_comercial_id IS NOT NULL) AND (motivo IS NULL) AND (expira_en IS NOT NULL) AND isfinite(expira_en) AND (expira_en <= lower(intervalo)))))
);


--
-- Name: operacion_arriendo_archivo_candidato_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operacion_arriendo_archivo_candidato_local (
    archivo_id uuid NOT NULL,
    reserva_id uuid,
    actor_id uuid,
    tipo text NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    proximo_intento_en timestamp with time zone CONSTRAINT operacion_arriendo_archivo_candidat_proximo_intento_en_not_null NOT NULL,
    lease_iniciado_en timestamp with time zone,
    intentos_limpieza integer DEFAULT 0 CONSTRAINT operacion_arriendo_archivo_candidato_intentos_limpieza_not_null NOT NULL,
    ultimo_codigo_error text,
    CONSTRAINT operacion_arriendo_archivo_candidato_lo_intentos_limpieza_check CHECK ((intentos_limpieza >= 0)),
    CONSTRAINT operacion_arriendo_archivo_candidato_local_estado_check CHECK ((estado = ANY (ARRAY['reservado'::text, 'pendiente_limpieza'::text, 'limpiando'::text]))),
    CONSTRAINT operacion_arriendo_archivo_candidato_local_tipo_check CHECK ((tipo = ANY (ARRAY['checkin'::text, 'checkout'::text, 'recepcion'::text, 'descargo'::text])))
);


--
-- Name: operacion_arriendo_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operacion_arriendo_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid,
    tipo text NOT NULL,
    actor_id uuid,
    ocurrio_en timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    ubicacion_sintetica jsonb NOT NULL,
    comentarios text DEFAULT ''::text NOT NULL,
    observacion text DEFAULT ''::text NOT NULL,
    resultado text NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT operacion_arriendo_ensayo_local_campos_tipo_check CHECK ((((tipo = ANY (ARRAY['checkin'::text, 'checkout'::text])) OR (comentarios = ''::text)) AND ((tipo = 'recepcion'::text) OR (observacion = ''::text)))),
    CONSTRAINT operacion_arriendo_ensayo_local_check1 CHECK ((((tipo = ANY (ARRAY['checkin'::text, 'checkout'::text])) OR (comentarios = ''::text)) AND ((tipo = 'recepcion'::text) OR (observacion = ''::text)))),
    CONSTRAINT operacion_arriendo_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT operacion_arriendo_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT operacion_arriendo_ensayo_local_resultado_check CHECK ((resultado = ANY (ARRAY['registrada'::text, 'recepcion_conforme'::text, 'recepcion_con_observaciones'::text]))),
    CONSTRAINT operacion_arriendo_ensayo_local_resultado_tipo_check CHECK ((((tipo = 'recepcion'::text) AND (resultado = ANY (ARRAY['recepcion_conforme'::text, 'recepcion_con_observaciones'::text]))) OR ((tipo = ANY (ARRAY['checkin'::text, 'checkout'::text, 'descargo'::text])) AND (resultado = 'registrada'::text)))),
    CONSTRAINT operacion_arriendo_ensayo_local_tipo_check CHECK ((tipo = ANY (ARRAY['checkin'::text, 'checkout'::text, 'recepcion'::text, 'descargo'::text]))),
    CONSTRAINT operacion_arriendo_ensayo_local_ubicacion_sintetica_check CHECK (((jsonb_typeof(ubicacion_sintetica) = 'object'::text) AND ((ubicacion_sintetica ->> 'source'::text) = 'synthetic-fixture-v1'::text) AND ((ubicacion_sintetica ->> 'location_code'::text) = 'santiago-demo-center-v1'::text) AND ((((ubicacion_sintetica ->> 'latitude'::text))::double precision >= ('-90'::integer)::double precision) AND (((ubicacion_sintetica ->> 'latitude'::text))::double precision <= (90)::double precision)) AND ((((ubicacion_sintetica ->> 'longitude'::text))::double precision >= ('-180'::integer)::double precision) AND (((ubicacion_sintetica ->> 'longitude'::text))::double precision <= (180)::double precision)))),
    CONSTRAINT operacion_arriendo_ensayo_local_zona_horaria_check CHECK (((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100)))
);


--
-- Name: operacion_arriendo_evidencia_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operacion_arriendo_evidencia_ensayo_local (
    id uuid NOT NULL,
    operacion_id uuid NOT NULL,
    fixture_code text NOT NULL,
    mime_type text NOT NULL,
    sha256 character(64) NOT NULL,
    size_bytes bigint NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirar_archivo_en timestamp with time zone,
    archivo_retirado_en timestamp with time zone,
    CONSTRAINT operacion_arriendo_evidencia_ensayo_local_fixture_code_check CHECK ((fixture_code = 'synthetic-png-v1'::text)),
    CONSTRAINT operacion_arriendo_evidencia_ensayo_local_mime_type_check CHECK ((mime_type = 'image/png'::text)),
    CONSTRAINT operacion_arriendo_evidencia_ensayo_local_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT operacion_arriendo_evidencia_ensayo_local_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 1048576)))
);


--
-- Name: COLUMN operacion_arriendo_evidencia_ensayo_local.retirar_archivo_en; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.operacion_arriendo_evidencia_ensayo_local.retirar_archivo_en IS 'Plazo derivado de la retención ratificada de reservas sintéticas: 24 meses desde el último cierre relacionado.';


--
-- Name: COLUMN operacion_arriendo_evidencia_ensayo_local.archivo_retirado_en; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.operacion_arriendo_evidencia_ensayo_local.archivo_retirado_en IS 'Marca mínima de limpieza durable del blob; metadatos/historial pueden permanecer minimizados.';


--
-- Name: operacion_arriendo_historial_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operacion_arriendo_historial_ensayo_local (
    secuencia bigint NOT NULL,
    reserva_id uuid,
    operacion_id uuid NOT NULL,
    accion text NOT NULL,
    actor_id uuid,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT operacion_arriendo_historial_ensayo_local_accion_check CHECK ((accion = ANY (ARRAY['checkin_registrado'::text, 'checkout_registrado'::text, 'recepcion_confirmada'::text, 'observacion_registrada'::text, 'descargo_evidencia'::text])))
);


--
-- Name: operacion_arriendo_historial_ensayo_local_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.operacion_arriendo_historial_ensayo_local ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.operacion_arriendo_historial_ensayo_local_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: organizacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    usuario_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    created_by_usuario_id uuid NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    company_rut_fingerprint character varying(97),
    sii_giro_code character varying(6),
    legal_name character varying(200),
    trade_name character varying(200),
    public_summary character varying(2000),
    logo_media_id uuid,
    commune_code character varying(32),
    region_code character varying(32),
    compliance_status character varying(32) DEFAULT 'grace_active'::character varying NOT NULL,
    grace_started_at timestamp with time zone,
    grace_due_at timestamp with time zone,
    disabled_at timestamp with time zone,
    disabled_reason character varying(500),
    disabled_by uuid,
    last_reminder_stage character varying(24),
    CONSTRAINT organizacion_compliance_status_check CHECK (((compliance_status)::text = ANY ((ARRAY['grace_active'::character varying, 'in_review'::character varying, 'compliant'::character varying, 'disabled_missing_docs'::character varying])::text[]))),
    CONSTRAINT organizacion_disabled_state_ck CHECK (((((compliance_status)::text = 'disabled_missing_docs'::text) AND (disabled_at IS NOT NULL) AND (disabled_reason IS NOT NULL)) OR (((compliance_status)::text <> 'disabled_missing_docs'::text) AND (disabled_at IS NULL)))),
    CONSTRAINT organizacion_grace_dates_ck CHECK (((grace_due_at IS NULL) OR ((grace_started_at IS NOT NULL) AND (grace_due_at > grace_started_at)))),
    CONSTRAINT organizacion_rut_fingerprint_ck CHECK (((company_rut_fingerprint IS NULL) OR ((company_rut_fingerprint)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text))),
    CONSTRAINT organizacion_sii_giro_code_check CHECK (((sii_giro_code IS NULL) OR ((sii_giro_code)::text ~ '^[0-9]{6}$'::text)))
);


--
-- Name: organizacion_invitacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizacion_invitacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organizacion_id uuid NOT NULL,
    tipo character varying(24) NOT NULL,
    target_email_normalized character varying(320),
    target_usuario_id uuid,
    rol_propuesto character varying(32) NOT NULL,
    token_sha256 bytea,
    estado character varying(16) DEFAULT 'pendiente'::character varying NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_by uuid NOT NULL,
    resolved_by uuid,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT organizacion_invitacion_check CHECK (((target_email_normalized IS NOT NULL) OR (target_usuario_id IS NOT NULL))),
    CONSTRAINT organizacion_invitacion_check1 CHECK (((((estado)::text = 'pendiente'::text) AND (resolved_at IS NULL)) OR (((estado)::text <> 'pendiente'::text) AND (resolved_at IS NOT NULL)))),
    CONSTRAINT organizacion_invitacion_estado_check CHECK (((estado)::text = ANY ((ARRAY['pendiente'::character varying, 'aceptada'::character varying, 'rechazada'::character varying, 'expirada'::character varying, 'revocada'::character varying])::text[]))),
    CONSTRAINT organizacion_invitacion_rol_propuesto_check CHECK (((rol_propuesto)::text = ANY ((ARRAY['representante_legal'::character varying, 'administrador'::character varying, 'operador'::character varying])::text[]))),
    CONSTRAINT organizacion_invitacion_target_email_normalized_check CHECK (((target_email_normalized IS NULL) OR ((target_email_normalized)::text = lower(btrim((target_email_normalized)::text))))),
    CONSTRAINT organizacion_invitacion_tipo_check CHECK (((tipo)::text = ANY ((ARRAY['invitacion'::character varying, 'solicitud_adhesion'::character varying])::text[]))),
    CONSTRAINT organizacion_invitacion_token_sha256_check CHECK (((token_sha256 IS NULL) OR (octet_length(token_sha256) = 32)))
);


--
-- Name: organizacion_miembro; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizacion_miembro (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organizacion_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    rol character varying(32) NOT NULL,
    estado character varying(16) DEFAULT 'activo'::character varying NOT NULL,
    invited_by uuid,
    joined_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    removed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT organizacion_miembro_check CHECK (((((estado)::text = 'retirado'::text) AND (removed_at IS NOT NULL)) OR (((estado)::text <> 'retirado'::text) AND (removed_at IS NULL)))),
    CONSTRAINT organizacion_miembro_estado_check CHECK (((estado)::text = ANY ((ARRAY['activo'::character varying, 'suspendido'::character varying, 'retirado'::character varying])::text[]))),
    CONSTRAINT organizacion_miembro_rol_check CHECK (((rol)::text = ANY ((ARRAY['representante_legal'::character varying, 'administrador'::character varying, 'operador'::character varying])::text[])))
);


--
-- Name: organizacion_rut_fingerprint; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizacion_rut_fingerprint (
    organizacion_id uuid NOT NULL,
    fingerprint character varying(97) NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT organizacion_rut_fingerprint_fingerprint_check CHECK (((fingerprint)::text ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}:[0-9a-f]{64}$'::text))
);


--
-- Name: outbox_evento_ciclo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_evento_ciclo_local (
    evento_id uuid NOT NULL,
    numero_ciclo integer NOT NULL,
    estado text NOT NULL,
    iniciada_en timestamp with time zone NOT NULL,
    finalizada_en timestamp with time zone,
    intentos integer DEFAULT 0 NOT NULL,
    codigo_resultado text,
    actor_reapertura_id uuid,
    motivo_reapertura_codigo text,
    correlacion_id text,
    clave_idempotencia text,
    CONSTRAINT outbox_evento_ciclo_local_check CHECK (((estado = 'pendiente'::text) = (finalizada_en IS NULL))),
    CONSTRAINT outbox_evento_ciclo_local_check1 CHECK ((((actor_reapertura_id IS NULL) AND (motivo_reapertura_codigo IS NULL) AND (correlacion_id IS NULL) AND (clave_idempotencia IS NULL)) OR ((actor_reapertura_id IS NOT NULL) AND (motivo_reapertura_codigo IS NOT NULL) AND (correlacion_id IS NOT NULL) AND (clave_idempotencia IS NOT NULL)))),
    CONSTRAINT outbox_evento_ciclo_local_check2 CHECK (((finalizada_en IS NULL) OR (finalizada_en >= iniciada_en))),
    CONSTRAINT outbox_evento_ciclo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'entregada'::text, 'fallo_terminal'::text, 'cancelada'::text]))),
    CONSTRAINT outbox_evento_ciclo_local_intentos_check CHECK (((intentos >= 0) AND (intentos <= 8))),
    CONSTRAINT outbox_evento_ciclo_local_motivo_reapertura_codigo_check CHECK (((motivo_reapertura_codigo IS NULL) OR (motivo_reapertura_codigo = ANY (ARRAY['smtp_restaurado'::text, 'reintento_operativo'::text])))),
    CONSTRAINT outbox_evento_ciclo_local_numero_ciclo_check CHECK ((numero_ciclo > 0))
);


--
-- Name: outbox_evento_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_evento_local (
    id uuid NOT NULL,
    agregado_tipo text NOT NULL,
    agregado_id uuid NOT NULL,
    tipo text NOT NULL,
    clave_deduplicacion text NOT NULL,
    version smallint NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    disponible_en timestamp with time zone NOT NULL,
    lease_hasta timestamp with time zone,
    intentos integer DEFAULT 0 NOT NULL,
    entregada_en timestamp with time zone,
    ultimo_error text,
    cancelada_en timestamp with time zone,
    motivo_cancelacion_codigo text,
    retirar_en timestamp with time zone,
    ciclo_actual integer DEFAULT 1 NOT NULL,
    intentos_ciclo integer DEFAULT 0 NOT NULL,
    fallo_terminal_en timestamp with time zone,
    codigo_fallo_terminal text,
    CONSTRAINT outbox_cancelacion_ck CHECK ((((cancelada_en IS NULL) AND (motivo_cancelacion_codigo IS NULL)) OR ((cancelada_en IS NOT NULL) AND (motivo_cancelacion_codigo = ANY (ARRAY['baja_local_sin_finalidad'::text, 'destinatario_inactivo'::text]))))),
    CONSTRAINT outbox_evento_entrega_ck CHECK (((entregada_en IS NULL) OR (entregada_en >= creada_en))),
    CONSTRAINT outbox_evento_lease_ck CHECK (((lease_hasta IS NULL) OR (lease_hasta >= creada_en))),
    CONSTRAINT outbox_evento_local_agregado_tipo_check CHECK (((length(btrim(agregado_tipo)) >= 1) AND (length(btrim(agregado_tipo)) <= 40))),
    CONSTRAINT outbox_evento_local_ciclo_actual_check CHECK ((ciclo_actual > 0)),
    CONSTRAINT outbox_evento_local_clave_deduplicacion_check CHECK (((length(btrim(clave_deduplicacion)) >= 1) AND (length(btrim(clave_deduplicacion)) <= 120))),
    CONSTRAINT outbox_evento_local_intentos_check CHECK ((intentos >= 0)),
    CONSTRAINT outbox_evento_local_intentos_ciclo_check CHECK (((intentos_ciclo >= 0) AND (intentos_ciclo <= 8))),
    CONSTRAINT outbox_evento_local_tipo_check CHECK ((tipo ~ '^[a-z][a-z0-9_.-]{2,100}$'::text)),
    CONSTRAINT outbox_evento_local_version_check CHECK ((version > 0)),
    CONSTRAINT outbox_terminal_failure_ck CHECK ((((fallo_terminal_en IS NULL) AND (codigo_fallo_terminal IS NULL)) OR ((fallo_terminal_en IS NOT NULL) AND (codigo_fallo_terminal IS NOT NULL) AND (entregada_en IS NULL) AND (cancelada_en IS NULL) AND (lease_hasta IS NULL) AND (retirar_en = (fallo_terminal_en + '30 days'::interval)))))
);


--
-- Name: perfil_usuario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.perfil_usuario (
    usuario_id uuid NOT NULL,
    nombre_visible text NOT NULL,
    telefono_normalizado text,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT perfil_usuario_nombre_ck CHECK (((length(btrim(nombre_visible)) >= 1) AND (length(btrim(nombre_visible)) <= 120) AND (nombre_visible ~ '^[[:alpha:] ]+$'::text))),
    CONSTRAINT perfil_usuario_telefono_ck CHECK (((telefono_normalizado IS NULL) OR (telefono_normalizado ~ '^[0-9]{9}$'::text)))
);


--
-- Name: privacidad_bloqueo_tratamiento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_bloqueo_tratamiento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    cuenta_id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    ambito text NOT NULL,
    estado text DEFAULT 'activo'::text NOT NULL,
    motivo text NOT NULL,
    aplicado_por uuid NOT NULL,
    aplicado_en timestamp with time zone DEFAULT now() NOT NULL,
    revisar_en timestamp with time zone,
    levantado_por uuid,
    levantado_en timestamp with time zone,
    motivo_levantamiento text,
    CONSTRAINT privacidad_bloqueo_tratamiento_ambito_check CHECK ((ambito = ANY (ARRAY['comunicaciones_comerciales'::text, 'analitica_y_perfilado'::text, 'publicaciones_y_visibilidad'::text, 'exportaciones_y_terceros'::text, 'tratamiento_general'::text]))),
    CONSTRAINT privacidad_bloqueo_tratamiento_check CHECK ((((estado = 'activo'::text) AND (levantado_por IS NULL) AND (levantado_en IS NULL) AND (motivo_levantamiento IS NULL)) OR ((estado = 'levantado'::text) AND (levantado_por IS NOT NULL) AND (levantado_en IS NOT NULL) AND (motivo_levantamiento IS NOT NULL)))),
    CONSTRAINT privacidad_bloqueo_tratamiento_estado_check CHECK ((estado = ANY (ARRAY['activo'::text, 'levantado'::text]))),
    CONSTRAINT privacidad_bloqueo_tratamiento_motivo_check CHECK (((char_length(btrim(motivo)) >= 10) AND (char_length(btrim(motivo)) <= 1000))),
    CONSTRAINT privacidad_bloqueo_tratamiento_motivo_levantamiento_check CHECK (((motivo_levantamiento IS NULL) OR ((char_length(btrim(motivo_levantamiento)) >= 10) AND (char_length(btrim(motivo_levantamiento)) <= 1000))))
);


--
-- Name: privacidad_ejecucion_derecho; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_ejecucion_derecho (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    solicitud_id uuid NOT NULL,
    cuenta_id uuid,
    tipo_ejecucion text NOT NULL,
    estado text DEFAULT 'ejecutada'::text NOT NULL,
    ambito text NOT NULL,
    resumen_operativo text NOT NULL,
    destino_referencia text,
    sha256_artefacto text,
    ejecutado_por uuid NOT NULL,
    ejecutado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT privacidad_ejecucion_derecho_ambito_check CHECK (((char_length(btrim(ambito)) >= 2) AND (char_length(btrim(ambito)) <= 120))),
    CONSTRAINT privacidad_ejecucion_derecho_destino_referencia_check CHECK (((destino_referencia IS NULL) OR ((char_length(btrim(destino_referencia)) >= 2) AND (char_length(btrim(destino_referencia)) <= 200)))),
    CONSTRAINT privacidad_ejecucion_derecho_estado_check CHECK ((estado = ANY (ARRAY['ejecutada'::text, 'parcial'::text, 'denegada'::text]))),
    CONSTRAINT privacidad_ejecucion_derecho_resumen_operativo_check CHECK (((char_length(btrim(resumen_operativo)) >= 10) AND (char_length(btrim(resumen_operativo)) <= 1000))),
    CONSTRAINT privacidad_ejecucion_derecho_sha256_artefacto_check CHECK (((sha256_artefacto IS NULL) OR (sha256_artefacto ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT privacidad_ejecucion_derecho_tipo_ejecucion_check CHECK ((tipo_ejecucion = ANY (ARRAY['rectificacion_perfil'::text, 'oposicion_finalidad'::text, 'bloqueo_aplicado'::text, 'bloqueo_levantado'::text, 'portabilidad_emitida'::text, 'transferencia_portabilidad'::text, 'propagacion_encargado'::text])))
);


--
-- Name: privacidad_incidente_datos; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_incidente_datos (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    folio_incidente text NOT NULL,
    titulo text NOT NULL,
    severidad text NOT NULL,
    estado text DEFAULT 'abierto'::text NOT NULL,
    categorias_datos text[] NOT NULL,
    sistemas_afectados text[] NOT NULL,
    titulares_estimados integer DEFAULT 0 NOT NULL,
    resumen_hechos text NOT NULL,
    medidas_contencion text,
    requiere_notificacion_agencia boolean DEFAULT false CONSTRAINT privacidad_incidente_datos_requiere_notificacion_agenc_not_null NOT NULL,
    notificado_agencia_en timestamp with time zone,
    referencia_agencia text,
    requiere_notificacion_titulares boolean DEFAULT false CONSTRAINT privacidad_incidente_datos_requiere_notificacion_titul_not_null NOT NULL,
    notificado_titulares_en timestamp with time zone,
    lider_id uuid NOT NULL,
    suplente_id uuid,
    detectado_en timestamp with time zone NOT NULL,
    creado_por uuid NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    cerrado_en timestamp with time zone,
    version integer DEFAULT 1 NOT NULL,
    CONSTRAINT privacidad_incidente_datos_categorias_datos_check CHECK ((cardinality(categorias_datos) >= 1)),
    CONSTRAINT privacidad_incidente_datos_check CHECK (((estado = 'cerrado'::text) = (cerrado_en IS NOT NULL))),
    CONSTRAINT privacidad_incidente_datos_check1 CHECK (((notificado_agencia_en IS NULL) OR (requiere_notificacion_agencia = true))),
    CONSTRAINT privacidad_incidente_datos_check2 CHECK (((notificado_titulares_en IS NULL) OR (requiere_notificacion_titulares = true))),
    CONSTRAINT privacidad_incidente_datos_estado_check CHECK ((estado = ANY (ARRAY['abierto'::text, 'contenido'::text, 'evaluado'::text, 'notificado'::text, 'cerrado'::text]))),
    CONSTRAINT privacidad_incidente_datos_medidas_contencion_check CHECK (((medidas_contencion IS NULL) OR ((char_length(btrim(medidas_contencion)) >= 10) AND (char_length(btrim(medidas_contencion)) <= 2000)))),
    CONSTRAINT privacidad_incidente_datos_referencia_agencia_check CHECK (((referencia_agencia IS NULL) OR ((char_length(btrim(referencia_agencia)) >= 2) AND (char_length(btrim(referencia_agencia)) <= 160)))),
    CONSTRAINT privacidad_incidente_datos_resumen_hechos_check CHECK (((char_length(btrim(resumen_hechos)) >= 10) AND (char_length(btrim(resumen_hechos)) <= 2000))),
    CONSTRAINT privacidad_incidente_datos_severidad_check CHECK ((severidad = ANY (ARRAY['baja'::text, 'media'::text, 'alta'::text, 'critica'::text]))),
    CONSTRAINT privacidad_incidente_datos_sistemas_afectados_check CHECK ((cardinality(sistemas_afectados) >= 1)),
    CONSTRAINT privacidad_incidente_datos_titulares_estimados_check CHECK ((titulares_estimados >= 0)),
    CONSTRAINT privacidad_incidente_datos_titulo_check CHECK (((char_length(btrim(titulo)) >= 5) AND (char_length(btrim(titulo)) <= 200))),
    CONSTRAINT privacidad_incidente_datos_version_check CHECK ((version >= 1))
);


--
-- Name: privacidad_incidente_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_incidente_evento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    incidente_id uuid NOT NULL,
    secuencia integer NOT NULL,
    tipo_evento text NOT NULL,
    actor_id uuid NOT NULL,
    detalle text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT privacidad_incidente_evento_detalle_check CHECK (((char_length(btrim(detalle)) >= 5) AND (char_length(btrim(detalle)) <= 2000))),
    CONSTRAINT privacidad_incidente_evento_secuencia_check CHECK ((secuencia >= 1)),
    CONSTRAINT privacidad_incidente_evento_tipo_evento_check CHECK ((tipo_evento = ANY (ARRAY['apertura'::text, 'contencion'::text, 'evaluacion'::text, 'notificacion_agencia'::text, 'notificacion_titulares'::text, 'nota'::text, 'cierre'::text])))
);


--
-- Name: privacidad_notificacion_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_notificacion_outbox (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    solicitud_id uuid NOT NULL,
    tipo_aviso text NOT NULL,
    clave_idempotencia text NOT NULL,
    destinatario_correo text NOT NULL,
    asunto text NOT NULL,
    cuerpo_texto text NOT NULL,
    estado text DEFAULT 'pending'::text NOT NULL,
    intentos smallint DEFAULT 0 NOT NULL,
    disponible_en timestamp with time zone DEFAULT now() NOT NULL,
    reclamado_en timestamp with time zone,
    entregado_en timestamp with time zone,
    terminal_en timestamp with time zone,
    ultimo_error_codigo text,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT privacidad_notificacion_outbox_asunto_check CHECK (((char_length(asunto) >= 3) AND (char_length(asunto) <= 200))),
    CONSTRAINT privacidad_notificacion_outbox_check CHECK (((estado = 'processing'::text) = (reclamado_en IS NOT NULL))),
    CONSTRAINT privacidad_notificacion_outbox_check1 CHECK (((estado = 'delivered'::text) = (entregado_en IS NOT NULL))),
    CONSTRAINT privacidad_notificacion_outbox_check2 CHECK (((estado = 'terminal'::text) = (terminal_en IS NOT NULL))),
    CONSTRAINT privacidad_notificacion_outbox_check3 CHECK ((num_nonnulls(entregado_en, terminal_en) <= 1)),
    CONSTRAINT privacidad_notificacion_outbox_cuerpo_texto_check CHECK (((char_length(cuerpo_texto) >= 10) AND (char_length(cuerpo_texto) <= 4000))),
    CONSTRAINT privacidad_notificacion_outbox_destinatario_correo_check CHECK (((char_length(destinatario_correo) >= 5) AND (char_length(destinatario_correo) <= 254))),
    CONSTRAINT privacidad_notificacion_outbox_estado_check CHECK ((estado = ANY (ARRAY['pending'::text, 'processing'::text, 'delivered'::text, 'terminal'::text]))),
    CONSTRAINT privacidad_notificacion_outbox_intentos_check CHECK (((intentos >= 0) AND (intentos <= 6))),
    CONSTRAINT privacidad_notificacion_outbox_tipo_aviso_check CHECK ((tipo_aviso = ANY (ARRAY['acuse_recepcion'::text, 'aclaracion_solicitada'::text, 'prorroga_plazo'::text, 'resolucion_publicada'::text]))),
    CONSTRAINT privacidad_notificacion_outbox_ultimo_error_codigo_check CHECK (((ultimo_error_codigo IS NULL) OR (ultimo_error_codigo = ANY (ARRAY['mail_delivery_failed'::text, 'attempt_limit_reached'::text]))))
);


--
-- Name: privacidad_preferencia_finalidad; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_preferencia_finalidad (
    cuenta_id uuid NOT NULL,
    finalidad text NOT NULL,
    estado text NOT NULL,
    causal text,
    solicitud_id uuid,
    actualizado_por uuid NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT privacidad_preferencia_finalidad_causal_check CHECK (((causal IS NULL) OR ((char_length(btrim(causal)) >= 5) AND (char_length(btrim(causal)) <= 500)))),
    CONSTRAINT privacidad_preferencia_finalidad_estado_check CHECK ((estado = ANY (ARRAY['permitido'::text, 'opuesto'::text, 'retirado'::text]))),
    CONSTRAINT privacidad_preferencia_finalidad_finalidad_check CHECK ((finalidad = ANY (ARRAY['comunicaciones_comerciales'::text, 'analitica_producto'::text, 'personalizacion_recomendaciones'::text, 'comparticion_encargados_opcionales'::text])))
);


--
-- Name: privacidad_retencion_legal; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacidad_retencion_legal (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    folio_retencion text NOT NULL,
    cuenta_id uuid,
    solicitud_id uuid,
    incidente_id uuid,
    ambito text NOT NULL,
    autoridad text NOT NULL,
    fundamento text NOT NULL,
    estado text DEFAULT 'activa'::text NOT NULL,
    revisar_en timestamp with time zone,
    aplicada_por uuid NOT NULL,
    aplicada_en timestamp with time zone DEFAULT now() NOT NULL,
    liberada_por uuid,
    liberada_en timestamp with time zone,
    motivo_liberacion text,
    CONSTRAINT privacidad_retencion_legal_ambito_check CHECK ((ambito = ANY (ARRAY['expediente_privacidad'::text, 'cuenta_completa'::text, 'evidencia_kyc'::text, 'contratos_y_pagos'::text, 'incidente_seguridad'::text]))),
    CONSTRAINT privacidad_retencion_legal_autoridad_check CHECK (((char_length(btrim(autoridad)) >= 3) AND (char_length(btrim(autoridad)) <= 200))),
    CONSTRAINT privacidad_retencion_legal_check CHECK ((((estado = 'activa'::text) AND (liberada_por IS NULL) AND (liberada_en IS NULL) AND (motivo_liberacion IS NULL)) OR ((estado = 'liberada'::text) AND (liberada_por IS NOT NULL) AND (liberada_en IS NOT NULL) AND (motivo_liberacion IS NOT NULL)))),
    CONSTRAINT privacidad_retencion_legal_estado_check CHECK ((estado = ANY (ARRAY['activa'::text, 'liberada'::text]))),
    CONSTRAINT privacidad_retencion_legal_fundamento_check CHECK (((char_length(btrim(fundamento)) >= 10) AND (char_length(btrim(fundamento)) <= 1000))),
    CONSTRAINT privacidad_retencion_legal_motivo_liberacion_check CHECK (((motivo_liberacion IS NULL) OR ((char_length(btrim(motivo_liberacion)) >= 10) AND (char_length(btrim(motivo_liberacion)) <= 1000))))
);


--
-- Name: privacy_response_dispatch_receipt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_response_dispatch_receipt (
    request_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    channel text NOT NULL,
    destination text NOT NULL,
    sent_at timestamp with time zone NOT NULL,
    evidence_ref text NOT NULL,
    dispatch_recorded_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT privacy_dispatch_destination_ck CHECK ((((channel = 'email'::text) AND (destination ~* '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'::text)) OR ((channel = 'postal'::text) AND (length(btrim(destination)) >= 5)))),
    CONSTRAINT privacy_response_dispatch_receipt_channel_check CHECK ((channel = ANY (ARRAY['email'::text, 'postal'::text]))),
    CONSTRAINT privacy_response_dispatch_receipt_destination_check CHECK (((length(btrim(destination)) >= 1) AND (length(btrim(destination)) <= 500))),
    CONSTRAINT privacy_response_dispatch_receipt_evidence_ref_check CHECK (((length(btrim(evidence_ref)) >= 1) AND (length(btrim(evidence_ref)) <= 500)))
);


--
-- Name: profile_media; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.profile_media (
    propietario_id uuid NOT NULL,
    media_id uuid NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: profile_pinpass_reauth; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.profile_pinpass_reauth (
    account_id uuid NOT NULL,
    failed_attempts integer DEFAULT 0 NOT NULL,
    blocked_until timestamp with time zone,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT profile_pinpass_reauth_failed_attempts_check CHECK ((failed_attempts >= 0))
);


--
-- Name: reaplicacion_baja_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reaplicacion_baja_local (
    restore_id uuid NOT NULL,
    ejecucion_origen_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    estado text NOT NULL,
    intentos integer DEFAULT 1 NOT NULL,
    aplicada_en timestamp with time zone NOT NULL,
    CONSTRAINT reaplicacion_baja_local_estado_check CHECK ((estado = ANY (ARRAY['reaplicada'::text, 'ya_presente'::text, 'pendiente'::text]))),
    CONSTRAINT reaplicacion_baja_local_intentos_check CHECK ((intentos > 0))
);


--
-- Name: reclamo_dano_descargo_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reclamo_dano_descargo_ensayo_local (
    id uuid NOT NULL,
    reclamo_id uuid NOT NULL,
    actor_id uuid,
    descripcion text NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    creado_en timestamp with time zone NOT NULL,
    CONSTRAINT reclamo_dano_descargo_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reclamo_dano_descargo_ensayo_local_descripcion_check CHECK ((length(btrim(descripcion)) > 0)),
    CONSTRAINT reclamo_dano_descargo_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32))
);


--
-- Name: reclamo_dano_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reclamo_dano_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid,
    anfitrion_id uuid,
    arrendatario_id uuid,
    checkout_operacion_id uuid,
    checkout_evidencia_id uuid,
    descripcion text NOT NULL,
    estado text NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    abierto_en timestamp with time zone NOT NULL,
    plazo_reclamo_hasta timestamp with time zone NOT NULL,
    CONSTRAINT reclamo_dano_ensayo_local_check CHECK ((anfitrion_id <> arrendatario_id)),
    CONSTRAINT reclamo_dano_ensayo_local_check1 CHECK ((plazo_reclamo_hasta > abierto_en)),
    CONSTRAINT reclamo_dano_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reclamo_dano_ensayo_local_descripcion_check CHECK ((length(btrim(descripcion)) > 0)),
    CONSTRAINT reclamo_dano_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['abierto'::text, 'resuelta'::text]))),
    CONSTRAINT reclamo_dano_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32))
);


--
-- Name: COLUMN reclamo_dano_ensayo_local.reserva_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.reclamo_dano_ensayo_local.reserva_id IS 'Retained until the reservation privacy deadline; nulled by the local 24-month history-link purger.';


--
-- Name: reclamo_dano_historial_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reclamo_dano_historial_ensayo_local (
    secuencia bigint NOT NULL,
    reclamo_id uuid NOT NULL,
    accion text NOT NULL,
    actor_id uuid,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT reclamo_dano_historial_ensayo_local_accion_check CHECK ((accion = ANY (ARRAY['reclamo_abierto'::text, 'descargo_registrado'::text, 'reclamo_resuelto'::text])))
);


--
-- Name: reclamo_dano_historial_ensayo_local_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.reclamo_dano_historial_ensayo_local ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.reclamo_dano_historial_ensayo_local_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: reclamo_dano_resolucion_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reclamo_dano_resolucion_ensayo_local (
    reclamo_id uuid NOT NULL,
    resultado text NOT NULL,
    motivo_codigo text NOT NULL,
    administrador_id uuid,
    resuelta_en timestamp with time zone NOT NULL,
    clave_idempotencia text CONSTRAINT reclamo_dano_resolucion_ensayo_loca_clave_idempotencia_not_null NOT NULL,
    huella_solicitud bytea NOT NULL,
    CONSTRAINT reclamo_dano_resolucion_ensayo_local_check CHECK ((((resultado = 'acogido'::text) AND (motivo_codigo = 'evidencia_suficiente'::text)) OR ((resultado = 'rechazado'::text) AND (motivo_codigo <> 'evidencia_suficiente'::text)))),
    CONSTRAINT reclamo_dano_resolucion_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reclamo_dano_resolucion_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reclamo_dano_resolucion_ensayo_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['evidencia_suficiente'::text, 'evidencia_insuficiente'::text, 'hecho_no_acreditado'::text, 'informacion_insuficiente'::text]))),
    CONSTRAINT reclamo_dano_resolucion_ensayo_local_resultado_check CHECK ((resultado = ANY (ARRAY['acogido'::text, 'rechazado'::text])))
);


--
-- Name: TABLE reclamo_dano_resolucion_ensayo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.reclamo_dano_resolucion_ensayo_local IS 'Synthetic local adjudication only. It stores no guarantee amount and performs no capture, release, refund or settlement; those effects are deferred to LOCAL-FIN-01.';


--
-- Name: region_publica; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.region_publica (
    catalogo_version text NOT NULL,
    codigo text NOT NULL,
    nombre text NOT NULL,
    CONSTRAINT region_publica_codigo_check CHECK ((codigo ~ '^[0-9]{2}$'::text)),
    CONSTRAINT region_publica_nombre_check CHECK (((length(btrim(nombre)) >= 1) AND (length(btrim(nombre)) <= 100)))
);


--
-- Name: registro_replay_baja_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.registro_replay_baja_local (
    ejecucion_origen_id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    solicitada_en timestamp with time zone NOT NULL,
    completada_en timestamp with time zone NOT NULL
);


--
-- Name: reporte_resena_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reporte_resena_ensayo_local (
    id uuid NOT NULL,
    resena_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    anfitrion_id uuid,
    motivo_codigo text NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    resolver_en timestamp with time zone,
    resuelta_por uuid,
    motivo_resolucion_codigo text,
    retirar_en timestamp with time zone,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    CONSTRAINT reporte_resena_ensayo_local_check CHECK ((((estado = 'pendiente'::text) AND (resolver_en IS NULL) AND (resuelta_por IS NULL) AND (motivo_resolucion_codigo IS NULL) AND (retirar_en IS NULL)) OR ((estado = ANY (ARRAY['desestimada'::text, 'oculta'::text])) AND (resolver_en IS NOT NULL) AND (resuelta_por IS NOT NULL) AND (motivo_resolucion_codigo IS NOT NULL) AND (retirar_en = (resolver_en + '2 years'::interval))))),
    CONSTRAINT reporte_resena_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reporte_resena_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'desestimada'::text, 'oculta'::text]))),
    CONSTRAINT reporte_resena_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reporte_resena_ensayo_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['insultos_acoso'::text, 'datos_personales'::text, 'spam'::text, 'ajeno_experiencia'::text]))),
    CONSTRAINT reporte_resena_ensayo_local_motivo_resolucion_codigo_check CHECK ((motivo_resolucion_codigo = ANY (ARRAY['sin_infraccion'::text, 'contenido_inadecuado'::text, 'duplicado'::text, 'error_registro'::text])))
);


--
-- Name: TABLE reporte_resena_ensayo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.reporte_resena_ensayo_local IS 'Synthetic local moderation report; retain while pending and 24 months after resolution. Not a legal retention determination.';


--
-- Name: reporte_resena_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reporte_resena_historial_local (
    secuencia bigint NOT NULL,
    reporte_id uuid NOT NULL,
    estado_anterior text,
    estado_nuevo text NOT NULL,
    actor_id uuid,
    motivo_codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT reporte_resena_historial_local_estado_anterior_check CHECK ((estado_anterior = ANY (ARRAY['pendiente'::text, 'desestimada'::text, 'oculta'::text]))),
    CONSTRAINT reporte_resena_historial_local_estado_nuevo_check CHECK ((estado_nuevo = ANY (ARRAY['pendiente'::text, 'desestimada'::text, 'oculta'::text])))
);


--
-- Name: reporte_resena_historial_local_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.reporte_resena_historial_local ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.reporte_resena_historial_local_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: resena_autoria_marca_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resena_autoria_marca_local (
    huella_autoria bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirar_en timestamp with time zone,
    CONSTRAINT resena_autoria_marca_local_huella_autoria_check CHECK ((octet_length(huella_autoria) = 32))
);


--
-- Name: TABLE resena_autoria_marca_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.resena_autoria_marca_local IS 'Opaque uniqueness marker only; no rating/comment/account identifier. A NULL retirement date means the reservation still permits reviews; otherwise retained through the reservation-link deadline.';


--
-- Name: resena_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resena_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    autor_id uuid,
    destinatario_tipo text NOT NULL,
    destinatario_id uuid,
    puntuacion smallint NOT NULL,
    comentario text DEFAULT ''::text NOT NULL,
    estado text DEFAULT 'publicada'::text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirar_en timestamp with time zone NOT NULL,
    huella_solicitud bytea NOT NULL,
    clave_idempotencia text NOT NULL,
    CONSTRAINT resena_destinatario_privacidad_ck CHECK ((((destinatario_tipo = 'espacio'::text) AND (destinatario_id IS NULL)) OR ((destinatario_tipo = 'arrendatario'::text) AND (destinatario_id IS NOT NULL)) OR ((destinatario_tipo = 'arrendatario_retirado'::text) AND (destinatario_id IS NULL)))),
    CONSTRAINT resena_destinatario_tipo_ck CHECK ((destinatario_tipo = ANY (ARRAY['espacio'::text, 'arrendatario'::text, 'arrendatario_retirado'::text]))),
    CONSTRAINT resena_ensayo_local_check1 CHECK ((retirar_en = (creada_en + '2 years'::interval))),
    CONSTRAINT resena_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT resena_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['publicada'::text, 'reportada'::text, 'oculta'::text]))),
    CONSTRAINT resena_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT resena_ensayo_local_puntuacion_check CHECK (((puntuacion >= 1) AND (puntuacion <= 5)))
);


--
-- Name: TABLE resena_ensayo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.resena_ensayo_local IS 'Synthetic local reviews; remove after 24 calendar months from creation. Not a legal retention determination.';


--
-- Name: resena_reserva_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resena_reserva_comercial (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    autor_id uuid,
    destinatario_tipo text NOT NULL,
    destinatario_id uuid,
    calificacion smallint NOT NULL,
    comentario text DEFAULT ''::text NOT NULL,
    estado text DEFAULT 'publicada'::text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    retirar_en timestamp with time zone NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    CONSTRAINT resena_reserva_comercial_destino_ck CHECK ((((destinatario_tipo = 'espacio'::text) AND (destinatario_id IS NULL)) OR ((destinatario_tipo = 'arrendatario'::text) AND (destinatario_id IS NOT NULL)))),
    CONSTRAINT resena_reserva_comercial_hash_ck CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT resena_reserva_comercial_key_ck CHECK (((length(btrim(clave_idempotencia)) >= 8) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT resena_reserva_comercial_participants_ck CHECK (((autor_id IS NULL) OR (destinatario_id IS NULL) OR (autor_id <> destinatario_id))),
    CONSTRAINT resena_reserva_comercial_rating_ck CHECK (((calificacion >= 1) AND (calificacion <= 5))),
    CONSTRAINT resena_reserva_comercial_retencion_ck CHECK ((retirar_en = (creada_en + '2 years'::interval))),
    CONSTRAINT resena_reserva_comercial_state_ck CHECK ((estado = ANY (ARRAY['publicada'::text, 'reportada'::text, 'oculta'::text])))
);


--
-- Name: TABLE resena_reserva_comercial; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.resena_reserva_comercial IS 'Reviews from paid commercial reservations; participant identifiers are nulled after the 24-month link retention window.';


--
-- Name: reserva_arriendo_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_arriendo_comercial (
    id uuid NOT NULL,
    operacion_checkout_id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    anfitrion_id uuid NOT NULL,
    arrendatario_id uuid NOT NULL,
    estado text NOT NULL,
    tarifa_version integer NOT NULL,
    modalidad text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    unidades bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    moneda character(3) NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    intervalo tstzrange GENERATED ALWAYS AS (tstzrange(inicio, termino, '[)'::text)) STORED,
    zona_horaria text NOT NULL,
    ocupacion_id uuid NOT NULL,
    pago_vence_en timestamp with time zone NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_arriendo_comercial_check CHECK ((subtotal_clp = (precio_unitario_clp * unidades))),
    CONSTRAINT reserva_arriendo_comercial_check1 CHECK ((termino > inicio)),
    CONSTRAINT reserva_arriendo_comercial_check2 CHECK ((isfinite(inicio) AND isfinite(termino) AND isfinite(pago_vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en))),
    CONSTRAINT reserva_arriendo_comercial_check3 CHECK ((((estado = ANY (ARRAY['pendiente_de_pago'::text, 'reconciliation_required'::text])) AND (pago_vence_en IS NOT NULL)) OR (estado = ANY (ARRAY['pagada'::text, 'cancelada_por_pago'::text, 'vencida_pago'::text, 'cancelada'::text, 'reembolsada'::text])))),
    CONSTRAINT reserva_arriendo_comercial_distinct_participants_ck CHECK ((anfitrion_id <> arrendatario_id)),
    CONSTRAINT reserva_arriendo_comercial_estado_check CHECK ((estado = ANY (ARRAY['pendiente_de_pago'::text, 'pagada'::text, 'cancelada_por_pago'::text, 'vencida_pago'::text, 'reconciliation_required'::text, 'cancelada'::text, 'reembolsada'::text]))),
    CONSTRAINT reserva_arriendo_comercial_modalidad_check CHECK ((modalidad = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text]))),
    CONSTRAINT reserva_arriendo_comercial_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT reserva_arriendo_comercial_precio_unitario_clp_check CHECK ((precio_unitario_clp > 0)),
    CONSTRAINT reserva_arriendo_comercial_tarifa_version_check CHECK ((tarifa_version > 0)),
    CONSTRAINT reserva_arriendo_comercial_unidades_check CHECK ((unidades > 0)),
    CONSTRAINT reserva_arriendo_comercial_zona_horaria_check CHECK (((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100)))
);


--
-- Name: reserva_arriendo_tramo_estado; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_arriendo_tramo_estado (
    reserva_id uuid NOT NULL,
    tramo_id uuid NOT NULL,
    estado text DEFAULT 'pendiente'::text NOT NULL,
    check_in_en timestamp with time zone,
    check_out_en timestamp with time zone,
    actualizado_por uuid,
    actualizada_en timestamp with time zone DEFAULT now() NOT NULL,
    codigo_corto text,
    CONSTRAINT reserva_arriendo_tramo_estado_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'en_uso'::text, 'completado'::text, 'no_asistio'::text])))
);


--
-- Name: reserva_cancelacion_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_cancelacion_ensayo (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    arrendatario_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    motivo text DEFAULT ''::text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_cancelacion_ensayo_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reserva_cancelacion_ensayo_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32))
);


--
-- Name: reserva_comercial_cancelacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_comercial_cancelacion (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    rol_actor text NOT NULL,
    politica_version text NOT NULL,
    politica_texto text NOT NULL,
    motivo text NOT NULL,
    porcentaje_reembolso smallint NOT NULL,
    monto_total_clp bigint NOT NULL,
    monto_reembolso_clp bigint NOT NULL,
    clave_idempotencia text NOT NULL,
    provider_refund_id text,
    refund_status text NOT NULL,
    intentos integer DEFAULT 0 NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    resuelta_en timestamp with time zone,
    CONSTRAINT reserva_comercial_cancelacion_check CHECK (((monto_reembolso_clp >= 0) AND (monto_reembolso_clp <= monto_total_clp))),
    CONSTRAINT reserva_comercial_cancelacion_check1 CHECK ((isfinite(creada_en) AND isfinite(actualizada_en) AND ((resuelta_en IS NULL) OR isfinite(resuelta_en)))),
    CONSTRAINT reserva_comercial_cancelacion_check2 CHECK ((((refund_status = ANY (ARRAY['pendiente'::text, 'fallido'::text])) AND (resuelta_en IS NULL)) OR ((refund_status = ANY (ARRAY['aprobado'::text, 'no_aplica'::text])) AND (resuelta_en IS NOT NULL)))),
    CONSTRAINT reserva_comercial_cancelacion_check3 CHECK ((((monto_reembolso_clp = 0) AND (refund_status = ANY (ARRAY['no_aplica'::text, 'pendiente'::text, 'fallido'::text]))) OR ((monto_reembolso_clp > 0) AND (refund_status = ANY (ARRAY['pendiente'::text, 'fallido'::text, 'aprobado'::text]))))),
    CONSTRAINT reserva_comercial_cancelacion_clave_idempotencia_check CHECK (((length(clave_idempotencia) >= 1) AND (length(clave_idempotencia) <= 200))),
    CONSTRAINT reserva_comercial_cancelacion_intentos_check CHECK ((intentos >= 0)),
    CONSTRAINT reserva_comercial_cancelacion_monto_total_clp_check CHECK ((monto_total_clp > 0)),
    CONSTRAINT reserva_comercial_cancelacion_motivo_check CHECK (((length(btrim(motivo)) >= 1) AND (length(btrim(motivo)) <= 500))),
    CONSTRAINT reserva_comercial_cancelacion_politica_texto_check CHECK (((length(btrim(politica_texto)) >= 1) AND (length(btrim(politica_texto)) <= 500))),
    CONSTRAINT reserva_comercial_cancelacion_politica_version_check CHECK ((politica_version = ANY (ARRAY['facil_v1'::text, 'flexible_v1'::text, 'moderada_v1'::text, 'estricta_v1'::text, 'sin_snapshot_v1'::text]))),
    CONSTRAINT reserva_comercial_cancelacion_porcentaje_reembolso_check CHECK (((porcentaje_reembolso >= 0) AND (porcentaje_reembolso <= 100))),
    CONSTRAINT reserva_comercial_cancelacion_provider_refund_id_check CHECK (((provider_refund_id IS NULL) OR (provider_refund_id ~ '^[A-Za-z0-9_-]{1,100}$'::text))),
    CONSTRAINT reserva_comercial_cancelacion_refund_status_check CHECK ((refund_status = ANY (ARRAY['pendiente'::text, 'aprobado'::text, 'fallido'::text, 'no_aplica'::text]))),
    CONSTRAINT reserva_comercial_cancelacion_rol_actor_check CHECK ((rol_actor = ANY (ARRAY['arrendatario'::text, 'anfitrion'::text])))
);


--
-- Name: TABLE reserva_comercial_cancelacion; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.reserva_comercial_cancelacion IS 'Durable cancellation and refund intent for commercial reservations; retries reuse clave_idempotencia.';


--
-- Name: reserva_comercial_mensaje_lectura; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_comercial_mensaje_lectura (
    reserva_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    ultima_secuencia_leida bigint CONSTRAINT reserva_comercial_mensaje_lectu_ultima_secuencia_leida_not_null NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_comercial_mensaje_lectura_sequence_ck CHECK ((ultima_secuencia_leida > 0))
);


--
-- Name: reserva_comprobante_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_comprobante_ensayo_local (
    id uuid NOT NULL,
    liquidacion_id uuid NOT NULL,
    version smallint NOT NULL,
    contenido jsonb NOT NULL,
    emitido_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_comprobante_ensayo_local_contenido_check CHECK ((((contenido ->> 'kind'::text) = 'comprobante_ensayo'::text) AND ((contenido ->> 'banner'::text) = 'ENSAYO LOCAL — SIN VALIDEZ FISCAL — SIN MOVIMIENTOS REALES'::text))),
    CONSTRAINT reserva_comprobante_ensayo_local_version_check CHECK ((version = 1))
);


--
-- Name: reserva_cuota_mensual; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_cuota_mensual (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reserva_arriendo_comercial_id uuid NOT NULL,
    numero_cuota integer NOT NULL,
    periodo_inicio timestamp with time zone NOT NULL,
    periodo_termino timestamp with time zone NOT NULL,
    monto_clp bigint NOT NULL,
    estado text DEFAULT 'pendiente'::text NOT NULL,
    pagada_en timestamp with time zone,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT reserva_cuota_mensual_check CHECK ((periodo_termino > periodo_inicio)),
    CONSTRAINT reserva_cuota_mensual_check1 CHECK ((isfinite(periodo_inicio) AND isfinite(periodo_termino))),
    CONSTRAINT reserva_cuota_mensual_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'pagada'::text, 'vencida'::text, 'cancelada'::text]))),
    CONSTRAINT reserva_cuota_mensual_monto_clp_check CHECK ((monto_clp > 0)),
    CONSTRAINT reserva_cuota_mensual_numero_cuota_check CHECK ((numero_cuota >= 1))
);


--
-- Name: reserva_decision_financiera_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_decision_financiera_ensayo_local (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    garantia_id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    reclamo_id uuid,
    resultado_reclamo text CONSTRAINT reserva_decision_financiera_ensayo_l_resultado_reclamo_not_null NOT NULL,
    deduccion_clp numeric(14,0) NOT NULL,
    motivo_codigo text NOT NULL,
    evidencia_id uuid,
    administrador_id uuid CONSTRAINT reserva_decision_financiera_ensayo_lo_administrador_id_not_null NOT NULL,
    clave_idempotencia text CONSTRAINT reserva_decision_financiera_ensayo__clave_idempotencia_not_null NOT NULL,
    huella_solicitud bytea CONSTRAINT reserva_decision_financiera_ensayo_lo_huella_solicitud_not_null NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone CONSTRAINT reserva_decision_financiera_ensayo_loca_actualizada_en_not_null NOT NULL,
    CONSTRAINT reserva_decision_financiera_ensayo_loc_clave_idempotencia_check CHECK (((length(clave_idempotencia) >= 1) AND (length(clave_idempotencia) <= 200))),
    CONSTRAINT reserva_decision_financiera_ensayo_loca_resultado_reclamo_check CHECK ((resultado_reclamo = ANY (ARRAY['acogido'::text, 'rechazado'::text]))),
    CONSTRAINT reserva_decision_financiera_ensayo_local_check CHECK ((((deduccion_clp = (0)::numeric) AND (motivo_codigo = 'sin_deduccion'::text) AND (evidencia_id IS NULL)) OR ((deduccion_clp > (0)::numeric) AND (motivo_codigo = ANY (ARRAY['dano_acreditado'::text, 'faltante_acreditado'::text])) AND (evidencia_id IS NOT NULL)))),
    CONSTRAINT reserva_decision_financiera_ensayo_local_check1 CHECK (((resultado_reclamo <> 'rechazado'::text) OR (deduccion_clp = (0)::numeric))),
    CONSTRAINT reserva_decision_financiera_ensayo_local_deduccion_clp_check CHECK ((deduccion_clp >= (0)::numeric)),
    CONSTRAINT reserva_decision_financiera_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'por_conciliar'::text, 'aplicada'::text, 'sin_deduccion'::text]))),
    CONSTRAINT reserva_decision_financiera_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reserva_decision_financiera_ensayo_local_motivo_codigo_check CHECK ((motivo_codigo = ANY (ARRAY['dano_acreditado'::text, 'faltante_acreditado'::text, 'sin_deduccion'::text]))),
    CONSTRAINT reserva_decision_financiera_ensayo_local_reclamo_id_check CHECK ((reclamo_id IS NOT NULL))
);


--
-- Name: TABLE reserva_decision_financiera_ensayo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.reserva_decision_financiera_ensayo_local IS 'Decisión económica local inmutable y separada de la resolución histórica del reclamo.';


--
-- Name: reserva_devolucion_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_devolucion_ensayo (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    operacion_id uuid NOT NULL,
    importe_clp bigint NOT NULL,
    moneda character(3) NOT NULL,
    estado text NOT NULL,
    ultimo_resultado text,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    completada_en timestamp with time zone,
    CONSTRAINT reserva_devolucion_ensayo_check CHECK (((estado = 'completada'::text) = (completada_en IS NOT NULL))),
    CONSTRAINT reserva_devolucion_ensayo_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'completada'::text]))),
    CONSTRAINT reserva_devolucion_ensayo_importe_clp_check CHECK ((importe_clp > 0)),
    CONSTRAINT reserva_devolucion_ensayo_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT reserva_devolucion_ensayo_ultimo_resultado_check CHECK ((ultimo_resultado = ANY (ARRAY['exito_simulado'::text, 'fallo_simulado'::text, 'sin_respuesta_simulada'::text])))
);


--
-- Name: reserva_devolucion_intento_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_devolucion_intento_ensayo (
    id uuid NOT NULL,
    devolucion_id uuid NOT NULL,
    secuencia bigint NOT NULL,
    resultado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_devolucion_intento_ensayo_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'fallo_simulado'::text, 'sin_respuesta_simulada'::text]))),
    CONSTRAINT reserva_devolucion_intento_ensayo_secuencia_check CHECK ((secuencia > 0))
);


--
-- Name: reserva_documento_tributario_comercial; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_documento_tributario_comercial (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    anfitrion_id uuid NOT NULL,
    object_id uuid NOT NULL,
    object_key text GENERATED ALWAYS AS ((('rental-tax-documents/'::text || (object_id)::text) || '.blob'::text)) STORED,
    bucket_name text NOT NULL,
    sha256 bytea NOT NULL,
    size_bytes bigint NOT NULL,
    inspected_at timestamp with time zone NOT NULL,
    uploaded_at timestamp with time zone NOT NULL,
    CONSTRAINT reserva_documento_tributario_comercial_bucket_name_check CHECK ((bucket_name ~ '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$'::text)),
    CONSTRAINT reserva_documento_tributario_comercial_check CHECK ((isfinite(inspected_at) AND isfinite(uploaded_at))),
    CONSTRAINT reserva_documento_tributario_comercial_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT reserva_documento_tributario_comercial_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760)))
);


--
-- Name: reserva_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_ensayo_local (
    id uuid NOT NULL,
    cotizacion_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    anfitrion_id uuid,
    arrendatario_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    ocupacion_id uuid NOT NULL,
    estado text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    unidades bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    modalidad text NOT NULL,
    moneda character(3) NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    pago_vence_en timestamp with time zone NOT NULL,
    anfitrion_vence_en timestamp with time zone,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    condiciones_snapshot text NOT NULL,
    politica_cancelacion_version text DEFAULT 'local_flexible_v1'::text NOT NULL,
    vinculos_retirar_en timestamp with time zone,
    garantia_politica_version text,
    garantia_moneda character(3),
    garantia_prevista_clp numeric(14,0),
    liquidacion_politica_version text,
    liquidacion_comision_bps integer,
    CONSTRAINT reserva_ensayo_garantia_snapshot_ck CHECK ((((garantia_politica_version IS NULL) AND (garantia_moneda IS NULL) AND (garantia_prevista_clp IS NULL)) OR ((garantia_politica_version = 'garantia_local_fija_v1'::text) AND (garantia_moneda = 'CLP'::bpchar) AND (garantia_prevista_clp = (50000)::numeric)))),
    CONSTRAINT reserva_ensayo_liquidacion_snapshot_ck CHECK ((((liquidacion_politica_version IS NULL) AND (liquidacion_comision_bps IS NULL)) OR ((liquidacion_politica_version = 'liquidacion_local_0pct_v1'::text) AND (liquidacion_comision_bps = 0)))),
    CONSTRAINT reserva_ensayo_local_check CHECK ((termino > inicio)),
    CONSTRAINT reserva_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reserva_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente_de_pago'::text, 'pagada'::text, 'aprobada_host'::text, 'firma_parcial'::text, 'lista_para_checkin'::text, 'en_curso'::text, 'finalizada'::text, 'en_disputa'::text, 'cerrada'::text, 'cancelada_por_pago'::text, 'rechazada_arrendador'::text, 'vencida_pago'::text, 'vencida_host'::text, 'cancelada_arrendatario'::text, 'cancelada_por_firma'::text]))),
    CONSTRAINT reserva_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reserva_ensayo_local_moneda_check CHECK ((moneda = 'CLP'::bpchar))
);


--
-- Name: reserva_ensayo_local_fixture; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_ensayo_local_fixture (
    espacio_id uuid NOT NULL,
    anfitrion_id uuid,
    arrendatario_id uuid,
    habilitada_en timestamp with time zone DEFAULT now() NOT NULL,
    habilitada boolean DEFAULT true NOT NULL,
    politica_cancelacion_version text DEFAULT 'local_flexible_v1'::text CONSTRAINT reserva_ensayo_local_fixtur_politica_cancelacion_versi_not_null NOT NULL,
    CONSTRAINT fixture_participantes_distintos_ck CHECK ((anfitrion_id <> arrendatario_id))
);


--
-- Name: reserva_ensayo_local_ubicacion_sintetica; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_ensayo_local_ubicacion_sintetica (
    espacio_id uuid NOT NULL,
    latitud double precision NOT NULL,
    longitud double precision NOT NULL,
    punto public.geography(Point,4326) GENERATED ALWAYS AS ((public.st_setsrid(public.st_makepoint(longitud, latitud), 4326))::public.geography) STORED,
    es_sintetica boolean DEFAULT true NOT NULL,
    CONSTRAINT reserva_ensayo_local_ubicacion_sintetica_es_sintetica_check CHECK (es_sintetica),
    CONSTRAINT reserva_ensayo_local_ubicacion_sintetica_latitud_check CHECK (((latitud >= ('-90'::integer)::double precision) AND (latitud <= (90)::double precision))),
    CONSTRAINT reserva_ensayo_local_ubicacion_sintetica_longitud_check CHECK (((longitud >= ('-180'::integer)::double precision) AND (longitud <= (180)::double precision)))
);


--
-- Name: reserva_ensayo_transicion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_ensayo_transicion (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    estado_anterior text,
    estado_nuevo text NOT NULL,
    actor_id uuid,
    motivo text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    secuencia bigint NOT NULL,
    CONSTRAINT reserva_ensayo_transicion_secuencia_ck CHECK ((secuencia > 0))
);


--
-- Name: reserva_finanzas_historial_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_finanzas_historial_ensayo_local (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    secuencia bigint NOT NULL,
    reserva_id uuid NOT NULL,
    tipo text NOT NULL,
    actor_id uuid,
    detalle_codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_finanzas_historial_ensayo_local_tipo_check CHECK ((tipo = ANY (ARRAY['garantia_solicitada'::text, 'garantia_autorizada'::text, 'garantia_rechazada'::text, 'garantia_vencida'::text, 'garantia_liberada'::text, 'garantia_capturada'::text, 'decision_financiera_registrada'::text, 'operacion_pendiente_conciliacion'::text, 'liquidacion_solicitada'::text, 'liquidacion_confirmada'::text, 'liquidacion_rechazada'::text])))
);


--
-- Name: reserva_finanzas_historial_ensayo_local_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reserva_finanzas_historial_ensayo_local_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reserva_finanzas_historial_ensayo_local_secuencia_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reserva_finanzas_historial_ensayo_local_secuencia_seq OWNED BY public.reserva_finanzas_historial_ensayo_local.secuencia;


--
-- Name: reserva_garantia_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_garantia_ensayo_local (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    reserva_id uuid NOT NULL,
    politica_version text NOT NULL,
    moneda character(3) NOT NULL,
    previsto_clp numeric(14,0) NOT NULL,
    autorizado_clp numeric(14,0) DEFAULT 0 NOT NULL,
    capturado_clp numeric(14,0) DEFAULT 0 NOT NULL,
    liberado_clp numeric(14,0) DEFAULT 0 NOT NULL,
    estado text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_garantia_ensayo_local_autorizado_clp_check CHECK ((autorizado_clp >= (0)::numeric)),
    CONSTRAINT reserva_garantia_ensayo_local_capturado_clp_check CHECK ((capturado_clp >= (0)::numeric)),
    CONSTRAINT reserva_garantia_ensayo_local_check CHECK ((autorizado_clp <= previsto_clp)),
    CONSTRAINT reserva_garantia_ensayo_local_check1 CHECK (((capturado_clp + liberado_clp) <= autorizado_clp)),
    CONSTRAINT reserva_garantia_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente_pago'::text, 'pendiente_autorizacion'::text, 'autorizada'::text, 'no_disponible'::text, 'por_conciliar'::text, 'captura_pendiente'::text, 'captura_por_conciliar'::text, 'parcialmente_capturada'::text, 'liberacion_pendiente'::text, 'liberacion_por_conciliar'::text, 'liberada'::text, 'cancelada'::text]))),
    CONSTRAINT reserva_garantia_ensayo_local_liberado_clp_check CHECK ((liberado_clp >= (0)::numeric)),
    CONSTRAINT reserva_garantia_ensayo_local_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT reserva_garantia_ensayo_local_politica_version_check CHECK ((politica_version = 'garantia_local_fija_v1'::text)),
    CONSTRAINT reserva_garantia_ensayo_local_previsto_clp_check CHECK ((previsto_clp = (50000)::numeric))
);


--
-- Name: TABLE reserva_garantia_ensayo_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.reserva_garantia_ensayo_local IS 'Obligación fake independiente del pago del arriendo; no representa dinero ni custodia real.';


--
-- Name: reserva_garantia_evento_aplicacion_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_garantia_evento_aplicacion_ensayo_local (
    evento_id uuid CONSTRAINT reserva_garantia_evento_aplicacion_ensayo_lo_evento_id_not_null NOT NULL,
    estado text NOT NULL,
    codigo_resultado text,
    creada_en timestamp with time zone CONSTRAINT reserva_garantia_evento_aplicacion_ensayo_lo_creada_en_not_null NOT NULL,
    procesada_en timestamp with time zone,
    CONSTRAINT reserva_garantia_evento_aplicacion_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'aplicada'::text, 'por_conciliar'::text, 'ignorada'::text])))
);


--
-- Name: reserva_garantia_evento_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_garantia_evento_ensayo_local (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    operacion_id uuid NOT NULL,
    evento_id text NOT NULL,
    resultado text NOT NULL,
    autenticado_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_garantia_evento_ensayo_local_evento_id_check CHECK (((length(evento_id) >= 1) AND (length(evento_id) <= 200))),
    CONSTRAINT reserva_garantia_evento_ensayo_local_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text])))
);


--
-- Name: reserva_garantia_operacion_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_garantia_operacion_ensayo_local (
    id uuid NOT NULL,
    garantia_id uuid NOT NULL,
    tipo text NOT NULL,
    clave_idempotencia text CONSTRAINT reserva_garantia_operacion_ensayo_l_clave_idempotencia_not_null NOT NULL,
    huella_solicitud bytea CONSTRAINT reserva_garantia_operacion_ensayo_loc_huella_solicitud_not_null NOT NULL,
    importe_clp numeric(14,0) NOT NULL,
    resultado_solicitado text CONSTRAINT reserva_garantia_operacion_ensayo_resultado_solicitado_not_null NOT NULL,
    estado text NOT NULL,
    primer_intento_en timestamp with time zone CONSTRAINT reserva_garantia_operacion_ensayo_lo_primer_intento_en_not_null NOT NULL,
    vence_en timestamp with time zone,
    ultimo_resultado text,
    completada_en timestamp with time zone,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_garantia_operacion_ensayo_lo_resultado_solicitado_check CHECK ((resultado_solicitado = ANY (ARRAY['exito'::text, 'rechazo'::text, 'sin_respuesta'::text]))),
    CONSTRAINT reserva_garantia_operacion_ensayo_loca_clave_idempotencia_check CHECK (((length(clave_idempotencia) >= 1) AND (length(clave_idempotencia) <= 200))),
    CONSTRAINT reserva_garantia_operacion_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'por_conciliar'::text, 'confirmada'::text, 'rechazada'::text, 'vencida'::text]))),
    CONSTRAINT reserva_garantia_operacion_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reserva_garantia_operacion_ensayo_local_importe_clp_check CHECK ((importe_clp >= (0)::numeric)),
    CONSTRAINT reserva_garantia_operacion_ensayo_local_tipo_check CHECK ((tipo = ANY (ARRAY['autorizacion'::text, 'captura'::text, 'liberacion'::text]))),
    CONSTRAINT reserva_garantia_operacion_ensayo_local_ultimo_resultado_check CHECK (((ultimo_resultado IS NULL) OR (ultimo_resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text, 'sin_respuesta_simulada'::text, 'resultado_tardio'::text, 'autorizacion_vencida'::text, 'cancelada'::text]))))
);


--
-- Name: reserva_garantia_resultado_fake_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_garantia_resultado_fake_ensayo_local (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    operacion_id uuid CONSTRAINT reserva_garantia_resultado_fake_ensayo_lo_operacion_id_not_null NOT NULL,
    evento_id text,
    resultado text NOT NULL,
    registrada_en timestamp with time zone CONSTRAINT reserva_garantia_resultado_fake_ensayo_l_registrada_en_not_null NOT NULL,
    CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_check CHECK ((((resultado = 'sin_respuesta_simulada'::text) AND (evento_id IS NULL)) OR ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text])) AND (evento_id IS NOT NULL)))),
    CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_evento_id_check CHECK (((evento_id IS NULL) OR ((length(evento_id) >= 1) AND (length(evento_id) <= 200)))),
    CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text, 'sin_respuesta_simulada'::text])))
);


--
-- Name: reserva_liquidacion_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_liquidacion_ensayo_local (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    anfitrion_id uuid,
    politica_version text NOT NULL,
    moneda character(3) NOT NULL,
    arriendo_confirmado_clp bigint CONSTRAINT reserva_liquidacion_ensayo_loc_arriendo_confirmado_clp_not_null NOT NULL,
    devoluciones_confirmadas_clp bigint CONSTRAINT reserva_liquidacion_ensayo__devoluciones_confirmadas_c_not_null NOT NULL,
    comision_bps integer NOT NULL,
    comision_clp bigint NOT NULL,
    transferencia_clp bigint NOT NULL,
    estado text NOT NULL,
    ultimo_resultado text,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    completada_en timestamp with time zone,
    CONSTRAINT reserva_liquidacion_ensayo_l_devoluciones_confirmadas_clp_check CHECK ((devoluciones_confirmadas_clp >= 0)),
    CONSTRAINT reserva_liquidacion_ensayo_local_arriendo_confirmado_clp_check CHECK ((arriendo_confirmado_clp >= 0)),
    CONSTRAINT reserva_liquidacion_ensayo_local_check CHECK ((transferencia_clp = ((arriendo_confirmado_clp - devoluciones_confirmadas_clp) - comision_clp))),
    CONSTRAINT reserva_liquidacion_ensayo_local_check1 CHECK (((estado = ANY (ARRAY['liquidada'::text, 'rechazada'::text])) = (completada_en IS NOT NULL))),
    CONSTRAINT reserva_liquidacion_ensayo_local_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reserva_liquidacion_ensayo_local_comision_bps_check CHECK (((comision_bps >= 0) AND (comision_bps <= 10000))),
    CONSTRAINT reserva_liquidacion_ensayo_local_comision_clp_check CHECK ((comision_clp >= 0)),
    CONSTRAINT reserva_liquidacion_ensayo_local_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'por_conciliar'::text, 'liquidada'::text, 'rechazada'::text]))),
    CONSTRAINT reserva_liquidacion_ensayo_local_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reserva_liquidacion_ensayo_local_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT reserva_liquidacion_ensayo_local_politica_version_check CHECK ((politica_version = 'liquidacion_local_0pct_v1'::text)),
    CONSTRAINT reserva_liquidacion_ensayo_local_transferencia_clp_check CHECK ((transferencia_clp >= 0)),
    CONSTRAINT reserva_liquidacion_ensayo_local_ultimo_resultado_check CHECK ((ultimo_resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text, 'sin_respuesta_simulada'::text])))
);


--
-- Name: reserva_liquidacion_historial_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_liquidacion_historial_ensayo_local (
    secuencia bigint NOT NULL,
    liquidacion_id uuid CONSTRAINT reserva_liquidacion_historial_ensayo_lo_liquidacion_id_not_null NOT NULL,
    accion text NOT NULL,
    actor_id uuid,
    codigo text NOT NULL,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_liquidacion_historial_ensayo_local_accion_check CHECK ((accion = ANY (ARRAY['liquidacion_solicitada'::text, 'liquidacion_por_conciliar'::text, 'liquidacion_confirmada'::text, 'liquidacion_rechazada'::text])))
);


--
-- Name: reserva_liquidacion_historial_ensayo_local_secuencia_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.reserva_liquidacion_historial_ensayo_local ALTER COLUMN secuencia ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.reserva_liquidacion_historial_ensayo_local_secuencia_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: reserva_liquidacion_operacion_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_liquidacion_operacion_ensayo_local (
    id uuid NOT NULL,
    liquidacion_id uuid CONSTRAINT reserva_liquidacion_operacion_ensayo_lo_liquidacion_id_not_null NOT NULL,
    resultado_solicitado text CONSTRAINT reserva_liquidacion_operacion_ens_resultado_solicitado_not_null NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone CONSTRAINT reserva_liquidacion_operacion_ensayo_lo_actualizada_en_not_null NOT NULL,
    CONSTRAINT reserva_liquidacion_operacion_ensayo_resultado_solicitado_check CHECK ((resultado_solicitado = ANY (ARRAY['exito'::text, 'rechazo'::text, 'sin_respuesta'::text])))
);


--
-- Name: reserva_liquidacion_resultado_fake_ensayo_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_liquidacion_resultado_fake_ensayo_local (
    operacion_id uuid CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_operacion_id_not_null NOT NULL,
    event_id text,
    resultado text CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_lo_resultado_not_null NOT NULL,
    registrada_en timestamp with time zone CONSTRAINT reserva_liquidacion_resultado_fake_ensay_registrada_en_not_null NOT NULL,
    CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_local_check CHECK ((((resultado = 'sin_respuesta_simulada'::text) AND (event_id IS NULL)) OR ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text])) AND (event_id IS NOT NULL)))),
    CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_local_event_id_check CHECK (((event_id IS NULL) OR ((length(event_id) >= 1) AND (length(event_id) <= 200)))),
    CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_local_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text, 'sin_respuesta_simulada'::text])))
);


--
-- Name: reserva_mensaje_lectura; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_mensaje_lectura (
    reserva_id uuid NOT NULL,
    participante_id uuid NOT NULL,
    ultima_secuencia_leida bigint NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_mensaje_lectura_ultima_secuencia_leida_check CHECK ((ultima_secuencia_leida > 0))
);


--
-- Name: reserva_pago_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_pago_ensayo (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    resultado text NOT NULL,
    clave_idempotencia text NOT NULL,
    creada_en timestamp with time zone NOT NULL,
    importe_clp bigint DEFAULT 0 NOT NULL,
    CONSTRAINT reserva_pago_ensayo_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reserva_pago_ensayo_importe_clp_check CHECK ((importe_clp >= 0)),
    CONSTRAINT reserva_pago_ensayo_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text, 'sin_respuesta_simulada'::text, 'devolucion_simulada'::text])))
);


--
-- Name: reserva_pago_ensayo_operacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_pago_ensayo_operacion (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    arrendatario_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    resultado_solicitado text NOT NULL,
    estado text NOT NULL,
    resultado_final text,
    creada_en timestamp with time zone NOT NULL,
    actualizada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_pago_ensayo_operacion_check CHECK (((estado = 'aplicada'::text) = (resultado_final IS NOT NULL))),
    CONSTRAINT reserva_pago_ensayo_operacion_clave_idempotencia_check CHECK (((length(btrim(clave_idempotencia)) >= 1) AND (length(btrim(clave_idempotencia)) <= 200))),
    CONSTRAINT reserva_pago_ensayo_operacion_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'aplicada'::text, 'vencida'::text]))),
    CONSTRAINT reserva_pago_ensayo_operacion_huella_solicitud_check CHECK ((octet_length(huella_solicitud) = 32)),
    CONSTRAINT reserva_pago_ensayo_operacion_resultado_final_check CHECK ((resultado_final = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text]))),
    CONSTRAINT reserva_pago_ensayo_operacion_resultado_solicitado_check CHECK ((resultado_solicitado = ANY (ARRAY['exito'::text, 'rechazo'::text, 'sin_respuesta'::text])))
);


--
-- Name: reserva_pago_evento_aplicacion_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_pago_evento_aplicacion_ensayo (
    evento_id uuid NOT NULL,
    estado text NOT NULL,
    codigo_resultado text,
    procesado_en timestamp with time zone,
    creada_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_pago_evento_aplicacion_ensayo_check CHECK (((estado = ANY (ARRAY['pendiente'::text, 'pendiente_conciliacion'::text])) = (procesado_en IS NULL))),
    CONSTRAINT reserva_pago_evento_aplicacion_ensayo_codigo_resultado_check CHECK ((codigo_resultado = ANY (ARRAY['aplicado'::text, 'operacion_terminal'::text, 'reserva_vencida'::text, 'resultado_tardio'::text]))),
    CONSTRAINT reserva_pago_evento_aplicacion_ensayo_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'aplicada'::text, 'ignorada'::text, 'vencida'::text, 'pendiente_conciliacion'::text])))
);


--
-- Name: reserva_pago_evento_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_pago_evento_ensayo (
    id uuid NOT NULL,
    operacion_id uuid NOT NULL,
    proveedor_evento_id text NOT NULL,
    resultado text NOT NULL,
    huella_payload bytea NOT NULL,
    autenticado_en timestamp with time zone NOT NULL,
    recibido_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_pago_evento_ensayo_huella_payload_check CHECK ((octet_length(huella_payload) = 32)),
    CONSTRAINT reserva_pago_evento_ensayo_proveedor_evento_id_check CHECK (((length(btrim(proveedor_evento_id)) >= 1) AND (length(btrim(proveedor_evento_id)) <= 200))),
    CONSTRAINT reserva_pago_evento_ensayo_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text])))
);


--
-- Name: reserva_pago_fake_resultado_ensayo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_pago_fake_resultado_ensayo (
    operacion_id uuid NOT NULL,
    estado text NOT NULL,
    proveedor_evento_id text,
    resultado text,
    registrado_en timestamp with time zone NOT NULL,
    CONSTRAINT reserva_pago_fake_resultado_ensayo_check CHECK (((estado = 'resultado'::text) = ((proveedor_evento_id IS NOT NULL) AND (resultado IS NOT NULL)))),
    CONSTRAINT reserva_pago_fake_resultado_ensayo_check1 CHECK (((estado <> 'sin_respuesta'::text) OR ((proveedor_evento_id IS NULL) AND (resultado IS NULL)))),
    CONSTRAINT reserva_pago_fake_resultado_ensayo_estado_check CHECK ((estado = ANY (ARRAY['resultado'::text, 'sin_respuesta'::text]))),
    CONSTRAINT reserva_pago_fake_resultado_ensayo_proveedor_evento_id_check CHECK (((proveedor_evento_id IS NULL) OR ((length(btrim(proveedor_evento_id)) >= 1) AND (length(btrim(proveedor_evento_id)) <= 200)))),
    CONSTRAINT reserva_pago_fake_resultado_ensayo_resultado_check CHECK ((resultado = ANY (ARRAY['exito_simulado'::text, 'rechazo_simulado'::text])))
);


--
-- Name: reserva_vinculo_purgado_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reserva_vinculo_purgado_local (
    reserva_id uuid NOT NULL,
    vencio_en timestamp with time zone NOT NULL,
    purgado_en timestamp with time zone NOT NULL,
    campos_retirados text[] NOT NULL,
    CONSTRAINT reserva_vinculo_purgado_local_campos_retirados_check CHECK ((cardinality(campos_retirados) > 0)),
    CONSTRAINT reserva_vinculo_purgado_local_check CHECK ((purgado_en >= vencio_en))
);


--
-- Name: rol_usuario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rol_usuario (
    usuario_id uuid NOT NULL,
    rol text NOT NULL,
    concedido_en timestamp with time zone DEFAULT now() NOT NULL,
    concedido_por uuid,
    CONSTRAINT rol_usuario_rol_ck CHECK ((rol = ANY (ARRAY['arrendatario'::text, 'arrendador'::text, 'administrador'::text])))
);


--
-- Name: rol_usuario_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rol_usuario_evento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    usuario_id uuid NOT NULL,
    rol text NOT NULL,
    accion text NOT NULL,
    recurso_id uuid NOT NULL,
    ocurrida_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rol_usuario_evento_accion_check CHECK ((accion = ANY (ARRAY['primer_borrador_espacio'::text, 'primera_solicitud_reserva'::text]))),
    CONSTRAINT rol_usuario_evento_accion_rol_ck CHECK ((((rol = 'arrendador'::text) AND (accion = 'primer_borrador_espacio'::text)) OR ((rol = 'arrendatario'::text) AND (accion = 'primera_solicitud_reserva'::text)))),
    CONSTRAINT rol_usuario_evento_rol_check CHECK ((rol = ANY (ARRAY['arrendatario'::text, 'arrendador'::text])))
);


--
-- Name: sesion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sesion (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    ultima_actividad_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    cliente_resumen text,
    device_label text,
    ip_masked text,
    user_agent text,
    CONSTRAINT sesion_actividad_ck CHECK (((ultima_actividad_en >= creada_en) AND (ultima_actividad_en <= expira_en))),
    CONSTRAINT sesion_creacion_expiracion_ck CHECK (((expira_en > creada_en) AND (expira_en <= (creada_en + '08:00:00'::interval)))),
    CONSTRAINT sesion_revocacion_ck CHECK (((revocada_en IS NULL) OR (revocada_en >= creada_en))),
    CONSTRAINT sesion_token_hash_ck CHECK ((token_hash ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: COLUMN sesion.device_label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sesion.device_label IS 'Etiqueta descriptiva del dispositivo o cliente (ej. Chrome en Linux, Safari en iPhone).';


--
-- Name: COLUMN sesion.ip_masked; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sesion.ip_masked IS 'Direccion IP de origen enmascarada para resguardo de privacidad (ej. 190.162.•••.•••).';


--
-- Name: COLUMN sesion.user_agent; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.sesion.user_agent IS 'Encabezado User-Agent sanitizado del cliente HTTP.';


--
-- Name: sesion_administrativa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sesion_administrativa (
    id uuid NOT NULL,
    cuenta_id uuid NOT NULL,
    audiencia text DEFAULT 'admin'::text NOT NULL,
    token_hash character(64) NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    ultima_actividad_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    mfa_verificado_en timestamp with time zone,
    mfa_metodo text DEFAULT ''::text NOT NULL,
    etiqueta_dispositivo text DEFAULT ''::text NOT NULL,
    ip_enmascarada text DEFAULT ''::text NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL,
    CONSTRAINT sesion_administrativa_audiencia_check CHECK ((audiencia = 'admin'::text)),
    CONSTRAINT sesion_administrativa_creada_en_check CHECK (isfinite(creada_en)),
    CONSTRAINT sesion_administrativa_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT sesion_administrativa_ultima_actividad_en_check CHECK (isfinite(ultima_actividad_en))
);


--
-- Name: simulacion_precio_privada; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.simulacion_precio_privada (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    tarifa_version integer NOT NULL,
    modalidad text NOT NULL,
    precio_unitario_clp bigint NOT NULL,
    moneda character(3) NOT NULL,
    unidades_facturadas bigint NOT NULL,
    subtotal_clp bigint NOT NULL,
    inicio timestamp with time zone NOT NULL,
    termino timestamp with time zone NOT NULL,
    zona_horaria text NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT simulacion_precio_privada_check CHECK ((termino > inicio)),
    CONSTRAINT simulacion_precio_privada_modalidad_check CHECK ((modalidad = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text]))),
    CONSTRAINT simulacion_precio_privada_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT simulacion_precio_privada_precio_unitario_clp_min_1000_ck CHECK ((precio_unitario_clp >= 1000)),
    CONSTRAINT simulacion_precio_privada_subtotal_clp_check CHECK ((subtotal_clp > 0)),
    CONSTRAINT simulacion_precio_privada_unidades_facturadas_check CHECK ((unidades_facturadas > 0)),
    CONSTRAINT simulacion_precio_privada_zona_horaria_check CHECK (((length(btrim(zona_horaria)) >= 1) AND (length(btrim(zona_horaria)) <= 100)))
);


--
-- Name: solicitud_privacidad_administrativa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solicitud_privacidad_administrativa (
    id uuid NOT NULL,
    cuenta_id uuid NOT NULL,
    tipo_derecho text NOT NULL,
    estado text DEFAULT 'pendiente'::text NOT NULL,
    motivo text NOT NULL,
    caso_ref text DEFAULT ''::text NOT NULL,
    solicitado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    resuelto_en timestamp with time zone,
    CONSTRAINT solicitud_privacidad_administrativa_actualizado_en_check CHECK (isfinite(actualizado_en)),
    CONSTRAINT solicitud_privacidad_administrativa_creado_en_check CHECK (isfinite(creado_en)),
    CONSTRAINT solicitud_privacidad_administrativa_estado_check CHECK ((estado = ANY (ARRAY['pendiente'::text, 'en_revision'::text, 'aprobada'::text, 'rechazada'::text, 'completada'::text]))),
    CONSTRAINT solicitud_privacidad_administrativa_tipo_derecho_check CHECK ((tipo_derecho = ANY (ARRAY['acceso'::text, 'rectificacion'::text, 'supresion'::text, 'oposicion'::text, 'portabilidad'::text, 'bloqueo'::text])))
);


--
-- Name: solicitud_titular; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solicitud_titular (
    id uuid NOT NULL,
    usuario_id uuid,
    tipo text NOT NULL,
    canal text NOT NULL,
    estado text DEFAULT 'en_revision'::text NOT NULL,
    solicitada_en timestamp with time zone DEFAULT now() NOT NULL,
    resuelta_en timestamp with time zone,
    motivo_resolucion_codigo text,
    retirar_en timestamp with time zone,
    solicitado_por uuid,
    motivo_administrativo text,
    caso_ref_administrativo text,
    respuesta_titular text,
    detalle_titular text,
    asignado_a_id uuid,
    vence_en timestamp with time zone,
    prorrogada_hasta timestamp with time zone,
    prorroga_motivo text,
    version integer DEFAULT 1 NOT NULL,
    folio_publico text DEFAULT ('PRIV-'::text || upper(substr(replace((gen_random_uuid())::text, '-'::text, ''::text), 1, 10))) NOT NULL,
    solicitante_tipo text DEFAULT 'titular'::text NOT NULL,
    solicitante_nombre text,
    solicitante_correo text,
    representante_nombre text,
    representante_relacion text,
    titular_referencia text,
    consulta_token_hash bytea,
    CONSTRAINT solicitud_titular_canal_ck CHECK ((canal = ANY (ARRAY['web'::text, 'api'::text, 'publico'::text, 'correo'::text]))),
    CONSTRAINT solicitud_titular_consulta_token_hash_check CHECK (((consulta_token_hash IS NULL) OR (octet_length(consulta_token_hash) = 32))),
    CONSTRAINT solicitud_titular_detalle_titular_ck CHECK (((detalle_titular IS NULL) OR (((length(btrim(detalle_titular)) >= 1) AND (length(btrim(detalle_titular)) <= 1000)) AND ((tipo <> ALL (ARRAY['rectificacion'::text, 'oposicion'::text, 'bloqueo'::text])) OR (length(btrim(detalle_titular)) >= 10))))),
    CONSTRAINT solicitud_titular_estado_ck CHECK ((estado = ANY (ARRAY['recibida'::text, 'identidad_pendiente'::text, 'en_revision'::text, 'resuelta'::text, 'denegada_fundada'::text]))),
    CONSTRAINT solicitud_titular_identificacion_ck CHECK (((usuario_id IS NOT NULL) OR ((solicitante_nombre IS NOT NULL) AND (solicitante_correo IS NOT NULL)))),
    CONSTRAINT solicitud_titular_motivo_ck CHECK (((motivo_resolucion_codigo IS NULL) OR (motivo_resolucion_codigo = ANY (ARRAY['baja_local_minimizada'::text, 'derecho_tramitado'::text, 'respuesta_acceso_publicada'::text, 'respuesta_derecho_publicada'::text, 'denegacion_fundada'::text])))),
    CONSTRAINT solicitud_titular_prorroga_ck CHECK ((((prorrogada_hasta IS NULL) AND (prorroga_motivo IS NULL)) OR ((prorrogada_hasta IS NOT NULL) AND (vence_en IS NOT NULL) AND (prorrogada_hasta > vence_en) AND (prorrogada_hasta <= (vence_en + '31 days'::interval)) AND (prorroga_motivo IS NOT NULL) AND ((length(btrim(prorroga_motivo)) >= 10) AND (length(btrim(prorroga_motivo)) <= 1000))))),
    CONSTRAINT solicitud_titular_representante_ck CHECK ((((solicitante_tipo = 'titular'::text) AND (representante_nombre IS NULL) AND (representante_relacion IS NULL)) OR ((solicitante_tipo = 'representante'::text) AND (representante_nombre IS NOT NULL) AND (representante_relacion IS NOT NULL)))),
    CONSTRAINT solicitud_titular_representante_nombre_check CHECK (((representante_nombre IS NULL) OR ((char_length(btrim(representante_nombre)) >= 2) AND (char_length(btrim(representante_nombre)) <= 160)))),
    CONSTRAINT solicitud_titular_representante_relacion_check CHECK (((representante_relacion IS NULL) OR (representante_relacion = ANY (ARRAY['mandatario'::text, 'representante_legal'::text, 'tutor_nna'::text])))),
    CONSTRAINT solicitud_titular_resolucion_ck CHECK ((((resuelta_en IS NULL) AND (motivo_resolucion_codigo IS NULL) AND (retirar_en IS NULL)) OR ((resuelta_en IS NOT NULL) AND (motivo_resolucion_codigo IS NOT NULL) AND (retirar_en = (resuelta_en + '5 years'::interval))))),
    CONSTRAINT solicitud_titular_respuesta_ck CHECK (((respuesta_titular IS NULL) OR ((estado = ANY (ARRAY['resuelta'::text, 'denegada_fundada'::text])) AND (resuelta_en IS NOT NULL) AND (motivo_resolucion_codigo = ANY (ARRAY['respuesta_acceso_publicada'::text, 'respuesta_derecho_publicada'::text, 'denegacion_fundada'::text, 'derecho_tramitado'::text, 'baja_local_minimizada'::text])) AND ((length(btrim(respuesta_titular)) >= 10) AND (length(btrim(respuesta_titular)) <= 4000))))),
    CONSTRAINT solicitud_titular_solicitante_correo_check CHECK (((solicitante_correo IS NULL) OR ((char_length(btrim(solicitante_correo)) >= 5) AND (char_length(btrim(solicitante_correo)) <= 254)))),
    CONSTRAINT solicitud_titular_solicitante_nombre_check CHECK (((solicitante_nombre IS NULL) OR ((char_length(btrim(solicitante_nombre)) >= 2) AND (char_length(btrim(solicitante_nombre)) <= 160)))),
    CONSTRAINT solicitud_titular_solicitante_tipo_check CHECK ((solicitante_tipo = ANY (ARRAY['titular'::text, 'representante'::text]))),
    CONSTRAINT solicitud_titular_tipo_ck CHECK ((tipo = ANY (ARRAY['acceso'::text, 'rectificacion'::text, 'supresion'::text, 'oposicion'::text, 'portabilidad'::text, 'bloqueo'::text]))),
    CONSTRAINT solicitud_titular_titular_referencia_check CHECK (((titular_referencia IS NULL) OR ((char_length(btrim(titular_referencia)) >= 2) AND (char_length(btrim(titular_referencia)) <= 200)))),
    CONSTRAINT solicitud_titular_version_ck CHECK ((version >= 1))
);


--
-- Name: solicitud_titular_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.solicitud_titular_evento (
    id uuid NOT NULL,
    solicitud_id uuid NOT NULL,
    secuencia integer NOT NULL,
    tipo_evento text NOT NULL,
    visibilidad text NOT NULL,
    estado_anterior text,
    estado_nuevo text,
    actor_id uuid,
    detalle text,
    vence_en_nuevo timestamp with time zone,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT solicitud_titular_evento_detalle_ck CHECK (((detalle IS NULL) OR ((length(btrim(detalle)) >= 1) AND (length(btrim(detalle)) <= 2000)))),
    CONSTRAINT solicitud_titular_evento_secuencia_check CHECK ((secuencia >= 1)),
    CONSTRAINT solicitud_titular_evento_tipo_evento_ck CHECK ((tipo_evento = ANY (ARRAY['recepcion'::text, 'asignacion'::text, 'identidad_verificada'::text, 'en_revision'::text, 'aclaracion_solicitada'::text, 'nota_interna'::text, 'prorroga'::text, 'ejecucion_derecho'::text, 'resolucion'::text, 'despacho_registrado'::text]))),
    CONSTRAINT solicitud_titular_evento_visibilidad_check CHECK ((visibilidad = ANY (ARRAY['titular'::text, 'interna'::text]))),
    CONSTRAINT solicitud_titular_evento_visibilidad_ck CHECK (((tipo_evento <> 'nota_interna'::text) OR (visibilidad = 'interna'::text)))
);


--
-- Name: space_kyb_verification; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_kyb_verification (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    espacio_id uuid NOT NULL,
    host_mode character varying(16) NOT NULL,
    owner_usuario_id uuid NOT NULL,
    organizacion_id uuid,
    kyb_verification_id uuid,
    document_storage_ref uuid NOT NULL,
    mime_type character varying(32) NOT NULL,
    size_bytes bigint NOT NULL,
    content_sha256 bytea NOT NULL,
    status character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    rejection_reason character varying(500),
    submitted_by uuid NOT NULL,
    submitted_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    reviewer_id uuid,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    document_kind character varying(40) DEFAULT 'property_or_sublease_authorization'::character varying NOT NULL,
    sii_giro_code character varying(6),
    categoria_codigo text,
    CONSTRAINT space_kyb_verification_check CHECK (((((host_mode)::text = 'empresa'::text) AND (organizacion_id IS NOT NULL)) OR (((host_mode)::text = 'personal'::text) AND (organizacion_id IS NULL)))),
    CONSTRAINT space_kyb_verification_check1 CHECK (((((status)::text = 'REJECTED'::text) AND (rejection_reason IS NOT NULL) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text <> 'REJECTED'::text) AND (rejection_reason IS NULL)))),
    CONSTRAINT space_kyb_verification_check2 CHECK (((((status)::text = ANY ((ARRAY['APPROVED'::character varying, 'REJECTED'::character varying])::text[])) AND (reviewer_id IS NOT NULL) AND (reviewed_at IS NOT NULL)) OR (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[])) AND (reviewer_id IS NULL) AND (reviewed_at IS NULL)))),
    CONSTRAINT space_kyb_verification_content_sha256_check CHECK ((octet_length(content_sha256) = 32)),
    CONSTRAINT space_kyb_verification_document_kind_check CHECK (((document_kind)::text = ANY ((ARRAY['property_or_sublease_authorization'::character varying, 'sii_start_activities'::character varying])::text[]))),
    CONSTRAINT space_kyb_verification_host_mode_check CHECK (((host_mode)::text = ANY ((ARRAY['personal'::character varying, 'empresa'::character varying])::text[]))),
    CONSTRAINT space_kyb_verification_mime_type_check CHECK (((mime_type)::text = ANY ((ARRAY['application/pdf'::character varying, 'image/jpeg'::character varying, 'image/png'::character varying])::text[]))),
    CONSTRAINT space_kyb_verification_sii_fields_ck CHECK (((((document_kind)::text = 'sii_start_activities'::text) AND ((host_mode)::text = 'empresa'::text) AND (organizacion_id IS NOT NULL) AND (sii_giro_code IS NOT NULL) AND (categoria_codigo IS NOT NULL)) OR (((document_kind)::text = 'property_or_sublease_authorization'::text) AND (sii_giro_code IS NULL)))),
    CONSTRAINT space_kyb_verification_sii_giro_code_check CHECK (((sii_giro_code IS NULL) OR ((sii_giro_code)::text ~ '^[0-9]{6}$'::text))),
    CONSTRAINT space_kyb_verification_size_bytes_check CHECK (((size_bytes >= 1) AND (size_bytes <= 10485760))),
    CONSTRAINT space_kyb_verification_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying])::text[])))
);


--
-- Name: space_media; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_media (
    id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    propietario_id uuid NOT NULL,
    media_id uuid NOT NULL,
    alt_text character varying(250) DEFAULT ''::character varying NOT NULL,
    visible boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    retired_at timestamp with time zone,
    CONSTRAINT space_media_alt_text_check CHECK ((length((alt_text)::text) <= 250)),
    CONSTRAINT space_media_retirement_ck CHECK (((retired_at IS NULL) OR (NOT visible)))
);


--
-- Name: tarifa_espacio; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tarifa_espacio (
    espacio_id uuid NOT NULL,
    version integer NOT NULL,
    modalidad text NOT NULL,
    precio_base_clp bigint NOT NULL,
    moneda character(3) DEFAULT 'CLP'::bpchar NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tarifa_espacio_modalidad_check CHECK ((modalidad = ANY (ARRAY['hora'::text, 'dia'::text, 'mes'::text]))),
    CONSTRAINT tarifa_espacio_moneda_check CHECK ((moneda = 'CLP'::bpchar)),
    CONSTRAINT tarifa_espacio_precio_base_clp_min_1000_ck CHECK ((precio_base_clp >= 1000)),
    CONSTRAINT tarifa_espacio_version_check CHECK ((version > 0))
);


--
-- Name: ticket_soporte_administrativo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_soporte_administrativo (
    id uuid NOT NULL,
    cuenta_id uuid,
    asignado_a_id uuid,
    categoria text NOT NULL,
    prioridad text DEFAULT 'media'::text NOT NULL,
    estado text DEFAULT 'abierto'::text NOT NULL,
    asunto text NOT NULL,
    caso_ref text DEFAULT ''::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ticket_soporte_administrativo_actualizado_en_check CHECK (isfinite(actualizado_en)),
    CONSTRAINT ticket_soporte_administrativo_creado_en_check CHECK (isfinite(creado_en)),
    CONSTRAINT ticket_soporte_administrativo_estado_check CHECK ((estado = ANY (ARRAY['abierto'::text, 'en_progreso'::text, 'resuelto'::text, 'cerrado'::text]))),
    CONSTRAINT ticket_soporte_administrativo_prioridad_check CHECK ((prioridad = ANY (ARRAY['baja'::text, 'media'::text, 'alta'::text, 'critica'::text])))
);


--
-- Name: token_accion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.token_accion (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    proposito text NOT NULL,
    token_hash character(64) NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    consumido_en timestamp with time zone,
    invalidado_en timestamp with time zone,
    intentos integer DEFAULT 0 NOT NULL,
    CONSTRAINT token_accion_consumido_ck CHECK (((consumido_en IS NULL) OR (consumido_en >= creado_en))),
    CONSTRAINT token_accion_creacion_expiracion_ck CHECK ((expira_en > creado_en)),
    CONSTRAINT token_accion_intentos_ck CHECK (((intentos >= 0) AND (intentos <= 5))),
    CONSTRAINT token_accion_invalidado_ck CHECK (((invalidado_en IS NULL) OR (invalidado_en >= creado_en))),
    CONSTRAINT token_accion_proposito_ck CHECK ((proposito = ANY (ARRAY['verificar_correo'::text, 'recuperar_clave'::text]))),
    CONSTRAINT token_accion_terminal_mutuo_ck CHECK (((consumido_en IS NULL) OR (invalidado_en IS NULL))),
    CONSTRAINT token_accion_token_hash_ck CHECK ((token_hash ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: usuario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.usuario (
    id uuid NOT NULL,
    correo_original text NOT NULL,
    correo_normalizado text NOT NULL,
    hash_clave text NOT NULL,
    estado text DEFAULT 'correo_pendiente'::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    baja_solicitada_en timestamp with time zone,
    intentos_fallidos_consecutivos integer DEFAULT 0 NOT NULL,
    bloqueado_hasta timestamp with time zone,
    preferencia_uso text,
    baja_iniciada_en timestamp with time zone,
    CONSTRAINT usuario_estado_ck CHECK ((estado = ANY (ARRAY['correo_pendiente'::text, 'activo'::text, 'bloqueado'::text, 'baja_solicitada'::text, 'desidentificado'::text]))),
    CONSTRAINT usuario_intentos_fallidos_ck CHECK ((intentos_fallidos_consecutivos >= 0)),
    CONSTRAINT usuario_preferencia_uso_ck CHECK (((preferencia_uso IS NULL) OR (preferencia_uso = ANY (ARRAY['ofrecer'::text, 'arrendar'::text]))))
);


--
-- Name: verificacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.verificacion (
    id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    tipo text NOT NULL,
    estado text DEFAULT 'en_revision'::text NOT NULL,
    proveedor_ref text DEFAULT 'local-fixture-v1'::text NOT NULL,
    referencia_evidencia text NOT NULL,
    clave_idempotencia text NOT NULL,
    reintento_de uuid,
    revisor_id uuid,
    motivo_codigo text,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    resuelta_en timestamp with time zone,
    retirar_en timestamp with time zone,
    retirada_privacidad_en timestamp with time zone,
    correccion_codigo text,
    revocada_en timestamp with time zone,
    revocada_por uuid,
    motivo_revocacion_codigo text,
    CONSTRAINT verificacion_correccion_codigo_ck CHECK (((correccion_codigo IS NULL) OR (correccion_codigo = ANY (ARRAY['fixture_vigente_actualizado'::text, 'antecedentes_fixture_actualizados'::text, 'inicio_actividades_fixture_actualizadas'::text, 'legado_pre_v28_sin_codigo'::text])))),
    CONSTRAINT verificacion_correccion_retry_ck CHECK ((((reintento_de IS NULL) AND (correccion_codigo IS NULL)) OR ((reintento_de IS NOT NULL) AND (correccion_codigo IS NOT NULL)))),
    CONSTRAINT verificacion_estado_ck CHECK ((estado = ANY (ARRAY['en_revision'::text, 'aprobada'::text, 'rechazada'::text, 'revocada'::text, 'retirada_privacidad'::text]))),
    CONSTRAINT verificacion_idempotencia_ck CHECK (((length(clave_idempotencia) >= 8) AND (length(clave_idempotencia) <= 80))),
    CONSTRAINT verificacion_motivo_ck CHECK ((((estado = 'rechazada'::text) AND (revisor_id IS NOT NULL) AND (motivo_codigo IS NOT NULL) AND (resuelta_en IS NOT NULL)) OR ((estado = 'aprobada'::text) AND (revisor_id IS NOT NULL) AND (motivo_codigo IS NULL) AND (resuelta_en IS NOT NULL)) OR ((estado = 'en_revision'::text) AND (revisor_id IS NULL) AND (motivo_codigo IS NULL) AND (resuelta_en IS NULL)) OR ((estado = 'revocada'::text) AND (revisor_id IS NOT NULL) AND (motivo_codigo IS NULL) AND (resuelta_en IS NOT NULL) AND (revocada_en IS NOT NULL) AND (revocada_por IS NOT NULL) AND (motivo_revocacion_codigo IS NOT NULL)) OR ((estado = 'retirada_privacidad'::text) AND (revisor_id IS NULL) AND (motivo_codigo = 'baja_privacidad'::text) AND (resuelta_en IS NOT NULL)))),
    CONSTRAINT verificacion_proveedor_ck CHECK ((proveedor_ref = 'local-fixture-v1'::text)),
    CONSTRAINT verificacion_reason_code_ck CHECK (((motivo_codigo IS NULL) OR (motivo_codigo = ANY (ARRAY['documento_vencido'::text, 'antecedentes_incompletos'::text, 'inicio_actividades_no_confirmado'::text, 'baja_privacidad'::text])))),
    CONSTRAINT verificacion_ref_sintetica_ck CHECK ((referencia_evidencia ~ '^fixture:[0-9a-f-]{36}$'::text)),
    CONSTRAINT verificacion_reintento_no_self_ck CHECK (((reintento_de IS NULL) OR (reintento_de <> id))),
    CONSTRAINT verificacion_retirada_privacidad_ck CHECK ((((estado = 'retirada_privacidad'::text) AND (retirada_privacidad_en IS NOT NULL)) OR ((estado <> 'retirada_privacidad'::text) AND (retirada_privacidad_en IS NULL)))),
    CONSTRAINT verificacion_revocacion_codigo_ck CHECK ((((revocada_en IS NULL) AND (revocada_por IS NULL) AND (motivo_revocacion_codigo IS NULL)) OR ((revocada_en IS NOT NULL) AND (revocada_por IS NOT NULL) AND (motivo_revocacion_codigo = ANY (ARRAY['aprobacion_fixture_incorrecta'::text, 'revision_fixture_actualizada'::text]))))),
    CONSTRAINT verificacion_tipo_ck CHECK ((tipo = ANY (ARRAY['kyc'::text, 'kyb'::text])))
);


--
-- Name: verificacion_archivo_retiro_pendiente_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.verificacion_archivo_retiro_pendiente_local (
    evidencia_id uuid CONSTRAINT verificacion_archivo_retiro_pendiente_loc_evidencia_id_not_null NOT NULL,
    archivo_id uuid NOT NULL,
    disponible_en timestamp with time zone CONSTRAINT verificacion_archivo_retiro_pendiente_lo_disponible_en_not_null NOT NULL,
    lease_hasta timestamp with time zone,
    intentos integer DEFAULT 0 NOT NULL,
    ultimo_codigo_error text,
    CONSTRAINT verificacion_archivo_retiro_error_ck CHECK (((ultimo_codigo_error IS NULL) OR (ultimo_codigo_error = 'archivo_sintetico_no_retirado'::text))),
    CONSTRAINT verificacion_archivo_retiro_pendiente_local_intentos_check CHECK ((intentos >= 0))
);


--
-- Name: TABLE verificacion_archivo_retiro_pendiente_local; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.verificacion_archivo_retiro_pendiente_local IS 'Durable, retryable deletion of local synthetic verification blobs; contains no user or provider payload.';


--
-- Name: verificacion_evidencia_sintetica; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.verificacion_evidencia_sintetica (
    id uuid NOT NULL,
    verificacion_id uuid NOT NULL,
    codigo_fixture text NOT NULL,
    mime_type text NOT NULL,
    tamano_bytes bigint NOT NULL,
    sha256 character(64) NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    archivo_id uuid,
    retirar_archivo_en timestamp with time zone,
    archivo_retirado_en timestamp with time zone,
    CONSTRAINT verificacion_evidencia_archivo_retencion_ck CHECK ((((archivo_id IS NULL) AND (archivo_retirado_en IS NOT NULL)) OR ((archivo_id IS NOT NULL) AND (archivo_retirado_en IS NULL)))),
    CONSTRAINT verificacion_evidencia_fixture_ck CHECK ((codigo_fixture = 'synthetic-png-v1'::text)),
    CONSTRAINT verificacion_evidencia_mime_ck CHECK ((mime_type = 'image/png'::text)),
    CONSTRAINT verificacion_evidencia_sha_ck CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT verificacion_evidencia_size_ck CHECK (((tamano_bytes >= 1) AND (tamano_bytes <= 1048576)))
);


--
-- Name: verificacion_historial_local; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.verificacion_historial_local (
    id bigint NOT NULL,
    verificacion_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    accion text NOT NULL,
    estado_anterior text,
    estado_nuevo text NOT NULL,
    motivo_codigo text,
    correlacion_id text NOT NULL,
    clave_idempotencia text,
    ocurrida_en timestamp with time zone NOT NULL,
    CONSTRAINT verificacion_historial_estado_ck CHECK ((((accion = 'solicitada'::text) AND (estado_anterior IS NULL) AND (estado_nuevo = 'en_revision'::text)) OR ((accion = 'revision_aprobada'::text) AND (estado_anterior = 'en_revision'::text) AND (estado_nuevo = 'aprobada'::text) AND (motivo_codigo IS NULL)) OR ((accion = 'revision_rechazada'::text) AND (estado_anterior = 'en_revision'::text) AND (estado_nuevo = 'rechazada'::text) AND (motivo_codigo IS NOT NULL)) OR ((accion = 'subsanacion_solicitada'::text) AND (estado_anterior = 'rechazada'::text) AND (estado_nuevo = 'rechazada'::text) AND (motivo_codigo IS NOT NULL)) OR ((accion = 'elegibilidad_revocada'::text) AND (estado_anterior = 'aprobada'::text) AND (estado_nuevo = 'revocada'::text) AND (motivo_codigo IS NOT NULL)) OR ((accion = 'elegibilidad_retirada_privacidad'::text) AND (estado_anterior = 'aprobada'::text) AND (estado_nuevo = 'retirada_privacidad'::text) AND (motivo_codigo = 'baja_privacidad'::text)) OR ((accion = 'estado_observado_en_migracion'::text) AND (estado_anterior IS NULL) AND (estado_nuevo = 'retirada_privacidad'::text)))),
    CONSTRAINT verificacion_historial_local_accion_check CHECK ((accion = ANY (ARRAY['solicitada'::text, 'revision_aprobada'::text, 'revision_rechazada'::text, 'subsanacion_solicitada'::text, 'elegibilidad_revocada'::text, 'elegibilidad_retirada_privacidad'::text, 'estado_observado_en_migracion'::text]))),
    CONSTRAINT verificacion_historial_local_correlacion_id_check CHECK (((length(btrim(correlacion_id)) >= 1) AND (length(btrim(correlacion_id)) <= 120))),
    CONSTRAINT verificacion_historial_local_estado_nuevo_check CHECK ((estado_nuevo = ANY (ARRAY['en_revision'::text, 'aprobada'::text, 'rechazada'::text, 'revocada'::text, 'retirada_privacidad'::text])))
);


--
-- Name: verificacion_historial_local_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.verificacion_historial_local ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.verificacion_historial_local_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: version_terminos; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.version_terminos (
    id uuid NOT NULL,
    codigo text NOT NULL,
    tipo text NOT NULL,
    hash_sha256 character(64) NOT NULL,
    publicada_en timestamp with time zone NOT NULL,
    CONSTRAINT version_terminos_hash_ck CHECK ((hash_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT version_terminos_tipo_ck CHECK ((tipo = ANY (ARRAY['terminos'::text, 'privacidad'::text, 'politica_arrendador'::text])))
);


--
-- Name: vista_asistida_administrativa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vista_asistida_administrativa (
    id uuid NOT NULL,
    sesion_administrativa_id uuid NOT NULL,
    actor_cuenta_id uuid NOT NULL,
    objetivo_cuenta_id uuid NOT NULL,
    rol_vista text NOT NULL,
    caso_id text NOT NULL,
    motivo text NOT NULL,
    creada_en timestamp with time zone DEFAULT now() NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    revocada_en timestamp with time zone,
    CONSTRAINT vista_asistida_actor_distinto_objetivo CHECK ((actor_cuenta_id <> objetivo_cuenta_id)),
    CONSTRAINT vista_asistida_administrativa_creada_en_check CHECK (isfinite(creada_en)),
    CONSTRAINT vista_asistida_administrativa_expira_en_check CHECK (isfinite(expira_en)),
    CONSTRAINT vista_asistida_administrativa_rol_vista_check CHECK ((rol_vista = ANY (ARRAY['arrendador'::text, 'arrendatario'::text])))
);


--
-- Name: cuenta_cobro_sintetica_historial_local id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_historial_local ALTER COLUMN id SET DEFAULT nextval('public.cuenta_cobro_sintetica_historial_local_id_seq'::regclass);


--
-- Name: reserva_finanzas_historial_ensayo_local secuencia; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_finanzas_historial_ensayo_local ALTER COLUMN secuencia SET DEFAULT nextval('public.reserva_finanzas_historial_ensayo_local_secuencia_seq'::regclass);


--
-- Name: acceso_sensible_administrativo acceso_sensible_administrativo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceso_sensible_administrativo
    ADD CONSTRAINT acceso_sensible_administrativo_pkey PRIMARY KEY (id);


--
-- Name: account_identity_profile_draft account_identity_profile_draft_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_draft
    ADD CONSTRAINT account_identity_profile_draft_pkey PRIMARY KEY (account_id);


--
-- Name: account_identity_profile_phone account_identity_profile_phone_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_phone
    ADD CONSTRAINT account_identity_profile_phone_pkey PRIMARY KEY (account_id);


--
-- Name: account_identity_profile account_identity_profile_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile
    ADD CONSTRAINT account_identity_profile_pkey PRIMARY KEY (account_id);


--
-- Name: account_identity_profile_revision account_identity_profile_revision_account_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_revision
    ADD CONSTRAINT account_identity_profile_revision_account_id_id_key UNIQUE (account_id, id);


--
-- Name: account_identity_profile_revision account_identity_profile_revision_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_revision
    ADD CONSTRAINT account_identity_profile_revision_pkey PRIMARY KEY (id);


--
-- Name: account_identity_rut_claim account_identity_rut_claim_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_claim
    ADD CONSTRAINT account_identity_rut_claim_pkey PRIMARY KEY (account_id);


--
-- Name: account_identity_rut_fingerprint account_identity_rut_fingerprint_account_id_fingerprint_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_fingerprint
    ADD CONSTRAINT account_identity_rut_fingerprint_account_id_fingerprint_key UNIQUE (account_id, fingerprint);


--
-- Name: account_identity_rut_fingerprint account_identity_rut_fingerprint_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_fingerprint
    ADD CONSTRAINT account_identity_rut_fingerprint_pkey PRIMARY KEY (fingerprint);


--
-- Name: account_identity_verification account_identity_verification_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_verification
    ADD CONSTRAINT account_identity_verification_pkey PRIMARY KEY (id);


--
-- Name: account_identity_verification account_identity_verification_provider_session_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_verification
    ADD CONSTRAINT account_identity_verification_provider_session_key UNIQUE (provider_session);


--
-- Name: account_identity_webhook_receipt account_identity_webhook_receipt_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_webhook_receipt
    ADD CONSTRAINT account_identity_webhook_receipt_pkey PRIMARY KEY (event_id);


--
-- Name: account_pinpass_access account_pinpass_access_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_access
    ADD CONSTRAINT account_pinpass_access_pkey PRIMARY KEY (account_id, session_id);


--
-- Name: account_pinpass_attempt account_pinpass_attempt_scope_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_attempt
    ADD CONSTRAINT account_pinpass_attempt_scope_uk UNIQUE NULLS NOT DISTINCT (account_id, session_id);


--
-- Name: account_pinpass_audit account_pinpass_audit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_audit
    ADD CONSTRAINT account_pinpass_audit_pkey PRIMARY KEY (id);


--
-- Name: account_pinpass_email_challenge account_pinpass_email_challenge_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_email_challenge
    ADD CONSTRAINT account_pinpass_email_challenge_pkey PRIMARY KEY (account_id);


--
-- Name: account_pinpass account_pinpass_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass
    ADD CONSTRAINT account_pinpass_pkey PRIMARY KEY (account_id);


--
-- Name: aceptacion_terminos aceptacion_terminos_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aceptacion_terminos
    ADD CONSTRAINT aceptacion_terminos_pk PRIMARY KEY (id);


--
-- Name: aceptacion_terminos aceptacion_terminos_usuario_version_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aceptacion_terminos
    ADD CONSTRAINT aceptacion_terminos_usuario_version_uk UNIQUE (usuario_id, version_id);


--
-- Name: administrador_global_bootstrap_evento administrador_global_bootstrap_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global_bootstrap_evento
    ADD CONSTRAINT administrador_global_bootstrap_evento_pkey PRIMARY KEY (id);


--
-- Name: administrador_global_bootstrap administrador_global_bootstrap_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global_bootstrap
    ADD CONSTRAINT administrador_global_bootstrap_pkey PRIMARY KEY (singleton);


--
-- Name: administrador_global_bootstrap administrador_global_bootstrap_usuario_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global_bootstrap
    ADD CONSTRAINT administrador_global_bootstrap_usuario_id_key UNIQUE (usuario_id);


--
-- Name: administrador_global administrador_global_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global
    ADD CONSTRAINT administrador_global_pkey PRIMARY KEY (usuario_id);


--
-- Name: anfitrion_cumplimiento_documento anfitrion_cumplimiento_documento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_documento
    ADD CONSTRAINT anfitrion_cumplimiento_documento_pkey PRIMARY KEY (id);


--
-- Name: anfitrion_cumplimiento_documento anfitrion_cumplimiento_documento_storage_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_documento
    ADD CONSTRAINT anfitrion_cumplimiento_documento_storage_ref_key UNIQUE (storage_ref);


--
-- Name: anfitrion_cumplimiento_personal anfitrion_cumplimiento_personal_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_personal
    ADD CONSTRAINT anfitrion_cumplimiento_personal_pkey PRIMARY KEY (usuario_id);


--
-- Name: aprobacion_administrativa aprobacion_administrativa_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aprobacion_administrativa
    ADD CONSTRAINT aprobacion_administrativa_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: aprobacion_administrativa aprobacion_administrativa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aprobacion_administrativa
    ADD CONSTRAINT aprobacion_administrativa_pkey PRIMARY KEY (id);


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_arriendo_id_actor_id_clase_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_confirmacion
    ADD CONSTRAINT arriendo_temprano_confirmacion_arriendo_id_actor_id_clase_key UNIQUE (arriendo_id, actor_id, clase);


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_id_arriendo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_confirmacion
    ADD CONSTRAINT arriendo_temprano_confirmacion_id_arriendo_id_key UNIQUE (id, arriendo_id);


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_confirmacion
    ADD CONSTRAINT arriendo_temprano_confirmacion_pkey PRIMARY KEY (id);


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_arriendo_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_arriendo_id_revision_key UNIQUE (arriendo_id, revision);


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_arriendo_id_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_arriendo_id_secuencia_key UNIQUE (arriendo_id, secuencia);


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_id_arriendo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_id_arriendo_id_key UNIQUE (id, arriendo_id);


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_pkey PRIMARY KEY (id);


--
-- Name: arriendo_temprano arriendo_temprano_id_space_interval_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_id_space_interval_uk UNIQUE (id, espacio_id, intervalo, ocupacion_id);


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotenci_actor_id_arriendo_id_comando__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_idempotencia
    ADD CONSTRAINT arriendo_temprano_idempotenci_actor_id_arriendo_id_comando__key UNIQUE (actor_id, arriendo_id, comando, clave);


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotencia_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_idempotencia
    ADD CONSTRAINT arriendo_temprano_idempotencia_pkey PRIMARY KEY (id);


--
-- Name: arriendo_temprano arriendo_temprano_ocupacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_ocupacion_id_key UNIQUE (ocupacion_id);


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_arriendo_id_pago_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_arriendo_id_pago_id_key UNIQUE (arriendo_id, pago_id);


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_pago_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_pago_id_key UNIQUE (pago_id);


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_pkey PRIMARY KEY (arriendo_id);


--
-- Name: arriendo_temprano arriendo_temprano_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_pkey PRIMARY KEY (id);


--
-- Name: atributo_filtro_publico atributo_filtro_publico_categoria_codigo_perfil_version_ord_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.atributo_filtro_publico
    ADD CONSTRAINT atributo_filtro_publico_categoria_codigo_perfil_version_ord_key UNIQUE (categoria_codigo, perfil_version, orden);


--
-- Name: atributo_filtro_publico atributo_filtro_publico_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.atributo_filtro_publico
    ADD CONSTRAINT atributo_filtro_publico_pkey PRIMARY KEY (categoria_codigo, perfil_version, codigo);


--
-- Name: autorizacion_arriendo_comercial_evento autorizacion_arriendo_comercial_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial_evento
    ADD CONSTRAINT autorizacion_arriendo_comercial_evento_pkey PRIMARY KEY (id);


--
-- Name: autorizacion_arriendo_comercial autorizacion_arriendo_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial
    ADD CONSTRAINT autorizacion_arriendo_comercial_pkey PRIMARY KEY (id);


--
-- Name: autorizacion_firma_desafio_otp autorizacion_firma_desafio_otp_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_firma_desafio_otp
    ADD CONSTRAINT autorizacion_firma_desafio_otp_pkey PRIMARY KEY (id);


--
-- Name: aviso_local_ciclo aviso_local_ciclo_aviso_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_ciclo
    ADD CONSTRAINT aviso_local_ciclo_aviso_id_clave_idempotencia_key UNIQUE (aviso_id, clave_idempotencia);


--
-- Name: aviso_local_ciclo aviso_local_ciclo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_ciclo
    ADD CONSTRAINT aviso_local_ciclo_pkey PRIMARY KEY (aviso_id, ciclo);


--
-- Name: aviso_local aviso_local_clave_deduplicacion_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local
    ADD CONSTRAINT aviso_local_clave_deduplicacion_key UNIQUE (clave_deduplicacion);


--
-- Name: aviso_local aviso_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local
    ADD CONSTRAINT aviso_local_pkey PRIMARY KEY (id);


--
-- Name: aviso_local_recuperacion_auditoria aviso_local_recuperacion_audito_aviso_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_recuperacion_auditoria
    ADD CONSTRAINT aviso_local_recuperacion_audito_aviso_id_clave_idempotencia_key UNIQUE (aviso_id, clave_idempotencia);


--
-- Name: aviso_local_recuperacion_auditoria aviso_local_recuperacion_auditoria_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_recuperacion_auditoria
    ADD CONSTRAINT aviso_local_recuperacion_auditoria_pkey PRIMARY KEY (id);


--
-- Name: baja_archivo_pendiente_local baja_archivo_pendiente_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.baja_archivo_pendiente_local
    ADD CONSTRAINT baja_archivo_pendiente_local_pkey PRIMARY KEY (ejecucion_id, evidencia_id);


--
-- Name: bloqueo_cuenta_administrativo_local bloqueo_cuenta_administrativo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_administrativo_local
    ADD CONSTRAINT bloqueo_cuenta_administrativo_local_pkey PRIMARY KEY (cuenta_id);


--
-- Name: bloqueo_cuenta_historial_local bloqueo_cuenta_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_historial_local
    ADD CONSTRAINT bloqueo_cuenta_historial_local_pkey PRIMARY KEY (id);


--
-- Name: caso_soporte_reserva_comercial caso_soporte_reserva_comercia_reserva_id_iniciador_id_clave_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caso_soporte_reserva_comercial
    ADD CONSTRAINT caso_soporte_reserva_comercia_reserva_id_iniciador_id_clave_key UNIQUE (reserva_id, iniciador_id, clave_idempotencia);


--
-- Name: caso_soporte_reserva_comercial caso_soporte_reserva_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caso_soporte_reserva_comercial
    ADD CONSTRAINT caso_soporte_reserva_comercial_pkey PRIMARY KEY (id);


--
-- Name: catalogo_ubicacion_publica catalogo_ubicacion_publica_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogo_ubicacion_publica
    ADD CONSTRAINT catalogo_ubicacion_publica_pkey PRIMARY KEY (version);


--
-- Name: categoria_espacio categoria_espacio_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categoria_espacio
    ADD CONSTRAINT categoria_espacio_nombre_key UNIQUE (nombre);


--
-- Name: categoria_espacio categoria_espacio_orden_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categoria_espacio
    ADD CONSTRAINT categoria_espacio_orden_key UNIQUE (orden);


--
-- Name: categoria_espacio categoria_espacio_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categoria_espacio
    ADD CONSTRAINT categoria_espacio_pkey PRIMARY KEY (codigo);


--
-- Name: categoria_perfil_atributos categoria_perfil_atributos_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categoria_perfil_atributos
    ADD CONSTRAINT categoria_perfil_atributos_pkey PRIMARY KEY (categoria_codigo, version);


--
-- Name: checkout_cancelacion_pendiente checkout_cancelacion_pendiente_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_cancelacion_pendiente
    ADD CONSTRAINT checkout_cancelacion_pendiente_clave_idempotencia_key UNIQUE (clave_idempotencia);


--
-- Name: checkout_cancelacion_pendiente checkout_cancelacion_pendiente_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_cancelacion_pendiente
    ADD CONSTRAINT checkout_cancelacion_pendiente_pkey PRIMARY KEY (operacion_id);


--
-- Name: checkout_payment_notice_outbox checkout_payment_notice_outbox_dedupe_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_payment_notice_outbox
    ADD CONSTRAINT checkout_payment_notice_outbox_dedupe_key_key UNIQUE (dedupe_key);


--
-- Name: checkout_payment_notice_outbox checkout_payment_notice_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_payment_notice_outbox
    ADD CONSTRAINT checkout_payment_notice_outbox_pkey PRIMARY KEY (id);


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_pkey PRIMARY KEY (id);


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_solicitud_comercial_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_solicitud_comercial_id_key UNIQUE (solicitud_comercial_id);


--
-- Name: comuna_publica comuna_publica_catalogo_version_region_codigo_codigo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comuna_publica
    ADD CONSTRAINT comuna_publica_catalogo_version_region_codigo_codigo_key UNIQUE (catalogo_version, region_codigo, codigo);


--
-- Name: comuna_publica comuna_publica_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comuna_publica
    ADD CONSTRAINT comuna_publica_pkey PRIMARY KEY (catalogo_version, codigo);


--
-- Name: contrato_documento_comercial contrato_documento_comercial_object_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_object_id_key UNIQUE (object_id);


--
-- Name: contrato_documento_comercial contrato_documento_comercial_operacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_operacion_id_key UNIQUE (operacion_id);


--
-- Name: contrato_documento_comercial contrato_documento_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_pkey PRIMARY KEY (id);


--
-- Name: contrato_documento_comercial contrato_documento_comercial_solicitud_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_solicitud_id_version_key UNIQUE (solicitud_id, version);


--
-- Name: contrato_ensayo_firma contrato_ensayo_firma_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_firma
    ADD CONSTRAINT contrato_ensayo_firma_pkey PRIMARY KEY (contrato_id, firmante_id);


--
-- Name: contrato_ensayo_historial contrato_ensayo_historial_contrato_id_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_historial
    ADD CONSTRAINT contrato_ensayo_historial_contrato_id_secuencia_key UNIQUE (contrato_id, secuencia);


--
-- Name: contrato_ensayo_historial contrato_ensayo_historial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_historial
    ADD CONSTRAINT contrato_ensayo_historial_pkey PRIMARY KEY (id);


--
-- Name: contrato_ensayo_local contrato_ensayo_local_documento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_documento_id_key UNIQUE (documento_id);


--
-- Name: contrato_ensayo_local contrato_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: contrato_ensayo_local contrato_ensayo_local_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_reserva_id_key UNIQUE (reserva_id);


--
-- Name: contrato_ensayo_local contrato_ensayo_local_reserva_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_reserva_id_version_key UNIQUE (reserva_id, version);


--
-- Name: contrato_firma_comercial contrato_firma_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_comercial
    ADD CONSTRAINT contrato_firma_comercial_pkey PRIMARY KEY (id);


--
-- Name: contrato_firma_comercial contrato_firma_comercial_solicitud_id_rol_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_comercial
    ADD CONSTRAINT contrato_firma_comercial_solicitud_id_rol_key UNIQUE (solicitud_id, rol);


--
-- Name: contrato_firma_desafio_otp contrato_firma_desafio_otp_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_desafio_otp
    ADD CONSTRAINT contrato_firma_desafio_otp_pkey PRIMARY KEY (id);


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_cotizacion_id_documento_sha256_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_cotizacion_id_documento_sha256_key UNIQUE (cotizacion_id, documento_sha256);


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_cotizacion_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_cotizacion_id_version_key UNIQUE (cotizacion_id, version);


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_pkey PRIMARY KEY (id);


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_arrendatario_id_idempotency_ke_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_arrendatario_id_idempotency_ke_key UNIQUE (arrendatario_id, idempotency_key);


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_pkey PRIMARY KEY (id);


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_propuesta_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_propuesta_id_key UNIQUE (propuesta_id);


--
-- Name: contrato_solicitud_evento_comercial contrato_solicitud_evento_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_evento_comercial
    ADD CONSTRAINT contrato_solicitud_evento_comercial_pkey PRIMARY KEY (id);


--
-- Name: contrato_solicitud_evento_comercial contrato_solicitud_evento_comercial_solicitud_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_evento_comercial
    ADD CONSTRAINT contrato_solicitud_evento_comercial_solicitud_id_revision_key UNIQUE (solicitud_id, revision);


--
-- Name: contrato_solicitud_notice_outbox contrato_solicitud_notice_out_solicitud_id_destinatario_id__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_notice_outbox
    ADD CONSTRAINT contrato_solicitud_notice_out_solicitud_id_destinatario_id__key UNIQUE (solicitud_id, destinatario_id, tipo);


--
-- Name: contrato_solicitud_notice_outbox contrato_solicitud_notice_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_notice_outbox
    ADD CONSTRAINT contrato_solicitud_notice_outbox_pkey PRIMARY KEY (id);


--
-- Name: contrato_solicitud_comercial contrato_solicitud_ocupacion_ref_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_ocupacion_ref_uk UNIQUE (id, espacio_id, ocupacion_id);


--
-- Name: contrato_solicitud_quote_activa contrato_solicitud_quote_activa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_quote_activa
    ADD CONSTRAINT contrato_solicitud_quote_activa_pkey PRIMARY KEY (solicitud_id);


--
-- Name: cotizacion_arriendo_comercial cotizacion_arriendo_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_pkey PRIMARY KEY (id);


--
-- Name: cotizacion_arriendo_comercial cotizacion_arriendo_comercial_vigencia_ck; Type: CHECK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_vigencia_ck CHECK ((vence_en = (creada_en + '00:06:00'::interval))) NOT VALID;


--
-- Name: cotizacion_arriendo_tramo cotizacion_arriendo_tramo_cotizacion_id_orden_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_tramo
    ADD CONSTRAINT cotizacion_arriendo_tramo_cotizacion_id_orden_key UNIQUE (cotizacion_id, orden);


--
-- Name: cotizacion_arriendo_tramo cotizacion_arriendo_tramo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_tramo
    ADD CONSTRAINT cotizacion_arriendo_tramo_pkey PRIMARY KEY (id);


--
-- Name: cotizacion_modalidad_cobro cotizacion_modalidad_cobro_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_modalidad_cobro
    ADD CONSTRAINT cotizacion_modalidad_cobro_pkey PRIMARY KEY (cotizacion_id);


--
-- Name: cotizacion_reserva_ensayo cotizacion_reserva_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_reserva_ensayo_pkey PRIMARY KEY (id);


--
-- Name: cuenta_alerta_evento cuenta_alerta_evento_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_alerta_evento
    ADD CONSTRAINT cuenta_alerta_evento_pk PRIMARY KEY (id);


--
-- Name: cuenta_alerta_preferencia cuenta_alerta_preferencia_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_alerta_preferencia
    ADD CONSTRAINT cuenta_alerta_preferencia_pk PRIMARY KEY (account_id);


--
-- Name: cuenta_cobro_sintetica_historial_local cuenta_cobro_sintetica_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_historial_local
    ADD CONSTRAINT cuenta_cobro_sintetica_historial_local_pkey PRIMARY KEY (id);


--
-- Name: cuenta_cobro_sintetica_local cuenta_cobro_sintetica_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_local
    ADD CONSTRAINT cuenta_cobro_sintetica_local_pkey PRIMARY KEY (id);


--
-- Name: cuenta_passkey cuenta_passkey_credential_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_passkey
    ADD CONSTRAINT cuenta_passkey_credential_id_key UNIQUE (credential_id);


--
-- Name: cuenta_passkey cuenta_passkey_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_passkey
    ADD CONSTRAINT cuenta_passkey_pk PRIMARY KEY (id);


--
-- Name: cuenta_permiso_ubicacion cuenta_permiso_ubicacion_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_permiso_ubicacion
    ADD CONSTRAINT cuenta_permiso_ubicacion_pk PRIMARY KEY (account_id);


--
-- Name: cuenta_persona_confianza cuenta_persona_confianza_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_persona_confianza
    ADD CONSTRAINT cuenta_persona_confianza_pk PRIMARY KEY (id);


--
-- Name: cuenta_qr_dispositivo cuenta_qr_dispositivo_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_qr_dispositivo
    ADD CONSTRAINT cuenta_qr_dispositivo_pk PRIMARY KEY (account_id);


--
-- Name: cuenta_qr_ticket cuenta_qr_ticket_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_qr_ticket
    ADD CONSTRAINT cuenta_qr_ticket_pk PRIMARY KEY (id);


--
-- Name: cuenta_qr_ticket cuenta_qr_ticket_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_qr_ticket
    ADD CONSTRAINT cuenta_qr_ticket_token_hash_key UNIQUE (token_hash);


--
-- Name: cuenta_totp_backup_code cuenta_totp_backup_code_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_totp_backup_code
    ADD CONSTRAINT cuenta_totp_backup_code_pk PRIMARY KEY (id);


--
-- Name: cuenta_totp cuenta_totp_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_totp
    ADD CONSTRAINT cuenta_totp_pk PRIMARY KEY (account_id);


--
-- Name: cuenta_verificacion_telefono cuenta_verificacion_telefono_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_verificacion_telefono
    ADD CONSTRAINT cuenta_verificacion_telefono_pk PRIMARY KEY (account_id);


--
-- Name: cuenta_webauthn_challenge cuenta_webauthn_challenge_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_webauthn_challenge
    ADD CONSTRAINT cuenta_webauthn_challenge_pk PRIMARY KEY (id);


--
-- Name: desafio_mfa_administrativo desafio_mfa_administrativo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.desafio_mfa_administrativo
    ADD CONSTRAINT desafio_mfa_administrativo_pkey PRIMARY KEY (id);


--
-- Name: didit_webhook_inbox didit_webhook_inbox_event_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.didit_webhook_inbox
    ADD CONSTRAINT didit_webhook_inbox_event_id_key UNIQUE (event_id);


--
-- Name: didit_webhook_inbox didit_webhook_inbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.didit_webhook_inbox
    ADD CONSTRAINT didit_webhook_inbox_pkey PRIMARY KEY (id);


--
-- Name: disputa_ensayo_historial disputa_ensayo_historial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_historial
    ADD CONSTRAINT disputa_ensayo_historial_pkey PRIMARY KEY (secuencia);


--
-- Name: disputa_ensayo_local disputa_ensayo_local_anfitrion_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_anfitrion_id_clave_idempotencia_key UNIQUE (anfitrion_id, clave_idempotencia);


--
-- Name: disputa_ensayo_local disputa_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: document_verification_case document_verification_case_id_scope_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_id_scope_uq UNIQUE (id, purpose, subject_id, space_id);


--
-- Name: document_verification_case document_verification_case_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_pkey PRIMARY KEY (id);


--
-- Name: document_verification_event document_verification_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_event
    ADD CONSTRAINT document_verification_event_pkey PRIMARY KEY (id);


--
-- Name: document_verification_file document_verification_file_id_case_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_file
    ADD CONSTRAINT document_verification_file_id_case_id_key UNIQUE (id, case_id);


--
-- Name: document_verification_file document_verification_file_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_file
    ADD CONSTRAINT document_verification_file_pkey PRIMARY KEY (id);


--
-- Name: document_verification_job document_verification_job_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_job
    ADD CONSTRAINT document_verification_job_pkey PRIMARY KEY (id);


--
-- Name: document_verification_reviewer_grant_event document_verification_reviewer_grant_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_reviewer_grant_event
    ADD CONSTRAINT document_verification_reviewer_grant_event_pkey PRIMARY KEY (id);


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_reviewer_grant
    ADD CONSTRAINT document_verification_reviewer_grant_pkey PRIMARY KEY (id);


--
-- Name: documento_privado_sintetico_local documento_privado_sintetico_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_privado_sintetico_local
    ADD CONSTRAINT documento_privado_sintetico_local_pkey PRIMARY KEY (id);


--
-- Name: ejecucion_baja_local ejecucion_baja_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ejecucion_baja_local
    ADD CONSTRAINT ejecucion_baja_local_pkey PRIMARY KEY (id);


--
-- Name: ejecucion_baja_local ejecucion_baja_local_solicitud_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ejecucion_baja_local
    ADD CONSTRAINT ejecucion_baja_local_solicitud_id_clave_idempotencia_key UNIQUE (solicitud_id, clave_idempotencia);


--
-- Name: elegibilidad_verificacion_local elegibilidad_verificacion_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.elegibilidad_verificacion_local
    ADD CONSTRAINT elegibilidad_verificacion_local_pkey PRIMARY KEY (usuario_id, tipo);


--
-- Name: elegibilidad_verificacion_local elegibilidad_verificacion_local_verificacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.elegibilidad_verificacion_local
    ADD CONSTRAINT elegibilidad_verificacion_local_verificacion_id_key UNIQUE (verificacion_id);


--
-- Name: espacio_calendario_override espacio_calendario_override_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_calendario_override
    ADD CONSTRAINT espacio_calendario_override_pkey PRIMARY KEY (id);


--
-- Name: espacio_calendario_override espacio_calendario_override_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_calendario_override
    ADD CONSTRAINT espacio_calendario_override_uk UNIQUE (espacio_id, fecha);


--
-- Name: espacio_caracteristicas espacio_caracteristicas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_caracteristicas
    ADD CONSTRAINT espacio_caracteristicas_pkey PRIMARY KEY (espacio_id);


--
-- Name: espacio_contrato_borrador espacio_contrato_borrador_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_borrador
    ADD CONSTRAINT espacio_contrato_borrador_pkey PRIMARY KEY (espacio_id);


--
-- Name: espacio_contrato_version espacio_contrato_version_espacio_id_revision_origen_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_version
    ADD CONSTRAINT espacio_contrato_version_espacio_id_revision_origen_key UNIQUE (espacio_id, revision_origen);


--
-- Name: espacio_contrato_version espacio_contrato_version_espacio_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_version
    ADD CONSTRAINT espacio_contrato_version_espacio_id_version_key UNIQUE (espacio_id, version);


--
-- Name: espacio_contrato_version espacio_contrato_version_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_version
    ADD CONSTRAINT espacio_contrato_version_pkey PRIMARY KEY (id);


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_historial_local_espacio_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_eliminacion_historial_local
    ADD CONSTRAINT espacio_eliminacion_historial_local_espacio_id_key UNIQUE (espacio_id);


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_eliminacion_historial_local
    ADD CONSTRAINT espacio_eliminacion_historial_local_pkey PRIMARY KEY (id);


--
-- Name: espacio_galeria_archivo_candidato_local espacio_galeria_archivo_candidato_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_archivo_candidato_local
    ADD CONSTRAINT espacio_galeria_archivo_candidato_local_pkey PRIMARY KEY (archivo_id);


--
-- Name: espacio_galeria_sintetica_local espacio_galeria_cover_owner_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_sintetica_local
    ADD CONSTRAINT espacio_galeria_cover_owner_uq UNIQUE (espacio_id, propietario_id, id);


--
-- Name: espacio_galeria_sintetica_local espacio_galeria_sintetica_loc_espacio_id_propietario_id_cla_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_sintetica_local
    ADD CONSTRAINT espacio_galeria_sintetica_loc_espacio_id_propietario_id_cla_key UNIQUE (espacio_id, propietario_id, clave_idempotencia);


--
-- Name: espacio_galeria_sintetica_local espacio_galeria_sintetica_local_archivo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_sintetica_local
    ADD CONSTRAINT espacio_galeria_sintetica_local_archivo_id_key UNIQUE (archivo_id);


--
-- Name: espacio_galeria_sintetica_local espacio_galeria_sintetica_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_sintetica_local
    ADD CONSTRAINT espacio_galeria_sintetica_local_pkey PRIMARY KEY (id);


--
-- Name: espacio_horario_semanal espacio_horario_semanal_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_horario_semanal
    ADD CONSTRAINT espacio_horario_semanal_pkey PRIMARY KEY (espacio_id);


--
-- Name: espacio_horario_semanal_tramo espacio_horario_semanal_tramo_espacio_id_dia_iso_int4range_excl; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_horario_semanal_tramo
    ADD CONSTRAINT espacio_horario_semanal_tramo_espacio_id_dia_iso_int4range_excl EXCLUDE USING gist (espacio_id WITH =, dia_iso WITH =, int4range((apertura_minuto)::integer, (cierre_minuto)::integer, '[)'::text) WITH &&);


--
-- Name: espacio_horario_semanal_tramo espacio_horario_semanal_tramo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_horario_semanal_tramo
    ADD CONSTRAINT espacio_horario_semanal_tramo_pkey PRIMARY KEY (espacio_id, dia_iso, orden);


--
-- Name: espacio espacio_id_categoria_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_id_categoria_unique UNIQUE (id, categoria_codigo);


--
-- Name: espacio espacio_id_propietario_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_id_propietario_uk UNIQUE (id, propietario_id);


--
-- Name: espacio espacio_owner_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_owner_id_uq UNIQUE (id, propietario_id);


--
-- Name: espacio espacio_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_pkey PRIMARY KEY (id);


--
-- Name: espacio_publicacion_historial_local espacio_publicacion_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_publicacion_historial_local
    ADD CONSTRAINT espacio_publicacion_historial_local_pkey PRIMARY KEY (id);


--
-- Name: espacio_regla_disponibilidad espacio_regla_disponibilidad_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_regla_disponibilidad
    ADD CONSTRAINT espacio_regla_disponibilidad_pkey PRIMARY KEY (espacio_id);


--
-- Name: espacio_regla_mensual espacio_regla_mensual_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_regla_mensual
    ADD CONSTRAINT espacio_regla_mensual_pkey PRIMARY KEY (espacio_id);


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_solicitud_documental
    ADD CONSTRAINT espacio_solicitud_documental_pkey PRIMARY KEY (espacio_id);


--
-- Name: evento_auditoria_local evento_auditoria_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_auditoria_local
    ADD CONSTRAINT evento_auditoria_local_pkey PRIMARY KEY (id);


--
-- Name: evento_caso_soporte_comercial evento_caso_soporte_comercial_caso_id_actor_id_clave_idempo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_caso_soporte_comercial
    ADD CONSTRAINT evento_caso_soporte_comercial_caso_id_actor_id_clave_idempo_key UNIQUE (caso_id, actor_id, clave_idempotencia);


--
-- Name: evento_caso_soporte_comercial evento_caso_soporte_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_caso_soporte_comercial
    ADD CONSTRAINT evento_caso_soporte_comercial_pkey PRIMARY KEY (id);


--
-- Name: evidencia_caso_soporte_comercial evidencia_caso_soporte_comercial_caso_id_autor_id_sha256_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidencia_caso_soporte_comercial
    ADD CONSTRAINT evidencia_caso_soporte_comercial_caso_id_autor_id_sha256_key UNIQUE (caso_id, autor_id, sha256);


--
-- Name: evidencia_caso_soporte_comercial evidencia_caso_soporte_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidencia_caso_soporte_comercial
    ADD CONSTRAINT evidencia_caso_soporte_comercial_pkey PRIMARY KEY (id);


--
-- Name: foto_perfil_sintetica_local foto_perfil_sintetica_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.foto_perfil_sintetica_local
    ADD CONSTRAINT foto_perfil_sintetica_local_pkey PRIMARY KEY (id);


--
-- Name: gallery_item_order gallery_item_order_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_item_order_pkey PRIMARY KEY (id);


--
-- Name: gallery_item_order gallery_order_position_deferred_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_position_deferred_uq UNIQUE (espacio_id, "position") DEFERRABLE INITIALLY DEFERRED;


--
-- Name: gallery_item_order gallery_order_real_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_real_uq UNIQUE (space_media_id);


--
-- Name: gallery_item_order gallery_order_synthetic_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_synthetic_uq UNIQUE (synthetic_photo_id);


--
-- Name: gcp06_test_access_batch_audit gcp06_test_access_batch_audit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gcp06_test_access_batch_audit
    ADD CONSTRAINT gcp06_test_access_batch_audit_pkey PRIMARY KEY (id);


--
-- Name: historial_clave_local historial_clave_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.historial_clave_local
    ADD CONSTRAINT historial_clave_local_pkey PRIMARY KEY (id);


--
-- Name: host_compliance_event host_compliance_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_compliance_event
    ADD CONSTRAINT host_compliance_event_pkey PRIMARY KEY (id);


--
-- Name: identity_declaration_revision identity_declaration_revision_account_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_declaration_revision
    ADD CONSTRAINT identity_declaration_revision_account_id_revision_key UNIQUE (account_id, revision);


--
-- Name: identity_declaration_revision identity_declaration_revision_id_account_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_declaration_revision
    ADD CONSTRAINT identity_declaration_revision_id_account_id_key UNIQUE (id, account_id);


--
-- Name: identity_declaration_revision identity_declaration_revision_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_declaration_revision
    ADD CONSTRAINT identity_declaration_revision_pkey PRIMARY KEY (id);


--
-- Name: identity_provider_attempt identity_provider_attempt_id_provider_environment_applicati_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_id_provider_environment_applicati_key UNIQUE (id, provider, environment, application_id);


--
-- Name: identity_provider_attempt identity_provider_attempt_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_pkey PRIMARY KEY (id);


--
-- Name: identity_provider_attempt identity_provider_attempt_provider_environment_application__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_provider_environment_application__key UNIQUE (provider, environment, application_id, idempotency_key);


--
-- Name: identity_provider_attempt identity_provider_attempt_provider_environment_application_key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_provider_environment_application_key1 UNIQUE (provider, environment, application_id, correlation_id);


--
-- Name: identity_provider_attempt identity_provider_attempt_provider_environment_application_key2; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_provider_environment_application_key2 UNIQUE (provider, environment, application_id, provider_session_id);


--
-- Name: identity_provider_event identity_provider_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_event
    ADD CONSTRAINT identity_provider_event_pkey PRIMARY KEY (id);


--
-- Name: identity_provider_event identity_provider_event_provider_environment_application_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_event
    ADD CONSTRAINT identity_provider_event_provider_environment_application_id_key UNIQUE (provider, environment, application_id, event_id);


--
-- Name: identity_rut_claim identity_rut_claim_account_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_rut_claim
    ADD CONSTRAINT identity_rut_claim_account_uk UNIQUE (account_id);


--
-- Name: identity_rut_claim identity_rut_claim_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_rut_claim
    ADD CONSTRAINT identity_rut_claim_pk PRIMARY KEY (fingerprint);


--
-- Name: identity_verification_attempts identity_verification_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_verification_attempts
    ADD CONSTRAINT identity_verification_attempts_pkey PRIMARY KEY (id);


--
-- Name: kyb_compliance_notice_outbox kyb_compliance_notice_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_compliance_notice_outbox
    ADD CONSTRAINT kyb_compliance_notice_outbox_pkey PRIMARY KEY (id);


--
-- Name: kyb_rut_claim kyb_rut_claim_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_rut_claim
    ADD CONSTRAINT kyb_rut_claim_pkey PRIMARY KEY (verification_id, rut_kind, fingerprint);


--
-- Name: kyb_verification_document kyb_verification_document_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_pkey PRIMARY KEY (id);


--
-- Name: kyb_verification_document kyb_verification_document_storage_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_storage_ref_key UNIQUE (storage_ref);


--
-- Name: kyb_verification_document kyb_verification_document_verification_id_document_kind_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_verification_id_document_kind_key UNIQUE (verification_id, document_kind);


--
-- Name: kyb_verification_event kyb_verification_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_event
    ADD CONSTRAINT kyb_verification_event_pkey PRIMARY KEY (id);


--
-- Name: kyb_verifications kyb_verifications_id_organization_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_id_organization_id_key UNIQUE (id, organization_id);


--
-- Name: kyb_verifications kyb_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_pkey PRIMARY KEY (id);


--
-- Name: kyb_verifications kyb_verifications_submitted_by_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_submitted_by_idempotency_key_key UNIQUE (submitted_by, idempotency_key);


--
-- Name: m02_operacion_idempotente_local m02_operacion_idempotente_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.m02_operacion_idempotente_local
    ADD CONSTRAINT m02_operacion_idempotente_local_pkey PRIMARY KEY (usuario_id, recurso, clave_idempotencia);


--
-- Name: media_asset media_asset_owner_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_owner_id_uq UNIQUE (id, propietario_id);


--
-- Name: media_asset media_asset_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_pkey PRIMARY KEY (id);


--
-- Name: media_asset media_asset_space_owner_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_space_owner_id_uq UNIQUE (id, espacio_id, propietario_id);


--
-- Name: media_variant media_variant_asset_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant
    ADD CONSTRAINT media_variant_asset_uq UNIQUE (media_id, variant, object_key);


--
-- Name: media_variant_intent media_variant_intent_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant_intent
    ADD CONSTRAINT media_variant_intent_pkey PRIMARY KEY (media_id, variant);


--
-- Name: media_variant_intent media_variant_intent_provider_bucket_name_object_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant_intent
    ADD CONSTRAINT media_variant_intent_provider_bucket_name_object_key_key UNIQUE (provider, bucket_name, object_key);


--
-- Name: media_variant media_variant_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant
    ADD CONSTRAINT media_variant_pkey PRIMARY KEY (media_id, variant);


--
-- Name: mensaje_reserva_comercial mensaje_reserva_comercial_idempotency_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_comercial
    ADD CONSTRAINT mensaje_reserva_comercial_idempotency_uq UNIQUE (reserva_id, autor_id, clave_idempotencia);


--
-- Name: mensaje_reserva_comercial mensaje_reserva_comercial_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_comercial
    ADD CONSTRAINT mensaje_reserva_comercial_pk PRIMARY KEY (id);


--
-- Name: mensaje_reserva_comercial mensaje_reserva_comercial_reserva_secuencia_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_comercial
    ADD CONSTRAINT mensaje_reserva_comercial_reserva_secuencia_uq UNIQUE (reserva_id, secuencia);


--
-- Name: mensaje_reserva_ensayo mensaje_reserva_ensayo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_ensayo
    ADD CONSTRAINT mensaje_reserva_ensayo_id_key UNIQUE (id);


--
-- Name: mensaje_reserva_ensayo mensaje_reserva_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_ensayo
    ADD CONSTRAINT mensaje_reserva_ensayo_pkey PRIMARY KEY (secuencia);


--
-- Name: mensaje_reserva_ensayo mensaje_reserva_ensayo_reserva_id_autor_id_clave_idempotenc_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_ensayo
    ADD CONSTRAINT mensaje_reserva_ensayo_reserva_id_autor_id_clave_idempotenc_key UNIQUE (reserva_id, autor_id, clave_idempotencia);


--
-- Name: mercadopago_oauth_state mercadopago_oauth_state_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_oauth_state
    ADD CONSTRAINT mercadopago_oauth_state_pk PRIMARY KEY (state_hash);


--
-- Name: mercadopago_payment_webhook_receipt mercadopago_payment_webhook_receipt_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_payment_webhook_receipt
    ADD CONSTRAINT mercadopago_payment_webhook_receipt_pkey PRIMARY KEY (id);


--
-- Name: mercadopago_payment_webhook_receipt mercadopago_payment_webhook_request_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_payment_webhook_receipt
    ADD CONSTRAINT mercadopago_payment_webhook_request_uq UNIQUE (topic, provider_request_id);


--
-- Name: mercadopago_seller_connection mercadopago_seller_connection_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_seller_connection
    ADD CONSTRAINT mercadopago_seller_connection_pk PRIMARY KEY (anfitrion_id);


--
-- Name: mercadopago_seller_connection mercadopago_seller_connection_seller_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_seller_connection
    ADD CONSTRAINT mercadopago_seller_connection_seller_uk UNIQUE (seller_id);


--
-- Name: notificacion_usuario notificacion_usuario_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notificacion_usuario
    ADD CONSTRAINT notificacion_usuario_pkey PRIMARY KEY (id);


--
-- Name: ocupacion ocupacion_id_arriendo_space_interval_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_id_arriendo_space_interval_uk UNIQUE (arriendo_temprano_id, espacio_id, intervalo, id);


--
-- Name: ocupacion ocupacion_no_solapa_excl; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_no_solapa_excl EXCLUDE USING gist (espacio_id WITH =, intervalo WITH &&) WHERE (activo);


--
-- Name: ocupacion ocupacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_pkey PRIMARY KEY (id);


--
-- Name: ocupacion ocupacion_reserva_comercial_ref_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_reserva_comercial_ref_uk UNIQUE (reserva_arriendo_comercial_id, espacio_id, intervalo, id);


--
-- Name: ocupacion ocupacion_reserva_id_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_reserva_id_uk UNIQUE (reserva_id);


--
-- Name: ocupacion ocupacion_solicitud_comercial_ref_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_solicitud_comercial_ref_uk UNIQUE (solicitud_comercial_id, espacio_id, id);


--
-- Name: ocupacion ocupacion_solicitud_comercial_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_solicitud_comercial_uk UNIQUE (solicitud_comercial_id);


--
-- Name: operacion_arriendo_archivo_candidato_local operacion_arriendo_archivo_candidato_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_archivo_candidato_local
    ADD CONSTRAINT operacion_arriendo_archivo_candidato_local_pkey PRIMARY KEY (archivo_id);


--
-- Name: operacion_arriendo_ensayo_local operacion_arriendo_ensayo_loc_reserva_id_tipo_clave_idempot_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_ensayo_local
    ADD CONSTRAINT operacion_arriendo_ensayo_loc_reserva_id_tipo_clave_idempot_key UNIQUE (reserva_id, tipo, clave_idempotencia);


--
-- Name: operacion_arriendo_ensayo_local operacion_arriendo_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_ensayo_local
    ADD CONSTRAINT operacion_arriendo_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: operacion_arriendo_ensayo_local operacion_arriendo_ensayo_local_reserva_id_tipo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_ensayo_local
    ADD CONSTRAINT operacion_arriendo_ensayo_local_reserva_id_tipo_key UNIQUE (reserva_id, tipo);


--
-- Name: operacion_arriendo_evidencia_ensayo_local operacion_arriendo_evidencia_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_evidencia_ensayo_local
    ADD CONSTRAINT operacion_arriendo_evidencia_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: operacion_arriendo_historial_ensayo_local operacion_arriendo_historial_ensayo_local_operacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_historial_ensayo_local
    ADD CONSTRAINT operacion_arriendo_historial_ensayo_local_operacion_id_key UNIQUE (operacion_id);


--
-- Name: operacion_arriendo_historial_ensayo_local operacion_arriendo_historial_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_historial_ensayo_local
    ADD CONSTRAINT operacion_arriendo_historial_ensayo_local_pkey PRIMARY KEY (secuencia);


--
-- Name: organizacion organizacion_id_usuario_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion
    ADD CONSTRAINT organizacion_id_usuario_id_key UNIQUE (id, usuario_id);


--
-- Name: organizacion_invitacion organizacion_invitacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_pkey PRIMARY KEY (id);


--
-- Name: organizacion_invitacion organizacion_invitacion_token_sha256_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_token_sha256_key UNIQUE (token_sha256);


--
-- Name: organizacion_miembro organizacion_miembro_id_organizacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_miembro
    ADD CONSTRAINT organizacion_miembro_id_organizacion_id_key UNIQUE (id, organizacion_id);


--
-- Name: organizacion_miembro organizacion_miembro_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_miembro
    ADD CONSTRAINT organizacion_miembro_pkey PRIMARY KEY (id);


--
-- Name: organizacion organizacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion
    ADD CONSTRAINT organizacion_pkey PRIMARY KEY (id);


--
-- Name: organizacion_rut_fingerprint organizacion_rut_fingerprint_fingerprint_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_rut_fingerprint
    ADD CONSTRAINT organizacion_rut_fingerprint_fingerprint_key UNIQUE (fingerprint);


--
-- Name: organizacion_rut_fingerprint organizacion_rut_fingerprint_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_rut_fingerprint
    ADD CONSTRAINT organizacion_rut_fingerprint_pkey PRIMARY KEY (organizacion_id, fingerprint);


--
-- Name: outbox_evento_ciclo_local outbox_evento_ciclo_local_evento_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_ciclo_local
    ADD CONSTRAINT outbox_evento_ciclo_local_evento_id_clave_idempotencia_key UNIQUE (evento_id, clave_idempotencia);


--
-- Name: outbox_evento_ciclo_local outbox_evento_ciclo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_ciclo_local
    ADD CONSTRAINT outbox_evento_ciclo_local_pkey PRIMARY KEY (evento_id, numero_ciclo);


--
-- Name: outbox_evento_local outbox_evento_local_clave_deduplicacion_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_local
    ADD CONSTRAINT outbox_evento_local_clave_deduplicacion_key UNIQUE (clave_deduplicacion);


--
-- Name: outbox_evento_local outbox_evento_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_local
    ADD CONSTRAINT outbox_evento_local_pkey PRIMARY KEY (id);


--
-- Name: perfil_usuario perfil_usuario_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perfil_usuario
    ADD CONSTRAINT perfil_usuario_pk PRIMARY KEY (usuario_id);


--
-- Name: privacidad_bloqueo_tratamiento privacidad_bloqueo_tratamiento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_bloqueo_tratamiento
    ADD CONSTRAINT privacidad_bloqueo_tratamiento_pkey PRIMARY KEY (id);


--
-- Name: privacidad_ejecucion_derecho privacidad_ejecucion_derecho_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_ejecucion_derecho
    ADD CONSTRAINT privacidad_ejecucion_derecho_pkey PRIMARY KEY (id);


--
-- Name: privacidad_incidente_datos privacidad_incidente_datos_folio_incidente_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_datos
    ADD CONSTRAINT privacidad_incidente_datos_folio_incidente_key UNIQUE (folio_incidente);


--
-- Name: privacidad_incidente_datos privacidad_incidente_datos_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_datos
    ADD CONSTRAINT privacidad_incidente_datos_pkey PRIMARY KEY (id);


--
-- Name: privacidad_incidente_evento privacidad_incidente_evento_incidente_id_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_evento
    ADD CONSTRAINT privacidad_incidente_evento_incidente_id_secuencia_key UNIQUE (incidente_id, secuencia);


--
-- Name: privacidad_incidente_evento privacidad_incidente_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_evento
    ADD CONSTRAINT privacidad_incidente_evento_pkey PRIMARY KEY (id);


--
-- Name: privacidad_notificacion_outbox privacidad_notificacion_outbox_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_notificacion_outbox
    ADD CONSTRAINT privacidad_notificacion_outbox_clave_idempotencia_key UNIQUE (clave_idempotencia);


--
-- Name: privacidad_notificacion_outbox privacidad_notificacion_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_notificacion_outbox
    ADD CONSTRAINT privacidad_notificacion_outbox_pkey PRIMARY KEY (id);


--
-- Name: privacidad_preferencia_finalidad privacidad_preferencia_finalidad_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_preferencia_finalidad
    ADD CONSTRAINT privacidad_preferencia_finalidad_pkey PRIMARY KEY (cuenta_id, finalidad);


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_folio_retencion_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_folio_retencion_key UNIQUE (folio_retencion);


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_pkey PRIMARY KEY (id);


--
-- Name: privacy_response_dispatch_receipt privacy_response_dispatch_receipt_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_response_dispatch_receipt
    ADD CONSTRAINT privacy_response_dispatch_receipt_pkey PRIMARY KEY (request_id);


--
-- Name: profile_media profile_media_media_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_media
    ADD CONSTRAINT profile_media_media_id_key UNIQUE (media_id);


--
-- Name: profile_media profile_media_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_media
    ADD CONSTRAINT profile_media_pkey PRIMARY KEY (propietario_id);


--
-- Name: profile_pinpass_reauth profile_pinpass_reauth_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_pinpass_reauth
    ADD CONSTRAINT profile_pinpass_reauth_pkey PRIMARY KEY (account_id);


--
-- Name: reaplicacion_baja_local reaplicacion_baja_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reaplicacion_baja_local
    ADD CONSTRAINT reaplicacion_baja_local_pkey PRIMARY KEY (restore_id, ejecucion_origen_id);


--
-- Name: reclamo_dano_descargo_ensayo_local reclamo_dano_descargo_ensayo__reclamo_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_descargo_ensayo_local
    ADD CONSTRAINT reclamo_dano_descargo_ensayo__reclamo_id_clave_idempotencia_key UNIQUE (reclamo_id, clave_idempotencia);


--
-- Name: reclamo_dano_descargo_ensayo_local reclamo_dano_descargo_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_descargo_ensayo_local
    ADD CONSTRAINT reclamo_dano_descargo_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reclamo_dano_descargo_ensayo_local reclamo_dano_descargo_ensayo_local_reclamo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_descargo_ensayo_local
    ADD CONSTRAINT reclamo_dano_descargo_ensayo_local_reclamo_id_key UNIQUE (reclamo_id);


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_anfitrion_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_anfitrion_id_clave_idempotencia_key UNIQUE (anfitrion_id, clave_idempotencia);


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reclamo_dano_historial_ensayo_local reclamo_dano_historial_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_historial_ensayo_local
    ADD CONSTRAINT reclamo_dano_historial_ensayo_local_pkey PRIMARY KEY (secuencia);


--
-- Name: reclamo_dano_resolucion_ensayo_local reclamo_dano_resolucion_ensay_administrador_id_clave_idempo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_resolucion_ensayo_local
    ADD CONSTRAINT reclamo_dano_resolucion_ensay_administrador_id_clave_idempo_key UNIQUE (administrador_id, clave_idempotencia);


--
-- Name: reclamo_dano_resolucion_ensayo_local reclamo_dano_resolucion_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_resolucion_ensayo_local
    ADD CONSTRAINT reclamo_dano_resolucion_ensayo_local_pkey PRIMARY KEY (reclamo_id);


--
-- Name: region_publica region_publica_catalogo_version_codigo_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.region_publica
    ADD CONSTRAINT region_publica_catalogo_version_codigo_nombre_key UNIQUE (catalogo_version, codigo, nombre);


--
-- Name: region_publica region_publica_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.region_publica
    ADD CONSTRAINT region_publica_pkey PRIMARY KEY (catalogo_version, codigo);


--
-- Name: registro_replay_baja_local registro_replay_baja_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.registro_replay_baja_local
    ADD CONSTRAINT registro_replay_baja_local_pkey PRIMARY KEY (ejecucion_origen_id);


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_resena_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_resena_id_key UNIQUE (resena_id);


--
-- Name: reporte_resena_historial_local reporte_resena_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_historial_local
    ADD CONSTRAINT reporte_resena_historial_local_pkey PRIMARY KEY (secuencia);


--
-- Name: resena_autoria_marca_local resena_autoria_marca_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_autoria_marca_local
    ADD CONSTRAINT resena_autoria_marca_local_pkey PRIMARY KEY (huella_autoria);


--
-- Name: resena_ensayo_local resena_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: resena_ensayo_local resena_ensayo_local_reserva_id_autor_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_reserva_id_autor_id_key UNIQUE (reserva_id, autor_id);


--
-- Name: resena_reserva_comercial resena_reserva_comercial_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_pk PRIMARY KEY (id);


--
-- Name: resena_reserva_comercial resena_reserva_comercial_reserva_autor_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_reserva_autor_uq UNIQUE (reserva_id, autor_id);


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_cotizacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_cotizacion_id_key UNIQUE (cotizacion_id);


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_id_espacio_id_intervalo_ocupacio_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_id_espacio_id_intervalo_ocupacio_key UNIQUE (id, espacio_id, intervalo, ocupacion_id);


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_ocupacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_ocupacion_id_key UNIQUE (ocupacion_id);


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_operacion_checkout_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_operacion_checkout_id_key UNIQUE (operacion_checkout_id);


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_pkey PRIMARY KEY (id);


--
-- Name: reserva_arriendo_tramo_estado reserva_arriendo_tramo_estado_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_tramo_estado
    ADD CONSTRAINT reserva_arriendo_tramo_estado_pkey PRIMARY KEY (reserva_id, tramo_id);


--
-- Name: reserva_cancelacion_ensayo reserva_cancelacion_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cancelacion_ensayo
    ADD CONSTRAINT reserva_cancelacion_ensayo_pkey PRIMARY KEY (id);


--
-- Name: reserva_cancelacion_ensayo reserva_cancelacion_ensayo_reserva_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cancelacion_ensayo
    ADD CONSTRAINT reserva_cancelacion_ensayo_reserva_id_clave_idempotencia_key UNIQUE (reserva_id, clave_idempotencia);


--
-- Name: reserva_cancelacion_ensayo reserva_cancelacion_ensayo_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cancelacion_ensayo
    ADD CONSTRAINT reserva_cancelacion_ensayo_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_comercial_cancelacion reserva_comercial_cancelacion_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_cancelacion
    ADD CONSTRAINT reserva_comercial_cancelacion_clave_idempotencia_key UNIQUE (clave_idempotencia);


--
-- Name: reserva_comercial_cancelacion reserva_comercial_cancelacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_cancelacion
    ADD CONSTRAINT reserva_comercial_cancelacion_pkey PRIMARY KEY (id);


--
-- Name: reserva_comercial_cancelacion reserva_comercial_cancelacion_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_cancelacion
    ADD CONSTRAINT reserva_comercial_cancelacion_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_comercial_mensaje_lectura reserva_comercial_mensaje_lectura_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_mensaje_lectura
    ADD CONSTRAINT reserva_comercial_mensaje_lectura_pk PRIMARY KEY (reserva_id, usuario_id);


--
-- Name: reserva_comprobante_ensayo_local reserva_comprobante_ensayo_local_liquidacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comprobante_ensayo_local
    ADD CONSTRAINT reserva_comprobante_ensayo_local_liquidacion_id_key UNIQUE (liquidacion_id);


--
-- Name: reserva_comprobante_ensayo_local reserva_comprobante_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comprobante_ensayo_local
    ADD CONSTRAINT reserva_comprobante_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_cuota_mensual reserva_cuota_mensual_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cuota_mensual
    ADD CONSTRAINT reserva_cuota_mensual_pkey PRIMARY KEY (id);


--
-- Name: reserva_cuota_mensual reserva_cuota_mensual_reserva_arriendo_comercial_id_numero__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cuota_mensual
    ADD CONSTRAINT reserva_cuota_mensual_reserva_arriendo_comercial_id_numero__key UNIQUE (reserva_arriendo_comercial_id, numero_cuota);


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_e_reserva_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_e_reserva_id_clave_idempotencia_key UNIQUE (reserva_id, clave_idempotencia);


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_garantia_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_garantia_id_key UNIQUE (garantia_id);


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_devolucion_ensayo reserva_devolucion_ensayo_operacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_ensayo
    ADD CONSTRAINT reserva_devolucion_ensayo_operacion_id_key UNIQUE (operacion_id);


--
-- Name: reserva_devolucion_ensayo reserva_devolucion_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_ensayo
    ADD CONSTRAINT reserva_devolucion_ensayo_pkey PRIMARY KEY (id);


--
-- Name: reserva_devolucion_ensayo reserva_devolucion_ensayo_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_ensayo
    ADD CONSTRAINT reserva_devolucion_ensayo_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_devolucion_intento_ensayo reserva_devolucion_intento_ensayo_devolucion_id_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_intento_ensayo
    ADD CONSTRAINT reserva_devolucion_intento_ensayo_devolucion_id_secuencia_key UNIQUE (devolucion_id, secuencia);


--
-- Name: reserva_devolucion_intento_ensayo reserva_devolucion_intento_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_intento_ensayo
    ADD CONSTRAINT reserva_devolucion_intento_ensayo_pkey PRIMARY KEY (id);


--
-- Name: reserva_documento_tributario_comercial reserva_documento_tributario_comercial_object_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_documento_tributario_comercial
    ADD CONSTRAINT reserva_documento_tributario_comercial_object_id_key UNIQUE (object_id);


--
-- Name: reserva_documento_tributario_comercial reserva_documento_tributario_comercial_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_documento_tributario_comercial
    ADD CONSTRAINT reserva_documento_tributario_comercial_pkey PRIMARY KEY (id);


--
-- Name: reserva_documento_tributario_comercial reserva_documento_tributario_comercial_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_documento_tributario_comercial
    ADD CONSTRAINT reserva_documento_tributario_comercial_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_ensayo_local reserva_ensayo_local_arrendatario_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_arrendatario_id_clave_idempotencia_key UNIQUE (arrendatario_id, clave_idempotencia);


--
-- Name: reserva_ensayo_local reserva_ensayo_local_cotizacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_cotizacion_id_key UNIQUE (cotizacion_id);


--
-- Name: reserva_ensayo_local_fixture reserva_ensayo_local_fixture_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_fixture
    ADD CONSTRAINT reserva_ensayo_local_fixture_pkey PRIMARY KEY (espacio_id);


--
-- Name: reserva_ensayo_local reserva_ensayo_local_ocupacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_ocupacion_id_key UNIQUE (ocupacion_id);


--
-- Name: reserva_ensayo_local reserva_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_ensayo_local_ubicacion_sintetica reserva_ensayo_local_ubicacion_sintetica_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_ubicacion_sintetica
    ADD CONSTRAINT reserva_ensayo_local_ubicacion_sintetica_pkey PRIMARY KEY (espacio_id);


--
-- Name: reserva_ensayo_transicion reserva_ensayo_transicion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_transicion
    ADD CONSTRAINT reserva_ensayo_transicion_pkey PRIMARY KEY (id);


--
-- Name: reserva_ensayo_transicion reserva_ensayo_transicion_reserva_secuencia_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_transicion
    ADD CONSTRAINT reserva_ensayo_transicion_reserva_secuencia_uq UNIQUE (reserva_id, secuencia);


--
-- Name: reserva_finanzas_historial_ensayo_local reserva_finanzas_historial_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_finanzas_historial_ensayo_local
    ADD CONSTRAINT reserva_finanzas_historial_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_finanzas_historial_ensayo_local reserva_finanzas_historial_ensayo_local_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_finanzas_historial_ensayo_local
    ADD CONSTRAINT reserva_finanzas_historial_ensayo_local_secuencia_key UNIQUE (secuencia);


--
-- Name: reserva_garantia_ensayo_local reserva_garantia_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_ensayo_local
    ADD CONSTRAINT reserva_garantia_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_garantia_ensayo_local reserva_garantia_ensayo_local_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_ensayo_local
    ADD CONSTRAINT reserva_garantia_ensayo_local_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_garantia_evento_aplicacion_ensayo_local reserva_garantia_evento_aplicacion_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_aplicacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_aplicacion_ensayo_local_pkey PRIMARY KEY (evento_id);


--
-- Name: reserva_garantia_evento_ensayo_local reserva_garantia_evento_ensayo_local_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_ensayo_local_evento_id_key UNIQUE (evento_id);


--
-- Name: reserva_garantia_evento_ensayo_local reserva_garantia_evento_ensayo_local_operacion_id_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_ensayo_local_operacion_id_evento_id_key UNIQUE (operacion_id, evento_id);


--
-- Name: reserva_garantia_evento_ensayo_local reserva_garantia_evento_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_garantia_operacion_ensayo_local reserva_garantia_operacion_en_garantia_id_clave_idempotenci_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_operacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_operacion_en_garantia_id_clave_idempotenci_key UNIQUE (garantia_id, clave_idempotencia);


--
-- Name: reserva_garantia_operacion_ensayo_local reserva_garantia_operacion_ensayo_local_garantia_id_tipo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_operacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_operacion_ensayo_local_garantia_id_tipo_key UNIQUE (garantia_id, tipo);


--
-- Name: reserva_garantia_operacion_ensayo_local reserva_garantia_operacion_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_operacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_operacion_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_garantia_resultado_fake_ensayo_local reserva_garantia_resultado_fake_ensayo_local_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_evento_id_key UNIQUE (evento_id);


--
-- Name: reserva_garantia_resultado_fake_ensayo_local reserva_garantia_resultado_fake_ensayo_local_operacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_operacion_id_key UNIQUE (operacion_id);


--
-- Name: reserva_garantia_resultado_fake_ensayo_local reserva_garantia_resultado_fake_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_liquidacion_ensayo_local reserva_liquidacion_ensayo_lo_reserva_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_ensayo_lo_reserva_id_clave_idempotencia_key UNIQUE (reserva_id, clave_idempotencia);


--
-- Name: reserva_liquidacion_ensayo_local reserva_liquidacion_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_liquidacion_ensayo_local reserva_liquidacion_ensayo_local_reserva_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_ensayo_local_reserva_id_key UNIQUE (reserva_id);


--
-- Name: reserva_liquidacion_historial_ensayo_local reserva_liquidacion_historial_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_historial_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_historial_ensayo_local_pkey PRIMARY KEY (secuencia);


--
-- Name: reserva_liquidacion_operacion_ensayo_local reserva_liquidacion_operacion_ensayo_local_liquidacion_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_operacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_operacion_ensayo_local_liquidacion_id_key UNIQUE (liquidacion_id);


--
-- Name: reserva_liquidacion_operacion_ensayo_local reserva_liquidacion_operacion_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_operacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_operacion_ensayo_local_pkey PRIMARY KEY (id);


--
-- Name: reserva_liquidacion_resultado_fake_ensayo_local reserva_liquidacion_resultado_fake_ensayo_local_event_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_local_event_id_key UNIQUE (event_id);


--
-- Name: reserva_liquidacion_resultado_fake_ensayo_local reserva_liquidacion_resultado_fake_ensayo_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_local_pkey PRIMARY KEY (operacion_id);


--
-- Name: reserva_mensaje_lectura reserva_mensaje_lectura_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_mensaje_lectura
    ADD CONSTRAINT reserva_mensaje_lectura_pkey PRIMARY KEY (reserva_id, participante_id);


--
-- Name: reserva_pago_ensayo_operacion reserva_pago_ensayo_operacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo_operacion
    ADD CONSTRAINT reserva_pago_ensayo_operacion_pkey PRIMARY KEY (id);


--
-- Name: reserva_pago_ensayo_operacion reserva_pago_ensayo_operacion_reserva_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo_operacion
    ADD CONSTRAINT reserva_pago_ensayo_operacion_reserva_id_clave_idempotencia_key UNIQUE (reserva_id, clave_idempotencia);


--
-- Name: reserva_pago_ensayo reserva_pago_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo
    ADD CONSTRAINT reserva_pago_ensayo_pkey PRIMARY KEY (id);


--
-- Name: reserva_pago_ensayo reserva_pago_ensayo_reserva_id_clave_idempotencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo
    ADD CONSTRAINT reserva_pago_ensayo_reserva_id_clave_idempotencia_key UNIQUE (reserva_id, clave_idempotencia);


--
-- Name: reserva_pago_evento_aplicacion_ensayo reserva_pago_evento_aplicacion_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_aplicacion_ensayo
    ADD CONSTRAINT reserva_pago_evento_aplicacion_ensayo_pkey PRIMARY KEY (evento_id);


--
-- Name: reserva_pago_evento_ensayo reserva_pago_evento_ensayo_operacion_id_proveedor_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_ensayo
    ADD CONSTRAINT reserva_pago_evento_ensayo_operacion_id_proveedor_evento_id_key UNIQUE (operacion_id, proveedor_evento_id);


--
-- Name: reserva_pago_evento_ensayo reserva_pago_evento_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_ensayo
    ADD CONSTRAINT reserva_pago_evento_ensayo_pkey PRIMARY KEY (id);


--
-- Name: reserva_pago_evento_ensayo reserva_pago_evento_ensayo_proveedor_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_ensayo
    ADD CONSTRAINT reserva_pago_evento_ensayo_proveedor_evento_id_key UNIQUE (proveedor_evento_id);


--
-- Name: reserva_pago_fake_resultado_ensayo reserva_pago_fake_resultado_ensayo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_fake_resultado_ensayo
    ADD CONSTRAINT reserva_pago_fake_resultado_ensayo_pkey PRIMARY KEY (operacion_id);


--
-- Name: reserva_pago_fake_resultado_ensayo reserva_pago_fake_resultado_ensayo_proveedor_evento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_fake_resultado_ensayo
    ADD CONSTRAINT reserva_pago_fake_resultado_ensayo_proveedor_evento_id_key UNIQUE (proveedor_evento_id);


--
-- Name: reserva_vinculo_purgado_local reserva_vinculo_purgado_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_vinculo_purgado_local
    ADD CONSTRAINT reserva_vinculo_purgado_local_pkey PRIMARY KEY (reserva_id);


--
-- Name: rol_usuario_evento rol_usuario_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario_evento
    ADD CONSTRAINT rol_usuario_evento_pkey PRIMARY KEY (id);


--
-- Name: rol_usuario_evento rol_usuario_evento_usuario_rol_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario_evento
    ADD CONSTRAINT rol_usuario_evento_usuario_rol_uk UNIQUE (usuario_id, rol);


--
-- Name: rol_usuario rol_usuario_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario
    ADD CONSTRAINT rol_usuario_pk PRIMARY KEY (usuario_id, rol);


--
-- Name: sesion_administrativa sesion_administrativa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion_administrativa
    ADD CONSTRAINT sesion_administrativa_pkey PRIMARY KEY (id);


--
-- Name: sesion_administrativa sesion_administrativa_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion_administrativa
    ADD CONSTRAINT sesion_administrativa_token_hash_key UNIQUE (token_hash);


--
-- Name: sesion sesion_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_pk PRIMARY KEY (id);


--
-- Name: sesion sesion_token_hash_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_token_hash_uk UNIQUE (token_hash);


--
-- Name: simulacion_precio_privada simulacion_precio_privada_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.simulacion_precio_privada
    ADD CONSTRAINT simulacion_precio_privada_pkey PRIMARY KEY (id);


--
-- Name: solicitud_privacidad_administrativa solicitud_privacidad_administrativa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_privacidad_administrativa
    ADD CONSTRAINT solicitud_privacidad_administrativa_pkey PRIMARY KEY (id);


--
-- Name: solicitud_titular_evento solicitud_titular_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular_evento
    ADD CONSTRAINT solicitud_titular_evento_pkey PRIMARY KEY (id);


--
-- Name: solicitud_titular_evento solicitud_titular_evento_solicitud_id_secuencia_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular_evento
    ADD CONSTRAINT solicitud_titular_evento_solicitud_id_secuencia_key UNIQUE (solicitud_id, secuencia);


--
-- Name: solicitud_titular solicitud_titular_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular
    ADD CONSTRAINT solicitud_titular_pk PRIMARY KEY (id);


--
-- Name: space_kyb_verification space_kyb_verification_document_storage_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_document_storage_ref_key UNIQUE (document_storage_ref);


--
-- Name: space_kyb_verification space_kyb_verification_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_pkey PRIMARY KEY (id);


--
-- Name: space_media space_media_id_scope_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_media
    ADD CONSTRAINT space_media_id_scope_uq UNIQUE (espacio_id, propietario_id, id);


--
-- Name: space_media space_media_media_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_media
    ADD CONSTRAINT space_media_media_id_key UNIQUE (media_id);


--
-- Name: space_media space_media_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_media
    ADD CONSTRAINT space_media_pkey PRIMARY KEY (id);


--
-- Name: tarifa_espacio tarifa_espacio_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tarifa_espacio
    ADD CONSTRAINT tarifa_espacio_pkey PRIMARY KEY (espacio_id, version);


--
-- Name: ticket_soporte_administrativo ticket_soporte_administrativo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_soporte_administrativo
    ADD CONSTRAINT ticket_soporte_administrativo_pkey PRIMARY KEY (id);


--
-- Name: token_accion token_accion_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.token_accion
    ADD CONSTRAINT token_accion_pk PRIMARY KEY (id);


--
-- Name: token_accion token_accion_token_hash_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.token_accion
    ADD CONSTRAINT token_accion_token_hash_uk UNIQUE (token_hash);


--
-- Name: usuario usuario_correo_normalizado_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario
    ADD CONSTRAINT usuario_correo_normalizado_uk UNIQUE (correo_normalizado);


--
-- Name: usuario usuario_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario
    ADD CONSTRAINT usuario_pk PRIMARY KEY (id);


--
-- Name: verificacion_archivo_retiro_pendiente_local verificacion_archivo_retiro_pendiente_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_archivo_retiro_pendiente_local
    ADD CONSTRAINT verificacion_archivo_retiro_pendiente_local_pkey PRIMARY KEY (evidencia_id);


--
-- Name: verificacion_evidencia_sintetica verificacion_evidencia_sintetica_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_evidencia_sintetica
    ADD CONSTRAINT verificacion_evidencia_sintetica_pk PRIMARY KEY (id);


--
-- Name: verificacion_historial_local verificacion_historial_event_idempotency_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_historial_local
    ADD CONSTRAINT verificacion_historial_event_idempotency_uk UNIQUE (verificacion_id, clave_idempotencia);


--
-- Name: verificacion_historial_local verificacion_historial_local_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_historial_local
    ADD CONSTRAINT verificacion_historial_local_pkey PRIMARY KEY (id);


--
-- Name: verificacion verificacion_idempotencia_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_idempotencia_uk UNIQUE (usuario_id, clave_idempotencia);


--
-- Name: verificacion verificacion_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_pk PRIMARY KEY (id);


--
-- Name: version_terminos version_terminos_codigo_uk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.version_terminos
    ADD CONSTRAINT version_terminos_codigo_uk UNIQUE (codigo);


--
-- Name: version_terminos version_terminos_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.version_terminos
    ADD CONSTRAINT version_terminos_pk PRIMARY KEY (id);


--
-- Name: vista_asistida_administrativa vista_asistida_administrativa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vista_asistida_administrativa
    ADD CONSTRAINT vista_asistida_administrativa_pkey PRIMARY KEY (id);


--
-- Name: acceso_sensible_administrativo_actor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acceso_sensible_administrativo_actor_idx ON public.acceso_sensible_administrativo USING btree (actor_cuenta_id, otorgado_en DESC);


--
-- Name: acceso_sensible_administrativo_objetivo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acceso_sensible_administrativo_objetivo_idx ON public.acceso_sensible_administrativo USING btree (objetivo_cuenta_id, otorgado_en DESC);


--
-- Name: account_identity_profile_username_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX account_identity_profile_username_unique ON public.account_identity_profile USING btree (lower(username));


--
-- Name: account_identity_verification_latest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX account_identity_verification_latest ON public.account_identity_verification USING btree (account_id, profile_revision, started_at DESC, id DESC);


--
-- Name: account_identity_verification_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX account_identity_verification_pending ON public.account_identity_verification USING btree (account_id, profile_revision) WHERE (state = ANY (ARRAY['in_progress'::text, 'in_review'::text]));


--
-- Name: account_pinpass_access_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX account_pinpass_access_expiry_idx ON public.account_pinpass_access USING btree (expires_at);


--
-- Name: account_pinpass_audit_account_time_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX account_pinpass_audit_account_time_idx ON public.account_pinpass_audit USING btree (account_id, occurred_at DESC, id DESC);


--
-- Name: aceptacion_terminos_version_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aceptacion_terminos_version_idx ON public.aceptacion_terminos USING btree (version_id);


--
-- Name: administrador_global_bootstrap_evento_objetivo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX administrador_global_bootstrap_evento_objetivo_idx ON public.administrador_global_bootstrap_evento USING btree (usuario_objetivo_id, ocurrida_en DESC, id);


--
-- Name: anfitrion_cumplimiento_documento_one_open_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX anfitrion_cumplimiento_documento_one_open_uq ON public.anfitrion_cumplimiento_documento USING btree (usuario_id) WHERE ((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[]));


--
-- Name: aprobacion_administrativa_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aprobacion_administrativa_estado_idx ON public.aprobacion_administrativa USING btree (estado, creada_en DESC, id DESC);


--
-- Name: aprobacion_administrativa_solicitante_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aprobacion_administrativa_solicitante_idx ON public.aprobacion_administrativa USING btree (solicitante_id, creada_en DESC);


--
-- Name: autorizacion_arriendo_comercial_activa_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX autorizacion_arriendo_comercial_activa_idx ON public.autorizacion_arriendo_comercial USING btree (espacio_id) WHERE (revocada_en IS NULL);


--
-- Name: autorizacion_arriendo_comercial_event_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX autorizacion_arriendo_comercial_event_idx ON public.autorizacion_arriendo_comercial_evento USING btree (espacio_id, ocurrida_en, id);


--
-- Name: autorizacion_arriendo_comercial_evento_lifecycle_uk; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX autorizacion_arriendo_comercial_evento_lifecycle_uk ON public.autorizacion_arriendo_comercial_evento USING btree (autorizacion_id, accion) WHERE (accion = ANY (ARRAY['activated'::text, 'revoked'::text]));


--
-- Name: autorizacion_arriendo_comercial_history_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX autorizacion_arriendo_comercial_history_idx ON public.autorizacion_arriendo_comercial USING btree (espacio_id, creada_en DESC, id);


--
-- Name: autorizacion_firma_desafio_otp_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX autorizacion_firma_desafio_otp_lookup_idx ON public.autorizacion_firma_desafio_otp USING btree (actor_id, espacio_id, plantilla_version, creada_en DESC);


--
-- Name: aviso_local_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aviso_local_due_idx ON public.aviso_local USING btree (estado, proximo_intento_en, id);


--
-- Name: aviso_local_recipient_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aviso_local_recipient_idx ON public.aviso_local USING btree (destinatario_id, creada_en DESC, id);


--
-- Name: baja_archivo_pendiente_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX baja_archivo_pendiente_claim_idx ON public.baja_archivo_pendiente_local USING btree (disponible_en, claim_hasta, ejecucion_id, evidencia_id) WHERE (completada_en IS NULL);


--
-- Name: bloqueo_cuenta_historial_local_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bloqueo_cuenta_historial_local_idx ON public.bloqueo_cuenta_historial_local USING btree (cuenta_id, ocurrida_en, id);


--
-- Name: caso_soporte_dano_unico; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX caso_soporte_dano_unico ON public.caso_soporte_reserva_comercial USING btree (reserva_id) WHERE (categoria = 'dano_inmueble'::text);


--
-- Name: caso_soporte_descargo_unico; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX caso_soporte_descargo_unico ON public.evento_caso_soporte_comercial USING btree (caso_id) WHERE (accion = 'descargo'::text);


--
-- Name: caso_soporte_mediacion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX caso_soporte_mediacion_idx ON public.caso_soporte_reserva_comercial USING btree (creada_en DESC, id) WHERE (enrutado_a = 'espacigo'::text);


--
-- Name: caso_soporte_reserva_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX caso_soporte_reserva_idx ON public.caso_soporte_reserva_comercial USING btree (reserva_id, creada_en DESC, id);


--
-- Name: catalogo_ubicacion_publica_activo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX catalogo_ubicacion_publica_activo_idx ON public.catalogo_ubicacion_publica USING btree (activo) WHERE activo;


--
-- Name: checkout_cancelacion_pendiente_trabajo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX checkout_cancelacion_pendiente_trabajo_idx ON public.checkout_cancelacion_pendiente USING btree (ultimo_intento_en NULLS FIRST, operacion_id) WHERE (resultado IS NULL);


--
-- Name: checkout_comercial_settlement_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX checkout_comercial_settlement_due_idx ON public.checkout_preferencia_comercial USING btree (provider_settlement_attempted_at NULLS FIRST, id) WHERE ((estado = 'approved'::text) AND (checkout_method = 'checkout_pro'::text) AND (provider_payment_id IS NOT NULL));


--
-- Name: checkout_payment_notice_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX checkout_payment_notice_due_idx ON public.checkout_payment_notice_outbox USING btree (available_at, created_at, id) WHERE (state = ANY (ARRAY['pending'::text, 'processing'::text]));


--
-- Name: checkout_payment_notice_transition_recipient_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX checkout_payment_notice_transition_recipient_uq ON public.checkout_payment_notice_outbox USING btree (reservation_id, transition, recipient_id);


--
-- Name: checkout_preferencia_comercial_external_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX checkout_preferencia_comercial_external_idx ON public.checkout_preferencia_comercial USING btree (id) WHERE (estado = ANY (ARRAY['pending'::text, 'reconciliation_required'::text]));


--
-- Name: checkout_preferencia_comercial_hold_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX checkout_preferencia_comercial_hold_idx ON public.checkout_preferencia_comercial USING btree (espacio_id, vence_en) WHERE (estado = ANY (ARRAY['pending'::text, 'reconciliation_required'::text]));


--
-- Name: checkout_preferencia_comercial_idempotency_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX checkout_preferencia_comercial_idempotency_uq ON public.checkout_preferencia_comercial USING btree (arrendatario_id, cotizacion_id, idempotency_key);


--
-- Name: checkout_preferencia_comercial_payment_id_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX checkout_preferencia_comercial_payment_id_uq ON public.checkout_preferencia_comercial USING btree (provider_payment_id) WHERE (provider_payment_id IS NOT NULL);


--
-- Name: comuna_publica_region_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comuna_publica_region_idx ON public.comuna_publica USING btree (catalogo_version, region_codigo, nombre COLLATE "C", codigo);


--
-- Name: contrato_documento_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_documento_due_idx ON public.contrato_documento_comercial USING btree (disponible_en, creada_en, id) WHERE (estado = ANY (ARRAY['pending'::text, 'retry'::text, 'processing'::text]));


--
-- Name: contrato_ensayo_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_ensayo_historial_idx ON public.contrato_ensayo_historial USING btree (contrato_id, secuencia);


--
-- Name: contrato_firma_desafio_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_firma_desafio_lookup_idx ON public.contrato_firma_desafio_otp USING btree (actor_id, propuesta_id, rol, creada_en DESC);


--
-- Name: contrato_propuesta_comercial_participantes_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_propuesta_comercial_participantes_idx ON public.contrato_propuesta_comercial USING btree (arrendatario_id, propietario_id, creada_en DESC);


--
-- Name: contrato_solicitud_expira_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_solicitud_expira_idx ON public.contrato_solicitud_comercial USING btree (vence_en, id) WHERE (estado = 'pending_landlord'::text);


--
-- Name: contrato_solicitud_notice_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_solicitud_notice_due_idx ON public.contrato_solicitud_notice_outbox USING btree (disponible_en, creada_en, id) WHERE (estado = ANY (ARRAY['pending'::text, 'processing'::text]));


--
-- Name: contrato_solicitud_owner_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_solicitud_owner_idx ON public.contrato_solicitud_comercial USING btree (propietario_id, creada_en DESC, id);


--
-- Name: contrato_solicitud_pago_expira_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_solicitud_pago_expira_idx ON public.contrato_solicitud_comercial USING btree (pago_vence_en, id) WHERE (estado = 'ready_for_payment'::text);


--
-- Name: contrato_solicitud_renter_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contrato_solicitud_renter_idx ON public.contrato_solicitud_comercial USING btree (arrendatario_id, creada_en DESC, id);


--
-- Name: cotizacion_arriendo_comercial_expira_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX cotizacion_arriendo_comercial_expira_idx ON public.cotizacion_arriendo_comercial USING btree (vence_en, id);


--
-- Name: cotizacion_arriendo_tramo_cotizacion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX cotizacion_arriendo_tramo_cotizacion_idx ON public.cotizacion_arriendo_tramo USING btree (cotizacion_id, orden);


--
-- Name: cotizacion_reserva_ensayo_titular_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX cotizacion_reserva_ensayo_titular_idx ON public.cotizacion_reserva_ensayo USING btree (arrendatario_id, creada_en DESC);


--
-- Name: cuenta_cobro_sintetica_activa_owner_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX cuenta_cobro_sintetica_activa_owner_uq ON public.cuenta_cobro_sintetica_local USING btree (usuario_id) WHERE (estado = 'activa'::text);


--
-- Name: cuenta_cobro_sintetica_historial_owner_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX cuenta_cobro_sintetica_historial_owner_idx ON public.cuenta_cobro_sintetica_historial_local USING btree (usuario_id, ocurrida_en, id);


--
-- Name: cuenta_cobro_sintetica_owner_date_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX cuenta_cobro_sintetica_owner_date_idx ON public.cuenta_cobro_sintetica_local USING btree (usuario_id, creada_en, id);


--
-- Name: desafio_mfa_administrativo_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX desafio_mfa_administrativo_cuenta_idx ON public.desafio_mfa_administrativo USING btree (cuenta_id, creado_en DESC);


--
-- Name: disputa_ensayo_abiertas_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX disputa_ensayo_abiertas_idx ON public.disputa_ensayo_local USING btree (abierta_en, id) WHERE (estado = 'abierta'::text);


--
-- Name: disputa_ensayo_anfitrion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX disputa_ensayo_anfitrion_idx ON public.disputa_ensayo_local USING btree (anfitrion_id, abierta_en DESC, id);


--
-- Name: disputa_ensayo_arrendatario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX disputa_ensayo_arrendatario_idx ON public.disputa_ensayo_local USING btree (arrendatario_id, abierta_en DESC, id);


--
-- Name: disputa_ensayo_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX disputa_ensayo_historial_idx ON public.disputa_ensayo_historial USING btree (disputa_id, secuencia);


--
-- Name: disputa_ensayo_una_abierta_por_reserva_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX disputa_ensayo_una_abierta_por_reserva_uq ON public.disputa_ensayo_local USING btree (reserva_id) WHERE (estado = 'abierta'::text);


--
-- Name: document_verification_case_identity_declaration_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_case_identity_declaration_idx ON public.document_verification_case USING btree (identity_declaration_revision_id, subject_id) WHERE (identity_declaration_revision_id IS NOT NULL);


--
-- Name: document_verification_event_case_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_event_case_idx ON public.document_verification_event USING btree (case_id, occurred_at, id);


--
-- Name: document_verification_event_file_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_event_file_idx ON public.document_verification_event USING btree (file_id, occurred_at, id) WHERE (file_id IS NOT NULL);


--
-- Name: document_verification_file_case_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_file_case_idx ON public.document_verification_file USING btree (case_id);


--
-- Name: document_verification_file_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_file_retention_idx ON public.document_verification_file USING btree (retention_at) WHERE (retention_at IS NOT NULL);


--
-- Name: document_verification_identity_current_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_identity_current_idx ON public.document_verification_case USING btree (subject_id, verified_until DESC) WHERE ((purpose = 'identity'::text) AND (state = 'aprobada'::text));


--
-- Name: document_verification_job_active_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX document_verification_job_active_uq ON public.document_verification_job USING btree (file_id, kind) WHERE (state = ANY (ARRAY['pendiente'::text, 'procesando'::text, 'reintentar'::text]));


--
-- Name: document_verification_job_ready_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_job_ready_idx ON public.document_verification_job USING btree (available_at, id) WHERE (state = ANY (ARRAY['pendiente'::text, 'reintentar'::text]));


--
-- Name: document_verification_one_open_identity_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX document_verification_one_open_identity_uq ON public.document_verification_case USING btree (subject_id, purpose) WHERE ((purpose = 'identity'::text) AND (state = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text])));


--
-- Name: document_verification_one_open_space_subject_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX document_verification_one_open_space_subject_uq ON public.document_verification_case USING btree (space_id, purpose, subject_id) WHERE ((purpose = 'space'::text) AND (superseded_at IS NULL) AND (state = ANY (ARRAY['borrador'::text, 'enviada'::text, 'en_revision'::text, 'requiere_antecedentes'::text])));


--
-- Name: INDEX document_verification_one_open_space_subject_uq; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON INDEX public.document_verification_one_open_space_subject_uq IS 'One active open space case per historical owner subject; current ownership is verified on every operation.';


--
-- Name: document_verification_reviewer_grant_active_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX document_verification_reviewer_grant_active_uq ON public.document_verification_reviewer_grant USING btree (reviewer_id, capability, scope_kind, scope_id) WHERE (revoked_at IS NULL);


--
-- Name: document_verification_reviewer_grant_actor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_verification_reviewer_grant_actor_idx ON public.document_verification_reviewer_grant USING btree (reviewer_id, capability, scope_id) WHERE (revoked_at IS NULL);


--
-- Name: ejecucion_baja_local_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ejecucion_baja_local_pendiente_idx ON public.ejecucion_baja_local USING btree (iniciada_en, id) WHERE (estado = 'limpieza_pendiente'::text);


--
-- Name: elegibilidad_local_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX elegibilidad_local_active_idx ON public.elegibilidad_verificacion_local USING btree (usuario_id, tipo) WHERE (estado = 'elegible'::text);


--
-- Name: espacio_b2b_organization_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_b2b_organization_idx ON public.espacio USING btree (organizacion_id) WHERE es_b2b;


--
-- Name: espacio_caracteristicas_public_filter_keys_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_caracteristicas_public_filter_keys_idx ON public.espacio_caracteristicas USING gin (valores);


--
-- Name: espacio_caracteristicas_public_values_gin_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_caracteristicas_public_values_gin_idx ON public.espacio_caracteristicas USING gin (valores jsonb_path_ops);


--
-- Name: espacio_commercial_location_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_commercial_location_idx ON public.espacio USING btree (ubicacion_latitud, ubicacion_longitud) WHERE ((estado = 'activa'::text) AND (ubicacion_confirmada_en IS NOT NULL));


--
-- Name: espacio_eliminacion_historial_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_eliminacion_historial_retention_idx ON public.espacio_eliminacion_historial_local USING btree (retirar_en, id) WHERE (retirar_en IS NOT NULL);


--
-- Name: espacio_eliminacion_owner_order_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_eliminacion_owner_order_idx ON public.espacio_eliminacion_historial_local USING btree (actor_id, ocurrida_en, id);


--
-- Name: espacio_galeria_activa_orden_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_galeria_activa_orden_idx ON public.espacio_galeria_sintetica_local USING btree (espacio_id, creada_en, id) WHERE (estado = 'activa'::text);


--
-- Name: espacio_galeria_candidato_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_galeria_candidato_cleanup_idx ON public.espacio_galeria_archivo_candidato_local USING btree (estado, proximo_intento_en, creada_en, archivo_id);


--
-- Name: espacio_galeria_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_galeria_cleanup_idx ON public.espacio_galeria_sintetica_local USING btree (proximo_intento_en, id) WHERE ((estado <> 'activa'::text) AND (archivo_id IS NOT NULL));


--
-- Name: espacio_galeria_publicada_local_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_galeria_publicada_local_idx ON public.espacio_galeria_sintetica_local USING btree (espacio_id, id) WHERE ((estado = 'activa'::text) AND publicada_en_catalogo AND (archivo_id IS NOT NULL));


--
-- Name: espacio_propietario_actualizacion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_propietario_actualizacion_idx ON public.espacio USING btree (propietario_id, actualizado_en DESC, id);


--
-- Name: espacio_public_area_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_public_area_idx ON public.espacio USING btree (superficie_m2, id) WHERE (estado = 'activa'::text);


--
-- Name: espacio_public_region_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_public_region_idx ON public.espacio USING btree (ubicacion_catalogo_version, region_codigo, comuna_codigo, id) WHERE ((estado = 'activa'::text) AND (ubicacion_catalogo_version IS NOT NULL));


--
-- Name: espacio_public_search_gin_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_public_search_gin_idx ON public.espacio USING gin (busqueda_publica) WHERE (estado = 'activa'::text);


--
-- Name: espacio_publicacion_historial_order_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_publicacion_historial_order_idx ON public.espacio_publicacion_historial_local USING btree (espacio_id, id);


--
-- Name: espacio_publicacion_historial_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX espacio_publicacion_historial_retention_idx ON public.espacio_publicacion_historial_local USING btree (retirar_en, id) WHERE (retirar_en IS NOT NULL);


--
-- Name: evento_auditoria_actor_tiempo_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX evento_auditoria_actor_tiempo_id_idx ON public.evento_auditoria_local USING btree (actor_id, ocurrido_en DESC, id DESC);


--
-- Name: evento_auditoria_idempotente_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX evento_auditoria_idempotente_uq ON public.evento_auditoria_local USING btree (recurso_id, accion, clave_idempotencia) WHERE (clave_idempotencia IS NOT NULL);


--
-- Name: evento_auditoria_objetivo_tiempo_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX evento_auditoria_objetivo_tiempo_id_idx ON public.evento_auditoria_local USING btree (cuenta_objetivo_id, ocurrido_en DESC, id DESC) WHERE (cuenta_objetivo_id IS NOT NULL);


--
-- Name: evento_auditoria_recurso_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX evento_auditoria_recurso_idx ON public.evento_auditoria_local USING btree (recurso_tipo, recurso_id, ocurrido_en, id);


--
-- Name: evento_auditoria_tiempo_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX evento_auditoria_tiempo_id_idx ON public.evento_auditoria_local USING btree (ocurrido_en DESC, id DESC);


--
-- Name: foto_perfil_sintetica_activa_owner_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX foto_perfil_sintetica_activa_owner_uq ON public.foto_perfil_sintetica_local USING btree (usuario_id) WHERE (estado = 'activa'::text);


--
-- Name: foto_perfil_sintetica_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX foto_perfil_sintetica_cleanup_idx ON public.foto_perfil_sintetica_local USING btree (proximo_intento_en, id) WHERE ((estado <> 'activa'::text) AND (archivo_id IS NOT NULL));


--
-- Name: historial_clave_usuario_vigente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX historial_clave_usuario_vigente_idx ON public.historial_clave_local USING btree (usuario_id, retirar_en DESC);


--
-- Name: host_compliance_event_profile_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_compliance_event_profile_idx ON public.host_compliance_event USING btree (usuario_id, organizacion_id, occurred_at DESC);


--
-- Name: identity_provider_attempt_one_active_intent; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX identity_provider_attempt_one_active_intent ON public.identity_provider_attempt USING btree (account_id, declaration_id, provider, environment, application_id) WHERE ((status)::text = ANY (ARRAY[('CREATED'::character varying)::text, ('PENDING_USER_INPUT'::character varying)::text, ('PROCESSING'::character varying)::text, ('IN_REVIEW'::character varying)::text]));


--
-- Name: identity_rut_claim_pending_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX identity_rut_claim_pending_expiry_idx ON public.identity_rut_claim USING btree (expires_at) WHERE (verified_at IS NULL);


--
-- Name: idx_cuenta_alerta_evento_account_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_alerta_evento_account_created ON public.cuenta_alerta_evento USING btree (account_id, created_at DESC);


--
-- Name: idx_cuenta_passkey_account_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_passkey_account_created ON public.cuenta_passkey USING btree (account_id, created_at DESC);


--
-- Name: idx_cuenta_persona_confianza_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_persona_confianza_account ON public.cuenta_persona_confianza USING btree (account_id, created_at DESC);


--
-- Name: idx_cuenta_qr_ticket_account_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_qr_ticket_account_created ON public.cuenta_qr_ticket USING btree (account_id, created_at DESC);


--
-- Name: idx_cuenta_totp_backup_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_totp_backup_account ON public.cuenta_totp_backup_code USING btree (account_id, used_at);


--
-- Name: idx_cuenta_verificacion_telefono_phone; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_verificacion_telefono_phone ON public.cuenta_verificacion_telefono USING btree (phone_e164);


--
-- Name: idx_cuenta_webauthn_challenge_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_webauthn_challenge_lookup ON public.cuenta_webauthn_challenge USING btree (account_id, purpose, expires_at);


--
-- Name: idx_didit_webhook_inbox_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_didit_webhook_inbox_session_id ON public.didit_webhook_inbox USING btree (session_id);


--
-- Name: idx_didit_webhook_inbox_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_didit_webhook_inbox_status ON public.didit_webhook_inbox USING btree (status);


--
-- Name: idx_identity_verification_attempts_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_verification_attempts_account_id ON public.identity_verification_attempts USING btree (account_id);


--
-- Name: idx_identity_verification_attempts_didit_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_verification_attempts_didit_session_id ON public.identity_verification_attempts USING btree (didit_session_id);


--
-- Name: idx_identity_verification_attempts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_verification_attempts_status ON public.identity_verification_attempts USING btree (status);


--
-- Name: kyb_compliance_notice_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_compliance_notice_pending_idx ON public.kyb_compliance_notice_outbox USING btree (available_at, created_at, id) WHERE ((state)::text = ANY ((ARRAY['pending'::character varying, 'processing'::character varying])::text[]));


--
-- Name: kyb_compliance_notice_profile_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_compliance_notice_profile_idx ON public.kyb_compliance_notice_outbox USING btree (profile_type, profile_id, created_at DESC);


--
-- Name: kyb_compliance_notice_stage_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX kyb_compliance_notice_stage_uq ON public.kyb_compliance_notice_outbox USING btree (profile_type, profile_id, recipient_id, reminder_stage) WHERE ((reminder_stage)::text <> 'reactivated'::text);


--
-- Name: kyb_compliance_notice_transition_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX kyb_compliance_notice_transition_uq ON public.kyb_compliance_notice_outbox USING btree (profile_type, profile_id, recipient_id, source_transition_id) WHERE ((reminder_stage)::text = 'reactivated'::text);


--
-- Name: kyb_rut_claim_verification_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_rut_claim_verification_idx ON public.kyb_rut_claim USING btree (verification_id, rut_kind);


--
-- Name: kyb_verification_document_verification_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_verification_document_verification_idx ON public.kyb_verification_document USING btree (verification_id, document_kind);


--
-- Name: kyb_verification_event_verification_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_verification_event_verification_idx ON public.kyb_verification_event USING btree (verification_id, occurred_at, id);


--
-- Name: kyb_verifications_one_open_per_organization_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX kyb_verifications_one_open_per_organization_uq ON public.kyb_verifications USING btree (organization_id) WHERE ((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[]));


--
-- Name: kyb_verifications_status_submitted_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kyb_verifications_status_submitted_idx ON public.kyb_verifications USING btree (status, submitted_at, id);


--
-- Name: m02_operacion_idempotente_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX m02_operacion_idempotente_retention_idx ON public.m02_operacion_idempotente_local USING btree (creada_en);


--
-- Name: media_asset_owner_profile_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX media_asset_owner_profile_idx ON public.media_asset USING btree (propietario_id, created_at DESC) WHERE (proposito = 'perfil'::text);


--
-- Name: media_asset_pending_replacement_target_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX media_asset_pending_replacement_target_uq ON public.media_asset USING btree (espacio_id, propietario_id, replacement_target_space_media_id) WHERE ((estado = 'pending'::text) AND (replacement_target_space_media_id IS NOT NULL));


--
-- Name: media_asset_profile_idempotency_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX media_asset_profile_idempotency_uq ON public.media_asset USING btree (propietario_id, idempotency_key) WHERE (proposito = 'perfil'::text);


--
-- Name: media_asset_space_idempotency_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX media_asset_space_idempotency_uq ON public.media_asset USING btree (propietario_id, espacio_id, idempotency_key) WHERE (proposito = 'espacio'::text);


--
-- Name: media_asset_space_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX media_asset_space_state_idx ON public.media_asset USING btree (espacio_id, estado) WHERE ((proposito = 'espacio'::text) AND (estado = ANY (ARRAY['pending'::text, 'ready'::text])));


--
-- Name: media_variant_intent_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX media_variant_intent_cleanup_idx ON public.media_variant_intent USING btree (media_id);


--
-- Name: media_variant_object_generation_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX media_variant_object_generation_uq ON public.media_variant USING btree (provider, bucket_name, object_key, generation);


--
-- Name: mensaje_reserva_comercial_page_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mensaje_reserva_comercial_page_idx ON public.mensaje_reserva_comercial USING btree (reserva_id, secuencia DESC);


--
-- Name: mensaje_reserva_ensayo_hilo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mensaje_reserva_ensayo_hilo_idx ON public.mensaje_reserva_ensayo USING btree (reserva_id, secuencia DESC);


--
-- Name: mercadopago_oauth_state_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mercadopago_oauth_state_expiry_idx ON public.mercadopago_oauth_state USING btree (expira_en);


--
-- Name: mercadopago_payment_webhook_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mercadopago_payment_webhook_lease_idx ON public.mercadopago_payment_webhook_receipt USING btree (claimed_at) WHERE (estado = 'processing'::text);


--
-- Name: mercadopago_payment_webhook_retry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mercadopago_payment_webhook_retry_idx ON public.mercadopago_payment_webhook_receipt USING btree (reintentar_en, recibida_en) WHERE (estado = ANY (ARRAY['received'::text, 'retry'::text]));


--
-- Name: notificacion_usuario_lista_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notificacion_usuario_lista_idx ON public.notificacion_usuario USING btree (usuario_id, creada_en DESC, id DESC);


--
-- Name: notificacion_usuario_no_leida_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notificacion_usuario_no_leida_idx ON public.notificacion_usuario USING btree (usuario_id) WHERE (leida_en IS NULL);


--
-- Name: notificacion_usuario_ref_uk; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX notificacion_usuario_ref_uk ON public.notificacion_usuario USING btree (referencia_id, usuario_id, tipo) WHERE (referencia_id IS NOT NULL);


--
-- Name: ocupacion_espacio_activa_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ocupacion_espacio_activa_idx ON public.ocupacion USING btree (espacio_id, lower(intervalo)) WHERE activo;


--
-- Name: ocupacion_reserva_comercial_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ocupacion_reserva_comercial_id_idx ON public.ocupacion USING btree (reserva_arriendo_comercial_id) WHERE (reserva_arriendo_comercial_id IS NOT NULL);


--
-- Name: operacion_arriendo_candidato_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operacion_arriendo_candidato_cleanup_idx ON public.operacion_arriendo_archivo_candidato_local USING btree (estado, proximo_intento_en, archivo_id);


--
-- Name: operacion_arriendo_evidencia_operacion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operacion_arriendo_evidencia_operacion_idx ON public.operacion_arriendo_evidencia_ensayo_local USING btree (operacion_id, id);


--
-- Name: operacion_arriendo_evidencia_retencion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operacion_arriendo_evidencia_retencion_idx ON public.operacion_arriendo_evidencia_ensayo_local USING btree (retirar_archivo_en, id) WHERE ((retirar_archivo_en IS NOT NULL) AND (archivo_retirado_en IS NULL));


--
-- Name: operacion_arriendo_historial_reserva_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operacion_arriendo_historial_reserva_idx ON public.operacion_arriendo_historial_ensayo_local USING btree (reserva_id, secuencia);


--
-- Name: operacion_arriendo_reserva_hist_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operacion_arriendo_reserva_hist_idx ON public.operacion_arriendo_ensayo_local USING btree (reserva_id, ocurrio_en, id);


--
-- Name: organizacion_company_rut_fingerprint_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX organizacion_company_rut_fingerprint_uq ON public.organizacion USING btree (company_rut_fingerprint) WHERE (company_rut_fingerprint IS NOT NULL);


--
-- Name: organizacion_invitacion_org_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX organizacion_invitacion_org_status_idx ON public.organizacion_invitacion USING btree (organizacion_id, estado, created_at DESC);


--
-- Name: organizacion_invitacion_pending_email_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX organizacion_invitacion_pending_email_uq ON public.organizacion_invitacion USING btree (organizacion_id, target_email_normalized) WHERE (((estado)::text = 'pendiente'::text) AND (target_email_normalized IS NOT NULL));


--
-- Name: organizacion_invitacion_pending_user_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX organizacion_invitacion_pending_user_uq ON public.organizacion_invitacion USING btree (organizacion_id, target_usuario_id) WHERE (((estado)::text = 'pendiente'::text) AND (target_usuario_id IS NOT NULL));


--
-- Name: organizacion_miembro_active_user_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX organizacion_miembro_active_user_uq ON public.organizacion_miembro USING btree (organizacion_id, usuario_id) WHERE ((estado)::text = 'activo'::text);


--
-- Name: organizacion_miembro_role_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX organizacion_miembro_role_idx ON public.organizacion_miembro USING btree (organizacion_id, rol) WHERE ((estado)::text = 'activo'::text);


--
-- Name: organizacion_miembro_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX organizacion_miembro_user_idx ON public.organizacion_miembro USING btree (usuario_id, estado, organizacion_id);


--
-- Name: outbox_evento_local_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX outbox_evento_local_pendiente_idx ON public.outbox_evento_local USING btree (disponible_en, creada_en, id) WHERE (entregada_en IS NULL);


--
-- Name: outbox_evento_local_terminal_retention_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX outbox_evento_local_terminal_retention_idx ON public.outbox_evento_local USING btree (retirar_en, id) WHERE (retirar_en IS NOT NULL);


--
-- Name: perfil_usuario_nombre_lower_prefix_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX perfil_usuario_nombre_lower_prefix_idx ON public.perfil_usuario USING btree (lower(nombre_visible) text_pattern_ops);


--
-- Name: privacidad_bloqueo_activo_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX privacidad_bloqueo_activo_uq ON public.privacidad_bloqueo_tratamiento USING btree (cuenta_id, ambito) WHERE (estado = 'activo'::text);


--
-- Name: privacidad_bloqueo_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_bloqueo_cuenta_idx ON public.privacidad_bloqueo_tratamiento USING btree (cuenta_id, aplicado_en DESC);


--
-- Name: privacidad_bloqueo_solicitud_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_bloqueo_solicitud_idx ON public.privacidad_bloqueo_tratamiento USING btree (solicitud_id, aplicado_en DESC);


--
-- Name: privacidad_ejecucion_solicitud_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_ejecucion_solicitud_idx ON public.privacidad_ejecucion_derecho USING btree (solicitud_id, ejecutado_en);


--
-- Name: privacidad_incidente_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_incidente_estado_idx ON public.privacidad_incidente_datos USING btree (estado, severidad, creado_en DESC);


--
-- Name: privacidad_notificacion_outbox_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_notificacion_outbox_pending_idx ON public.privacidad_notificacion_outbox USING btree (disponible_en, creado_en, id) WHERE (estado = ANY (ARRAY['pending'::text, 'processing'::text]));


--
-- Name: privacidad_notificacion_outbox_solicitud_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_notificacion_outbox_solicitud_idx ON public.privacidad_notificacion_outbox USING btree (solicitud_id, creado_en DESC);


--
-- Name: privacidad_retencion_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_retencion_cuenta_idx ON public.privacidad_retencion_legal USING btree (cuenta_id, estado, aplicada_en DESC);


--
-- Name: privacidad_retencion_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX privacidad_retencion_estado_idx ON public.privacidad_retencion_legal USING btree (estado, aplicada_en DESC);


--
-- Name: reaplicacion_baja_local_usuario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reaplicacion_baja_local_usuario_idx ON public.reaplicacion_baja_local USING btree (usuario_id, aplicada_en);


--
-- Name: reclamo_dano_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reclamo_dano_historial_idx ON public.reclamo_dano_historial_ensayo_local USING btree (reclamo_id, secuencia);


--
-- Name: registro_replay_baja_local_completed_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX registro_replay_baja_local_completed_idx ON public.registro_replay_baja_local USING btree (completada_en, ejecucion_origen_id);


--
-- Name: reporte_resena_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reporte_resena_historial_idx ON public.reporte_resena_historial_local USING btree (reporte_id, secuencia);


--
-- Name: reporte_resena_retencion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reporte_resena_retencion_idx ON public.reporte_resena_ensayo_local USING btree (retirar_en, id) WHERE (retirar_en IS NOT NULL);


--
-- Name: resena_arrendatario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_arrendatario_idx ON public.resena_ensayo_local USING btree (destinatario_id, creada_en DESC, id) WHERE ((destinatario_tipo = 'arrendatario'::text) AND (estado = ANY (ARRAY['publicada'::text, 'reportada'::text])));


--
-- Name: resena_autoria_marca_retencion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_autoria_marca_retencion_idx ON public.resena_autoria_marca_local USING btree (retirar_en, huella_autoria);


--
-- Name: resena_espacio_publica_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_espacio_publica_idx ON public.resena_ensayo_local USING btree (espacio_id, creada_en DESC, id) WHERE (estado = ANY (ARRAY['publicada'::text, 'reportada'::text]));


--
-- Name: resena_reserva_comercial_author_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_reserva_comercial_author_idx ON public.resena_reserva_comercial USING btree (autor_id, creada_en DESC) WHERE (autor_id IS NOT NULL);


--
-- Name: resena_reserva_comercial_space_visible_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_reserva_comercial_space_visible_idx ON public.resena_reserva_comercial USING btree (espacio_id, creada_en DESC) WHERE ((destinatario_tipo = 'espacio'::text) AND (estado = ANY (ARRAY['publicada'::text, 'reportada'::text])));


--
-- Name: resena_retirar_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX resena_retirar_idx ON public.resena_ensayo_local USING btree (retirar_en, id);


--
-- Name: reserva_arriendo_comercial_anfitrion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_arriendo_comercial_anfitrion_idx ON public.reserva_arriendo_comercial USING btree (anfitrion_id, actualizada_en DESC, id);


--
-- Name: reserva_arriendo_comercial_arrendatario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_arriendo_comercial_arrendatario_idx ON public.reserva_arriendo_comercial USING btree (arrendatario_id, actualizada_en DESC, id);


--
-- Name: reserva_arriendo_tramo_estado_codigo_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_arriendo_tramo_estado_codigo_idx ON public.reserva_arriendo_tramo_estado USING btree (codigo_corto) WHERE (codigo_corto IS NOT NULL);


--
-- Name: reserva_arriendo_tramo_estado_reserva_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_arriendo_tramo_estado_reserva_idx ON public.reserva_arriendo_tramo_estado USING btree (reserva_id);


--
-- Name: reserva_comercial_cancelacion_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_comercial_cancelacion_pendiente_idx ON public.reserva_comercial_cancelacion USING btree (actualizada_en) WHERE (refund_status = 'pendiente'::text);


--
-- Name: reserva_cuota_mensual_reserva_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_cuota_mensual_reserva_idx ON public.reserva_cuota_mensual USING btree (reserva_arriendo_comercial_id, numero_cuota);


--
-- Name: reserva_devolucion_intento_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_devolucion_intento_historial_idx ON public.reserva_devolucion_intento_ensayo USING btree (devolucion_id, secuencia);


--
-- Name: reserva_ensayo_local_anfitrion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_local_anfitrion_idx ON public.reserva_ensayo_local USING btree (anfitrion_id, actualizada_en DESC, id);


--
-- Name: reserva_ensayo_local_arrendatario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_local_arrendatario_idx ON public.reserva_ensayo_local USING btree (arrendatario_id, actualizada_en DESC, id);


--
-- Name: reserva_ensayo_local_fixture_enabled_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_local_fixture_enabled_idx ON public.reserva_ensayo_local_fixture USING btree (espacio_id) WHERE habilitada;


--
-- Name: reserva_ensayo_local_ubicacion_gist; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_local_ubicacion_gist ON public.reserva_ensayo_local_ubicacion_sintetica USING gist (punto);


--
-- Name: reserva_ensayo_retencion_local_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_retencion_local_idx ON public.reserva_ensayo_local USING btree (vinculos_retirar_en, id) WHERE (estado = ANY (ARRAY['finalizada'::text, 'cerrada'::text, 'cancelada_por_pago'::text, 'rechazada_arrendador'::text, 'vencida_pago'::text, 'vencida_host'::text, 'cancelada_arrendatario'::text, 'cancelada_por_firma'::text]));


--
-- Name: reserva_ensayo_transicion_historial_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_ensayo_transicion_historial_idx ON public.reserva_ensayo_transicion USING btree (reserva_id, secuencia);


--
-- Name: reserva_garantia_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_garantia_estado_idx ON public.reserva_garantia_ensayo_local USING btree (estado, actualizada_en);


--
-- Name: reserva_garantia_evento_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_garantia_evento_pendiente_idx ON public.reserva_garantia_evento_aplicacion_ensayo_local USING btree (estado, creada_en) WHERE (estado = 'pendiente'::text);


--
-- Name: reserva_garantia_operacion_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_garantia_operacion_pendiente_idx ON public.reserva_garantia_operacion_ensayo_local USING btree (estado, vence_en) WHERE (estado = ANY (ARRAY['pendiente'::text, 'por_conciliar'::text]));


--
-- Name: reserva_liquidacion_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_liquidacion_pendiente_idx ON public.reserva_liquidacion_ensayo_local USING btree (estado, actualizada_en, id) WHERE (estado = ANY (ARRAY['pendiente'::text, 'por_conciliar'::text]));


--
-- Name: reserva_pago_ensayo_operacion_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_pago_ensayo_operacion_pendiente_idx ON public.reserva_pago_ensayo_operacion USING btree (creada_en, id) WHERE (estado = 'pendiente'::text);


--
-- Name: reserva_pago_ensayo_operacion_pendiente_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX reserva_pago_ensayo_operacion_pendiente_uq ON public.reserva_pago_ensayo_operacion USING btree (reserva_id) WHERE (estado = 'pendiente'::text);


--
-- Name: reserva_pago_evento_aplicacion_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_pago_evento_aplicacion_pendiente_idx ON public.reserva_pago_evento_aplicacion_ensayo USING btree (creada_en, evento_id) WHERE (estado = ANY (ARRAY['pendiente'::text, 'pendiente_conciliacion'::text]));


--
-- Name: reserva_pago_evento_ensayo_operacion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reserva_pago_evento_ensayo_operacion_idx ON public.reserva_pago_evento_ensayo USING btree (operacion_id, recibido_en, id);


--
-- Name: rol_usuario_concedido_por_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX rol_usuario_concedido_por_idx ON public.rol_usuario USING btree (concedido_por) WHERE (concedido_por IS NOT NULL);


--
-- Name: sesion_administrativa_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sesion_administrativa_cuenta_idx ON public.sesion_administrativa USING btree (cuenta_id, creada_en DESC);


--
-- Name: sesion_usuario_activa_actividad_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sesion_usuario_activa_actividad_idx ON public.sesion USING btree (usuario_id, expira_en, ultima_actividad_en DESC) WHERE (revocada_en IS NULL);


--
-- Name: sesion_usuario_activa_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sesion_usuario_activa_idx ON public.sesion USING btree (usuario_id, expira_en) WHERE (revocada_en IS NULL);


--
-- Name: sesion_usuario_actividad_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sesion_usuario_actividad_idx ON public.sesion USING btree (usuario_id, revocada_en, expira_en, ultima_actividad_en DESC);


--
-- Name: sesion_usuario_id_pinpass_uk; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX sesion_usuario_id_pinpass_uk ON public.sesion USING btree (usuario_id, id);


--
-- Name: simulacion_precio_espacio_propietario_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX simulacion_precio_espacio_propietario_idx ON public.simulacion_precio_privada USING btree (espacio_id, propietario_id, creada_en DESC, id);


--
-- Name: solicitud_privacidad_administrativa_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_privacidad_administrativa_cuenta_idx ON public.solicitud_privacidad_administrativa USING btree (cuenta_id, creado_en DESC);


--
-- Name: solicitud_privacidad_administrativa_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_privacidad_administrativa_estado_idx ON public.solicitud_privacidad_administrativa USING btree (estado, creado_en DESC, id DESC);


--
-- Name: solicitud_titular_asignado_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_titular_asignado_estado_idx ON public.solicitud_titular USING btree (asignado_a_id, estado, solicitada_en DESC);


--
-- Name: solicitud_titular_evento_solicitud_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_titular_evento_solicitud_idx ON public.solicitud_titular_evento USING btree (solicitud_id, secuencia);


--
-- Name: solicitud_titular_folio_publico_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX solicitud_titular_folio_publico_uq ON public.solicitud_titular USING btree (folio_publico);


--
-- Name: solicitud_titular_usuario_fecha_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_titular_usuario_fecha_idx ON public.solicitud_titular USING btree (usuario_id, solicitada_en DESC);


--
-- Name: solicitud_titular_vence_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX solicitud_titular_vence_idx ON public.solicitud_titular USING btree (COALESCE(prorrogada_hasta, vence_en), estado);


--
-- Name: space_kyb_verification_one_open_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX space_kyb_verification_one_open_uq ON public.space_kyb_verification USING btree (espacio_id, document_kind) WHERE ((status)::text = ANY ((ARRAY['PENDING'::character varying, 'IN_REVIEW'::character varying])::text[]));


--
-- Name: space_kyb_verification_org_cat_sii_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX space_kyb_verification_org_cat_sii_idx ON public.space_kyb_verification USING btree (organizacion_id, categoria_codigo) WHERE (((host_mode)::text = 'empresa'::text) AND ((document_kind)::text = 'sii_start_activities'::text) AND ((status)::text = 'APPROVED'::text));


--
-- Name: space_kyb_verification_space_history_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX space_kyb_verification_space_history_idx ON public.space_kyb_verification USING btree (espacio_id, submitted_at DESC, id);


--
-- Name: space_media_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX space_media_active_idx ON public.space_media USING btree (espacio_id, created_at, id) WHERE (retired_at IS NULL);


--
-- Name: tarifa_espacio_latest_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tarifa_espacio_latest_idx ON public.tarifa_espacio USING btree (espacio_id, version DESC);


--
-- Name: ticket_soporte_administrativo_cuenta_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ticket_soporte_administrativo_cuenta_idx ON public.ticket_soporte_administrativo USING btree (cuenta_id, creado_en DESC) WHERE (cuenta_id IS NOT NULL);


--
-- Name: ticket_soporte_administrativo_estado_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ticket_soporte_administrativo_estado_idx ON public.ticket_soporte_administrativo USING btree (estado, creado_en DESC, id DESC);


--
-- Name: token_accion_usuario_emision_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX token_accion_usuario_emision_idx ON public.token_accion USING btree (usuario_id, proposito, creado_en);


--
-- Name: token_accion_usuario_pendiente_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX token_accion_usuario_pendiente_idx ON public.token_accion USING btree (usuario_id, proposito, expira_en) WHERE ((consumido_en IS NULL) AND (invalidado_en IS NULL));


--
-- Name: usuario_correo_normalizado_prefix_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX usuario_correo_normalizado_prefix_idx ON public.usuario USING btree (correo_normalizado text_pattern_ops);


--
-- Name: usuario_creado_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX usuario_creado_id_idx ON public.usuario USING btree (creado_en DESC, id DESC);


--
-- Name: usuario_estado_creado_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX usuario_estado_creado_id_idx ON public.usuario USING btree (estado, creado_en DESC, id DESC);


--
-- Name: verificacion_archivo_retiro_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_archivo_retiro_claim_idx ON public.verificacion_archivo_retiro_pendiente_local USING btree (disponible_en, evidencia_id);


--
-- Name: verificacion_evidencia_archivo_retencion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_evidencia_archivo_retencion_idx ON public.verificacion_evidencia_sintetica USING btree (retirar_archivo_en, id) WHERE ((archivo_id IS NOT NULL) AND (retirar_archivo_en IS NOT NULL));


--
-- Name: verificacion_evidencia_case_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_evidencia_case_idx ON public.verificacion_evidencia_sintetica USING btree (verificacion_id, creada_en DESC);


--
-- Name: verificacion_historial_order_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_historial_order_idx ON public.verificacion_historial_local USING btree (verificacion_id, id);


--
-- Name: verificacion_reintento_unico_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX verificacion_reintento_unico_idx ON public.verificacion USING btree (reintento_de) WHERE (reintento_de IS NOT NULL);


--
-- Name: verificacion_revision_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_revision_idx ON public.verificacion USING btree (creada_en) WHERE (estado = 'en_revision'::text);


--
-- Name: verificacion_usuario_fecha_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verificacion_usuario_fecha_idx ON public.verificacion USING btree (usuario_id, creada_en DESC);


--
-- Name: vista_asistida_administrativa_actor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX vista_asistida_administrativa_actor_idx ON public.vista_asistida_administrativa USING btree (actor_cuenta_id, revocada_en, expira_en);


--
-- Name: anfitrion_cumplimiento_personal anfitrion_personal_reactivation_notice; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER anfitrion_personal_reactivation_notice AFTER UPDATE OF compliance_status ON public.anfitrion_cumplimiento_personal FOR EACH ROW WHEN ((((old.compliance_status)::text = 'disabled_missing_docs'::text) AND ((new.compliance_status)::text = 'compliant'::text))) EXECUTE FUNCTION public.kyb_personal_reactivation_notice();


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arriendo_temprano_confirmacion_guard BEFORE INSERT OR DELETE OR UPDATE ON public.arriendo_temprano_confirmacion FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_confirmacion_guard();


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_consistency; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER arriendo_temprano_evento_consistency AFTER INSERT ON public.arriendo_temprano_evento DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_consistency();


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arriendo_temprano_evento_guard BEFORE INSERT OR DELETE OR UPDATE ON public.arriendo_temprano_evento FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_guard();


--
-- Name: arriendo_temprano arriendo_temprano_evento_revision_consistency; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER arriendo_temprano_evento_revision_consistency AFTER INSERT OR UPDATE ON public.arriendo_temprano DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_evento_consistency();


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotencia_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arriendo_temprano_idempotencia_guard BEFORE INSERT OR DELETE OR UPDATE ON public.arriendo_temprano_idempotencia FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_idempotencia_guard();


--
-- Name: arriendo_temprano arriendo_temprano_immutable_update_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arriendo_temprano_immutable_update_trg BEFORE UPDATE ON public.arriendo_temprano FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_immutable_update();


--
-- Name: arriendo_temprano arriendo_temprano_occupancy_consistency_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER arriendo_temprano_occupancy_consistency_trg AFTER INSERT OR UPDATE ON public.arriendo_temprano DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_occupancy_consistency();


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER arriendo_temprano_pago_guard BEFORE INSERT OR DELETE OR UPDATE ON public.arriendo_temprano_pago FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_pago_guard();


--
-- Name: categoria_espacio categoria_public_search_document_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER categoria_public_search_document_trg AFTER UPDATE OF nombre ON public.categoria_espacio FOR EACH ROW WHEN ((old.nombre IS DISTINCT FROM new.nombre)) EXECUTE FUNCTION public.refresh_category_public_search_document();


--
-- Name: checkout_preferencia_comercial checkout_commercial_contract_binding_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER checkout_commercial_contract_binding_guard BEFORE INSERT OR UPDATE ON public.checkout_preferencia_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_checkout_commercial_contract_binding();


--
-- Name: contrato_propuesta_comercial commercial_contract_proposal_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_contract_proposal_immutable BEFORE DELETE OR UPDATE ON public.contrato_propuesta_comercial FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_contract_template_version();


--
-- Name: espacio_contrato_version commercial_contract_template_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_contract_template_immutable BEFORE DELETE OR UPDATE ON public.espacio_contrato_version FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_contract_template_version();


--
-- Name: autorizacion_arriendo_comercial_evento commercial_rental_authorization_event_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_rental_authorization_event_immutable BEFORE DELETE OR UPDATE ON public.autorizacion_arriendo_comercial_evento FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_rental_authorization_event();


--
-- Name: autorizacion_arriendo_comercial commercial_rental_authorization_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_rental_authorization_guard BEFORE INSERT OR UPDATE ON public.autorizacion_arriendo_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_rental_authorization();


--
-- Name: espacio_contrato_borrador commercial_rental_authorization_template_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_rental_authorization_template_change AFTER UPDATE OF version_publicada ON public.espacio_contrato_borrador FOR EACH ROW EXECUTE FUNCTION public.revoke_commercial_rental_authorization_on_template_change();


--
-- Name: reserva_documento_tributario_comercial commercial_rental_tax_document_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER commercial_rental_tax_document_guard BEFORE INSERT OR DELETE OR UPDATE ON public.reserva_documento_tributario_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_rental_tax_document();


--
-- Name: contrato_documento_comercial contrato_documento_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER contrato_documento_guard BEFORE INSERT OR UPDATE ON public.contrato_documento_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_contract_document();


--
-- Name: contrato_firma_comercial contrato_firma_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER contrato_firma_guard BEFORE INSERT OR DELETE OR UPDATE ON public.contrato_firma_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_signature();


--
-- Name: contrato_solicitud_evento_comercial contrato_solicitud_evento_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER contrato_solicitud_evento_immutable BEFORE DELETE OR UPDATE ON public.contrato_solicitud_evento_comercial FOR EACH ROW EXECUTE FUNCTION public.protect_commercial_agreement_event();


--
-- Name: contrato_solicitud_comercial contrato_solicitud_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER contrato_solicitud_guard BEFORE INSERT OR UPDATE ON public.contrato_solicitud_comercial FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_agreement();


--
-- Name: contrato_solicitud_comercial contrato_solicitud_quote_anchor; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER contrato_solicitud_quote_anchor AFTER INSERT OR UPDATE ON public.contrato_solicitud_comercial FOR EACH ROW EXECUTE FUNCTION public.maintain_commercial_agreement_quote_anchor();


--
-- Name: contrato_solicitud_comercial contrato_solicitud_signatures; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER contrato_solicitud_signatures AFTER INSERT OR UPDATE ON public.contrato_solicitud_comercial DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_commercial_agreement_signatures();


--
-- Name: document_verification_event document_verification_event_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_event_no_truncate BEFORE TRUNCATE ON public.document_verification_event FOR EACH STATEMENT EXECUTE FUNCTION public.document_verification_event_immutable();


--
-- Name: document_verification_event document_verification_event_no_update_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_event_no_update_delete BEFORE DELETE OR UPDATE ON public.document_verification_event FOR EACH ROW EXECUTE FUNCTION public.document_verification_event_immutable();


--
-- Name: document_verification_reviewer_grant_event document_verification_grant_event_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_grant_event_no_truncate BEFORE TRUNCATE ON public.document_verification_reviewer_grant_event FOR EACH STATEMENT EXECUTE FUNCTION public.document_verification_event_immutable();


--
-- Name: document_verification_reviewer_grant_event document_verification_grant_event_no_update_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_grant_event_no_update_delete BEFORE DELETE OR UPDATE ON public.document_verification_reviewer_grant_event FOR EACH ROW EXECUTE FUNCTION public.document_verification_event_immutable();


--
-- Name: document_verification_case document_verification_identity_case_insert_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_identity_case_insert_guard BEFORE INSERT ON public.document_verification_case FOR EACH ROW EXECUTE FUNCTION public.document_verification_identity_case_insert_guard();


--
-- Name: document_verification_case document_verification_identity_case_link_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_identity_case_link_guard BEFORE UPDATE OF identity_declaration_revision_id ON public.document_verification_case FOR EACH ROW EXECUTE FUNCTION public.document_verification_identity_case_link_guard();


--
-- Name: document_verification_event document_verification_identity_review_audit_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_identity_review_audit_guard BEFORE INSERT ON public.document_verification_event FOR EACH ROW EXECUTE FUNCTION public.document_verification_identity_review_audit_guard();


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_audit; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_reviewer_grant_audit AFTER INSERT OR UPDATE ON public.document_verification_reviewer_grant FOR EACH ROW EXECUTE FUNCTION public.document_verification_reviewer_grant_audit();


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_reviewer_grant_guard BEFORE INSERT OR DELETE OR UPDATE ON public.document_verification_reviewer_grant FOR EACH ROW EXECUTE FUNCTION public.document_verification_reviewer_grant_guard();


--
-- Name: espacio document_verification_space_owner_changed_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER document_verification_space_owner_changed_trg AFTER UPDATE OF propietario_id ON public.espacio FOR EACH ROW WHEN ((old.propietario_id IS DISTINCT FROM new.propietario_id)) EXECUTE FUNCTION public.document_verification_space_owner_changed();


--
-- Name: espacio espacio_commercial_location_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_commercial_location_guard BEFORE UPDATE OF direccion, ubicacion_catalogo_version, region_codigo, comuna_codigo, ubicacion_latitud, ubicacion_longitud, ubicacion_confirmada_en, ubicacion_revision ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_space_location();


--
-- Name: espacio espacio_commercial_pin_provenance_clear; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_commercial_pin_provenance_clear BEFORE UPDATE OF direccion, ubicacion_catalogo_version, region_codigo, comuna_codigo, ubicacion_latitud, ubicacion_longitud ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.clear_commercial_space_pin_provenance();


--
-- Name: espacio_solicitud_documental espacio_document_request_notice; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_document_request_notice AFTER INSERT ON public.espacio_solicitud_documental FOR EACH ROW EXECUTE FUNCTION public.kyb_space_document_request_notice();


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_historial_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_eliminacion_historial_append_only BEFORE DELETE OR UPDATE ON public.espacio_eliminacion_historial_local FOR EACH ROW EXECUTE FUNCTION public.prevent_local_space_deletion_history_mutation();


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_retention_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_eliminacion_retention_trg BEFORE INSERT ON public.espacio_eliminacion_historial_local FOR EACH ROW EXECUTE FUNCTION public.assign_local_space_history_retention();


--
-- Name: espacio espacio_modalidad_horario_semanal_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_modalidad_horario_semanal_guard BEFORE UPDATE OF modalidad_tarifa ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.guardar_modalidad_horario_activo();


--
-- Name: espacio espacio_public_search_document_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_public_search_document_trg BEFORE INSERT OR UPDATE OF estado, categoria_codigo, titulo, descripcion, ubicacion_catalogo_version, region_codigo, comuna_codigo ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.refresh_espacio_public_search_document();


--
-- Name: espacio_publicacion_historial_local espacio_publicacion_historial_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_publicacion_historial_append_only BEFORE DELETE OR UPDATE ON public.espacio_publicacion_historial_local FOR EACH ROW EXECUTE FUNCTION public.prevent_local_publication_history_mutation();


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_no_mutation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_solicitud_documental_no_mutation BEFORE DELETE OR UPDATE ON public.espacio_solicitud_documental FOR EACH ROW EXECUTE FUNCTION public.host_compliance_event_immutable();


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_solicitud_documental_no_truncate BEFORE TRUNCATE ON public.espacio_solicitud_documental FOR EACH STATEMENT EXECUTE FUNCTION public.host_compliance_event_immutable();


--
-- Name: espacio espacio_start_kyb_grace_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_start_kyb_grace_insert AFTER INSERT ON public.espacio FOR EACH ROW WHEN ((new.estado = 'activa'::text)) EXECUTE FUNCTION public.kyb_start_space_grace();


--
-- Name: espacio espacio_start_kyb_grace_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_start_kyb_grace_update AFTER UPDATE OF estado ON public.espacio FOR EACH ROW WHEN (((new.estado = 'activa'::text) AND (old.estado IS DISTINCT FROM 'activa'::text))) EXECUTE FUNCTION public.kyb_start_space_grace();


--
-- Name: espacio espacio_zona_horario_semanal_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER espacio_zona_horario_semanal_guard BEFORE UPDATE OF zona_horaria ON public.espacio FOR EACH ROW EXECUTE FUNCTION public.guardar_zona_sin_horario_activo();


--
-- Name: espacio_horario_semanal horario_semanal_fixture_hora_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER horario_semanal_fixture_hora_guard BEFORE INSERT OR UPDATE OF activo ON public.espacio_horario_semanal FOR EACH ROW EXECUTE FUNCTION public.validar_horario_semanal_fixture_hora();


--
-- Name: host_compliance_event host_compliance_event_no_mutation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_compliance_event_no_mutation BEFORE DELETE OR UPDATE ON public.host_compliance_event FOR EACH ROW EXECUTE FUNCTION public.host_compliance_event_immutable();


--
-- Name: host_compliance_event host_compliance_event_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_compliance_event_no_truncate BEFORE TRUNCATE ON public.host_compliance_event FOR EACH STATEMENT EXECUTE FUNCTION public.host_compliance_event_immutable();


--
-- Name: identity_declaration_revision identity_declaration_case_retention_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER identity_declaration_case_retention_guard BEFORE DELETE ON public.identity_declaration_revision FOR EACH ROW EXECUTE FUNCTION public.identity_declaration_case_retention_guard();


--
-- Name: kyb_verifications kyb_bind_company_rut_before_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER kyb_bind_company_rut_before_insert BEFORE INSERT ON public.kyb_verifications FOR EACH ROW EXECUTE FUNCTION public.kyb_bind_company_rut();


--
-- Name: kyb_verifications kyb_submission_member_before_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER kyb_submission_member_before_insert BEFORE INSERT ON public.kyb_verifications FOR EACH ROW EXECUTE FUNCTION public.kyb_submission_requires_active_member();


--
-- Name: kyb_verification_event kyb_verification_event_no_mutation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER kyb_verification_event_no_mutation BEFORE DELETE OR UPDATE ON public.kyb_verification_event FOR EACH ROW EXECUTE FUNCTION public.kyb_verification_event_immutable();


--
-- Name: kyb_verification_event kyb_verification_event_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER kyb_verification_event_no_truncate BEFORE TRUNCATE ON public.kyb_verification_event FOR EACH STATEMENT EXECUTE FUNCTION public.kyb_verification_event_immutable();


--
-- Name: reserva_ensayo_local local_booking_payment_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_booking_payment_notice_trg AFTER UPDATE OF estado ON public.reserva_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_booking_payment_notices();


--
-- Name: reserva_ensayo_local local_cancel_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_cancel_notice_trg AFTER UPDATE OF estado ON public.reserva_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_local_cancel_notice();


--
-- Name: operacion_arriendo_ensayo_local local_checkin_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_checkin_notice_trg AFTER INSERT ON public.operacion_arriendo_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_local_checkin_notice();


--
-- Name: reclamo_dano_ensayo_local local_claim_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_claim_notice_trg AFTER INSERT ON public.reclamo_dano_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_local_claim_notice();


--
-- Name: contrato_ensayo_local local_contract_complete_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_contract_complete_notice_trg AFTER UPDATE OF estado ON public.contrato_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_contract_complete_notices();


--
-- Name: contrato_ensayo_firma local_contract_signature_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_contract_signature_notice_trg AFTER INSERT ON public.contrato_ensayo_firma FOR EACH ROW EXECUTE FUNCTION public.enqueue_contract_signature_notices();


--
-- Name: reclamo_dano_resolucion_ensayo_local local_damage_claim_resolution_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_damage_claim_resolution_notice_trg AFTER INSERT ON public.reclamo_dano_resolucion_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_local_damage_claim_resolution_notice();


--
-- Name: usuario local_login_lock_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_login_lock_notice_trg AFTER UPDATE OF bloqueado_hasta ON public.usuario FOR EACH ROW EXECUTE FUNCTION public.enqueue_login_lock_notice();


--
-- Name: reporte_resena_ensayo_local local_review_report_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_review_report_notice_trg AFTER INSERT ON public.reporte_resena_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.enqueue_local_review_report_notice();


--
-- Name: verificacion local_verification_result_notice_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER local_verification_result_notice_trg AFTER UPDATE OF estado ON public.verificacion FOR EACH ROW EXECUTE FUNCTION public.enqueue_verification_result_notice();


--
-- Name: media_asset media_asset_replacement_request_revision_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER media_asset_replacement_request_revision_guard BEFORE INSERT OR UPDATE OF replacement_target_space_media_id, replacement_expected_revision ON public.media_asset FOR EACH ROW EXECUTE FUNCTION public.img01b_guard_replacement_request_revision();


--
-- Name: media_asset media_asset_slot_reservation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER media_asset_slot_reservation BEFORE INSERT OR UPDATE ON public.media_asset FOR EACH ROW EXECUTE FUNCTION public.img01b_enforce_gallery_slot();


--
-- Name: ocupacion ocupacion_arriendo_temprano_consistency_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER ocupacion_arriendo_temprano_consistency_trg AFTER INSERT OR UPDATE ON public.ocupacion DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.arriendo_temprano_occupancy_consistency();


--
-- Name: organizacion_miembro organizacion_miembro_legal_rep_ck; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER organizacion_miembro_legal_rep_ck AFTER INSERT OR DELETE OR UPDATE ON public.organizacion_miembro DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.organizacion_miembro_requires_legal_rep();


--
-- Name: organizacion organizacion_reactivation_notice; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER organizacion_reactivation_notice AFTER UPDATE OF compliance_status ON public.organizacion FOR EACH ROW WHEN ((((old.compliance_status)::text = 'disabled_missing_docs'::text) AND ((new.compliance_status)::text = 'compliant'::text))) EXECUTE FUNCTION public.kyb_organization_reactivation_notice();


--
-- Name: profile_media profile_media_scope_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER profile_media_scope_guard BEFORE INSERT OR UPDATE OF media_id, propietario_id ON public.profile_media FOR EACH ROW EXECUTE FUNCTION public.img01b_check_media_relation_scope();


--
-- Name: reserva_ensayo_local reserva_cerrada_liquidacion_fake_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER reserva_cerrada_liquidacion_fake_guard BEFORE UPDATE OF estado ON public.reserva_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.guard_local_reservation_close_after_settlement();


--
-- Name: reserva_ensayo_local reserva_vinculos_retencion_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER reserva_vinculos_retencion_trg BEFORE INSERT OR UPDATE OF estado ON public.reserva_ensayo_local FOR EACH ROW EXECUTE FUNCTION public.schedule_local_reservation_link_retention();


--
-- Name: space_kyb_verification space_kyb_owner_guard_before_insert_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER space_kyb_owner_guard_before_insert_update BEFORE INSERT OR UPDATE ON public.space_kyb_verification FOR EACH ROW EXECUTE FUNCTION public.space_kyb_owner_guard();


--
-- Name: space_media space_media_scope_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER space_media_scope_guard BEFORE INSERT OR UPDATE OF media_id, propietario_id, espacio_id ON public.space_media FOR EACH ROW EXECUTE FUNCTION public.img01b_check_media_relation_scope();


--
-- Name: espacio_galeria_archivo_candidato_local synthetic_candidate_slot_reservation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER synthetic_candidate_slot_reservation BEFORE INSERT OR UPDATE ON public.espacio_galeria_archivo_candidato_local FOR EACH ROW EXECUTE FUNCTION public.img01b_enforce_gallery_slot();


--
-- Name: espacio_galeria_sintetica_local synthetic_gallery_slot_reservation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER synthetic_gallery_slot_reservation BEFORE INSERT OR UPDATE ON public.espacio_galeria_sintetica_local FOR EACH ROW EXECUTE FUNCTION public.img01b_enforce_gallery_slot();


--
-- Name: tarifa_espacio tarifa_horario_semanal_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tarifa_horario_semanal_guard BEFORE INSERT ON public.tarifa_espacio FOR EACH ROW EXECUTE FUNCTION public.rechazar_cambio_tarifa_con_horario_activo();


--
-- Name: usuario usuario_gadm_bootstrap_identity_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER usuario_gadm_bootstrap_identity_immutable BEFORE UPDATE OF id ON public.usuario FOR EACH ROW EXECUTE FUNCTION public.reject_gadm_bootstrap_identity_change();


--
-- Name: verificacion verificacion_evidencia_retencion_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER verificacion_evidencia_retencion_trg AFTER UPDATE OF estado, resuelta_en, retirada_privacidad_en ON public.verificacion FOR EACH ROW EXECUTE FUNCTION public.set_verification_evidence_retention_deadline();


--
-- Name: verificacion verificacion_retencion_sintetica_trg; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER verificacion_retencion_sintetica_trg BEFORE INSERT OR UPDATE OF estado, resuelta_en ON public.verificacion FOR EACH ROW EXECUTE FUNCTION public.set_local_verification_retention_deadline();


--
-- Name: acceso_sensible_administrativo acceso_sensible_administrativo_actor_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceso_sensible_administrativo
    ADD CONSTRAINT acceso_sensible_administrativo_actor_cuenta_id_fkey FOREIGN KEY (actor_cuenta_id) REFERENCES public.usuario(id);


--
-- Name: acceso_sensible_administrativo acceso_sensible_administrativo_objetivo_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceso_sensible_administrativo
    ADD CONSTRAINT acceso_sensible_administrativo_objetivo_cuenta_id_fkey FOREIGN KEY (objetivo_cuenta_id) REFERENCES public.usuario(id);


--
-- Name: acceso_sensible_administrativo acceso_sensible_administrativo_sesion_administrativa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceso_sensible_administrativo
    ADD CONSTRAINT acceso_sensible_administrativo_sesion_administrativa_id_fkey FOREIGN KEY (sesion_administrativa_id) REFERENCES public.sesion_administrativa(id);


--
-- Name: account_identity_profile account_identity_profile_account_id_current_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile
    ADD CONSTRAINT account_identity_profile_account_id_current_revision_fkey FOREIGN KEY (account_id, current_revision) REFERENCES public.account_identity_profile_revision(account_id, id);


--
-- Name: account_identity_profile account_identity_profile_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile
    ADD CONSTRAINT account_identity_profile_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_identity_profile_draft account_identity_profile_draft_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_draft
    ADD CONSTRAINT account_identity_profile_draft_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_identity_profile_phone account_identity_profile_phone_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_phone
    ADD CONSTRAINT account_identity_profile_phone_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.account_identity_profile(account_id) ON DELETE CASCADE;


--
-- Name: account_identity_profile_revision account_identity_profile_revision_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_profile_revision
    ADD CONSTRAINT account_identity_profile_revision_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_identity_rut_claim account_identity_rut_claim_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_claim
    ADD CONSTRAINT account_identity_rut_claim_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id);


--
-- Name: account_identity_rut_claim account_identity_rut_claim_account_id_identity_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_claim
    ADD CONSTRAINT account_identity_rut_claim_account_id_identity_revision_fkey FOREIGN KEY (account_id, identity_revision) REFERENCES public.account_identity_profile_revision(account_id, id);


--
-- Name: account_identity_rut_fingerprint account_identity_rut_fingerprint_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_rut_fingerprint
    ADD CONSTRAINT account_identity_rut_fingerprint_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.account_identity_rut_claim(account_id);


--
-- Name: account_identity_verification account_identity_verification_account_id_profile_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_verification
    ADD CONSTRAINT account_identity_verification_account_id_profile_revision_fkey FOREIGN KEY (account_id, profile_revision) REFERENCES public.account_identity_profile_revision(account_id, id) ON DELETE CASCADE;


--
-- Name: account_identity_webhook_receipt account_identity_webhook_receipt_attempt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_identity_webhook_receipt
    ADD CONSTRAINT account_identity_webhook_receipt_attempt_id_fkey FOREIGN KEY (attempt_id) REFERENCES public.account_identity_verification(id) ON DELETE CASCADE;


--
-- Name: account_pinpass_access account_pinpass_access_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_access
    ADD CONSTRAINT account_pinpass_access_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_pinpass_access account_pinpass_access_account_id_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_access
    ADD CONSTRAINT account_pinpass_access_account_id_session_id_fkey FOREIGN KEY (account_id, session_id) REFERENCES public.sesion(usuario_id, id) ON DELETE CASCADE;


--
-- Name: account_pinpass account_pinpass_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass
    ADD CONSTRAINT account_pinpass_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_pinpass_attempt account_pinpass_attempt_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_attempt
    ADD CONSTRAINT account_pinpass_attempt_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_pinpass_attempt account_pinpass_attempt_session_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_attempt
    ADD CONSTRAINT account_pinpass_attempt_session_fk FOREIGN KEY (account_id, session_id) REFERENCES public.sesion(usuario_id, id) ON DELETE CASCADE;


--
-- Name: account_pinpass_email_challenge account_pinpass_email_challenge_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_email_challenge
    ADD CONSTRAINT account_pinpass_email_challenge_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: account_pinpass_email_challenge account_pinpass_email_challenge_account_id_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_pinpass_email_challenge
    ADD CONSTRAINT account_pinpass_email_challenge_account_id_session_id_fkey FOREIGN KEY (account_id, session_id) REFERENCES public.sesion(usuario_id, id) ON DELETE CASCADE;


--
-- Name: aceptacion_terminos aceptacion_terminos_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aceptacion_terminos
    ADD CONSTRAINT aceptacion_terminos_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: aceptacion_terminos aceptacion_terminos_version_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aceptacion_terminos
    ADD CONSTRAINT aceptacion_terminos_version_fk FOREIGN KEY (version_id) REFERENCES public.version_terminos(id) ON DELETE RESTRICT;


--
-- Name: administrador_global_bootstrap administrador_global_bootstrap_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global_bootstrap
    ADD CONSTRAINT administrador_global_bootstrap_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: administrador_global administrador_global_concedido_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global
    ADD CONSTRAINT administrador_global_concedido_por_fkey FOREIGN KEY (concedido_por) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: administrador_global administrador_global_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.administrador_global
    ADD CONSTRAINT administrador_global_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: anfitrion_cumplimiento_documento anfitrion_cumplimiento_documento_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_documento
    ADD CONSTRAINT anfitrion_cumplimiento_documento_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: anfitrion_cumplimiento_documento anfitrion_cumplimiento_documento_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_documento
    ADD CONSTRAINT anfitrion_cumplimiento_documento_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: anfitrion_cumplimiento_documento anfitrion_cumplimiento_documento_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_documento
    ADD CONSTRAINT anfitrion_cumplimiento_documento_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.anfitrion_cumplimiento_personal(usuario_id) ON DELETE RESTRICT;


--
-- Name: anfitrion_cumplimiento_personal anfitrion_cumplimiento_personal_disabled_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_personal
    ADD CONSTRAINT anfitrion_cumplimiento_personal_disabled_by_fkey FOREIGN KEY (disabled_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: anfitrion_cumplimiento_personal anfitrion_cumplimiento_personal_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anfitrion_cumplimiento_personal
    ADD CONSTRAINT anfitrion_cumplimiento_personal_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: aprobacion_administrativa aprobacion_administrativa_aprobador_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aprobacion_administrativa
    ADD CONSTRAINT aprobacion_administrativa_aprobador_id_fkey FOREIGN KEY (aprobador_id) REFERENCES public.usuario(id);


--
-- Name: aprobacion_administrativa aprobacion_administrativa_solicitante_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aprobacion_administrativa
    ADD CONSTRAINT aprobacion_administrativa_solicitante_id_fkey FOREIGN KEY (solicitante_id) REFERENCES public.usuario(id);


--
-- Name: arriendo_temprano arriendo_temprano_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano arriendo_temprano_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_confirmacion
    ADD CONSTRAINT arriendo_temprano_confirmacion_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_confirmacion arriendo_temprano_confirmacion_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_confirmacion
    ADD CONSTRAINT arriendo_temprano_confirmacion_arriendo_id_fkey FOREIGN KEY (arriendo_id) REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano arriendo_temprano_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_arriendo_id_fkey FOREIGN KEY (arriendo_id) REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_arriendo_id_pago_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_arriendo_id_pago_id_fkey FOREIGN KEY (arriendo_id, pago_id) REFERENCES public.arriendo_temprano_pago(arriendo_id, pago_id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_evento arriendo_temprano_evento_confirmacion_id_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_evento
    ADD CONSTRAINT arriendo_temprano_evento_confirmacion_id_arriendo_id_fkey FOREIGN KEY (confirmacion_id, arriendo_id) REFERENCES public.arriendo_temprano_confirmacion(id, arriendo_id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotencia_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_idempotencia
    ADD CONSTRAINT arriendo_temprano_idempotencia_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotencia_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_idempotencia
    ADD CONSTRAINT arriendo_temprano_idempotencia_arriendo_id_fkey FOREIGN KEY (arriendo_id) REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_idempotencia arriendo_temprano_idempotencia_evento_id_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_idempotencia
    ADD CONSTRAINT arriendo_temprano_idempotencia_evento_id_arriendo_id_fkey FOREIGN KEY (evento_id, arriendo_id) REFERENCES public.arriendo_temprano_evento(id, arriendo_id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano arriendo_temprano_ocupacion_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano
    ADD CONSTRAINT arriendo_temprano_ocupacion_fk FOREIGN KEY (id, espacio_id, intervalo, ocupacion_id) REFERENCES public.ocupacion(arriendo_temprano_id, espacio_id, intervalo, id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_arriendo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_arriendo_id_fkey FOREIGN KEY (arriendo_id) REFERENCES public.arriendo_temprano(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_emisor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_emisor_id_fkey FOREIGN KEY (emisor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: arriendo_temprano_pago arriendo_temprano_pago_receptor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arriendo_temprano_pago
    ADD CONSTRAINT arriendo_temprano_pago_receptor_id_fkey FOREIGN KEY (receptor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: atributo_filtro_publico atributo_filtro_publico_categoria_codigo_perfil_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.atributo_filtro_publico
    ADD CONSTRAINT atributo_filtro_publico_categoria_codigo_perfil_version_fkey FOREIGN KEY (categoria_codigo, perfil_version) REFERENCES public.categoria_perfil_atributos(categoria_codigo, version);


--
-- Name: autorizacion_arriendo_comercial autorizacion_arriendo_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial
    ADD CONSTRAINT autorizacion_arriendo_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_arriendo_comercial_evento autorizacion_arriendo_comercial_evento_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial_evento
    ADD CONSTRAINT autorizacion_arriendo_comercial_evento_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_arriendo_comercial_evento autorizacion_arriendo_comercial_evento_autorizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial_evento
    ADD CONSTRAINT autorizacion_arriendo_comercial_evento_autorizacion_id_fkey FOREIGN KEY (autorizacion_id) REFERENCES public.autorizacion_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_arriendo_comercial_evento autorizacion_arriendo_comercial_evento_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial_evento
    ADD CONSTRAINT autorizacion_arriendo_comercial_evento_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_arriendo_comercial autorizacion_arriendo_comercial_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial
    ADD CONSTRAINT autorizacion_arriendo_comercial_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_arriendo_comercial autorizacion_arriendo_comercial_revocada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_arriendo_comercial
    ADD CONSTRAINT autorizacion_arriendo_comercial_revocada_por_fkey FOREIGN KEY (revocada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: autorizacion_firma_desafio_otp autorizacion_firma_desafio_otp_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_firma_desafio_otp
    ADD CONSTRAINT autorizacion_firma_desafio_otp_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: autorizacion_firma_desafio_otp autorizacion_firma_desafio_otp_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autorizacion_firma_desafio_otp
    ADD CONSTRAINT autorizacion_firma_desafio_otp_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: aviso_local_ciclo aviso_local_ciclo_actor_reapertura_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_ciclo
    ADD CONSTRAINT aviso_local_ciclo_actor_reapertura_id_fkey FOREIGN KEY (actor_reapertura_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: aviso_local_ciclo aviso_local_ciclo_aviso_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_ciclo
    ADD CONSTRAINT aviso_local_ciclo_aviso_id_fkey FOREIGN KEY (aviso_id) REFERENCES public.aviso_local(id) ON DELETE RESTRICT;


--
-- Name: aviso_local aviso_local_destinatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local
    ADD CONSTRAINT aviso_local_destinatario_id_fkey FOREIGN KEY (destinatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: aviso_local_recuperacion_auditoria aviso_local_recuperacion_auditoria_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aviso_local_recuperacion_auditoria
    ADD CONSTRAINT aviso_local_recuperacion_auditoria_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: baja_archivo_pendiente_local baja_archivo_pendiente_local_ejecucion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.baja_archivo_pendiente_local
    ADD CONSTRAINT baja_archivo_pendiente_local_ejecucion_id_fkey FOREIGN KEY (ejecucion_id) REFERENCES public.ejecucion_baja_local(id) ON DELETE RESTRICT;


--
-- Name: bloqueo_cuenta_administrativo_local bloqueo_cuenta_administrativo_local_bloqueada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_administrativo_local
    ADD CONSTRAINT bloqueo_cuenta_administrativo_local_bloqueada_por_fkey FOREIGN KEY (bloqueada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: bloqueo_cuenta_administrativo_local bloqueo_cuenta_administrativo_local_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_administrativo_local
    ADD CONSTRAINT bloqueo_cuenta_administrativo_local_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: bloqueo_cuenta_historial_local bloqueo_cuenta_historial_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_historial_local
    ADD CONSTRAINT bloqueo_cuenta_historial_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: bloqueo_cuenta_historial_local bloqueo_cuenta_historial_local_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bloqueo_cuenta_historial_local
    ADD CONSTRAINT bloqueo_cuenta_historial_local_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: caso_soporte_reserva_comercial caso_soporte_reserva_comercial_iniciador_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caso_soporte_reserva_comercial
    ADD CONSTRAINT caso_soporte_reserva_comercial_iniciador_id_fkey FOREIGN KEY (iniciador_id) REFERENCES public.usuario(id);


--
-- Name: caso_soporte_reserva_comercial caso_soporte_reserva_comercial_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caso_soporte_reserva_comercial
    ADD CONSTRAINT caso_soporte_reserva_comercial_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id);


--
-- Name: categoria_perfil_atributos categoria_perfil_atributos_categoria_codigo_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categoria_perfil_atributos
    ADD CONSTRAINT categoria_perfil_atributos_categoria_codigo_fkey FOREIGN KEY (categoria_codigo) REFERENCES public.categoria_espacio(codigo) ON DELETE RESTRICT;


--
-- Name: checkout_cancelacion_pendiente checkout_cancelacion_pendiente_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_cancelacion_pendiente
    ADD CONSTRAINT checkout_cancelacion_pendiente_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: checkout_cancelacion_pendiente checkout_cancelacion_pendiente_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_cancelacion_pendiente
    ADD CONSTRAINT checkout_cancelacion_pendiente_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.checkout_preferencia_comercial(id) ON DELETE CASCADE;


--
-- Name: checkout_payment_notice_outbox checkout_payment_notice_outbox_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_payment_notice_outbox
    ADD CONSTRAINT checkout_payment_notice_outbox_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT;


--
-- Name: checkout_payment_notice_outbox checkout_payment_notice_outbox_recipient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_payment_notice_outbox
    ADD CONSTRAINT checkout_payment_notice_outbox_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: checkout_payment_notice_outbox checkout_payment_notice_outbox_reservation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_payment_notice_outbox
    ADD CONSTRAINT checkout_payment_notice_outbox_reservation_id_fkey FOREIGN KEY (reservation_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE;


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_host_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_host_id_fkey FOREIGN KEY (host_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: checkout_preferencia_comercial checkout_preferencia_comercial_solicitud_comercial_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_solicitud_comercial_id_fkey FOREIGN KEY (solicitud_comercial_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: comuna_publica comuna_publica_catalogo_version_region_codigo_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comuna_publica
    ADD CONSTRAINT comuna_publica_catalogo_version_region_codigo_fkey FOREIGN KEY (catalogo_version, region_codigo) REFERENCES public.region_publica(catalogo_version, codigo);


--
-- Name: contrato_documento_comercial contrato_documento_comercial_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_documento_comercial contrato_documento_comercial_propuesta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_propuesta_id_fkey FOREIGN KEY (propuesta_id) REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_documento_comercial contrato_documento_comercial_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_documento_comercial
    ADD CONSTRAINT contrato_documento_comercial_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_firma contrato_ensayo_firma_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_firma
    ADD CONSTRAINT contrato_ensayo_firma_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_firma contrato_ensayo_firma_firmante_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_firma
    ADD CONSTRAINT contrato_ensayo_firma_firmante_id_fkey FOREIGN KEY (firmante_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_historial contrato_ensayo_historial_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_historial
    ADD CONSTRAINT contrato_ensayo_historial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_historial contrato_ensayo_historial_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_historial
    ADD CONSTRAINT contrato_ensayo_historial_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_local contrato_ensayo_local_documento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_documento_id_fkey FOREIGN KEY (documento_id) REFERENCES public.documento_privado_sintetico_local(id) ON DELETE RESTRICT;


--
-- Name: contrato_ensayo_local contrato_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_ensayo_local
    ADD CONSTRAINT contrato_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_comercial contrato_firma_comercial_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_comercial
    ADD CONSTRAINT contrato_firma_comercial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_comercial contrato_firma_comercial_desafio_otp_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_comercial
    ADD CONSTRAINT contrato_firma_comercial_desafio_otp_id_fkey FOREIGN KEY (desafio_otp_id) REFERENCES public.contrato_firma_desafio_otp(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_comercial contrato_firma_comercial_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_comercial
    ADD CONSTRAINT contrato_firma_comercial_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_desafio_otp contrato_firma_desafio_otp_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_desafio_otp
    ADD CONSTRAINT contrato_firma_desafio_otp_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_desafio_otp contrato_firma_desafio_otp_propuesta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_desafio_otp
    ADD CONSTRAINT contrato_firma_desafio_otp_propuesta_id_fkey FOREIGN KEY (propuesta_id) REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_firma_desafio_otp contrato_firma_desafio_otp_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_firma_desafio_otp
    ADD CONSTRAINT contrato_firma_desafio_otp_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_plantilla_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_plantilla_version_id_fkey FOREIGN KEY (plantilla_version_id) REFERENCES public.espacio_contrato_version(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_propuesta_comercial contrato_propuesta_comercial_representante_legal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_propuesta_comercial
    ADD CONSTRAINT contrato_propuesta_comercial_representante_legal_id_fkey FOREIGN KEY (representante_legal_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_propuesta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_propuesta_id_fkey FOREIGN KEY (propuesta_id) REFERENCES public.contrato_propuesta_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_comercial_representante_legal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_comercial_representante_legal_id_fkey FOREIGN KEY (representante_legal_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_evento_comercial contrato_solicitud_evento_comercial_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_evento_comercial
    ADD CONSTRAINT contrato_solicitud_evento_comercial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_evento_comercial contrato_solicitud_evento_comercial_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_evento_comercial
    ADD CONSTRAINT contrato_solicitud_evento_comercial_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_notice_outbox contrato_solicitud_notice_outbox_destinatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_notice_outbox
    ADD CONSTRAINT contrato_solicitud_notice_outbox_destinatario_id_fkey FOREIGN KEY (destinatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_notice_outbox contrato_solicitud_notice_outbox_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_notice_outbox
    ADD CONSTRAINT contrato_solicitud_notice_outbox_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_comercial contrato_solicitud_ocupacion_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_comercial
    ADD CONSTRAINT contrato_solicitud_ocupacion_fk FOREIGN KEY (id, espacio_id, ocupacion_id) REFERENCES public.ocupacion(solicitud_comercial_id, espacio_id, id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: contrato_solicitud_quote_activa contrato_solicitud_quote_activa_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_quote_activa
    ADD CONSTRAINT contrato_solicitud_quote_activa_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: contrato_solicitud_quote_activa contrato_solicitud_quote_activa_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_solicitud_quote_activa
    ADD CONSTRAINT contrato_solicitud_quote_activa_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.contrato_solicitud_comercial(id) ON DELETE RESTRICT;


--
-- Name: cotizacion_arriendo_comercial cotizacion_arriendo_comercial_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cotizacion_arriendo_comercial cotizacion_arriendo_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: cotizacion_arriendo_comercial cotizacion_arriendo_comercial_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cotizacion_arriendo_tramo cotizacion_arriendo_tramo_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_arriendo_tramo
    ADD CONSTRAINT cotizacion_arriendo_tramo_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE;


--
-- Name: cotizacion_reserva_ensayo cotizacion_ensayo_perfil_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_ensayo_perfil_fk FOREIGN KEY (categoria_codigo, perfil_version) REFERENCES public.categoria_perfil_atributos(categoria_codigo, version) ON DELETE RESTRICT;


--
-- Name: cotizacion_modalidad_cobro cotizacion_modalidad_cobro_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_modalidad_cobro
    ADD CONSTRAINT cotizacion_modalidad_cobro_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE;


--
-- Name: cotizacion_reserva_ensayo cotizacion_reserva_ensayo_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_reserva_ensayo_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: cotizacion_reserva_ensayo cotizacion_reserva_ensayo_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_reserva_ensayo_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: cotizacion_reserva_ensayo cotizacion_reserva_ensayo_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_reserva_ensayo_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: cotizacion_reserva_ensayo cotizacion_reserva_ensayo_espacio_id_tarifa_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cotizacion_reserva_ensayo
    ADD CONSTRAINT cotizacion_reserva_ensayo_espacio_id_tarifa_version_fkey FOREIGN KEY (espacio_id, tarifa_version) REFERENCES public.tarifa_espacio(espacio_id, version) ON DELETE RESTRICT;


--
-- Name: cuenta_alerta_evento cuenta_alerta_evento_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_alerta_evento
    ADD CONSTRAINT cuenta_alerta_evento_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_alerta_preferencia cuenta_alerta_preferencia_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_alerta_preferencia
    ADD CONSTRAINT cuenta_alerta_preferencia_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_cobro_sintetica_historial_local cuenta_cobro_sintetica_historial_local_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_historial_local
    ADD CONSTRAINT cuenta_cobro_sintetica_historial_local_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.cuenta_cobro_sintetica_local(id) ON DELETE RESTRICT;


--
-- Name: cuenta_cobro_sintetica_historial_local cuenta_cobro_sintetica_historial_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_historial_local
    ADD CONSTRAINT cuenta_cobro_sintetica_historial_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: cuenta_cobro_sintetica_local cuenta_cobro_sintetica_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_cobro_sintetica_local
    ADD CONSTRAINT cuenta_cobro_sintetica_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: cuenta_passkey cuenta_passkey_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_passkey
    ADD CONSTRAINT cuenta_passkey_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_permiso_ubicacion cuenta_permiso_ubicacion_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_permiso_ubicacion
    ADD CONSTRAINT cuenta_permiso_ubicacion_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_persona_confianza cuenta_persona_confianza_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_persona_confianza
    ADD CONSTRAINT cuenta_persona_confianza_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_qr_dispositivo cuenta_qr_dispositivo_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_qr_dispositivo
    ADD CONSTRAINT cuenta_qr_dispositivo_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_qr_ticket cuenta_qr_ticket_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_qr_ticket
    ADD CONSTRAINT cuenta_qr_ticket_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_totp_backup_code cuenta_totp_backup_code_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_totp_backup_code
    ADD CONSTRAINT cuenta_totp_backup_code_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_totp cuenta_totp_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_totp
    ADD CONSTRAINT cuenta_totp_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_verificacion_telefono cuenta_verificacion_telefono_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_verificacion_telefono
    ADD CONSTRAINT cuenta_verificacion_telefono_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cuenta_webauthn_challenge cuenta_webauthn_challenge_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_webauthn_challenge
    ADD CONSTRAINT cuenta_webauthn_challenge_usuario_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: desafio_mfa_administrativo desafio_mfa_administrativo_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.desafio_mfa_administrativo
    ADD CONSTRAINT desafio_mfa_administrativo_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: desafio_mfa_administrativo desafio_mfa_administrativo_sesion_administrativa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.desafio_mfa_administrativo
    ADD CONSTRAINT desafio_mfa_administrativo_sesion_administrativa_id_fkey FOREIGN KEY (sesion_administrativa_id) REFERENCES public.sesion_administrativa(id) ON DELETE CASCADE;


--
-- Name: disputa_ensayo_historial disputa_ensayo_historial_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_historial
    ADD CONSTRAINT disputa_ensayo_historial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_historial disputa_ensayo_historial_disputa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_historial
    ADD CONSTRAINT disputa_ensayo_historial_disputa_id_fkey FOREIGN KEY (disputa_id) REFERENCES public.disputa_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_local disputa_ensayo_local_abierta_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_abierta_por_fkey FOREIGN KEY (abierta_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_local disputa_ensayo_local_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_local disputa_ensayo_local_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_local disputa_ensayo_local_cerrada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_cerrada_por_fkey FOREIGN KEY (cerrada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: disputa_ensayo_local disputa_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.disputa_ensayo_local
    ADD CONSTRAINT disputa_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: document_verification_case document_verification_case_identity_declaration_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_identity_declaration_fk FOREIGN KEY (identity_declaration_revision_id, subject_id) REFERENCES public.identity_declaration_revision(id, account_id) ON DELETE RESTRICT;


--
-- Name: document_verification_case document_verification_case_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: document_verification_case document_verification_case_space_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_space_fk FOREIGN KEY (space_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: document_verification_case document_verification_case_subject_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_case
    ADD CONSTRAINT document_verification_case_subject_id_fkey FOREIGN KEY (subject_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: document_verification_file document_verification_file_case_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_file
    ADD CONSTRAINT document_verification_file_case_id_fkey FOREIGN KEY (case_id) REFERENCES public.document_verification_case(id) ON DELETE RESTRICT;


--
-- Name: document_verification_job document_verification_job_file_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_job
    ADD CONSTRAINT document_verification_job_file_id_fkey FOREIGN KEY (file_id) REFERENCES public.document_verification_file(id) ON DELETE CASCADE;


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_granted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_reviewer_grant
    ADD CONSTRAINT document_verification_reviewer_grant_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_reviewer_grant
    ADD CONSTRAINT document_verification_reviewer_grant_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: document_verification_reviewer_grant document_verification_reviewer_grant_revoked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_verification_reviewer_grant
    ADD CONSTRAINT document_verification_reviewer_grant_revoked_by_fkey FOREIGN KEY (revoked_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: documento_privado_sintetico_local documento_privado_sintetico_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_privado_sintetico_local
    ADD CONSTRAINT documento_privado_sintetico_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: ejecucion_baja_local ejecucion_baja_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ejecucion_baja_local
    ADD CONSTRAINT ejecucion_baja_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: ejecucion_baja_local ejecucion_baja_local_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ejecucion_baja_local
    ADD CONSTRAINT ejecucion_baja_local_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE RESTRICT;


--
-- Name: ejecucion_baja_local ejecucion_baja_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ejecucion_baja_local
    ADD CONSTRAINT ejecucion_baja_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: elegibilidad_verificacion_local elegibilidad_verificacion_local_revocada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.elegibilidad_verificacion_local
    ADD CONSTRAINT elegibilidad_verificacion_local_revocada_por_fkey FOREIGN KEY (revocada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: elegibilidad_verificacion_local elegibilidad_verificacion_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.elegibilidad_verificacion_local
    ADD CONSTRAINT elegibilidad_verificacion_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: elegibilidad_verificacion_local elegibilidad_verificacion_local_verificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.elegibilidad_verificacion_local
    ADD CONSTRAINT elegibilidad_verificacion_local_verificacion_id_fkey FOREIGN KEY (verificacion_id) REFERENCES public.verificacion(id) ON DELETE RESTRICT;


--
-- Name: espacio espacio_b2b_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_b2b_owner_fk FOREIGN KEY (organizacion_id, propietario_id) REFERENCES public.organizacion(id, usuario_id) ON DELETE RESTRICT;


--
-- Name: espacio_calendario_override espacio_calendario_override_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_calendario_override
    ADD CONSTRAINT espacio_calendario_override_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: espacio_caracteristicas espacio_caracteristicas_categoria_codigo_perfil_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_caracteristicas
    ADD CONSTRAINT espacio_caracteristicas_categoria_codigo_perfil_version_fkey FOREIGN KEY (categoria_codigo, perfil_version) REFERENCES public.categoria_perfil_atributos(categoria_codigo, version) ON DELETE RESTRICT;


--
-- Name: espacio_caracteristicas espacio_caracteristicas_category_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_caracteristicas
    ADD CONSTRAINT espacio_caracteristicas_category_fk FOREIGN KEY (espacio_id, categoria_codigo) REFERENCES public.espacio(id, categoria_codigo) ON DELETE CASCADE;


--
-- Name: espacio_caracteristicas espacio_caracteristicas_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_caracteristicas
    ADD CONSTRAINT espacio_caracteristicas_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: espacio espacio_categoria_codigo_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_categoria_codigo_fkey FOREIGN KEY (categoria_codigo) REFERENCES public.categoria_espacio(codigo) ON DELETE RESTRICT;


--
-- Name: espacio_contrato_borrador espacio_contrato_borrador_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_borrador
    ADD CONSTRAINT espacio_contrato_borrador_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_contrato_version espacio_contrato_version_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_version
    ADD CONSTRAINT espacio_contrato_version_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_contrato_version espacio_contrato_version_publicada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_contrato_version
    ADD CONSTRAINT espacio_contrato_version_publicada_por_fkey FOREIGN KEY (publicada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_historial_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_eliminacion_historial_local
    ADD CONSTRAINT espacio_eliminacion_historial_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: espacio_eliminacion_historial_local espacio_eliminacion_historial_local_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_eliminacion_historial_local
    ADD CONSTRAINT espacio_eliminacion_historial_local_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_galeria_archivo_candidato_local espacio_galeria_candidato_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_archivo_candidato_local
    ADD CONSTRAINT espacio_galeria_candidato_owner_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: espacio_galeria_sintetica_local espacio_galeria_sintetica_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_galeria_sintetica_local
    ADD CONSTRAINT espacio_galeria_sintetica_owner_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: espacio_horario_semanal espacio_horario_semanal_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_horario_semanal
    ADD CONSTRAINT espacio_horario_semanal_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_horario_semanal_tramo espacio_horario_semanal_tramo_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_horario_semanal_tramo
    ADD CONSTRAINT espacio_horario_semanal_tramo_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio_horario_semanal(espacio_id) ON DELETE CASCADE;


--
-- Name: espacio espacio_portada_galeria_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_portada_galeria_fk FOREIGN KEY (id, propietario_id, portada_foto_id) REFERENCES public.espacio_galeria_sintetica_local(espacio_id, propietario_id, id) ON DELETE RESTRICT;


--
-- Name: espacio espacio_portada_media_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_portada_media_fk FOREIGN KEY (id, propietario_id, portada_media_id) REFERENCES public.space_media(espacio_id, propietario_id, id) ON DELETE RESTRICT;


--
-- Name: espacio espacio_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: espacio espacio_public_commune_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_public_commune_fk FOREIGN KEY (ubicacion_catalogo_version, region_codigo, comuna_codigo) REFERENCES public.comuna_publica(catalogo_version, region_codigo, codigo);


--
-- Name: espacio espacio_public_region_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio
    ADD CONSTRAINT espacio_public_region_fk FOREIGN KEY (ubicacion_catalogo_version, region_codigo) REFERENCES public.region_publica(catalogo_version, codigo);


--
-- Name: espacio_publicacion_historial_local espacio_publicacion_historial_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_publicacion_historial_local
    ADD CONSTRAINT espacio_publicacion_historial_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: espacio_publicacion_historial_local espacio_publicacion_historial_local_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_publicacion_historial_local
    ADD CONSTRAINT espacio_publicacion_historial_local_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_regla_disponibilidad espacio_regla_disponibilidad_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_regla_disponibilidad
    ADD CONSTRAINT espacio_regla_disponibilidad_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: espacio_regla_mensual espacio_regla_mensual_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_regla_mensual
    ADD CONSTRAINT espacio_regla_mensual_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE CASCADE;


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_actor_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_solicitud_documental
    ADD CONSTRAINT espacio_solicitud_documental_actor_usuario_id_fkey FOREIGN KEY (actor_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_solicitud_documental
    ADD CONSTRAINT espacio_solicitud_documental_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_solicitud_documental
    ADD CONSTRAINT espacio_solicitud_documental_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: espacio_solicitud_documental espacio_solicitud_documental_titular_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.espacio_solicitud_documental
    ADD CONSTRAINT espacio_solicitud_documental_titular_usuario_id_fkey FOREIGN KEY (titular_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: evento_auditoria_local evento_auditoria_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_auditoria_local
    ADD CONSTRAINT evento_auditoria_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: evento_auditoria_local evento_auditoria_local_cuenta_objetivo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_auditoria_local
    ADD CONSTRAINT evento_auditoria_local_cuenta_objetivo_id_fkey FOREIGN KEY (cuenta_objetivo_id) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: evento_caso_soporte_comercial evento_caso_soporte_comercial_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_caso_soporte_comercial
    ADD CONSTRAINT evento_caso_soporte_comercial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id);


--
-- Name: evento_caso_soporte_comercial evento_caso_soporte_comercial_caso_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evento_caso_soporte_comercial
    ADD CONSTRAINT evento_caso_soporte_comercial_caso_id_fkey FOREIGN KEY (caso_id) REFERENCES public.caso_soporte_reserva_comercial(id);


--
-- Name: evidencia_caso_soporte_comercial evidencia_caso_soporte_comercial_autor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidencia_caso_soporte_comercial
    ADD CONSTRAINT evidencia_caso_soporte_comercial_autor_id_fkey FOREIGN KEY (autor_id) REFERENCES public.usuario(id);


--
-- Name: evidencia_caso_soporte_comercial evidencia_caso_soporte_comercial_caso_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evidencia_caso_soporte_comercial
    ADD CONSTRAINT evidencia_caso_soporte_comercial_caso_id_fkey FOREIGN KEY (caso_id) REFERENCES public.caso_soporte_reserva_comercial(id);


--
-- Name: foto_perfil_sintetica_local foto_perfil_sintetica_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.foto_perfil_sintetica_local
    ADD CONSTRAINT foto_perfil_sintetica_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: gallery_item_order gallery_order_real_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_real_fk FOREIGN KEY (espacio_id, propietario_id, space_media_id) REFERENCES public.space_media(espacio_id, propietario_id, id) ON DELETE RESTRICT;


--
-- Name: gallery_item_order gallery_order_space_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_space_owner_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: gallery_item_order gallery_order_synthetic_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gallery_item_order
    ADD CONSTRAINT gallery_order_synthetic_fk FOREIGN KEY (espacio_id, propietario_id, synthetic_photo_id) REFERENCES public.espacio_galeria_sintetica_local(espacio_id, propietario_id, id) ON DELETE RESTRICT;


--
-- Name: historial_clave_local historial_clave_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.historial_clave_local
    ADD CONSTRAINT historial_clave_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: host_compliance_event host_compliance_event_actor_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_compliance_event
    ADD CONSTRAINT host_compliance_event_actor_usuario_id_fkey FOREIGN KEY (actor_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: host_compliance_event host_compliance_event_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_compliance_event
    ADD CONSTRAINT host_compliance_event_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: host_compliance_event host_compliance_event_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_compliance_event
    ADD CONSTRAINT host_compliance_event_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: host_compliance_event host_compliance_event_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_compliance_event
    ADD CONSTRAINT host_compliance_event_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: identity_declaration_revision identity_declaration_revision_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_declaration_revision
    ADD CONSTRAINT identity_declaration_revision_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id);


--
-- Name: identity_provider_attempt identity_provider_attempt_declaration_id_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_attempt
    ADD CONSTRAINT identity_provider_attempt_declaration_id_account_id_fkey FOREIGN KEY (declaration_id, account_id) REFERENCES public.identity_declaration_revision(id, account_id);


--
-- Name: identity_provider_event identity_provider_event_attempt_id_provider_environment_ap_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_provider_event
    ADD CONSTRAINT identity_provider_event_attempt_id_provider_environment_ap_fkey FOREIGN KEY (attempt_id, provider, environment, application_id) REFERENCES public.identity_provider_attempt(id, provider, environment, application_id);


--
-- Name: identity_rut_claim identity_rut_claim_revision_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_rut_claim
    ADD CONSTRAINT identity_rut_claim_revision_fk FOREIGN KEY (declaration_id, account_id) REFERENCES public.identity_declaration_revision(id, account_id) ON DELETE RESTRICT;


--
-- Name: identity_rut_claim identity_rut_claim_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_rut_claim
    ADD CONSTRAINT identity_rut_claim_user_fk FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: identity_verification_attempts identity_verification_attempts_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_verification_attempts
    ADD CONSTRAINT identity_verification_attempts_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: kyb_compliance_notice_outbox kyb_compliance_notice_outbox_recipient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_compliance_notice_outbox
    ADD CONSTRAINT kyb_compliance_notice_outbox_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: kyb_rut_claim kyb_rut_claim_verification_id_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_rut_claim
    ADD CONSTRAINT kyb_rut_claim_verification_id_organization_id_fkey FOREIGN KEY (verification_id, organization_id) REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT;


--
-- Name: kyb_verification_document kyb_verification_document_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: kyb_verification_document kyb_verification_document_verification_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_verification_id_fkey FOREIGN KEY (verification_id) REFERENCES public.kyb_verifications(id) ON DELETE RESTRICT;


--
-- Name: kyb_verification_event kyb_verification_event_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_event
    ADD CONSTRAINT kyb_verification_event_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: kyb_verification_event kyb_verification_event_verification_id_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verification_event
    ADD CONSTRAINT kyb_verification_event_verification_id_organization_id_fkey FOREIGN KEY (verification_id, organization_id) REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT;


--
-- Name: kyb_verifications kyb_verifications_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: kyb_verifications kyb_verifications_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: kyb_verifications kyb_verifications_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kyb_verifications
    ADD CONSTRAINT kyb_verifications_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: m02_operacion_idempotente_local m02_operacion_idempotente_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.m02_operacion_idempotente_local
    ADD CONSTRAINT m02_operacion_idempotente_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: media_asset media_asset_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: media_asset media_asset_replacement_target_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_replacement_target_fk FOREIGN KEY (espacio_id, propietario_id, replacement_target_space_media_id) REFERENCES public.space_media(espacio_id, propietario_id, id) ON DELETE RESTRICT;


--
-- Name: media_asset media_asset_space_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_asset
    ADD CONSTRAINT media_asset_space_owner_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: media_variant_intent media_variant_intent_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant_intent
    ADD CONSTRAINT media_variant_intent_media_id_fkey FOREIGN KEY (media_id) REFERENCES public.media_asset(id) ON DELETE RESTRICT;


--
-- Name: media_variant media_variant_media_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_variant
    ADD CONSTRAINT media_variant_media_id_fkey FOREIGN KEY (media_id) REFERENCES public.media_asset(id) ON DELETE RESTRICT;


--
-- Name: mensaje_reserva_comercial mensaje_reserva_comercial_autor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_comercial
    ADD CONSTRAINT mensaje_reserva_comercial_autor_fk FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: mensaje_reserva_comercial mensaje_reserva_comercial_reserva_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_comercial
    ADD CONSTRAINT mensaje_reserva_comercial_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: mensaje_reserva_ensayo mensaje_reserva_ensayo_autor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_ensayo
    ADD CONSTRAINT mensaje_reserva_ensayo_autor_id_fkey FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: mensaje_reserva_ensayo mensaje_reserva_ensayo_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mensaje_reserva_ensayo
    ADD CONSTRAINT mensaje_reserva_ensayo_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: mercadopago_oauth_state mercadopago_oauth_state_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_oauth_state
    ADD CONSTRAINT mercadopago_oauth_state_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: mercadopago_seller_connection mercadopago_seller_connection_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mercadopago_seller_connection
    ADD CONSTRAINT mercadopago_seller_connection_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: notificacion_usuario notificacion_usuario_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notificacion_usuario
    ADD CONSTRAINT notificacion_usuario_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: ocupacion ocupacion_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: ocupacion ocupacion_reserva_ensayo_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_reserva_ensayo_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: ocupacion ocupacion_solicitud_comercial_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_solicitud_comercial_fk FOREIGN KEY (solicitud_comercial_id, espacio_id, id) REFERENCES public.contrato_solicitud_comercial(id, espacio_id, ocupacion_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: ocupacion ocupacion_temprana_agregado_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ocupacion
    ADD CONSTRAINT ocupacion_temprana_agregado_fk FOREIGN KEY (arriendo_temprano_id, espacio_id, intervalo, id) REFERENCES public.arriendo_temprano(id, espacio_id, intervalo, ocupacion_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: operacion_arriendo_archivo_candidato_local operacion_arriendo_archivo_candidato_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_archivo_candidato_local
    ADD CONSTRAINT operacion_arriendo_archivo_candidato_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_archivo_candidato_local operacion_arriendo_archivo_candidato_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_archivo_candidato_local
    ADD CONSTRAINT operacion_arriendo_archivo_candidato_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_ensayo_local operacion_arriendo_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_ensayo_local
    ADD CONSTRAINT operacion_arriendo_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_ensayo_local operacion_arriendo_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_ensayo_local
    ADD CONSTRAINT operacion_arriendo_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_evidencia_ensayo_local operacion_arriendo_evidencia_ensayo_local_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_evidencia_ensayo_local
    ADD CONSTRAINT operacion_arriendo_evidencia_ensayo_local_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.operacion_arriendo_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_historial_ensayo_local operacion_arriendo_historial_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_historial_ensayo_local
    ADD CONSTRAINT operacion_arriendo_historial_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_historial_ensayo_local operacion_arriendo_historial_ensayo_local_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_historial_ensayo_local
    ADD CONSTRAINT operacion_arriendo_historial_ensayo_local_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.operacion_arriendo_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: operacion_arriendo_historial_ensayo_local operacion_arriendo_historial_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operacion_arriendo_historial_ensayo_local
    ADD CONSTRAINT operacion_arriendo_historial_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: organizacion organizacion_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion
    ADD CONSTRAINT organizacion_created_by_fk FOREIGN KEY (created_by_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion organizacion_disabled_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion
    ADD CONSTRAINT organizacion_disabled_by_fkey FOREIGN KEY (disabled_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_invitacion organizacion_invitacion_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_invitacion organizacion_invitacion_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: organizacion_invitacion organizacion_invitacion_resolved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_resolved_by_fkey FOREIGN KEY (resolved_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_invitacion organizacion_invitacion_target_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_invitacion
    ADD CONSTRAINT organizacion_invitacion_target_usuario_id_fkey FOREIGN KEY (target_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_miembro organizacion_miembro_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_miembro
    ADD CONSTRAINT organizacion_miembro_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_miembro organizacion_miembro_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_miembro
    ADD CONSTRAINT organizacion_miembro_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: organizacion_miembro organizacion_miembro_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_miembro
    ADD CONSTRAINT organizacion_miembro_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: organizacion_rut_fingerprint organizacion_rut_fingerprint_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion_rut_fingerprint
    ADD CONSTRAINT organizacion_rut_fingerprint_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: organizacion organizacion_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizacion
    ADD CONSTRAINT organizacion_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: outbox_evento_ciclo_local outbox_evento_ciclo_local_actor_reapertura_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_ciclo_local
    ADD CONSTRAINT outbox_evento_ciclo_local_actor_reapertura_id_fkey FOREIGN KEY (actor_reapertura_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: outbox_evento_ciclo_local outbox_evento_ciclo_local_evento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_evento_ciclo_local
    ADD CONSTRAINT outbox_evento_ciclo_local_evento_id_fkey FOREIGN KEY (evento_id) REFERENCES public.outbox_evento_local(id) ON DELETE CASCADE;


--
-- Name: perfil_usuario perfil_usuario_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.perfil_usuario
    ADD CONSTRAINT perfil_usuario_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: privacidad_bloqueo_tratamiento privacidad_bloqueo_tratamiento_aplicado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_bloqueo_tratamiento
    ADD CONSTRAINT privacidad_bloqueo_tratamiento_aplicado_por_fkey FOREIGN KEY (aplicado_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_bloqueo_tratamiento privacidad_bloqueo_tratamiento_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_bloqueo_tratamiento
    ADD CONSTRAINT privacidad_bloqueo_tratamiento_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: privacidad_bloqueo_tratamiento privacidad_bloqueo_tratamiento_levantado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_bloqueo_tratamiento
    ADD CONSTRAINT privacidad_bloqueo_tratamiento_levantado_por_fkey FOREIGN KEY (levantado_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_bloqueo_tratamiento privacidad_bloqueo_tratamiento_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_bloqueo_tratamiento
    ADD CONSTRAINT privacidad_bloqueo_tratamiento_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE CASCADE;


--
-- Name: privacidad_ejecucion_derecho privacidad_ejecucion_derecho_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_ejecucion_derecho
    ADD CONSTRAINT privacidad_ejecucion_derecho_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: privacidad_ejecucion_derecho privacidad_ejecucion_derecho_ejecutado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_ejecucion_derecho
    ADD CONSTRAINT privacidad_ejecucion_derecho_ejecutado_por_fkey FOREIGN KEY (ejecutado_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_ejecucion_derecho privacidad_ejecucion_derecho_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_ejecucion_derecho
    ADD CONSTRAINT privacidad_ejecucion_derecho_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE CASCADE;


--
-- Name: privacidad_incidente_datos privacidad_incidente_datos_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_datos
    ADD CONSTRAINT privacidad_incidente_datos_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_incidente_datos privacidad_incidente_datos_lider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_datos
    ADD CONSTRAINT privacidad_incidente_datos_lider_id_fkey FOREIGN KEY (lider_id) REFERENCES public.usuario(id);


--
-- Name: privacidad_incidente_datos privacidad_incidente_datos_suplente_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_datos
    ADD CONSTRAINT privacidad_incidente_datos_suplente_id_fkey FOREIGN KEY (suplente_id) REFERENCES public.usuario(id);


--
-- Name: privacidad_incidente_evento privacidad_incidente_evento_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_evento
    ADD CONSTRAINT privacidad_incidente_evento_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id);


--
-- Name: privacidad_incidente_evento privacidad_incidente_evento_incidente_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_incidente_evento
    ADD CONSTRAINT privacidad_incidente_evento_incidente_id_fkey FOREIGN KEY (incidente_id) REFERENCES public.privacidad_incidente_datos(id) ON DELETE CASCADE;


--
-- Name: privacidad_notificacion_outbox privacidad_notificacion_outbox_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_notificacion_outbox
    ADD CONSTRAINT privacidad_notificacion_outbox_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE CASCADE;


--
-- Name: privacidad_preferencia_finalidad privacidad_preferencia_finalidad_actualizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_preferencia_finalidad
    ADD CONSTRAINT privacidad_preferencia_finalidad_actualizado_por_fkey FOREIGN KEY (actualizado_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_preferencia_finalidad privacidad_preferencia_finalidad_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_preferencia_finalidad
    ADD CONSTRAINT privacidad_preferencia_finalidad_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: privacidad_preferencia_finalidad privacidad_preferencia_finalidad_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_preferencia_finalidad
    ADD CONSTRAINT privacidad_preferencia_finalidad_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE SET NULL;


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_aplicada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_aplicada_por_fkey FOREIGN KEY (aplicada_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_incidente_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_incidente_id_fkey FOREIGN KEY (incidente_id) REFERENCES public.privacidad_incidente_datos(id) ON DELETE SET NULL;


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_liberada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_liberada_por_fkey FOREIGN KEY (liberada_por) REFERENCES public.usuario(id);


--
-- Name: privacidad_retencion_legal privacidad_retencion_legal_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacidad_retencion_legal
    ADD CONSTRAINT privacidad_retencion_legal_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE SET NULL;


--
-- Name: privacy_response_dispatch_receipt privacy_response_dispatch_receipt_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_response_dispatch_receipt
    ADD CONSTRAINT privacy_response_dispatch_receipt_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: privacy_response_dispatch_receipt privacy_response_dispatch_receipt_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_response_dispatch_receipt
    ADD CONSTRAINT privacy_response_dispatch_receipt_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.solicitud_titular(id) ON DELETE CASCADE;


--
-- Name: profile_media profile_media_asset_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_media
    ADD CONSTRAINT profile_media_asset_owner_fk FOREIGN KEY (media_id, propietario_id) REFERENCES public.media_asset(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: profile_media profile_media_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_media
    ADD CONSTRAINT profile_media_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: profile_pinpass_reauth profile_pinpass_reauth_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.profile_pinpass_reauth
    ADD CONSTRAINT profile_pinpass_reauth_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: reaplicacion_baja_local reaplicacion_baja_local_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reaplicacion_baja_local
    ADD CONSTRAINT reaplicacion_baja_local_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_descargo_ensayo_local reclamo_dano_descargo_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_descargo_ensayo_local
    ADD CONSTRAINT reclamo_dano_descargo_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_descargo_ensayo_local reclamo_dano_descargo_ensayo_local_reclamo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_descargo_ensayo_local
    ADD CONSTRAINT reclamo_dano_descargo_ensayo_local_reclamo_id_fkey FOREIGN KEY (reclamo_id) REFERENCES public.reclamo_dano_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_checkout_evidencia_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_checkout_evidencia_id_fkey FOREIGN KEY (checkout_evidencia_id) REFERENCES public.operacion_arriendo_evidencia_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_checkout_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_checkout_operacion_id_fkey FOREIGN KEY (checkout_operacion_id) REFERENCES public.operacion_arriendo_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_ensayo_local reclamo_dano_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_ensayo_local
    ADD CONSTRAINT reclamo_dano_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_historial_ensayo_local reclamo_dano_historial_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_historial_ensayo_local
    ADD CONSTRAINT reclamo_dano_historial_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_historial_ensayo_local reclamo_dano_historial_ensayo_local_reclamo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_historial_ensayo_local
    ADD CONSTRAINT reclamo_dano_historial_ensayo_local_reclamo_id_fkey FOREIGN KEY (reclamo_id) REFERENCES public.reclamo_dano_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_resolucion_ensayo_local reclamo_dano_resolucion_ensayo_local_administrador_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_resolucion_ensayo_local
    ADD CONSTRAINT reclamo_dano_resolucion_ensayo_local_administrador_id_fkey FOREIGN KEY (administrador_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reclamo_dano_resolucion_ensayo_local reclamo_dano_resolucion_ensayo_local_reclamo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reclamo_dano_resolucion_ensayo_local
    ADD CONSTRAINT reclamo_dano_resolucion_ensayo_local_reclamo_id_fkey FOREIGN KEY (reclamo_id) REFERENCES public.reclamo_dano_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: region_publica region_publica_catalogo_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.region_publica
    ADD CONSTRAINT region_publica_catalogo_version_fkey FOREIGN KEY (catalogo_version) REFERENCES public.catalogo_ubicacion_publica(version);


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_resena_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_resena_id_fkey FOREIGN KEY (resena_id) REFERENCES public.resena_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reporte_resena_ensayo_local reporte_resena_ensayo_local_resuelta_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_ensayo_local
    ADD CONSTRAINT reporte_resena_ensayo_local_resuelta_por_fkey FOREIGN KEY (resuelta_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reporte_resena_historial_local reporte_resena_historial_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_historial_local
    ADD CONSTRAINT reporte_resena_historial_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reporte_resena_historial_local reporte_resena_historial_local_reporte_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reporte_resena_historial_local
    ADD CONSTRAINT reporte_resena_historial_local_reporte_id_fkey FOREIGN KEY (reporte_id) REFERENCES public.reporte_resena_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: resena_ensayo_local resena_ensayo_local_autor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_autor_id_fkey FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: resena_ensayo_local resena_ensayo_local_destinatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_destinatario_id_fkey FOREIGN KEY (destinatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: resena_ensayo_local resena_ensayo_local_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: resena_ensayo_local resena_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_ensayo_local
    ADD CONSTRAINT resena_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: resena_reserva_comercial resena_reserva_comercial_autor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_autor_fk FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: resena_reserva_comercial resena_reserva_comercial_destinatario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_destinatario_fk FOREIGN KEY (destinatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: resena_reserva_comercial resena_reserva_comercial_espacio_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_espacio_fk FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: resena_reserva_comercial resena_reserva_comercial_reserva_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resena_reserva_comercial
    ADD CONSTRAINT resena_reserva_comercial_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_comercial reserva_arriendo_comercial_operacion_checkout_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_comercial
    ADD CONSTRAINT reserva_arriendo_comercial_operacion_checkout_id_fkey FOREIGN KEY (operacion_checkout_id) REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_arriendo_tramo_estado reserva_arriendo_tramo_estado_actualizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_tramo_estado
    ADD CONSTRAINT reserva_arriendo_tramo_estado_actualizado_por_fkey FOREIGN KEY (actualizado_por) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: reserva_arriendo_tramo_estado reserva_arriendo_tramo_estado_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_tramo_estado
    ADD CONSTRAINT reserva_arriendo_tramo_estado_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE CASCADE;


--
-- Name: reserva_arriendo_tramo_estado reserva_arriendo_tramo_estado_tramo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_arriendo_tramo_estado
    ADD CONSTRAINT reserva_arriendo_tramo_estado_tramo_id_fkey FOREIGN KEY (tramo_id) REFERENCES public.cotizacion_arriendo_tramo(id) ON DELETE CASCADE;


--
-- Name: reserva_cancelacion_ensayo reserva_cancelacion_ensayo_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cancelacion_ensayo
    ADD CONSTRAINT reserva_cancelacion_ensayo_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_cancelacion_ensayo reserva_cancelacion_ensayo_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cancelacion_ensayo
    ADD CONSTRAINT reserva_cancelacion_ensayo_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_comercial_cancelacion reserva_comercial_cancelacion_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_cancelacion
    ADD CONSTRAINT reserva_comercial_cancelacion_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_comercial_cancelacion reserva_comercial_cancelacion_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_cancelacion
    ADD CONSTRAINT reserva_comercial_cancelacion_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_comercial_mensaje_lectura reserva_comercial_mensaje_lectura_reserva_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_mensaje_lectura
    ADD CONSTRAINT reserva_comercial_mensaje_lectura_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_comercial_mensaje_lectura reserva_comercial_mensaje_lectura_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comercial_mensaje_lectura
    ADD CONSTRAINT reserva_comercial_mensaje_lectura_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_comprobante_ensayo_local reserva_comprobante_ensayo_local_liquidacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_comprobante_ensayo_local
    ADD CONSTRAINT reserva_comprobante_ensayo_local_liquidacion_id_fkey FOREIGN KEY (liquidacion_id) REFERENCES public.reserva_liquidacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_cuota_mensual reserva_cuota_mensual_reserva_arriendo_comercial_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_cuota_mensual
    ADD CONSTRAINT reserva_cuota_mensual_reserva_arriendo_comercial_id_fkey FOREIGN KEY (reserva_arriendo_comercial_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE CASCADE;


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_administrador_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_administrador_id_fkey FOREIGN KEY (administrador_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_evidencia_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_evidencia_id_fkey FOREIGN KEY (evidencia_id) REFERENCES public.operacion_arriendo_evidencia_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_garantia_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_garantia_id_fkey FOREIGN KEY (garantia_id) REFERENCES public.reserva_garantia_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_reclamo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_reclamo_id_fkey FOREIGN KEY (reclamo_id) REFERENCES public.reclamo_dano_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_decision_financiera_ensayo_local reserva_decision_financiera_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_decision_financiera_ensayo_local
    ADD CONSTRAINT reserva_decision_financiera_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_devolucion_ensayo reserva_devolucion_ensayo_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_ensayo
    ADD CONSTRAINT reserva_devolucion_ensayo_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_devolucion_intento_ensayo reserva_devolucion_intento_ensayo_devolucion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_devolucion_intento_ensayo
    ADD CONSTRAINT reserva_devolucion_intento_ensayo_devolucion_id_fkey FOREIGN KEY (devolucion_id) REFERENCES public.reserva_devolucion_ensayo(id) ON DELETE RESTRICT;


--
-- Name: reserva_documento_tributario_comercial reserva_documento_tributario_comercial_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_documento_tributario_comercial
    ADD CONSTRAINT reserva_documento_tributario_comercial_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_documento_tributario_comercial reserva_documento_tributario_comercial_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_documento_tributario_comercial
    ADD CONSTRAINT reserva_documento_tributario_comercial_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local reserva_ensayo_local_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local reserva_ensayo_local_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local reserva_ensayo_local_cotizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_cotizacion_id_fkey FOREIGN KEY (cotizacion_id) REFERENCES public.cotizacion_reserva_ensayo(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local reserva_ensayo_local_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_local_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local_fixture reserva_ensayo_local_fixture_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_fixture
    ADD CONSTRAINT reserva_ensayo_local_fixture_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local_fixture reserva_ensayo_local_fixture_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_fixture
    ADD CONSTRAINT reserva_ensayo_local_fixture_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local_fixture reserva_ensayo_local_fixture_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_fixture
    ADD CONSTRAINT reserva_ensayo_local_fixture_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_local_ubicacion_sintetica reserva_ensayo_local_ubicacion_sintetica_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local_ubicacion_sintetica
    ADD CONSTRAINT reserva_ensayo_local_ubicacion_sintetica_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.reserva_ensayo_local_fixture(espacio_id) ON DELETE CASCADE;


--
-- Name: reserva_ensayo_local reserva_ensayo_ocupacion_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_local
    ADD CONSTRAINT reserva_ensayo_ocupacion_fk FOREIGN KEY (ocupacion_id) REFERENCES public.ocupacion(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: reserva_ensayo_transicion reserva_ensayo_transicion_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_transicion
    ADD CONSTRAINT reserva_ensayo_transicion_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_ensayo_transicion reserva_ensayo_transicion_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_ensayo_transicion
    ADD CONSTRAINT reserva_ensayo_transicion_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_finanzas_historial_ensayo_local reserva_finanzas_historial_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_finanzas_historial_ensayo_local
    ADD CONSTRAINT reserva_finanzas_historial_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_finanzas_historial_ensayo_local reserva_finanzas_historial_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_finanzas_historial_ensayo_local
    ADD CONSTRAINT reserva_finanzas_historial_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_garantia_ensayo_local reserva_garantia_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_ensayo_local
    ADD CONSTRAINT reserva_garantia_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_garantia_evento_aplicacion_ensayo_local reserva_garantia_evento_aplicacion_ensayo_local_evento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_aplicacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_aplicacion_ensayo_local_evento_id_fkey FOREIGN KEY (evento_id) REFERENCES public.reserva_garantia_evento_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_garantia_evento_ensayo_local reserva_garantia_evento_ensayo_local_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_evento_ensayo_local
    ADD CONSTRAINT reserva_garantia_evento_ensayo_local_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.reserva_garantia_operacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_garantia_operacion_ensayo_local reserva_garantia_operacion_ensayo_local_garantia_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_operacion_ensayo_local
    ADD CONSTRAINT reserva_garantia_operacion_ensayo_local_garantia_id_fkey FOREIGN KEY (garantia_id) REFERENCES public.reserva_garantia_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_garantia_resultado_fake_ensayo_local reserva_garantia_resultado_fake_ensayo_local_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_garantia_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_garantia_resultado_fake_ensayo_local_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.reserva_garantia_operacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_ensayo_local reserva_liquidacion_ensayo_local_anfitrion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_ensayo_local_anfitrion_id_fkey FOREIGN KEY (anfitrion_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_ensayo_local reserva_liquidacion_ensayo_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_ensayo_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_historial_ensayo_local reserva_liquidacion_historial_ensayo_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_historial_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_historial_ensayo_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_historial_ensayo_local reserva_liquidacion_historial_ensayo_local_liquidacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_historial_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_historial_ensayo_local_liquidacion_id_fkey FOREIGN KEY (liquidacion_id) REFERENCES public.reserva_liquidacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_operacion_ensayo_local reserva_liquidacion_operacion_ensayo_local_liquidacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_operacion_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_operacion_ensayo_local_liquidacion_id_fkey FOREIGN KEY (liquidacion_id) REFERENCES public.reserva_liquidacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_liquidacion_resultado_fake_ensayo_local reserva_liquidacion_resultado_fake_ensayo_loc_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_liquidacion_resultado_fake_ensayo_local
    ADD CONSTRAINT reserva_liquidacion_resultado_fake_ensayo_loc_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.reserva_liquidacion_operacion_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_mensaje_lectura reserva_mensaje_lectura_participante_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_mensaje_lectura
    ADD CONSTRAINT reserva_mensaje_lectura_participante_id_fkey FOREIGN KEY (participante_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_mensaje_lectura reserva_mensaje_lectura_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_mensaje_lectura
    ADD CONSTRAINT reserva_mensaje_lectura_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_ensayo_operacion reserva_pago_ensayo_operacion_arrendatario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo_operacion
    ADD CONSTRAINT reserva_pago_ensayo_operacion_arrendatario_id_fkey FOREIGN KEY (arrendatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_ensayo_operacion reserva_pago_ensayo_operacion_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo_operacion
    ADD CONSTRAINT reserva_pago_ensayo_operacion_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_ensayo reserva_pago_ensayo_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_ensayo
    ADD CONSTRAINT reserva_pago_ensayo_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_evento_aplicacion_ensayo reserva_pago_evento_aplicacion_ensayo_evento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_aplicacion_ensayo
    ADD CONSTRAINT reserva_pago_evento_aplicacion_ensayo_evento_id_fkey FOREIGN KEY (evento_id) REFERENCES public.reserva_pago_evento_ensayo(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_evento_ensayo reserva_pago_evento_ensayo_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_evento_ensayo
    ADD CONSTRAINT reserva_pago_evento_ensayo_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.reserva_pago_ensayo_operacion(id) ON DELETE RESTRICT;


--
-- Name: reserva_pago_fake_resultado_ensayo reserva_pago_fake_resultado_ensayo_operacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_pago_fake_resultado_ensayo
    ADD CONSTRAINT reserva_pago_fake_resultado_ensayo_operacion_id_fkey FOREIGN KEY (operacion_id) REFERENCES public.reserva_pago_ensayo_operacion(id) ON DELETE RESTRICT;


--
-- Name: reserva_vinculo_purgado_local reserva_vinculo_purgado_local_reserva_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reserva_vinculo_purgado_local
    ADD CONSTRAINT reserva_vinculo_purgado_local_reserva_id_fkey FOREIGN KEY (reserva_id) REFERENCES public.reserva_ensayo_local(id) ON DELETE RESTRICT;


--
-- Name: rol_usuario rol_usuario_concedido_por_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario
    ADD CONSTRAINT rol_usuario_concedido_por_fk FOREIGN KEY (concedido_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: rol_usuario_evento rol_usuario_evento_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario_evento
    ADD CONSTRAINT rol_usuario_evento_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: rol_usuario rol_usuario_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_usuario
    ADD CONSTRAINT rol_usuario_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: sesion_administrativa sesion_administrativa_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion_administrativa
    ADD CONSTRAINT sesion_administrativa_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: sesion sesion_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: simulacion_precio_privada simulacion_precio_espacio_propietario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.simulacion_precio_privada
    ADD CONSTRAINT simulacion_precio_espacio_propietario_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: simulacion_precio_privada simulacion_precio_privada_propietario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.simulacion_precio_privada
    ADD CONSTRAINT simulacion_precio_privada_propietario_id_fkey FOREIGN KEY (propietario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: simulacion_precio_privada simulacion_precio_tarifa_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.simulacion_precio_privada
    ADD CONSTRAINT simulacion_precio_tarifa_fk FOREIGN KEY (espacio_id, tarifa_version) REFERENCES public.tarifa_espacio(espacio_id, version) ON DELETE RESTRICT;


--
-- Name: solicitud_privacidad_administrativa solicitud_privacidad_administrativa_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_privacidad_administrativa
    ADD CONSTRAINT solicitud_privacidad_administrativa_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id);


--
-- Name: solicitud_privacidad_administrativa solicitud_privacidad_administrativa_solicitado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_privacidad_administrativa
    ADD CONSTRAINT solicitud_privacidad_administrativa_solicitado_por_fkey FOREIGN KEY (solicitado_por) REFERENCES public.usuario(id);


--
-- Name: solicitud_titular solicitud_titular_asignado_a_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular
    ADD CONSTRAINT solicitud_titular_asignado_a_id_fkey FOREIGN KEY (asignado_a_id) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: solicitud_titular_evento solicitud_titular_evento_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular_evento
    ADD CONSTRAINT solicitud_titular_evento_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: solicitud_titular_evento solicitud_titular_evento_solicitud_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular_evento
    ADD CONSTRAINT solicitud_titular_evento_solicitud_id_fkey FOREIGN KEY (solicitud_id) REFERENCES public.solicitud_titular(id) ON DELETE CASCADE;


--
-- Name: solicitud_titular solicitud_titular_solicitado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular
    ADD CONSTRAINT solicitud_titular_solicitado_por_fkey FOREIGN KEY (solicitado_por) REFERENCES public.usuario(id) ON DELETE SET NULL;


--
-- Name: solicitud_titular solicitud_titular_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.solicitud_titular
    ADD CONSTRAINT solicitud_titular_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_categoria_codigo_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_categoria_codigo_fkey FOREIGN KEY (categoria_codigo) REFERENCES public.categoria_espacio(codigo) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_kyb_verification_id_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_kyb_verification_id_organizacion_id_fkey FOREIGN KEY (kyb_verification_id, organizacion_id) REFERENCES public.kyb_verifications(id, organization_id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_organizacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_organizacion_id_fkey FOREIGN KEY (organizacion_id) REFERENCES public.organizacion(id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_owner_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_owner_usuario_id_fkey FOREIGN KEY (owner_usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: space_kyb_verification space_kyb_verification_submitted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_kyb_verification
    ADD CONSTRAINT space_kyb_verification_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: space_media space_media_asset_scope_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_media
    ADD CONSTRAINT space_media_asset_scope_fk FOREIGN KEY (media_id, espacio_id, propietario_id) REFERENCES public.media_asset(id, espacio_id, propietario_id) ON DELETE RESTRICT;


--
-- Name: space_media space_media_space_owner_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_media
    ADD CONSTRAINT space_media_space_owner_fk FOREIGN KEY (espacio_id, propietario_id) REFERENCES public.espacio(id, propietario_id) ON DELETE RESTRICT;


--
-- Name: tarifa_espacio tarifa_espacio_espacio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tarifa_espacio
    ADD CONSTRAINT tarifa_espacio_espacio_id_fkey FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT;


--
-- Name: ticket_soporte_administrativo ticket_soporte_administrativo_asignado_a_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_soporte_administrativo
    ADD CONSTRAINT ticket_soporte_administrativo_asignado_a_id_fkey FOREIGN KEY (asignado_a_id) REFERENCES public.usuario(id);


--
-- Name: ticket_soporte_administrativo ticket_soporte_administrativo_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_soporte_administrativo
    ADD CONSTRAINT ticket_soporte_administrativo_cuenta_id_fkey FOREIGN KEY (cuenta_id) REFERENCES public.usuario(id);


--
-- Name: token_accion token_accion_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.token_accion
    ADD CONSTRAINT token_accion_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: verificacion_evidencia_sintetica verificacion_evidencia_verificacion_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_evidencia_sintetica
    ADD CONSTRAINT verificacion_evidencia_verificacion_fk FOREIGN KEY (verificacion_id) REFERENCES public.verificacion(id) ON DELETE RESTRICT;


--
-- Name: verificacion_historial_local verificacion_historial_local_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_historial_local
    ADD CONSTRAINT verificacion_historial_local_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: verificacion_historial_local verificacion_historial_local_verificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion_historial_local
    ADD CONSTRAINT verificacion_historial_local_verificacion_id_fkey FOREIGN KEY (verificacion_id) REFERENCES public.verificacion(id) ON DELETE RESTRICT;


--
-- Name: verificacion verificacion_reintento_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_reintento_fk FOREIGN KEY (reintento_de) REFERENCES public.verificacion(id) ON DELETE RESTRICT;


--
-- Name: verificacion verificacion_revisor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_revisor_fk FOREIGN KEY (revisor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: verificacion verificacion_revocada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_revocada_por_fkey FOREIGN KEY (revocada_por) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: verificacion verificacion_usuario_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verificacion
    ADD CONSTRAINT verificacion_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT;


--
-- Name: vista_asistida_administrativa vista_asistida_administrativa_actor_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vista_asistida_administrativa
    ADD CONSTRAINT vista_asistida_administrativa_actor_cuenta_id_fkey FOREIGN KEY (actor_cuenta_id) REFERENCES public.usuario(id);


--
-- Name: vista_asistida_administrativa vista_asistida_administrativa_objetivo_cuenta_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vista_asistida_administrativa
    ADD CONSTRAINT vista_asistida_administrativa_objetivo_cuenta_id_fkey FOREIGN KEY (objetivo_cuenta_id) REFERENCES public.usuario(id);


--
-- Name: vista_asistida_administrativa vista_asistida_administrativa_sesion_administrativa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vista_asistida_administrativa
    ADD CONSTRAINT vista_asistida_administrativa_sesion_administrativa_id_fkey FOREIGN KEY (sesion_administrativa_id) REFERENCES public.sesion_administrativa(id) ON DELETE CASCADE;


--
-- Name: FUNCTION account_profile_identity_status(subject uuid, as_of timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.account_profile_identity_status(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.account_profile_identity_status(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_confirmacion_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_confirmacion_guard() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_evento_consistency(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_evento_consistency() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_evento_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_evento_guard() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_idempotencia_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_idempotencia_guard() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_immutable_update(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_immutable_update() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_occupancy_consistency(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_occupancy_consistency() TO espacigo_runtime;


--
-- Name: FUNCTION arriendo_temprano_pago_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.arriendo_temprano_pago_guard() TO espacigo_runtime;


--
-- Name: FUNCTION assign_local_space_history_retention(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.assign_local_space_history_retention() FROM PUBLIC;


--
-- Name: FUNCTION build_public_search_document(p_status text, p_category text, p_title text, p_description text, p_location_version text, p_region_code text, p_commune_code text); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.build_public_search_document(text, text, text, text, text, text, text) TO espacigo_runtime;


--
-- Name: FUNCTION claim_expired_local_verification_files(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.claim_expired_local_verification_files(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.claim_expired_local_verification_files(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION claim_private_media_cleanup(p_limit integer, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION clear_commercial_space_pin_provenance(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.clear_commercial_space_pin_provenance() TO espacigo_runtime;


--
-- Name: TABLE identity_provider_attempt; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.identity_provider_attempt TO espacigo_runtime;


--
-- Name: FUNCTION commit_verified_identity_decision(p_attempt_id uuid, p_event_row_id uuid, p_rut_fingerprint text, p_session_id text, p_processed_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.commit_verified_identity_decision(uuid, uuid, text, text, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION complete_expired_local_verification_file(p_evidencia uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.complete_expired_local_verification_file(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION complete_private_media_cleanup(p_id uuid, p_token bigint, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_event_immutable(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_event_immutable() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_identity_case_insert_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_identity_case_insert_guard() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_identity_case_link_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_identity_case_link_guard() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_identity_review_audit_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_identity_review_audit_guard() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_reviewer_grant_audit(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_reviewer_grant_audit() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_reviewer_grant_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_reviewer_grant_guard() TO espacigo_runtime;


--
-- Name: FUNCTION document_verification_space_owner_changed(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.document_verification_space_owner_changed() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_booking_payment_notices(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_booking_payment_notices() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_checkout_payment_notices(p_operation_id uuid, p_transition text, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_checkout_payment_notices(uuid, text, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_contract_complete_notices(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_contract_complete_notices() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_contract_signature_notices(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_contract_signature_notices() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_cancel_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_cancel_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_checkin_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_checkin_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_claim_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_claim_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_damage_claim_resolution_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_damage_claim_resolution_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_notice(p_type text, p_aggregate_type text, p_aggregate uuid, p_recipient uuid, p_key text, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_notice(text, text, uuid, uuid, text, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_local_review_report_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_local_review_report_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_login_lock_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_login_lock_notice() TO espacigo_runtime;


--
-- Name: FUNCTION enqueue_verification_result_notice(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.enqueue_verification_result_notice() TO espacigo_runtime;


--
-- Name: FUNCTION fail_expired_local_verification_file(p_evidencia uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.fail_expired_local_verification_file(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION fail_private_media_cleanup(p_id uuid, p_token bigint, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION finalize_pending_suppression_cleanup(p_request_id uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION gcp06_test_access_operation_exists(p_id uuid); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.gcp06_test_access_operation_exists(uuid) FROM PUBLIC;


--
-- Name: FUNCTION gcp06_test_access_owner_ready(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.gcp06_test_access_owner_ready() FROM PUBLIC;


--
-- Name: FUNCTION gcp06_test_access_target_ids(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.gcp06_test_access_target_ids() FROM PUBLIC;


--
-- Name: FUNCTION guard_checkout_commercial_contract_binding(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_checkout_commercial_contract_binding() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_agreement(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_agreement() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_contract_document(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_contract_document() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_rental_authorization(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_rental_authorization() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_rental_tax_document(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_rental_tax_document() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_signature(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_signature() TO espacigo_runtime;


--
-- Name: FUNCTION guard_commercial_space_location(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_commercial_space_location() TO espacigo_runtime;


--
-- Name: FUNCTION guard_local_reservation_close_after_settlement(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guard_local_reservation_close_after_settlement() TO espacigo_runtime;


--
-- Name: FUNCTION guardar_modalidad_horario_activo(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guardar_modalidad_horario_activo() TO espacigo_runtime;


--
-- Name: FUNCTION guardar_zona_sin_horario_activo(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.guardar_zona_sin_horario_activo() TO espacigo_runtime;


--
-- Name: FUNCTION host_compliance_event_immutable(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.host_compliance_event_immutable() FROM PUBLIC;
GRANT ALL ON FUNCTION public.host_compliance_event_immutable() TO espacigo_runtime;


--
-- Name: FUNCTION identity_declaration_case_retention_guard(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.identity_declaration_case_retention_guard() TO espacigo_runtime;


--
-- Name: FUNCTION img01b_check_media_relation_scope(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.img01b_check_media_relation_scope() TO espacigo_runtime;


--
-- Name: FUNCTION img01b_enforce_gallery_slot(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.img01b_enforce_gallery_slot() FROM PUBLIC;


--
-- Name: FUNCTION img01b_guard_replacement_request_revision(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.img01b_guard_replacement_request_revision() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_bind_company_rut(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_bind_company_rut() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_bind_company_rut() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_enqueue_due_reminders(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_enqueue_due_reminders(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_enqueue_due_reminders(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_expire_noncompliant_profiles(p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_expire_noncompliant_profiles(timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_expire_noncompliant_profiles(timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_next_reminder(p_started_at timestamp with time zone, p_due_at timestamp with time zone, p_last_stage text, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_next_reminder(timestamptz, timestamptz, text, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_next_reminder(timestamptz, timestamptz, text, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_organization_reactivation_notice(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_organization_reactivation_notice() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_organization_reactivation_notice() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_personal_reactivation_notice(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_personal_reactivation_notice() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_personal_reactivation_notice() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_record_publication_document_request(p_space uuid, p_actor uuid); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_record_publication_document_request(uuid, uuid) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_record_publication_document_request(uuid, uuid) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_space_compliance_detail(p_espacio_id uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_space_compliance_detail(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_space_compliance_detail(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_space_document_request_notice(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_space_document_request_notice() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_space_document_request_notice() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_space_document_state(p_espacio_id uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_space_document_state(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_space_document_state(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_space_eligibility(p_espacio_id uuid, p_at timestamp with time zone); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_space_eligibility(uuid, timestamptz) FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_space_eligibility(uuid, timestamptz) TO espacigo_runtime;


--
-- Name: FUNCTION kyb_start_space_grace(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_start_space_grace() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_start_space_grace() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_submission_requires_active_member(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_submission_requires_active_member() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_submission_requires_active_member() TO espacigo_runtime;


--
-- Name: FUNCTION kyb_verification_event_immutable(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.kyb_verification_event_immutable() FROM PUBLIC;
GRANT ALL ON FUNCTION public.kyb_verification_event_immutable() TO espacigo_runtime;


--
-- Name: FUNCTION local_contract_effective_retention_deadline(p_reservation uuid); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.local_contract_effective_retention_deadline(uuid) FROM PUBLIC;


--
-- Name: FUNCTION local_retention_reservation_has_open_obligation(p_reservation uuid); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.local_retention_reservation_has_open_obligation(uuid) FROM PUBLIC;


--
-- Name: FUNCTION maintain_commercial_agreement_quote_anchor(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.maintain_commercial_agreement_quote_anchor() TO espacigo_runtime;


--
-- Name: FUNCTION organizacion_miembro_requires_legal_rep(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.organizacion_miembro_requires_legal_rep() FROM PUBLIC;
GRANT ALL ON FUNCTION public.organizacion_miembro_requires_legal_rep() TO espacigo_runtime;


--
-- Name: FUNCTION prevent_local_publication_history_mutation(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.prevent_local_publication_history_mutation() TO espacigo_runtime;


--
-- Name: FUNCTION prevent_local_space_deletion_history_mutation(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.prevent_local_space_deletion_history_mutation() TO espacigo_runtime;


--
-- Name: FUNCTION protect_commercial_agreement_event(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.protect_commercial_agreement_event() TO espacigo_runtime;


--
-- Name: FUNCTION protect_commercial_contract_template_version(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.protect_commercial_contract_template_version() TO espacigo_runtime;


--
-- Name: FUNCTION protect_commercial_rental_authorization_event(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.protect_commercial_rental_authorization_event() TO espacigo_runtime;


--
-- Name: FUNCTION public_catalog_billed_units(p_unit text, p_start timestamp with time zone, p_end timestamp with time zone, p_zone text); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.public_catalog_billed_units(text, timestamptz, timestamptz, text) TO espacigo_runtime;


--
-- Name: FUNCTION public_catalog_resolve_wall(p_wall timestamp without time zone, p_zone text); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.public_catalog_resolve_wall(timestamp, text) TO espacigo_runtime;


--
-- Name: FUNCTION purge_delivered_identity_session_outbox(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_delivered_identity_session_outbox() FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_delivered_identity_session_outbox() TO espacigo_runtime;


--
-- Name: FUNCTION purge_expired_identity_declaration_revisions(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_expired_identity_declaration_revisions() FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_expired_identity_declaration_revisions() TO espacigo_runtime;


--
-- Name: FUNCTION purge_expired_local_communication(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_expired_local_communication(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_expired_local_communication(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION purge_expired_local_contracts_and_publication_history(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_expired_local_contracts_and_publication_history(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION purge_expired_local_credential_notices(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_expired_local_credential_notices(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_expired_local_credential_notices(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION purge_expired_local_privacy_rows(p_at timestamp with time zone, p_limit integer); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_expired_local_privacy_rows(timestamptz, integer) FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_expired_local_privacy_rows(timestamptz, integer) TO espacigo_runtime;


--
-- Name: FUNCTION purge_unreferenced_local_publication_rates(p_space_id uuid); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) FROM PUBLIC;
GRANT ALL ON FUNCTION public.purge_unreferenced_local_publication_rates(uuid) TO espacigo_runtime;


--
-- Name: FUNCTION rechazar_cambio_tarifa_con_horario_activo(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.rechazar_cambio_tarifa_con_horario_activo() TO espacigo_runtime;


--
-- Name: FUNCTION refresh_category_public_search_document(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.refresh_category_public_search_document() TO espacigo_runtime;


--
-- Name: FUNCTION refresh_espacio_public_search_document(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.refresh_espacio_public_search_document() TO espacigo_runtime;


--
-- Name: FUNCTION reject_gadm_bootstrap_identity_change(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.reject_gadm_bootstrap_identity_change() TO espacigo_runtime;


--
-- Name: FUNCTION require_commercial_agreement_signatures(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.require_commercial_agreement_signatures() TO espacigo_runtime;


--
-- Name: FUNCTION revoke_commercial_rental_authorization_on_template_change(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.revoke_commercial_rental_authorization_on_template_change() TO espacigo_runtime;


--
-- Name: FUNCTION schedule_local_reservation_link_retention(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.schedule_local_reservation_link_retention() TO espacigo_runtime;


--
-- Name: FUNCTION set_local_verification_retention_deadline(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.set_local_verification_retention_deadline() TO espacigo_runtime;


--
-- Name: FUNCTION set_verification_evidence_retention_deadline(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.set_verification_evidence_retention_deadline() FROM PUBLIC;


--
-- Name: FUNCTION space_kyb_owner_guard(); Type: ACL; Schema: public; Owner: -
--

REVOKE ALL ON FUNCTION public.space_kyb_owner_guard() FROM PUBLIC;
GRANT ALL ON FUNCTION public.space_kyb_owner_guard() TO espacigo_runtime;


--
-- Name: FUNCTION validar_horario_semanal_fixture_hora(); Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON FUNCTION public.validar_horario_semanal_fixture_hora() TO espacigo_runtime;


--
-- Name: TABLE acceso_sensible_administrativo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.acceso_sensible_administrativo TO espacigo_runtime;


--
-- Name: TABLE account_identity_profile; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.account_identity_profile TO espacigo_runtime;


--
-- Name: TABLE account_identity_profile_draft; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.account_identity_profile_draft TO espacigo_runtime;


--
-- Name: TABLE account_identity_profile_phone; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.account_identity_profile_phone TO espacigo_runtime;


--
-- Name: TABLE account_identity_profile_revision; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.account_identity_profile_revision TO espacigo_runtime;


--
-- Name: TABLE account_identity_rut_claim; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.account_identity_rut_claim TO espacigo_runtime;


--
-- Name: TABLE account_identity_rut_fingerprint; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.account_identity_rut_fingerprint TO espacigo_runtime;


--
-- Name: TABLE account_identity_verification; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.account_identity_verification TO espacigo_runtime;


--
-- Name: TABLE account_identity_webhook_receipt; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.account_identity_webhook_receipt TO espacigo_runtime;


--
-- Name: TABLE account_pinpass; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.account_pinpass TO espacigo_runtime;


--
-- Name: TABLE account_pinpass_access; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.account_pinpass_access TO espacigo_runtime;


--
-- Name: TABLE account_pinpass_attempt; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.account_pinpass_attempt TO espacigo_runtime;


--
-- Name: TABLE account_pinpass_audit; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT ON TABLE public.account_pinpass_audit TO espacigo_runtime;


--
-- Name: TABLE account_pinpass_email_challenge; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.account_pinpass_email_challenge TO espacigo_runtime;


--
-- Name: TABLE aceptacion_terminos; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.aceptacion_terminos TO espacigo_runtime;


--
-- Name: TABLE administrador_global; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.administrador_global TO espacigo_runtime;


--
-- Name: TABLE administrador_global_bootstrap; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.administrador_global_bootstrap TO espacigo_runtime;


--
-- Name: TABLE administrador_global_bootstrap_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.administrador_global_bootstrap_evento TO espacigo_runtime;


--
-- Name: TABLE anfitrion_cumplimiento_documento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.anfitrion_cumplimiento_documento TO espacigo_runtime;


--
-- Name: TABLE anfitrion_cumplimiento_personal; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.anfitrion_cumplimiento_personal TO espacigo_runtime;


--
-- Name: COLUMN anfitrion_cumplimiento_personal.last_reminder_stage; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(last_reminder_stage) ON TABLE public.anfitrion_cumplimiento_personal TO espacigo_runtime;


--
-- Name: COLUMN anfitrion_cumplimiento_personal.updated_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(updated_at) ON TABLE public.anfitrion_cumplimiento_personal TO espacigo_runtime;


--
-- Name: TABLE aprobacion_administrativa; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.aprobacion_administrativa TO espacigo_runtime;


--
-- Name: TABLE arriendo_temprano; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.arriendo_temprano TO espacigo_runtime;


--
-- Name: TABLE arriendo_temprano_confirmacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.arriendo_temprano_confirmacion TO espacigo_runtime;


--
-- Name: TABLE arriendo_temprano_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.arriendo_temprano_evento TO espacigo_runtime;


--
-- Name: TABLE arriendo_temprano_idempotencia; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.arriendo_temprano_idempotencia TO espacigo_runtime;


--
-- Name: TABLE arriendo_temprano_pago; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.arriendo_temprano_pago TO espacigo_runtime;


--
-- Name: TABLE atributo_filtro_publico; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.atributo_filtro_publico TO espacigo_runtime;


--
-- Name: TABLE autorizacion_arriendo_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.autorizacion_arriendo_comercial TO espacigo_runtime;


--
-- Name: TABLE autorizacion_arriendo_comercial_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.autorizacion_arriendo_comercial_evento TO espacigo_runtime;


--
-- Name: TABLE autorizacion_firma_desafio_otp; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.autorizacion_firma_desafio_otp TO espacigo_runtime;


--
-- Name: TABLE aviso_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.aviso_local TO espacigo_runtime;


--
-- Name: TABLE aviso_local_ciclo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.aviso_local_ciclo TO espacigo_runtime;


--
-- Name: TABLE aviso_local_recuperacion_auditoria; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.aviso_local_recuperacion_auditoria TO espacigo_runtime;


--
-- Name: TABLE baja_archivo_pendiente_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.baja_archivo_pendiente_local TO espacigo_runtime;


--
-- Name: COLUMN baja_archivo_pendiente_local.ejecucion_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.evidencia_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.intentos; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.disponible_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.ultimo_codigo_error; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.completada_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.claim_owner; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.claim_hasta; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN baja_archivo_pendiente_local.claim_generacion; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE bloqueo_cuenta_administrativo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.bloqueo_cuenta_administrativo_local TO espacigo_runtime;


--
-- Name: TABLE bloqueo_cuenta_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.bloqueo_cuenta_historial_local TO espacigo_runtime;


--
-- Name: TABLE caso_soporte_reserva_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.caso_soporte_reserva_comercial TO espacigo_runtime;


--
-- Name: COLUMN caso_soporte_reserva_comercial.enrutado_a; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(enrutado_a) ON TABLE public.caso_soporte_reserva_comercial TO espacigo_runtime;


--
-- Name: COLUMN caso_soporte_reserva_comercial.estado; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(estado) ON TABLE public.caso_soporte_reserva_comercial TO espacigo_runtime;


--
-- Name: COLUMN caso_soporte_reserva_comercial.actualizada_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(actualizada_en) ON TABLE public.caso_soporte_reserva_comercial TO espacigo_runtime;


--
-- Name: TABLE catalogo_ubicacion_publica; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.catalogo_ubicacion_publica TO espacigo_runtime;


--
-- Name: TABLE categoria_espacio; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.categoria_espacio TO espacigo_runtime;


--
-- Name: TABLE categoria_perfil_atributos; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.categoria_perfil_atributos TO espacigo_runtime;


--
-- Name: TABLE checkout_cancelacion_pendiente; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.checkout_cancelacion_pendiente TO espacigo_runtime;


--
-- Name: TABLE checkout_payment_notice_outbox; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.checkout_payment_notice_outbox TO espacigo_runtime;


--
-- Name: TABLE checkout_preferencia_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.checkout_preferencia_comercial TO espacigo_runtime;


--
-- Name: TABLE comuna_publica; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.comuna_publica TO espacigo_runtime;


--
-- Name: TABLE contrato_documento_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_documento_comercial TO espacigo_runtime;


--
-- Name: TABLE contrato_ensayo_firma; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_ensayo_firma TO espacigo_runtime;


--
-- Name: TABLE contrato_ensayo_historial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_ensayo_historial TO espacigo_runtime;


--
-- Name: TABLE contrato_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE contrato_firma_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_firma_comercial TO espacigo_runtime;


--
-- Name: TABLE contrato_firma_desafio_otp; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_firma_desafio_otp TO espacigo_runtime;


--
-- Name: TABLE contrato_propuesta_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_propuesta_comercial TO espacigo_runtime;


--
-- Name: TABLE contrato_solicitud_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_solicitud_comercial TO espacigo_runtime;


--
-- Name: TABLE contrato_solicitud_evento_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_solicitud_evento_comercial TO espacigo_runtime;


--
-- Name: TABLE contrato_solicitud_notice_outbox; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_solicitud_notice_outbox TO espacigo_runtime;


--
-- Name: TABLE contrato_solicitud_quote_activa; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.contrato_solicitud_quote_activa TO espacigo_runtime;


--
-- Name: TABLE cotizacion_arriendo_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cotizacion_arriendo_comercial TO espacigo_runtime;


--
-- Name: TABLE cotizacion_arriendo_tramo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cotizacion_arriendo_tramo TO espacigo_runtime;


--
-- Name: TABLE cotizacion_modalidad_cobro; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cotizacion_modalidad_cobro TO espacigo_runtime;


--
-- Name: TABLE cotizacion_reserva_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cotizacion_reserva_ensayo TO espacigo_runtime;


--
-- Name: TABLE cuenta_alerta_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_alerta_evento TO espacigo_runtime;


--
-- Name: TABLE cuenta_alerta_preferencia; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_alerta_preferencia TO espacigo_runtime;


--
-- Name: TABLE cuenta_cobro_sintetica_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_cobro_sintetica_historial_local TO espacigo_runtime;


--
-- Name: SEQUENCE cuenta_cobro_sintetica_historial_local_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.cuenta_cobro_sintetica_historial_local_id_seq TO espacigo_runtime;


--
-- Name: TABLE cuenta_cobro_sintetica_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_cobro_sintetica_local TO espacigo_runtime;


--
-- Name: TABLE cuenta_passkey; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_passkey TO espacigo_runtime;


--
-- Name: TABLE cuenta_permiso_ubicacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_permiso_ubicacion TO espacigo_runtime;


--
-- Name: TABLE cuenta_persona_confianza; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_persona_confianza TO espacigo_runtime;


--
-- Name: TABLE cuenta_qr_dispositivo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_qr_dispositivo TO espacigo_runtime;


--
-- Name: TABLE cuenta_qr_ticket; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_qr_ticket TO espacigo_runtime;


--
-- Name: TABLE cuenta_totp; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_totp TO espacigo_runtime;


--
-- Name: TABLE cuenta_totp_backup_code; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_totp_backup_code TO espacigo_runtime;


--
-- Name: TABLE cuenta_verificacion_telefono; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_verificacion_telefono TO espacigo_runtime;


--
-- Name: TABLE cuenta_webauthn_challenge; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.cuenta_webauthn_challenge TO espacigo_runtime;


--
-- Name: TABLE desafio_mfa_administrativo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.desafio_mfa_administrativo TO espacigo_runtime;


--
-- Name: TABLE didit_webhook_inbox; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.didit_webhook_inbox TO espacigo_runtime;


--
-- Name: TABLE disputa_ensayo_historial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.disputa_ensayo_historial TO espacigo_runtime;


--
-- Name: SEQUENCE disputa_ensayo_historial_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.disputa_ensayo_historial_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE disputa_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.disputa_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE document_verification_case; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.document_verification_case TO espacigo_runtime;


--
-- Name: TABLE document_verification_event; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.document_verification_event TO espacigo_runtime;


--
-- Name: SEQUENCE document_verification_event_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.document_verification_event_id_seq TO espacigo_runtime;


--
-- Name: TABLE document_verification_file; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.document_verification_file TO espacigo_runtime;


--
-- Name: TABLE document_verification_job; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.document_verification_job TO espacigo_runtime;


--
-- Name: TABLE document_verification_reviewer_grant; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.document_verification_reviewer_grant TO espacigo_runtime;


--
-- Name: TABLE document_verification_reviewer_grant_event; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.document_verification_reviewer_grant_event TO espacigo_runtime;


--
-- Name: SEQUENCE document_verification_reviewer_grant_event_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.document_verification_reviewer_grant_event_id_seq TO espacigo_runtime;


--
-- Name: TABLE documento_privado_sintetico_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.documento_privado_sintetico_local TO espacigo_runtime;


--
-- Name: TABLE ejecucion_baja_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.ejecucion_baja_local TO espacigo_runtime;


--
-- Name: COLUMN ejecucion_baja_local.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN ejecucion_baja_local.solicitud_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN ejecucion_baja_local.estado; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN ejecucion_baja_local.iniciada_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE elegibilidad_verificacion_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.elegibilidad_verificacion_local TO espacigo_runtime;


--
-- Name: TABLE espacio; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: COLUMN espacio.es_b2b; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(es_b2b),UPDATE(es_b2b) ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: COLUMN espacio.organizacion_id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(organizacion_id),UPDATE(organizacion_id) ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: COLUMN espacio.modo_anfitrion; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(modo_anfitrion) ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: COLUMN espacio.kyb_grace_started_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(kyb_grace_started_at) ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: COLUMN espacio.kyb_grace_due_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(kyb_grace_due_at) ON TABLE public.espacio TO espacigo_runtime;


--
-- Name: TABLE espacio_calendario_override; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_calendario_override TO espacigo_runtime;


--
-- Name: TABLE espacio_caracteristicas; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_caracteristicas TO espacigo_runtime;


--
-- Name: TABLE espacio_contrato_borrador; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_contrato_borrador TO espacigo_runtime;


--
-- Name: TABLE espacio_contrato_version; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_contrato_version TO espacigo_runtime;


--
-- Name: TABLE espacio_eliminacion_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_eliminacion_historial_local TO espacigo_runtime;


--
-- Name: SEQUENCE espacio_eliminacion_historial_local_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.espacio_eliminacion_historial_local_id_seq TO espacigo_runtime;


--
-- Name: TABLE espacio_galeria_archivo_candidato_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_galeria_archivo_candidato_local TO espacigo_runtime;


--
-- Name: TABLE espacio_galeria_sintetica_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_galeria_sintetica_local TO espacigo_runtime;


--
-- Name: COLUMN espacio_galeria_sintetica_local.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN espacio_galeria_sintetica_local.archivo_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN espacio_galeria_sintetica_local.estado; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN espacio_galeria_sintetica_local.proximo_intento_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN espacio_galeria_sintetica_local.limpia_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN espacio_galeria_sintetica_local.ultimo_codigo_error; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE espacio_horario_semanal; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_horario_semanal TO espacigo_runtime;


--
-- Name: TABLE espacio_horario_semanal_tramo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_horario_semanal_tramo TO espacigo_runtime;


--
-- Name: TABLE espacio_publicacion_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_publicacion_historial_local TO espacigo_runtime;


--
-- Name: SEQUENCE espacio_publicacion_historial_local_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.espacio_publicacion_historial_local_id_seq TO espacigo_runtime;


--
-- Name: TABLE espacio_regla_disponibilidad; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_regla_disponibilidad TO espacigo_runtime;


--
-- Name: TABLE espacio_regla_mensual; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.espacio_regla_mensual TO espacigo_runtime;


--
-- Name: TABLE espacio_solicitud_documental; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT ON TABLE public.espacio_solicitud_documental TO espacigo_runtime;


--
-- Name: TABLE evento_auditoria_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.evento_auditoria_local TO espacigo_runtime;


--
-- Name: TABLE evento_caso_soporte_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.evento_caso_soporte_comercial TO espacigo_runtime;


--
-- Name: TABLE evidencia_caso_soporte_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.evidencia_caso_soporte_comercial TO espacigo_runtime;


--
-- Name: TABLE foto_perfil_sintetica_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.foto_perfil_sintetica_local TO espacigo_runtime;


--
-- Name: COLUMN foto_perfil_sintetica_local.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.archivo_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.mime_type; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.sha256; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.size_bytes; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.estado; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.limpia_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.proximo_intento_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN foto_perfil_sintetica_local.ultimo_codigo_error; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE gallery_item_order; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.gallery_item_order TO espacigo_runtime;


--
-- Name: TABLE gcp06_test_access_batch_audit; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.gcp06_test_access_batch_audit TO espacigo_runtime;


--
-- Name: TABLE historial_clave_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.historial_clave_local TO espacigo_runtime;


--
-- Name: TABLE host_compliance_event; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.host_compliance_event TO espacigo_runtime;


--
-- Name: SEQUENCE host_compliance_event_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT USAGE ON SEQUENCE public.host_compliance_event_id_seq TO espacigo_runtime;


--
-- Name: TABLE identity_declaration_revision; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.identity_declaration_revision TO espacigo_runtime;


--
-- Name: TABLE identity_provider_event; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.identity_provider_event TO espacigo_runtime;


--
-- Name: TABLE identity_rut_claim; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.identity_rut_claim TO espacigo_runtime;


--
-- Name: TABLE identity_verification_attempts; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.identity_verification_attempts TO espacigo_runtime;


--
-- Name: TABLE kyb_compliance_notice_outbox; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.kyb_compliance_notice_outbox TO espacigo_runtime;


--
-- Name: TABLE kyb_rut_claim; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: COLUMN kyb_rut_claim.rut_kind; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(rut_kind) ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: COLUMN kyb_rut_claim.fingerprint; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(fingerprint) ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: COLUMN kyb_rut_claim.verification_id; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT(verification_id),INSERT(verification_id) ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: COLUMN kyb_rut_claim.organization_id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(organization_id) ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: COLUMN kyb_rut_claim.active; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(active) ON TABLE public.kyb_rut_claim TO espacigo_runtime;


--
-- Name: TABLE kyb_verification_document; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(id) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.verification_id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(verification_id) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.document_kind; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(document_kind) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.storage_ref; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(storage_ref) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.mime_type; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(mime_type) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.size_bytes; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(size_bytes) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.content_sha256; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(content_sha256) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.review_status; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(review_status) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.rejection_reason; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(rejection_reason) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.reviewer_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(reviewer_id) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: COLUMN kyb_verification_document.reviewed_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(reviewed_at) ON TABLE public.kyb_verification_document TO espacigo_runtime;


--
-- Name: TABLE kyb_verification_event; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.kyb_verification_event TO espacigo_runtime;


--
-- Name: SEQUENCE kyb_verification_event_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.kyb_verification_event_id_seq TO espacigo_runtime;


--
-- Name: TABLE kyb_verifications; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(id) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.organization_id; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(organization_id) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.submitted_by; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(submitted_by) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.idempotency_key; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(idempotency_key) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.legal_name; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(legal_name) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.company_rut_fingerprint; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(company_rut_fingerprint) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.company_rut_key_version; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(company_rut_key_version) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.company_rut_ciphertext; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(company_rut_ciphertext) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.representative_name; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(representative_name) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.representative_rut_fingerprint; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(representative_rut_fingerprint) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.representative_rut_key_version; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(representative_rut_key_version) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.representative_rut_ciphertext; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(representative_rut_ciphertext) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.sii_giro_code; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(sii_giro_code) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.status; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(status),UPDATE(status) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.rejection_reason; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(rejection_reason) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.submitted_at; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(submitted_at) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.reviewed_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(reviewed_at) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.reviewer_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(reviewer_id) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.created_at; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(created_at) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: COLUMN kyb_verifications.updated_at; Type: ACL; Schema: public; Owner: -
--

GRANT INSERT(updated_at),UPDATE(updated_at) ON TABLE public.kyb_verifications TO espacigo_runtime;


--
-- Name: TABLE m02_operacion_idempotente_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.m02_operacion_idempotente_local TO espacigo_runtime;


--
-- Name: TABLE media_asset; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.media_asset TO espacigo_runtime;


--
-- Name: COLUMN media_asset.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_asset.propietario_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_asset.estado; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_asset.fencing_token; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_asset.lease_until; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_asset.updated_at; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE media_variant; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.media_variant TO espacigo_runtime;


--
-- Name: COLUMN media_variant.media_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.variant; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.provider; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.bucket_name; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.object_key; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.generation; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.mime_type; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.size_bytes; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.sha256; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.width; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant.height; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE media_variant_intent; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.media_variant_intent TO espacigo_runtime;


--
-- Name: COLUMN media_variant_intent.media_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant_intent.variant; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant_intent.object_key; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant_intent.generation; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN media_variant_intent.write_complete; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE mensaje_reserva_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.mensaje_reserva_comercial TO espacigo_runtime;


--
-- Name: SEQUENCE mensaje_reserva_comercial_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.mensaje_reserva_comercial_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE mensaje_reserva_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.mensaje_reserva_ensayo TO espacigo_runtime;


--
-- Name: SEQUENCE mensaje_reserva_ensayo_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.mensaje_reserva_ensayo_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE mercadopago_oauth_state; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.mercadopago_oauth_state TO espacigo_runtime;


--
-- Name: TABLE mercadopago_payment_webhook_receipt; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.mercadopago_payment_webhook_receipt TO espacigo_runtime;


--
-- Name: TABLE mercadopago_seller_connection; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.mercadopago_seller_connection TO espacigo_runtime;


--
-- Name: TABLE notificacion_usuario; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.notificacion_usuario TO espacigo_runtime;


--
-- Name: TABLE ocupacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.ocupacion TO espacigo_runtime;


--
-- Name: TABLE operacion_arriendo_archivo_candidato_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.operacion_arriendo_archivo_candidato_local TO espacigo_runtime;


--
-- Name: COLUMN operacion_arriendo_archivo_candidato_local.archivo_id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE operacion_arriendo_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.operacion_arriendo_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE operacion_arriendo_evidencia_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.operacion_arriendo_evidencia_ensayo_local TO espacigo_runtime;


--
-- Name: COLUMN operacion_arriendo_evidencia_ensayo_local.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN operacion_arriendo_evidencia_ensayo_local.retirar_archivo_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: COLUMN operacion_arriendo_evidencia_ensayo_local.archivo_retirado_en; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE operacion_arriendo_historial_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.operacion_arriendo_historial_ensayo_local TO espacigo_runtime;


--
-- Name: SEQUENCE operacion_arriendo_historial_ensayo_local_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.operacion_arriendo_historial_ensayo_local_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE organizacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.created_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(created_at) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.updated_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(updated_at) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.sii_giro_code; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(sii_giro_code) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.trade_name; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(trade_name) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.public_summary; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(public_summary) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.logo_media_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(logo_media_id) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.commune_code; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(commune_code) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.region_code; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(region_code) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.compliance_status; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(compliance_status) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.grace_started_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(grace_started_at) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.grace_due_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(grace_due_at) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.disabled_at; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(disabled_at) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.disabled_reason; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(disabled_reason) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.disabled_by; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(disabled_by) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: COLUMN organizacion.last_reminder_stage; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(last_reminder_stage) ON TABLE public.organizacion TO espacigo_runtime;


--
-- Name: TABLE organizacion_invitacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.organizacion_invitacion TO espacigo_runtime;


--
-- Name: TABLE organizacion_miembro; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.organizacion_miembro TO espacigo_runtime;


--
-- Name: TABLE organizacion_rut_fingerprint; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.organizacion_rut_fingerprint TO espacigo_runtime;


--
-- Name: TABLE outbox_evento_ciclo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.outbox_evento_ciclo_local TO espacigo_runtime;


--
-- Name: TABLE outbox_evento_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.outbox_evento_local TO espacigo_runtime;


--
-- Name: TABLE perfil_usuario; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.perfil_usuario TO espacigo_runtime;


--
-- Name: TABLE privacidad_bloqueo_tratamiento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: COLUMN privacidad_bloqueo_tratamiento.estado; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(estado) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: COLUMN privacidad_bloqueo_tratamiento.revisar_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(revisar_en) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: COLUMN privacidad_bloqueo_tratamiento.levantado_por; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(levantado_por) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: COLUMN privacidad_bloqueo_tratamiento.levantado_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(levantado_en) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: COLUMN privacidad_bloqueo_tratamiento.motivo_levantamiento; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(motivo_levantamiento) ON TABLE public.privacidad_bloqueo_tratamiento TO espacigo_runtime;


--
-- Name: TABLE privacidad_ejecucion_derecho; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacidad_ejecucion_derecho TO espacigo_runtime;


--
-- Name: TABLE privacidad_incidente_datos; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.privacidad_incidente_datos TO espacigo_runtime;


--
-- Name: TABLE privacidad_incidente_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacidad_incidente_evento TO espacigo_runtime;


--
-- Name: TABLE privacidad_notificacion_outbox; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.estado; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(estado) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.intentos; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(intentos) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.disponible_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(disponible_en) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.reclamado_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(reclamado_en) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.entregado_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(entregado_en) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.terminal_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(terminal_en) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: COLUMN privacidad_notificacion_outbox.ultimo_error_codigo; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(ultimo_error_codigo) ON TABLE public.privacidad_notificacion_outbox TO espacigo_runtime;


--
-- Name: TABLE privacidad_preferencia_finalidad; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.privacidad_preferencia_finalidad TO espacigo_runtime;


--
-- Name: TABLE privacidad_retencion_legal; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: COLUMN privacidad_retencion_legal.estado; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(estado) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: COLUMN privacidad_retencion_legal.revisar_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(revisar_en) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: COLUMN privacidad_retencion_legal.liberada_por; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(liberada_por) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: COLUMN privacidad_retencion_legal.liberada_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(liberada_en) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: COLUMN privacidad_retencion_legal.motivo_liberacion; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(motivo_liberacion) ON TABLE public.privacidad_retencion_legal TO espacigo_runtime;


--
-- Name: TABLE privacy_response_dispatch_receipt; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.privacy_response_dispatch_receipt TO espacigo_runtime;


--
-- Name: TABLE profile_media; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.profile_media TO espacigo_runtime;


--
-- Name: TABLE profile_pinpass_reauth; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.profile_pinpass_reauth TO espacigo_runtime;


--
-- Name: TABLE reaplicacion_baja_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reaplicacion_baja_local TO espacigo_runtime;


--
-- Name: TABLE reclamo_dano_descargo_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reclamo_dano_descargo_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reclamo_dano_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reclamo_dano_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reclamo_dano_historial_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reclamo_dano_historial_ensayo_local TO espacigo_runtime;


--
-- Name: SEQUENCE reclamo_dano_historial_ensayo_local_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.reclamo_dano_historial_ensayo_local_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE reclamo_dano_resolucion_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reclamo_dano_resolucion_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE region_publica; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.region_publica TO espacigo_runtime;


--
-- Name: TABLE registro_replay_baja_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.registro_replay_baja_local TO espacigo_runtime;


--
-- Name: TABLE reporte_resena_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reporte_resena_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reporte_resena_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reporte_resena_historial_local TO espacigo_runtime;


--
-- Name: SEQUENCE reporte_resena_historial_local_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.reporte_resena_historial_local_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE resena_autoria_marca_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.resena_autoria_marca_local TO espacigo_runtime;


--
-- Name: TABLE resena_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.resena_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE resena_reserva_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.resena_reserva_comercial TO espacigo_runtime;


--
-- Name: TABLE reserva_arriendo_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_arriendo_comercial TO espacigo_runtime;


--
-- Name: TABLE reserva_arriendo_tramo_estado; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_arriendo_tramo_estado TO espacigo_runtime;


--
-- Name: TABLE reserva_cancelacion_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_cancelacion_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_comercial_cancelacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.clave_idempotencia; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(clave_idempotencia) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.provider_refund_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(provider_refund_id) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.refund_status; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(refund_status) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.intentos; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(intentos) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.actualizada_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(actualizada_en) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: COLUMN reserva_comercial_cancelacion.resuelta_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(resuelta_en) ON TABLE public.reserva_comercial_cancelacion TO espacigo_runtime;


--
-- Name: TABLE reserva_comercial_mensaje_lectura; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_comercial_mensaje_lectura TO espacigo_runtime;


--
-- Name: TABLE reserva_comprobante_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_comprobante_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_cuota_mensual; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_cuota_mensual TO espacigo_runtime;


--
-- Name: TABLE reserva_decision_financiera_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_decision_financiera_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_devolucion_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_devolucion_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_devolucion_intento_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_devolucion_intento_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_documento_tributario_comercial; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_documento_tributario_comercial TO espacigo_runtime;


--
-- Name: TABLE reserva_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_ensayo_local_fixture; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_ensayo_local_fixture TO espacigo_runtime;


--
-- Name: TABLE reserva_ensayo_local_ubicacion_sintetica; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_ensayo_local_ubicacion_sintetica TO espacigo_runtime;


--
-- Name: TABLE reserva_ensayo_transicion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_ensayo_transicion TO espacigo_runtime;


--
-- Name: TABLE reserva_finanzas_historial_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_finanzas_historial_ensayo_local TO espacigo_runtime;


--
-- Name: SEQUENCE reserva_finanzas_historial_ensayo_local_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.reserva_finanzas_historial_ensayo_local_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE reserva_garantia_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_garantia_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_garantia_evento_aplicacion_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_garantia_evento_aplicacion_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_garantia_evento_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_garantia_evento_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_garantia_operacion_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_garantia_operacion_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_garantia_resultado_fake_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_garantia_resultado_fake_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_liquidacion_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_liquidacion_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_liquidacion_historial_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_liquidacion_historial_ensayo_local TO espacigo_runtime;


--
-- Name: SEQUENCE reserva_liquidacion_historial_ensayo_local_secuencia_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.reserva_liquidacion_historial_ensayo_local_secuencia_seq TO espacigo_runtime;


--
-- Name: TABLE reserva_liquidacion_operacion_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_liquidacion_operacion_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_liquidacion_resultado_fake_ensayo_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_liquidacion_resultado_fake_ensayo_local TO espacigo_runtime;


--
-- Name: TABLE reserva_mensaje_lectura; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_mensaje_lectura TO espacigo_runtime;


--
-- Name: TABLE reserva_pago_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_pago_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_pago_ensayo_operacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_pago_ensayo_operacion TO espacigo_runtime;


--
-- Name: TABLE reserva_pago_evento_aplicacion_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_pago_evento_aplicacion_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_pago_evento_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_pago_evento_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_pago_fake_resultado_ensayo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_pago_fake_resultado_ensayo TO espacigo_runtime;


--
-- Name: TABLE reserva_vinculo_purgado_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.reserva_vinculo_purgado_local TO espacigo_runtime;


--
-- Name: TABLE rol_usuario; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.rol_usuario TO espacigo_runtime;


--
-- Name: TABLE rol_usuario_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.rol_usuario_evento TO espacigo_runtime;


--
-- Name: TABLE sesion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.sesion TO espacigo_runtime;


--
-- Name: TABLE sesion_administrativa; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.sesion_administrativa TO espacigo_runtime;


--
-- Name: TABLE simulacion_precio_privada; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.simulacion_precio_privada TO espacigo_runtime;


--
-- Name: TABLE solicitud_privacidad_administrativa; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.solicitud_privacidad_administrativa TO espacigo_runtime;


--
-- Name: TABLE solicitud_titular; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.usuario_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(usuario_id) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.respuesta_titular; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(respuesta_titular) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.asignado_a_id; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(asignado_a_id) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.vence_en; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(vence_en) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.prorrogada_hasta; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(prorrogada_hasta) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.prorroga_motivo; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(prorroga_motivo) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.version; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(version) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: COLUMN solicitud_titular.folio_publico; Type: ACL; Schema: public; Owner: -
--

GRANT UPDATE(folio_publico) ON TABLE public.solicitud_titular TO espacigo_runtime;


--
-- Name: TABLE solicitud_titular_evento; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT ON TABLE public.solicitud_titular_evento TO espacigo_runtime;


--
-- Name: TABLE space_kyb_verification; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,UPDATE ON TABLE public.space_kyb_verification TO espacigo_runtime;


--
-- Name: TABLE space_media; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.space_media TO espacigo_runtime;


--
-- Name: TABLE tarifa_espacio; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.tarifa_espacio TO espacigo_runtime;


--
-- Name: TABLE ticket_soporte_administrativo; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.ticket_soporte_administrativo TO espacigo_runtime;


--
-- Name: TABLE token_accion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.token_accion TO espacigo_runtime;


--
-- Name: TABLE usuario; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.usuario TO espacigo_runtime;


--
-- Name: TABLE verificacion; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.verificacion TO espacigo_runtime;


--
-- Name: TABLE verificacion_archivo_retiro_pendiente_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.verificacion_archivo_retiro_pendiente_local TO espacigo_runtime;


--
-- Name: TABLE verificacion_evidencia_sintetica; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.verificacion_evidencia_sintetica TO espacigo_runtime;


--
-- Name: COLUMN verificacion_evidencia_sintetica.id; Type: ACL; Schema: public; Owner: -
--



--
-- Name: TABLE verificacion_historial_local; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.verificacion_historial_local TO espacigo_runtime;


--
-- Name: SEQUENCE verificacion_historial_local_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,USAGE ON SEQUENCE public.verificacion_historial_local_id_seq TO espacigo_runtime;


--
-- Name: TABLE version_terminos; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.version_terminos TO espacigo_runtime;


--
-- Name: TABLE vista_asistida_administrativa; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.vista_asistida_administrativa TO espacigo_runtime;


--
-- Name: DEFAULT PRIVILEGES FOR SEQUENCES; Type: DEFAULT ACL; Schema: public; Owner: -
--

ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT SELECT,USAGE ON SEQUENCES TO espacigo_runtime;


--
-- Name: DEFAULT PRIVILEGES FOR FUNCTIONS; Type: DEFAULT ACL; Schema: public; Owner: -
--

ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT ALL ON FUNCTIONS TO espacigo_runtime;


--
-- Name: DEFAULT PRIVILEGES FOR TABLES; Type: DEFAULT ACL; Schema: public; Owner: -
--

ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT SELECT,INSERT,DELETE,UPDATE ON TABLES TO espacigo_runtime;


--
-- PostgreSQL database dump complete
--
--
-- PostgreSQL database dump
--


-- Dumped from database version 18.6 (Debian 18.6-1.pgdg13+2)
-- Dumped by pg_dump version 18.6 (Debian 18.6-1.pgdg13+2)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Data for Name: categoria_espacio; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.categoria_espacio VALUES ('oficina', 'Oficina', 1, true);
INSERT INTO public.categoria_espacio VALUES ('sala_multiproposito', 'Sala o espacio multipropósito', 2, true);
INSERT INTO public.categoria_espacio VALUES ('bodega', 'Bodega', 3, true);
INSERT INTO public.categoria_espacio VALUES ('estacionamiento', 'Estacionamiento', 4, true);
INSERT INTO public.categoria_espacio VALUES ('local_flexible', 'Local flexible', 5, true);
INSERT INTO public.categoria_espacio VALUES ('stand', 'Stand', 6, true);
INSERT INTO public.categoria_espacio VALUES ('quincho', 'Quincho', 7, true);
INSERT INTO public.categoria_espacio VALUES ('parcela_eventos', 'Parcela o espacio para eventos', 8, true);
INSERT INTO public.categoria_espacio VALUES ('inmuebles', 'Inmuebles', 9, true);


--
-- Data for Name: categoria_perfil_atributos; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.categoria_perfil_atributos VALUES ('parcela_eventos', 1, '{"attributes": [{"code": "superficie_exterior_util_m2", "step": 0.01, "type": "number", "unit": "m²", "label": "Superficie exterior útil", "order": 1, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "superficie_cubierta_util_m2", "step": 0.01, "type": "number", "unit": "m²", "label": "Superficie cubierta útil", "order": 2, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "estacionamientos_disponibles", "type": "integer", "label": "Estacionamientos disponibles", "order": 3, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "salon_cubierto", "type": "boolean", "label": "Salón cubierto", "order": 4, "filter_candidate": false}, {"code": "cocina_disponible", "type": "boolean", "label": "Cocina disponible", "order": 5, "filter_candidate": false}, {"code": "quincho_disponible", "type": "boolean", "label": "Quincho disponible", "order": 6, "filter_candidate": false}, {"code": "piscina_disponible", "type": "boolean", "label": "Piscina disponible", "order": 7, "filter_candidate": false}, {"code": "acceso_bus", "type": "boolean", "label": "Acceso para bus", "order": 8, "filter_candidate": false}, {"code": "agua_potable", "type": "boolean", "label": "Agua potable", "order": 9, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 10, "filter_candidate": false}, {"code": "banos_disponibles", "type": "integer", "label": "Baños disponibles", "order": 11, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}], "category_code": "parcela_eventos", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('oficina', 1, '{"attributes": [{"code": "puestos_trabajo", "type": "integer", "label": "Puestos de trabajo", "order": 1, "maximum": 2147483647, "minimum": 1, "filter_candidate": false}, {"code": "mobiliario_incluido", "type": "boolean", "label": "Mobiliario incluido", "order": 2, "filter_candidate": false}, {"code": "tipo_uso_oficina", "type": "enum", "label": "Tipo de uso", "order": 3, "options": ["privada", "compartida"], "filter_candidate": false}, {"code": "escritorios", "type": "integer", "label": "Escritorios", "order": 4, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "sillas_trabajo", "type": "integer", "label": "Sillas de trabajo", "order": 5, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "salas_reunion_disponibles", "type": "integer", "label": "Salas de reunión disponibles", "order": 6, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "impresora_disponible", "type": "boolean", "label": "Impresora disponible", "order": 7, "filter_candidate": false}, {"code": "wifi", "type": "boolean", "label": "Wi-Fi", "order": 8, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 9, "filter_candidate": false}, {"code": "banos_disponibles", "type": "integer", "label": "Baños disponibles", "order": 10, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "climatizacion", "type": "enum_list", "label": "Climatización", "order": 11, "options": ["aire_acondicionado", "calefaccion", "ventilacion_mecanica"], "filter_candidate": false}], "category_code": "oficina", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('sala_multiproposito', 1, '{"attributes": [{"code": "disposiciones_admitidas", "type": "enum_list", "label": "Disposiciones admitidas", "order": 1, "options": ["auditorio", "mesa_reunion", "aula", "espacio_libre"], "filter_candidate": false}, {"code": "sillas_disponibles", "type": "integer", "label": "Sillas disponibles", "order": 2, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "proyector", "type": "boolean", "label": "Proyector", "order": 3, "filter_candidate": false}, {"code": "mesas_disponibles", "type": "integer", "label": "Mesas disponibles", "order": 4, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "pantalla", "type": "boolean", "label": "Pantalla", "order": 5, "filter_candidate": false}, {"code": "pizarra", "type": "boolean", "label": "Pizarra", "order": 6, "filter_candidate": false}, {"code": "videoconferencia", "type": "boolean", "label": "Videoconferencia", "order": 7, "filter_candidate": false}, {"code": "equipo_audio", "type": "boolean", "label": "Equipo de audio", "order": 8, "filter_candidate": false}, {"code": "wifi", "type": "boolean", "label": "Wi-Fi", "order": 9, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 10, "filter_candidate": false}, {"code": "banos_disponibles", "type": "integer", "label": "Baños disponibles", "order": 11, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "climatizacion", "type": "enum_list", "label": "Climatización", "order": 12, "options": ["aire_acondicionado", "calefaccion", "ventilacion_mecanica"], "filter_candidate": false}], "category_code": "sala_multiproposito", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('bodega', 1, '{"attributes": [{"code": "altura_util_m", "step": 0.01, "type": "number", "unit": "m", "label": "Altura útil", "order": 1, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "volumen_util_m3", "step": 0.01, "type": "number", "unit": "m³", "label": "Volumen útil", "order": 2, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "acceso_carga", "type": "enum", "label": "Acceso de carga", "order": 3, "options": ["peatonal", "vehiculo_liviano", "furgon", "camion"], "filter_candidate": false}, {"code": "ancho_acceso_m", "step": 0.01, "type": "number", "unit": "m", "label": "Ancho de acceso", "order": 4, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "altura_acceso_m", "step": 0.01, "type": "number", "unit": "m", "label": "Altura de acceso", "order": 5, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "carro_carga_disponible", "type": "boolean", "label": "Carro de carga disponible", "order": 6, "filter_candidate": false}, {"code": "muelle_carga", "type": "boolean", "label": "Muelle de carga", "order": 7, "filter_candidate": false}, {"code": "tipo_almacenamiento", "type": "enum", "label": "Tipo de almacenamiento", "order": 8, "options": ["seco", "temperatura_controlada", "refrigerado"], "filter_candidate": false}], "category_code": "bodega", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('estacionamiento', 1, '{"attributes": [{"code": "largo_plaza_m", "step": 0.01, "type": "number", "unit": "m", "label": "Largo de plaza", "order": 1, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "ancho_plaza_m", "step": 0.01, "type": "number", "unit": "m", "label": "Ancho de plaza", "order": 2, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "vehiculos_admitidos", "type": "enum_list", "label": "Vehículos admitidos", "order": 3, "options": ["motocicleta", "automovil", "suv", "furgon"], "filter_candidate": false}, {"code": "altura_maxima_m", "step": 0.01, "type": "number", "unit": "m", "label": "Altura máxima", "order": 4, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "techado", "type": "boolean", "label": "Techado", "order": 5, "filter_candidate": false}, {"code": "tipo_acceso", "type": "enum", "label": "Tipo de acceso", "order": 6, "options": ["abierto", "porton_manual", "porton_automatico"], "filter_candidate": false}, {"code": "carga_electrica", "type": "boolean", "label": "Carga eléctrica", "order": 7, "filter_candidate": false}], "category_code": "estacionamiento", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('local_flexible', 1, '{"attributes": [{"code": "acceso_nivel_calle", "type": "boolean", "label": "Acceso a nivel de calle", "order": 1, "filter_candidate": false}, {"code": "vitrina", "type": "boolean", "label": "Vitrina", "order": 2, "filter_candidate": false}, {"code": "frente_m", "step": 0.01, "type": "number", "unit": "m", "label": "Frente", "order": 3, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "mostradores", "type": "integer", "label": "Mostradores", "order": 4, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "probadores", "type": "integer", "label": "Probadores", "order": 5, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "bodega_apoyo_m2", "step": 0.01, "type": "number", "unit": "m²", "label": "Bodega de apoyo", "order": 6, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "iluminacion_exhibicion", "type": "boolean", "label": "Iluminación de exhibición", "order": 7, "filter_candidate": false}, {"code": "wifi", "type": "boolean", "label": "Wi-Fi", "order": 8, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 9, "filter_candidate": false}, {"code": "banos_disponibles", "type": "integer", "label": "Baños disponibles", "order": 10, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "climatizacion", "type": "enum_list", "label": "Climatización", "order": 11, "options": ["aire_acondicionado", "calefaccion", "ventilacion_mecanica"], "filter_candidate": false}], "category_code": "local_flexible", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('stand', 1, '{"attributes": [{"code": "ancho_stand_m", "step": 0.01, "type": "number", "unit": "m", "label": "Ancho del stand", "order": 1, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "fondo_stand_m", "step": 0.01, "type": "number", "unit": "m", "label": "Fondo del stand", "order": 2, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "montaje_incluido", "type": "boolean", "label": "Montaje incluido", "order": 3, "filter_candidate": false}, {"code": "lados_abiertos", "type": "integer", "label": "Lados abiertos", "order": 4, "maximum": 4, "minimum": 1, "filter_candidate": false}, {"code": "altura_maxima_m", "step": 0.01, "type": "number", "unit": "m", "label": "Altura máxima", "order": 5, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": false}, {"code": "potencia_disponible_w", "type": "integer", "label": "Potencia disponible", "order": 6, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "paneleria", "type": "boolean", "label": "Panelería", "order": 7, "filter_candidate": false}, {"code": "mesas_incluidas", "type": "integer", "label": "Mesas incluidas", "order": 8, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "sillas_incluidas", "type": "integer", "label": "Sillas incluidas", "order": 9, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 10, "filter_candidate": false}, {"code": "wifi", "type": "boolean", "label": "Wi-Fi", "order": 11, "filter_candidate": false}], "category_code": "stand", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('quincho', 1, '{"attributes": [{"code": "tipo_parrilla", "type": "enum", "label": "Tipo de parrilla", "order": 1, "options": ["carbon", "gas", "electrica", "lena", "sin_parrilla"], "filter_candidate": false}, {"code": "techado", "type": "boolean", "label": "Techado", "order": 2, "filter_candidate": false}, {"code": "asientos_disponibles", "type": "integer", "label": "Asientos disponibles", "order": 3, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "parrillas_disponibles", "type": "integer", "label": "Parrillas disponibles", "order": 4, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "mesas_disponibles", "type": "integer", "label": "Mesas disponibles", "order": 5, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "lavaplatos", "type": "boolean", "label": "Lavaplatos", "order": 6, "filter_candidate": false}, {"code": "refrigerador", "type": "boolean", "label": "Refrigerador", "order": 7, "filter_candidate": false}, {"code": "utensilios_incluidos", "type": "boolean", "label": "Utensilios incluidos", "order": 8, "filter_candidate": false}, {"code": "agua_potable", "type": "boolean", "label": "Agua potable", "order": 9, "filter_candidate": false}, {"code": "energia_electrica", "type": "boolean", "label": "Energía eléctrica", "order": 10, "filter_candidate": false}, {"code": "banos_disponibles", "type": "integer", "label": "Baños disponibles", "order": 11, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}], "category_code": "quincho", "schema_version": 1}', '2026-11-15 21:54:49.637547+00');
INSERT INTO public.categoria_perfil_atributos VALUES ('inmuebles', 1, '{"attributes": [{"code": "tipo_inmueble", "type": "enum", "label": "Tipo de inmueble", "order": 1, "options": ["Casa", "Departamento", "Pieza o habitación", "Estudio o loft", "Cabaña", "Casa anexa", "Dúplex", "Otro"], "required": true, "filter_candidate": true}, {"code": "tipo_de_espacio", "type": "enum", "label": "Qué se arrienda", "order": 2, "options": ["Inmueble completo", "Habitación privada", "Habitación compartida", "Cama en habitación compartida"], "required": true, "filter_candidate": true}, {"code": "superficie_construida_m2", "step": 0.01, "type": "number", "unit": "m²", "label": "Superficie construida", "order": 3, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": true}, {"code": "superficie_terreno_m2", "step": 0.01, "type": "number", "unit": "m²", "label": "Superficie del terreno", "order": 4, "maximum": 99999999.99, "minimum": 0.01, "filter_candidate": true}, {"code": "dormitorios", "type": "integer", "label": "Dormitorios", "order": 5, "maximum": 2147483647, "minimum": 0, "filter_candidate": true}, {"code": "camas", "type": "integer", "label": "Camas", "order": 6, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "banos_completos", "type": "integer", "label": "Baños completos", "order": 7, "maximum": 2147483647, "minimum": 0, "filter_candidate": true}, {"code": "medios_banos", "type": "integer", "label": "Medios baños", "order": 8, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "privacidad_bano", "type": "enum", "label": "Uso del baño", "order": 9, "options": ["Privado en suite", "Privado de uso exclusivo", "Compartido", "No disponible"], "filter_candidate": true}, {"code": "cocina", "type": "enum", "label": "Cocina", "order": 10, "options": ["Completa", "Kitchenette", "Compartida", "No disponible"], "filter_candidate": false}, {"code": "amoblado", "type": "boolean", "label": "Amoblado", "order": 11, "filter_candidate": true}, {"code": "estacionamientos", "type": "integer", "label": "Estacionamientos", "order": 12, "maximum": 2147483647, "minimum": 0, "filter_candidate": true}, {"code": "bodegas", "type": "integer", "label": "Bodegas", "order": 13, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "piso", "type": "integer", "label": "Piso de la unidad", "order": 14, "maximum": 2147483647, "minimum": 0, "filter_candidate": false}, {"code": "pisos_edificio", "type": "integer", "label": "Pisos del edificio", "order": 15, "maximum": 2147483647, "minimum": 1, "filter_candidate": false}, {"code": "ascensor", "type": "boolean", "label": "Ascensor", "order": 16, "filter_candidate": true}, {"code": "balcon", "type": "boolean", "label": "Balcón", "order": 17, "filter_candidate": true}, {"code": "terraza", "type": "boolean", "label": "Terraza", "order": 18, "filter_candidate": true}, {"code": "patio_o_jardin", "type": "boolean", "label": "Patio o jardín", "order": 19, "filter_candidate": true}, {"code": "piscina", "type": "boolean", "label": "Piscina", "order": 20, "filter_candidate": true}, {"code": "wifi", "type": "boolean", "label": "Wi-Fi", "order": 21, "filter_candidate": true}, {"code": "calefaccion", "type": "boolean", "label": "Calefacción", "order": 22, "filter_candidate": true}, {"code": "aire_acondicionado", "type": "boolean", "label": "Aire acondicionado", "order": 23, "filter_candidate": true}, {"code": "agua_caliente", "type": "boolean", "label": "Agua caliente", "order": 24, "filter_candidate": false}, {"code": "lavadora", "type": "boolean", "label": "Lavadora disponible", "order": 25, "filter_candidate": false}, {"code": "secadora", "type": "boolean", "label": "Secadora disponible", "order": 26, "filter_candidate": false}, {"code": "refrigerador", "type": "boolean", "label": "Refrigerador", "order": 27, "filter_candidate": false}, {"code": "servicios_incluidos", "type": "enum_list", "label": "Servicios incluidos", "order": 28, "options": ["Agua", "Electricidad", "Gas", "Internet", "Gastos comunes", "Limpieza", "Ropa de cama"], "filter_candidate": false}, {"code": "mascotas", "type": "enum", "label": "Mascotas", "order": 29, "options": ["Permitidas", "No permitidas", "Consultar"], "filter_candidate": true}, {"code": "visitas", "type": "enum", "label": "Visitas", "order": 30, "options": ["Permitidas", "Con restricciones", "No permitidas"], "filter_candidate": false}, {"code": "fumadores", "type": "boolean", "label": "Fumadores", "order": 31, "filter_candidate": false}, {"code": "fiestas_o_reuniones", "type": "boolean", "label": "Fiestas o reuniones", "order": 32, "filter_candidate": false}, {"code": "duracion_minima_unidades", "type": "integer", "label": "Estadía mínima en la modalidad seleccionada", "order": 33, "maximum": 2147483647, "minimum": 1, "description": "Se expresa en la unidad de la tarifa base: horas, días o meses.", "filter_candidate": false}, {"code": "duracion_maxima_unidades", "type": "integer", "label": "Estadía máxima en la modalidad seleccionada", "order": 34, "maximum": 2147483647, "minimum": 1, "description": "Se expresa en la unidad de la tarifa base: horas, días o meses.", "filter_candidate": false}], "category_code": "inmuebles", "schema_version": 1}', '2026-11-15 21:55:07.43609+00');


--
-- Data for Name: atributo_filtro_publico; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'wifi', 'Wi-Fi', 'boolean', 8);
INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'mobiliario_incluido', 'Mobiliario incluido', 'boolean', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'puestos_trabajo', 'Puestos de trabajo', 'integer', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'tipo_uso_oficina', 'Tipo de uso', 'enum', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'escritorios', 'Escritorios', 'integer', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('oficina', 1, 'climatizacion', 'Climatización', 'enum_list', 11);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'wifi', 'Wi-Fi', 'boolean', 9);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'proyector', 'Proyector', 'boolean', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'disposiciones_admitidas', 'Disposiciones admitidas', 'enum_list', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'sillas_disponibles', 'Sillas disponibles', 'integer', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'mesas_disponibles', 'Mesas disponibles', 'integer', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'pantalla', 'Pantalla', 'boolean', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('sala_multiproposito', 1, 'videoconferencia', 'Videoconferencia', 'boolean', 7);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'carro_carga_disponible', 'Carro de carga disponible', 'boolean', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'muelle_carga', 'Muelle de carga', 'boolean', 7);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'altura_util_m', 'Altura útil', 'number', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'volumen_util_m3', 'Volumen útil', 'number', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'acceso_carga', 'Acceso de carga', 'enum', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'ancho_acceso_m', 'Ancho de acceso', 'number', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'altura_acceso_m', 'Altura de acceso', 'number', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('bodega', 1, 'tipo_almacenamiento', 'Tipo de almacenamiento', 'enum', 8);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'techado', 'Techado', 'boolean', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'carga_electrica', 'Carga eléctrica', 'boolean', 7);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'largo_plaza_m', 'Largo de plaza', 'number', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'ancho_plaza_m', 'Ancho de plaza', 'number', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'vehiculos_admitidos', 'Vehículos admitidos', 'enum_list', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'altura_maxima_m', 'Altura máxima', 'number', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('estacionamiento', 1, 'tipo_acceso', 'Tipo de acceso', 'enum', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'acceso_nivel_calle', 'Acceso a nivel de calle', 'boolean', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'vitrina', 'Vitrina', 'boolean', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'frente_m', 'Frente', 'number', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'probadores', 'Probadores', 'integer', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'bodega_apoyo_m2', 'Bodega de apoyo', 'number', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('local_flexible', 1, 'climatizacion', 'Climatización', 'enum_list', 11);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'montaje_incluido', 'Montaje incluido', 'boolean', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'wifi', 'Wi-Fi', 'boolean', 11);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'ancho_stand_m', 'Ancho del stand', 'number', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'fondo_stand_m', 'Fondo del stand', 'number', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'lados_abiertos', 'Lados abiertos', 'integer', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'altura_maxima_m', 'Altura máxima', 'number', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'potencia_disponible_w', 'Potencia disponible', 'integer', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'mesas_incluidas', 'Mesas incluidas', 'integer', 8);
INSERT INTO public.atributo_filtro_publico VALUES ('stand', 1, 'sillas_incluidas', 'Sillas incluidas', 'integer', 9);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'techado', 'Techado', 'boolean', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'agua_potable', 'Agua potable', 'boolean', 9);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'tipo_parrilla', 'Tipo de parrilla', 'enum', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'asientos_disponibles', 'Asientos disponibles', 'integer', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'parrillas_disponibles', 'Parrillas disponibles', 'integer', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'mesas_disponibles', 'Mesas disponibles', 'integer', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'lavaplatos', 'Lavaplatos', 'boolean', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('quincho', 1, 'refrigerador', 'Refrigerador', 'boolean', 7);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'salon_cubierto', 'Salón cubierto', 'boolean', 4);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'quincho_disponible', 'Quincho disponible', 'boolean', 6);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'superficie_exterior_util_m2', 'Superficie exterior útil', 'number', 1);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'superficie_cubierta_util_m2', 'Superficie cubierta útil', 'number', 2);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'estacionamientos_disponibles', 'Estacionamientos disponibles', 'integer', 3);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'cocina_disponible', 'Cocina disponible', 'boolean', 5);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'piscina_disponible', 'Piscina disponible', 'boolean', 7);
INSERT INTO public.atributo_filtro_publico VALUES ('parcela_eventos', 1, 'acceso_bus', 'Acceso para bus', 'boolean', 8);


--
-- Data for Name: catalogo_ubicacion_publica; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.catalogo_ubicacion_publica VALUES ('SUBDERE_DPA_2018', 'SUBDERE CUT 2018 v04 / IDE Chile DPA 2018', '2018-09-21', true);


--
-- Data for Name: region_publica; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '01', 'Tarapacá');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '02', 'Antofagasta');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '03', 'Atacama');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '04', 'Coquimbo');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '05', 'Valparaíso');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '06', 'Libertador General Bernardo O''Higgins');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '07', 'Maule');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '08', 'Biobío');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '09', 'La Araucanía');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '10', 'Los Lagos');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '11', 'Aysén del General Carlos Ibáñez del Campo');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '12', 'Magallanes y de la Antártica Chilena');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '13', 'Metropolitana de Santiago');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '14', 'Los Ríos');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '15', 'Arica y Parinacota');
INSERT INTO public.region_publica VALUES ('SUBDERE_DPA_2018', '16', 'Ñuble');


--
-- Data for Name: comuna_publica; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01101', '01', 'Iquique');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01107', '01', 'Alto Hospicio');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01401', '01', 'Pozo Almonte');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01402', '01', 'Camiña');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01403', '01', 'Colchane');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01404', '01', 'Huara');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '01405', '01', 'Pica');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02101', '02', 'Antofagasta');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02102', '02', 'Mejillones');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02103', '02', 'Sierra Gorda');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02104', '02', 'Taltal');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02201', '02', 'Calama');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02202', '02', 'Ollagüe');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02203', '02', 'San Pedro de Atacama');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02301', '02', 'Tocopilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '02302', '02', 'María Elena');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03101', '03', 'Copiapó');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03102', '03', 'Caldera');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03103', '03', 'Tierra Amarilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03201', '03', 'Chañaral');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03202', '03', 'Diego de Almagro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03301', '03', 'Vallenar');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03302', '03', 'Alto del Carmen');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03303', '03', 'Freirina');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '03304', '03', 'Huasco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04101', '04', 'La Serena');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04102', '04', 'Coquimbo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04103', '04', 'Andacollo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04104', '04', 'La Higuera');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04105', '04', 'Paiguano');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04106', '04', 'Vicuña');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04201', '04', 'Illapel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04202', '04', 'Canela');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04203', '04', 'Los Vilos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04204', '04', 'Salamanca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04301', '04', 'Ovalle');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04302', '04', 'Combarbalá');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04303', '04', 'Monte Patria');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04304', '04', 'Punitaqui');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '04305', '04', 'Río Hurtado');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05101', '05', 'Valparaíso');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05102', '05', 'Casablanca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05103', '05', 'Concón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05104', '05', 'Juan Fernández');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05105', '05', 'Puchuncaví');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05107', '05', 'Quintero');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05109', '05', 'Viña del Mar');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05201', '05', 'Isla de Pascua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05301', '05', 'Los Andes');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05302', '05', 'Calle Larga');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05303', '05', 'Rinconada');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05304', '05', 'San Esteban');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05401', '05', 'La Ligua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05402', '05', 'Cabildo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05403', '05', 'Papudo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05404', '05', 'Petorca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05405', '05', 'Zapallar');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05501', '05', 'Quillota');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05502', '05', 'Calera');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05503', '05', 'Hijuelas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05504', '05', 'La Cruz');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05506', '05', 'Nogales');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05601', '05', 'San Antonio');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05602', '05', 'Algarrobo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05603', '05', 'Cartagena');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05604', '05', 'El Quisco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05605', '05', 'El Tabo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05606', '05', 'Santo Domingo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05701', '05', 'San Felipe');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05702', '05', 'Catemu');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05703', '05', 'Llaillay');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05704', '05', 'Panquehue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05705', '05', 'Putaendo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05706', '05', 'Santa María');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05801', '05', 'Quilpué');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05802', '05', 'Limache');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05803', '05', 'Olmué');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '05804', '05', 'Villa Alemana');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06101', '06', 'Rancagua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06102', '06', 'Codegua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06103', '06', 'Coinco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06104', '06', 'Coltauco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06105', '06', 'Doñihue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06106', '06', 'Graneros');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06107', '06', 'Las Cabras');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06108', '06', 'Machalí');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06109', '06', 'Malloa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06110', '06', 'Mostazal');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06111', '06', 'Olivar');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06112', '06', 'Peumo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06113', '06', 'Pichidegua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06114', '06', 'Quinta de Tilcoco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06115', '06', 'Rengo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06116', '06', 'Requínoa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06117', '06', 'San Vicente');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06201', '06', 'Pichilemu');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06202', '06', 'La Estrella');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06203', '06', 'Litueche');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06204', '06', 'Marchihue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06205', '06', 'Navidad');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06206', '06', 'Paredones');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06301', '06', 'San Fernando');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06302', '06', 'Chépica');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06303', '06', 'Chimbarongo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06304', '06', 'Lolol');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06305', '06', 'Nancagua');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06306', '06', 'Palmilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06307', '06', 'Peralillo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06308', '06', 'Placilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06309', '06', 'Pumanque');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '06310', '06', 'Santa Cruz');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07101', '07', 'Talca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07102', '07', 'Constitución');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07103', '07', 'Curepto');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07104', '07', 'Empedrado');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07105', '07', 'Maule');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07106', '07', 'Pelarco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07107', '07', 'Pencahue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07108', '07', 'Río Claro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07109', '07', 'San Clemente');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07110', '07', 'San Rafael');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07201', '07', 'Cauquenes');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07202', '07', 'Chanco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07203', '07', 'Pelluhue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07301', '07', 'Curicó');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07302', '07', 'Hualañé');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07303', '07', 'Licantén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07304', '07', 'Molina');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07305', '07', 'Rauco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07306', '07', 'Romeral');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07307', '07', 'Sagrada Familia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07308', '07', 'Teno');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07309', '07', 'Vichuquén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07401', '07', 'Linares');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07402', '07', 'Colbún');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07403', '07', 'Longaví');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07404', '07', 'Parral');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07405', '07', 'Retiro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07406', '07', 'San Javier');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07407', '07', 'Villa Alegre');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '07408', '07', 'Yerbas Buenas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08101', '08', 'Concepción');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08102', '08', 'Coronel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08103', '08', 'Chiguayante');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08104', '08', 'Florida');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08105', '08', 'Hualqui');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08106', '08', 'Lota');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08107', '08', 'Penco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08108', '08', 'San Pedro de la Paz');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08109', '08', 'Santa Juana');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08110', '08', 'Talcahuano');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08111', '08', 'Tomé');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08112', '08', 'Hualpén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08201', '08', 'Lebu');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08202', '08', 'Arauco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08203', '08', 'Cañete');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08204', '08', 'Contulmo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08205', '08', 'Curanilahue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08206', '08', 'Los Alamos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08207', '08', 'Tirúa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08301', '08', 'Los Angeles');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08302', '08', 'Antuco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08303', '08', 'Cabrero');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08304', '08', 'Laja');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08305', '08', 'Mulchén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08306', '08', 'Nacimiento');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08307', '08', 'Negrete');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08308', '08', 'Quilaco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08309', '08', 'Quilleco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08310', '08', 'San Rosendo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08311', '08', 'Santa Bárbara');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08312', '08', 'Tucapel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08313', '08', 'Yumbel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '08314', '08', 'Alto Biobío');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09101', '09', 'Temuco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09102', '09', 'Carahue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09103', '09', 'Cunco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09104', '09', 'Curarrehue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09105', '09', 'Freire');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09106', '09', 'Galvarino');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09107', '09', 'Gorbea');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09108', '09', 'Lautaro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09109', '09', 'Loncoche');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09110', '09', 'Melipeuco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09111', '09', 'Nueva Imperial');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09112', '09', 'Padre Las Casas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09113', '09', 'Perquenco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09114', '09', 'Pitrufquén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09115', '09', 'Pucón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09116', '09', 'Saavedra');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09117', '09', 'Teodoro Schmidt');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09118', '09', 'Toltén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09119', '09', 'Vilcún');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09120', '09', 'Villarrica');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09121', '09', 'Cholchol');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09201', '09', 'Angol');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09202', '09', 'Collipulli');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09203', '09', 'Curacautín');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09204', '09', 'Ercilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09205', '09', 'Lonquimay');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09206', '09', 'Los Sauces');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09207', '09', 'Lumaco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09208', '09', 'Purén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09209', '09', 'Renaico');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09210', '09', 'Traiguén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '09211', '09', 'Victoria');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10101', '10', 'Puerto Montt');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10102', '10', 'Calbuco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10103', '10', 'Cochamó');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10104', '10', 'Fresia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10105', '10', 'Frutillar');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10106', '10', 'Los Muermos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10107', '10', 'Llanquihue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10108', '10', 'Maullín');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10109', '10', 'Puerto Varas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10201', '10', 'Castro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10202', '10', 'Ancud');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10203', '10', 'Chonchi');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10204', '10', 'Curaco de Vélez');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10205', '10', 'Dalcahue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10206', '10', 'Puqueldón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10207', '10', 'Queilén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10208', '10', 'Quellón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10209', '10', 'Quemchi');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10210', '10', 'Quinchao');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10301', '10', 'Osorno');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10302', '10', 'Puerto Octay');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10303', '10', 'Purranque');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10304', '10', 'Puyehue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10305', '10', 'Río Negro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10306', '10', 'San Juan de la Costa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10307', '10', 'San Pablo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10401', '10', 'Chaitén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10402', '10', 'Futaleufú');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10403', '10', 'Hualaihué');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '10404', '10', 'Palena');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11101', '11', 'Coihaique');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11102', '11', 'Lago Verde');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11201', '11', 'Aisén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11202', '11', 'Cisnes');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11203', '11', 'Guaitecas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11301', '11', 'Cochrane');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11302', '11', 'O''Higgins');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11303', '11', 'Tortel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11401', '11', 'Chile Chico');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '11402', '11', 'Río Ibáñez');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12101', '12', 'Punta Arenas');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12102', '12', 'Laguna Blanca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12103', '12', 'Río Verde');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12104', '12', 'San Gregorio');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12201', '12', 'Cabo de Hornos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12301', '12', 'Porvenir');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12302', '12', 'Primavera');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12303', '12', 'Timaukel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12401', '12', 'Natales');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '12402', '12', 'Torres del Paine');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13101', '13', 'Santiago');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13102', '13', 'Cerrillos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13103', '13', 'Cerro Navia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13104', '13', 'Conchalí');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13105', '13', 'El Bosque');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13106', '13', 'Estación Central');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13107', '13', 'Huechuraba');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13108', '13', 'Independencia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13109', '13', 'La Cisterna');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13110', '13', 'La Florida');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13111', '13', 'La Granja');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13112', '13', 'La Pintana');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13113', '13', 'La Reina');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13114', '13', 'Las Condes');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13115', '13', 'Lo Barnechea');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13116', '13', 'Lo Espejo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13117', '13', 'Lo Prado');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13118', '13', 'Macul');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13119', '13', 'Maipú');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13120', '13', 'Ñuñoa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13121', '13', 'Pedro Aguirre Cerda');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13122', '13', 'Peñalolén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13123', '13', 'Providencia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13124', '13', 'Pudahuel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13125', '13', 'Quilicura');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13126', '13', 'Quinta Normal');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13127', '13', 'Recoleta');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13128', '13', 'Renca');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13129', '13', 'San Joaquín');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13130', '13', 'San Miguel');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13131', '13', 'San Ramón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13132', '13', 'Vitacura');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13201', '13', 'Puente Alto');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13202', '13', 'Pirque');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13203', '13', 'San José de Maipo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13301', '13', 'Colina');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13302', '13', 'Lampa');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13303', '13', 'Tiltil');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13401', '13', 'San Bernardo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13402', '13', 'Buin');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13403', '13', 'Calera de Tango');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13404', '13', 'Paine');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13501', '13', 'Melipilla');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13502', '13', 'Alhué');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13503', '13', 'Curacaví');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13504', '13', 'María Pinto');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13505', '13', 'San Pedro');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13601', '13', 'Talagante');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13602', '13', 'El Monte');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13603', '13', 'Isla de Maipo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13604', '13', 'Padre Hurtado');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '13605', '13', 'Peñaflor');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14101', '14', 'Valdivia');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14102', '14', 'Corral');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14103', '14', 'Lanco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14104', '14', 'Los Lagos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14105', '14', 'Máfil');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14106', '14', 'Mariquina');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14107', '14', 'Paillaco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14108', '14', 'Panguipulli');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14201', '14', 'La Unión');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14202', '14', 'Futrono');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14203', '14', 'Lago Ranco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '14204', '14', 'Río Bueno');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '15101', '15', 'Arica');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '15102', '15', 'Camarones');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '15201', '15', 'Putre');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '15202', '15', 'General Lagos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16101', '16', 'Chillán');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16102', '16', 'Bulnes');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16103', '16', 'Chillán Viejo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16104', '16', 'El Carmen');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16105', '16', 'Pemuco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16106', '16', 'Pinto');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16107', '16', 'Quillón');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16108', '16', 'San Ignacio');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16109', '16', 'Yungay');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16201', '16', 'Quirihue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16202', '16', 'Cobquecura');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16203', '16', 'Coelemu');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16204', '16', 'Ninhue');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16205', '16', 'Portezuelo');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16206', '16', 'Ranquil');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16207', '16', 'Treguaco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16301', '16', 'San Carlos');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16302', '16', 'Coihueco');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16303', '16', 'Ñiquén');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16304', '16', 'San Fabián');
INSERT INTO public.comuna_publica VALUES ('SUBDERE_DPA_2018', '16305', '16', 'San Nicolás');


--
-- Data for Name: version_terminos; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000001', 'sintetico-terminos-v0.1', 'terminos', '1519668e1eaa6612ba184a1573db9fa7b95f61d3aab410b4f02028411ec483f2', '2026-10-04 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000002', 'sintetico-privacidad-v0.1', 'privacidad', 'e443086352ba2e1180afdf8521c03565769a583160775d416d807df0df8bcad3', '2026-10-04 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000003', 'sintetico-politica-arrendador-v0.1', 'politica_arrendador', 'cc1690d0d0088a9cd2f1d0ec588862f7c28dfe916350243336340776df290438', '2026-10-04 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000075', 'evaluacion-terminos-20261023-v1', 'terminos', '7340bb37a24678a289443cc36e54bd552228c7f2196b077300bfc1e7aceb6ada', '2026-10-23 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000076', 'evaluacion-privacidad-20261023-v1', 'privacidad', '45dbeaef01b2ae7cc6375b5a8ef6a574f95eab702fbde7f152a742ad154fdd3b', '2026-10-23 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000101', 'espacigo-terminos-20261118-v1', 'terminos', '54a5cf3b3a9193a9d44db9c30c1891daa2c389ca9482f17fb88b0ed0c4f5e30a', '2026-11-18 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000102', 'espacigo-privacidad-20261118-v1', 'privacidad', 'cb83ce4c6219d3d7d8cbc0bfd2430f32f84334e5ec377dfe0994fedd9f76ba74', '2026-11-18 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000103', 'evaluacion-terminos-20261126-v2', 'terminos', 'd952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57', '2026-11-26 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000104', 'espacigo-terminos-20261126-v2', 'terminos', 'b5dc085757725d8bc60a545f98b9b544a1b1cb07c16355df7bb56bac3e274d3c', '2026-11-26 00:00:00+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000105', 'espacigo-terminos-hotusoft-v1-2', 'terminos', '1c17baa937446e5136faa232ae3aaabda8c9579fd4f824791f9ea073cf787d78', '2026-11-28 01:00:55.778678+00');
INSERT INTO public.version_terminos VALUES ('00000000-0000-4000-8000-000000000106', 'espacigo-privacidad-hotusoft-v1-2', 'privacidad', '35f430d2463b6036337caa3baf6520d3a1a19d26262dd8339dcf19ccf0cb5eae', '2026-11-28 01:00:55.778678+00');


--
-- PostgreSQL database dump complete
--

-- Preserve explicit denials for the ACL reconciler and the migration ledger.
REVOKE UPDATE, DELETE ON TABLE public.evento_auditoria_local FROM espacigo_runtime;
REVOKE ALL ON TABLE public.schema_migrations FROM PUBLIC;
REVOKE ALL ON TABLE public.schema_migrations FROM espacigo_runtime;
REVOKE UPDATE, DELETE ON TABLE public.aviso_local_recuperacion_auditoria FROM espacigo_runtime;
REVOKE UPDATE, DELETE ON TABLE public.rol_usuario_evento FROM espacigo_runtime;
REVOKE UPDATE, DELETE ON TABLE public.document_verification_reviewer_grant_event FROM espacigo_runtime;

DO $worker_acl$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_privacy_cleanup_worker') THEN
        GRANT ALL ON FUNCTION public.claim_private_media_cleanup(integer, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT ALL ON FUNCTION public.complete_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT ALL ON FUNCTION public.fail_private_media_cleanup(uuid, bigint, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT ALL ON FUNCTION public.finalize_pending_suppression_cleanup(uuid, timestamptz) TO espacigo_privacy_cleanup_worker;
        GRANT ALL ON FUNCTION public.purge_delivered_identity_session_outbox() TO espacigo_privacy_cleanup_worker;
        GRANT ALL ON FUNCTION public.purge_expired_identity_declaration_revisions() TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(ejecucion_id) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(evidencia_id) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(intentos),UPDATE(intentos) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(disponible_en),UPDATE(disponible_en) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(ultimo_codigo_error),UPDATE(ultimo_codigo_error) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(completada_en),UPDATE(completada_en) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(claim_owner),UPDATE(claim_owner) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(claim_hasta),UPDATE(claim_hasta) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(claim_generacion),UPDATE(claim_generacion) ON TABLE public.baja_archivo_pendiente_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.ejecucion_baja_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(solicitud_id) ON TABLE public.ejecucion_baja_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(estado) ON TABLE public.ejecucion_baja_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(iniciada_en) ON TABLE public.ejecucion_baja_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(archivo_id) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(estado) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(proximo_intento_en) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(limpia_en) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(ultimo_codigo_error) ON TABLE public.espacio_galeria_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(archivo_id) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(mime_type) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(sha256) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(size_bytes) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(estado) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(limpia_en) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(proximo_intento_en) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT UPDATE(ultimo_codigo_error) ON TABLE public.foto_perfil_sintetica_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(propietario_id) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(estado) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(fencing_token) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(lease_until) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(updated_at) ON TABLE public.media_asset TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(media_id) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(variant) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(provider) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(bucket_name) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(object_key) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(generation) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(mime_type) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(size_bytes) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(sha256) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(width) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(height) ON TABLE public.media_variant TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(media_id) ON TABLE public.media_variant_intent TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(variant) ON TABLE public.media_variant_intent TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(object_key) ON TABLE public.media_variant_intent TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(generation) ON TABLE public.media_variant_intent TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(write_complete) ON TABLE public.media_variant_intent TO espacigo_privacy_cleanup_worker;
        GRANT DELETE ON TABLE public.operacion_arriendo_archivo_candidato_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(archivo_id) ON TABLE public.operacion_arriendo_archivo_candidato_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.operacion_arriendo_evidencia_ensayo_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(retirar_archivo_en),UPDATE(retirar_archivo_en) ON TABLE public.operacion_arriendo_evidencia_ensayo_local TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(archivo_retirado_en),UPDATE(archivo_retirado_en) ON TABLE public.operacion_arriendo_evidencia_ensayo_local TO espacigo_privacy_cleanup_worker;
        GRANT DELETE ON TABLE public.verificacion_evidencia_sintetica TO espacigo_privacy_cleanup_worker;
        GRANT SELECT(id) ON TABLE public.verificacion_evidencia_sintetica TO espacigo_privacy_cleanup_worker;
    END IF;
END
$worker_acl$;
