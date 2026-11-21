import { clpAmount } from "@/lib/api/clp";
import { isPublicUuid } from "./media";
import { parseRentalReturnSelection } from "./availability-return";

export interface MarketplaceBookingInterval {
  startAt: string;
  endAt: string;
}

export interface MarketplaceBookingSelection {
  spaceId: string;
  date: string;
  duration: number;
  rateUnit: "hora" | "dia" | "mes";
  startAt: string;
  endAt: string;
  timeZone: string;
  intervals?: readonly MarketplaceBookingInterval[];
}

const int64Max = BigInt("9223372036854775807");
const requiredSelectionKeys = [
  "spaceId",
  "date",
  "duration",
  "start",
  "end",
  "rate_unit",
  "time_zone",
  "source",
] as const;
const allowedSelectionKeys = [...requiredSelectionKeys, "intervals"] as const;

export function marketplaceSubtotal(
  basePrice: number | string | null,
  duration: number,
): string | null {
  if (
    basePrice === null ||
    !Number.isSafeInteger(duration) ||
    duration < 1 ||
    duration > 64
  )
    return null;
  try {
    const amount = BigInt(clpAmount(basePrice));
    if (amount <= BigInt(0)) return null;
    const total = amount * BigInt(duration);
    return total <= int64Max ? total.toString() : null;
  } catch {
    return null;
  }
}

function validTimeZone(value: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}

function allowedDuration(rateUnit: string, duration: number): boolean {
  if (!Number.isSafeInteger(duration)) return false;
  if (rateUnit === "hora") return duration >= 1 && duration <= 64;
  if (rateUnit === "dia") return duration >= 1 && duration <= 64;
  return rateUnit === "mes" && duration >= 1 && duration <= 12;
}

function serializeIntervals(
  intervals?: readonly MarketplaceBookingInterval[],
): string | null {
  if (!intervals || intervals.length === 0) return null;
  return intervals.map((iv) => `${iv.startAt},${iv.endAt}`).join(";");
}

function parseIntervalsParam(
  raw: string | null,
): MarketplaceBookingInterval[] | null {
  if (!raw) return null;
  const parts = raw.split(";").filter(Boolean);
  if (parts.length === 0 || parts.length > 64) return null;
  const parsed: MarketplaceBookingInterval[] = [];
  for (const part of parts) {
    const [startAt, endAt, extra] = part.split(",");
    if (!startAt || !endAt || extra !== undefined) return null;
    const startMs = Date.parse(startAt);
    const endMs = Date.parse(endAt);
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs)
      return null;
    parsed.push({ startAt, endAt });
  }
  return parsed;
}

export function marketplaceBookingHref(
  selection: MarketplaceBookingSelection,
): string | null {
  if (
    !isPublicUuid(selection.spaceId) ||
    !allowedDuration(selection.rateUnit, selection.duration) ||
    !validTimeZone(selection.timeZone)
  )
    return null;
  const params = new URLSearchParams();
  params.set("spaceId", selection.spaceId);
  params.set("date", selection.date);
  params.set("duration", String(selection.duration));
  params.set("start", selection.startAt);
  params.set("end", selection.endAt);
  params.set("rate_unit", selection.rateUnit);
  params.set("time_zone", selection.timeZone);
  params.set("source", "marketplace");
  const serializedIntervals = serializeIntervals(selection.intervals);
  if (serializedIntervals && selection.intervals && selection.intervals.length > 1) {
    params.set("intervals", serializedIntervals);
  }
  if (!parseMarketplaceBookingSelection(params.toString())) return null;
  return `/reservas/nueva?${params.toString()}`;
}

export function parseMarketplaceBookingSelection(
  search: string,
): MarketplaceBookingSelection | null {
  const raw = search.startsWith("?") ? search.slice(1) : search;
  if (!raw || raw.length > 4096) return null;
  const params = new URLSearchParams(raw);
  if (
    requiredSelectionKeys.some((key) => params.getAll(key).length !== 1) ||
    [...params.keys()].some(
      (key) =>
        !allowedSelectionKeys.includes(
          key as (typeof allowedSelectionKeys)[number],
        ),
    ) ||
    params.get("source") !== "marketplace"
  )
    return null;

  const spaceId = params.get("spaceId") ?? "";
  const date = params.get("date") ?? "";
  const rawDuration = params.get("duration") ?? "";
  const startAt = params.get("start") ?? "";
  const endAt = params.get("end") ?? "";
  const rateUnit = params.get("rate_unit") ?? "";
  const timeZone = params.get("time_zone") ?? "";
  const rawIntervals = params.get("intervals");
  if (
    !isPublicUuid(spaceId) ||
    !validTimeZone(timeZone) ||
    !/^\d{1,2}$/.test(rawDuration)
  )
    return null;

  const parsedIntervals = rawIntervals ? parseIntervalsParam(rawIntervals) : null;
  if (rawIntervals !== null && !parsedIntervals) return null;

  const rental = new URLSearchParams();
  rental.set("rental_date", date);
  rental.set("rental_duration", rawDuration);
  rental.set("rental_start_at", startAt);
  rental.set("rental_end_at", endAt);
  const validated = parseRentalReturnSelection(rental.toString());
  if (!validated || !allowedDuration(rateUnit, validated.duration)) return null;
  return {
    spaceId,
    date: validated.date,
    duration: validated.duration,
    rateUnit: rateUnit as MarketplaceBookingSelection["rateUnit"],
    startAt: validated.startAt,
    endAt: validated.endAt,
    timeZone,
    ...(parsedIntervals ? { intervals: parsedIntervals } : {}),
  };
}
