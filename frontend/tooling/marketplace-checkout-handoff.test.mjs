import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const require = createRequire(import.meta.url);
async function load(path, aliases = {}) {
  const url = new URL(path, import.meta.url);
  const source = await readFile(url, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
  const mod = { exports: {} };
  const localRequire = (id) => {
    if (Object.hasOwn(aliases, id)) return aliases[id];
    if (id === "@/lib/api/clp") return require("../src/lib/api/clp.ts");
    if (id === "./media")
      return require("../src/features/public-catalog/domain/media.ts");
    if (id === "./availability-return")
      return require("../src/features/public-catalog/domain/availability-return.ts");
    if (id === "@/features/public-catalog/domain/query-state")
      return {
        parseCatalogQuery: () => ({}),
        serializeCatalogQuery: () => new URLSearchParams(),
      };
    if (id === "@/features/public-catalog/domain/marketplace-booking-selection")
      return aliases.marketplace;
    return require(id);
  };
  new Function("require", "module", "exports", compiled)(
    localRequire,
    mod,
    mod.exports,
  );
  return mod.exports;
}

test("return target preserves only a validated canonical marketplace handoff", async () => {
  const availability = await load(
    "../src/features/public-catalog/domain/availability-return.ts",
  );
  const marketplace = await load(
    "../src/features/public-catalog/domain/marketplace-booking-selection.ts",
    { "./availability-return": availability },
  );
  const { resolveReturnTarget } = await load(
    "../src/features/auth/domain/return-target.ts",
    {
      marketplace,
      "@/features/public-catalog/domain/availability-return": availability,
      "@/features/public-catalog/domain/marketplace-booking-selection":
        marketplace,
    },
  );
  const id = "123e4567-e89b-42d3-a456-426614174000";
  const query = `spaceId=${id}&date=2026-12-01&duration=2&start=2026-12-01T09%3A00%3A00-03%3A00&end=2026-12-01T11%3A00%3A00-03%3A00&rate_unit=hora&time_zone=America%2FSantiago&source=marketplace`;
  const canonical = `/reservas/nueva?${query}`;
  assert.equal(resolveReturnTarget(canonical), canonical);
  assert.equal(resolveReturnTarget(`/reservas/nueva?${query}&extra=x`), "/");
  assert.equal(resolveReturnTarget(`/reservas/nueva?${query}&duration=2`), "/");
  assert.equal(
    resolveReturnTarget(`/reservas/nueva?${query}&source=marketplace`),
    "/",
  );
  assert.equal(
    resolveReturnTarget(
      `/reservas/nueva?${query}&return_to=https%3A%2F%2Fevil.test`,
    ),
    "/",
  );
  assert.equal(
    resolveReturnTarget("/reservas/nueva?space_id=legacy&title=Prueba"),
    "/",
  );
  assert.equal(resolveReturnTarget("/reservas/nueva"), "/reservas/nueva");
});
