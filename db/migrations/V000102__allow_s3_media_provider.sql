-- Preserve historical storage providers and allow the AWS object store.
ALTER TABLE public.media_variant
    DROP CONSTRAINT media_variant_provider_check,
    ADD CONSTRAINT media_variant_provider_check
        CHECK (provider IN ('gcs', 'filesystem', 's3'));

ALTER TABLE public.media_variant_intent
    DROP CONSTRAINT media_variant_intent_provider_check,
    ADD CONSTRAINT media_variant_intent_provider_check
        CHECK (provider IN ('gcs', 'filesystem', 's3'));
