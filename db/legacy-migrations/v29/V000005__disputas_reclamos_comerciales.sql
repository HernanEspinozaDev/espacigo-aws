CREATE TABLE public.caso_soporte_reserva_comercial (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reserva_id uuid NOT NULL REFERENCES public.reserva_arriendo_comercial(id),
    iniciador_id uuid NOT NULL REFERENCES public.usuario(id),
    rol_iniciador text NOT NULL CHECK (rol_iniciador IN ('anfitrion','arrendatario')),
    categoria text NOT NULL CHECK (categoria IN ('problemas_acceso','condiciones_diferentes','incumplimiento_normas','espacio_no_disponible','problema_pago_reembolso','mediacion_directa','dano_inmueble')),
    enrutado_a text NOT NULL CHECK (enrutado_a IN ('contraparte','espacigo')),
    estado text NOT NULL CHECK (estado IN ('esperando_contraparte','en_mediacion_espacigo','resuelto','cerrado')),
    descripcion text NOT NULL CHECK (char_length(descripcion) BETWEEN 1 AND 3000),
    clave_idempotencia text NOT NULL CHECK (char_length(clave_idempotencia) BETWEEN 1 AND 128),
    plazo_descargo timestamptz,
    creada_en timestamptz NOT NULL,
    actualizada_en timestamptz NOT NULL,
    UNIQUE (reserva_id, iniciador_id, clave_idempotencia),
    CHECK ((categoria = 'dano_inmueble') = (plazo_descargo IS NOT NULL)),
    CHECK (categoria <> 'dano_inmueble' OR (rol_iniciador = 'anfitrion' AND enrutado_a = 'espacigo'))
);

CREATE UNIQUE INDEX caso_soporte_dano_unico ON public.caso_soporte_reserva_comercial (reserva_id) WHERE categoria = 'dano_inmueble';
CREATE INDEX caso_soporte_reserva_idx ON public.caso_soporte_reserva_comercial (reserva_id, creada_en DESC, id);
CREATE INDEX caso_soporte_mediacion_idx ON public.caso_soporte_reserva_comercial (creada_en DESC, id) WHERE enrutado_a = 'espacigo';

CREATE TABLE public.evento_caso_soporte_comercial (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    caso_id uuid NOT NULL REFERENCES public.caso_soporte_reserva_comercial(id),
    actor_id uuid NOT NULL REFERENCES public.usuario(id),
    accion text NOT NULL CHECK (accion IN ('abrir','responder','escalar','descargo','resolver','cerrar')),
    descripcion text NOT NULL CHECK (char_length(descripcion) <= 3000),
    clave_idempotencia text NOT NULL CHECK (char_length(clave_idempotencia) BETWEEN 1 AND 128),
    creada_en timestamptz NOT NULL,
    UNIQUE (caso_id, actor_id, clave_idempotencia)
);

CREATE UNIQUE INDEX caso_soporte_descargo_unico ON public.evento_caso_soporte_comercial (caso_id) WHERE accion = 'descargo';

CREATE TABLE public.evidencia_caso_soporte_comercial (
    id uuid PRIMARY KEY,
    caso_id uuid NOT NULL REFERENCES public.caso_soporte_reserva_comercial(id),
    autor_id uuid NOT NULL REFERENCES public.usuario(id),
    mime text NOT NULL CHECK (mime IN ('image/jpeg','image/png','application/pdf')),
    tamano_bytes integer NOT NULL CHECK (tamano_bytes BETWEEN 1 AND 5242880),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    creada_en timestamptz NOT NULL,
    UNIQUE (caso_id, autor_id, sha256)
);

GRANT SELECT, INSERT, UPDATE (enrutado_a, estado, actualizada_en) ON public.caso_soporte_reserva_comercial TO espacigo_runtime;
GRANT SELECT, INSERT ON public.evento_caso_soporte_comercial, public.evidencia_caso_soporte_comercial TO espacigo_runtime;
