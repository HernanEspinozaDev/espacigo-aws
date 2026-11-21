import type {
  BookingInterval,
  BookingReservation,
  IntervalUsageStatus,
} from "./types";

export interface GroupedDaySlot {
  readonly id?: string;
  readonly startAt: string;
  readonly endAt: string;
  readonly timeLabel: string;
  readonly units: number;
  readonly usageStatus: IntervalUsageStatus;
  readonly checkInAt?: string;
  readonly checkOutAt?: string;
  readonly passCode?: string;
  readonly qrPayload?: string;
}

export interface GroupedReservationDay {
  readonly dateKey: string;
  readonly dateLabel: string;
  readonly totalUnits: number;
  readonly subtotalClp: number;
  readonly slots: readonly GroupedDaySlot[];
}

export function usageStatusLabel(status: IntervalUsageStatus): string {
  switch (status) {
    case "en_uso":
      return "En uso";
    case "completado":
      return "Usada";
    case "no_asistio":
      return "No asistió";
    default:
      return "Por usar";
  }
}

export function getReservationIntervals(
  reservation: BookingReservation,
): readonly BookingInterval[] {
  if (reservation.intervals && reservation.intervals.length > 0) {
    return reservation.intervals;
  }
  return [
    {
      start_at: reservation.start_at,
      end_at: reservation.end_at,
      units: reservation.units,
      usage_status: "pendiente",
    },
  ];
}

export function dateKeyInTimeZone(
  isoTimestamp: string,
  timeZone = "America/Santiago",
): string {
  try {
    const d = new Date(isoTimestamp);
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(d);
    const y = parts.find((p) => p.type === "year")?.value;
    const m = parts.find((p) => p.type === "month")?.value;
    const day = parts.find((p) => p.type === "day")?.value;
    if (y && m && day) return `${y}-${m}-${day}`;
  } catch {
    // Fallback to UTC date slice
  }
  return isoTimestamp.slice(0, 10);
}

export function formatTimeRange24h(
  startIso: string,
  endIso: string,
  timeZone = "America/Santiago",
): string {
  try {
    const fmt = (iso: string) =>
      new Date(iso).toLocaleTimeString("es-CL", {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
        timeZone,
      });
    return `${fmt(startIso)} – ${fmt(endIso)}`;
  } catch {
    return `${startIso} – ${endIso}`;
  }
}

export function groupReservationIntervalsByDay(
  reservation: BookingReservation,
): GroupedReservationDay[] {
  const tz = reservation.time_zone || "America/Santiago";
  const intervals = getReservationIntervals(reservation);
  const byDate = new Map<
    string,
    {
      dateLabel: string;
      totalUnits: number;
      slots: GroupedDaySlot[];
    }
  >();

  for (const iv of intervals) {
    const dateKey = dateKeyInTimeZone(iv.start_at, tz);
    let entry = byDate.get(dateKey);
    if (!entry) {
      let dateLabel = dateKey;
      try {
        dateLabel = new Intl.DateTimeFormat("es-CL", {
          weekday: "short",
          day: "2-digit",
          month: "short",
          year: "numeric",
          timeZone: tz,
        }).format(new Date(iv.start_at));
      } catch {
        dateLabel = dateKey;
      }
      entry = { dateLabel, totalUnits: 0, slots: [] };
      byDate.set(dateKey, entry);
    }
    entry.totalUnits += iv.units;
    entry.slots.push({
      ...(iv.id ? { id: iv.id } : {}),
      startAt: iv.start_at,
      endAt: iv.end_at,
      timeLabel: formatTimeRange24h(iv.start_at, iv.end_at, tz),
      units: iv.units,
      usageStatus: iv.usage_status ?? "pendiente",
      ...(iv.check_in_at ? { checkInAt: iv.check_in_at } : {}),
      ...(iv.check_out_at ? { checkOutAt: iv.check_out_at } : {}),
      ...(iv.pass_code ? { passCode: iv.pass_code } : {}),
      ...(iv.qr_payload ? { qrPayload: iv.qr_payload } : {}),
    });
  }

  return Array.from(byDate.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([dateKey, group]) => ({
      dateKey,
      dateLabel: group.dateLabel,
      totalUnits: group.totalUnits,
      subtotalClp: group.totalUnits * reservation.unit_price_clp,
      slots: group.slots,
    }));
}
