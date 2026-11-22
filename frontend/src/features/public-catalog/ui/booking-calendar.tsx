"use client";

import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  monthDays,
  shiftMonth,
  localDateLabel,
  type MonthDay,
} from "@/features/rental-demo/domain/calendar";
import { addCivilDays } from "../domain/local-date";
import type { RentalReturnSelection } from "../domain/availability-return";
import type {
  MarketplaceBookingInterval,
  MarketplaceBookingSelection,
} from "../domain/marketplace-booking-selection";
import type { RentalScheduleSlot } from "../domain/rental-availability";
import { createGenerationFence } from "../domain/booking-calendar";
import { publicRentalAvailability } from "../infrastructure/rental-availability-runtime";
import { publicMonth } from "../infrastructure/public-month-openapi-adapter";

export interface CalendarDayAvailability {
  date: string;
  status: "available" | "partial" | "unavailable" | "outside_horizon";
  price?: number;
  occupancy?: "held" | "booked";
  intervals?: readonly { start_at: string; end_at: string }[];
}

export interface PublicBookingCalendarProps {
  spaceId: string;
  rateUnit: "hora" | "dia" | "mes";
  basePriceCLP: number | string | null;
  today: string;
  initialSelection?: RentalReturnSelection;
  onSelectionChange?: (selection: MarketplaceBookingSelection | null) => void;
  disabled?: boolean;
  disponibleDesde?: string | null;
  estadiaMinimaMeses?: number | null;
}

interface SelectedSlotEntry {
  date: string;
  endDate?: string;
  startAt: string;
  endAt: string;
  units?: number;
}

function formatCivilDate(dateISO: string): string {
  try {
    const [y, m, d] = dateISO.split("-").map(Number);
    const date = new Date(Date.UTC(y, m - 1, d));
    return new Intl.DateTimeFormat("es-CL", {
      day: "numeric",
      month: "short",
      year: "numeric",
      timeZone: "UTC",
    }).format(date);
  } catch {
    return dateISO;
  }
}

function addMonthsCivilDay(dateISO: string, months: number): string {
  const [y, m, d] = dateISO.split("-").map(Number);
  const dt = new Date(Date.UTC(y, m - 1 + months, d));
  const lastDay = new Date(Date.UTC(y, m + months, 0)).getUTCDate();
  if (dt.getUTCDate() > lastDay || d > lastDay) {
    return new Date(Date.UTC(y, m + months, 0)).toISOString().slice(0, 10);
  }
  return dt.toISOString().slice(0, 10);
}

const WEEKDAYS = ["Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"] as const;

export function formatCompactPrice(price: number | string | null): string {
  if (price === null || price === undefined || price === "") return "-";
  const num = typeof price === "number" ? price : parseInt(String(price), 10);
  if (isNaN(num) || num <= 0) return "-";
  if (num >= 1_000_000) {
    const m = num / 1_000_000;
    return `$${m % 1 === 0 ? m : m.toFixed(1)}M`;
  }
  if (num >= 1000) {
    const k = num / 1000;
    return `$${k % 1 === 0 ? k : k.toFixed(1)}k`;
  }
  return `$${num}`;
}

function monthLabel(month: string): string {
  const date = new Date(`${month}-01T00:00:00Z`);
  return new Intl.DateTimeFormat("es-CL", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(date);
}

function buildDisplaySlots(
  rawSlots: readonly RentalScheduleSlot[],
  rateUnit: "hora" | "dia" | "mes",
): RentalScheduleSlot[] {
  if (rateUnit !== "hora" || rawSlots.length <= 1) {
    return [...rawSlots];
  }
  const result: RentalScheduleSlot[] = [];
  let nextAllowedStartMs = -Infinity;
  for (const slot of rawSlots) {
    const startMs = Date.parse(slot.start_at);
    const endMs = Date.parse(slot.end_at);
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) continue;
    if (startMs >= nextAllowedStartMs) {
      result.push(slot);
      nextAllowedStartMs = endMs;
    }
  }
  return result;
}

function mergeContiguousIntervals(
  entries: readonly SelectedSlotEntry[],
): MarketplaceBookingInterval[] {
  if (entries.length === 0) return [];
  const sorted = [...entries].sort(
    (a, b) => Date.parse(a.startAt) - Date.parse(b.startAt),
  );
  const merged: MarketplaceBookingInterval[] = [];
  for (const entry of sorted) {
    const prev = merged[merged.length - 1];
    if (prev && prev.endAt === entry.startAt) {
      prev.endAt = entry.endAt;
    } else {
      merged.push({ startAt: entry.startAt, endAt: entry.endAt });
    }
  }
  return merged;
}

