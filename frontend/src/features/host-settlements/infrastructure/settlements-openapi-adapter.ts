import "client-only";
import type { AccessToken } from "@/features/auth/application/auth.port";
import { apiClient } from "@/lib/api/client";
import { normalizeApiError } from "@/lib/api/errors";
import { parseSettlementPage } from "../domain/settlement";
import type { HostSettlementsPort } from "../application/settlements.port";

export class HostSettlementsOpenApiAdapter implements HostSettlementsPort {
  async list(
    token: AccessToken,
    month: string,
    mode: "live" | "test",
    cursor: string | null,
    signal: AbortSignal,
  ) {
    const result = await apiClient.GET("/host/settlements", {
      params: { query: { month, mode, ...(cursor ? { cursor } : {}) } },
      headers: { Authorization: ["Bearer", token].join(" ") },
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    try {
      return parseSettlementPage(result.data);
    } catch {
      throw new Error("settlement_response_invalid");
    }
  }
}
