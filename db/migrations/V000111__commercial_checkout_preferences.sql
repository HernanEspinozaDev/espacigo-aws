-- Durable Checkout Pro preference operations and interval holds. These rows
-- reference only commercial quote snapshots, never local booking-trial data.
CREATE TABLE public.checkout_preferencia_comercial (
    id uuid PRIMARY KEY,
    cotizacion_id uuid NOT NULL REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE CASCADE,
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE CASCADE,
    arrendatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    host_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL,
    fingerprint bytea NOT NULL,
    estado text NOT NULL DEFAULT 'pending',
    base_clp bigint NOT NULL,
    commission_clp bigint NOT NULL,
    iva_clp bigint NOT NULL,
    marketplace_fee_clp bigint NOT NULL,
    test_mode boolean NOT NULL,
    preference_id text,
    init_point text,
    preference_claimed_at timestamptz,
    vence_en timestamptz NOT NULL,
    creada_en timestamptz NOT NULL,
    actualizada_en timestamptz NOT NULL,
    CONSTRAINT checkout_preferencia_comercial_fingerprint_ck CHECK (octet_length(fingerprint)=32),
    CONSTRAINT checkout_preferencia_comercial_key_ck CHECK (char_length(idempotency_key) BETWEEN 1 AND 200),
    CONSTRAINT checkout_preferencia_comercial_estado_ck CHECK (estado IN ('pending','approved','rejected','expired','cancelled','reconciliation_required')),
    CONSTRAINT checkout_preferencia_comercial_amounts_ck CHECK (base_clp > 0 AND commission_clp >= 0 AND iva_clp >= 0 AND marketplace_fee_clp = commission_clp + iva_clp AND marketplace_fee_clp < base_clp),
    CONSTRAINT checkout_preferencia_comercial_provider_ck CHECK ((preference_id IS NULL AND init_point IS NULL) OR (preference_id IS NOT NULL AND init_point IS NOT NULL)),
    CONSTRAINT checkout_preferencia_comercial_dates_ck CHECK (isfinite(vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en) AND vence_en > creada_en AND actualizada_en >= creada_en)
);

-- The operation ID is also the commercial reservation ID so a verified payment
-- can atomically confirm the durable interval hold without touching trial data.
CREATE TABLE public.reserva_arriendo_comercial (
    id uuid PRIMARY KEY,
    operacion_checkout_id uuid NOT NULL UNIQUE REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT,
    cotizacion_id uuid NOT NULL UNIQUE REFERENCES public.cotizacion_arriendo_comercial(id) ON DELETE RESTRICT,
    espacio_id uuid NOT NULL REFERENCES public.espacio(id) ON DELETE RESTRICT,
    anfitrion_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    arrendatario_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    estado text NOT NULL CHECK (estado IN ('pendiente_de_pago','pagada','cancelada_por_pago','vencida_pago','reconciliation_required')),
    tarifa_version integer NOT NULL CHECK (tarifa_version>0),
    modalidad text NOT NULL CHECK (modalidad IN ('hora','dia','mes')),
    precio_unitario_clp bigint NOT NULL CHECK (precio_unitario_clp > 0),
    unidades bigint NOT NULL CHECK (unidades > 0),
    subtotal_clp bigint NOT NULL CHECK (subtotal_clp = precio_unitario_clp * unidades),
    moneda char(3) NOT NULL CHECK (moneda='CLP'),
    inicio timestamptz NOT NULL,
    termino timestamptz NOT NULL CHECK (termino > inicio),
    intervalo tstzrange GENERATED ALWAYS AS (tstzrange(inicio,termino,'[)')) STORED,
    zona_horaria text NOT NULL CHECK (length(btrim(zona_horaria)) BETWEEN 1 AND 100),
    ocupacion_id uuid NOT NULL UNIQUE,
    pago_vence_en timestamptz NOT NULL,
    creada_en timestamptz NOT NULL,
    actualizada_en timestamptz NOT NULL,
    UNIQUE(id,espacio_id,intervalo,ocupacion_id),
    CHECK (isfinite(inicio) AND isfinite(termino) AND isfinite(pago_vence_en) AND isfinite(creada_en) AND isfinite(actualizada_en)),
    CHECK ((estado IN ('pendiente_de_pago','reconciliation_required') AND pago_vence_en IS NOT NULL) OR estado IN ('pagada','cancelada_por_pago','vencida_pago'))
);

