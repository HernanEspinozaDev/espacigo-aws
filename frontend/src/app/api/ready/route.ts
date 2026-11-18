import { getApiBackendBaseUrl } from "@/lib/api/server-config";

export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
  let ready = false;
  try {
    const endpoint = new URL(
      "../../health/ready",
      getApiBackendBaseUrl() + "/",
    );
    const upstream = await fetch(endpoint, {
      cache: "no-store",
      redirect: "error",
      signal: AbortSignal.timeout(2000),
    });
    ready = upstream.status === 200;
    await upstream.body?.cancel();
  } catch {
    ready = false;
  }

  return Response.json(
    { status: ready ? "ready" : "unavailable" },
    {
      status: ready ? 200 : 503,
      headers: { "cache-control": "no-store" },
    },
  );
}
