import "server-only";

import {
  buildAdminJsonResponse,
  extractSanitizedAdminPayload,
  forwardAdminUpstreamError,
  readBoundedJsonRequest,
  validateAdminBffRequest,
} from "@/lib/api/admin-bff-session";
import { readBoundedUpstreamJson } from "@/lib/api/bff-login-validation";
import { errorResponse } from "@/lib/api/server-session";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function POST(request: Request): Promise<Response> {
  const ctx = validateAdminBffRequest(request, {
    isMutation: true,
    requireJsonBody: true,
  });
  if (!ctx.ok) return ctx.response;

  const bodyResult = await readBoundedJsonRequest(request);
  if (!bodyResult.ok) {
    return errorResponse(bodyResult.status, bodyResult.code, ctx.requestId);
  }

  const email =
    typeof bodyResult.data.email === "string"
      ? bodyResult.data.email.trim()
      : "";
  const password =
    typeof bodyResult.data.password === "string" ? bodyResult.data.password : "";
  if (!email || !password) {
    return errorResponse(422, "validation_error", ctx.requestId);
  }

  const upstreamUrl = new URL("/api/v1/admin/session/start", ctx.baseUrl);
  let upstreamRes: Response;
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        accept: "application/json",
        "x-request-id": ctx.requestId,
      },
      body: JSON.stringify({ email, password }),
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

  const cookieToken =
    extracted.data.status === "authenticated" && extracted.token
      ? { token: extracted.token, expiresAt: extracted.expiresAt }
      : null;

  return buildAdminJsonResponse(extracted.data, ctx.requestId, cookieToken);
}
