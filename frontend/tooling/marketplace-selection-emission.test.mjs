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

const calendar = await readFile(
  new URL(
    "../src/features/public-catalog/ui/booking-calendar.tsx",
    import.meta.url,
  ),
  "utf8",
);
const selection = await readFile(
  new URL(
    "../src/features/public-catalog/domain/booking-calendar.ts",
    import.meta.url,
  ),
  "utf8",
);

test("calendar exposes an optional marketplace selection callback without changing consumers", () => {
  assert.match(
    source,
    /onSelectionChange\?: \(selection: MarketplaceBookingSelection \| null\) => void/,
  );
  assert.match(source, /onSelectionChange=\{onSelectionChange\}/);
  assert.doesNotMatch(source, /getAvailability\(/);
});

test("calendar emits only selected response intervals with the exact response instants and timezone", () => {
  assert.match(calendar, /timeZone/);
  assert.match(
    calendar,
    /startAt:\s*first\.startAt,[\s\S]*?endAt:\s*last\.endAt,[\s\S]*?timeZone:\s*tz,[\s\S]*?intervals:\s*merged/,
  );
  assert.match(
    selection,
    /startAt:\s*item\.start_at,\s*endAt:\s*item\.end_at,\s*timeZone/,
  );
  assert.doesNotMatch(
    calendar,
    /ticket-morning|ticket-afternoon|T09:00:00-03:00/,
  );
});

test("date and duration changes handle pending callback generations with intervalFence", () => {
  assert.match(calendar, /const fence = intervalFence\.current;/);
  assert.match(calendar, /const generation = fence\.next\(\);/);
  assert.match(
    calendar,
    /if \(!result \|\| !fence\.isCurrent\(generation\)\) return/,
  );
});

test("fresh availability errors and non-aborted query failures clear rawSlots safely", () => {
  assert.match(
    calendar,
    /if \(!result\.ok\) \{\s*setRawSlots\(\[\]\);\s*return;\s*\}/,
  );
  assert.match(
    calendar,
    /\.catch\(\(\) => \{\s*if \(!fence\.isCurrent\(generation\)\) return;\s*setRawSlots\(\[\]\);/,
  );
});
