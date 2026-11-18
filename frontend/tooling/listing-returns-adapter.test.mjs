import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { pathToFileURL } from "node:url";

const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-returns-adapter-"),
);

const adapterSource = await readFile(
  "src/features/host-listings/infrastructure/listing-returns-openapi-adapter.ts",
  "utf8",
);

// Transpile with a stubbed apiClient
const transpiled = ts.transpileModule(
  adapterSource.replace('import { apiClient } from "@/lib/api/client";', ""),
  {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  },
).outputText;

// Wrap to provide mock apiClient in module scope
const moduleCode = `
export const mockState = {
  get: async () => ({})
};
export const apiClient = {
  GET: (...args) => mockState.get(...args),
  POST: () => Promise.resolve({ data: {}, error: null }),
};
${transpiled}
`;

const modulePath = path.join(scratch, "adapter.mjs");
await writeFile(modulePath, moduleCode, "utf8");

const imported = await import(pathToFileURL(modulePath).href);
const { ListingReturnsOpenApiAdapter, mockState } = imported;

test.after(async () => {
  await rm(scratch, { recursive: true, force: true });
});

test("listReturns: respuesta 200 con lista vacía retorna items vacíos sin error", async () => {
  mockState.get = async () => ({
    data: { data: { space_id: "s-1", items: [] } },
    error: undefined,
    response: { status: 200 },
  });
  const adapter = new ListingReturnsOpenApiAdapter();
  const res = await adapter.listReturns("token-1", "s-1");
  assert.equal(res.space_id, "s-1");
  assert.deepEqual(res.items, []);
});

test("listReturns: respuesta 404 retorna items vacíos sin error", async () => {
  mockState.get = async () => ({
    data: undefined,
    error: { error: { code: "not_found" } },
    response: { status: 404 },
  });
  const adapter = new ListingReturnsOpenApiAdapter();
  const res = await adapter.listReturns("token-1", "s-1");
  assert.equal(res.space_id, "s-1");
  assert.deepEqual(res.items, []);
});

test("listReturns: respuesta 200 con items retorna los elementos", async () => {
  const sampleItem = {
    reservation_id: "r-1",
    state: "finalizada",
    start_at: "2026-01-01T00:00:00Z",
    end_at: "2026-01-02T00:00:00Z",
    check_out_recorded: true,
    vacancy_confirmed: false,
  };
  mockState.get = async () => ({
    data: { data: { space_id: "s-1", items: [sampleItem] } },
    error: undefined,
    response: { status: 200 },
  });
  const adapter = new ListingReturnsOpenApiAdapter();
  const res = await adapter.listReturns("token-1", "s-1");
  assert.equal(res.space_id, "s-1");
  assert.equal(res.items.length, 1);
  assert.equal(res.items[0].reservation_id, "r-1");
});

test("listReturns: error de servidor 500 lanza 'No fue posible cargar entregas.'", async () => {
  mockState.get = async () => ({
    data: undefined,
    error: { error: { code: "internal_error" } },
    response: { status: 500 },
  });
  const adapter = new ListingReturnsOpenApiAdapter();
  await assert.rejects(
    () => adapter.listReturns("token-1", "s-1"),
    (err) => {
      assert.match(err.message, /No fue posible cargar entregas\./);
      return true;
    },
  );
});

test("listReturns: fallo de red sin response lanza 'No fue posible cargar entregas.'", async () => {
  mockState.get = async () => ({
    data: undefined,
    error: new Error("Network failure"),
    response: undefined,
  });
  const adapter = new ListingReturnsOpenApiAdapter();
  await assert.rejects(
    () => adapter.listReturns("token-1", "s-1"),
    (err) => {
      assert.match(err.message, /No fue posible cargar entregas\./);
      return true;
    },
  );
});
