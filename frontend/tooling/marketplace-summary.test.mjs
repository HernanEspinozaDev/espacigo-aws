import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../src",
);
const summary = await readFile(
  path.join(root, "features/public-catalog/ui/public-space-summary.tsx"),
  "utf8",
);

test("summary renders detail-only declared fields and explicit unknown states", () => {
  assert.match(summary, /space:\s*PublicSpaceDetail/);
  assert.match(summary, /space\.capacity_max\s*>\s*0/);
  assert.match(summary, /Hasta \$\{space\.capacity_max\} personas/);
  assert.match(summary, /Horario informado/);
  assert.match(summary, /Horario no informado/);
  assert.match(summary, /space\.cancellation_policy\s*===\s*null/);
  assert.match(summary, /Política de cancelación no informada/);
  assert.match(summary, /Garantía no informada/);
  assert.doesNotMatch(summary, /50000|Number\s*\(\s*space\.base_price_clp/);
  assert.match(summary, /publicPrice\(space:\s*PublicSpace\)/);
});

test("summary keeps disclosure, badge, exact formatter and wrap-safe category link", () => {
  assert.match(
    summary,
    /PublicVerificationBadges verification=\{space\.verification\}/,
  );
  assert.match(summary, /formatPublicClp\(space\.base_price_clp\)/);
  assert.match(summary, /aria-expanded=\{expanded\}/);
  assert.match(summary, /Ver \{expanded \? "menos" : "más"\}/);
  assert.match(summary, /min-h-11 min-w-0/);
  assert.match(summary, /overflow-wrap:anywhere/);
});
