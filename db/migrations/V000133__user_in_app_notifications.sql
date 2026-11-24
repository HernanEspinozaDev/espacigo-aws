-- In-app notifications for commercial contract signatures and lifecycle events.
CREATE TABLE public.notificacion_usuario (
    id uuid PRIMARY KEY,
    usuario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    tipo text NOT NULL CHECK (char_length(tipo) BETWEEN 1 AND 80),
    titulo text NOT NULL CHECK (char_length(titulo) BETWEEN 1 AND 160),
    mensaje text NOT NULL CHECK (char_length(mensaje) BETWEEN 1 AND 500),
    href text NOT NULL CHECK (char_length(href) BETWEEN 1 AND 300),
    referencia_id uuid,
    leida_en timestamptz,
    creada_en timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notificacion_usuario_lista_idx
    ON public.notificacion_usuario (usuario_id, creada_en DESC, id DESC);

CREATE INDEX notificacion_usuario_no_leida_idx
    ON public.notificacion_usuario (usuario_id)
    WHERE leida_en IS NULL;

CREATE UNIQUE INDEX notificacion_usuario_ref_uk
    ON public.notificacion_usuario (referencia_id, usuario_id, tipo)
    WHERE referencia_id IS NOT NULL;

COMMENT ON TABLE public.notificacion_usuario IS
    'Notificaciones in-app por cuenta para solicitudes de contrato, firmas y eventos de arriendo.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.notificacion_usuario TO espacigo_runtime;
    END IF;
END $grants$;

-- Backfill active pending_landlord and ready_for_payment agreements so existing requests appear in the bell.
INSERT INTO public.notificacion_usuario (id, usuario_id, tipo, titulo, mensaje, href, referencia_id, creada_en)
SELECT
    (
        substr(md5(s.id::text || ':' || s.propietario_id::text || ':request_created'), 1, 8) || '-' ||
        substr(md5(s.id::text || ':' || s.propietario_id::text || ':request_created'), 9, 4) || '-' ||
        '4' || substr(md5(s.id::text || ':' || s.propietario_id::text || ':request_created'), 14, 3) || '-' ||
        '8' || substr(md5(s.id::text || ':' || s.propietario_id::text || ':request_created'), 18, 3) || '-' ||
        substr(md5(s.id::text || ':' || s.propietario_id::text || ':request_created'), 21, 12)
    )::uuid,
    s.propietario_id,
    'request_created',
    'Nueva solicitud de contrato',
    'Tienes un contrato firmado por el arrendatario esperando tu firma.',
    '/reservas/contratos?rol=anfitrion&solicitud=' || s.id::text,
    s.id,
    s.creada_en
FROM public.contrato_solicitud_comercial s
WHERE s.estado = 'pending_landlord'
ON CONFLICT DO NOTHING;

INSERT INTO public.notificacion_usuario (id, usuario_id, tipo, titulo, mensaje, href, referencia_id, creada_en)
SELECT
    (
        substr(md5(s.id::text || ':' || s.arrendatario_id::text || ':landlord_signed'), 1, 8) || '-' ||
        substr(md5(s.id::text || ':' || s.arrendatario_id::text || ':landlord_signed'), 9, 4) || '-' ||
        '4' || substr(md5(s.id::text || ':' || s.arrendatario_id::text || ':landlord_signed'), 14, 3) || '-' ||
        '8' || substr(md5(s.id::text || ':' || s.arrendatario_id::text || ':landlord_signed'), 18, 3) || '-' ||
        substr(md5(s.id::text || ':' || s.arrendatario_id::text || ':landlord_signed'), 21, 12)
    )::uuid,
    s.arrendatario_id,
    'landlord_signed',
    'Contrato firmado por el anfitrión',
    'El anfitrión firmó tu solicitud. Ya puedes continuar al pago.',
    '/reservas/contratos/' || s.id::text,
    s.id,
    s.actualizada_en
FROM public.contrato_solicitud_comercial s
WHERE s.estado = 'ready_for_payment'
ON CONFLICT DO NOTHING;
