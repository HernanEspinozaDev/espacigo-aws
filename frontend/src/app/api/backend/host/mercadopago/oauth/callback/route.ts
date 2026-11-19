import "server-only";
import { getApiBackendBaseUrl } from "@/lib/api/server-config";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const allowedQueryKeys = new Set([
  "code",
  "state",
  "error",
  "error_description",
  "error_uri",
]);
const validConnectionStates = new Set([
  "connected",
  "denied",
  "mode_mismatch",
  "invalid_state",
  "exchange_failed",
  "not_configured",
]);
const requestIdPattern = /^[A-Za-z0-9._-]{1,128}$/;

function errorResponse(status: number, code: string, requestId: string) {
  return Response.json(
    {
      error: {
        code,
        message: "No fue posible completar la vinculación.",
        request_id: requestId,
      },
    },
    {
      status,
      headers: { "cache-control": "no-store", "x-request-id": requestId },
    },
  );
}

export async function GET(request: Request): Promise<Response> {
  const requestId = request.headers.get("x-request-id") ?? crypto.randomUUID();
  if (!requestIdPattern.test(requestId)) {
    return errorResponse(400, "invalid_request_id", crypto.randomUUID());
  }

  const incoming = new URL(request.url);
  for (const key of incoming.searchParams.keys()) {
    if (!allowedQueryKeys.has(key)) {
      return errorResponse(400, "query_parameter_not_allowed", requestId);
    }
    if (incoming.searchParams.getAll(key).length !== 1) {
      return errorResponse(400, "duplicate_query_parameter", requestId);
    }
  }
  const state = incoming.searchParams.get("state");
  const code = incoming.searchParams.get("code");
  const oauthError = incoming.searchParams.get("error");
  const errorDescription = incoming.searchParams.get("error_description");
  const errorUri = incoming.searchParams.get("error_uri");
  if (
    !state ||
    state.length > 512 ||
    (code !== null && (!code || code.length > 8192)) ||
    (oauthError !== null && !/^[A-Za-z0-9_.-]{1,128}$/.test(oauthError)) ||
    (errorDescription !== null && errorDescription.length > 1024) ||
    (errorUri !== null && errorUri.length > 2048) ||
    (code !== null && oauthError !== null)
  ) {
    return errorResponse(400, "invalid_oauth_callback", requestId);
  }

  let baseUrl: URL;
  try {
    baseUrl = getApiBackendBaseUrl();
  } catch {
    return errorResponse(503, "api_not_configured", requestId);
  }
  const upstreamUrl = new URL(
    "/api/v1/host/mercadopago/oauth/callback",
    baseUrl,
  );
  upstreamUrl.searchParams.set("state", state);
  if (code !== null) upstreamUrl.searchParams.set("code", code);
  if (oauthError !== null) upstreamUrl.searchParams.set("error", oauthError);

  let upstream: Response;
  try {
    upstream = await fetch(upstreamUrl, {
      method: "GET",
      headers: { accept: "application/json", "x-request-id": requestId },
      cache: "no-store",
      credentials: "omit",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(30_000)]),
    });
  } catch {
    return errorResponse(502, "api_unavailable", requestId);
  }

  if (upstream.status === 303) {
    const location = upstream.headers.get("location");
    if (!location)
      return errorResponse(502, "invalid_upstream_redirect", requestId);
    let target: URL;
    try {
      target = new URL(location, request.url);
    } catch {
      return errorResponse(502, "invalid_upstream_redirect", requestId);
    }
    const connection = target.searchParams.get("connection");
    if (
      target.origin !== incoming.origin ||
      target.pathname !== "/anfitrion/mercado-pago" ||
      !connection ||
      target.searchParams.getAll("connection").length !== 1 ||
      !validConnectionStates.has(connection) ||
      [...target.searchParams.keys()].some((key) => key !== "connection") ||
      target.hash
    ) {
      return errorResponse(502, "invalid_upstream_redirect", requestId);
    }
    return new Response(null, {
      status: 303,
      headers: {
        location: `${target.pathname}?connection=${encodeURIComponent(connection)}`,
        "cache-control": "no-store",
        "x-request-id": requestId,
      },
    });
  }

  await upstream.body?.cancel().catch(() => undefined);
  if (upstream.status >= 400 && upstream.status <= 599) {
    return errorResponse(upstream.status, "oauth_callback_failed", requestId);
  }
  return errorResponse(502, "invalid_upstream_response", requestId);
}
