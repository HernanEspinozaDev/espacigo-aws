-- Add short pass codes per reservation interval for QR and manual reception
-- check-in/check-out validation, and backfill paid commercial reservations.
ALTER TABLE public.reserva_arriendo_tramo_estado
    ADD COLUMN IF NOT EXISTS codigo_corto text;

CREATE INDEX IF NOT EXISTS reserva_arriendo_tramo_estado_codigo_idx
    ON public.reserva_arriendo_tramo_estado (codigo_corto)
    WHERE codigo_corto IS NOT NULL;

-- Backfill pass rows with deterministic 6-char codes for existing paid reservations.
INSERT INTO public.reserva_arriendo_tramo_estado (reserva_id, tramo_id, estado, codigo_corto)
SELECT r.id,
       t.id,
       'pendiente',
       upper(substr(md5(r.id::text || ':' || t.id::text), 1, 6))
FROM public.reserva_arriendo_comercial r
JOIN public.cotizacion_arriendo_tramo t ON t.cotizacion_id = r.cotizacion_id
WHERE r.estado = 'pagada'
ON CONFLICT (reserva_id, tramo_id) DO UPDATE
SET codigo_corto = COALESCE(
    public.reserva_arriendo_tramo_estado.codigo_corto,
    upper(substr(md5(public.reserva_arriendo_tramo_estado.reserva_id::text || ':' || public.reserva_arriendo_tramo_estado.tramo_id::text), 1, 6))
);
