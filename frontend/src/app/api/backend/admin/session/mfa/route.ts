import "server-only";

import {
  buildAdminJsonResponse,
  extractSanitizedAdminPayload,
  forwardAdminUpstreamError,
  readBoundedJsonRequest,
  validateAdminBffRequest,
} from "@/lib/api/admin-bff-session";
import { readBoundedUpstreamJson } from "@/lib/api/bff-login-validation";
import {
  errorResponse,
  parseAdminSessionCookie,
} from "@/lib/api/server-session";

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

  const code =
    typeof bodyResult.data.code === "string" ? bodyResult.data.code.trim() : "";
  const challengeId =
    typeof bodyResult.data.challenge_id === "string"
      ? bodyResult.data.challenge_id.trim()
      : undefined;

  if (!code) {
    return errorResponse(422, "validation_error", ctx.requestId);
  }

  const existingAdminToken = parseAdminSessionCookie(
    request.headers.get("cookie"),
  );
  if (!challengeId && !existingAdminToken) {
    return errorResponse(401, "unauthenticated", ctx.requestId);
  }

  const headers = new Headers({
    "content-type": "application/json",
    accept: "application/json",
    "x-request-id": ctx.requestId,
  });
  if (existingAdminToken) {
    headers.set("authorization", `Bearer ${existingAdminToken}`);
  }

  const payload = challengeId ? { challenge_id: challengeId, code } : { code };
  const upstreamUrl = new URL("/api/v1/admin/session/mfa", ctx.baseUrl);
  let upstreamRes: Response;
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
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

  const extracted = extractSanitizedAdminPayload(jsonResult.data, {
    stripStatus: true,
  });
  if (!extracted) {
    return errorResponse(502, "invalid_upstream_response", ctx.requestId);
  }

  const cookieToken = extracted.token
    ? { token: extracted.token, expiresAt: extracted.expiresAt }
    : null;

  return buildAdminJsonResponse(extracted.data, ctx.requestId, cookieToken);
}
