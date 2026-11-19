-- Persist signed payment notifications before acknowledging Mercado Pago.
-- No raw payload, signature, access token or refresh token is stored here.
ALTER TABLE public.checkout_preferencia_comercial
    ADD COLUMN provider_payment_id text,
    ADD COLUMN provider_payment_status text,
    ADD COLUMN provider_payment_updated_at timestamptz,
    ADD COLUMN reconciliation_reason text,
    ADD COLUMN payment_verified_at timestamptz,
    ADD CONSTRAINT checkout_preferencia_comercial_payment_id_ck
        CHECK (provider_payment_id IS NULL OR provider_payment_id ~ '^[0-9]{1,64}$'),
    ADD CONSTRAINT checkout_preferencia_comercial_payment_status_ck
        CHECK (provider_payment_status IS NULL OR provider_payment_status ~ '^[a-z_]{1,40}$'),
    ADD CONSTRAINT checkout_preferencia_comercial_reconciliation_reason_ck
        CHECK (reconciliation_reason IS NULL OR reconciliation_reason ~ '^[a-z_]{1,64}$'),
    ADD CONSTRAINT checkout_preferencia_comercial_payment_fields_ck
        CHECK ((provider_payment_id IS NULL AND payment_verified_at IS NULL) OR
               (provider_payment_id IS NOT NULL AND provider_payment_status IS NOT NULL AND provider_payment_updated_at IS NOT NULL));

CREATE UNIQUE INDEX checkout_preferencia_comercial_payment_id_uq
    ON public.checkout_preferencia_comercial(provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;

CREATE TABLE public.mercadopago_payment_webhook_receipt (
    id uuid PRIMARY KEY,
    topic text NOT NULL,
    action text NOT NULL DEFAULT '',
    provider_payment_id text NOT NULL,
    provider_seller_hint text,
    provider_request_id text NOT NULL,
    signature_timestamp text NOT NULL,
    estado text NOT NULL DEFAULT 'received',
    intentos integer NOT NULL DEFAULT 0,
    reintentar_en timestamptz NOT NULL,
    claimed_at timestamptz,
    last_error_code text,
    provider_status text,
    recibida_en timestamptz NOT NULL,
    procesada_en timestamptz,
    actualizada_en timestamptz NOT NULL,
    CONSTRAINT mercadopago_payment_webhook_topic_ck CHECK (topic = 'payment'),
    CONSTRAINT mercadopago_payment_webhook_id_ck CHECK (provider_payment_id ~ '^[0-9]{1,64}$'),
    CONSTRAINT mercadopago_payment_webhook_seller_ck CHECK (provider_seller_hint IS NULL OR provider_seller_hint ~ '^[0-9]{1,64}$'),
    CONSTRAINT mercadopago_payment_webhook_request_ck CHECK (char_length(provider_request_id) BETWEEN 1 AND 256),
    CONSTRAINT mercadopago_payment_webhook_timestamp_ck CHECK (signature_timestamp ~ '^[0-9]{1,20}$'),
    CONSTRAINT mercadopago_payment_webhook_state_ck CHECK (estado IN ('received','processing','retry','processed','reconciliation_required')),
    CONSTRAINT mercadopago_payment_webhook_attempts_ck CHECK (intentos BETWEEN 0 AND 1000000),
    CONSTRAINT mercadopago_payment_webhook_error_ck CHECK (last_error_code IS NULL OR last_error_code IN (
        'seller_hint_missing','provider_identity_mismatch','payment_requires_reconciliation',
        'seller_token_unavailable','provider_lookup_failed','payment_outcome_unavailable')),
    CONSTRAINT mercadopago_payment_webhook_provider_status_ck CHECK (provider_status IS NULL OR provider_status ~ '^[a-z_]{1,40}$'),
    CONSTRAINT mercadopago_payment_webhook_processed_ck CHECK (estado <> 'processed' OR procesada_en IS NOT NULL),
    CONSTRAINT mercadopago_payment_webhook_request_uq UNIQUE (topic, provider_request_id)
);

CREATE INDEX mercadopago_payment_webhook_retry_idx
    ON public.mercadopago_payment_webhook_receipt(reintentar_en, recibida_en)
    WHERE estado IN ('received','retry');
CREATE INDEX mercadopago_payment_webhook_lease_idx
    ON public.mercadopago_payment_webhook_receipt(claimed_at)
    WHERE estado='processing';

REVOKE ALL ON TABLE public.mercadopago_payment_webhook_receipt FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.mercadopago_payment_webhook_receipt TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.checkout_preferencia_comercial TO espacigo_runtime;

COMMENT ON TABLE public.mercadopago_payment_webhook_receipt IS
    'Minimal durable receipt for signed Mercado Pago payment notification delivery and idempotent reconciliation; excludes raw payloads and credentials.';
