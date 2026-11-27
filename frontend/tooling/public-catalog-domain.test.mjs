import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-public-catalog-"),
);

async function compile(sourcePath, outputName, replacements = []) {
  let source = await readFile(path.join(root, sourcePath), "utf8");
  for (const [before, after] of replacements)
    source = source.replaceAll(before, after);
  const output = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText;
  const destination = path.join(scratch, outputName);
  await writeFile(destination, output);
  return destination;
}

await compile("src/lib/api/clp.ts", "clp.mjs");
await compile("src/lib/api/time.ts", "time.mjs");
const verificationPath = await compile(
  "src/features/public-catalog/domain/verification.ts",
  "verification.mjs",
);
const queryPath = await compile(
  "src/features/public-catalog/domain/query-state.ts",
  "query-state.mjs",
  [
    ["@/lib/api/clp", "./clp.mjs"],
    ["@/lib/api/time", "./time.mjs"],
  ],
);
const presentationPath = await compile(
  "src/features/public-catalog/domain/presentation.ts",
  "presentation.mjs",
  [["@/lib/api/clp", "./clp.mjs"]],
);
const mediaPath = await compile(
  "src/features/public-catalog/domain/media.ts",
  "media.mjs",
);
const queryModule = await import(pathToFileURL(queryPath));
const presentationModule = await import(pathToFileURL(presentationPath));
const mediaModule = await import(pathToFileURL(mediaPath));
const verificationModule = await import(pathToFileURL(verificationPath));

test("normalizes optional verification flags strictly and fails closed", () => {
  assert.deepEqual(verificationModule.publicVerification(undefined), {
    identityVerified: false,
    spaceVerified: false,
  });
  assert.deepEqual(
    verificationModule.publicVerification({
      identity_verified: true,
      space_verified: false,
    }),
    {
      identityVerified: true,
      spaceVerified: false,
    },
  );
  assert.deepEqual(
    verificationModule.publicVerification({
      identity_verified: "true",
      space_verified: true,
    }),
    {
      identityVerified: false,
      spaceVerified: false,
    },
  );
  assert.deepEqual(
    verificationModule.publicVerification({ identity_verified: true }),
    {
      identityVerified: true,
      spaceVerified: false,
    },
  );
});

test("allowlists scalar query parameters and preserves opaque cursor values", () => {
  const query = queryModule.parseCatalogQuery({
    q: "  Casa  ",
    cursor: "eyJjdXJzb3IiOiJvcGFxdWUi",
    page: "9",
    unknown: "drop-me",
    category_code: "inmuebles",
    page_size: "24",
    rate_unit: "dia",
  });
  assert.equal(query.q, "Casa");
  assert.equal(query.cursor, "eyJjdXJzb3IiOiJvcGFxdWUi");
  assert.equal(query.page, undefined);
  assert.equal(query.page_size, 24);
  assert.equal(query.unknown, undefined);
  assert.equal(query.category_code, "inmuebles");
});

test("accepts repeated dynamic attributes only against the category profile", () => {
  const filters = [
    { code: "capacity", type: "integer", order: 1, minimum: 1, maximum: 20 },
    { code: "furnished", type: "boolean", order: 2 },
    { code: "dwelling", type: "enum", order: 3, options: ["casa", "cabana"] },
    {
      code: "amenities",
      type: "enum_list",
      order: 4,
      options: ["wifi", "parking"],
    },
  ];
  const query = queryModule.parseCatalogQuery(
    {
      category_code: "inmuebles",
      attribute: [
        "capacity:gte:2",
        "capacity:lte:8",
        "furnished:true",
        "dwelling:eq:casa",
        "amenities:in:wifi,parking",
        "unknown:true",
        "dwelling:eq:office",
      ],
    },
    filters,
  );
  assert.deepEqual(query.attribute, [
    "capacity:gte:2",
    "capacity:lte:8",
    "furnished:true",
    "dwelling:eq:casa",
    "amenities:in:wifi,parking",
  ]);
});

test("rejects ambiguous duplicate attributes and clears cursor after filter changes", () => {
  const filters = [{ code: "furnished", type: "boolean", order: 1 }];
  const query = queryModule.parseCatalogQuery(
    {
      category_code: "inmuebles",
      attribute: ["furnished:true", "furnished:false"],
      cursor: "opaque-token",
    },
    filters,
  );
  assert.deepEqual(query.attribute, []);
  assert.equal(queryModule.withoutCursor(query).cursor, undefined);
  assert.equal(query.cursor, "opaque-token");
});

