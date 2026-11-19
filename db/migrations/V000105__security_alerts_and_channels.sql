-- Security alerts and notification channel preferences (Fase 3)
CREATE TABLE public.cuenta_alerta_preferencia (
    account_id uuid NOT NULL,
    app_notifications_enabled boolean NOT NULL DEFAULT true,
    email_alerts_enabled boolean NOT NULL DEFAULT true,
    whatsapp_alerts_enabled boolean NOT NULL DEFAULT false,
    extra_email text,
    extra_email_verified_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_alerta_preferencia_pk PRIMARY KEY (account_id),
    CONSTRAINT cuenta_alerta_preferencia_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

COMMENT ON TABLE public.cuenta_alerta_preferencia IS
    'Preferencias de canales de alerta de seguridad por cuenta.';

CREATE TABLE public.cuenta_alerta_evento (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    event_code text NOT NULL,
    title text NOT NULL,
    device_label text,
    ip_masked text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_alerta_evento_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_alerta_evento_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

CREATE INDEX idx_cuenta_alerta_evento_account_created
    ON public.cuenta_alerta_evento (account_id, created_at DESC);

COMMENT ON TABLE public.cuenta_alerta_evento IS
    'Historial de eventos y alertas criticas de seguridad por cuenta.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_alerta_preferencia TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_alerta_evento TO espacigo_runtime;
    END IF;
END $grants$;
