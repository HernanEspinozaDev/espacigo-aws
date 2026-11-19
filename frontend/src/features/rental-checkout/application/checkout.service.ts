import type { AccessToken } from "@/features/auth/application/auth.port";
import type { CheckoutPort } from "./checkout.port";

export function createCheckoutService(port: CheckoutPort) {
  return {
    start(
      quoteId: string,
      idempotencyKey: string,
      token: AccessToken,
      signal: AbortSignal,
    ) {
      return port.start(quoteId, idempotencyKey, token, signal);
    },
    read(quoteId: string, token: AccessToken, signal: AbortSignal) {
      return port.read(quoteId, token, signal);
    },
  };
}
