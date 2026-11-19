import "client-only";

import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";

type Schemas = components["schemas"];
export type DemoRental = Schemas["LocalEarlyRentalDetail"];
export type DemoRentalSelection = Schemas["CreateLocalEarlyRentalRequest"];
export type DemoRentalAction = "identity" | "contract" | "reject" | "pay";

function unwrap<T>(response: Response, data: T | undefined, error: unknown): T {
  if (!response.ok) throw normalizeApiError(response, error);
  if (!data) throw new ApiProtocolError();
  return data;
}

function detail(result: {
  response: Response;
  data?: { data: DemoRental };
  error?: unknown;
}) {
  const rental = unwrap(result.response, result.data, result.error).data;
  const revision = result.response.headers.get("etag");
  if (!revision) throw new ApiProtocolError();
  return { rental, revision };
}

export const earlyRentalAdapter = {
  async enabled(signal: AbortSignal) {
    const result = await apiClient.GET("/local/early-rentals/capabilities", {
      signal,
    });
    if (result.response.status === 404 || result.response.status === 403)
      return false;
    return unwrap(result.response, result.data, result.error).data.enabled;
  },
  async list(signal: AbortSignal, cursor?: string) {
    const result = await apiClient.GET("/local/early-rentals", {
      signal,
      params: { query: { cursor } },
    });
    return unwrap(result.response, result.data, result.error).data;
  },
  async read(id: string, signal: AbortSignal) {
    return detail(
      await apiClient.GET("/local/early-rentals/{earlyRentalID}", {
        params: { path: { earlyRentalID: id } },
        signal,
      }),
    );
  },
  async create(body: DemoRentalSelection, key: string, signal: AbortSignal) {
    const result = await apiClient.POST("/local/early-rentals", {
      params: { header: { "Idempotency-Key": key } },
      body,
      signal,
    });
    return unwrap(result.response, result.data, result.error).data;
  },
  async act(
    action: DemoRentalAction,
    rental: DemoRental,
    revision: string,
    signal: AbortSignal,
  ) {
    const path = { earlyRentalID: rental.id };
    const header = {
      "If-Match": revision,
      "Idempotency-Key": rental.id + ":" + action,
    };
    switch (action) {
      case "identity":
        return detail(
          await apiClient.POST(
            "/local/early-rentals/{earlyRentalID}/identity-confirmations",
            {
              params: { path, header },
              body: {},
              signal,
            },
          ),
        );
      case "contract":
        return detail(
          await apiClient.POST(
            "/local/early-rentals/{earlyRentalID}/contract-acceptances",
            {
              params: { path, header },
              body: {
                contract_version: Number(rental.contract.version),
                contract_digest: rental.contract.digest,
              },
              signal,
            },
          ),
        );
      case "reject":
        return detail(
          await apiClient.POST("/local/early-rentals/{earlyRentalID}/reject", {
            params: { path, header },
            body: { reason_code: "condiciones_no_aceptadas" },
            signal,
          }),
        );
      case "pay":
        return detail(
          await apiClient.POST("/local/early-rentals/{earlyRentalID}/pay", {
            params: { path, header: { "If-Match": revision } },
            body: {},
            signal,
          }),
        );
    }
  },
};
