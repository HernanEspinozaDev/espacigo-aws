"use client";

import { useId, useMemo, useState } from "react";
import {
  monthDays,
  shiftMonth,
  localDateLabel,
} from "@/features/rental-demo/domain/calendar";
import type { ScheduleSlot } from "../domain/types";
import { formatCompactPrice } from "@/features/public-catalog/ui/booking-calendar";

export interface HostAvailabilityCalendarProps {
  spaceId?: string;
  rateUnit?: "hora" | "dia" | "mes";
  basePriceCLP?: number | string | null;
  timeZone?: string;
  onTimeZoneChange?: (tz: string) => void;
  onScheduleChange?: (slots: ScheduleSlot[]) => void;
  initialSlots?: readonly ScheduleSlot[];
  disabled?: boolean;
}

interface CustomDateOverride {
  blocked: boolean;
  specialRateCLP?: number;
}

const DAY_PILLS = [
  { day: 1, label: "L", name: "Lunes" },
  { day: 2, label: "M", name: "Martes" },
  { day: 3, label: "X", name: "Miércoles" },
  { day: 4, label: "J", name: "Jueves" },
  { day: 5, label: "V", name: "Viernes" },
  { day: 6, label: "S", name: "Sábado" },
  { day: 7, label: "D", name: "Domingo" },
] as const;

const WEEKDAYS = ["Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"] as const;

function currentMonthISO(): string {
  const now = new Date();
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, "0");
  return `${y}-${m}`;
}

