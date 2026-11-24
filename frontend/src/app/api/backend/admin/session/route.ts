import "server-only";

import {
  buildAdminJsonResponse,
  extractSanitizedAdminPayload,
  forwardAdminUpstreamError,
  validateAdminBffRequest,
} from "@/lib/api/admin-bff-session";
import { readBoundedUpstreamJson } from "@/lib/api/bff-login-validation";
import {
  buildClearAdminSessionCookie,
  errorResponse,
  parseAdminSessionCookie,
} from "@/lib/api/server-session";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(request: Request): Promise<Response> {
  const ctx = validateAdminBffRequest(request, { isMutation: false });
  if (!ctx.ok) return ctx.response;

  const adminToken = parseAdminSessionCookie(request.headers.get("cookie"));
  if (!adminToken) {
    const res = errorResponse(401, "unauthenticated", ctx.requestId);
    res.headers.append("set-cookie", buildClearAdminSessionCookie());
    return res;
  }

  const upstreamUrl = new URL("/api/v1/admin/session", ctx.baseUrl);
  let upstreamRes: Response;
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: "GET",
      headers: {
        accept: "application/json",
        authorization: `Bearer ${adminToken}`,
        "x-request-id": ctx.requestId,
      },
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(30_000)]),
    });
  } catch {
    return errorResponse(502, "api_unavailable", ctx.requestId);
  }

  if (upstreamRes.status >= 300 && upstreamRes.status < 400) {
    await upstreamRes.body?.cancel().catch(() => undefined);
    return errorResponse(502, "invalid_upstream_response", ctx.requestId);
  }

  if (upstreamRes.status === 401) {
    await upstreamRes.body?.cancel().catch(() => undefined);
    const res = errorResponse(401, "unauthenticated", ctx.requestId);
    res.headers.append("set-cookie", buildClearAdminSessionCookie());
    return res;
  }

  if (!upstreamRes.ok) {
    return forwardAdminUpstreamError(upstreamRes, ctx.requestId);
  }

  const jsonResult = await readBoundedUpstreamJson(upstreamRes);
  if (!jsonResult.ok) {
    return errorResponse(502, "invalid_upstream_response", ctx.requestId);
  }

  const extracted = extractSanitizedAdminPayload(jsonResult.data);
  if (!extracted) {
    return errorResponse(502, "invalid_upstream_response", ctx.requestId);
  }

  return buildAdminJsonResponse(extracted.data, ctx.requestId);
}

export async function DELETE(request: Request): Promise<Response> {
  const ctx = validateAdminBffRequest(request, { isMutation: true });
  if (!ctx.ok) return ctx.response;

  const clearCookie = buildClearAdminSessionCookie();
  const adminToken = parseAdminSessionCookie(request.headers.get("cookie"));
  if (!adminToken) {
    const headers = new Headers({
      "cache-control": "private, no-store",
      "x-request-id": ctx.requestId,
    });
    headers.append("set-cookie", clearCookie);
    return new Response(null, { status: 204, headers });
  }

  const upstreamUrl = new URL("/api/v1/admin/session", ctx.baseUrl);
  let upstreamRes: Response;
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: "DELETE",
      headers: {
        accept: "application/json",
        authorization: `Bearer ${adminToken}`,
        "x-request-id": ctx.requestId,
      },
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(30_000)]),
    });
  } catch {
    const res = errorResponse(502, "api_unavailable", ctx.requestId);
    res.headers.append("set-cookie", clearCookie);
    return res;
  }

  if (upstreamRes.status === 204 || upstreamRes.status === 401) {
    await upstreamRes.body?.cancel().catch(() => undefined);
    const headers = new Headers({
      "cache-control": "private, no-store",
      "x-request-id": ctx.requestId,
    });
    headers.append("set-cookie", clearCookie);
    return new Response(null, { status: 204, headers });
  }

  const errRes = await forwardAdminUpstreamError(upstreamRes, ctx.requestId);
  errRes.headers.append("set-cookie", clearCookie);
  return errRes;
}
