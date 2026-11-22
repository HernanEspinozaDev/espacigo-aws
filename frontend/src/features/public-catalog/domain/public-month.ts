export type PublicMonthDayStatus =
  "available" | "partial" | "unavailable" | "outside_horizon";

export interface PublicMonthDay {
  date: string;
  status: PublicMonthDayStatus;
  price: number;
  occupancy?: "held" | "booked";
}

export interface PublicMonth {
  month: string;
  timeZone: string;
  rateUnit: "hora" | "dia" | "mes";
  basePrice: number;
  days: readonly PublicMonthDay[];
  disponible_desde?: string | null;
  estadia_minima_meses?: number | null;
}

export type PublicMonthResult =
  | { ok: true; value: PublicMonth }
  | { ok: false; kind: "error" | "invalid" | "aborted" };
