import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import ts from "typescript";
const source = readFileSync(
  new URL("../src/features/bookings/domain/quote-interval.ts", import.meta.url),
  "utf8",
);
const js = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText;
const context = vm.createContext({ Intl, Date, exports: {} });
vm.runInContext(js, context);
const select = context.exports.selectQuoteIntervalPayload;
const normalize = (value) => JSON.parse(JSON.stringify(value));
const authoritative = {
  startAt: "2024-09-08T03:30:00.000Z",
  endAt: "2024-09-08T04:30:00.000Z",
  timeZone: "America/Santiago",
};
test("authoritative DST-crossing interval is forwarded byte-for-byte", () =>
  assert.deepEqual(
    normalize(
      select({
        authoritative,
        startDate: "bad",
        rateUnit: "hora",
        durationUnits: 4,
      }),
    ),
    { start_at: authoritative.startAt, end_at: authoritative.endAt },
  ));
test("authoritative clamped month end survives differing local calculation", () => {
  const interval = {
    startAt: "2024-01-31T03:00:00.000Z",
    endAt: "2024-02-29T03:00:00.000Z",
    timeZone: "America/Santiago",
  };
  assert.deepEqual(
    normalize(
      select({
        authoritative: interval,
        startDate: "2024-01-31T00:00",
        rateUnit: "mes",
        durationUnits: 1,
      }),
    ),
    { start_at: interval.startAt, end_at: interval.endAt },
  );
});
test("invalid authoritative interval is rejected", () =>
  assert.equal(
    select({
      authoritative: { ...authoritative, endAt: authoritative.startAt },
      startDate: "2024-09-08T00:00",
      rateUnit: "hora",
      durationUnits: 1,
    }),
    null,
  ));
test("real handler invokes interval selector before POST", () => {
  const card = readFileSync(
    new URL(
      "../src/features/bookings/ui/booking-quote-card.tsx",
      import.meta.url,
    ),
    "utf8",
  );
  const guard = card.indexOf("if (!intervalPayload)");
  const request = card.indexOf("requestQuote({");
  assert.ok(
    guard >= 0 && request > guard,
    "invalid interval guard must precede requestQuote",
  );
});
