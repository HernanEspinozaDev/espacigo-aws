import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const source = await readFile(
  new URL(
    "../src/features/host-settlements/domain/settlement.ts",
    import.meta.url,
  ),
  "utf8",
);
const loaded = { exports: {} };
new Function(
  "module",
  "exports",
  ts.transpile(source, { module: ts.ModuleKind.CommonJS }),
)(loaded, loaded.exports);
const { parseSettlementPage } = loaded.exports;

const item = {
  reservation_id: "reservation-1",
  space_id: "space-1",
  space_title: "Sala de reuniones",
  provider_reference: "123456",
  verified_at: "2026-11-05T12:00:00Z",
  currency: "CLP",
  test_mode: false,
  gross_clp: "100000",
  base_clp: "100000",
  commission_clp: "3000",
  commission_vat_clp: "570",
  marketplace_fee_clp: "3570",
  after_marketplace_fee_clp: "96430",
  processor_fee_clp: null,
  adjustments_clp: null,
  reconciled_net_clp: null,
  fee_policy: "three_percent_plus_vat",
  commission_rate_percent: 3,
  commission_vat_rate_percent: 19,
};
const response = {
  data: {
    summary: {
      month: "2026-11",
      time_zone: "America/Santiago",
      mode: "live",
      currency: "CLP",
      payment_count: 1,
      ...Object.fromEntries(
        [
          "gross_clp",
          "base_clp",
          "commission_clp",
          "commission_vat_clp",
          "marketplace_fee_clp",
          "after_marketplace_fee_clp",
        ].map((key) => [key, item[key]]),
      ),
      processor_fee_clp: null,
      adjustments_clp: null,
      reconciled_net_clp: null,
    },
    items: [item],
    next_cursor: null,
  },
};

test("settlement parser preserves CLP precision and unavailable values", () => {
  const parsed = parseSettlementPage(response);
  assert.equal(parsed.summary.gross_clp, "100000");
  assert.equal(parsed.items[0].after_marketplace_fee_clp, "96430");
  assert.equal(parsed.summary.processor_fee_clp, null);
  assert.equal(parsed.summary.reconciled_net_clp, null);
});

test("settlement parser rejects unsafe or malformed amounts", () => {
  assert.throws(() =>
    parseSettlementPage({
      ...response,
      data: {
        ...response.data,
        items: [{ ...item, commission_clp: 3000 }],
      },
    }),
  );
});
