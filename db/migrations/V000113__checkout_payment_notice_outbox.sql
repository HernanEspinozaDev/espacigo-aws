-- Durable, deduplicated email intent for verified commercial Checkout Pro
-- transitions. No email address, provider token, payload or rendered body is
-- stored in this table.
CREATE TABLE public.checkout_payment_notice_outbox (
    id uuid PRIMARY KEY,
    reservation_id uuid NOT NULL REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
    operation_id uuid NOT NULL REFERENCES public.checkout_preferencia_comercial(id) ON DELETE RESTRICT,
    recipient_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
    recipient_role text NOT NULL,
    transition text NOT NULL,
    dedupe_key text NOT NULL UNIQUE,
    state text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL,
    claimed_at timestamptz,
    delivered_at timestamptz,
    terminal_at timestamptz,
    last_error_code text,
    created_at timestamptz NOT NULL,
    CONSTRAINT checkout_payment_notice_role_ck CHECK (recipient_role IN ('arrendatario','anfitrion')),
    CONSTRAINT checkout_payment_notice_transition_ck CHECK (transition IN ('approved','rejected','expired')),
    CONSTRAINT checkout_payment_notice_state_ck CHECK (state IN ('pending','processing','delivered','terminal','cancelled')),
    CONSTRAINT checkout_payment_notice_attempts_ck CHECK (attempts BETWEEN 0 AND 8),
    CONSTRAINT checkout_payment_notice_dedupe_ck CHECK (char_length(dedupe_key) BETWEEN 1 AND 200),
    CONSTRAINT checkout_payment_notice_error_ck CHECK (last_error_code IS NULL OR last_error_code IN ('mail_delivery_failed','recipient_unavailable','attempt_limit_reached')),
    CONSTRAINT checkout_payment_notice_state_time_ck CHECK (
        (state='delivered' AND delivered_at IS NOT NULL AND terminal_at IS NULL) OR
        (state='terminal' AND terminal_at IS NOT NULL AND delivered_at IS NULL) OR
        (state IN ('pending','processing','cancelled') AND delivered_at IS NULL AND terminal_at IS NULL)
    ),
    CONSTRAINT checkout_payment_notice_timestamps_ck CHECK (
        created_at <= available_at AND (claimed_at IS NULL OR claimed_at >= created_at) AND
        (delivered_at IS NULL OR delivered_at >= created_at) AND (terminal_at IS NULL OR terminal_at >= created_at)
    ),
    CONSTRAINT checkout_payment_notice_event_role_ck CHECK (
        (transition='approved' AND recipient_role IN ('arrendatario','anfitrion')) OR
        (transition IN ('rejected','expired') AND recipient_role='arrendatario')
    )
);

CREATE UNIQUE INDEX checkout_payment_notice_transition_recipient_uq
    ON public.checkout_payment_notice_outbox(reservation_id, transition, recipient_id);
CREATE INDEX checkout_payment_notice_due_idx
    ON public.checkout_payment_notice_outbox(available_at, created_at, id)
    WHERE state IN ('pending','processing');

CREATE FUNCTION public.enqueue_checkout_payment_notices(p_operation_id uuid, p_transition text, p_at timestamptz)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE reservation public.reserva_arriendo_comercial%ROWTYPE;
BEGIN
    IF p_transition NOT IN ('approved','rejected','expired') THEN
        RAISE EXCEPTION 'unsupported commercial payment notice transition' USING ERRCODE='22023';
    END IF;
    SELECT * INTO reservation FROM public.reserva_arriendo_comercial
      WHERE operacion_checkout_id=p_operation_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'commercial reservation missing for payment transition' USING ERRCODE='23503';
    END IF;
    IF (p_transition='approved' AND reservation.estado <> 'pagada') OR
       (p_transition='rejected' AND reservation.estado <> 'cancelada_por_pago') OR
       (p_transition='expired' AND reservation.estado <> 'vencida_pago') THEN
        RAISE EXCEPTION 'reservation state does not match payment notice transition' USING ERRCODE='23514';
    END IF;

    IF p_transition='approved' THEN
        INSERT INTO public.checkout_payment_notice_outbox(
            id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
        VALUES
            (gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,
             'checkout-payment:'||p_operation_id::text||':approved:'||reservation.arrendatario_id::text,p_at,p_at),
            (gen_random_uuid(),reservation.id,p_operation_id,reservation.anfitrion_id,'anfitrion',p_transition,
             'checkout-payment:'||p_operation_id::text||':approved:'||reservation.anfitrion_id::text,p_at,p_at)
        ON CONFLICT (dedupe_key) DO NOTHING;
    ELSE
        INSERT INTO public.checkout_payment_notice_outbox(
            id,reservation_id,operation_id,recipient_id,recipient_role,transition,dedupe_key,available_at,created_at)
        VALUES (gen_random_uuid(),reservation.id,p_operation_id,reservation.arrendatario_id,'arrendatario',p_transition,
                'checkout-payment:'||p_operation_id::text||':'||p_transition||':'||reservation.arrendatario_id::text,p_at,p_at)
        ON CONFLICT (dedupe_key) DO NOTHING;
    END IF;
END $$;

REVOKE ALL ON TABLE public.checkout_payment_notice_outbox FROM PUBLIC;
REVOKE ALL ON FUNCTION public.enqueue_checkout_payment_notices(uuid,text,timestamptz) FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        GRANT SELECT, INSERT, UPDATE ON TABLE public.checkout_payment_notice_outbox TO espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.enqueue_checkout_payment_notices(uuid,text,timestamptz) TO espacigo_runtime;
    END IF;
END $$;

COMMENT ON TABLE public.checkout_payment_notice_outbox IS
    'Transactional email intent created only for backend-verified commercial checkout transitions; recipient email is resolved at delivery time.';
