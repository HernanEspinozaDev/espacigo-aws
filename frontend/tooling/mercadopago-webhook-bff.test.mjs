import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(os.tmpdir(), "espacigo-mp-webhook-bff-"),
);
let source = await readFile(
  path.join(root, "src/app/api/v1/webhooks/mercadopago/route.ts"),
  "utf8",
);
source = source
  .replace('import "server-only";', "")
  .replace(
    'import { getApiBackendBaseUrl } from "@/lib/api/server-config";',
    'const getApiBackendBaseUrl = () => new URL("http://127.0.0.1:18082/api/v1");',
  );
const output = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;
const modulePath = path.join(scratch, "route.mjs");
await writeFile(modulePath, output);
const { POST } = await import(pathToFileURL(modulePath).href);

const validHeaders = {
  "content-type": "application/json",
  "x-signature": "ts=123,v1=abc",
  "x-request-id": "mp-request-01",
};

test("forwards raw notification query, bytes and signature headers only", async () => {
  const originalFetch = globalThis.fetch;
  const payload = new Uint8Array([123, 34, 120, 34, 58, 49, 125]);
  let observed;
  globalThis.fetch = async (url, init) => {
    observed = { url: String(url), init };
    return new Response(null, { status: 200 });
  };
  try {
    const request = new Request(
      "http://localhost/api/v1/webhooks/mercadopago?data.id=000%31%32%33&type=payment",
      {
        method: "POST",
        headers: {
          ...validHeaders,
          cookie: "session=must-not-leak",
          origin: "https://attacker.example",
          authorization: "Bearer must-not-leak",
        },
        body: payload,
      },
    );
    const response = await POST(request);

    assert.equal(response.status, 200);
    assert.equal(response.headers.get("set-cookie"), null);
    assert.equal(response.headers.get("location"), null);
    assert.equal(
      observed.url,
      "http://127.0.0.1:18082/api/v1/webhooks/mercadopago?data.id=000%31%32%33&type=payment",
    );
    assert.equal(observed.init.method, "POST");
    assert.equal(observed.init.redirect, "manual");
    assert.equal(observed.init.credentials, "omit");
    assert.equal(observed.init.cache, "no-store");
    assert.deepEqual([...new Uint8Array(observed.init.body)], [...payload]);
    assert.deepEqual(Object.fromEntries(new Headers(observed.init.headers)), {
      "content-type": "application/json",
      "x-request-id": "mp-request-01",
      "x-signature": "ts=123,v1=abc",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("rejects duplicate or unsupported query fields before calling Go", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => {
    assert.fail("invalid requests must not reach Go");
  };
  try {
    for (const query of [
      "data.id=1&data.id=2",
      "data.id=1&type=payment&redirect=https://attacker.example",
    ]) {
      const response = await POST(
        new Request(`http://localhost/api/v1/webhooks/mercadopago?${query}`, {
          method: "POST",
          headers: validHeaders,
          body: "{}",
        }),
      );
      assert.equal(response.status, 400);
    }
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("bounds request body and turns upstream redirects into gateway errors", async () => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = async () => {
    calls += 1;
    return new Response(null, {
      status: 303,
      headers: { location: "https://attacker.example/" },
    });
  };
  try {
    const tooLarge = await POST(
      new Request("http://localhost/api/v1/webhooks/mercadopago", {
        method: "POST",
        headers: { ...validHeaders, "content-length": "65537" },
        body: "{}",
      }),
    );
    assert.equal(tooLarge.status, 413);
    assert.equal(calls, 0);

    const redirect = await POST(
      new Request("http://localhost/api/v1/webhooks/mercadopago?type=payment", {
        method: "POST",
        headers: validHeaders,
        body: "{}",
      }),
    );
    assert.equal(redirect.status, 502);
    assert.equal(redirect.headers.get("location"), null);
    assert.equal(calls, 1);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("preserves upstream error status without forwarding its body or cookies", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    new Response("secret backend detail", {
      status: 503,
      headers: { "set-cookie": "backend=session", location: "/internal" },
    });
  try {
    const response = await POST(
      new Request("http://localhost/api/v1/webhooks/mercadopago?type=payment", {
        method: "POST",
        headers: validHeaders,
        body: "{}",
      }),
    );
    assert.equal(response.status, 503);
    assert.equal(response.headers.get("set-cookie"), null);
    assert.equal(response.headers.get("location"), null);
    assert.equal(await response.text(), "");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test.after(async () => rm(scratch, { recursive: true, force: true }));