test("enforces sort context, exact CLP bounds, and offset-aware RFC3339 intervals", () => {
  const unqualified = queryModule.parseCatalogQuery({ sort: "unit_price_asc" });
  assert.equal(unqualified.sort, "unit_price_asc");
  assert.equal(unqualified.rate_unit, "hora");
  const total = queryModule.parseCatalogQuery({
    sort: "unit_price_desc",
    start_at: "2026-11-02T09:00:00-03:00",
    end_at: "2026-11-02T11:00:00-03:00",
  });
  assert.equal(total.sort, "estimated_total_desc");
  assert.equal(total.rate_unit, undefined);
  const qualified = queryModule.parseCatalogQuery({
    rate_unit: "mes",
    sort: "unit_price_asc",
    min_unit_price_clp: "9007199254740993",
    start_at: "2026-11-02T09:00:00-03:00",
    end_at: "2026-11-03T09:00:00-03:00",
  });
  assert.equal(qualified.sort, "unit_price_asc");
  assert.equal(qualified.min_unit_price_clp, "9007199254740993");
  assert.equal(qualified.start_at, "2026-11-02T09:00:00-03:00");
  const invalid = queryModule.parseCatalogQuery({
    min_unit_price_clp: "9223372036854775808",
  });
  assert.equal(invalid.min_unit_price_clp, undefined);
  const noOffset = queryModule.parseCatalogQuery({
    start_at: "2026-11-02T09:00:00",
    end_at: "2026-11-03T09:00:00",
  });
  assert.equal(noOffset.start_at, undefined);
});

test("keeps null CLP totals absent and formats exact amounts", () => {
  assert.equal(presentationModule.formatPublicClp(null), null);
  assert.equal(
    presentationModule.formatPublicClp("9007199254740993"),
    "$9.007.199.254.740.993",
  );
  assert.equal(
    presentationModule.formatPublicClp(Number.MAX_SAFE_INTEGER + 1),
    null,
  );
});

test("builds only same-origin public image paths and supports zero, one, and many images", () => {
  const spaceId = "11111111-1111-4111-8111-111111111111";
  const first = {
    image_id: "22222222-2222-4222-8222-222222222222",
    is_cover: true,
    source: "media",
  };
  const second = {
    image_id: "33333333-3333-4333-8333-333333333333",
    is_cover: false,
    source: "synthetic",
  };
  assert.deepEqual(mediaModule.publicGalleryImages(spaceId, []), []);
  const one = mediaModule.publicGalleryImages(spaceId, [first]);
  assert.equal(one.length, 1);
  assert.equal(
    one[0].src,
    `/api/backend/public/spaces/${spaceId}/images/${first.image_id}/variants/display`,
  );
  assert.equal(
    one[0].fallbackSrc,
    `/api/backend/public/spaces/${spaceId}/images/${first.image_id}`,
  );
  const legacy = mediaModule.publicGalleryImages(spaceId, [
    { image_id: first.image_id, is_cover: true },
  ]);
  assert.equal(
    legacy[0].src,
    `/api/backend/public/spaces/${spaceId}/images/${first.image_id}/variants/display`,
  );
  assert.equal(
    legacy[0].fallbackSrc,
    `/api/backend/public/spaces/${spaceId}/images/${first.image_id}`,
  );
  const many = mediaModule.publicGalleryImages(spaceId, [first, second]);
  assert.equal(many.length, 2);
  assert.equal(
    many[1].src,
    `/api/backend/public/spaces/${spaceId}/images/${second.image_id}`,
  );
  assert.equal(
    mediaModule.publicImageUrl("not-a-uuid", first, "display"),
    null,
  );
  assert.equal(
    mediaModule.publicCoverUrl(
      { space_id: spaceId, cover_image_id: first.image_id, images: [] },
      "thumbnail",
    ),
    `/api/backend/public/spaces/${spaceId}/images/${first.image_id}`,
  );
  assert.equal(
    mediaModule.publicCoverUrl(
      { space_id: spaceId, cover_image_id: "not-a-uuid", images: [] },
      "thumbnail",
    ),
    null,
  );
});

