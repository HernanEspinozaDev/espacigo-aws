ALTER TABLE public.checkout_preferencia_comercial
    ADD COLUMN provider_settlement jsonb CHECK (jsonb_typeof(provider_settlement) = 'object'),
    ADD COLUMN provider_settlement_attempted_at timestamptz;

CREATE INDEX checkout_comercial_settlement_due_idx
    ON public.checkout_preferencia_comercial (provider_settlement_attempted_at NULLS FIRST, id)
    WHERE estado = 'approved' AND checkout_method = 'checkout_pro' AND provider_payment_id IS NOT NULL;
