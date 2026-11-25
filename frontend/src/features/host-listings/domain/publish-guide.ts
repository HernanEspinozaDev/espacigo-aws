const HOST_SPACES_RETURN_PATH_RE =
  /^\/anfitrion\/espacios(?:\/nuevo|\/[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\/editar(?:\?tab=(?:info|location|pricing|schedule|fotos|photos|contrato|contract))?)?$/i;

export type PublishGuideStepStatus = "completed" | "pending" | "blocked";

export type PublishGateReasonCode =
  | "profile_incomplete"
  | "identity_verification_required"
  | "mercadopago_connection_required"
  | "contract_template_required"
  | "location_confirmation_required"
  | "time_zone_required";

export function safeHostReturnPath(
  raw: string | string[] | null | undefined,
): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  if (!trimmed || !HOST_SPACES_RETURN_PATH_RE.test(trimmed)) {
    return null;
  }
  return trimmed;
}

export function verificationReturnHref(returnTo?: string | null): string {
  const safe = safeHostReturnPath(returnTo);
  return safe
    ? `/mi-cuenta/perfil?return_to=${encodeURIComponent(safe)}#identity-verification`
    : "/mi-cuenta/perfil#identity-verification";
}

export function mercadoPagoReturnHref(returnTo?: string | null): string {
  const safe = safeHostReturnPath(returnTo);
  return safe
    ? `/anfitrion/mercado-pago?return_to=${encodeURIComponent(safe)}`
    : "/anfitrion/mercado-pago";
}

export function isPublishGateReasonCode(
  code: unknown,
): code is PublishGateReasonCode {
  return (
    code === "profile_incomplete" ||
    code === "identity_verification_required" ||
    code === "mercadopago_connection_required" ||
    code === "contract_template_required" ||
    code === "location_confirmation_required" ||
    code === "time_zone_required"
  );
}
