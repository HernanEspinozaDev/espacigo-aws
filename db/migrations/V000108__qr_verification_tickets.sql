-- QR Code verification tickets and trusted devices (Fase 6)
CREATE TABLE public.cuenta_qr_dispositivo (
    account_id uuid NOT NULL,
    enabled_at timestamptz,
    trusted_device_label text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_qr_dispositivo_pk PRIMARY KEY (account_id),
    CONSTRAINT cuenta_qr_dispositivo_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE
);

COMMENT ON TABLE public.cuenta_qr_dispositivo IS
    'Estado y dispositivo de confianza para verificacion mediante codigo QR.';

CREATE TABLE public.cuenta_qr_ticket (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    token_hash text NOT NULL,
    status text NOT NULL,
    device_label text,
    ip_masked text,
    expires_at timestamptz NOT NULL,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_qr_ticket_pk PRIMARY KEY (id),
    CONSTRAINT cuenta_qr_ticket_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE,
    CONSTRAINT cuenta_qr_ticket_token_hash_key UNIQUE (token_hash),
    CONSTRAINT cuenta_qr_ticket_status_ck CHECK (status IN ('pending', 'approved', 'rejected', 'expired'))
);

CREATE INDEX idx_cuenta_qr_ticket_account_created
    ON public.cuenta_qr_ticket (account_id, created_at DESC);

COMMENT ON TABLE public.cuenta_qr_ticket IS
    'Tickets y desafios de verificacion de inicio de sesion mediante codigo QR.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_qr_dispositivo TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_qr_ticket TO espacigo_runtime;
    END IF;
END $grants$;
