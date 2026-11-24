import assert from "node:assert/strict";
import test from "node:test";
import {
  resolveSantiagoLocalDateTime,
  santiagoLocalInputValue,
} from "../src/features/public-catalog/domain/santiago-datetime.ts";

test("converts Santiago local time using the applicable seasonal offset", () => {
  assert.deepEqual(resolveSantiagoLocalDateTime("2026-01-15T12:30"), {
    kind: "unique",
    value: "2026-01-15T12:30:00-03:00",
  });
  assert.deepEqual(resolveSantiagoLocalDateTime("2026-07-15T12:30"), {
    kind: "unique",
    value: "2026-07-15T12:30:00-04:00",
  });
});

test("rejects a nonexistent wall time at the spring DST transition", () => {
  assert.equal(
    resolveSantiagoLocalDateTime("2026-09-06T00:30").kind,
    "nonexistent",
  );
});

test("reports both explicit offsets during the fall DST overlap", () => {
  assert.deepEqual(resolveSantiagoLocalDateTime("2026-04-04T23:30"), {
    kind: "ambiguous",
    earlier: "2026-04-04T23:30:00-03:00",
    later: "2026-04-04T23:30:00-04:00",
  });
});

test("renders existing RFC3339 instants in Santiago local time", () => {
  assert.equal(
    santiagoLocalInputValue("2026-01-15T15:30:00Z"),
    "2026-01-15T12:30",
  );
});