test("geographic query preserves shared filters and only enables distance ordering with proximity", () => {
  const query = queryModule.parseCatalogQuery({
    latitude: "-33.45",
    longitude: "-70.66",
    radius_km: "10",
    sort: "distance_asc",
    q: "Santiago",
  });
  assert.equal(query.sort, "distance_asc");
  assert.equal(queryModule.serializeCatalogQuery(query).get("radius_km"), "10");
  assert.equal(
    queryModule.parseCatalogQuery({ sort: "distance_asc" }).sort,
    undefined,
  );
  const bbox = queryModule.parseCatalogQuery({
    bbox: "-71,-34,-70,-33",
    category_code: "oficina",
  });
  assert.equal(bbox.bbox, "-71,-34,-70,-33");
  assert.equal(queryModule.hasCatalogFilters(bbox), true);
  assert.equal(
    queryModule.parseCatalogQuery({ bbox: "NaN,-34,-70,-33" }).bbox,
    undefined,
  );
  assert.equal(
    queryModule.parseCatalogQuery({
      latitude: "-33",
      longitude: "-70",
      radius_km: "201",
    }).latitude,
    undefined,
  );
});

test("applies only the requested area and keeps capacity and unrelated filters", () => {
  const params = {
    region: "13",
    comuna: "13101",
    lat: "-33.45",
    lng: "-70.66",
    radius: "10",
    bbox: "-71,-34,-70,-33",
    category: "oficina",
    min_capacity: "4",
    min_price: "10000",
    from: "2026-07-02T09:00:00-04:00",
    to: "2026-07-02T11:00:00-04:00",
    cursor: "older-query-cursor",
  };
  const admin = queryModule.parseCatalogQuery({ ...params, areaMode: "admin" });
  assert.equal(admin.comuna_code, "13101");
  assert.equal(admin.latitude, undefined);
  assert.equal(admin.bbox, undefined);
  assert.equal(admin.cursor, undefined);
  assert.equal(admin.min_capacity, "4");
  assert.equal(admin.rate_unit, "hora");
  assert.equal(admin.min_unit_price_clp, "10000");
  assert.equal(admin.category_code, "oficina");
  assert.equal(admin.start_at, params.from);
  assert.equal(
    queryModule.serializeCatalogQuery(admin).get("areaMode"),
    "admin",
  );
  assert.equal(
    queryModule.serializeCatalogApiQuery(admin).has("areaMode"),
    false,
  );

  const bbox = queryModule.parseCatalogQuery(params);
  assert.equal(bbox.areaMode, "bbox");
  assert.equal(bbox.region_code, undefined);
  assert.equal(bbox.latitude, undefined);
  const radius = queryModule.parseCatalogQuery({
    ...params,
    bbox: ",-34,-70,-33",
  });
  assert.equal(radius.areaMode, "radius");
  assert.equal(radius.comuna_code, undefined);
  const cleared = queryModule.withoutCatalogArea({
    ...radius,
    sort: "distance_asc",
  });
  assert.equal(cleared.min_capacity, "4");
  assert.equal(cleared.latitude, undefined);
  assert.equal(cleared.sort, undefined);
});

test("accepts only positive integer minimum capacity within PostgreSQL integer range", () => {
  for (const value of ["0", "-2", "1.5", "1e2", "2147483648", ["1", "2"]]) {
    assert.equal(
      queryModule.parseCatalogQuery({ min_capacity: value }).min_capacity,
      undefined,
    );
  }
  assert.equal(
    queryModule.parseCatalogQuery({ min_capacity: "2147483647" }).min_capacity,
    "2147483647",
  );
});

test("parses, serializes, and forwards verified_only only when true", () => {
  const verified = queryModule.parseCatalogQuery({
    verified_only: "true",
    category_code: "oficina",
  });
  assert.equal(verified.verified_only, true);
  assert.equal(queryModule.hasCatalogFilters(verified), true);
  assert.equal(
    queryModule.serializeCatalogQuery(verified).get("verified_only"),
    "true",
  );
  assert.equal(
    queryModule.serializeCatalogApiQuery(verified).get("verified_only"),
    "true",
  );

  for (const invalid of ["false", "1", "yes", "", ["true", "false"]]) {
    const parsed = queryModule.parseCatalogQuery({ verified_only: invalid });
    assert.equal(parsed.verified_only, undefined);
    assert.equal(queryModule.hasCatalogFilters(parsed), false);
    assert.equal(
      queryModule.serializeCatalogQuery(parsed).has("verified_only"),
      false,
    );
  }

  assert.equal(
    queryModule
      .serializeCatalogQuery({ verified_only: false, attribute: [] })
      .has("verified_only"),
    false,
  );
});

test.after(async () => rm(scratch, { recursive: true, force: true }));
