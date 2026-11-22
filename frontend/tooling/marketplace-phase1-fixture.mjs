import http from "node:http";

// Explicitly synthetic upstream for browser acceptance; no DB or AWS access.
const port = Number(process.env.MARKETPLACE_FIXTURE_PORT ?? 18121);
const prefix = "10000000-0000-4000-8000-";
const imagePrefix = "20000000-0000-4000-8000-";
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
  "base64",
);
const idFor = (n) => `${prefix}${String(n).padStart(12, "0")}`;
const imageId = (n) => `${imagePrefix}${String(n).padStart(12, "0")}`;
const detail = (id) => {
  const n = Number(id.slice(-12));
  const count = n === 2 ? 0 : n === 3 || n === 4 ? 1 : 5;
  return {
    space_id: id,
    category_code: "oficina",
    category_name: "Oficina",
    title: "Espacio de prueba del marketplace",
    description:
      "Descripción pública de prueba. " + "TextoContinuo".repeat(160),
    area_m2: 42,
    capacity_max: 8,
    rate_unit: "hora",
    base_price_clp: n === 5 ? "9007199254740993" : 12000,
    currency: "CLP",
    estimated_total_clp: null,
    usage_rules: "Respetar los horarios y cuidar las instalaciones.",
    features: [{ code: "wifi", label: "Wi-Fi", value: true }],
    attributes: [
      { code: "wifi", label: "Wi-Fi", type: "boolean", order: 1, value: true },
      {
        code: "puestos_trabajo",
        label: "Puestos de trabajo",
        type: "integer",
        order: 2,
        value: 8,
      },
      {
        code: "tipo_uso_oficina",
        label: "Tipo de uso",
        type: "enum",
        order: 3,
        value: "privada",
      },
      {
        code: "climatizacion",
        label: "Climatización",
        type: "enum_list",
        order: 4,
        value: ["aire_acondicionado", "calefaccion"],
      },
    ],
    host: {
      name: "Hernán",
      joined_at: "2025-01-15T12:00:00Z",
      spaces_count: 2,
      identity_verified: true,
    },
    cancellation_policy: null,
    guarantee: null,
    verification: { identity_verified: true, space_verified: false },
    images: Array.from({ length: count }, (_, i) => ({
      image_id: imageId(i + 1),
      is_cover: i === 0,
      source: n === 4 ? "media" : "synthetic",
    })),
    owner_id: "PRIVATE_OWNER_MARKER",
    owner_email: "PRIVATE_EMAIL_MARKER",
    exact_address: "PRIVATE_ADDRESS_MARKER",
    document_key: "PRIVATE_DOCUMENT_MARKER",
  };
};
function reply(res, status, data) {
  res.writeHead(status, {
    "content-type": "application/json",
    "cache-control": "no-store",
  });
  res.end(JSON.stringify(data));
}
http
  .createServer((req, res) => {
    const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
    if (req.method !== "GET" && req.method !== "HEAD") {
      reply(res, 405, { error: { code: "method_not_allowed" } });
      return;
    }
    if (url.pathname === "/healthz") return reply(res, 200, { status: "ok" });
    if (url.pathname === "/api/v1/public/spaces")
      return reply(res, 200, { items: [detail(idFor(1))] });
    if (url.pathname === "/api/v1/public/space-categories")
      return reply(res, 200, { items: [] });
    if (url.pathname === "/api/v1/public/locations")
      return reply(res, 200, { items: [] });
    const space = /^\/api\/v1\/public\/spaces\/([^/]+)$/.exec(url.pathname);
    if (space && [1, 2, 3, 4, 5, 6, 7].some((n) => space[1] === idFor(n)))
      return reply(res, 200, { data: detail(space[1]) });
    const image =
      /^\/api\/v1\/public\/spaces\/([^/]+)\/images\/([^/]+)(?:\/variants\/(display|thumbnail))?$/.exec(
        url.pathname,
      );
    if (image) {
      if (image[1] === idFor(6))
        return reply(res, 404, { error: { code: "not_found" } });
      if (image[1] === idFor(4) && image[3])
        return reply(res, 404, { error: { code: "not_found" } });
      res.writeHead(200, {
        "content-type": "image/png",
        "content-length": png.length,
        "cache-control": "no-store",
      });
      res.end(png);
      return;
    }
    const availability =
      /^\/api\/v1\/spaces\/([^/]+)\/rental-availability$/.exec(url.pathname);
    if (availability) {
      if (availability[1] === idFor(7))
        return reply(res, 503, { error: { code: "unavailable" } });
      const date = url.searchParams.get("date") ?? "";
      const duration = Number(url.searchParams.get("duration") ?? "1");
      if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || ![1, 2, 4].includes(duration))
        return reply(res, 422, { error: { code: "invalid" } });
      const start = new Date(`${date}T12:00:00Z`);
      const end = new Date(start.getTime() + duration * 3600000);
      return reply(res, 200, {
        data: {
          space_id: availability[1],
          rate_unit: "hora",
          time_zone: "America/Santiago",
          checked_at: new Date().toISOString(),
          items: [{ start_at: start.toISOString(), end_at: end.toISOString() }],
        },
      });
    }
    reply(res, 404, { error: { code: "not_found" } });
  })
  .listen(port, "127.0.0.1");
