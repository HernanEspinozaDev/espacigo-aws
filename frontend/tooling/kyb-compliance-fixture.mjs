import http from "node:http";

const port = Number(process.env.KYB_COMPLIANCE_FIXTURE_PORT ?? 18450);
if (!Number.isInteger(port) || port < 1024 || port > 65535) {
  throw new Error("Invalid fixture port");
}

const spaceAId = "880e8400-e29b-41d4-a716-446655440041";
const spaceBId = "880e8400-e29b-41d4-a716-446655440042";

function publicSpace(spaceId, title, verified) {
  return {
    space_id: spaceId,
    category_code: "oficina",
    category_name: "Oficina",
    title,
    description: `Ficha pública sintética para ${title}.`,
    area_m2: 18,
    capacity_max: 6,
    rate_unit: "hora",
    base_price_clp: 15000,
    currency: "CLP",
    region_code: "13",
    region_name: "Metropolitana",
    comuna_code: "13101",
    comuna_name: "Santiago",
    estimated_total_clp: null,
    usage_rules: "Uso sintético de evaluación.",
    features: [],
    attributes: [],
    host: {
      name: "Anfitrión Sintético",
      joined_at: "2025-01-01T12:00:00Z",
      spaces_count: 2,
      identity_verified: true,
    },
    cancellation_policy: null,
    guarantee: null,
    images: [],
    verification: {
      identity_verified: true,
      space_verified: verified,
    },
  };
}

const spaces = [
  publicSpace(spaceAId, "Espacio A Verificado", true),
  publicSpace(spaceBId, "Espacio B Sin Respaldo", false),
];

const termsV2Payload = {
  items: [
    {
      code: "evaluacion-terminos-20261126-v2",
      type: "terminos",
      synthetic: true,
      document: {
        id: "00000000-0000-4000-8000-000000000103",
        code: "evaluacion-terminos-20261126-v2",
        type: "terminos",
        title: "Términos y condiciones",
        scope: "Entorno de evaluación de EspaciGo",
        status:
          "Documento para evaluación con datos ficticios de ejemplo. No son condiciones aprobadas para un servicio comercial.",
        sections: [
          {
            heading: "Documentación comercial y plazo para publicar",
            body: "En esta evaluación, el plazo para presentar la documentación comercial es de dos meses calendario según la hora de Chile continental. Empieza con la primera publicación activa del titular, sea una persona natural o una empresa.",
          },
          {
            heading: "Documentos del titular y de cada espacio",
            body: "Solo los espacios con su documentación y la del titular aprobadas obtienen la insignia de verificación.",
          },
          {
            heading: "Vencimiento y regularización",
            body: "Si al vencer el plazo faltan documentos del titular, EspaciGo inhabilita su perfil para publicar, administrar espacios y recibir nuevas reservas. Puedes seguir arrendando espacios como cliente y presentar documentos para regularizar.",
          },
        ],
      },
    },
  ],
};

const server = http.createServer((request, response) => {
  const parsedUrl = new URL(request.url ?? "/", "http://127.0.0.1");
  const pathname = parsedUrl.pathname;
  response.setHeader("Content-Type", "application/json");
  response.setHeader("Cache-Control", "no-store");

  if (pathname === "/healthz" || pathname === "/health/ready") {
    response.writeHead(200);
    response.end(JSON.stringify({ status: "ok" }));
    return;
  }
  if (pathname === "/api/v1/public/spaces") {
    const verifiedOnly = parsedUrl.searchParams.get("verified_only") === "true";
    const items = verifiedOnly
      ? spaces.filter((item) => item.verification?.space_verified === true)
      : spaces;
    response.writeHead(200);
    response.end(
      JSON.stringify({
        items,
        has_more: false,
        total_count: items.length,
      }),
    );
    return;
  }
  const detailMatch = pathname.match(/^\/api\/v1\/public\/spaces\/([^/]+)$/);
  if (detailMatch) {
    const found = spaces.find((item) => item.space_id === detailMatch[1]);
    if (!found) {
      response.writeHead(404);
      response.end(JSON.stringify({ error: { code: "not_found" } }));
      return;
    }
    response.writeHead(200);
    response.end(JSON.stringify({ data: found }));
    return;
  }
  if (pathname === "/api/v1/public/space-categories") {
    response.writeHead(200);
    response.end(
      JSON.stringify({
        items: [{ code: "oficina", name: "Oficina", active_spaces: 2 }],
      }),
    );
    return;
  }
  if (pathname === "/api/v1/public/locations") {
    response.writeHead(200);
    response.end(JSON.stringify({ items: [] }));
    return;
  }
  if (pathname === "/api/v1/auth/terms") {
    response.writeHead(200);
    response.end(JSON.stringify(termsV2Payload));
    return;
  }

  response.writeHead(404);
  response.end(JSON.stringify({ error: { code: "not_found" } }));
});

server.listen(port, "127.0.0.1");
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
