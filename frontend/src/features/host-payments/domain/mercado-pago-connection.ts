import { ApiProtocolError } from "@/lib/api/errors";

export interface MercadoPagoConnection {
  connected: boolean;
  sellerId: string | null;
  expiresAt: string | null;
  authorizationAvailable: boolean;
}

export function parseMercadoPagoConnection(
  value: unknown,
): MercadoPagoConnection {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ApiProtocolError();
  }
  const data = value as Record<string, unknown>;
  if (
    typeof data.connected !== "boolean" ||
    typeof data.authorization_available !== "boolean" ||
    (data.seller_id !== undefined && typeof data.seller_id !== "string") ||
    (data.expires_at !== undefined && typeof data.expires_at !== "string")
  ) {
    throw new ApiProtocolError();
  }
  if (
    data.expires_at !== undefined &&
    Number.isNaN(Date.parse(data.expires_at))
  ) {
    throw new ApiProtocolError();
  }
  return {
    connected: data.connected,
    sellerId: typeof data.seller_id === "string" ? data.seller_id : null,
    expiresAt: typeof data.expires_at === "string" ? data.expires_at : null,
    authorizationAvailable: data.authorization_available,
  };
}

export function parseAuthorizationUrl(value: unknown): string {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ApiProtocolError();
  }
  const url = (value as Record<string, unknown>).authorization_url;
  if (typeof url !== "string") throw new ApiProtocolError();
  try {
    const parsed = new URL(url);
    if (
      (parsed.origin !== "https://auth.mercadopago.cl" &&
        parsed.origin !== "https://auth.mercadopago.com") ||
      parsed.username.length > 0 ||
      parsed.password.length > 0
    ) {
      throw new ApiProtocolError();
    }
  } catch {
    throw new ApiProtocolError();
  }
  return url;
}
