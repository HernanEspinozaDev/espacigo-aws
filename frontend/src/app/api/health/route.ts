import { validateServerConfig } from "@/lib/api/server-config";

export const dynamic = "force-dynamic";

export function GET(): Response {
  const config = validateServerConfig();
  if (!config.valid) {
    return Response.json(
      {
        status: "error",
        error: config.error,
        environment: config.environment,
        timestamp: new Date().toISOString(),
        uptime: process.uptime(),
      },
      {
        status: 503,
        headers: {
          "cache-control": "no-store, no-cache, must-revalidate",
        },
      },
    );
  }

  return Response.json(
    {
      status: "ok",
      environment: config.environment,
      timestamp: new Date().toISOString(),
      uptime: process.uptime(),
    },
    {
      status: 200,
      headers: {
        "cache-control": "no-store, no-cache, must-revalidate",
      },
    },
  );
}

export function HEAD(): Response {
  const config = validateServerConfig();
  if (!config.valid) {
    return new Response(null, {
      status: 503,
      headers: {
        "cache-control": "no-store, no-cache, must-revalidate",
      },
    });
  }

  return new Response(null, {
    status: 200,
    headers: {
      "cache-control": "no-store, no-cache, must-revalidate",
    },
  });
}
