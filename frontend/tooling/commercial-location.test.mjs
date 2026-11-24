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
test("address replacement respects the selected anchor and manual edits", () => {
  const policy = domain.shouldApplyPinAddressSuggestion;
  const state = {
    distanceFromAddressAnchorMeters: 30,
    editedDuringResolution: false,
    verifyingExisting: false,
  };
  assert.equal(policy(state), false);
  assert.equal(
    policy({ ...state, distanceFromAddressAnchorMeters: 30.1 }),
    true,
  );
  assert.equal(
    policy({
      ...state,
      distanceFromAddressAnchorMeters: 100,
      editedDuringResolution: true,
    }),
    false,
  );
  assert.equal(
    policy({
      ...state,
      distanceFromAddressAnchorMeters: 100,
      verifyingExisting: true,
    }),
    false,
  );
});
test("pin validation requires complete coordinates even without public confirmation", () => {
  assert.doesNotThrow(() => domain.validateLocation(valid));
  assert.throws(() =>
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
  await new CommercialLocationOpenApiAdapter().save(
    "space",
    { ...valid, source: "geocoded", precision: "approximate" },
    '"current"',
  );
  assert.deepEqual(payload.body, {
    ...valid,
    source: "geocoded",
    precision: "approximate",
  });
  await new CommercialLocationOpenApiAdapter().save(
    "space",
    { ...valid, source: "manual", precision: "approximate" },
    '"current"',
  );
  assert.deepEqual(payload.body, { ...valid, source: "manual" });
});
test("geocoder sends a structured authenticated request and filters invalid pins", async () => {
  let request;
  const { searchCommercialAddress } = await load(
    "infrastructure/commercial-geocoder.ts",
    {
      "@/lib/api/client": {
        apiClient: {
          POST: async (path, options) => {
            request = { path, options };
            return {
              response: new Response(null, { status: 200 }),
              data: {
                candidates: [
                  {
                    label: "Dirección 123",
                    latitude: -33.45,
                    longitude: -70.66,
                    precision: "approximate",
                  },
                  {
                    label: "Inválida",
                    latitude: 91,
                    longitude: -70.66,
                    precision: "address",
                  },
                ],
              },
            };
          },
        },
      },
      "@/lib/api/errors": {
        ApiProtocolError: Error,
        normalizeApiError: () => new Error("request failed"),
      },
    },
  );
  const query = {
    address: "Dirección 123",
    region_code: "13",
    comuna_code: "13101",
  };
  const controller = new AbortController();
  const candidates = await searchCommercialAddress(query, controller.signal);
  assert.equal(request.path, "/spaces/location/geocode");
  assert.deepEqual(request.options.body, query);
  assert.equal(request.options.signal, controller.signal);
  assert.deepEqual(candidates, [
    {
      label: "Dirección 123",
      latitude: -33.45,
      longitude: -70.66,
      precision: "approximate",
    },
  ]);
});
test("geocoder reports API failures without contacting an external provider", async () => {
  const { searchCommercialAddress } = await load(
    "infrastructure/commercial-geocoder.ts",
    {
      "@/lib/api/client": {
        apiClient: {
          POST: async () => ({
            response: new Response(null, { status: 503 }),
            error: {},
          }),
        },
      },
      "@/lib/api/errors": {
        ApiProtocolError: Error,
        normalizeApiError: (response) =>
          new Error(`service ${response.status}`),
      },
    },
  );
  await assert.rejects(
    () =>
      searchCommercialAddress({
        address: "Dirección 123",
        region_code: "13",
        comuna_code: "13101",
      }),
    /service 503/,
  );
});
test("reverse geocoding sends only pin coordinates and accepts a partial location", async () => {
  let request;
  const partial = { region_code: "13", comuna_code: "13101" };
  const { reverseGeocodeCommercialPin } = await load(
    "infrastructure/commercial-geocoder.ts",
    {
      "@/lib/api/client": {
        apiClient: {
          POST: async (path, options) => {
            request = { path, options };
            return { response: new Response(null), data: partial };
          },
        },
      },
      "@/lib/api/errors": { ApiProtocolError: Error },
    },
  );
  const { signal } = new AbortController();
  assert.deepEqual(
    await reverseGeocodeCommercialPin(-33.45, -70.66, signal),
    partial,
  );
  assert.deepEqual(request, {
    path: "/spaces/location/reverse-geocode",
    options: { body: { latitude: -33.45, longitude: -70.66 }, signal },
  });
});

test("commune viewport uses the authenticated API and tolerates missing boundaries", async () => {
  const requests = [];
  const { getCommercialCommuneViewport } = await load(
    "infrastructure/commercial-geocoder.ts",
    {
      "@/lib/api/client": {
        apiClient: {
          GET: async (path, options) => {
            requests.push({ path, options });
            return options.params.path.communeCode === "13101"
              ? {
                  response: new Response(null, { status: 200 }),
                  data: {
                    west: -70.9,
                    south: -33.7,
                    east: -70.4,
                    north: -33.2,
                  },
                }
              : { response: new Response(null, { status: 404 }) };
          },
        },
      },
      "@/lib/api/errors": {
        ApiProtocolError: Error,
        normalizeApiError: () => new Error("request failed"),
      },
    },
  );
  const controller = new AbortController();
  assert.deepEqual(
    await getCommercialCommuneViewport("13101", controller.signal),
    { west: -70.9, south: -33.7, east: -70.4, north: -33.2 },
  );
  assert.equal(await getCommercialCommuneViewport("99999"), null);
  assert.equal(requests[0].path, "/spaces/location/communes/{communeCode}");
  assert.equal(requests[0].options.params.path.communeCode, "13101");
  assert.equal(requests[0].options.signal, controller.signal);
});
