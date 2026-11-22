import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(os.tmpdir(), "espacigo-mp-oauth-callback-bff-"),
);
let source = await readFile(
  path.join(
    root,
    "src/app/api/backend/host/mercadopago/oauth/callback/route.ts",
  ),
  "utf8",
);
source = source.replace('import "server-only";', "").replace(
  `import {
  getApiBackendBaseUrl,
  getFrontendPublicOrigin,
} from "@/lib/api/server-config";`,
  `const getApiBackendBaseUrl = () => new URL("http://127.0.0.1:18082/api/v1");
const getFrontendPublicOrigin = () => new URL("https://prueba-mercadopago.testingpage.store");`,
);
const output = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;
const modulePath = path.join(scratch, "route.mjs");
await writeFile(modulePath, output);
const { GET } = await import(pathToFileURL(modulePath).href);

test("accepts Go callback redirect using configured public origin behind proxy", async (t) => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    new Response(null, {
      status: 303,
      headers: {
        location:
          "https://prueba-mercadopago.testingpage.store/anfitrion/mercado-pago?connection=invalid_state",
      },
    });
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const response = await GET(
    new Request(
      "http://127.0.0.1:3000/api/backend/host/mercadopago/oauth/callback?error=access_denied&state=synthetic-state",
      {
        headers: {
          host: "prueba-mercadopago.testingpage.store",
          "x-forwarded-host": "prueba-mercadopago.testingpage.store",
          "x-forwarded-proto": "https",
        },
      },
    ),
  );

  assert.equal(response.status, 303);
  assert.equal(
    response.headers.get("location"),
    "/anfitrion/mercado-pago?connection=invalid_state",
  );
  assert.equal(response.headers.get("set-cookie"), null);
});

test("rejects upstream redirects outside configured public origin and return enum", async (t) => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    new Response(null, {
      status: 303,
      headers: {
        location:
          "https://attacker.example/anfitrion/mercado-pago?connection=connected",
      },
    });
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const response = await GET(
    new Request(
      "http://127.0.0.1:3000/api/backend/host/mercadopago/oauth/callback?error=access_denied&state=synthetic-state",
    ),
  );

  assert.equal(response.status, 502);
  assert.equal(response.headers.get("location"), null);
  assert.equal((await response.json()).error.code, "invalid_upstream_redirect");
});

test.after(async () => rm(scratch, { recursive: true, force: true }));