REVOKE ALL ON TABLE public.checkout_preferencia_comercial FROM PUBLIC;
REVOKE ALL ON TABLE public.reserva_arriendo_comercial FROM PUBLIC;

-- Add a commercial owner to the shared occupancy ledger. Its GiST exclusion
-- constraint remains the cross-flow authority for local, early and commercial
-- rental intervals.
ALTER TABLE public.ocupacion ADD COLUMN reserva_arriendo_comercial_id uuid;
ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_check;
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_check CHECK (
    tipo IN ('bloqueo_manual','retencion','reserva','retencion_temprana','reserva_temprana','retencion_comercial','reserva_comercial')
);
ALTER TABLE public.ocupacion DROP CONSTRAINT ocupacion_tipo_reserva_ck;
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_tipo_reserva_ck CHECK (
    (tipo='bloqueo_manual' AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NULL AND expira_en IS NULL AND motivo IS NOT NULL AND length(btrim(motivo)) BETWEEN 1 AND 500)
    OR (tipo IN ('retencion','reserva') AND reserva_id IS NOT NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NULL AND (tipo<>'retencion' OR expira_en IS NOT NULL))
    OR (tipo IN ('retencion_temprana','reserva_temprana') AND reserva_id IS NULL AND arriendo_temprano_id IS NOT NULL AND reserva_arriendo_comercial_id IS NULL AND motivo IS NULL AND (tipo<>'retencion_temprana' OR expira_en IS NOT NULL))
    OR (tipo IN ('retencion_comercial','reserva_comercial') AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND reserva_arriendo_comercial_id IS NOT NULL AND motivo IS NULL AND (tipo<>'retencion_comercial' OR expira_en IS NOT NULL))
);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_reserva_comercial_id_uk UNIQUE(reserva_arriendo_comercial_id);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_reserva_comercial_ref_uk UNIQUE(reserva_arriendo_comercial_id,espacio_id,intervalo,id);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_reserva_comercial_expira_ck CHECK (
    tipo NOT IN ('retencion_comercial','reserva_comercial')
    OR (tipo='retencion_comercial' AND expira_en IS NOT NULL AND isfinite(expira_en) AND expira_en<=lower(intervalo))
    OR (tipo='reserva_comercial' AND expira_en IS NULL)
);
ALTER TABLE public.ocupacion ADD CONSTRAINT ocupacion_reserva_comercial_fk
    FOREIGN KEY(reserva_arriendo_comercial_id,espacio_id,intervalo,id)
    REFERENCES public.reserva_arriendo_comercial(id,espacio_id,intervalo,ocupacion_id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE public.reserva_arriendo_comercial ADD CONSTRAINT reserva_arriendo_comercial_ocupacion_fk
    FOREIGN KEY(id,espacio_id,intervalo,ocupacion_id)
    REFERENCES public.ocupacion(reserva_arriendo_comercial_id,espacio_id,intervalo,id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX reserva_arriendo_comercial_anfitrion_idx ON public.reserva_arriendo_comercial(anfitrion_id,actualizada_en DESC,id);
CREATE INDEX reserva_arriendo_comercial_arrendatario_idx ON public.reserva_arriendo_comercial(arrendatario_id,actualizada_en DESC,id);

CREATE UNIQUE INDEX checkout_preferencia_comercial_idempotency_uq
    ON public.checkout_preferencia_comercial(arrendatario_id,cotizacion_id,idempotency_key);
CREATE INDEX checkout_preferencia_comercial_hold_idx
    ON public.checkout_preferencia_comercial(espacio_id,vence_en)
    WHERE estado IN ('pending','reconciliation_required');
CREATE INDEX checkout_preferencia_comercial_external_idx
    ON public.checkout_preferencia_comercial(id) WHERE estado IN ('pending','reconciliation_required');

COMMENT ON TABLE public.checkout_preferencia_comercial IS
    'Durable Mercado Pago Checkout Pro preference, immutable price reference and pending interval hold for commercial quotes.';

DO $grants$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.checkout_preferencia_comercial TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.reserva_arriendo_comercial TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.ocupacion TO espacigo_runtime;
    END IF;
END $grants$;
