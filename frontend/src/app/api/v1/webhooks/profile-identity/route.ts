import "server-only";
import { getApiBackendBaseUrl } from "@/lib/api/server-config";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const maximumBytes = 1024 * 1024;
const signatureHeaders = [
  "x-timestamp",
  "x-signature-v2",
  "x-signature-v2-previous",
  "x-signature",
  "x-signature-previous",
] as const;

function respond(status: number) {
  return new Response(null, {
    status,
    headers: { "cache-control": "no-store" },
  });
}

export async function POST(request: Request) {
  if (new URL(request.url).search || !request.body) return respond(400);
  const headers = new Headers({ "content-type": "application/json" });
  for (const name of signatureHeaders) {
    const value = request.headers.get(name);
    if (value && value.length <= 128) headers.set(name, value);
  }
  if (
    !headers.has("x-timestamp") ||
    !signatureHeaders.slice(1).some((name) => headers.has(name))
  ) {
    return respond(400);
  }

  const reader = request.body.getReader();
  const signal = AbortSignal.any([request.signal, AbortSignal.timeout(12_000)]);
  const cancel = () => void reader.cancel().catch(() => undefined);
  signal.addEventListener("abort", cancel, { once: true });
  try {
    const buffer = new Uint8Array(maximumBytes);
    let size = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (size + value.byteLength > maximumBytes) {
        cancel();
        return respond(413);
      }
      buffer.set(value, size);
      size += value.byteLength;
    }
    if (signal.aborted) return respond(408);
    if (!size) return respond(400);
    const base = getApiBackendBaseUrl();
    const destination = new URL(
      `${base.pathname.replace(/\/$/, "")}/webhooks/profile-identity`,
      base,
    );
    const upstream = await fetch(destination, {
      method: "POST",
      body: buffer.slice(0, size),
      headers,
      signal,
      cache: "no-store",
      credentials: "omit",
      redirect: "error",
    });
    await upstream.body?.cancel().catch(() => undefined);
    return respond(
      upstream.status === 204 ? 204 : upstream.status === 400 ? 400 : 503,
    );
  } catch {
    return respond(503);
  } finally {
    signal.removeEventListener("abort", cancel);
    reader.releaseLock();
  }
}