export function PublicBookingCalendar({
  spaceId,
  rateUnit,
  today,
  initialSelection,
  onSelectionChange,
  disabled = false,
  disponibleDesde: propDisponibleDesde,
  estadiaMinimaMeses: propEstadiaMinimaMeses,
}: PublicBookingCalendarProps) {
  const headingId = useId();
  const [currentMonth, setCurrentMonth] = useState(
    initialSelection?.date.slice(0, 7) ?? today.slice(0, 7),
  );

  const [disponibleDesde, setDisponibleDesde] = useState<string | null>(
    propDisponibleDesde ?? null,
  );
  const [estadiaMinimaMeses, setEstadiaMinimaMeses] = useState<number | null>(
    propEstadiaMinimaMeses ?? null,
  );

  useEffect(() => {
    if (propDisponibleDesde !== undefined) {
      setDisponibleDesde(propDisponibleDesde);
    }
  }, [propDisponibleDesde]);

  useEffect(() => {
    if (propEstadiaMinimaMeses !== undefined) {
      setEstadiaMinimaMeses(propEstadiaMinimaMeses);
    }
  }, [propEstadiaMinimaMeses]);

  const effectiveMinMonths =
    estadiaMinimaMeses && estadiaMinimaMeses >= 1 ? estadiaMinimaMeses : 1;

  const [startDate, setStartDate] = useState<string | null>(
    initialSelection?.date ?? null,
  );
  const [endDate, setEndDate] = useState<string | null>(() => {
    if (initialSelection?.date && initialSelection.duration > 1) {
      try {
        return addCivilDays(
          initialSelection.date,
          initialSelection.duration - 1,
        );
      } catch {
        return initialSelection.date;
      }
    }
    return initialSelection?.date ?? null;
  });

  const [selectedDuration, setSelectedDuration] = useState<number>(
    initialSelection?.duration && initialSelection.duration >= 1
      ? initialSelection.duration
      : 1,
  );
  const [monthCount, setMonthCount] = useState<number>(() => {
    if (rateUnit !== "mes") return 1;
    const initial =
      initialSelection?.duration && initialSelection.duration >= 1
        ? initialSelection.duration
        : 1;
    return Math.min(12, Math.max(effectiveMinMonths, initial));
  });

  useEffect(() => {
    if (rateUnit === "mes" && estadiaMinimaMeses && estadiaMinimaMeses > 1) {
      setMonthCount((prev) => Math.max(prev, estadiaMinimaMeses));
    }
  }, [rateUnit, estadiaMinimaMeses]);

  const effectiveDuration = useMemo(() => {
    if (rateUnit === "dia") {
      if (endDate && startDate && endDate >= startDate) {
        const days =
          Math.round(
            (new Date(`${endDate}T00:00:00Z`).getTime() -
              new Date(`${startDate}T00:00:00Z`).getTime()) /
              (1000 * 60 * 60 * 24),
          ) + 1;
        return Math.min(31, Math.max(1, days));
      }
      return selectedDuration;
    }
    return 1;
  }, [rateUnit, startDate, endDate, selectedDuration]);

  // Persistent map of selected slots across days, keyed by slot startAt ISO.
  const [selectedSlots, setSelectedSlots] = useState<
    Map<string, SelectedSlotEntry>
  >(() => new Map());

  const [monthData, setMonthData] = useState<
    Map<string, CalendarDayAvailability>
  >(new Map());
  const [loadingMonth, setLoadingMonth] = useState(false);
  const [monthError, setMonthError] = useState(false);
  const [loadingIntervals, setLoadingIntervals] = useState(false);
  const [rawSlots, setRawSlots] = useState<RentalScheduleSlot[]>([]);
  const [timeZone, setTimeZone] = useState<string>("America/Santiago");
  const intervalFence = useRef(createGenerationFence());

  const minDate = today;
  const maxDate = useMemo(() => {
    try {
      return addCivilDays(today, 90);
    } catch {
      return today;
    }
  }, [today]);

  const notifySelection = useCallback(
    (sel: MarketplaceBookingSelection | null) => {
      onSelectionChange?.(sel);
      if (typeof window !== "undefined") {
        window.dispatchEvent(
          new CustomEvent("espacigo:marketplace-selection", {
            detail: sel,
          }),
        );
      }
    },
    [onSelectionChange],
  );

  const emitFromSelectedMap = useCallback(
    (map: Map<string, SelectedSlotEntry>, tz: string) => {
      const entries = [...map.values()].sort(
        (a, b) => Date.parse(a.startAt) - Date.parse(b.startAt),
      );
      if (entries.length === 0) {
        notifySelection(null);
        return;
      }
      const merged = mergeContiguousIntervals(entries);
      const first = entries[0];
      const last = entries[entries.length - 1];
      const totalUnits = entries.reduce(
        (sum, entry) => sum + (entry.units ?? 1),
        0,
      );
      notifySelection({
        spaceId,
        date: first.date,
        duration: totalUnits,
        rateUnit,
        startAt: first.startAt,
        endAt: last.endAt,
        timeZone: tz,
        intervals: merged,
        ...(rateUnit === "mes" && estadiaMinimaMeses && estadiaMinimaMeses > 1
          ? { minMonths: estadiaMinimaMeses }
          : {}),
      });
    },
    [
      spaceId,
      rateUnit,
      effectiveDuration,
      notifySelection,
      estadiaMinimaMeses,
    ],
  );

  // Load the authoritative typed month snapshot.
  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    async function loadMonth() {
      await Promise.resolve();
      if (!active) return;
      setMonthData(new Map());
      setMonthError(false);
      setLoadingMonth(true);
      try {
        const result = await publicMonth.getMonth(
          spaceId,
          currentMonth,
          controller.signal,
        );
        if (!active) return;
        if (!result.ok) {
          setMonthError(true);
          return;
        }
        const map = new Map<string, CalendarDayAvailability>();
        for (const day of result.value.days) map.set(day.date, day);
        setMonthData(map);
        setTimeZone(result.value.timeZone);
        if (result.value.disponible_desde !== undefined) {
          setDisponibleDesde(result.value.disponible_desde);
        }
        if (result.value.estadia_minima_meses !== undefined) {
          setEstadiaMinimaMeses(result.value.estadia_minima_meses);
        }
      } catch {
        if (active) {
          setMonthData(new Map());
          setMonthError(true);
        }
      } finally {
        if (active) setLoadingMonth(false);
      }
    }

    void loadMonth();
    return () => {
      active = false;
      controller.abort();
    };
  }, [spaceId, currentMonth]);

  // Fetch authoritative schedule slots (available, held, booked) when startDate changes.
  // Preserves selectedSlots across day navigation!
  useEffect(() => {
    if (rateUnit !== "hora") return;
    const fence = intervalFence.current;
    const generation = fence.next();
    if (!startDate) return;

    const controller = new AbortController();
    const queryDuration = rateUnit === "hora" ? 1 : effectiveDuration;

    Promise.resolve()
      .then(() => {
        if (!fence.isCurrent(generation)) return;
        setLoadingIntervals(true);
        return publicRentalAvailability.getSchedule(
          spaceId,
          startDate,
          queryDuration,
          controller.signal,
        );
      })
      .then((result) => {
        if (!result || !fence.isCurrent(generation)) return;
        if (!result.ok) {
          setRawSlots([]);
          return;
        }
        const tz = result.value.time_zone || "America/Santiago";
        setTimeZone(tz);
        setRawSlots(result.value.slots);
      })
      .catch(() => {
        if (!fence.isCurrent(generation)) return;
        setRawSlots([]);
      })
      .finally(() => {
        if (fence.isCurrent(generation)) setLoadingIntervals(false);
      });

    return () => {
      fence.next();
      controller.abort();
    };
  }, [spaceId, startDate, effectiveDuration, rateUnit]);

  // Keep slot occupancy ("held" / "booked") synchronized while viewing a day.
  useEffect(() => {
    if (!startDate) return;
    let active = true;
    const queryDuration = rateUnit === "hora" ? 1 : effectiveDuration;

    const refreshSchedule = async () => {
      try {
        const result = await publicRentalAvailability.getSchedule(
          spaceId,
          startDate,
          queryDuration,
        );
        if (!active || !result.ok) return;
        setRawSlots(result.value.slots);
        const statusByStart = new Map(
          result.value.slots.map((s) => [s.start_at, s.status]),
        );
        setSelectedSlots((prev) => {
          let changed = false;
          const next = new Map(prev);
          for (const [key, entry] of prev.entries()) {
            if (entry.date === startDate) {
              const st = statusByStart.get(key);
              if (st && st !== "available") {
                next.delete(key);
                changed = true;
              }
            }
          }
          if (changed) {
            emitFromSelectedMap(next, result.value.time_zone || timeZone);
            return next;
          }
          return prev;
        });
      } catch {
        // Ignore transient background refresh errors.
      }
    };

    const intervalId = window.setInterval(() => {
      void refreshSchedule();
    }, 10_000);
    const handleFocus = () => {
      void refreshSchedule();
    };
    window.addEventListener("focus", handleFocus);

    return () => {
      active = false;
      window.clearInterval(intervalId);
      window.removeEventListener("focus", handleFocus);
    };
  }, [
    spaceId,
    startDate,
    effectiveDuration,
    rateUnit,
    timeZone,
    emitFromSelectedMap,
  ]);

  const handleDayClick = (date: string) => {
    if (disabled || date < minDate || date > maxDate) return;
    if (rateUnit === "mes" && disponibleDesde && date < disponibleDesde) return;
    if (rateUnit === "hora") {
      intervalFence.current.next();
      setLoadingIntervals(false);
      setRawSlots([]);
      setStartDate(date);
      setEndDate(null);
      return;
    }

    const dayAvail = monthData.get(date);
    const blocked =
      !dayAvail ||
      monthError ||
      dayAvail.status === "unavailable" ||
      dayAvail.status === "outside_horizon" ||
      dayAvail.occupancy === "held" ||
      dayAvail.occupancy === "booked";
    if (blocked) return;

    if (rateUnit === "dia") {
      const existingKey = Array.from(selectedSlots.entries()).find(
        ([, entry]) => entry.date === date,
      )?.[0];
      if (existingKey) {
        const next = new Map(selectedSlots);
        next.delete(existingKey);
        setSelectedSlots(next);
        emitFromSelectedMap(next, timeZone);
        if (startDate === date) setStartDate(null);
        return;
      }
      setStartDate(date);
      setLoadingIntervals(true);
      void publicRentalAvailability
        .getSchedule(spaceId, date, 1)
        .then((result) => {
          if (!result.ok) return;
          const tz = result.value.time_zone || timeZone;
          setTimeZone(tz);
          const availSlot = result.value.slots.find(
            (slot) => slot.status === "available",
          );
          if (!availSlot) return;
          setSelectedSlots((prev) => {
            const next = new Map(prev);
            next.set(availSlot.start_at, {
              date,
              startAt: availSlot.start_at,
              endAt: availSlot.end_at,
              units: 1,
            });
            emitFromSelectedMap(next, tz);
            return next;
          });
        })
        .finally(() => setLoadingIntervals(false));
      return;
    }

    // rateUnit === "mes": toggle or select N-month period starting on `date`
    const existingMonthKey = Array.from(selectedSlots.entries()).find(
      ([, entry]) =>
        entry.date === date ||
        (entry.endDate && date >= entry.date && date < entry.endDate),
    )?.[0];
    if (existingMonthKey) {
      const next = new Map(selectedSlots);
      next.delete(existingMonthKey);
      setSelectedSlots(next);
      emitFromSelectedMap(next, timeZone);
      if (startDate === date) setStartDate(null);
      return;
    }

    const minMonths =
      estadiaMinimaMeses && estadiaMinimaMeses >= 1 ? estadiaMinimaMeses : 1;
    const durationMonths = Math.max(minMonths, Math.min(12, monthCount));
    setStartDate(date);
    setLoadingIntervals(true);
    void publicRentalAvailability
      .getSchedule(spaceId, date, durationMonths)
      .then((result) => {
        if (!result.ok) return;
        const tz = result.value.time_zone || timeZone;
        setTimeZone(tz);
        const availSlot = result.value.slots.find(
          (slot) => slot.status === "available",
        );
        if (!availSlot) return;
        const endPeriod = addMonthsCivilDay(date, durationMonths);
        setSelectedSlots((prev) => {
          const next =
            estadiaMinimaMeses && estadiaMinimaMeses > 1
              ? new Map<string, SelectedSlotEntry>()
              : new Map(prev);
          next.set(availSlot.start_at, {
            date,
            endDate: endPeriod,
            startAt: availSlot.start_at,
            endAt: availSlot.end_at,
            units: durationMonths,
          });
          emitFromSelectedMap(next, tz);
          return next;
        });
      })
      .finally(() => setLoadingIntervals(false));
  };

  // Build grid days
  const grid = useMemo(() => monthDays(currentMonth), [currentMonth]);

  // Count selected slots per date for calendar badges
  const selectedCountByDate = useMemo(() => {
    const counts = new Map<string, number>();
    for (const entry of selectedSlots.values()) {
      counts.set(entry.date, (counts.get(entry.date) ?? 0) + 1);
    }
    return counts;
  }, [selectedSlots]);

  // Display slots formatted strictly in 24h without dots or "Mañana/Tarde/Noche"
  const tickets = useMemo(() => {
    if (!startDate) return [];
    const displaySlots = buildDisplaySlots(rawSlots, rateUnit);
    const formatT = (d: Date) =>
      d.toLocaleTimeString("es-CL", {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
        timeZone,
      });

    return displaySlots.map((slot, index) => {
      const dStart = new Date(slot.start_at);
      const dEnd = new Date(slot.end_at);
      return {
        id: slot.start_at,
        index,
        label: `${formatT(dStart)} – ${formatT(dEnd)}`,
        status: slot.status,
        slot,
      };
    });
  }, [startDate, rawSlots, rateUnit, timeZone]);

  const handleSelectTicket = (idx: number) => {
    const target = tickets[idx];
    if (!target || target.status !== "available" || !startDate) return;

    const next = new Map(selectedSlots);
    if (next.has(target.id)) {
      next.delete(target.id);
    } else {
      next.set(target.id, {
        date: startDate,
        startAt: target.slot.start_at,
        endAt: target.slot.end_at,
      });
    }
    setSelectedSlots(next);
    emitFromSelectedMap(next, timeZone);
  };

  const handleRemoveSlot = (startAt: string) => {
    const next = new Map(selectedSlots);
    next.delete(startAt);
    setSelectedSlots(next);
    emitFromSelectedMap(next, timeZone);
  };

  const handleClearAllSlots = () => {
    const empty = new Map<string, SelectedSlotEntry>();
    setSelectedSlots(empty);
    notifySelection(null);
  };

  const selectedEntries = useMemo(
    () =>
      [...selectedSlots.values()].sort(
        (a, b) => Date.parse(a.startAt) - Date.parse(b.startAt),
      ),
    [selectedSlots],
  );

  const formatSummarySlot = (entry: SelectedSlotEntry) => {
    try {
      const dStart = new Date(entry.startAt);
      const dEnd = new Date(entry.endAt);
      const dateTxt = new Intl.DateTimeFormat("es-CL", {
        day: "numeric",
        month: "short",
        timeZone,
      }).format(dStart);
      const fmtTime = (d: Date) =>
        d.toLocaleTimeString("es-CL", {
          hour: "2-digit",
          minute: "2-digit",
          hour12: false,
          timeZone,
        });
      return `${dateTxt} · ${fmtTime(dStart)}–${fmtTime(dEnd)}`;
    } catch {
      return `${entry.date} · ${entry.startAt}`;
    }
  };

  return (
    <section
      aria-labelledby={headingId}
      aria-busy={loadingMonth}
      className="w-full min-w-0 space-y-5 rounded-2xl border border-neutral-200 bg-white p-5 shadow-xs sm:p-6"
    >
      {/* Header with Month Navigation */}
      <header className="flex items-center justify-between gap-3">
        <div>
          <h2
            id={headingId}
            className="text-lg font-bold tracking-tight text-neutral-900 capitalize"
          >
            {monthLabel(currentMonth)}
          </h2>
          <p className="text-xs text-neutral-500">
            {rateUnit === "hora"
              ? "Elige uno o más días y marca los horarios que necesites (uno a uno)"
              : rateUnit === "dia"
                ? "Haz clic en los días disponibles que necesites (sueltos o continuos)"
                : "Elige la fecha de inicio disponible y la cantidad de meses"}
          </p>
          {rateUnit === "mes" && (
            <div className="mt-2.5 flex flex-wrap items-center gap-2 text-xs">
              {disponibleDesde && (
                <span className="inline-flex items-center gap-1.5 rounded-lg border border-[#e8c5b5] bg-[#fbf7f2] px-2.5 py-1 text-xs font-medium text-[#702a1b]">
                  <span
                    className="h-2 w-2 rounded-full bg-[#a44731]"
                    aria-hidden="true"
                  />
                  Disponible desde:{" "}
                  <strong className="font-semibold text-[#332720]">
                    {formatCivilDate(disponibleDesde)}
                  </strong>
                </span>
              )}
              {estadiaMinimaMeses && estadiaMinimaMeses > 1 ? (
                <span className="inline-flex items-center gap-1.5 rounded-lg border border-[#e8c5b5] bg-[#fbf7f2] px-2.5 py-1 text-xs font-medium text-[#702a1b]">
                  <span
                    className="h-2 w-2 rounded-full bg-[#a44731]"
                    aria-hidden="true"
                  />
                  Estancia mínima:{" "}
                  <strong className="font-semibold text-[#332720]">
                    {estadiaMinimaMeses} meses
                  </strong>
                </span>
              ) : null}
            </div>
          )}
          {rateUnit === "mes" && (
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
              <label
                htmlFor={`${headingId}-month-count`}
                className="font-medium text-neutral-700"
              >
                Duración:
              </label>
              <select
                id={`${headingId}-month-count`}
                value={monthCount}
                disabled={disabled}
                onChange={(e) =>
                  setMonthCount(
                    Math.max(
                      effectiveMinMonths,
                      Number(e.target.value) || effectiveMinMonths,
                    ),
                  )
                }
                className="rounded-lg border border-neutral-300 bg-white px-2.5 py-1 text-xs font-medium text-neutral-800"
              >
                {Array.from(
                  { length: 12 - effectiveMinMonths + 1 },
                  (_, idx) => effectiveMinMonths + idx,
                ).map((m) => (
                  <option key={m} value={m}>
                    {m} {m === 1 ? "mes" : "meses"}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>

        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="Mes anterior"
            disabled={disabled || !shiftMonth(currentMonth, -1)}
            onClick={() => {
              const prev = shiftMonth(currentMonth, -1);
              if (prev) {
                setMonthData(new Map());
                setStartDate(null);
                setEndDate(null);
                setRawSlots([]);
                setCurrentMonth(prev);
              }
            }}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-neutral-200 text-neutral-700 hover:bg-neutral-100 disabled:opacity-40 transition-colors"
          >
            ‹
          </button>
          <button
            type="button"
            aria-label="Mes siguiente"
            disabled={disabled || !shiftMonth(currentMonth, 1)}
            onClick={() => {
              const next = shiftMonth(currentMonth, 1);
              if (next) {
                setMonthData(new Map());
                setStartDate(null);
                setEndDate(null);
                setRawSlots([]);
                setCurrentMonth(next);
              }
            }}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-neutral-200 text-neutral-700 hover:bg-neutral-100 disabled:opacity-40 transition-colors"
          >
            ›
          </button>
        </div>
      </header>

      {/* Clean 7-column Calendar Grid */}
      <div className="space-y-1">
        <div className="grid grid-cols-7 gap-1">
          {WEEKDAYS.map((day) => (
            <div
              key={day}
              className="py-1.5 text-center text-xs font-semibold text-neutral-600"
            >
              {day}
            </div>
          ))}
        </div>

        <div className="grid grid-cols-7 gap-y-1">
          {grid.map(({ date, inMonth }: MonthDay) => {
            if (!inMonth) {
              return (
                <div
                  key={date}
                  aria-hidden="true"
                  className="h-14 w-full opacity-0"
                />
              );
            }

            const dayNumber = Number(date.slice(-2));
            const dayAvail = monthData.get(date);
            const isBeforeDisponibleDesde =
              rateUnit === "mes" &&
              Boolean(disponibleDesde && date < disponibleDesde);
            const isOutsideRange =
              date < minDate || date > maxDate || isBeforeDisponibleDesde;
            const isBlocked =
              isOutsideRange ||
              !dayAvail ||
              monthError ||
              dayAvail.status === "unavailable" ||
              dayAvail.status === "outside_horizon";
            const isPartial = dayAvail?.status === "partial";
            const isHeld =
              !isOutsideRange && !monthError && dayAvail?.occupancy === "held";
            const isConfirmed =
              !isOutsideRange &&
              !monthError &&
              dayAvail?.occupancy === "booked";

            const daySelectedCount = selectedCountByDate.get(date) ?? 0;
            const isDirectDaySelected =
              rateUnit !== "hora" &&
              Array.from(selectedSlots.values()).some((entry) =>
                entry.endDate
                  ? date >= entry.date && date < entry.endDate
                  : entry.date === date,
              );
            const isStart =
              (rateUnit === "hora" && startDate === date) ||
              isDirectDaySelected;
            const isEnd = endDate === date;
            const isSingle = isStart && (!endDate || endDate === startDate);
            const isIntermediate =
              startDate && endDate && date > startDate && date < endDate;
            const isSelected =
              isStart || isEnd || isIntermediate || isDirectDaySelected;

            let cellClass =
              "relative flex h-14 w-full flex-col items-center justify-center p-1 text-center transition-all ";

            if (isHeld) {
              cellClass +=
                "bg-orange-50 border border-orange-300 text-neutral-900 rounded-lg ";
              if (isBlocked) cellClass += "cursor-not-allowed ";
              if (isSelected) cellClass += "ring-2 ring-inset ring-orange-500 ";
            } else if (isBlocked) {
              cellClass +=
                "cursor-not-allowed bg-neutral-100/60 text-neutral-400 opacity-55 border border-neutral-200/40 rounded-lg ";
            } else if (isConfirmed || isPartial) {
              cellClass +=
                "bg-neutral-100 border border-neutral-300 text-neutral-900 rounded-lg hover:border-neutral-500 ";
              if (isSelected) cellClass += "ring-2 ring-inset ring-[#A83820] ";
            } else if (isSingle) {
              cellClass +=
                "bg-[#A83820] text-white rounded-lg shadow-sm font-semibold ";
            } else if (isStart) {
              cellClass +=
                "bg-[#A83820] text-white rounded-l-lg rounded-r-none font-semibold shadow-xs ";
            } else if (isEnd) {
              cellClass +=
                "bg-[#A83820] text-white rounded-r-lg rounded-l-none font-semibold shadow-xs ";
            } else if (isIntermediate) {
              cellClass +=
                "bg-[#F9EBE8] text-[#A83820] rounded-none font-medium ";
            } else if (daySelectedCount > 0) {
              cellClass +=
                "bg-[#F9EBE8] border-2 border-[#A83820] text-[#A83820] rounded-lg font-semibold ";
            } else {
              cellClass +=
                "bg-white border border-neutral-200 text-neutral-900 rounded-lg hover:border-[#A83820]/40 hover:bg-neutral-50/50 ";
            }

            const priceDisplay =
              isBlocked || !dayAvail
                ? "-"
                : formatCompactPrice(dayAvail.price ?? null);

            return (
              <div key={date} className="px-0.5">
                <button
                  type="button"
                  disabled={disabled || isBlocked}
                  aria-pressed={Boolean(isSelected || daySelectedCount > 0)}
                  onClick={() => handleDayClick(date)}
                  className={cellClass}
                >
                  {daySelectedCount > 0 && (
                    <span className="absolute top-0.5 right-0.5 rounded bg-[#A83820] px-1 text-[9px] font-bold leading-tight text-white">
                      {daySelectedCount}
                      {rateUnit === "hora"
                        ? "h"
                        : rateUnit === "dia"
                          ? "d"
                          : "m"}
                    </span>
                  )}
                  <span className="font-semibold text-sm leading-tight">
                    <span>{dayNumber}</span>
                  </span>
                  <span
                    className={`text-[11px] leading-tight ${
                      isSelected &&
                      !isBlocked &&
                      !isPartial &&
                      !isHeld &&
                      !isConfirmed
                        ? isIntermediate
                          ? "text-[#A83820]"
                          : "text-white/90"
                        : "text-neutral-500"
                    }`}
                  >
                    {priceDisplay}
                  </span>
                  <span className="sr-only">
                    , {localDateLabel(date)}
                    {dayAvail
                      ? `, ${dayAvail.status}`
                      : ", Disponibilidad no confirmada"}
                    {isHeld ? ", en proceso de reserva" : ""}
                    {isConfirmed ? ", con reserva confirmada" : ""}
                    {isSelected ? ", seleccionado" : ""}
                  </span>
                </button>
              </div>
            );
          })}
        </div>
      </div>

      {monthError && (
        <p role="alert" className="break-words text-sm text-red-800">
          No se pudo cargar la disponibilidad de este mes. Inténtalo de nuevo.
        </p>
      )}

      {/* Color Legend */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-neutral-100 pt-3 text-xs text-neutral-600">
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-3 w-3 rounded-sm border border-emerald-300 bg-emerald-100" />
          Disponible
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-3 w-3 rounded-sm border border-orange-300 bg-orange-200" />
          En proceso (otro usuario)
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-3 w-3 rounded-sm border border-neutral-300 bg-neutral-200" />
          Ocupado / Bloqueado
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-3 w-3 rounded-sm bg-[#A83820]" />
          Tu selección
        </span>
      </div>

      {/* Interactive Time Slots (only for hourly listings) */}
      {rateUnit === "hora" && startDate && (
        <div className="space-y-3 rounded-xl border border-neutral-200 bg-neutral-50/70 p-4 transition-all">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h3 className="text-sm font-semibold text-neutral-900">
                Horarios para el {startDate}
                {endDate && endDate !== startDate ? ` al ${endDate}` : ""}
              </h3>
              {rateUnit === "hora" && tickets.length > 0 && (
                <p className="text-[11px] text-neutral-500">
                  Haz clic en cada horario para marcarlo o desmarcarlo. Puedes
                  elegir horarios en distintos días.
                </p>
              )}
            </div>
            {loadingIntervals && (
              <span className="text-xs text-neutral-500 animate-pulse">
                Consultando disponibilidad...
              </span>
            )}
          </div>

          {!loadingIntervals && tickets.length === 0 ? (
            <p className="rounded-lg border border-dashed border-neutral-300 bg-white p-4 text-center text-xs text-neutral-500">
              No hay horarios publicados para esta fecha.
            </p>
          ) : (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
              {tickets.map((t) => {
                const isSelected = selectedSlots.has(t.id);
                const isHeld = t.status === "held";
                const isBooked = t.status === "booked";
                const isSlotDisabled = disabled || isHeld || isBooked;

                let cardStyle = "";
                if (isBooked) {
                  cardStyle =
                    "border-neutral-300 bg-neutral-200/80 text-neutral-500 cursor-not-allowed line-through";
                } else if (isHeld) {
                  cardStyle =
                    "border-orange-300 bg-orange-100 text-orange-900 cursor-not-allowed";
                } else if (isSelected) {
                  cardStyle =
                    "border-[#A83820] bg-[#A83820] text-white shadow-xs ring-2 ring-[#A83820]/20";
                } else {
                  cardStyle =
                    "border-emerald-200 bg-emerald-50/80 text-emerald-950 hover:border-emerald-400 hover:bg-emerald-100/80";
                }

                return (
                  <button
                    key={t.id}
                    type="button"
                    disabled={isSlotDisabled}
                    aria-pressed={isSelected}
                    title={
                      isBooked
                        ? "Horario ocupado"
                        : isHeld
                          ? "Otro usuario está reservando este horario"
                          : "Horario disponible"
                    }
                    onClick={() => handleSelectTicket(t.index)}
                    className={`flex w-full items-center justify-center rounded-xl border px-3 py-2.5 text-center text-xs font-semibold tabular-nums transition-all ${cardStyle}`}
                  >
                    {t.label}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Persistent Multi-Day / Multi-Slot Selection Summary */}
      {selectedEntries.length > 0 && (
        <div className="space-y-2.5 rounded-xl border border-[#A83820]/25 bg-[#F9EBE8]/40 p-3.5">
          <div className="flex items-center justify-between gap-2">
            <p className="text-xs font-semibold text-neutral-900">
              Tramos elegidos ({selectedEntries.length}{" "}
              {rateUnit === "hora"
                ? selectedEntries.length === 1
                  ? "hora"
                  : "horas"
                : rateUnit === "dia"
                  ? selectedEntries.length === 1
                    ? "día"
                    : "días"
                  : selectedEntries.length === 1
                    ? "mes"
                    : "meses"}
              )
            </p>
            <button
              type="button"
              onClick={handleClearAllSlots}
              className="text-xs font-medium text-[#A83820] underline underline-offset-2 hover:text-[#862b17]"
            >
              Limpiar todo
            </button>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {selectedEntries.map((entry) => (
              <span
                key={entry.startAt}
                className="inline-flex items-center gap-1.5 rounded-lg border border-[#A83820]/30 bg-white px-2.5 py-1 text-xs font-medium text-neutral-900"
              >
                <span className="tabular-nums">{formatSummarySlot(entry)}</span>
                <button
                  type="button"
                  onClick={() => handleRemoveSlot(entry.startAt)}
                  aria-label={`Quitar ${formatSummarySlot(entry)}`}
                  className="rounded px-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}
