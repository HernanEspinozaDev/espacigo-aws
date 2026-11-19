import "client-only";
import type { AccessToken } from "@/features/auth/application/auth.port";
import { apiClient } from "@/lib/api/client";
import { normalizeApiError } from "@/lib/api/errors";
import { parseCheckoutLaunch, parseCheckoutRead } from "../domain/checkout";
import type { CheckoutPort } from "../application/checkout.port";

const auth = (token: AccessToken) => ({
  Authorization: ["Bearer", token].join(" "),
});

export class CheckoutOpenApiAdapter implements CheckoutPort {
  async start(
    quoteId: string,
    idempotencyKey: string,
    token: AccessToken,
    signal: AbortSignal,
  ) {
    const result = await apiClient.POST("/rental-quotes/{quoteID}/checkout", {
      params: {
        path: { quoteID: quoteId },
        header: { "Idempotency-Key": idempotencyKey },
      },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return parseCheckoutLaunch(result.data, quoteId);
  }

  async read(quoteId: string, token: AccessToken, signal: AbortSignal) {
    const result = await apiClient.GET("/rental-quotes/{quoteID}/checkout", {
      params: { path: { quoteID: quoteId } },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return parseCheckoutRead(result.data, quoteId);
  }
}
