import assert from "node:assert/strict";
import test from "node:test";
import ts from "typescript";
import { readFile, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

const root = path.resolve("src/features/public-catalog/domain");
const tmp = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "booking-calendar-"),
);
for (const name of [
  "booking-calendar",
  "rental-availability",
  "marketplace-booking-selection",
  "media",
  "availability-return",
  "types",
]) {
  const source = await readFile(path.join(root, `${name}.ts`), "utf8");
  const output =
    name === "booking-calendar"
      ? source.replace(/import type .*?;\n/g, "")
      : source.replace(/^import .*?;\n/gm, "");
  await writeFile(
    path.join(tmp, `${name}.mjs`),
    ts.transpileModule(output, {
      compilerOptions: {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ESNext,
      },
    }).outputText,
  );
}
const mod = await import(pathToFileURL(path.join(tmp, "booking-calendar.mjs")));

test("empty and failed availability yield no selection (no fabricated tickets)", () => {
  assert.deepEqual(
    mod.authoritativeIntervals({ ok: true, value: { items: [] } }),
    [],
  );
  assert.deepEqual(mod.authoritativeIntervals({ ok: false }), []);
  const context = {
    spaceId: "x",
    date: "2026-06-12",
    duration: 2,
    rateUnit: "hora",
  };
  assert.equal(
    mod.availabilitySelection([], context, "America/Santiago"),
    null,
  );
  assert.equal(mod.availabilitySelection([], context, undefined), null);
});

test("selected interval keeps API timestamps exactly and needs its real timezone", () => {
  const intervals = [
    {
      start_at: "2026-06-12T09:00:00-04:00",
      end_at: "2026-06-12T11:00:00-04:00",
    },
  ];
  const selected = mod.availabilitySelection(
    intervals,
    { spaceId: "x", date: "2026-06-12", duration: 2, rateUnit: "hora" },
    "America/Santiago",
  );
  assert.equal(selected.startAt, intervals[0].start_at);
  assert.equal(selected.endAt, intervals[0].end_at);
  assert.equal(selected.duration, 2);
  assert.equal(selected.timeZone, "America/Santiago");
  assert.equal(
    mod.availabilitySelection(
      intervals,
      { spaceId: "x", date: "2026-06-12", duration: 2, rateUnit: "hora" },
      undefined,
    ),
    null,
  );
});

test("generation fence rejects late A/B/A completion", () => {
  const fence = mod.createGenerationFence();
  const a1 = fence.next();
  fence.next();
  const a2 = fence.next();
  assert.equal(fence.isCurrent(a1), false);
  assert.equal(fence.isCurrent(a2), true);
});

test("suggestedStaysForUnit delivers coherent suggestions for hora, dia and mes", () => {
  assert.deepEqual(mod.suggestedStaysForUnit("hora"), [
    { label: "1 hora", duration: 1 },
    { label: "2 horas", duration: 2 },
    { label: "4 horas", duration: 4 },
  ]);
  assert.deepEqual(mod.suggestedStaysForUnit("dia"), [
    { label: "1 día", duration: 1 },
    { label: "2 días", duration: 2 },
    { label: "3 días", duration: 3 },
  ]);
  assert.deepEqual(mod.suggestedStaysForUnit("mes"), [
    { label: "1 mes", duration: 1 },
  ]);
  assert.deepEqual(mod.suggestedStaysForUnit("unknown"), []);
});
test.after(async () => rm(tmp, { recursive: true, force: true }));
