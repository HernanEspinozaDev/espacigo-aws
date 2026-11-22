-- Add monthly space availability rules (start date + optional minimum months)
-- and monthly billing mode + installment schedule tables without modifying
-- historical V98/V97 tables.

CREATE TABLE IF NOT EXISTS public.espacio_regla_mensual (
    espacio_id uuid PRIMARY KEY REFERENCES public.espacio(id) ON DELETE CASCADE,
    disponible_desde date NULL,
    estadia_minima_meses smallint NOT NULL DEFAULT 1 CHECK (estadia_minima_meses BETWEEN 1 AND 24),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.cotizacion_modalidad_cobro (
    cotizacion_id uuid PRIMARY KEY REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE,
    modalidad_cobro text NOT NULL DEFAULT 'full' CHECK (modalidad_cobro IN ('full', 'monthly_installments')),
    creado_en timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.reserva_cuota_mensual (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reserva_arriendo_comercial_id uuid NOT NULL REFERENCES public.reserva_arriendo_comercial(id) ON DELETE CASCADE,
    numero_cuota integer NOT NULL CHECK (numero_cuota >= 1),
    periodo_inicio timestamptz NOT NULL,
    periodo_termino timestamptz NOT NULL CHECK (periodo_termino > periodo_inicio),
    monto_clp bigint NOT NULL CHECK (monto_clp > 0),
    estado text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente', 'pagada', 'vencida', 'cancelada')),
    pagada_en timestamptz NULL,
    creado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (reserva_arriendo_comercial_id, numero_cuota),
    CHECK (isfinite(periodo_inicio) AND isfinite(periodo_termino))
);

CREATE INDEX IF NOT EXISTS reserva_cuota_mensual_reserva_idx
    ON public.reserva_cuota_mensual (reserva_arriendo_comercial_id, numero_cuota);

REVOKE ALL ON TABLE public.espacio_regla_mensual FROM PUBLIC;
REVOKE ALL ON TABLE public.cotizacion_modalidad_cobro FROM PUBLIC;
REVOKE ALL ON TABLE public.reserva_cuota_mensual FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.espacio_regla_mensual TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cotizacion_modalidad_cobro TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.reserva_cuota_mensual TO espacigo_runtime;
    END IF;
END $grants$;
