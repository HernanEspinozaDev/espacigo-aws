import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import ts from "typescript";

const temporary = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "market-detail-"),
);
async function compile(relative, name, replacements = []) {
  let source = await readFile(
    new URL(`../${relative}`, import.meta.url),
    "utf8",
  );
  for (const [from, to] of replacements) source = source.replaceAll(from, to);
  const output = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText;
  const file = path.join(temporary, name);
  await writeFile(file, output);
  return import(pathToFileURL(file));
}
await writeFile(
  path.join(temporary, "server.mjs"),
  'export const getApiBackendBaseUrl = () => new URL("http://127.0.0.1:18082/api/v1");',
);
await compile("src/lib/api/clp.ts", "clp.mjs");
await compile("src/lib/api/time.ts", "time.mjs");
const querySource = await readFile(
  new URL(
    "../src/features/public-catalog/domain/query-state.ts",
    import.meta.url,
  ),
  "utf8",
);
await writeFile(
  path.join(temporary, "query-state.mjs"),
  ts.transpileModule(
    querySource
      .replaceAll("@/lib/api/clp", "./clp.mjs")
      .replaceAll("@/lib/api/time", "./time.mjs"),
    {
      compilerOptions: {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ESNext,
      },
    },
  ).outputText,
);
await writeFile(
  path.join(temporary, "query.mjs"),
  'export { serializeCatalogQuery } from "./query-state.mjs";',
);
const verification = await readFile(
  new URL(
    "../src/features/public-catalog/domain/verification.ts",
    import.meta.url,
  ),
  "utf8",
);
await writeFile(
  path.join(temporary, "verification.mjs"),
  ts.transpileModule(verification, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText,
);
const { goPublicCatalogAdapter: adapter } = await compile(
  "src/features/public-catalog/infrastructure/go-public-catalog-adapter.ts",
  "adapter.mjs",
  [
    ['import "server-only";', ""],
    ["@/lib/api/server-config", "./server.mjs"],
    ["../domain/query-state", "./query.mjs"],
    ["../domain/verification", "./verification.mjs"],
  ],
);
const base = {
  space_id: "11111111-1111-4111-8111-111111111111",
  title: "Test",
  category_code: "x",
  category_name: "X",
  description: "",
  area_m2: 10,
  rate_unit: "dia",
  base_price_clp: "9007199254740993",
  currency: "CLP",
  estimated_total_clp: null,
  usage_rules: "",
  features: [],
  images: [],
};
const host = {
  name: null,
  joined_at: "2020-01-02T03:04:05Z",
  spaces_count: 0,
  identity_verified: false,
};
const attr = {
  code: "capacity",
  label: "Capacidad",
  type: "integer",
  order: 0,
  value: 0,
};
test("real getSpace adapts public detail fields, sanitizes nested objects and rejects invalid capacity", async () => {
  const original = globalThis.fetch;
  const wire = {
    ...base,
    capacity_max: 12,
    host: { ...host, owner_id: "SECRET", nested: { email: "SECRET" } },
    attributes: [
      { ...attr, private: "SECRET" },
      {
        code: "tags",
        label: "Tags",
        type: "enum_list",
        order: 1,
        value: ["a", "b"],
        unknown: { secret: "SECRET" },
      },
    ],
    cancellation_policy: { unknown: "SECRET" },
    guarantee: { secret: "SECRET" },
    owner_id: "SECRET",
  };
  try {
    globalThis.fetch = async () => Response.json({ data: wire });
    const r = await adapter.getSpace(base.space_id);
    assert.equal(r.ok, true);
    assert.equal(r.value.capacity_max, 12);
    assert.deepEqual(r.value.host, host);
    assert.deepEqual(r.value.attributes, [
      attr,
      {
        code: "tags",
        label: "Tags",
        type: "enum_list",
        order: 1,
        value: ["a", "b"],
      },
    ]);
    assert.equal(r.value.cancellation_policy, null);
    assert.equal(r.value.guarantee, null);
    assert.equal(r.value.base_price_clp, "9007199254740993");
    assert.equal(JSON.stringify(r).includes("SECRET"), false);
    for (const bad of [undefined, 0, -1, 1.2, "12"]) {
      globalThis.fetch = async () =>
        Response.json({ data: { ...wire, capacity_max: bad } });
      const invalid = await adapter.getSpace(base.space_id);
      assert.deepEqual(invalid, {
        ok: false,
        error: { kind: "unavailable", status: 502 },
      });
    }
  } finally {
    globalThis.fetch = original;
  }
});
test("nested public fields reject invalid types while preserving false and zero", async () => {
  const original = globalThis.fetch;
  const valid = [
    {
      code: "boolean",
      label: "Boolean",
      type: "boolean",
      order: 1,
      value: false,
    },
    { code: "integer", label: "Integer", type: "integer", order: 2, value: 0 },
    {
      code: "number",
      label: "Number",
      type: "number",
      order: 3,
      unit: "m",
      value: 0.5,
    },
    {
      code: "enum",
      label: "Enum",
      type: "enum",
      order: 4,
      value: "public_option",
    },
    {
      code: "list",
      label: "List",
      type: "enum_list",
      order: 5,
      value: ["public_option"],
    },
  ];
  const invalid = [
    { ...valid[0], code: "bad_boolean", value: "false" },
    { ...valid[1], code: "bad_integer", value: 0.5 },
    { ...valid[1], code: "unsafe_integer", value: Number.MAX_SAFE_INTEGER + 1 },
    { ...valid[2], code: "bad_number", value: { private: "SECRET" } },
    { ...valid[3], code: "bad_enum", value: null },
    { ...valid[4], code: "bad_list", value: [{ email: "SECRET" }] },
    {
      ...valid[0],
      code: "bad_type",
      type: "private_json",
      value: { private: "SECRET" },
    },
  ];
  try {
    globalThis.fetch = async () =>
      Response.json({
        data: {
          ...base,
          capacity_max: 12,
          host,
          attributes: [...valid].reverse().concat(invalid),
        },
      });
    const result = await adapter.getSpace(base.space_id);
    assert.equal(result.ok, true);
    assert.deepEqual(result.value.attributes, valid);
    assert.deepEqual(result.value.host, host);
    assert.equal(JSON.stringify(result).includes("SECRET"), false);
    for (const badHost of [
      null,
      {},
      { ...host, name: 1 },
      { ...host, joined_at: "not-a-date" },
      { ...host, spaces_count: -1 },
      { ...host, identity_verified: "true" },
    ]) {
      globalThis.fetch = async () =>
        Response.json({ data: { ...base, capacity_max: 12, host: badHost } });
      const invalidHost = await adapter.getSpace(base.space_id);
      assert.equal(invalidHost.ok, true);
      assert.equal(invalidHost.value.host, null);
      assert.deepEqual(invalidHost.value.attributes, []);
      assert.equal(invalidHost.value.cancellation_policy, null);
      assert.equal(invalidHost.value.guarantee, null);
    }
  } finally {
    globalThis.fetch = original;
  }
});

test.after(() => rm(temporary, { recursive: true, force: true }));
