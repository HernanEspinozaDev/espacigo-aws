-- KYB-03: persist the administrator's decision and reason for each uploaded document.
DO $guard$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles
        WHERE rolname = session_user AND rolsuper
    ) AND (
        session_user <> 'espacigo_migrator_login'
        OR current_user <> 'espacigo_schema_owner'
        OR current_schema() <> 'public'
        OR NOT pg_catalog.has_schema_privilege(current_user, 'public', 'CREATE')
    ) THEN
        RAISE EXCEPTION 'V000018 requires the pinned migrator/schema-owner identity and public CREATE';
    END IF;
END
$guard$;

ALTER TABLE public.kyb_verification_document
    ADD COLUMN review_status varchar(16) NOT NULL DEFAULT 'PENDING'
        CHECK (review_status IN ('PENDING','IN_REVIEW','APPROVED','REJECTED')),
    ADD COLUMN rejection_reason varchar(500),
    ADD COLUMN reviewer_id uuid REFERENCES public.usuario(id) ON DELETE RESTRICT,
    ADD COLUMN reviewed_at timestamptz;

-- Preserve decisions made before document-level review was introduced.
UPDATE public.kyb_verification_document d SET
    review_status = v.status,
    rejection_reason = CASE WHEN v.status='REJECTED' THEN v.rejection_reason ELSE NULL END,
    reviewer_id = v.reviewer_id,
    reviewed_at = v.reviewed_at
FROM public.kyb_verifications v
WHERE v.id=d.verification_id AND v.status IN ('IN_REVIEW','APPROVED','REJECTED');

ALTER TABLE public.kyb_verification_document
    ADD CONSTRAINT kyb_verification_document_review_consistency_ck CHECK (
        (review_status IN ('APPROVED','REJECTED') AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
        OR (review_status IN ('PENDING','IN_REVIEW') AND reviewer_id IS NULL AND reviewed_at IS NULL)
    ),
    ADD CONSTRAINT kyb_verification_document_rejection_reason_ck CHECK (
        (review_status='REJECTED' AND rejection_reason IS NOT NULL AND length(btrim(rejection_reason)) BETWEEN 5 AND 500)
        OR (review_status<>'REJECTED' AND rejection_reason IS NULL)
    );

REVOKE ALL ON TABLE public.kyb_verification_document FROM PUBLIC;
DO $runtime$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='espacigo_runtime') THEN
        REVOKE UPDATE ON public.kyb_verification_document FROM espacigo_runtime;
        GRANT UPDATE (review_status,rejection_reason,reviewer_id,reviewed_at)
            ON public.kyb_verification_document TO espacigo_runtime;
    END IF;
END
$runtime$;
