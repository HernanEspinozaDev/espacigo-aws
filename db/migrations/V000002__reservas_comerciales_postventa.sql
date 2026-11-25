ALTER TABLE public.espacio
  DROP CONSTRAINT IF EXISTS espacio_politica_cancelacion_version_check;
ALTER TABLE public.espacio
  ADD CONSTRAINT espacio_politica_cancelacion_version_check
  CHECK (politica_cancelacion_version = ANY (ARRAY['facil_v1','flexible_v1','moderada_v1','estricta_v1']));

ALTER TABLE public.cotizacion_arriendo_comercial
  DROP CONSTRAINT IF EXISTS cotizacion_comercial_cancelacion_snapshot_ck;
ALTER TABLE public.cotizacion_arriendo_comercial
  ADD CONSTRAINT cotizacion_comercial_cancelacion_snapshot_ck
  CHECK (((politica_cancelacion_version IS NULL AND politica_cancelacion_texto IS NULL) OR
    (politica_cancelacion_version IS NOT NULL AND
     politica_cancelacion_version = ANY (ARRAY['facil_v1','flexible_v1','moderada_v1','estricta_v1']) AND
     politica_cancelacion_texto IS NOT NULL AND length(btrim(politica_cancelacion_texto)) BETWEEN 1 AND 500)));

ALTER TABLE public.reserva_arriendo_comercial
  DROP CONSTRAINT IF EXISTS reserva_arriendo_comercial_estado_check;
ALTER TABLE public.reserva_arriendo_comercial
  ADD CONSTRAINT reserva_arriendo_comercial_estado_check
  CHECK (estado = ANY (ARRAY['pendiente_de_pago','pagada','cancelada_por_pago','vencida_pago',
    'reconciliation_required','cancelada','reembolsada']));

ALTER TABLE public.reserva_arriendo_comercial
  DROP CONSTRAINT IF EXISTS reserva_arriendo_comercial_check3;
ALTER TABLE public.reserva_arriendo_comercial
  ADD CONSTRAINT reserva_arriendo_comercial_check3
  CHECK (((estado = ANY (ARRAY['pendiente_de_pago','reconciliation_required']) AND pago_vence_en IS NOT NULL) OR
    estado = ANY (ARRAY['pagada','cancelada_por_pago','vencida_pago','cancelada','reembolsada'])));

CREATE TABLE public.reserva_comercial_cancelacion (
  id uuid PRIMARY KEY,
  reserva_id uuid NOT NULL UNIQUE REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
  actor_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
  rol_actor text NOT NULL CHECK (rol_actor IN ('arrendatario','anfitrion')),
  politica_version text NOT NULL CHECK (politica_version IN ('facil_v1','flexible_v1','moderada_v1','estricta_v1','sin_snapshot_v1')),
  politica_texto text NOT NULL CHECK (length(btrim(politica_texto)) BETWEEN 1 AND 500),
  motivo text NOT NULL CHECK (length(btrim(motivo)) BETWEEN 1 AND 500),
  porcentaje_reembolso smallint NOT NULL CHECK (porcentaje_reembolso BETWEEN 0 AND 100),
  monto_total_clp bigint NOT NULL CHECK (monto_total_clp > 0),
  monto_reembolso_clp bigint NOT NULL CHECK (monto_reembolso_clp BETWEEN 0 AND monto_total_clp),
  clave_idempotencia text NOT NULL UNIQUE CHECK (length(clave_idempotencia) BETWEEN 1 AND 200),
  provider_refund_id text CHECK (provider_refund_id IS NULL OR provider_refund_id ~ '^[A-Za-z0-9_-]{1,100}$'),
  refund_status text NOT NULL CHECK (refund_status IN ('pendiente','aprobado','fallido','no_aplica')),
  intentos integer NOT NULL DEFAULT 0 CHECK (intentos >= 0),
  creada_en timestamptz NOT NULL,
  actualizada_en timestamptz NOT NULL,
  resuelta_en timestamptz,
  CHECK (isfinite(creada_en) AND isfinite(actualizada_en) AND (resuelta_en IS NULL OR isfinite(resuelta_en))),
  CHECK ((refund_status IN ('pendiente','fallido') AND resuelta_en IS NULL) OR
         (refund_status IN ('aprobado','no_aplica') AND resuelta_en IS NOT NULL)),
  CHECK ((monto_reembolso_clp = 0 AND refund_status IN ('no_aplica','pendiente','fallido')) OR
         (monto_reembolso_clp > 0 AND refund_status IN ('pendiente','fallido','aprobado')))
);

CREATE INDEX reserva_comercial_cancelacion_pendiente_idx
  ON public.reserva_comercial_cancelacion(actualizada_en)
  WHERE refund_status = 'pendiente';

COMMENT ON TABLE public.reserva_comercial_cancelacion IS
  'Durable cancellation and refund intent for commercial reservations; retries reuse clave_idempotencia.';

GRANT SELECT, INSERT, UPDATE (clave_idempotencia, provider_refund_id, refund_status, intentos, actualizada_en, resuelta_en)
  ON public.reserva_comercial_cancelacion TO espacigo_runtime;
