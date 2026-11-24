export interface SettlementAmounts {
  gross_clp: string;
  base_clp: string;
  commission_clp: string;
  commission_vat_clp: string;
  marketplace_fee_clp: string;
  after_marketplace_fee_clp: string;
  processor_fee_clp: string | null;
  adjustments_clp: string | null;
  reconciled_net_clp: string | null;
}

export interface HostSettlement extends SettlementAmounts {
  reservation_id: string;
  space_id: string;
  space_title: string;
  provider_reference: string;
  verified_at: string;
  currency: "CLP";
  test_mode: boolean;
  fee_policy: string;
  commission_rate_percent: number | null;
  commission_vat_rate_percent: number | null;
}

export interface MonthlySettlement extends SettlementAmounts {
  month: string;
  time_zone: "America/Santiago";
  mode: "live" | "test";
  currency: "CLP";
  payment_count: number;
}

export interface SettlementPage {
  summary: MonthlySettlement;
  items: HostSettlement[];
  next_cursor: string | null;
}

const amountKeys = [
  "gross_clp",
  "base_clp",
  "commission_clp",
  "commission_vat_clp",
  "marketplace_fee_clp",
  "after_marketplace_fee_clp",
] as const;

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("invalid");
  return value as Record<string, unknown>;
}

function amountFields(value: Record<string, unknown>): SettlementAmounts {
  for (const key of amountKeys) {
    if (
      typeof value[key] !== "string" ||
      !/^(0|[1-9][0-9]*)$/.test(value[key] as string)
    )
      throw new Error("invalid");
  }
  for (const key of [
    "processor_fee_clp",
    "adjustments_clp",
    "reconciled_net_clp",
  ] as const) {
    const amount = value[key];
    if (
      amount !== null &&
      (typeof amount !== "string" || !/^(0|[1-9][0-9]*)$/.test(amount))
    )
      throw new Error("invalid");
  }
  return Object.fromEntries(
    [
      ...amountKeys,
      "processor_fee_clp",
      "adjustments_clp",
      "reconciled_net_clp",
    ].map((key) => [key, value[key]]),
  ) as unknown as SettlementAmounts;
}

export function parseSettlementPage(input: unknown): SettlementPage {
  const root = record(record(input).data);
  const summary = record(root.summary);
  if (
    typeof summary.month !== "string" ||
    typeof summary.payment_count !== "number" ||
    !Number.isSafeInteger(summary.payment_count) ||
    summary.time_zone !== "America/Santiago" ||
    (summary.mode !== "live" && summary.mode !== "test") ||
    summary.currency !== "CLP"
  )
    throw new Error("invalid");
  if (!Array.isArray(root.items)) throw new Error("invalid");
  const items = root.items.map((raw): HostSettlement => {
    const item = record(raw);
    if (
      [
        "reservation_id",
        "space_id",
        "space_title",
        "provider_reference",
        "verified_at",
      ].some((key) => typeof item[key] !== "string") ||
      item.currency !== "CLP" ||
      typeof item.test_mode !== "boolean" ||
      typeof item.fee_policy !== "string" ||
      (item.commission_rate_percent !== null &&
        typeof item.commission_rate_percent !== "number") ||
      (item.commission_vat_rate_percent !== null &&
        typeof item.commission_vat_rate_percent !== "number")
    )
      throw new Error("invalid");
    return { ...item, ...amountFields(item) } as HostSettlement;
  });
  if (root.next_cursor !== null && typeof root.next_cursor !== "string")
    throw new Error("invalid");
  return {
    summary: { ...summary, ...amountFields(summary) } as MonthlySettlement,
    items,
    next_cursor: root.next_cursor as string | null,
  };
}
