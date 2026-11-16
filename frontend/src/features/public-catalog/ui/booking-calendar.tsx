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
import type { MarketplaceBookingSelection } from "../domain/marketplace-booking-selection";
import type { PublicRentalAvailability } from "../domain/rental-availability";
import {
  availabilitySelection,
  createGenerationFence,
} from "../domain/booking-calendar";
import { publicRentalAvailability } from "../infrastructure/rental-availability-runtime";
import { publicMonth } from "../infrastructure/public-month-openapi-adapter";

export interface CalendarDayAvailability {
  date: string;
  status: "available" | "partial" | "unavailable" | "outside_horizon";
  price?: number;
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

export function PublicBookingCalendar({
  spaceId,
  rateUnit,
  today,
  initialSelection,
  onSelectionChange,
  disabled = false,
}: PublicBookingCalendarProps) {
  const headingId = useId();
  const [currentMonth, setCurrentMonth] = useState(
    initialSelection?.date.slice(0, 7) ?? today.slice(0, 7),
  );

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

  const [selectedStayDays, setSelectedStayDays] = useState<number>(
    initialSelection?.duration ?? (rateUnit === "hora" ? 2 : 1),
  );

  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(
    () => {
      if (initialSelection?.startAt) {
        return initialSelection.startAt;
      }
      return null;
    },
  );

  const [monthData, setMonthData] = useState<
    Map<string, CalendarDayAvailability>
  >(new Map());
  const [loadingMonth, setLoadingMonth] = useState(false);
  const [monthError, setMonthError] = useState(false);
  const [loadingIntervals, setLoadingIntervals] = useState(false);
  const [intervals, setIntervals] = useState<PublicRentalAvailability["items"]>(
    [],
  );
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
          notifySelection(null);
          return;
        }
        const map = new Map<string, CalendarDayAvailability>();
        for (const day of result.value.days) map.set(day.date, day);
        setMonthData(map);
        setTimeZone(result.value.timeZone);
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
  }, [spaceId, currentMonth, notifySelection]);

  // Fetch authoritative intervals when startDate changes
  useEffect(() => {
    const fence = intervalFence.current;
    const generation = fence.next();
    if (!startDate) return;

    const durationDays =
      endDate && endDate >= startDate
        ? Math.max(
            1,
            Math.round(
              (new Date(`${endDate}T00:00:00Z`).getTime() -
                new Date(`${startDate}T00:00:00Z`).getTime()) /
                (1000 * 60 * 60 * 24),
            ) + 1,
          )
        : selectedStayDays;

    const controller = new AbortController();

    Promise.resolve()
      .then(() => {
        if (!fence.isCurrent(generation)) return;
        setLoadingIntervals(true);
        return publicRentalAvailability.getAvailability(
          spaceId,
          startDate,
          rateUnit === "hora" ? selectedStayDays : durationDays,
          controller.signal,
        );
      })
      .then((result) => {
        if (!result || !fence.isCurrent(generation)) return;
        if (!result.ok) {
          setIntervals([]);
          setSelectedTicketId(null);
          notifySelection(null);
          return;
        }
        setIntervals(result.value.items);
        if (result.value.time_zone) {
          setTimeZone(result.value.time_zone);
        }

        // If an interval was already selected or we can pick the first available
        const selection = availabilitySelection(
          result.value.items,
          {
            spaceId,
            date: startDate,
            duration: rateUnit === "hora" ? selectedStayDays : durationDays,
            rateUnit,
          },
          result.value.time_zone,
        );
        setSelectedTicketId(selection?.startAt ?? null);
        notifySelection(selection);
      })
      .catch(() => {
        if (!fence.isCurrent(generation)) return;
        setIntervals([]);
        setSelectedTicketId(null);
        notifySelection(null);
      })
      .finally(() => {
        if (fence.isCurrent(generation)) setLoadingIntervals(false);
      });

    return () => {
      fence.next();
      controller.abort();
    };
  }, [
    spaceId,
    startDate,
    endDate,
    selectedStayDays,
    rateUnit,
    notifySelection,
  ]);

  const invalidateIntervals = () => {
    intervalFence.current.next();
    setLoadingIntervals(false);
    setIntervals([]);
    setSelectedTicketId(null);
    notifySelection(null);
  };

  // Handle day click for Check-in / Check-out range
  const handleDayClick = (date: string) => {
    if (disabled || date < minDate || date > maxDate) return;
    invalidateIntervals();

    if (!startDate || (startDate && endDate)) {
      // First click: sets start date, clears end date
      setStartDate(date);
      setEndDate(null);
      setSelectedTicketId(null);
      notifySelection(null);
    } else if (startDate && !endDate) {
      // Second click: completes the range
      if (date >= startDate) {
        setEndDate(date);
      } else {
        // Clicked date is earlier than startDate: reset start to clicked date
        setStartDate(date);
        setEndDate(null);
      }
    }
  };

  // Handle stay pills: "1 día", "2 días", "3 días", "1 semana"
  const handleStayPill = (days: number) => {
    invalidateIntervals();
    setSelectedStayDays(days);
    if (startDate) {
      try {
        const nextEnd = addCivilDays(startDate, days - 1);
        setEndDate(nextEnd);
      } catch {
        setEndDate(startDate);
      }
    }
  };

  // Build grid days
  const grid = useMemo(() => monthDays(currentMonth), [currentMonth]);

  // Tickets representation
  const tickets = useMemo(() => {
    if (!startDate) return [];

    if (intervals.length > 0) {
      return intervals.map((it, idx) => {
        const dStart = new Date(it.start_at);
        const dEnd = new Date(it.end_at);
        const formatT = (d: Date) =>
          d.toLocaleTimeString("es-CL", {
            hour: "2-digit",
            minute: "2-digit",
            hour12: false,
            timeZone,
          });
        const hourStart = dStart.getUTCHours();
        const label =
          idx === 0 || hourStart < 13
            ? "Mañana"
            : hourStart < 18
              ? "Tarde"
              : "Noche";
        const symbol = "●";

        return {
          id: it.start_at,
          symbol,
          symbolColor: "text-emerald-500",
          title: `${label} · ${formatT(dStart)} a ${formatT(dEnd)}`,
          interval: it,
        };
      });
    }

    return [];
  }, [startDate, intervals, timeZone]);

  const handleSelectTicket = (t: (typeof tickets)[number]) => {
    setSelectedTicketId(t.id);
    if (!startDate) return;

    const durationDays =
      endDate && endDate >= startDate
        ? Math.max(
            1,
            Math.round(
              (new Date(`${endDate}T00:00:00Z`).getTime() -
                new Date(`${startDate}T00:00:00Z`).getTime()) /
                (1000 * 60 * 60 * 24),
            ) + 1,
          )
        : selectedStayDays;

    notifySelection({
      spaceId,
      date: startDate,
      duration: rateUnit === "hora" ? selectedStayDays : durationDays,
      rateUnit,
      startAt: t.interval.start_at,
      endAt: t.interval.end_at,
      timeZone,
    });
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
            Selecciona tu fecha de check-in y check-out
          </p>
        </div>

        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="Mes anterior"
            disabled={disabled || !shiftMonth(currentMonth, -1)}
            onClick={() => {
              const prev = shiftMonth(currentMonth, -1);
              if (prev) {
                invalidateIntervals();
                setMonthData(new Map());
                setStartDate(null);
                setEndDate(null);
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
                invalidateIntervals();
                setMonthData(new Map());
                setStartDate(null);
                setEndDate(null);
                setCurrentMonth(next);
              }
            }}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-neutral-200 text-neutral-700 hover:bg-neutral-100 disabled:opacity-40 transition-colors"
          >
            ›
          </button>
        </div>
      </header>

      {/* Stay Pills */}
      <div className="flex flex-wrap items-center gap-2 pt-1">
        <span className="text-xs font-semibold text-neutral-600 mr-1">
          Estadía sugerida:
        </span>
        {[
          { label: "1 día", days: 1 },
          { label: "2 días", days: 2 },
          { label: "3 días", days: 3 },
          { label: "1 semana", days: 7 },
        ].map((pill) => {
          const isPillActive = selectedStayDays === pill.days;
          return (
            <button
              key={pill.days}
              type="button"
              disabled={disabled}
              onClick={() => handleStayPill(pill.days)}
              className={`rounded-full px-3 py-1 text-xs font-medium transition-all ${
                isPillActive
                  ? "bg-[#A83820] text-white shadow-xs"
                  : "bg-neutral-100 text-neutral-700 hover:bg-neutral-200/80"
              }`}
            >
              {pill.label}
            </button>
          );
        })}
      </div>

      {/* Clean 7-column Calendar Grid */}
      <div className="space-y-1">
        {/* Clean Weekday Headers */}
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

        {/* Days Grid */}
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
            const isOutsideRange = date < minDate || date > maxDate;
            const isBlocked =
              isOutsideRange ||
              !dayAvail ||
              monthError ||
              dayAvail.status === "unavailable" ||
              dayAvail.status === "outside_horizon";
            const isPartial = dayAvail?.status === "partial";

            // Terracotta continuous selection band logic
            const isStart = startDate === date;
            const isEnd = endDate === date;
            const isSingle = isStart && (!endDate || endDate === startDate);
            const isIntermediate =
              startDate && endDate && date > startDate && date < endDate;
            const isSelected = isStart || isEnd || isIntermediate;

            // Compute styling
            let cellClass =
              "relative flex h-14 w-full flex-col items-center justify-center p-1 text-center transition-all ";

            if (isBlocked) {
              cellClass +=
                "cursor-not-allowed bg-neutral-100/50 text-neutral-400 opacity-50 border border-neutral-200/40 ";
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
            } else if (isPartial) {
              cellClass +=
                "bg-amber-50/70 border border-amber-300 text-neutral-900 rounded-lg hover:border-amber-400 ";
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
                  aria-pressed={Boolean(isSelected)}
                  onClick={() => handleDayClick(date)}
                  className={cellClass}
                >
                  <span className="font-semibold text-sm leading-tight">
                    <span>{dayNumber}</span>
                  </span>
                  <span
                    className={`text-[11px] leading-tight ${
                      isSelected
                        ? isIntermediate
                          ? "text-[#A83820]"
                          : "text-white/90"
                        : "text-neutral-500"
                    }`}
                  >
                    {priceDisplay}
                  </span>
                  {/* Subtle semantic amber dot for partial if not in selection */}
                  {isPartial && !isSelected && (
                    <span
                      aria-hidden="true"
                      className="absolute bottom-1 h-1.5 w-1.5 rounded-full bg-amber-500"
                    />
                  )}
                  {/* Accessible label for screen readers & backwards compatibility */}
                  <span className="sr-only">
                    , {localDateLabel(date)}
                    {dayAvail
                      ? `, ${dayAvail.status}`
                      : ", Disponibilidad no confirmada"}
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

      {/* Chromatic Legend with Circles */}
      <div className="flex flex-wrap items-center justify-between gap-y-2 border-t border-neutral-100 pt-3 text-xs text-neutral-600">
        <span className="flex items-center">
          <span className="h-2.5 w-2.5 rounded-full bg-emerald-500 inline-block mr-1"></span>{" "}
          Disponible
        </span>
        <span className="flex items-center">
          <span className="h-2.5 w-2.5 rounded-full bg-amber-500 inline-block mr-1"></span>{" "}
          Parcial
        </span>
        <span className="flex items-center">
          <span className="h-2.5 w-2.5 rounded-full bg-neutral-400 inline-block mr-1"></span>{" "}
          Ocupado / Bloqueado
        </span>
        <span className="flex items-center">
          <span className="h-2.5 w-2.5 rounded-full bg-[#A83820] inline-block mr-1"></span>{" "}
          Tu selección
        </span>
      </div>

      {/* Interactive Ticket Time Slots */}
      {startDate && (
        <div className="space-y-2.5 rounded-xl border border-neutral-200 bg-neutral-50/60 p-4 transition-all">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-neutral-900">
              Horarios para el {startDate}
              {endDate && endDate !== startDate ? ` al ${endDate}` : ""}:
            </h3>
            {loadingIntervals && (
              <span className="text-xs text-neutral-500 animate-pulse">
                Consultando disponibilidad...
              </span>
            )}
          </div>

          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            {tickets.map((t) => {
              const active = selectedTicketId === t.id;
              return (
                <button
                  key={t.id}
                  type="button"
                  disabled={disabled}
                  onClick={() => handleSelectTicket(t)}
                  className={`flex items-center justify-start gap-2 rounded-xl border p-3 text-left transition-all ${
                    active
                      ? "border-[#A83820] bg-white ring-2 ring-[#A83820]/20 shadow-xs"
                      : "border-neutral-200 bg-white hover:border-neutral-300 hover:bg-neutral-50"
                  }`}
                >
                  <span className={`text-sm ${t.symbolColor} shrink-0`}>
                    {t.symbol}
                  </span>
                  <span className="text-xs font-semibold text-neutral-800 leading-snug">
                    {t.title}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </section>
  );
}
