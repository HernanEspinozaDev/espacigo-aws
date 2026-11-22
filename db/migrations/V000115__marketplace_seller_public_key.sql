-- Seller Public Key returned by OAuth. Public by design; access and refresh
-- tokens remain encrypted. Existing connections stay NULL and must reconnect
-- before the opt-in seller-key Checkout API test path can be used.
ALTER TABLE public.mercadopago_seller_connection
    ADD COLUMN public_key text;

ALTER TABLE public.mercadopago_seller_connection
    ADD CONSTRAINT mercadopago_seller_connection_public_key_ck
    CHECK (public_key IS NULL OR public_key ~ '^(TEST-|APP_USR-)[A-Za-z0-9_-]{16,120}$');
