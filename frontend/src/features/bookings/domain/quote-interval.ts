import type { RateUnit } from "./types";

export interface AuthoritativeBookingInterval {
  startAt: string;
  endAt: string;
  timeZone: string;
}

export interface QuoteIntervalPayload {
  start_at: string;
  end_at: string;
}

export function selectQuoteIntervalPayload(input: {
  authoritative?: AuthoritativeBookingInterval;
  startDate: string;
  rateUnit: RateUnit;
  durationUnits: number;
}): QuoteIntervalPayload | null {
  if (input.authoritative) {
    const { startAt, endAt, timeZone } = input.authoritative;
    if (
      !Number.isFinite(Date.parse(startAt)) ||
      !Number.isFinite(Date.parse(endAt)) ||
      Date.parse(endAt) <= Date.parse(startAt)
    )
      return null;
    try {
      new Intl.DateTimeFormat("en", { timeZone }).format(new Date(startAt));
    } catch {
      return null;
    }
    return { start_at: startAt, end_at: endAt };
  }
  const start = new Date(input.startDate);
  if (
    !Number.isFinite(start.getTime()) ||
    !Number.isInteger(input.durationUnits) ||
    input.durationUnits < 1
  )
    return null;
  const end = new Date(start);
  if (input.rateUnit === "hora")
    end.setHours(end.getHours() + input.durationUnits);
  else if (input.rateUnit === "dia")
    end.setDate(end.getDate() + input.durationUnits);
  else end.setMonth(end.getMonth() + input.durationUnits);
  if (!Number.isFinite(end.getTime())) return null;
  return { start_at: start.toISOString(), end_at: end.toISOString() };
}

export function formatIntervalInTimeZone(
  value: string,
  timeZone: string,
): string {
  return new Intl.DateTimeFormat("es-CL", {
    timeZone,
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(value));
}
