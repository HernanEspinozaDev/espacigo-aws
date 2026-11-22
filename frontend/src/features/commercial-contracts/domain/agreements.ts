import { ApiProtocolError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";
import { validContractSections } from "@/features/host-listings/domain/contract-template";

export type ContractProposal =
  components["schemas"]["CommercialContractProposal"];
export type CommercialAgreement = components["schemas"]["CommercialAgreement"];
export interface AgreementRead {
  agreement: CommercialAgreement;
  etag: string;
}
export const agreementConsentVersion = "commercial-agreement-v1" as const;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
const hash = /^[a-f0-9]{64}$/u;
const record = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const positive = (v: unknown): v is number =>
  Number.isSafeInteger(v) && Number(v) > 0;
const date = (v: unknown): v is string =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const amount = (v: unknown) =>
  (typeof v === "number" && Number.isSafeInteger(v) && v > 0) ||
  (typeof v === "string" && /^[1-9][0-9]*$/u.test(v));

export function parseProposal(
  value: unknown,
  expected: { quoteId?: string; proposalId?: string } = {},
): ContractProposal {
  if (
    !record(value) ||
    !uuid.test(String(value.id)) ||
    !uuid.test(String(value.quote_id)) ||
    !uuid.test(String(value.space_id)) ||
    !uuid.test(String(value.template_id)) ||
    (expected.quoteId !== undefined && value.quote_id !== expected.quoteId) ||
    (expected.proposalId !== undefined && value.id !== expected.proposalId) ||
    !positive(value.version) ||
    !positive(value.template_version) ||
    !hash.test(String(value.sha256)) ||
    typeof value.space_title !== "string" ||
    !record(value.location) ||
    typeof value.location.address !== "string" ||
    !Number.isFinite(value.location.latitude) ||
    !Number.isFinite(value.location.longitude) ||
    Number(value.location.latitude) < -90 ||
    Number(value.location.latitude) > 90 ||
    Number(value.location.longitude) < -180 ||
    Number(value.location.longitude) > 180 ||
    !Number.isSafeInteger(value.location.revision) ||
    Number(value.location.revision) < 0 ||
    !validContractSections(value.sections) ||
    !amount(value.unit_price_clp) ||
    !amount(value.subtotal_clp) ||
    !amount(value.units) ||
    value.currency !== "CLP" ||
    !["hora", "dia", "mes"].includes(String(value.rate_unit)) ||
    !positive(value.rate_version) ||
    !date(value.start_at) ||
    !date(value.end_at) ||
    Date.parse(value.end_at) <= Date.parse(value.start_at) ||
    !date(value.generated_at) ||
    typeof value.time_zone !== "string" ||
    !record(value.cancellation_policy) ||
    typeof value.cancellation_policy.version !== "string" ||
    typeof value.cancellation_policy.text !== "string" ||
    !value.cancellation_policy.text ||
    typeof value.contact_notice !== "string" ||
    !value.contact_notice ||
    !Array.isArray(value.parties) ||
    value.parties.length !== 2
  )
    throw new ApiProtocolError();
  const roles = new Set<string>();
  let selves = 0;
  for (const p of value.parties) {
    if (
      !record(p) ||
      !["arrendador", "arrendatario"].includes(String(p.role)) ||
      roles.has(String(p.role)) ||
      typeof p.self !== "boolean"
    )
      throw new ApiProtocolError();
    roles.add(String(p.role));
    if (p.self) {
      selves++;
      if (
        Object.keys(p).sort().join(",") !== "legal_name,role,rut,self" ||
        typeof p.legal_name !== "string" ||
        !p.legal_name.trim() ||
        typeof p.rut !== "string" ||
        !/^••••[0-9kK]{4}$/u.test(p.rut)
      )
        throw new ApiProtocolError();
    } else if (Object.keys(p).sort().join(",") !== "role,self")
      throw new ApiProtocolError();
  }
  if (selves !== 1) throw new ApiProtocolError();
  try {
    new Intl.DateTimeFormat("es-CL", { timeZone: value.time_zone });
  } catch {
    throw new ApiProtocolError();
  }
  return value as unknown as ContractProposal;
}

export function parseAgreement(
  value: unknown,
  expectedId?: string,
): CommercialAgreement {
  if (
    !record(value) ||
    !uuid.test(String(value.id)) ||
    (expectedId !== undefined && value.id !== expectedId) ||
    !uuid.test(String(value.proposal_id)) ||
    !uuid.test(String(value.quote_id)) ||
    !uuid.test(String(value.space_id)) ||
    !hash.test(String(value.snapshot_sha256)) ||
    !positive(value.proposal_version) ||
    !positive(value.revision) ||
    ![
      "pending_landlord",
      "ready_for_payment",
      "rejected",
      "withdrawn",
      "expired",
    ].includes(String(value.state)) ||
    !date(value.expires_at) ||
    !date(value.created_at) ||
    !date(value.updated_at) ||
    (value.payment_expires_at !== null && !date(value.payment_expires_at)) ||
    !Array.isArray(value.signatures)
  )
    throw new ApiProtocolError();
  const roles = new Set<string>();
  for (const s of value.signatures) {
    if (
      !record(s) ||
      !["arrendador", "arrendatario"].includes(String(s.role)) ||
      roles.has(String(s.role)) ||
      !date(s.signed_at) ||
      s.snapshot_sha256 !== value.snapshot_sha256 ||
      s.proposal_version !== value.proposal_version ||
      s.consent_version !== agreementConsentVersion
    )
      throw new ApiProtocolError();
    roles.add(String(s.role));
  }
  if (
    !roles.has("arrendatario") ||
    (value.state === "ready_for_payment" &&
      (!roles.has("arrendador") || !date(value.payment_expires_at)))
  )
    throw new ApiProtocolError();
  return value as unknown as CommercialAgreement;
}
export function agreementEtag(agreement: CommercialAgreement): string {
  return `"commercial-agreement-${agreement.id}-${agreement.revision}"`;
}
export function canPayAgreement(
  agreement: CommercialAgreement,
  now = Date.now(),
): boolean {
  return (
    agreement.state === "ready_for_payment" &&
    agreement.signatures.length === 2 &&
    agreement.payment_expires_at !== null &&
    Date.parse(agreement.payment_expires_at) > now
  );
}
export function agreementStateLabel(
  state: CommercialAgreement["state"],
): string {
  return {
    pending_landlord: "Esperando la firma del anfitrión",
    ready_for_payment: "Firmado por ambas partes",
    rejected: "Solicitud rechazada",
    withdrawn: "Solicitud retirada",
    expired: "Solicitud vencida",
  }[state];
}
