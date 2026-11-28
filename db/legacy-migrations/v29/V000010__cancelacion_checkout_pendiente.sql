CREATE TABLE public.checkout_cancelacion_pendiente (
    operacion_id uuid PRIMARY KEY REFERENCES public.checkout_preferencia_comercial(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    seller_id text NOT NULL CHECK (seller_id ~ '^[0-9]{1,64}$'),
    provider_payment_id text CHECK (provider_payment_id ~ '^[0-9]{1,64}$'),
    clave_idempotencia uuid NOT NULL UNIQUE,
    creada_en timestamptz NOT NULL,
    ultimo_intento_en timestamptz,
    resultado text CHECK (resultado IN ('approved', 'cancelled'))
);

CREATE INDEX checkout_cancelacion_pendiente_trabajo_idx
    ON public.checkout_cancelacion_pendiente(ultimo_intento_en NULLS FIRST, operacion_id)
    WHERE resultado IS NULL;

COMMENT ON TABLE public.checkout_cancelacion_pendiente IS
    'Durable Checkout Pro cancellation intent. No provider mutation without a bound payment ID.';

REVOKE ALL ON TABLE public.checkout_cancelacion_pendiente FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON TABLE public.checkout_cancelacion_pendiente TO espacigo_runtime;
