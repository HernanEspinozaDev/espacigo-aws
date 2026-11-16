import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../src",
);
const read = (relative) => readFile(path.join(root, relative), "utf8");

test("public space upper section composes summary, rental calendar and gallery in three desktop columns", async () => {
  const page = await read("app/(marketplace)/espacios/[id]/page.tsx");
  const layout = await read(
    "features/public-catalog/ui/public-space-marketplace-layout.tsx",
  );
  assert.match(page, /PublicSpaceMarketplaceLayout/);
  assert.match(page, /PublicSpaceSummary/);
  assert.match(page, /PublicSpaceMarketplaceGallery/);
  assert.match(page, /PublicSpaceRentalAvailability/);
  assert.match(layout, /lg:grid-cols-12/);
  assert.equal((layout.match(/lg:col-span-4/g) ?? []).length, 3);
});

test("summary exposes a keyboard-operable description disclosure and wrap-safe content", async () => {
  const summary = await read(
    "features/public-catalog/ui/public-space-summary.tsx",
  );
  assert.match(summary, /más/);
  assert.match(summary, /menos/);
  assert.match(summary, /aria-expanded/);
  assert.match(summary, /overflow-wrap:anywhere/);
  assert.match(summary, /min-w-0/);
});

test("gallery uses native modal, initial close focus, honest image fallback, and keyboard navigation", async () => {
  const gallery = await read(
    "features/public-catalog/ui/public-space-gallery.tsx",
  );
  assert.match(gallery, /1\//);
  assert.match(gallery, /showModal\(\)/);
  assert.match(gallery, /closeRef\.current\?\.focus\(\)/);
  assert.match(gallery, /opener\.current\?\.focus\(\)/);
  assert.match(gallery, /<dialog/);
  assert.match(gallery, /ArrowRight/);
  assert.match(gallery, /ArrowLeft/);
  assert.match(gallery, /fallbackSrc/);
  assert.match(gallery, /count > 4/);
  assert.equal(
    (gallery.match(/<PublicMediaImage[\s\S]*?unoptimized/g) ?? []).length,
    3,
  );
  assert.match(gallery, /relative[^\n]*aspect-\[4\/3\]/);
  assert.match(gallery, /<PublicMediaImage[\s\S]*?fill[\s\S]*?object-contain/);
});
