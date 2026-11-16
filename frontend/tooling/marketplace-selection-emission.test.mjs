import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(
  new URL(
    "../src/features/public-catalog/ui/public-space-rental-availability.tsx",
    import.meta.url,
  ),
  "utf8",
);

test("calendar exposes an optional marketplace selection callback without changing consumers", () => {
  assert.match(
    source,
    /onSelectionChange\?: \(selection: MarketplaceBookingSelection \| null\) => void/,
  );
  assert.match(source, /selectionChange = useRef\(onSelectionChange\)/);
});

test("calendar emits only selected response intervals with the exact response instants and timezone", () => {
  assert.match(
    source,
    /if \(matchingInterval\) \{[\s\S]*?selectionChange\.current\?\.\(\{[\s\S]*?timeZone: result\.value\.time_zone/,
  );
  assert.match(
    source,
    /startAt: it\.start_at,[\s\S]*?endAt: it\.end_at,[\s\S]*?timeZone: availabilityTimeZone/,
  );
  assert.match(
    source,
    /if \(availabilityTimeZone\)[\s\S]{0,80}selectionChange\.current\?\.\(/,
  );
});

test("date and duration changes invalidate pending callback generations synchronously", () => {
  assert.match(
    source,
    /const beginAvailabilityQuery = \(\) => \{\s*generation\.current \+= 1;\s*selectionChange\.current\?\.\(null\);/,
  );
  assert.match(source, /if \(generation\.current !== currentOp\) return/);
});

test("fresh availability errors and non-aborted query failures clear the emitted selection", () => {
  assert.match(
    source,
    /if \(!result\.ok\) \{[\s\S]*?selectionChange\.current\?\.\(null\);/,
  );
  assert.match(
    source,
    /\.catch\(\(\) => \{[\s\S]*?if \(controller\.signal\.aborted \|\| generation\.current !== currentOp\)\s*return;[\s\S]*?selectionChange\.current\?\.\(null\);/,
  );
});
