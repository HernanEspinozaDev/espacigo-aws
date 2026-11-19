-- Session device metadata and activity index for linked devices (Fase 2)
ALTER TABLE public.sesion
    ADD COLUMN IF NOT EXISTS device_label text,
    ADD COLUMN IF NOT EXISTS ip_masked text,
    ADD COLUMN IF NOT EXISTS user_agent text;

COMMENT ON COLUMN public.sesion.device_label IS
    'Etiqueta descriptiva del dispositivo o cliente (ej. Chrome en Linux, Safari en iPhone).';
COMMENT ON COLUMN public.sesion.ip_masked IS
    'Direccion IP de origen enmascarada para resguardo de privacidad (ej. 190.162.•••.•••).';
COMMENT ON COLUMN public.sesion.user_agent IS
    'Encabezado User-Agent sanitizado del cliente HTTP.';

-- Indice compuesto para listar y filtrar sesiones por cuenta ordenadas por actividad
CREATE INDEX IF NOT EXISTS sesion_usuario_actividad_idx
    ON public.sesion (usuario_id, revocada_en, expira_en, ultima_actividad_en DESC);

CREATE INDEX IF NOT EXISTS sesion_usuario_activa_actividad_idx
    ON public.sesion (usuario_id, expira_en, ultima_actividad_en DESC)
    WHERE revocada_en IS NULL;

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.sesion TO espacigo_runtime;
    END IF;
END $grants$;
