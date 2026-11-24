import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";

export type AddressCandidate =
  components["schemas"]["SpaceAddressGeocodeCandidate"];
export type AddressQuery = components["schemas"]["SpaceAddressGeocodeInput"];
export type CommuneViewport =
  components["schemas"]["SpaceLocationCommuneBounds"];

export async function getCommercialCommuneViewport(
  communeCode: string,
  signal?: AbortSignal,
): Promise<CommuneViewport | null> {
  const result = await apiClient.GET(
    "/spaces/location/communes/{communeCode}",
    { params: { path: { communeCode } }, signal },
  );
  if (result.response.status === 404) return null;
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  const bounds = result.data;
  if (
    !bounds ||
    !Number.isFinite(bounds.west) ||
    !Number.isFinite(bounds.south) ||
    !Number.isFinite(bounds.east) ||
    !Number.isFinite(bounds.north) ||
    bounds.west >= bounds.east ||
    bounds.south >= bounds.north ||
    bounds.west < -180 ||
    bounds.east > 180 ||
    bounds.south < -90 ||
    bounds.north > 90
  )
    throw new ApiProtocolError();
  return bounds;
}

export async function searchCommercialAddress(
  query: AddressQuery,
  signal?: AbortSignal,
): Promise<AddressCandidate[]> {
  const result = await apiClient.POST("/spaces/location/geocode", {
    body: query,
    signal,
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data || !Array.isArray(result.data.candidates))
    throw new ApiProtocolError();
  return result.data.candidates.filter(
    (candidate) =>
      typeof candidate.label === "string" &&
      candidate.label.length > 0 &&
      Number.isFinite(candidate.latitude) &&
      Number.isFinite(candidate.longitude) &&
      Math.abs(candidate.latitude) <= 90 &&
      Math.abs(candidate.longitude) <= 180 &&
      (candidate.precision === "address" ||
        candidate.precision === "approximate"),
  );
}

export async function reverseGeocodeCommercialPin(
  latitude: number,
  longitude: number,
  signal?: AbortSignal,
) {
  const result = await apiClient.POST("/spaces/location/reverse-geocode", {
    body: { latitude, longitude },
    signal,
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data) throw new ApiProtocolError();
  if (
    typeof result.data.region_code !== "string" ||
    !result.data.region_code.trim() ||
    typeof result.data.comuna_code !== "string" ||
    !result.data.comuna_code.trim()
  )
    throw new ApiProtocolError();
  return result.data;
}
