import "client-only";
import { authFacade } from "@/features/auth/auth-runtime";
import type { SettlementPage } from "../domain/settlement";
import type { HostSettlementsPort } from "./settlements.port";

export class HostSettlementsFacade {
  #generation = 0;
  constructor(private readonly port: HostSettlementsPort) {}

  async list(
    month: string,
    mode: "live" | "test",
    cursor: string | null,
  ): Promise<SettlementPage> {
    const generation = ++this.#generation;
    const result = await authFacade.runAuthenticatedOperation((token, signal) =>
      this.port.list(token, month, mode, cursor, signal),
    );
    if (generation !== this.#generation)
      throw new DOMException("Request superseded", "AbortError");
    return result;
  }
  cancel() {
    this.#generation++;
  }
}
