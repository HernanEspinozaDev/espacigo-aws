import { clpAmount } from "@/lib/api/clp";
import { isPublicUuid } from "./media";
import { parseRentalReturnSelection } from "./availability-return";

export interface MarketplaceBookingSelection {
  spaceId: string;
  date: string;
  duration: number;
  rateUnit: "hora" | "dia" | "mes";
  startAt: string;
  endAt: string;
  timeZone: string;
}

const int64Max = BigInt("9223372036854775807");
const selectionKeys = [
  "spaceId",
  "date",
  "duration",
  "start",
  "end",
  "rate_unit",
  "time_zone",
  "source",
] as const;

export function marketplaceSubtotal(
  basePrice: number | string | null,
  duration: number,
): string | null {
  if (
    basePrice === null ||
    !Number.isSafeInteger(duration) ||
    duration < 1 ||
    duration > 4
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
  if (rateUnit === "hora") return [1, 2, 4].includes(duration);
  if (rateUnit === "dia") return [1, 2, 3].includes(duration);
  return rateUnit === "mes" && duration === 1;
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
  if (!parseMarketplaceBookingSelection(params.toString())) return null;
  return `/reservas/nueva?${params.toString()}`;
}

export function parseMarketplaceBookingSelection(
  search: string,
): MarketplaceBookingSelection | null {
  const raw = search.startsWith("?") ? search.slice(1) : search;
  if (!raw || raw.length > 2048) return null;
  const params = new URLSearchParams(raw);
  if (
    params.size !== selectionKeys.length ||
    selectionKeys.some((key) => params.getAll(key).length !== 1) ||
    [...params.keys()].some(
      (key) => !selectionKeys.includes(key as (typeof selectionKeys)[number]),
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
  if (
    !isPublicUuid(spaceId) ||
    !validTimeZone(timeZone) ||
    !/^[1-4]$/.test(rawDuration)
  )
    return null;

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
  };
}
