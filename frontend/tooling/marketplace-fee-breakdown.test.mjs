import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import ts from "typescript";

const require = createRequire(import.meta.url);
const frontendRoot = path.resolve(import.meta.dirname, "..");

async function loadTsModule(relativePath, aliases = {}) {
  const source = await readFile(path.join(frontendRoot, relativePath), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
  const mod = { exports: {} };
  const localRequire = (id) => {
    if (Object.hasOwn(aliases, id)) return aliases[id];
    return require(id);
  };
  new Function("require", "module", "exports", compiled)(
    localRequire,
    mod,
    mod.exports,
  );
  return mod.exports;
}

test("calculateBuyerFeeBreakdown computes 3% commission + 19% VAT (3.57%) accurately", async () => {
  const { calculateBuyerFeeBreakdown } = await loadTsModule(
    "src/features/rental-checkout/domain/fee-breakdown.ts",
  );

  assert.deepEqual(calculateBuyerFeeBreakdown(12000), {
    baseClp: 12000,
    commissionClp: 360,
    vatClp: 68,
    serviceFeeClp: 428,
    totalClp: 12428,
  });

  assert.deepEqual(calculateBuyerFeeBreakdown(100000), {
    baseClp: 100000,
    commissionClp: 3000,
    vatClp: 570,
    serviceFeeClp: 3570,
    totalClp: 103570,
  });

  assert.deepEqual(calculateBuyerFeeBreakdown(0), {
    baseClp: 0,
    commissionClp: 0,
    vatClp: 0,
    serviceFeeClp: 0,
    totalClp: 0,
  });
});

test("getReservationFinancialBreakdown resolves double-sided marketplace fees and legacy fallbacks", async () => {
  const feeModule = await loadTsModule(
    "src/features/rental-checkout/domain/fee-breakdown.ts",
  );
  const { getReservationFinancialBreakdown } = await loadTsModule(
    "src/features/bookings/domain/financial-breakdown.ts",
    {
      "@/features/rental-checkout/domain/fee-breakdown": feeModule,
    },
  );

  const doubleSided = getReservationFinancialBreakdown({
    id: "r1",
    quote_id: "q1",
    space_id: "s1",
    host_id: "h1",
    renter_id: "u1",
    participant_role: "anfitrion",
    state: "pagada",
    rate_unit: "hora",
    unit_price_clp: 12000,
    currency: "CLP",
    units: 1,
    subtotal_clp: 12000,
    commission_clp: 360,
    vat_clp: 68,
    net_host_amount_clp: 11572,
    buyer_commission_clp: 360,
    buyer_vat_clp: 68,
    buyer_fee_clp: 428,
    total_charged_clp: 12428,
    marketplace_fee_clp: 856,
    start_at: "2026-12-01T10:00:00Z",
    end_at: "2026-12-01T11:00:00Z",
    time_zone: "America/Santiago",
    conditions: "",
    cancellation_policy_version: "v1",
    pay_expires_at: "2026-12-01T09:00:00Z",
    created_at: "2026-11-30T10:00:00Z",
    updated_at: "2026-11-30T10:05:00Z",
    unread_count: 0,
  });

  assert.deepEqual(doubleSided, {
    baseClp: 12000,
    hostCommissionClp: 360,
    hostVatClp: 68,
    hostFeeClp: 428,
    netHostClp: 11572,
    buyerCommissionClp: 360,
    buyerVatClp: 68,
    buyerFeeClp: 428,
    totalChargedClp: 12428,
    marketplaceFeeClp: 856,
  });

  const legacy = getReservationFinancialBreakdown({
    id: "r2",
    quote_id: "q2",
    space_id: "s1",
    host_id: "h1",
    renter_id: "u1",
    participant_role: "anfitrion",
    state: "pagada",
    rate_unit: "hora",
    unit_price_clp: 12000,
    currency: "CLP",
    units: 1,
    subtotal_clp: 12000,
    commission_clp: 360,
    vat_clp: 68,
    marketplace_fee_clp: 428,
    net_host_amount_clp: 11572,
    start_at: "2026-12-01T10:00:00Z",
    end_at: "2026-12-01T11:00:00Z",
    time_zone: "America/Santiago",
    conditions: "",
    cancellation_policy_version: "v1",
    pay_expires_at: "2026-12-01T09:00:00Z",
    created_at: "2026-11-30T10:00:00Z",
    updated_at: "2026-11-30T10:05:00Z",
    unread_count: 0,
  });

  assert.deepEqual(legacy, {
    baseClp: 12000,
    hostCommissionClp: 360,
    hostVatClp: 68,
    hostFeeClp: 428,
    netHostClp: 11572,
    buyerCommissionClp: 0,
    buyerVatClp: 0,
    buyerFeeClp: 0,
    totalChargedClp: 12000,
    marketplaceFeeClp: 428,
  });
});

test("UI files never prepend extra dollar sign to formatBookingClp or formatPublicClp", async () => {
  const files = [
    "src/features/public-catalog/ui/marketplace-rental-checkout.tsx",
    "src/features/rental-checkout/ui/checkout-flow-chrome.tsx",
    "src/features/rental-checkout/ui/checkout-api-card-form.tsx",
    "src/features/bookings/ui/reservations-list.tsx",
    "src/features/bookings/ui/reservation-invoice-modal.tsx",
    "src/features/bookings/ui/reservation-lifecycle-panel.tsx",
  ];
  for (const rel of files) {
    const content = await readFile(path.join(frontendRoot, rel), "utf8");
    assert.equal(
      /\$\s*\$\{(?:formatBookingClp|formatPublicClp|formatted)/.test(content),
      false,
      `Double dollar sign detected in ${rel}`,
    );
  }
});
