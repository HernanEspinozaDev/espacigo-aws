-- A single, immutable private rental tax document attached by the paid
-- reservation's host. This records a file; it does not issue or validate a DTE.
CREATE TABLE public.reserva_documento_tributario_comercial (
 id uuid PRIMARY KEY,
 reserva_id uuid NOT NULL UNIQUE REFERENCES public.reserva_arriendo_comercial(id) ON DELETE RESTRICT,
 anfitrion_id uuid NOT NULL REFERENCES public.usuario(id) ON DELETE RESTRICT,
 object_id uuid NOT NULL UNIQUE,
 object_key text GENERATED ALWAYS AS ('rental-tax-documents/' || object_id::text || '.blob') STORED,
 bucket_name text NOT NULL CHECK(bucket_name ~ '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$'),
 sha256 bytea NOT NULL CHECK(octet_length(sha256)=32),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 10485760),
 inspected_at timestamptz NOT NULL,
 uploaded_at timestamptz NOT NULL,
 CHECK(isfinite(inspected_at) AND isfinite(uploaded_at))
);
CREATE FUNCTION public.guard_commercial_rental_tax_document() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF TG_OP='UPDATE' OR TG_OP='DELETE' THEN
  RAISE EXCEPTION 'commercial rental tax document is immutable' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS (
  SELECT 1 FROM public.reserva_arriendo_comercial r
  JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
  WHERE r.id=NEW.reserva_id AND r.anfitrion_id=NEW.anfitrion_id
   AND r.estado='pagada' AND c.estado='approved'
   AND c.provider_payment_status='approved' AND c.payment_verified_at IS NOT NULL
   AND c.provider_payment_id IS NOT NULL AND c.base_clp=r.subtotal_clp
   AND r.moneda='CLP'
 ) THEN
  RAISE EXCEPTION 'commercial tax document requires host and verified paid reservation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commercial_rental_tax_document_guard
 BEFORE INSERT OR UPDATE OR DELETE ON public.reserva_documento_tributario_comercial
 FOR EACH ROW EXECUTE FUNCTION public.guard_commercial_rental_tax_document();
REVOKE ALL ON public.reserva_documento_tributario_comercial FROM PUBLIC;
GRANT SELECT,INSERT ON public.reserva_documento_tributario_comercial TO espacigo_runtime;