export function HostAvailabilityCalendar({
  spaceId,
  rateUnit = "hora",
  basePriceCLP,
  timeZone = "America/Santiago",
  onScheduleChange,
  initialSlots = [],
  disabled = false,
}: HostAvailabilityCalendarProps) {
  const headingId = useId();

  // 1. Quick Base Schedule Configurator State
  const [activeDays, setActiveDays] = useState<Set<number>>(
    () => new Set([1, 2, 3, 4, 5]), // Default: Mon-Fri
  );
  const [openTime, setOpenTime] = useState<string>(
    rateUnit === "dia" ? "15:00" : "09:00",
  );
  const [closeTime, setCloseTime] = useState<string>(
    rateUnit === "dia" ? "12:00" : "20:00",
  );
  const [ruleAppliedFeedback, setRuleAppliedFeedback] = useState<string | null>(
    null,
  );

  // 2. Calendar Month & Multi-Selection State
  const [month, setMonth] = useState<string>(currentMonthISO());
  const [selectedDates, setSelectedDates] = useState<Set<string>>(new Set());
  const [isDragging, setIsDragging] = useState(false);
  const [dateOverrides, setDateOverrides] = useState<
    Map<string, CustomDateOverride>
  >(new Map());
  const [specialRateInput, setSpecialRateInput] = useState<string>("");

  // 3. API Sync State
  const [savingServer, setSavingServer] = useState(false);
  const [serverMessage, setServerMessage] = useState<{
    type: "success" | "error";
    text: string;
  } | null>(null);

  // Toggle active days [L] [M] [X] [J] [V] [S] [D]
  const toggleDay = (day: number) => {
    setActiveDays((prev) => {
      const next = new Set(prev);
      if (next.has(day)) {
        if (next.size > 1) next.delete(day);
      } else {
        next.add(day);
      }
      return next;
    });
  };

  // 1-Click apply base rule to whole year
  const handleApplyBaseRule = () => {
    const slots: ScheduleSlot[] = [];
    const sortedDays = Array.from(activeDays).sort((a, b) => a - b);

    for (const d of sortedDays) {
      slots.push({
        day_of_week: d,
        start_time: openTime,
        end_time: closeTime,
      });
    }

    onScheduleChange?.(slots);
    setRuleAppliedFeedback(
      `¡Regla base aplicada exitosamente a ${sortedDays.length} días de la semana (${openTime} a ${closeTime})!`,
    );
    setTimeout(() => {
      setRuleAppliedFeedback(null);
    }, 4500);
  };

  // Multi-Selection handler for clicking on dates
  const handleDateClick = (date: string) => {
    setSelectedDates((prev) => {
      const next = new Set(prev);
      if (next.has(date)) {
        next.delete(date);
      } else {
        next.add(date);
      }
      return next;
    });
  };

  const handleMouseEnterDate = (date: string) => {
    if (isDragging) {
      setSelectedDates((prev) => {
        const next = new Set(prev);
        next.add(date);
        return next;
      });
    }
  };

  // Actions for selected dates
  const handleBlockSelectedDates = () => {
    if (selectedDates.size === 0) return;
    setDateOverrides((prev) => {
      const next = new Map(prev);
      for (const d of selectedDates) {
        const existing = next.get(d) ?? { blocked: false };
        next.set(d, { ...existing, blocked: true });
      }
      return next;
    });
    setSelectedDates(new Set());
  };

  const handleUnblockSelectedDates = () => {
    if (selectedDates.size === 0) return;
    setDateOverrides((prev) => {
      const next = new Map(prev);
      for (const d of selectedDates) {
        const existing = next.get(d);
        if (existing) {
          next.set(d, { ...existing, blocked: false });
        }
      }
      return next;
    });
    setSelectedDates(new Set());
  };

  const handleApplySpecialRate = () => {
    const rate = parseInt(specialRateInput.replace(/\D/g, ""), 10);
    if (isNaN(rate) || rate <= 0 || selectedDates.size === 0) return;

    setDateOverrides((prev) => {
      const next = new Map(prev);
      for (const d of selectedDates) {
        const existing = next.get(d) ?? { blocked: false };
        next.set(d, { ...existing, specialRateCLP: rate });
      }
      return next;
    });
    setSpecialRateInput("");
    setSelectedDates(new Set());
  };

  const handleSelectAllMonth = () => {
    const grid = monthDays(month);
    const inMonthDates = grid.filter((g) => g.inMonth).map((g) => g.date);
    setSelectedDates(new Set(inMonthDates));
  };

  const handleClearSelection = () => {
    setSelectedDates(new Set());
  };

  // Save to backend: PUT /api/v1/spaces/{spaceId}/availability
  const handleSaveToBackend = async () => {
    if (!spaceId) {
      setServerMessage({
        type: "success",
        text: "Configuración guardada localmente para la creación del espacio.",
      });
      return;
    }

    setSavingServer(true);
    setServerMessage(null);
    try {
      const res = await fetch(`/api/v1/spaces/${encodeURIComponent(spaceId)}/availability`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Accept: "application/json",
        },
        body: JSON.stringify({
          time_zone: timeZone,
        }),
      });

      if (res.ok) {
        setServerMessage({
          type: "success",
          text: "Disponibilidad sincronizada con éxito en el servidor.",
        });
      } else {
        setServerMessage({
          type: "error",
          text: "No se pudo sincronizar la disponibilidad con el servidor.",
        });
      }
    } catch {
      setServerMessage({
        type: "error",
        text: "Error de red al conectar con el servidor de disponibilidad.",
      });
    } finally {
      setSavingServer(false);
    }
  };

  const grid = useMemo(() => monthDays(month), [month]);

  const monthTitle = useMemo(() => {
    const d = new Date(`${month}-01T00:00:00Z`);
    return new Intl.DateTimeFormat("es-CL", {
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    }).format(d);
  }, [month]);

  return (
    <div
      aria-labelledby={headingId}
      className="space-y-6 rounded-2xl border border-neutral-200 bg-white p-5 shadow-xs sm:p-6"
      onMouseUp={() => setIsDragging(false)}
    >
      <header className="border-b border-neutral-100 pb-4">
        <h2
          id={headingId}
          className="text-lg font-bold tracking-tight text-neutral-900"
        >
          Gestor Avanzado de Disponibilidad y Tarifas
        </h2>
        <p className="text-xs text-neutral-500">
          Configura tus reglas base recurrentes y personaliza fechas específicas
          del año.
        </p>
      </header>

      {/* 1. Configurador Rápido de Horario Base (1 clic) */}
      <section
        aria-label="Configurador rápido de horario base"
        className="rounded-xl border border-neutral-200 bg-neutral-50/70 p-4 space-y-4"
      >
        <div className="flex items-center justify-between">
          <span className="text-sm font-semibold text-neutral-800">
            Regla de Horario Base (Días y Horarios Habituales)
          </span>
          <span className="text-xs font-medium text-neutral-500">
            Zona: {timeZone}
          </span>
        </div>

        {/* Day Pills: [L] [M] [X] [J] [V] [S] [D] */}
        <div>
          <label className="text-xs font-medium text-neutral-600 block mb-1.5">
            Días activos de la semana:
          </label>
          <div className="flex flex-wrap gap-1.5">
            {DAY_PILLS.map((pill) => {
              const active = activeDays.has(pill.day);
              return (
                <button
                  key={pill.day}
                  type="button"
                  disabled={disabled}
                  onClick={() => toggleDay(pill.day)}
                  title={pill.name}
                  aria-pressed={active}
                  className={`flex h-9 w-9 items-center justify-center rounded-lg text-xs font-bold transition-all ${
                    active
                      ? "bg-[#A83820] text-white shadow-xs scale-105"
                      : "bg-white text-neutral-600 border border-neutral-300 hover:bg-neutral-100"
                  }`}
                >
                  {pill.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* Opening / Closing times */}
        <div className="grid grid-cols-2 gap-3 max-w-sm">
          <div>
            <label className="text-xs font-medium text-neutral-600 block mb-1">
              {rateUnit === "dia" ? "Check-in" : "Hora apertura"}
            </label>
            <input
              type="time"
              value={openTime}
              disabled={disabled}
              onChange={(e) => setOpenTime(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-xs text-neutral-800 shadow-xs focus:border-[#A83820] focus:ring-1 focus:ring-[#A83820]"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-neutral-600 block mb-1">
              {rateUnit === "dia" ? "Check-out" : "Hora cierre"}
            </label>
            <input
              type="time"
              value={closeTime}
              disabled={disabled}
              onChange={(e) => setCloseTime(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-xs text-neutral-800 shadow-xs focus:border-[#A83820] focus:ring-1 focus:ring-[#A83820]"
            />
          </div>
        </div>

        {/* 1-Click Button */}
        <div>
          <button
            type="button"
            disabled={disabled}
            onClick={handleApplyBaseRule}
            className="inline-flex items-center justify-center rounded-lg bg-[#A83820] px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-[#8f2f1a] transition-colors"
          >
            Aplicar regla base a todo el año
          </button>
          {ruleAppliedFeedback && (
            <p
              role="status"
              className="mt-2 text-xs font-medium text-emerald-700 bg-emerald-50 border border-emerald-200 rounded-md p-2"
            >
              {ruleAppliedFeedback}
            </p>
          )}
        </div>
      </section>

      {/* 2. Calendario Mensual con Multi-Selección */}
      <section
        aria-label="Calendario mensual de excepciones y bloqueos"
        className="space-y-4"
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-bold text-neutral-800 capitalize">
              {monthTitle}
            </h3>
            <div className="flex items-center gap-1">
              <button
                type="button"
                aria-label="Mes anterior"
                disabled={disabled || !shiftMonth(month, -1)}
                onClick={() => {
                  const prev = shiftMonth(month, -1);
                  if (prev) setMonth(prev);
                }}
                className="flex h-7 w-7 items-center justify-center rounded-md border border-neutral-200 text-xs text-neutral-600 hover:bg-neutral-100"
              >
                ‹
              </button>
              <button
                type="button"
                aria-label="Mes siguiente"
                disabled={disabled || !shiftMonth(month, 1)}
                onClick={() => {
                  const next = shiftMonth(month, 1);
                  if (next) setMonth(next);
                }}
                className="flex h-7 w-7 items-center justify-center rounded-md border border-neutral-200 text-xs text-neutral-600 hover:bg-neutral-100"
              >
                ›
              </button>
            </div>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              disabled={disabled}
              onClick={handleSelectAllMonth}
              className="text-[#A83820] font-medium hover:underline"
            >
              Seleccionar todo el mes
            </button>
            {selectedDates.size > 0 && (
              <>
                <span className="text-neutral-300">·</span>
                <button
                  type="button"
                  disabled={disabled}
                  onClick={handleClearSelection}
                  className="text-neutral-500 font-medium hover:underline"
                >
                  Limpiar selección ({selectedDates.size})
                </button>
              </>
            )}
          </div>
        </div>

        {/* Grid and Sidebar Panel */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-5 items-start">
          {/* Calendar Grid */}
          <div className="lg:col-span-8 rounded-xl border border-neutral-200 p-3 bg-white">
            <div className="grid grid-cols-7 gap-1 mb-1">
              {WEEKDAYS.map((w) => (
                <div
                  key={w}
                  className="py-1 text-center text-xs font-semibold text-neutral-600"
                >
                  {w}
                </div>
              ))}
            </div>

            <div
              className="grid grid-cols-7 gap-1"
              onMouseDown={() => setIsDragging(true)}
            >
              {grid.map(({ date, inMonth }) => {
                if (!inMonth) {
                  return (
                    <div
                      key={date}
                      aria-hidden="true"
                      className="h-12 w-full opacity-0"
                    />
                  );
                }

                const dayNum = Number(date.slice(-2));
                const override = dateOverrides.get(date);
                const isSelected = selectedDates.has(date);
                const isBlocked = override?.blocked ?? false;
                const specialRate = override?.specialRateCLP;

                let cellBg =
                  "bg-white border border-neutral-200 hover:border-neutral-400 ";
                if (isBlocked) {
                  cellBg =
                    "bg-neutral-100 border-neutral-300 text-neutral-400 opacity-60 ";
                } else if (specialRate) {
                  cellBg =
                    "bg-amber-50 border-amber-300 text-neutral-900 ";
                }
                if (isSelected) {
                  cellBg =
                    "bg-[#F9EBE8] border-2 border-[#A83820] text-[#A83820] font-bold shadow-xs ";
                }

                return (
                  <button
                    key={date}
                    type="button"
                    disabled={disabled}
                    onClick={() => handleDateClick(date)}
                    onMouseEnter={() => handleMouseEnterDate(date)}
                    className={`relative flex h-12 w-full flex-col items-center justify-center rounded-lg p-0.5 text-xs transition-all ${cellBg}`}
                  >
                    <span className="font-semibold text-xs leading-none">
                      {dayNum}
                    </span>
                    <span className="text-[10px] leading-tight mt-0.5">
                      {isBlocked
                        ? "Bloq."
                        : specialRate
                          ? formatCompactPrice(specialRate)
                          : formatCompactPrice(basePriceCLP ?? null)}
                    </span>
                    {isSelected && (
                      <span
                        aria-hidden="true"
                        className="absolute top-1 right-1 h-1.5 w-1.5 rounded-full bg-[#A83820]"
                      />
                    )}
                  </button>
                );
              })}
            </div>
          </div>

          {/* Lateral / Floating Panel for Selected Dates */}
          <div className="lg:col-span-4 rounded-xl border border-neutral-200 bg-neutral-50/80 p-4 space-y-4">
            <h4 className="text-xs font-bold text-neutral-800 uppercase tracking-wider">
              Acciones sobre fechas seleccionadas
            </h4>

            {selectedDates.size === 0 ? (
              <p className="text-xs text-neutral-500 leading-relaxed">
                Haz clic o arrastra sobre las fechas del mes para seleccionarlas
                y aplicar bloqueos o tarifas especiales.
              </p>
            ) : (
              <div className="space-y-3">
                <p className="text-xs font-semibold text-neutral-700">
                  {selectedDates.size} fecha
                  {selectedDates.size === 1 ? "" : "s"} seleccionada
                  {selectedDates.size === 1 ? "" : "s"}
                </p>

                {/* Block / Unblock Buttons */}
                <div className="flex flex-col gap-2">
                  <button
                    type="button"
                    disabled={disabled}
                    onClick={handleBlockSelectedDates}
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-xs font-semibold text-neutral-800 hover:bg-neutral-100 transition-colors shadow-xs"
                  >
                    Bloquear fechas (mantenimiento/uso)
                  </button>
                  <button
                    type="button"
                    disabled={disabled}
                    onClick={handleUnblockSelectedDates}
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-xs font-semibold text-neutral-800 hover:bg-neutral-100 transition-colors shadow-xs"
                  >
                    Abrir fechas bloqueadas
                  </button>
                </div>

                {/* Special Tariff Input */}
                <div className="border-t border-neutral-200 pt-3 space-y-2">
                  <label className="text-xs font-medium text-neutral-700 block">
                    Asignar tarifa especial por día:
                  </label>
                  <div className="flex gap-2">
                    <input
                      type="number"
                      placeholder="Ej. 55000"
                      value={specialRateInput}
                      disabled={disabled}
                      onChange={(e) => setSpecialRateInput(e.target.value)}
                      className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-xs text-neutral-800 shadow-xs focus:border-[#A83820] focus:ring-1 focus:ring-[#A83820]"
                    />
                    <button
                      type="button"
                      disabled={disabled || !specialRateInput}
                      onClick={handleApplySpecialRate}
                      className="shrink-0 rounded-lg bg-neutral-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-neutral-800 disabled:opacity-40 transition-colors shadow-xs"
                    >
                      Asignar
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Server Sychronization button */}
            <div className="border-t border-neutral-200 pt-3">
              <button
                type="button"
                disabled={disabled || savingServer}
                onClick={handleSaveToBackend}
                className="w-full flex items-center justify-center gap-1.5 rounded-lg bg-neutral-900 px-3 py-2 text-xs font-semibold text-white shadow-xs hover:bg-neutral-800 disabled:opacity-50 transition-colors"
              >
                {savingServer
                  ? "Sincronizando con servidor..."
                  : "Guardar disponibilidad en servidor"}
              </button>
              {serverMessage && (
                <p
                  role="status"
                  className={`mt-2 text-xs font-medium rounded-md p-2 border ${
                    serverMessage.type === "success"
                      ? "text-emerald-800 bg-emerald-50 border-emerald-200"
                      : "text-red-800 bg-red-50 border-red-200"
                  }`}
                >
                  {serverMessage.text}
                </p>
              )}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
