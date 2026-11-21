-- Extend the durable commercial checkout operation with an explicit Payments
-- API path. Existing Checkout Pro rows retain the default method unchanged.
ALTER TABLE public.checkout_preferencia_comercial
    ADD COLUMN checkout_method text NOT NULL DEFAULT 'checkout_pro',
    ADD COLUMN payment_create_claimed_at timestamptz,
    ADD CONSTRAINT checkout_preferencia_comercial_method_ck
        CHECK (checkout_method IN ('checkout_pro','payments_api')),
    ADD CONSTRAINT checkout_preferencia_comercial_payment_method_mode_ck
        CHECK (checkout_method <> 'payments_api' OR test_mode),
    ADD CONSTRAINT checkout_preferencia_comercial_payment_claim_ck
        CHECK (payment_create_claimed_at IS NULL OR
               (checkout_method='payments_api' AND isfinite(payment_create_claimed_at))),
    ADD CONSTRAINT checkout_preferencia_comercial_payment_no_preference_ck
        CHECK (checkout_method <> 'payments_api' OR
               (preference_id IS NULL AND init_point IS NULL AND preference_claimed_at IS NULL));

COMMENT ON COLUMN public.checkout_preferencia_comercial.checkout_method IS
    'Durable provider path; existing rows default to checkout_pro and Payments API is test-mode-only.';
COMMENT ON COLUMN public.checkout_preferencia_comercial.payment_create_claimed_at IS
    'One-shot durable claim before Payments API creation; never reset after an uncertain provider outcome.';
