ALTER TABLE public.reserva_arriendo_comercial
  ADD CONSTRAINT reserva_arriendo_comercial_distinct_participants_ck
  CHECK (anfitrion_id <> arrendatario_id) NOT VALID;
ALTER TABLE public.reserva_arriendo_comercial
  VALIDATE CONSTRAINT reserva_arriendo_comercial_distinct_participants_ck;

CREATE TABLE public.resena_reserva_comercial (
    id uuid NOT NULL,
    reserva_id uuid NOT NULL,
    espacio_id uuid NOT NULL,
    autor_id uuid,
    destinatario_tipo text NOT NULL,
    destinatario_id uuid,
    calificacion smallint NOT NULL,
    comentario text NOT NULL DEFAULT '',
    estado text NOT NULL DEFAULT 'publicada',
    creada_en timestamptz NOT NULL,
    retirar_en timestamptz NOT NULL,
    clave_idempotencia text NOT NULL,
    huella_solicitud bytea NOT NULL,
    CONSTRAINT resena_reserva_comercial_pk PRIMARY KEY (id),
    CONSTRAINT resena_reserva_comercial_reserva_autor_uq UNIQUE (reserva_id, autor_id),
    CONSTRAINT resena_reserva_comercial_reserva_fk FOREIGN KEY (reserva_id) REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
    CONSTRAINT resena_reserva_comercial_espacio_fk FOREIGN KEY (espacio_id) REFERENCES public.espacio(id) ON DELETE RESTRICT,
    CONSTRAINT resena_reserva_comercial_autor_fk FOREIGN KEY (autor_id) REFERENCES public.usuario(id) ON DELETE RESTRICT,
    CONSTRAINT resena_reserva_comercial_destinatario_fk FOREIGN KEY (destinatario_id) REFERENCES public.usuario(id) ON DELETE RESTRICT,
    CONSTRAINT resena_reserva_comercial_destino_ck CHECK ((destinatario_tipo = 'espacio' AND destinatario_id IS NULL) OR (destinatario_tipo = 'arrendatario' AND destinatario_id IS NOT NULL)),
    CONSTRAINT resena_reserva_comercial_rating_ck CHECK (calificacion BETWEEN 1 AND 5),
    CONSTRAINT resena_reserva_comercial_state_ck CHECK (estado IN ('publicada','reportada','oculta')),
    CONSTRAINT resena_reserva_comercial_retencion_ck CHECK (retirar_en = creada_en + interval '24 months'),
    CONSTRAINT resena_reserva_comercial_key_ck CHECK (length(btrim(clave_idempotencia)) BETWEEN 8 AND 200),
    CONSTRAINT resena_reserva_comercial_hash_ck CHECK (octet_length(huella_solicitud) = 32),
    CONSTRAINT resena_reserva_comercial_participants_ck CHECK (autor_id IS NULL OR destinatario_id IS NULL OR autor_id <> destinatario_id)
);
CREATE INDEX resena_reserva_comercial_space_visible_idx ON public.resena_reserva_comercial(espacio_id, creada_en DESC) WHERE destinatario_tipo = 'espacio' AND estado IN ('publicada','reportada');
CREATE INDEX resena_reserva_comercial_author_idx ON public.resena_reserva_comercial(autor_id, creada_en DESC) WHERE autor_id IS NOT NULL;
COMMENT ON TABLE public.resena_reserva_comercial IS 'Reviews from paid commercial reservations; participant identifiers are nulled after the 24-month link retention window.';
GRANT SELECT, INSERT, UPDATE, DELETE ON public.resena_reserva_comercial TO espacigo_runtime;
