-- Durable, single-use OAuth state and encrypted Mercado Pago seller credentials.
CREATE TABLE public.mercadopago_oauth_state (
    state_hash bytea NOT NULL,
    anfitrion_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    encrypted_code_verifier bytea NOT NULL,
    creada_en timestamptz NOT NULL DEFAULT now(),
    expira_en timestamptz NOT NULL,
    consumida_en timestamptz,
    CONSTRAINT mercadopago_oauth_state_pk PRIMARY KEY (state_hash),
    CONSTRAINT mercadopago_oauth_state_hash_ck CHECK (octet_length(state_hash)=32),
    CONSTRAINT mercadopago_oauth_state_verifier_ck CHECK (octet_length(encrypted_code_verifier)>28),
    CONSTRAINT mercadopago_oauth_state_lifetime_ck CHECK (expira_en>creada_en AND expira_en<=creada_en+interval '15 minutes'),
    CONSTRAINT mercadopago_oauth_state_consumed_ck CHECK (consumida_en IS NULL OR consumida_en>=creada_en)
);
CREATE INDEX mercadopago_oauth_state_expiry_idx ON public.mercadopago_oauth_state(expira_en);

CREATE TABLE public.mercadopago_seller_connection (
    anfitrion_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    seller_id text NOT NULL,
    encrypted_access_token bytea NOT NULL,
    encrypted_refresh_token bytea NOT NULL,
    token_expires_at timestamptz NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    live_mode boolean NOT NULL,
    connection_state text NOT NULL DEFAULT 'active',
    connected_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mercadopago_seller_connection_pk PRIMARY KEY (anfitrion_id),
    CONSTRAINT mercadopago_seller_connection_seller_uk UNIQUE (seller_id),
    CONSTRAINT mercadopago_seller_connection_seller_ck CHECK (seller_id ~ '^[0-9]+$'),
    CONSTRAINT mercadopago_seller_connection_access_ck CHECK (octet_length(encrypted_access_token)>28),
    CONSTRAINT mercadopago_seller_connection_refresh_ck CHECK (octet_length(encrypted_refresh_token)>28),
    CONSTRAINT mercadopago_seller_connection_state_ck CHECK (connection_state IN ('active','reauthorization_required'))
);

REVOKE ALL ON TABLE public.mercadopago_oauth_state, public.mercadopago_seller_connection FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.mercadopago_oauth_state TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.mercadopago_seller_connection TO espacigo_runtime;
    END IF;
END $$;
