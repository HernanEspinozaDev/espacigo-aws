"use client";

import { useId, useMemo, useState } from "react";
import type { ScheduleSlot } from "../domain/types";
import { validateScheduleSlots } from "../domain/validation";
import {
  applyWeeklyPreset,
  toggleWeeklyHour,
  weeklyHourCoverage,
  type WeeklyPreset,
} from "../domain/weekly-schedule-slots";

export interface WeeklyScheduleSlotsProps {
  slots: readonly ScheduleSlot[];
  onChange?: (slots: ScheduleSlot[]) => void;
  disabled?: boolean;
  readOnly?: boolean;
  invalid?: boolean;
  errorId?: string;
  timeZone: string;
}
const DAYS = [
  { id: 1, label: "Lunes" },
  { id: 2, label: "Martes" },
  { id: 3, label: "Miércoles" },
  { id: 4, label: "Jueves" },
  { id: 5, label: "Viernes" },
  { id: 6, label: "Sábado" },
  { id: 7, label: "Domingo" },
];
const PRESETS: Array<{ id: WeeklyPreset; label: string }> = [
  { id: "morning", label: "Mañana 09:00–13:00" },
  { id: "afternoon", label: "Tarde 14:00–18:00" },
  { id: "full-day", label: "Jornada 09:00–18:00" },
  { id: "24h", label: "24 horas 00:00–24:00" },
];

