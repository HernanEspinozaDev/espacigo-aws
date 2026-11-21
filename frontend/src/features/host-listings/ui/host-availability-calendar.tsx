"use client";

import { useId, useMemo, useState } from "react";
import type {
  CalendarConfiguration,
  CalendarInventoryUpdate,
} from "../domain/calendar-configuration";
import type { CalendarConfigurationController } from "../application/calendar-configuration-controller";
import { useHostCalendarConfiguration } from "./use-host-calendar-configuration";
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
  state?: "disponible" | "bloqueado" | "mantenimiento";
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

export function HostAvailabilityCalendar(props: HostAvailabilityCalendarProps) {
  const { controller, state } = useHostCalendarConfiguration(props.spaceId);
  if (props.spaceId && state.phase !== "ready") {
    return (
      <div
        role="status"
        className="break-words rounded-2xl border border-neutral-200 bg-white p-5 text-sm text-neutral-700"
      >
        {state.phase === "needs_refresh"
          ? state.message
          : state.phase === "error"
            ? state.message
            : "Cargando configuración del calendario…"}
        {(state.phase === "needs_refresh" || state.phase === "error") &&
          controller && (
            <button
              type="button"
              className="ml-2 underline"
              onClick={() => void controller.refresh()}
            >
              Actualizar para verificar
            </button>
          )}
      </div>
    );
  }
  return (
    <>
      {state.message && (
        <p role="status" className="break-words text-sm">
          {state.message}
        </p>
      )}
      <HostAvailabilityCalendarEditor
        key={state.read?.etag ?? "draft"}
        {...props}
        controller={controller}
        configuration={state.read?.configuration ?? null}
        phase={state.phase}
      />
    </>
  );
}

