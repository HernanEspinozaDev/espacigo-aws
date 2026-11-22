-- Per-interval usage / check-in tracking for multi-slot and multi-day
-- commercial reservations so hosts and renters can track each session
-- independently under a single master payment and invoice.
CREATE TABLE IF NOT EXISTS public.reserva_arriendo_tramo_estado (
    reserva_id uuid NOT NULL REFERENCES public.reserva_arriendo_comercial(id) ON DELETE CASCADE,
    tramo_id uuid NOT NULL REFERENCES public.cotizacion_arriendo_tramo(id) ON DELETE CASCADE,
    estado text NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente', 'en_uso', 'completado', 'no_asistio')),
    check_in_en timestamptz,
    check_out_en timestamptz,
    actualizado_por uuid REFERENCES public.usuario(id) ON DELETE SET NULL,
    actualizada_en timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (reserva_id, tramo_id)
);

CREATE INDEX IF NOT EXISTS reserva_arriendo_tramo_estado_reserva_idx
    ON public.reserva_arriendo_tramo_estado (reserva_id);

REVOKE ALL ON TABLE public.reserva_arriendo_tramo_estado FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.reserva_arriendo_tramo_estado TO espacigo_runtime;
    END IF;
END $grants$;
