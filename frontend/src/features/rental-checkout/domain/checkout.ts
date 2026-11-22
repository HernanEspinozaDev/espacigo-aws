import { ApiProtocolError } from "@/lib/api/errors";

export const CHECKOUT_STATUSES = [
  "not_started",
  "pending",
  "approved",
  "rejected",
  "expired",
  "cancelled",
  "reconciliation_required",
] as const;

export type CheckoutStatus = (typeof CHECKOUT_STATUSES)[number];

export interface CheckoutLaunch {
  quoteId: string;
  operationId: string;
  status: "pending";
  initPoint: string;
  expiresAt: string;
}

export interface CheckoutRead {
  quoteId: string;
  operationId: string | null;
  status: CheckoutStatus;
  initPoint: string | null;
  expiresAt?: string | null;
  updatedAt: string | null;
}

const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function dateTime(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) &&
    Number.isFinite(Date.parse(value))
  );
}

export function validatedMercadoPagoUrl(value: unknown): string | null {
  if (typeof value !== "string" || value.length > 4096) return null;
  try {
    const url = new URL(value);
    if (
      !new Set([
        "https://www.mercadopago.cl",
        "https://sandbox.mercadopago.cl",
        "https://www.mercadopago.com",
        "https://sandbox.mercadopago.com",
      ]).has(url.origin) ||
      !url.pathname.startsWith("/checkout/") ||
      url.username ||
      url.password ||
      url.hash
    ) {
      return null;
    }
    return url.toString();
  } catch {
    return null;
  }
}

export function parseCheckoutLaunch(
  value: unknown,
  expectedQuoteId: string,
): CheckoutLaunch {
  if (!record(value) || !record(value.data)) throw new ApiProtocolError();
  const data = value.data;
  const initPoint = validatedMercadoPagoUrl(data.init_point);
  if (
    typeof data.quote_id !== "string" ||
    data.quote_id !== expectedQuoteId ||
    !uuid.test(data.operation_id as string) ||
    data.status !== "pending" ||
    !initPoint ||
    !dateTime(data.expires_at)
  ) {
    throw new ApiProtocolError();
  }
  return {
    quoteId: expectedQuoteId,
    operationId: data.operation_id as string,
    status: "pending",
    initPoint,
    expiresAt: data.expires_at,
  };
}

export function parseCheckoutRead(
  value: unknown,
  expectedQuoteId: string,
): CheckoutRead {
  if (!record(value) || !record(value.data)) throw new ApiProtocolError();
  const data = value.data;
  const initPoint =
    data.init_point === undefined || data.init_point === null
      ? null
      : validatedMercadoPagoUrl(data.init_point);
  if (
    typeof data.quote_id !== "string" ||
    data.quote_id !== expectedQuoteId ||
    !CHECKOUT_STATUSES.includes(data.status as CheckoutStatus) ||
    (data.operation_id !== undefined &&
      data.operation_id !== null &&
      (typeof data.operation_id !== "string" ||
        !uuid.test(data.operation_id))) ||
    (data.updated_at !== undefined &&
      data.updated_at !== null &&
      !dateTime(data.updated_at)) ||
    (data.expires_at !== undefined &&
      data.expires_at !== null &&
      !dateTime(data.expires_at)) ||
    (data.init_point !== undefined && data.init_point !== null && !initPoint)
  ) {
    throw new ApiProtocolError();
  }
  return {
    quoteId: expectedQuoteId,
    operationId:
      typeof data.operation_id === "string" ? data.operation_id : null,
    status: data.status as CheckoutStatus,
    initPoint,
    expiresAt: typeof data.expires_at === "string" ? data.expires_at : null,
    updatedAt: typeof data.updated_at === "string" ? data.updated_at : null,
  };
}
