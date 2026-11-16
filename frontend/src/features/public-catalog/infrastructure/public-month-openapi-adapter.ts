import "client-only";

import createClient from "openapi-fetch";
import type { paths } from "@/lib/api/generated/openapi";
import type { PublicMonth, PublicMonthResult } from "../domain/public-month";

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function integer(value: unknown): number | null {
  if (typeof value === "string" && /^\d+$/.test(value)) value = Number(value);
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0
    ? value
    : null;
}

function validMonth(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-(0[1-9]|1[0-2])$/.test(value))
    return false;
  return true;
}

function validDate(value: unknown, month: string): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}$/.test(value) ||
    value.slice(0, 7) !== month
  )
    return false;
  const date = new Date(`${value}T00:00:00Z`);
  return (
    Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value
  );
}

function parse(value: unknown, requestedMonth: string): PublicMonth | null {
  if (
    !record(value) ||
    !validMonth(value.month) ||
    value.month !== requestedMonth ||
    typeof value.time_zone !== "string" ||
    typeof value.rate_unit !== "string" ||
    !["hora", "dia", "mes"].includes(value.rate_unit) ||
    !Array.isArray(value.days)
  )
    return null;
  try {
    if (
      new Intl.DateTimeFormat("en", {
        timeZone: value.time_zone,
      }).resolvedOptions().timeZone !== value.time_zone
    )
      return null;
  } catch {
    return null;
  }
  const basePrice = integer(value.base_price_clp);
  if (basePrice === null) return null;
  const seen = new Set<string>();
  const days = [];
  for (const day of value.days) {
    if (
      !record(day) ||
      !validDate(day.date, requestedMonth) ||
      !Array.isArray(day.slots)
    )
      return null;
    if (seen.has(day.date)) return null;
    seen.add(day.date);
    const price = integer(day.price_clp);
    const statuses = {
      AVAILABLE: "available",
      PARTIAL: "partial",
      BLOCKED: "unavailable",
      BOOKED: "unavailable",
      PAST: "outside_horizon",
    } as const;
    const status =
      typeof day.status === "string" &&
      Object.prototype.hasOwnProperty.call(statuses, day.status)
        ? statuses[day.status as keyof typeof statuses]
        : null;
    if (price === null || !status) return null;
    days.push({ date: day.date, price, status });
  }
  return {
    month: requestedMonth,
    timeZone: value.time_zone,
    rateUnit: value.rate_unit as PublicMonth["rateUnit"],
    basePrice,
    days,
  };
}

export function createPublicMonthAdapter(
  fetcher: typeof fetch = fetch,
  baseUrl = "/api/backend",
) {
  const client = createClient<paths>({
    baseUrl,
    credentials: "same-origin",
    cache: "no-store",
    fetch: fetcher,
  });
  return {
    async getMonth(
      spaceId: string,
      month: string,
      signal?: AbortSignal,
    ): Promise<PublicMonthResult> {
      if (signal?.aborted) return { ok: false, kind: "aborted" };
      if (!validMonth(month)) return { ok: false, kind: "invalid" };
      try {
        const { data, response } = await client.GET(
          "/public/spaces/{spaceID}/month",
          {
            params: { path: { spaceID: spaceId }, query: { month } },
            signal,
          },
        );
        if (signal?.aborted) return { ok: false, kind: "aborted" };
        if (!response.ok) return { ok: false, kind: "error" };
        const result = parse(data, month);
        return result
          ? { ok: true, value: result }
          : { ok: false, kind: "invalid" };
      } catch {
        return { ok: false, kind: signal?.aborted ? "aborted" : "error" };
      }
    },
  };
}

export const publicMonth = createPublicMonthAdapter();