function HostAvailabilityCalendarEditor({
  spaceId,
  rateUnit = "hora",
  basePriceCLP,
  timeZone = "America/Santiago",
  onScheduleChange,
  initialSlots = [],
  disabled = false,
  controller,
  configuration,
  phase,
}: HostAvailabilityCalendarProps & {
  controller: CalendarConfigurationController | null;
  configuration: CalendarConfiguration | null;
  phase: string;
}) {
  const headingId = useId();

  // 1. Quick Base Schedule Configurator State
  const [activeDays, setActiveDays] = useState<Set<number>>(
    () =>
      new Set(
        configuration?.rule.dias_operacion ??
          (initialSlots.length
            ? initialSlots.map((slot) =>
                slot.day_of_week === 0 ? 7 : slot.day_of_week,
              )
            : [1, 2, 3, 4, 5]),
      ),
  );
  const [openTime, setOpenTime] = useState<string>(() =>
    configuration
      ? rateUnit === "dia"
        ? configuration.rule.check_in_hora
        : configuration.rule.hora_apertura
      : (initialSlots[0]?.start_time ??
        (rateUnit === "dia" ? "15:00" : "09:00")),
  );
  const [closeTime, setCloseTime] = useState<string>(() =>
    configuration
      ? rateUnit === "dia"
        ? configuration.rule.check_out_hora
        : configuration.rule.hora_cierre
      : (initialSlots[0]?.end_time ?? (rateUnit === "dia" ? "12:00" : "20:00")),
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
  >(
    () =>
      new Map(
        (configuration?.overrides ?? []).map((override) => [
          override.fecha,
          {
            blocked: override.estado !== "disponible",
            state: override.estado,
            ...(override.tarifa_personalizada_clp !== undefined
              ? { specialRateCLP: override.tarifa_personalizada_clp }
              : {}),
          },
        ]),
      ),
  );
  const [specialRateInput, setSpecialRateInput] = useState<string>("");
  const [baseRate, setBaseRate] = useState(
    String(configuration?.rule.tarifa_base_dia ?? basePriceCLP ?? ""),
  );
  const [minimumStay, setMinimumStay] = useState(
    String(configuration?.rule.estadia_minima ?? 1),
  );
  const [maximumStay, setMaximumStay] = useState(
    String(configuration?.rule.estadia_maxima ?? 30),
  );
  const [validationMessage, setValidationMessage] = useState<string | null>(
    null,
  );

  // 3. API Sync State

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

    if (rateUnit === "hora") onScheduleChange?.(slots);
    setRuleAppliedFeedback(
      `Regla preparada para ${sortedDays.length} días (${openTime} a ${closeTime}). Guarda para persistirla.`,
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
        next.set(d, { ...existing, blocked: true, state: "mantenimiento" });
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
        next.set(d, { ...existing, blocked: false, state: "disponible" });
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
    if (!spaceId || !configuration || !controller) return;
    const price = Number(baseRate),
      minStay = Number(minimumStay),
      maxStay = Number(maximumStay);
    if (
      !Number.isSafeInteger(price) ||
      price < 1 ||
      price > 2147483647 ||
      !Number.isInteger(minStay) ||
      !Number.isInteger(maxStay) ||
      minStay < 1 ||
      maxStay < minStay ||
      maxStay > 32767 ||
      !openTime ||
      !closeTime ||
      (rateUnit !== "dia" && openTime >= closeTime)
    ) {
      setValidationMessage(
        "Revisa el precio, los horarios y los límites de estadía antes de guardar.",
      );
      return;
    }
    setValidationMessage(null);
    const overrides = new Map(
      configuration.overrides.map((item) => [item.fecha, item]),
    );
    for (const [date, draft] of dateOverrides) {
      const previous = overrides.get(date);
      overrides.set(date, {
        fecha: date,
        estado: draft.state ?? (draft.blocked ? "mantenimiento" : "disponible"),
        ...(draft.specialRateCLP
          ? { tarifa_personalizada_clp: draft.specialRateCLP }
          : previous?.tarifa_personalizada_clp
            ? { tarifa_personalizada_clp: previous.tarifa_personalizada_clp }
            : {}),
        ...(previous?.motivo ? { motivo: previous.motivo } : {}),
      });
    }
    const rule = configuration.rule;
    const payload: CalendarInventoryUpdate = {
      time_zone: configuration.time_zone,
      dias_operacion: [...activeDays].sort((a, b) => a - b),
      hora_apertura: rateUnit === "dia" ? rule.hora_apertura : openTime,
      hora_cierre: rateUnit === "dia" ? rule.hora_cierre : closeTime,
      check_in_hora: rateUnit === "dia" ? openTime : rule.check_in_hora,
      check_out_hora: rateUnit === "dia" ? closeTime : rule.check_out_hora,
      tarifa_base_dia: price,
      estadia_minima: minStay,
      estadia_maxima: maxStay,
      overrides: [...overrides.values()],
    };
    await controller.save(payload);
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
            Zona: {configuration?.time_zone ?? timeZone}
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
            <label
              htmlFor={`${headingId}-open`}
              className="text-xs font-medium text-neutral-600 block mb-1"
            >
              {rateUnit === "dia" ? "Check-in" : "Hora apertura"}
            </label>
            <input
              type="time"
              id={`${headingId}-open`}
              value={openTime}
              disabled={disabled}
              onChange={(e) => setOpenTime(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-xs text-neutral-800 shadow-xs focus:border-[#A83820] focus:ring-1 focus:ring-[#A83820]"
            />
          </div>
          <div>
            <label
              htmlFor={`${headingId}-close`}
              className="text-xs font-medium text-neutral-600 block mb-1"
            >
              {rateUnit === "dia" ? "Check-out" : "Hora cierre"}
            </label>
            <input
              type="time"
              id={`${headingId}-close`}
              value={closeTime}
              disabled={disabled}
              onChange={(e) => setCloseTime(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-2.5 py-1.5 text-xs text-neutral-800 shadow-xs focus:border-[#A83820] focus:ring-1 focus:ring-[#A83820]"
            />
          </div>
        </div>

        <div className="grid min-w-0 gap-3 sm:grid-cols-3">
          <label
            className="min-w-0 break-words text-xs"
            htmlFor={`${headingId}-base-rate`}
          >
            Tarifa base del calendario (CLP)
            <input
              id={`${headingId}-base-rate`}
              type="number"
              min={1}
              max={2147483647}
              step={1}
              value={baseRate}
              onChange={(event) => setBaseRate(event.target.value)}
              disabled={disabled}
              className="mt-1 w-full min-w-0 rounded-lg border p-2"
            />
          </label>
          <label
            className="min-w-0 break-words text-xs"
            htmlFor={`${headingId}-min-stay`}
          >
            Estadía mínima
            <input
              id={`${headingId}-min-stay`}
              type="number"
              min={1}
              max={32767}
              step={1}
              value={minimumStay}
              onChange={(event) => setMinimumStay(event.target.value)}
              disabled={disabled}
              className="mt-1 w-full min-w-0 rounded-lg border p-2"
            />
          </label>
          <label
            className="min-w-0 break-words text-xs"
            htmlFor={`${headingId}-max-stay`}
          >
            Estadía máxima
            <input
              id={`${headingId}-max-stay`}
              type="number"
              min={1}
              max={32767}
              step={1}
              value={maximumStay}
              onChange={(event) => setMaximumStay(event.target.value)}
              disabled={disabled}
              className="mt-1 w-full min-w-0 rounded-lg border p-2"
            />
          </label>
        </div>
        <p className="break-words text-xs text-neutral-600">
          Los precios del calendario no constituyen una cotización ni modifican
          por sí solos la tarifa comercial.
        </p>
        {validationMessage && (
          <p role="alert" className="break-words text-sm text-red-800">
            {validationMessage}
          </p>
        )}
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
                  cellBg = "bg-amber-50 border-amber-300 text-neutral-900 ";
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
                    aria-label={`${localDateLabel(date)}${isBlocked ? ", bloqueada" : ""}`}
                    aria-pressed={isSelected}
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
                          : formatCompactPrice(
                              configuration?.rule.tarifa_base_dia ??
                                basePriceCLP ??
                                null,
                            )}
                    </span>
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
                  <label
                    htmlFor={`${headingId}-special`}
                    className="text-xs font-medium text-neutral-700 block"
                  >
                    Asignar tarifa especial por día:
                  </label>
                  <div className="flex gap-2">
                    <input
                      type="number"
                      id={`${headingId}-special`}
                      min={1}
                      step={1}
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
                disabled={disabled || !spaceId || phase !== "ready"}
                onClick={handleSaveToBackend}
                className="w-full flex items-center justify-center gap-1.5 rounded-lg bg-neutral-900 px-3 py-2 text-xs font-semibold text-white shadow-xs hover:bg-neutral-800 disabled:opacity-50 transition-colors"
              >
                {!spaceId
                  ? "Crear el espacio primero para guardar"
                  : phase === "saving"
                    ? "Guardando y verificando..."
                    : "Guardar disponibilidad en servidor"}
              </button>
              {!spaceId ? (
                <p
                  role="status"
                  className="mt-2 break-words text-xs text-amber-800"
                >
                  El calendario del asistente no se persiste aquí; crea primero
                  el espacio.
                </p>
              ) : phase === "needs_refresh" ? (
                <p
                  role="alert"
                  className="mt-2 break-words text-xs text-red-800"
                >
                  No se confirmó el resultado. Actualiza manualmente antes de
                  volver a guardar.
                </p>
              ) : null}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
