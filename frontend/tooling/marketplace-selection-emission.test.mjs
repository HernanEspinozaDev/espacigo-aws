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
  assert.match(
    calendar,
    /availabilitySelection\([\s\S]*?result\.value\.time_zone/,
  );
  assert.match(
    calendar,
    /startAt: t\.interval\.start_at,[\s\S]*?endAt: t\.interval\.end_at,[\s\S]*?timeZone/,
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

test("date and duration changes invalidate pending callback generations synchronously", () => {
  assert.match(
    calendar,
    /const invalidateIntervals = \(\) => \{\s*intervalFence\.current\.next\(\);[\s\S]*?notifySelection\(null\);/,
  );
  assert.match(calendar, /handleDayClick[\s\S]{0,180}invalidateIntervals\(\)/);
  assert.match(calendar, /handleStayPill[\s\S]{0,100}invalidateIntervals\(\)/);
  assert.match(
    calendar,
    /if \(!result \|\| !fence\.isCurrent\(generation\)\) return/,
  );
});

test("fresh availability errors and non-aborted query failures clear the emitted selection", () => {
  assert.match(
    calendar,
    /if \(!result\.ok\) \{\s*setIntervals\(\[\]\);\s*setSelectedTicketId\(null\);\s*notifySelection\(null\);/,
  );
  assert.match(
    calendar,
    /\.catch\(\(\) => \{\s*if \(!fence\.isCurrent\(generation\)\) return;[\s\S]*?notifySelection\(null\);/,
  );
});
