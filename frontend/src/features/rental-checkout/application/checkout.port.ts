import type { AccessToken } from "@/features/auth/application/auth.port";
import type { CheckoutLaunch, CheckoutRead } from "../domain/checkout";

export interface CheckoutPort {
  start(
    quoteId: string,
    idempotencyKey: string,
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<CheckoutLaunch>;
  startWallet(
    quoteId: string,
    idempotencyKey: string,
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<CheckoutLaunch>;
  read(
    quoteId: string,
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<CheckoutRead>;
}
