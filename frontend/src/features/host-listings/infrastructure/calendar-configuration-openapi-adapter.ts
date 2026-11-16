import "client-only";

import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { components } from "@/lib/api/generated/openapi";
import {
  strongEtag,
  validateCalendarConfiguration,
  validateCalendarUpdate,
  type CalendarConfigurationRead,
} from "../domain/calendar-configuration";

export type CalendarClient = Pick<typeof apiClient, "GET" | "PUT">;
export class OpenApiCalendarConfigurationAdapter {
  constructor(private readonly client: CalendarClient = apiClient) {}

  async read(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<CalendarConfigurationRead> {
    const result = await this.client.GET(
      "/spaces/{spaceId}/availability/configuration",
      {
        params: { path: { spaceId } },
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const etag = result.response.headers.get("ETag");
    if (!result.data || !strongEtag(etag)) throw new ApiProtocolError();
    return {
      configuration: validateCalendarConfiguration(result.data, spaceId),
      etag,
    };
  }

  async update(
    spaceId: string,
    input: components["schemas"]["CalendarInventoryUpdate"],
    etag: string,
    signal?: AbortSignal,
  ): Promise<{ etag: string }> {
    validateCalendarUpdate(input);
    if (!strongEtag(etag)) throw new TypeError("A strong ETag is required");
    const payload = Object.fromEntries(
      Object.entries(input).filter(([, value]) => value !== undefined),
    ) as components["schemas"]["CalendarInventoryUpdate"];
    if (input.overrides) {
      payload.overrides = input.overrides.map(
        (override) =>
          Object.fromEntries(
            Object.entries(override).filter(([, value]) => value !== undefined),
          ) as components["schemas"]["CalendarInventoryOverride"],
      );
    }
    const result = await this.client.PUT(
      "/spaces/{spaceId}/availability/configuration",
      {
        params: { path: { spaceId }, header: { "If-Match": etag } },
        body: payload,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const responseEtag = result.response.headers.get("ETag");
    if (!result.data || !strongEtag(responseEtag)) throw new ApiProtocolError();
    return { etag: responseEtag };
  }
}
