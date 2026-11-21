"use client";

import { useState } from "react";
import { Calendar, CheckCircle2, Clock, Play } from "lucide-react";
import type {
  BookingInterval,
  BookingReservation,
  IntervalUsageStatus,
} from "../domain/types";
import {
  groupReservationIntervalsByDay,
  usageStatusLabel,
} from "../domain/reservation-intervals";
import { formatBookingClp } from "./format";

function usageBadgeStyle(status: IntervalUsageStatus): string {
  switch (status) {
    case "en_uso":
      return "border-blue-300 bg-blue-50 text-blue-900 dark:border-blue-500/40 dark:bg-blue-500/15 dark:text-blue-300";
    case "completado":
      return "border-neutral-300 bg-neutral-200/80 text-neutral-700 dark:border-neutral-600 dark:bg-neutral-800 dark:text-neutral-300";
    case "no_asistio":
      return "border-red-300 bg-red-50 text-red-900 dark:border-red-500/40 dark:bg-red-500/15 dark:text-red-300";
    default:
      return "border-emerald-300/80 bg-emerald-50 text-emerald-900 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300";
  }
}

export function ReservationScheduleBreakdown({
  reservation,
  compact = false,
  allowHostControls = false,
}: {
  reservation: BookingReservation;
  compact?: boolean;
  allowHostControls?: boolean;
}) {
  const [localStatuses, setLocalStatuses] = useState<
    Record<string, IntervalUsageStatus>
  >({});
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);

  const effectiveReservation: BookingReservation =
    Object.keys(localStatuses).length === 0 || !reservation.intervals
      ? reservation
      : {
          ...reservation,
          intervals: reservation.intervals.map((iv) =>
            iv.id && localStatuses[iv.id]
              ? { ...iv, usage_status: localStatuses[iv.id] }
              : iv,
          ),
        };

  const days = groupReservationIntervalsByDay(effectiveReservation);
  const unitSingular =
    reservation.rate_unit === "hora"
      ? "hora"
      : reservation.rate_unit === "dia"
        ? "día"
        : "mes";
  const unitPlural =
    reservation.rate_unit === "mes" ? "meses" : `${unitSingular}s`;

  const canManageUsage =
    allowHostControls &&
    reservation.participant_role === "anfitrion" &&
    reservation.state === "pagada";

  const usedUnits = (effectiveReservation.intervals ?? []).reduce(
    (sum, iv) => (iv.usage_status === "completado" ? sum + iv.units : sum),
    0,
  );

  const handleUsageUpdate = async (
    intervalId: string,
    nextStatus: IntervalUsageStatus,
  ) => {
    setUpdatingId(intervalId);
    setUpdateError(null);
    try {
      const res = await fetch(
        `/api/backend/commercial-reservations/${encodeURIComponent(reservation.id)}/intervals/${encodeURIComponent(intervalId)}/usage`,
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/json",
          },
          body: JSON.stringify({ status: nextStatus }),
        },
      );
      if (!res.ok) {
        setUpdateError("No se pudo actualizar el estado del tramo.");
        return;
      }
      const body = (await res.json().catch(() => null)) as {
        data?: { intervals?: BookingInterval[] };
      } | null;
      const updatedIv = body?.data?.intervals?.find((i) => i.id === intervalId);
      setLocalStatuses((prev) => ({
        ...prev,
        [intervalId]: updatedIv?.usage_status ?? nextStatus,
      }));
    } catch {
      setUpdateError("Error de red al actualizar el estado del tramo.");
    } finally {
      setUpdatingId(null);
    }
  };

  if (
    (!reservation.intervals || reservation.intervals.length <= 1) &&
    compact &&
    !canManageUsage
  ) {
    return (
      <div className="flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <Calendar className="h-3.5 w-3.5" />
          Inicio:{" "}
          {new Date(reservation.start_at).toLocaleDateString("es-CL", {
            day: "2-digit",
            month: "short",
            year: "numeric",
            hour: "2-digit",
            minute: "2-digit",
          })}
        </span>
        <span className="flex items-center gap-1">
          <Clock className="h-3.5 w-3.5" />
          Término:{" "}
          {new Date(reservation.end_at).toLocaleDateString("es-CL", {
            day: "2-digit",
            month: "short",
            year: "numeric",
            hour: "2-digit",
            minute: "2-digit",
          })}
        </span>
      </div>
    );
  }

  return (
    <div className="space-y-2 rounded-xl border border-border/80 bg-muted/25 p-3 text-xs">
      <div className="flex flex-wrap items-center justify-between gap-2 font-semibold uppercase tracking-wider text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <Calendar className="h-3.5 w-3.5 text-primary" />
          Horarios Reservados ({days.length}{" "}
          {days.length === 1 ? "día" : "días"} · {reservation.units}{" "}
          {reservation.units === 1 ? unitSingular : unitPlural})
        </span>
        {reservation.state === "pagada" &&
          (effectiveReservation.intervals?.length ?? 0) > 0 && (
            <span className="rounded-full bg-background px-2 py-0.5 text-[10px] font-semibold normal-case text-foreground border border-border">
              {usedUnits} de {reservation.units}{" "}
              {reservation.units === 1 ? unitSingular : unitPlural} usadas
            </span>
          )}
      </div>

      {updateError && (
        <p className="text-[11px] font-medium text-destructive">{updateError}</p>
      )}

      <div className="space-y-1.5">
        {days.map((day) => (
          <div
            key={day.dateKey}
            className="flex flex-col gap-1.5 rounded-lg border border-border/60 bg-background px-2.5 py-2"
          >
            <div className="flex items-center justify-between gap-2">
              <span className="font-semibold capitalize text-foreground">
                {day.dateLabel}
              </span>
              <span className="font-semibold tabular-nums text-muted-foreground">
                {formatBookingClp(day.subtotalClp)} CLP
              </span>
            </div>

            <div className="flex flex-wrap items-center gap-1.5">
              {day.slots.map((slot) => {
                const isBusy = updatingId === slot.id;
                return (
                  <div
                    key={`${slot.startAt}-${slot.endAt}`}
                    className={`inline-flex flex-wrap items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] font-medium ${usageBadgeStyle(slot.usageStatus)}`}
                  >
                    <span className="font-mono font-semibold tabular-nums">
                      {slot.timeLabel} ({slot.units}
                      {reservation.rate_unit === "hora" ? "h" : ""})
                    </span>
                    {reservation.state === "pagada" && (
                      <span className="rounded bg-white/70 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide dark:bg-black/25">
                        {usageStatusLabel(slot.usageStatus)}
                      </span>
                    )}
                    {canManageUsage && slot.id && (
                      <span className="inline-flex items-center gap-1">
                        {slot.usageStatus === "pendiente" && (
                          <button
                            type="button"
                            disabled={isBusy}
                            onClick={() =>
                              void handleUsageUpdate(slot.id!, "en_uso")
                            }
                            className="inline-flex items-center gap-0.5 rounded bg-blue-600 px-1.5 py-0.5 text-[10px] font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
                          >
                            <Play className="h-2.5 w-2.5" />
                            Iniciar
                          </button>
                        )}
                        {(slot.usageStatus === "pendiente" ||
                          slot.usageStatus === "en_uso") && (
                          <button
                            type="button"
                            disabled={isBusy}
                            onClick={() =>
                              void handleUsageUpdate(slot.id!, "completado")
                            }
                            className="inline-flex items-center gap-0.5 rounded bg-emerald-600 px-1.5 py-0.5 text-[10px] font-semibold text-white hover:bg-emerald-700 disabled:opacity-50"
                          >
                            <CheckCircle2 className="h-2.5 w-2.5" />
                            Usada
                          </button>
                        )}
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
