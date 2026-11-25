CREATE TABLE public.mensaje_reserva_comercial (
    secuencia bigint GENERATED ALWAYS AS IDENTITY,
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    autor_id uuid,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    cuerpo text NOT NULL,
    creada_en timestamptz NOT NULL,
    CONSTRAINT mensaje_reserva_comercial_pk PRIMARY KEY (id),
    CONSTRAINT mensaje_reserva_comercial_reserva_secuencia_uq UNIQUE (reserva_id, secuencia),
    CONSTRAINT mensaje_reserva_comercial_idempotency_uq UNIQUE (reserva_id, autor_id, clave_idempotencia),
    CONSTRAINT mensaje_reserva_comercial_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
    CONSTRAINT mensaje_reserva_comercial_autor_fk FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT,
    CONSTRAINT mensaje_reserva_comercial_key_ck CHECK (length(btrim(clave_idempotencia)) BETWEEN 1 AND 200),
    CONSTRAINT mensaje_reserva_comercial_body_ck CHECK (char_length(cuerpo) BETWEEN 1 AND 2000 AND length(btrim(cuerpo)) > 0),
    CONSTRAINT mensaje_reserva_comercial_hash_ck CHECK (octet_length(huella_solicitud) = 32)
);
CREATE INDEX mensaje_reserva_comercial_page_idx ON public.mensaje_reserva_comercial(reserva_id, secuencia DESC);

CREATE TABLE public.reserva_comercial_mensaje_lectura (
    reserva_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    ultima_secuencia_leida bigint NOT NULL,
    actualizada_en timestamptz NOT NULL,
    CONSTRAINT reserva_comercial_mensaje_lectura_pk PRIMARY KEY (reserva_id, usuario_id),
    CONSTRAINT reserva_comercial_mensaje_lectura_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
    CONSTRAINT reserva_comercial_mensaje_lectura_usuario_fk FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT,
    CONSTRAINT reserva_comercial_mensaje_lectura_sequence_ck CHECK (ultima_secuencia_leida > 0)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.mensaje_reserva_comercial, public.reserva_comercial_mensaje_lectura TO espacigo_runtime;
GRANT USAGE, SELECT ON SEQUENCE public.mensaje_reserva_comercial_secuencia_seq TO espacigo_runtime;
