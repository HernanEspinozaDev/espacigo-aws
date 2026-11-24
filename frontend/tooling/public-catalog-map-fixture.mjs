import http from "node:http";
const port = Number(process.env.CATALOG_MAP_FIXTURE_PORT ?? 18445);
const ids = [
  "00000000-0000-4000-8000-000000000007",
  "00000000-0000-4000-8000-000000000008",
];
const listing = {
  space_id: ids[0],
  category_code: "oficina",
  category_name: "Oficina",
  title: "Sala con ubicación confirmada",
  description: "Espacio sintético para comprobar el mapa.",
  area_m2: 12,
  capacity_max: 5,
  rate_unit: "hora",
  base_price_clp: 12000,
  currency: "CLP",
  estimated_total_clp: null,
  features: [],
  images: [],
  attributes: [],
  usage_rules: "Respetar el horario acordado.",
  cancellation_policy: null,
  guarantee: null,
  host: {
    name: "Ana",
    avatar_url: null,
    joined_at: "2025-01-01T00:00:00Z",
    spaces_count: 1,
    identity_verified: false,
  },
  location: {
    address: "Calle Sintética 123",
    latitude: -33.45,
    longitude: -70.66,
  },
  verification: { identity_verified: false, space_verified: false },
};
const markers = ids.map((space_id, index) => ({
  space_id,
  capacity_max: index ? 3 : listing.capacity_max,
  title: index ? "Sala fuera de la página de cuadrícula" : listing.title,
  base_price_clp: 12000,
  currency: "CLP",
  rate_unit: "hora",
  location: {
    ...listing.location,
    latitude: listing.location.latitude + index * 0.005,
  },
}));
function matchingMarkers(query) {
  const category = query.get("category_code");
  if (category && category !== "oficina") return [];
  if (query.has("comuna_code") && query.get("comuna_code") !== "13101")
    return [];
  if (query.has("region_code") && query.get("region_code") !== "13") return [];
  const matches = markers.filter(
    (marker) => marker.capacity_max >= Number(query.get("min_capacity")),
  );
  if (query.get("q") === "Grupo")
    return Array.from({ length: matches.length ? 12 : 0 }, (_, index) => ({
      ...markers[0],
      space_id: `00000000-0000-4000-8000-${String(index + 100).padStart(12, "0")}`,
      title: `Sala del grupo ${index + 1}`,
    }));
  const radius = Number(query.get("radius_km"));
  if (radius <= 0) return matches;
  const lat = Number(query.get("latitude"));
  const lon = Number(query.get("longitude"));
  return matches
    .map((marker) => ({
      ...marker,
      distance_km: Math.hypot(
        (lat - marker.location.latitude) * 111,
        (lon - marker.location.longitude) * 92,
      ),
    }))
    .filter((marker) => marker.distance_km <= radius)
    .sort((a, b) => a.distance_km - b.distance_km);
}
http
  .createServer((request, response) => {
    const url = new URL(request.url, "http://127.0.0.1");
    let data;
    if (url.pathname === "/healthz") data = { status: "ok" };
    else if (url.pathname === "/api/v1/public/spaces/map") {
      const matches = matchingMarkers(url.searchParams);
      data = url.searchParams.has("cursor")
        ? { items: matches.slice(1) }
        : {
            items: matches.slice(0, 1),
            ...(matches.length > 1
              ? { next_cursor: "fixture-map-page-2" }
              : {}),
          };
    } else if (url.pathname === "/api/v1/public/spaces") {
      const matches = matchingMarkers(url.searchParams);
      const items =
        url.searchParams.has("radius_km") ||
        url.searchParams.has("min_capacity") ||
        url.searchParams.get("q") === "Grupo"
          ? matches
          : matches.slice(0, 1);
      data = { items: items.map((item) => ({ ...listing, ...item })) };
    } else if (url.pathname.startsWith("/api/v1/public/spaces/"))
      data = { data: listing };
    else if (url.pathname === "/api/v1/public/space-categories")
      data = {
        items: [
          { code: "oficina", name: "Oficina", order: 1 },
          { code: "quincho", name: "Quincho", order: 2 },
        ],
      };
    else if (url.pathname === "/api/v1/public/locations")
      data = {
        items: [
          {
            region_code: "13",
            region_name: "Metropolitana",
            comuna_code: "13101",
            comuna_name: "Santiago",
          },
          {
            region_code: "05",
            region_name: "Valparaíso",
            comuna_code: "05603",
            comuna_name: "Cartagena",
          },
        ],
      };
    else {
      response.writeHead(404, { "content-type": "application/json" });
      response.end(JSON.stringify({ error: { code: "not_found" } }));
      return;
    }
    response.writeHead(200, { "content-type": "application/json" });
    response.end(JSON.stringify(data));
  })
  .listen(port, "127.0.0.1");
