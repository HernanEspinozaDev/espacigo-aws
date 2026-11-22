-- Extend commercial checkout operations with double-sided marketplace
-- commissions (3% + 19% IVA on host and 3% + 19% IVA on buyer).
-- Historical rows keep buyer fees at 0 and backfill total_charged_clp = base_clp.
ALTER TABLE public.checkout_preferencia_comercial
    ADD COLUMN IF NOT EXISTS buyer_commission_clp bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS buyer_iva_clp bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS buyer_fee_clp bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_charged_clp bigint NOT NULL DEFAULT 0;

UPDATE public.checkout_preferencia_comercial
SET total_charged_clp = base_clp
WHERE total_charged_clp = 0;

ALTER TABLE public.checkout_preferencia_comercial
    DROP CONSTRAINT checkout_preferencia_comercial_amounts_ck;

ALTER TABLE public.checkout_preferencia_comercial
    ADD CONSTRAINT checkout_preferencia_comercial_amounts_ck CHECK (
        base_clp > 0
        AND commission_clp >= 0
        AND iva_clp >= 0
        AND buyer_commission_clp >= 0
        AND buyer_iva_clp >= 0
        AND buyer_fee_clp = buyer_commission_clp + buyer_iva_clp
        AND total_charged_clp = base_clp + buyer_fee_clp
        AND marketplace_fee_clp = commission_clp + iva_clp + buyer_fee_clp
        AND marketplace_fee_clp < total_charged_clp
    );
