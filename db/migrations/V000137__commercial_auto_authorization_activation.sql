-- Enable bounded automatic landlord signature and audit events for auto-confirmed commercial agreements.
ALTER TABLE public.autorizacion_arriendo_comercial_evento
    DROP CONSTRAINT autorizacion_arriendo_comercial_evento_accion_check,
    ADD CONSTRAINT autorizacion_arriendo_comercial_evento_accion_check
        CHECK (accion IN ('activated', 'revoked', 'auto_confirmed')),
    DROP CONSTRAINT autorizacion_arriendo_comercial_even_autorizacion_id_accion_key;

CREATE UNIQUE INDEX autorizacion_arriendo_comercial_evento_lifecycle_uk
    ON public.autorizacion_arriendo_comercial_evento (autorizacion_id, accion)
    WHERE accion IN ('activated', 'revoked');

ALTER TABLE public.contrato_firma_comercial
    DROP CONSTRAINT contrato_firma_comercial_metodo_autenticacion_check,
    ADD CONSTRAINT contrato_firma_comercial_metodo_autenticacion_check
        CHECK (metodo_autenticacion IN ('session_consent', 'email_otp', 'totp', 'external_provider', 'auto_authorization'));
