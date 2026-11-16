CREATE TABLE public.espacio_regla_disponibilidad (
    espacio_id uuid PRIMARY KEY REFERENCES public.espacio(id) ON DELETE CASCADE,
    dias_operacion smallint[] NOT NULL DEFAULT '{1,2,3,4,5,6,7}',
    hora_apertura time NOT NULL DEFAULT '09:00',
    hora_cierre time NOT NULL DEFAULT '20:00',
    check_in_hora time DEFAULT '15:00',
    check_out_hora time DEFAULT '12:00',
    tarifa_base_dia integer NOT NULL CHECK (tarifa_base_dia > 0),
    estadia_minima smallint NOT NULL DEFAULT 1,
    estadia_maxima smallint NOT NULL DEFAULT 30,
    actualizado_en timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.espacio_calendario_override (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE CASCADE,
    fecha date NOT NULL,
    estado text NOT NULL CHECK (estado IN ('disponible', 'bloqueado', 'mantenimiento')),
    tarifa_personalizada_clp integer CHECK (tarifa_personalizada_clp > 0),
    motivo text,
    creado_en timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT espacio_calendario_override_uk UNIQUE (espacio_id, fecha)
);

COMMENT ON TABLE public.espacio_regla_disponibilidad IS
    'Reglas base de disponibilidad, horario de operacion y tarifas diarias por espacio.';

COMMENT ON TABLE public.espacio_calendario_override IS
    'Excepciones y tarifas personalizadas por fecha y espacio para el calendario.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.espacio_regla_disponibilidad TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.espacio_calendario_override TO espacigo_runtime;
    END IF;
END $grants$;
