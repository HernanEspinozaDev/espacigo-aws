import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import { strongEtag } from "../domain/calendar-configuration";
import type { LocationPort } from "../application/commercial-location-service";
import type {
  LocationInput,
  LocationRead,
} from "../domain/commercial-location";
export class CommercialLocationOpenApiAdapter implements LocationPort {
  async read(spaceId: string, signal?: AbortSignal): Promise<LocationRead> {
    const result = await apiClient.GET("/spaces/{spaceId}/location", {
      params: { path: { spaceId } },
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const etag = result.response.headers.get("ETag");
    if (!result.data || !strongEtag(etag) || result.data.space_id !== spaceId)
      throw new ApiProtocolError();
    return { location: result.data, etag };
  }
  async save(
    spaceId: string,
    input: LocationInput,
    etag: string,
    signal?: AbortSignal,
  ): Promise<LocationRead> {
    if (!strongEtag(etag)) throw new ApiProtocolError();
    const result = await apiClient.PUT("/spaces/{spaceId}/location", {
      params: { path: { spaceId }, header: { "If-Match": etag } },
      body: {
        address: input.address,
        region_code: input.region_code,
        comuna_code: input.comuna_code,
        latitude: input.latitude,
        longitude: input.longitude,
        public_confirmed: input.public_confirmed,
      },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const next = result.response.headers.get("ETag");
    if (!result.data || !strongEtag(next) || result.data.space_id !== spaceId)
      throw new ApiProtocolError();
    return { location: result.data, etag: next };
  }
}
