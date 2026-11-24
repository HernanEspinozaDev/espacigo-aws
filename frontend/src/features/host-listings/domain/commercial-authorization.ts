import type { components } from "@/lib/api/generated/openapi";

export type CommercialAuthorization =
  components["schemas"]["CommercialAuthorizationGrant"];
export type CommercialAuthorizationEvent =
  components["schemas"]["CommercialAuthorizationEvent"];
export type CommercialAuthorizationResult =
  components["schemas"]["CommercialAuthorizationResult"];
export type CommercialAuthorizationOTPChallenge =
  components["schemas"]["CommercialAuthorizationOTPChallengeResult"];
export interface ActivateCommercialAuthorizationInput {
  template_version: number;
  consent: boolean;
  otp_challenge_id: string;
  otp_code: string;
}
export type AuthorizationActivation = ActivateCommercialAuthorizationInput;

const AUTHORIZATION_EVENT_ACTIONS = new Set<
  CommercialAuthorizationEvent["action"]
>(["activated", "revoked", "auto_confirmed"]);

export function isCommercialAuthorizationEvent(
  value: unknown,
): value is CommercialAuthorizationEvent {
  if (!value || typeof value !== "object") return false;
  const item = value as CommercialAuthorizationEvent;
  return (
    typeof item.grant_id === "string" &&
    typeof item.space_id === "string" &&
    typeof item.actor_id === "string" &&
    AUTHORIZATION_EVENT_ACTIONS.has(item.action) &&
    typeof item.reason === "string" &&
    typeof item.occurred_at === "string" &&
    Number.isFinite(Date.parse(item.occurred_at))
  );
}

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

export function isCommercialAuthorizationOTPChallenge(
  value: unknown,
): value is CommercialAuthorizationOTPChallenge {
  if (!value || typeof value !== "object") return false;
  const item = value as CommercialAuthorizationOTPChallenge;
  return (
    typeof item.challenge_id === "string" &&
    item.challenge_id.length > 0 &&
    typeof item.channel === "string" &&
    item.channel.length > 0 &&
    typeof item.expires_at === "string" &&
    Number.isFinite(Date.parse(item.expires_at)) &&
    typeof item.masked_email === "string" &&
    item.masked_email.length > 0
  );
}
