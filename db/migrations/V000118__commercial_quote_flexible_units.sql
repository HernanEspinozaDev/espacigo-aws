-- Allow multi-slot and multi-day commercial quotes to store any valid unit count
-- (up to 168 hours, 64 days, or 12 months) in public.cotizacion_arriendo_comercial.
ALTER TABLE public.cotizacion_arriendo_comercial
    DROP CONSTRAINT IF EXISTS cotizacion_arriendo_comercial_unidades_ck;

ALTER TABLE public.cotizacion_arriendo_comercial
    ADD CONSTRAINT cotizacion_arriendo_comercial_unidades_ck
        CHECK (
            (modalidad = 'hora' AND unidades BETWEEN 1 AND 168) OR
            (modalidad = 'dia' AND unidades BETWEEN 1 AND 64) OR
            (modalidad = 'mes' AND unidades BETWEEN 1 AND 12)
        );
