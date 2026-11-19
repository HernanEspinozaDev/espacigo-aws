import "client-only";
import type { AccessToken } from "@/features/auth/application/auth.port";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import {
  parseAuthorizationUrl,
  parseMercadoPagoConnection,
} from "../domain/mercado-pago-connection";
import type { MercadoPagoPort } from "../application/mercado-pago.port";

const auth = (token: AccessToken) => ({
  Authorization: ["Bearer", token].join(" "),
});

export class MercadoPagoOpenApiAdapter implements MercadoPagoPort {
  async getConnection(token: AccessToken, signal: AbortSignal) {
    const result = await apiClient.GET("/host/mercadopago", {
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    try {
      return parseMercadoPagoConnection(result.data);
    } catch {
      throw new ApiProtocolError();
    }
  }

  async startAuthorization(token: AccessToken, signal: AbortSignal) {
    const result = await apiClient.POST("/host/mercadopago/oauth/start", {
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    try {
      return parseAuthorizationUrl(result.data);
    } catch {
      throw new ApiProtocolError();
    }
  }
}
