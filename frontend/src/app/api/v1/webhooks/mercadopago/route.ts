import "server-only";
import { getApiBackendBaseUrl } from "@/lib/api/server-config";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const MAX_BODY_BYTES = 64 * 1024;
const MAX_QUERY_BYTES = 8 * 1024;
const MAX_DURATION_MS = 10_000;
const allowedQueryKeys = new Set(["data.id", "type"]);

function errorResponse(status: number, code: string): Response {
  return Response.json(
    { error: { code, message: "No fue posible recibir la notificación." } },
    { status, headers: { "cache-control": "no-store" } },
  );
}

async function readBoundedBody(
  request: Request,
  signal: AbortSignal,
): Promise<ArrayBuffer> {
  const declaredLength = request.headers.get("content-length");
  if (declaredLength !== null) {
    if (!/^\d+$/.test(declaredLength)) throw new TypeError("invalid_body");
    if (Number(declaredLength) > MAX_BODY_BYTES)
      throw new RangeError("too_large");
  }
  if (!request.body) throw new TypeError("invalid_body");

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  let onAbort: (() => void) | undefined;
  const rejectOnAbort = new Promise<never>((_, reject) => {
    onAbort = () => reject(signal.reason);
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });
  });
  try {
    while (true) {
      const { done, value } = await Promise.race([
        reader.read(),
        rejectOnAbort,
      ]);
      if (done) break;
      total += value.byteLength;
      if (total > MAX_BODY_BYTES) {
        await reader.cancel().catch(() => undefined);
        throw new RangeError("too_large");
      }
      chunks.push(value);
    }
  } finally {
    if (onAbort) signal.removeEventListener("abort", onAbort);
    if (signal.aborted) await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
  if (total === 0) throw new TypeError("invalid_body");

  const body = new ArrayBuffer(total);
  const view = new Uint8Array(body);
  let offset = 0;
  for (const chunk of chunks) {
    view.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body;
}

export async function POST(request: Request): Promise<Response> {
  const incoming = new URL(request.url);
  const rawQuery = incoming.search;
  if (new TextEncoder().encode(rawQuery).byteLength > MAX_QUERY_BYTES) {
    return errorResponse(414, "query_too_large");
  }
  for (const key of incoming.searchParams.keys()) {
    if (!allowedQueryKeys.has(key))
      return errorResponse(400, "query_parameter_not_allowed");
    if (incoming.searchParams.getAll(key).length !== 1) {
      return errorResponse(400, "duplicate_query_parameter");
    }
  }

  const signature = request.headers.get("x-signature");
  const requestId = request.headers.get("x-request-id");
  if (
    !signature ||
    signature.length > 512 ||
    !requestId ||
    requestId.length > 256
  ) {
    return errorResponse(400, "missing_webhook_headers");
  }

  let body: ArrayBuffer;
  const requestBodySignal = AbortSignal.any([
    request.signal,
    AbortSignal.timeout(MAX_DURATION_MS),
  ]);
  try {
    body = await readBoundedBody(request, requestBodySignal);
  } catch (error) {
    if (error instanceof RangeError) return errorResponse(413, "too_large");
    if (requestBodySignal.aborted) return errorResponse(408, "request_timeout");
    return errorResponse(400, "invalid_body");
  }

  let baseUrl: URL;
  try {
    baseUrl = getApiBackendBaseUrl();
  } catch {
    return errorResponse(503, "api_not_configured");
  }
  const upstreamUrl = new URL(
    `${baseUrl.pathname.replace(/\/$/, "")}/webhooks/mercadopago`,
    baseUrl,
  );
  upstreamUrl.search = rawQuery;

  let upstream: Response;
  try {
    upstream = await fetch(upstreamUrl, {
      method: "POST",
      headers: {
        "content-type":
          request.headers.get("content-type") ?? "application/json",
        "x-signature": signature,
        "x-request-id": requestId,
      },
      body,
      cache: "no-store",
      credentials: "omit",
      redirect: "manual",
      signal: AbortSignal.any([
        request.signal,
        AbortSignal.timeout(MAX_DURATION_MS),
      ]),
    });
  } catch (error) {
    return errorResponse(
      error instanceof DOMException && error.name === "TimeoutError"
        ? 504
        : 502,
      error instanceof DOMException && error.name === "TimeoutError"
        ? "api_timeout"
        : "api_unavailable",
    );
  }

  await upstream.body?.cancel().catch(() => undefined);
  if (upstream.status >= 300 && upstream.status < 400) {
    return errorResponse(502, "unexpected_upstream_redirect");
  }
  if (upstream.status < 200 || upstream.status > 599) {
    return errorResponse(502, "invalid_upstream_status");
  }
  return new Response(null, {
    status: upstream.status,
    headers: { "cache-control": "no-store" },
  });
}
