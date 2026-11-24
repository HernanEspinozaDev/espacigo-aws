import "server-only";

import { getApiBackendBaseUrl } from "@/lib/api/server-config";
import {
  buildAdminSessionCookie,
  errorResponse,
  validateCsrfOrigin,
} from "@/lib/api/server-session";
import {
  readBoundedUpstreamJson,
  safeErrorCodePattern,
  safeOpaqueToken,
  sanitizeRetryAfter,
} from "@/lib/api/bff-login-validation";

const safeRequestId = /^[A-Za-z0-9._-]{1,128}$/;

export function validateAdminBffRequest(
  request: Request,
  options: { isMutation: boolean; requireJsonBody?: boolean },
):
  | { ok: true; requestId: string; baseUrl: URL }
  | { ok: false; response: Response } {
  const rawRequestId = request.headers.get("x-request-id");
  const requestId = rawRequestId ?? crypto.randomUUID();
  if (!safeRequestId.test(requestId)) {
    return {
      ok: false,
      response: errorResponse(400, "invalid_request_id", crypto.randomUUID()),
    };
  }

  if (request.headers.has("authorization")) {
    return {
      ok: false,
      response: errorResponse(400, "authorization_not_allowed", requestId),
    };
  }

  if (!validateCsrfOrigin(request, { isMutation: options.isMutation })) {
    return {
      ok: false,
      response: errorResponse(403, "origin_not_allowed", requestId),
    };
  }

  if (options.requireJsonBody) {
    const contentType = request.headers.get("content-type");
    const mediaType = contentType?.split(";", 1)[0].trim().toLowerCase();
    if (mediaType !== "application/json") {
      return {
        ok: false,
        response: errorResponse(415, "content_type_not_allowed", requestId),
      };
    }
  }

  try {
    const baseUrl = getApiBackendBaseUrl();
    return { ok: true, requestId, baseUrl };
  } catch {
    return {
      ok: false,
      response: errorResponse(503, "api_not_configured", requestId),
    };
  }
}

export async function readBoundedJsonRequest(
  request: Request,
  maxBytes = 16 * 1024,
): Promise<
  | { ok: true; data: Record<string, unknown> }
  | { ok: false; status: number; code: string }
> {
  const rawLength = request.headers.get("content-length");
  if (rawLength !== null) {
    if (!/^\d+$/.test(rawLength)) {
      return { ok: false, status: 400, code: "invalid_body" };
    }
    if (Number(rawLength) > maxBytes) {
      return { ok: false, status: 413, code: "too_large" };
    }
  }

  if (!request.body) {
    return { ok: false, status: 400, code: "invalid_body" };
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (value) {
        total += value.byteLength;
        if (total > maxBytes) {
          await reader.cancel();
          return { ok: false, status: 413, code: "too_large" };
        }
        chunks.push(value);
      }
    }
  } finally {
    reader.releaseLock();
  }

  try {
    const combined = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      combined.set(chunk, offset);
      offset += chunk.byteLength;
    }
    const parsed = JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(combined),
    );
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ok: false, status: 400, code: "invalid_body" };
    }
    return { ok: true, data: parsed as Record<string, unknown> };
  } catch {
    return { ok: false, status: 400, code: "invalid_body" };
  }
}

export async function forwardAdminUpstreamError(
  upstream: Response,
  requestId: string,
): Promise<Response> {
  const status = upstream.status;
  let code = `http_${status}`;
  const ct = upstream.headers
    .get("content-type")
    ?.split(";", 1)[0]
    .trim()
    .toLowerCase();
  if (ct === "application/json") {
    const parsed = await readBoundedUpstreamJson(upstream);
    if (parsed.ok) {
      const rawCode = (parsed.data as { error?: { code?: unknown } } | null)
        ?.error?.code;
      if (
        status < 500 &&
        typeof rawCode === "string" &&
        safeErrorCodePattern.test(rawCode)
      ) {
        code = rawCode;
      }
    }
  } else {
    await upstream.body?.cancel().catch(() => undefined);
  }

  const retryAfter = sanitizeRetryAfter(upstream.headers.get("retry-after"));
  const extraHeaders: Record<string, string> = {};
  if (retryAfter) extraHeaders["retry-after"] = retryAfter;
  return errorResponse(status, code, requestId, extraHeaders);
}

export function extractSanitizedAdminPayload(
  rawJson: unknown,
  options?: { stripStatus?: boolean },
): {
  data: Record<string, unknown>;
  token: string | null;
  expiresAt: string | undefined;
} | null {
  if (!rawJson || typeof rawJson !== "object" || Array.isArray(rawJson)) {
    return null;
  }
  const envelope = rawJson as { data?: unknown };
  const rawData =
    envelope.data &&
    typeof envelope.data === "object" &&
    !Array.isArray(envelope.data)
      ? (envelope.data as Record<string, unknown>)
      : (rawJson as Record<string, unknown>);

  let token: string | null = null;
  for (const key of ["access_token", "token"] as const) {
    const candidate = rawData[key];
    if (typeof candidate === "string" && safeOpaqueToken.test(candidate)) {
      token = candidate;
      break;
    }
  }

  const sanitized: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(rawData)) {
    if (
      key === "access_token" ||
      key === "token" ||
      key === "refresh_token" ||
      key === "token_type" ||
      (options?.stripStatus && key === "status")
    ) {
      continue;
    }
    sanitized[key] = value;
  }

  const expiresAt =
    typeof sanitized.expires_at === "string" ? sanitized.expires_at : undefined;

  return { data: sanitized, token, expiresAt };
}

export function buildAdminJsonResponse(
  data: Record<string, unknown>,
  requestId: string,
  cookieToken?: { token: string; expiresAt?: string } | null,
): Response {
  const headers = new Headers({
    "cache-control": "no-store",
    "content-type": "application/json; charset=utf-8",
    "x-request-id": requestId,
  });
  if (cookieToken) {
    headers.append(
      "set-cookie",
      buildAdminSessionCookie(cookieToken.token, cookieToken.expiresAt),
    );
  }
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers,
  });
}
