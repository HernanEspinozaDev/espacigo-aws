import type { AccessToken } from "@/features/auth/application/auth.port";
import type { MercadoPagoConnection } from "../domain/mercado-pago-connection";

export interface MercadoPagoPort {
  getConnection(
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<MercadoPagoConnection>;
  startAuthorization(token: AccessToken, signal: AbortSignal): Promise<string>;
}