export function WeeklyScheduleSlots({
  slots,
  onChange,
  disabled = false,
  readOnly = false,
  invalid = false,
  errorId,
  timeZone,
}: WeeklyScheduleSlotsProps) {
  const titleId = useId();
  const [day, setDay] = useState(1);
  const [error, setError] = useState("");
  const locked = disabled || readOnly || !onChange;
  const daySlots = useMemo(
    () => slots.filter((slot) => slot.day_of_week === day),
    [slots, day],
  );
  const closed = daySlots.length === 0;
  const selected = (hour: number) =>
    daySlots.some((slot) => {
      const [sh, sm] = slot.start_time.split(":").map(Number);
      const [eh, em] = slot.end_time.split(":").map(Number);
      return hour * 60 < eh * 60 + em && (hour + 1) * 60 > sh * 60 + sm;
    });
  const commit = (result: { slots: ScheduleSlot[]; error: string | null }) => {
    setError(result.error ?? "");
    if (!result.error) onChange?.(result.slots);
  };
  return (
    <section aria-labelledby={titleId} className="space-y-4">
      <div>
        <h3 id={titleId} className="text-lg font-semibold">
          Horario semanal habitual
        </h3>
        <p className="text-sm text-muted-foreground">
          Selecciona un día, aplica un horario rápido o pulsa los bloques para
          activar/desactivar horas; puedes ajustar cada tramo abajo. Este
          horario es orientativo y no indica disponibilidad para fechas
          específicas. Zona horaria: {timeZone}
        </p>
      </div>
      <div
        role="group"
        aria-label="Día de la semana"
        className="flex flex-wrap gap-2"
      >
        {DAYS.map((item) => {
          const count = slots.filter(
            (slot) => slot.day_of_week === item.id,
          ).length;
          return (
            <button
              key={item.id}
              type="button"
              aria-pressed={day === item.id}
              disabled={disabled}
              onClick={() => setDay(item.id)}
              className="min-h-11 min-w-11 rounded-full border px-3 py-2 aria-pressed:bg-primary aria-pressed:text-primary-foreground"
            >
              <span className="block">{item.label}</span>
              <span className="block text-xs font-normal">
                {count === 0
                  ? "Cerrado"
                  : `${count} ${count === 1 ? "tramo" : "tramos"}`}
              </span>
            </button>
          );
        })}
      </div>
      <div className="flex flex-wrap items-center gap-2 rounded-md border p-3">
        <p className="mr-auto text-sm" aria-live="polite">
          {DAYS.find((item) => item.id === day)?.label}:{" "}
          {closed
            ? "Cerrado"
            : `${daySlots.length} ${daySlots.length === 1 ? "tramo" : "tramos"}`}
        </p>
        {!locked && (
          <>
            <button
              type="button"
              disabled={disabled || closed}
              onClick={() =>
                onChange?.(slots.filter((slot) => slot.day_of_week !== day))
              }
              className="min-h-11 rounded-md border px-3 py-2 text-sm"
            >
              Marcar cerrado
            </button>
            <button
              type="button"
              disabled={disabled || daySlots.length >= 2}
              onClick={() => {
                const candidates =
                  daySlots.length === 0
                    ? [["09:00", "18:00"]]
                    : [
                        ["09:00", "12:00"],
                        ["14:00", "18:00"],
                        ["19:00", "22:00"],
                        ["00:00", "08:00"],
                      ];
                for (const [start_time, end_time] of candidates) {
                  const next = [
                    ...slots,
                    { day_of_week: day, start_time, end_time },
                  ].sort(
                    (a, b) =>
                      a.day_of_week - b.day_of_week ||
                      a.start_time.localeCompare(b.start_time),
                  );
                  const validation = validateScheduleSlots(next);
                  if (validation.valid) {
                    commit({ slots: next, error: null });
                    return;
                  }
                }
                setError(
                  "No hay un tramo sugerido que pueda agregarse sin superponerse.",
                );
              }}
              className="min-h-11 rounded-md border px-3 py-2 text-sm"
            >
              Agregar tramo
            </button>
          </>
        )}
      </div>
      {!locked && (
        <div aria-label="Presets de horario" className="flex flex-wrap gap-2">
          {PRESETS.map((preset) => (
            <button
              key={preset.id}
              type="button"
              disabled={disabled}
              onClick={() => commit(applyWeeklyPreset(slots, day, preset.id))}
              className="min-h-11 min-w-11 rounded-md border px-3 py-2"
            >
              {preset.label}
            </button>
          ))}
        </div>
      )}
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
        {Array.from({ length: 24 }, (_, hour) => (
          <button
            key={hour}
            type="button"
            aria-pressed={
              weeklyHourCoverage(daySlots, day, hour) === "partial"
                ? "mixed"
                : weeklyHourCoverage(daySlots, day, hour) === "full"
            }
            disabled={locked}
            onClick={() =>
              commit(toggleWeeklyHour(slots, day, hour, !selected(hour)))
            }
            className="min-h-11 min-w-11 rounded-md border px-2 py-2 text-sm aria-pressed:bg-primary aria-pressed:text-primary-foreground"
          >
            {String(hour).padStart(2, "0")}:00–
            {String(hour + 1).padStart(2, "0")}:00
            {weeklyHourCoverage(daySlots, day, hour) === "partial" && (
              <>
                {" "}
                <span className="mt-1 block rounded bg-primary/10 px-1 text-xs font-medium text-primary">
                  Parcial
                </span>
                <span className="sr-only">
                  {" "}
                  Tramos:{" "}
                  {daySlots
                    .filter(
                      (slot) =>
                        weeklyHourCoverage([slot], day, hour) !== "none",
                    )
                    .map((slot) => `${slot.start_time}–${slot.end_time}`)
                    .join(", ")}
                </span>
              </>
            )}
          </button>
        ))}
      </div>
      {daySlots.map((slot, index) => (
        <div key={index} className="grid grid-cols-2 gap-3">
          <label className="text-sm">
            Desde
            <input
              aria-label={`Tramo ${index + 1}: hora de inicio`}
              aria-invalid={invalid || undefined}
              aria-describedby={invalid ? errorId : undefined}
              type="time"
              step={60}
              value={slot.start_time}
              disabled={locked}
              onChange={(event) => {
                if (!onChange) return;
                const next = [...slots];
                next[slots.indexOf(slot)] = {
                  ...slot,
                  start_time: event.target.value,
                };
                onChange(next);
              }}
              className="mt-1 block w-full rounded-md border p-2"
            />
          </label>
          <label className="text-sm">
            Hasta (incluye 24:00)
            <input
              aria-label={`Tramo ${index + 1}: hora de cierre (24:00 permitido)`}
              aria-invalid={invalid || undefined}
              aria-describedby={invalid ? errorId : undefined}
              type="text"
              inputMode="numeric"
              pattern="(?:[01][0-9]|2[0-3]):[0-5][0-9]|24:00"
              value={slot.end_time}
              disabled={locked}
              onChange={(event) => {
                if (!onChange) return;
                const next = [...slots];
                next[slots.indexOf(slot)] = {
                  ...slot,
                  end_time: event.target.value,
                };
                onChange(next);
              }}
              className="mt-1 block w-full rounded-md border p-2"
            />
          </label>
        </div>
      ))}
      <div
        className="rounded-md bg-muted/30 p-3"
        aria-label="Resumen de horario semanal"
      >
        <h4 className="text-sm font-semibold">Resumen de horario semanal</h4>
        <ul className="mt-2 grid grid-cols-1 gap-1 text-sm sm:grid-cols-2">
          {DAYS.map((item) => {
            const ranges = slots.filter((slot) => slot.day_of_week === item.id);
            return (
              <li key={item.id}>
                <span className="font-medium">{item.label}:</span>{" "}
                {ranges.length
                  ? ranges
                      .map((slot) => `${slot.start_time}–${slot.end_time}`)
                      .join(", ")
                  : "Cerrado"}
              </li>
            );
          })}
        </ul>
      </div>
      <div
        role="alert"
        aria-live="polite"
        className="min-h-5 text-sm text-red-800"
      >
        {error}
      </div>
    </section>
  );
}
