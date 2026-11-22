import type { components } from "@/lib/api/generated/openapi";

export type CommercialAuthorization =
  components["schemas"]["CommercialAuthorizationGrant"];
export type CommercialAuthorizationEvent =
  components["schemas"]["CommercialAuthorizationEvent"];
export type AuthorizationActivation =
  components["schemas"]["CommercialAuthorizationActivation"];

export function isCommercialAuthorization(
  value: unknown,
): value is CommercialAuthorization {
  if (!value || typeof value !== "object") return false;
  const item = value as CommercialAuthorization;
  return (
    typeof item.id === "string" &&
    typeof item.space_id === "string" &&
    Number.isSafeInteger(item.template_version) &&
    item.template_version > 0 &&
    Number.isSafeInteger(item.max_amount_clp) &&
    item.max_amount_clp > 0 &&
    typeof item.valid_from === "string" &&
    Number.isFinite(Date.parse(item.valid_from)) &&
    typeof item.expires_at === "string" &&
    Number.isFinite(Date.parse(item.expires_at)) &&
    typeof item.created_at === "string" &&
    Number.isFinite(Date.parse(item.created_at))
  );
}

export function validAuthorizationAmount(value: string): number | null {
  if (!/^[1-9][0-9]{0,8}$/.test(value)) return null;
  const amount = Number(value);
  return Number.isSafeInteger(amount) && amount <= 100_000_000 ? amount : null;
}
