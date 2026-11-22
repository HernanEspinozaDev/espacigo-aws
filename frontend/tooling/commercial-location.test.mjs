import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";
async function load(path, imports = {}, globals = {}) {
  const source = await readFile(
    new URL(`../src/features/host-listings/${path}`, import.meta.url),
    "utf8",
  );
  const javascript = ts.transpile(source, { module: ts.ModuleKind.CommonJS });
  const loaded = { exports: {} };
  new Function(
    "module",
    "exports",
    "require",
    ...Object.keys(globals),
    javascript,
  )(
    loaded,
    loaded.exports,
    (name) => {
      if (name === "client-only") return {};
      assert.ok(name in imports, `Unexpected import ${name}`);
      return imports[name];
    },
    ...Object.values(globals),
  );
  return loaded.exports;
}
const domain = await load("domain/commercial-location.ts");
const valid = {
  address: "Dirección 123",
  region_code: "13",
  comuna_code: "13101",
  latitude: -33.45,
  longitude: -70.66,
  public_confirmed: true,
};
test("pin validation permits unconfirmed drafts and rejects incomplete or nonfinite confirmations", () => {
  assert.doesNotThrow(() => domain.validateLocation(valid));
  assert.doesNotThrow(() =>
    domain.validateLocation({
      ...valid,
      latitude: null,
      longitude: null,
      public_confirmed: false,
    }),
  );
  for (const overrides of [
    { latitude: null },
    { latitude: NaN },
    { longitude: Infinity },
    { latitude: 91 },
    { longitude: -181 },
    { region_code: "" },
    { address: " " },
  ])
    assert.throws(() => domain.validateLocation({ ...valid, ...overrides }));
});
test("location service preserves CAS and never submits an invalid pin", async () => {
  const { CommercialLocationService } = await load(
    "application/commercial-location-service.ts",
    { "../domain/commercial-location": domain },
  );
  let writes = 0;
  const service = new CommercialLocationService({
    save: async (id, input, etag) => {
      writes++;
      assert.equal(id, "space");
      assert.equal(etag, '"revision"');
      assert.deepEqual(input, valid);
      return { etag: '"next"' };
    },
  });
  assert.throws(() =>
    service.save("space", { ...valid, longitude: null }, '"revision"'),
  );
  assert.equal(writes, 0);
  assert.deepEqual(await service.save("space", valid, '"revision"'), {
    etag: '"next"',
  });
  assert.equal(writes, 1);
});
test("generated adapter sends only write fields and carries the shared strong ETag", async () => {
  const location = {
    ...valid,
    space_id: "space",
    revision: 8,
    confirmed_at: "2026-11-21T12:00:00Z",
    updated_at: "2026-11-21T12:00:00Z",
  };
  let payload;
  const { CommercialLocationOpenApiAdapter } = await load(
    "infrastructure/commercial-location-openapi-adapter.ts",
    {
      "@/lib/api/client": {
        apiClient: {
          PUT: async (path, options) => {
            assert.equal(path, "/spaces/{spaceId}/location");
            payload = options;
            return {
              response: new Response(null, {
                status: 200,
                headers: { ETag: '"next"' },
              }),
              data: location,
            };
          },
        },
      },
      "@/lib/api/errors": {
        ApiProtocolError: Error,
        normalizeApiError: () => new Error("request failed"),
      },
      "@/lib/api/exact-json": { serializeExactJson: JSON.stringify },
      "../domain/calendar-configuration": {
        strongEtag: (value) =>
          typeof value === "string" && /^".+"$/.test(value),
      },
    },
  );
  const result = await new CommercialLocationOpenApiAdapter().save(
    "space",
    location,
    '"current"',
  );
  assert.deepEqual(payload.body, valid);
  assert.equal(payload.params.header["If-Match"], '"current"');
  assert.equal(result.etag, '"next"');
});
test("geocoder never implicitly contacts a public provider", async () => {
  let calls = 0;
  const { searchConfiguredAddress } = await load(
    "infrastructure/commercial-geocoder.ts",
    {},
    {
      process: { env: {} },
      fetch: () => {
        calls++;
        throw new Error("unexpected network");
      },
    },
  );
  await assert.rejects(
    () => searchConfiguredAddress("Dirección 123"),
    /no configurada/,
  );
  assert.equal(calls, 0);
});
