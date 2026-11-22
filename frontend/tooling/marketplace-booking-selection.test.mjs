import assert from "node:assert/strict";
import test from "node:test";
import { createRequire } from "node:module";
import ts from "typescript";

const require = createRequire(import.meta.url);
const source = new URL(
  "../src/features/public-catalog/domain/marketplace-booking-selection.ts",
  import.meta.url,
);
const output = ts.transpileModule(
  await (await import("node:fs/promises")).readFile(source, "utf8"),
  {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  },
);
const loaded = { exports: {} };
const localRequire = (id) =>
  id === "@/lib/api/clp"
    ? require("../src/lib/api/clp.ts")
    : id === "./media"
      ? require("../src/features/public-catalog/domain/media.ts")
      : id === "./availability-return"
        ? require("../src/features/public-catalog/domain/availability-return.ts")
        : require(id);
new Function("require", "module", "exports", output.outputText)(
  localRequire,
  loaded,
  loaded.exports,
);
const {
  marketplaceSubtotal,
  marketplaceBookingHref,
  parseMarketplaceBookingSelection,
} = loaded.exports;
const valid = {
  spaceId: "123e4567-e89b-42d3-a456-426614174000",
  date: "2026-12-01",
  duration: 2,
  rateUnit: "hora",
  startAt: "2026-12-01T09:00:00-03:00",
  endAt: "2026-12-01T11:00:00-03:00",
  timeZone: "America/Santiago",
};
const query =
  "spaceId=123e4567-e89b-42d3-a456-426614174000&date=2026-12-01&duration=2&start=2026-12-01T09%3A00%3A00-03%3A00&end=2026-12-01T11%3A00%3A00-03%3A00&rate_unit=hora&time_zone=America%2FSantiago&source=marketplace";

test("subtotal multiplica CLP exactamente y conserva strings grandes", () => {
  assert.equal(marketplaceSubtotal("9007199254740000", 2), "18014398509480000");
  assert.equal(marketplaceSubtotal(5001, 4), "20004");
  assert.equal(marketplaceSubtotal(Number.MAX_SAFE_INTEGER + 1, 1), null);
  assert.equal(marketplaceSubtotal("9223372036854775808", 1), null);
  assert.equal(marketplaceSubtotal(null, 1), null);
});

test("canonical href round-trips exact timestamps without a price", () => {
  const href = marketplaceBookingHref(valid);
  assert.equal(href, `/reservas/nueva?${query}`);
  assert.deepEqual(
    parseMarketplaceBookingSelection(href.slice(href.indexOf("?") + 1)),
    valid,
  );
  assert.doesNotMatch(href, /price|subtotal|return_to/);
});

test("rejects invalid, duplicate, unknown and unsafe booking parameters", () => {
  assert.equal(
    parseMarketplaceBookingSelection(`${query}&spaceId=${valid.spaceId}`),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(`${query}&space_id=${valid.spaceId}`),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(
      query.replace("source=marketplace", "source=https%3A%2F%2Fevil.example"),
    ),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(query.replace("duration=2", "duration=0")),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(
      query.replace("rate_unit=hora", "rate_unit=semana"),
    ),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(
      query.replace("America%2FSantiago", "Invalid%2FZone"),
    ),
    null,
  );
  assert.equal(
    parseMarketplaceBookingSelection(query.replace("2026-12-01", "2026-02-30")),
    null,
  );
  assert.equal(
    marketplaceBookingHref({ ...valid, spaceId: "not-a-uuid" }),
    null,
  );
});

test("dia multi-interval selection round-trips correctly and calculates subtotal", () => {
  const diaSelection = {
    spaceId: "123e4567-e89b-42d3-a456-426614174000",
    date: "2026-12-05",
    duration: 3,
    rateUnit: "dia",
    startAt: "2026-12-05T00:00:00-03:00",
    endAt: "2026-12-08T00:00:00-03:00",
    timeZone: "America/Santiago",
    intervals: [
      {
        startAt: "2026-12-05T00:00:00-03:00",
        endAt: "2026-12-06T00:00:00-03:00",
      },
      {
        startAt: "2026-12-07T00:00:00-03:00",
        endAt: "2026-12-08T00:00:00-03:00",
      },
    ],
  };
  const href = marketplaceBookingHref(diaSelection);
  assert.ok(href && href.startsWith("/reservas/nueva?"));
  assert.match(href, /rate_unit=dia/);
  assert.match(href, /duration=3/);
  assert.match(href, /intervals=/);

  const parsed = parseMarketplaceBookingSelection(
    href.slice(href.indexOf("?") + 1),
  );
  assert.deepEqual(parsed, diaSelection);

  assert.equal(marketplaceSubtotal(25000, 3), "75000");
});

test("mes duration, min-months, and billing_mode round-trip and validation rules", () => {
  const mesSelection = {
    spaceId: "123e4567-e89b-42d3-a456-426614174000",
    date: "2026-12-01",
    duration: 4,
    rateUnit: "mes",
    startAt: "2026-12-01T00:00:00-03:00",
    endAt: "2027-04-01T00:00:00-03:00",
    timeZone: "America/Santiago",
    billingMode: "monthly_installments",
    minMonths: 3,
  };
  const href = marketplaceBookingHref(mesSelection);
  assert.ok(href && href.startsWith("/reservas/nueva?"));
  assert.match(href, /rate_unit=mes/);
  assert.match(href, /duration=4/);
  assert.match(href, /billing_mode=monthly_installments/);
  assert.match(href, /min_months=3/);

  const parsed = parseMarketplaceBookingSelection(
    href.slice(href.indexOf("?") + 1),
  );
  assert.deepEqual(parsed, mesSelection);

  // Full billing mode
  const fullHref = marketplaceBookingHref({
    ...mesSelection,
    billingMode: "full",
  });
  assert.match(fullHref, /billing_mode=full/);
  const parsedFull = parseMarketplaceBookingSelection(
    fullHref.slice(fullHref.indexOf("?") + 1),
  );
  assert.equal(parsedFull?.billingMode, "full");

  // Rejects duration less than min_months
  const invalidDurationQuery = href
    .slice(href.indexOf("?") + 1)
    .replace("duration=4", "duration=2");
  assert.equal(parseMarketplaceBookingSelection(invalidDurationQuery), null);

  // Rejects invalid billing_mode
  const invalidModeQuery = href
    .slice(href.indexOf("?") + 1)
    .replace("billing_mode=monthly_installments", "billing_mode=quarterly");
  assert.equal(parseMarketplaceBookingSelection(invalidModeQuery), null);

  // Rejects billing_mode or min_months on hora/dia
  const modeOnHora = query + "&billing_mode=full";
  assert.equal(parseMarketplaceBookingSelection(modeOnHora), null);
  const minOnHora = query + "&min_months=2";
  assert.equal(parseMarketplaceBookingSelection(minOnHora), null);

  // Subtotal for mes
  assert.equal(marketplaceSubtotal(120000, 4), "480000");
});
