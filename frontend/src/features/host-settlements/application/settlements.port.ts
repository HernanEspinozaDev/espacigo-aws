import type { AccessToken } from "@/features/auth/application/auth.port";
import type { SettlementPage } from "../domain/settlement";

export interface HostSettlementsPort {
  list(
    token: AccessToken,
    month: string,
    mode: "live" | "test",
    cursor: string | null,
    signal: AbortSignal,
  ): Promise<SettlementPage>;
}
