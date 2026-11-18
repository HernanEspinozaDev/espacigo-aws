import assert from "node:assert/strict";
import { readFile, mkdtemp, symlink, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import ts from "typescript";

const folder = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "public-month-adapter-"),
);
await symlink(
  path.resolve("node_modules"),
  path.join(folder, "node_modules"),
  "dir",
);
const source = await readFile(
  "src/features/public-catalog/infrastructure/public-month-openapi-adapter.ts",
  "utf8",
);
const compiled = ts.transpileModule(
  source.replace('import "client-only";', ""),
  {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  },
).outputText;
await writeFile(path.join(folder, "adapter.mjs"), compiled);
const { createPublicMonthAdapter } = await import(
  pathToFileURL(path.join(folder, "adapter.mjs"))
);
test.after(() => rm(folder, { recursive: true, force: true }));

const id = "00000000-0000-4000-8000-000000000099";
const month = "2026-11";
function wire() {
  return {
    month,
    time_zone: "America/Santiago",
    rate_unit: "dia",
    base_price_clp: 35000,
    days: [
      {
        date: "2026-11-17",
        status: "AVAILABLE",
        price_clp: 48000,
        slots: [
          { name: "Horario", available: true, start: "09:00", end: "20:00" },
        ],
      },
    ],
  };
}
function adapter(body, status = 200, observe = () => {}) {
  return createPublicMonthAdapter(async (request) => {
    observe(request);
    return new Response(JSON.stringify(body), {
      status,
      headers: { "content-type": "application/json" },
    });
  }, "http://127.0.0.1:4316/api/backend");
}
function hasAvailableDay(result) {
  return (
    result.ok && result.value.days.some((day) => day.status === "available")
  );
}

test("real monthly wire maps status and effective CLP through the canonical public operation", async () => {
  let request;
  const result = await adapter(wire(), 200, (value) => {
    request = value;
  }).getMonth(id, month);
  assert.equal(result.ok, true);
  assert.equal(
    request.url,
    `http://127.0.0.1:4316/api/backend/public/spaces/${id}/month?month=${month}`,
  );
  assert.equal(request.method, "GET");
  assert.equal(request.cache, "no-store");
  assert.equal(request.credentials, "same-origin");
  assert.equal(request.headers.get("authorization"), null);
  assert.equal(request.headers.get("cookie"), null);
  assert.deepEqual(result.value.days, [
    { date: "2026-11-17", price: 48000, status: "available" },
  ]);
});

test("duplicate dates cannot preserve an earlier available state", async () => {
  const body = wire();
  body.days.push({ ...body.days[0], status: "BLOCKED" });
  assert.equal(hasAvailableDay(await adapter(body).getMonth(id, month)), false);
});

test("prototype property names are not availability statuses", async () => {
  for (const status of ["constructor", "__proto__", "toString"]) {
    const body = wire();
    body.days[0].status = status;
    const result = await adapter(body).getMonth(id, month);
    assert.ok(!result.ok || result.value.days.length === 0);
  }
});

test("nonpositive and unsafe CLP never create an available calendar day", async () => {
  for (const price of [0, -1, Number.MAX_SAFE_INTEGER + 1]) {
    const body = wire();
    body.days[0].price_clp = price;
    assert.equal(
      hasAvailableDay(await adapter(body).getMonth(id, month)),
      false,
    );
  }
  const body = wire();
  body.base_price_clp = 0;
  assert.equal((await adapter(body).getMonth(id, month)).ok, false);
});

test("invalid civil dates and invalid zones fail closed", async () => {
  const body = wire();
  body.days[0].date = "2026-11-31";
  assert.equal(hasAvailableDay(await adapter(body).getMonth(id, month)), false);
  body.time_zone = "Invalid/Zone";
  assert.equal((await adapter(body).getMonth(id, month)).ok, false);
});

test("HTTP failures and an aborted result are not publishable", async () => {
  assert.equal(
    (
      await adapter({ error: { code: "internal_error" } }, 500).getMonth(
        id,
        month,
      )
    ).ok,
    false,
  );
  const controller = new AbortController();
  controller.abort();
  const result = await adapter(wire()).getMonth(id, month, controller.signal);
  assert.deepEqual(result, { ok: false, kind: "aborted" });
});
