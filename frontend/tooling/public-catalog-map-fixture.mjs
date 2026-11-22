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
  title: index ? "Sala fuera de la página de cuadrícula" : listing.title,
  base_price_clp: 12000,
  currency: "CLP",
  rate_unit: "hora",
  location: {
    ...listing.location,
    latitude: listing.location.latitude + index * 0.005,
  },
}));
http
  .createServer((request, response) => {
    const url = new URL(request.url, "http://127.0.0.1");
    let data;
    if (url.pathname === "/healthz") data = { status: "ok" };
    else if (url.pathname === "/api/v1/public/spaces/map")
      data = url.searchParams.has("cursor")
        ? { items: [markers[1]] }
        : { items: [markers[0]], next_cursor: "fixture-map-page-2" };
    else if (url.pathname === "/api/v1/public/spaces")
      data = { items: [listing] };
    else if (url.pathname.startsWith("/api/v1/public/spaces/"))
      data = { data: listing };
    else if (url.pathname === "/api/v1/public/space-categories")
      data = { items: [{ code: "oficina", name: "Oficina", order: 1 }] };
    else if (url.pathname === "/api/v1/public/locations") data = { items: [] };
    else {
      response.writeHead(404, { "content-type": "application/json" });
      response.end(JSON.stringify({ error: { code: "not_found" } }));
      return;
    }
    response.writeHead(200, { "content-type": "application/json" });
    response.end(JSON.stringify(data));
  })
  .listen(port, "127.0.0.1");
