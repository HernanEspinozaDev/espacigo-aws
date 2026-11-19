-- Phone and SMS/OTP verification per account
CREATE TABLE public.cuenta_verificacion_telefono (
    account_id uuid NOT NULL,
    phone_e164 text NOT NULL,
    verified_at timestamptz,
    otp_hash text,
    otp_expires_at timestamptz,
    otp_attempts integer NOT NULL DEFAULT 0,
    resend_available_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cuenta_verificacion_telefono_pk PRIMARY KEY (account_id),
    CONSTRAINT cuenta_verificacion_telefono_usuario_fk FOREIGN KEY (account_id)
        REFERENCES public.usuario (id) ON DELETE CASCADE,
    CONSTRAINT cuenta_verificacion_telefono_otp_attempts_ck CHECK (otp_attempts >= 0)
);

CREATE INDEX idx_cuenta_verificacion_telefono_phone
    ON public.cuenta_verificacion_telefono (phone_e164);

COMMENT ON TABLE public.cuenta_verificacion_telefono IS
    'Registro y verificacion por SMS/OTP del numero telefonico de la cuenta.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.cuenta_verificacion_telefono TO espacigo_runtime;
    END IF;
END $grants$;
