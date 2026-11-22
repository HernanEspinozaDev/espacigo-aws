-- Support multiple non-contiguous and multi-day intervals per commercial quote
-- and reservation while keeping backward compatibility with single-interval rows.
CREATE TABLE IF NOT EXISTS public.cotizacion_arriendo_tramo (
    id uuid PRIMARY KEY,
    cotizacion_id uuid NOT NULL REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE,
    inicio timestamptz NOT NULL,
    termino timestamptz NOT NULL CHECK (termino > inicio),
    unidades bigint NOT NULL CHECK (unidades > 0),
    orden integer NOT NULL CHECK (orden >= 1),
    UNIQUE (cotizacion_id, orden),
    CHECK (isfinite(inicio) AND isfinite(termino))
);

CREATE INDEX IF NOT EXISTS cotizacion_arriendo_tramo_cotizacion_idx
    ON public.cotizacion_arriendo_tramo (cotizacion_id, orden);

REVOKE ALL ON TABLE public.cotizacion_arriendo_tramo FROM PUBLIC;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cotizacion_arriendo_tramo TO espacigo_runtime;
    END IF;
END $grants$;

-- Backfill one interval row for every existing commercial quote.
INSERT INTO public.cotizacion_arriendo_tramo (id, cotizacion_id, inicio, termino, unidades, orden)
SELECT c.id, c.id, c.inicio, c.termino, c.unidades, 1
FROM public.cotizacion_arriendo_comercial c
WHERE NOT EXISTS (
    SELECT 1 FROM public.cotizacion_arriendo_tramo t WHERE t.cotizacion_id = c.id
);

-- Allow a single commercial reservation to hold/book N non-contiguous occupancy rows.
ALTER TABLE public.ocupacion DROP CONSTRAINT IF EXISTS ocupacion_reserva_comercial_fk;
ALTER TABLE public.reserva_arriendo_comercial DROP CONSTRAINT IF EXISTS reserva_arriendo_comercial_ocupacion_fk;
ALTER TABLE public.ocupacion DROP CONSTRAINT IF EXISTS ocupacion_reserva_comercial_id_uk;

CREATE INDEX IF NOT EXISTS ocupacion_reserva_comercial_id_idx
    ON public.ocupacion (reserva_arriendo_comercial_id)
    WHERE reserva_arriendo_comercial_id IS NOT NULL;
