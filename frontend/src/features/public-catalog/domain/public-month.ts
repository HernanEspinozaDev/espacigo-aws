export type PublicMonthDayStatus =
  "available" | "partial" | "unavailable" | "outside_horizon";

export interface PublicMonthDay {
  date: string;
  status: PublicMonthDayStatus;
  price: number;
}

export interface PublicMonth {
  month: string;
  timeZone: string;
  rateUnit: "hora" | "dia" | "mes";
  basePrice: number;
  days: readonly PublicMonthDay[];
}

export type PublicMonthResult =
  | { ok: true; value: PublicMonth }
  | { ok: false; kind: "error" | "invalid" | "aborted" };
