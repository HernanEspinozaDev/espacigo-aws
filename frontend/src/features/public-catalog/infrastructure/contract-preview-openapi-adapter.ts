import "client-only";

import createClient from "openapi-fetch";
import type { paths } from "@/lib/api/generated/openapi";
import { classifyContractPreviewError } from "../domain/contract-preview-error";
import type {
  RentalContractPreview,
  RentalContractPreviewResult,
} from "../domain/contract-preview";
import type { ContractPreviewPort } from "../application/contract-preview.port";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNumericValue(
  value: unknown,
  positive = false,
): value is number | string {
  if (typeof value === "number") {
    return Number.isSafeInteger(value) && (positive ? value > 0 : value >= 0);
  }
  if (typeof value !== "string" || !/^\d+$/.test(value)) return false;
  return !positive || !/^0+$/.test(value);
}

function isDateTime(value: unknown): value is string {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

function isPreview(
  value: unknown,
  expectedQuoteId: string,
): value is RentalContractPreview {
  if (
    !isRecord(value) ||
    value.state !== "preview" ||
    value.quote_id !== expectedQuoteId ||
    typeof value.space_id !== "string" ||
    typeof value.space_title !== "string" ||
    (value.rate_unit !== "hora" &&
      value.rate_unit !== "dia" &&
      value.rate_unit !== "mes") ||
    !isNumericValue(value.unit_price_clp) ||
    !isNumericValue(value.units, true) ||
    !isNumericValue(value.subtotal_clp) ||
    value.currency !== "CLP" ||
    !isDateTime(value.start_at) ||
    !isDateTime(value.end_at) ||
    Date.parse(value.end_at) <= Date.parse(value.start_at) ||
    typeof value.time_zone !== "string" ||
    !isDateTime(value.generated_at) ||
    !Array.isArray(value.parties) ||
    value.parties.length !== 2 ||
    typeof value.contact_notice !== "string" ||
    value.contact_notice.trim().length === 0
  ) {
    return false;
  }

  try {
    new Intl.DateTimeFormat("es-CL", { timeZone: value.time_zone });
  } catch {
    return false;
  }

  const roles = new Set<string>();
  let selfCount = 0;
  for (const party of value.parties) {
    if (
      !isRecord(party) ||
      (party.role !== "arrendador" && party.role !== "arrendatario") ||
      typeof party.self !== "boolean" ||
      roles.has(party.role)
    ) {
      return false;
    }
    if (party.self) {
      selfCount++;
      if (
        Object.keys(party).sort().join(",") !== "legal_name,role,rut,self" ||
        typeof party.legal_name !== "string" ||
        !party.legal_name.trim() ||
        typeof party.rut !== "string" ||
        !/^••••[0-9kK]{4}$/.test(party.rut)
      )
        return false;
    } else if (Object.keys(party).sort().join(",") !== "role,self")
      return false;
    roles.add(party.role);
  }
  return roles.size === 2 && selfCount === 1;
}

function failure(status: number, body?: unknown): RentalContractPreviewResult {
  return {
    ok: false,
    error: { kind: classifyContractPreviewError(status, body), status },
  };
}

export function createContractPreviewAdapter(
  fetcher: typeof fetch = fetch,
  baseUrl = "/api/backend",
): ContractPreviewPort {
  const client = createClient<paths>({
    baseUrl,
    credentials: "same-origin",
    cache: "no-store",
    fetch: fetcher,
  });

  return {
    async getPreview(quoteId, signal) {
      const { data, error, response } = await client.POST(
        "/rental-quotes/{quoteID}/contract-preview",
        { params: { path: { quoteID: quoteId } }, signal },
      );
      if (!response.ok) return failure(response.status, error);
      if (!data || !isPreview(data.data, quoteId)) return failure(502);
      return { ok: true, value: data.data };
    },
  };
}
