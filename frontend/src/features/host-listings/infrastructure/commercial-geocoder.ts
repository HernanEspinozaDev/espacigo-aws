import "client-only";
export type AddressCandidate = {
  label: string;
  latitude: number;
  longitude: number;
};
// Explicit GeoJSON FeatureCollection endpoint configured by the deployment.
// No public Nominatim endpoint is selected or contacted implicitly.
export async function searchConfiguredAddress(
  query: string,
  signal?: AbortSignal,
): Promise<AddressCandidate[]> {
  const endpoint = process.env.NEXT_PUBLIC_GEOCODING_URL;
  if (!endpoint)
    throw new Error(
      "Búsqueda de direcciones no configurada. Ubica el pin en el mapa o ingresa las coordenadas.",
    );
  const url = new URL(endpoint, window.location.origin);
  url.searchParams.set("q", query);
  const response = await fetch(url, {
    signal,
    credentials: "omit",
    redirect: "error",
    referrerPolicy: "no-referrer",
    cache: "no-store",
  });
  if (!response.ok)
    throw new Error(
      "El proveedor de direcciones no respondió. Puedes ingresar el pin manualmente.",
    );
  const data: unknown = await response.json();
  if (
    !data ||
    typeof data !== "object" ||
    !("features" in data) ||
    !Array.isArray(data.features)
  )
    throw new Error(
      "El proveedor devolvió un formato de dirección no compatible.",
    );
  return data.features.slice(0, 8).flatMap((feature: unknown) => {
    if (
      !feature ||
      typeof feature !== "object" ||
      !("geometry" in feature) ||
      !("properties" in feature)
    )
      return [];
    const geometry = feature.geometry;
    const properties = feature.properties;
    if (
      !geometry ||
      typeof geometry !== "object" ||
      !("type" in geometry) ||
      geometry.type !== "Point" ||
      !("coordinates" in geometry) ||
      !Array.isArray(geometry.coordinates)
    )
      return [];
    const [longitude, latitude] = geometry.coordinates;
    if (
      typeof latitude !== "number" ||
      typeof longitude !== "number" ||
      !Number.isFinite(latitude) ||
      !Number.isFinite(longitude) ||
      Math.abs(latitude) > 90 ||
      Math.abs(longitude) > 180
    )
      return [];
    const label =
      properties &&
      typeof properties === "object" &&
      "label" in properties &&
      typeof properties.label === "string"
        ? properties.label
        : query;
    return [{ label: label.slice(0, 500), latitude, longitude }];
  });
}
