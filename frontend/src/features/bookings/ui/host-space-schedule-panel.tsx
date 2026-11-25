"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import type { BookingReservation } from "../domain/types";
import { getReservationFinancialBreakdown } from "../domain/financial-breakdown";
import { formatBookingClp } from "./format";
import { Button } from "@/components/workspace/ui/button";
import {
  Building2,
  Calendar,
  CheckCircle2,
  Clock,
  Clock3,
  DollarSign,
  ExternalLink,
  Loader2,
  Receipt,
  RefreshCw,
  XCircle,
} from "lucide-react";

interface FreeInterval {
  start_at: string;
  end_at: string;
  status?: "available" | "held" | "booked";
}

interface HostSpaceSchedulePanelProps {
  reservations: readonly BookingReservation[];
  selectedSpaceId: string;
  onSelectSpaceId: (spaceId: string) => void;
  detailBaseHref: string;
}

function todayISOInChile(): string {
  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: "America/Santiago",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(new Date());
    const y = parts.find((p) => p.type === "year")?.value;
    const m = parts.find((p) => p.type === "month")?.value;
    const d = parts.find((p) => p.type === "day")?.value;
    if (y && m && d) return `${y}-${m}-${d}`;
  } catch {
    // Fallback to UTC ISO slice
  }
  return new Date().toISOString().slice(0, 10);
}

function dateISOInZone(
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
    // Fallback
  }
  return isoTimestamp.slice(0, 10);
}

function formatTimeRangeInZone(
  startIso: string,
  endIso: string,
  timeZone = "America/Santiago",
): string {
  try {
    const fmt = new Intl.DateTimeFormat("es-CL", {
      timeZone,
      hour: "2-digit",
      minute: "2-digit",
    });
    return `${fmt.format(new Date(startIso))} – ${fmt.format(new Date(endIso))}`;
  } catch {
    return `${startIso.slice(11, 16)} – ${endIso.slice(11, 16)}`;
  }
}

export function HostSpaceSchedulePanel({
  reservations,
  selectedSpaceId,
  onSelectSpaceId,
  detailBaseHref,
}: HostSpaceSchedulePanelProps) {
  const hostReservations = useMemo(
    () =>
      reservations.filter(
        (r) => !r.participant_role || r.participant_role === "anfitrion",
      ),
    [reservations],
  );

  const spacesSummary = useMemo(() => {
    const map = new Map<
      string,
      {
        spaceId: string;
        spaceTitle: string;
        timeZone: string;
        rateUnit: string;
        confirmedCount: number;
        pendingCount: number;
        cancelledCount: number;
        confirmedGrossClp: number;
        confirmedNetClp: number;
        confirmedFeeClp: number;
      }
    >();

    for (const r of hostReservations) {
      const existing = map.get(r.space_id) ?? {
        spaceId: r.space_id,
        spaceTitle: r.space_title || `Espacio ${r.space_id.slice(0, 8)}`,
        timeZone: r.time_zone || "America/Santiago",
        rateUnit: r.rate_unit || "hora",
        confirmedCount: 0,
        pendingCount: 0,
        cancelledCount: 0,
        confirmedGrossClp: 0,
        confirmedNetClp: 0,
        confirmedFeeClp: 0,
      };
      if (r.space_title && existing.spaceTitle.startsWith("Espacio ")) {
        existing.spaceTitle = r.space_title;
      }
      if (
        r.state === "pagada" ||
        r.state === "aprobada_host" ||
        r.state === "lista_para_checkin" ||
        r.state === "en_curso" ||
        r.state === "finalizada" ||
        r.state === "cerrada"
      ) {
        existing.confirmedCount += 1;
        const breakdown = getReservationFinancialBreakdown(r);
        existing.confirmedGrossClp += breakdown.baseClp;
        existing.confirmedFeeClp += breakdown.hostFeeClp;
        existing.confirmedNetClp += r.is_commercial
          ? (breakdown.confirmedNetHostClp ?? 0)
          : breakdown.netHostClp;
      } else if (r.state === "pendiente_de_pago") {
        existing.pendingCount += 1;
      } else {
        existing.cancelledCount += 1;
      }
      map.set(r.space_id, existing);
    }

    return Array.from(map.values());
  }, [hostReservations]);

  const activeSpaceId =
    selectedSpaceId !== "all"
      ? selectedSpaceId
      : (spacesSummary[0]?.spaceId ?? "");

  const activeSpace = useMemo(
    () => spacesSummary.find((s) => s.spaceId === activeSpaceId) ?? null,
    [spacesSummary, activeSpaceId],
  );

  // Pick a default date: if the active space has a confirmed reservation in the future, default to the nearest confirmed date so the host immediately sees it!
  const defaultAgendaDate = useMemo(() => {
    const today = todayISOInChile();
    const spaceConfirmedDates: string[] = [];
    for (const r of hostReservations) {
      if (
        (activeSpaceId && r.space_id !== activeSpaceId) ||
        (r.state !== "pagada" && r.state !== "pendiente_de_pago")
      ) {
        continue;
      }
      const tz = r.time_zone || "America/Santiago";
      const intervals =
        r.intervals && r.intervals.length > 0
          ? r.intervals
          : [{ start_at: r.start_at, end_at: r.end_at, units: r.units }];
      for (const iv of intervals) {
        const d = dateISOInZone(iv.start_at, tz);
        if (d >= today) spaceConfirmedDates.push(d);
      }
    }
    spaceConfirmedDates.sort();
    return spaceConfirmedDates[0] ?? today;
  }, [hostReservations, activeSpaceId]);

  const [selectedDate, setSelectedDate] = useState<string>("");
  const effectiveDate = selectedDate || defaultAgendaDate;
  const [durationHours, setDurationHours] = useState<number>(2);
  const [freeSlotsState, setFreeSlotsState] = useState<{
    key: string;
    loading: boolean;
    items: FreeInterval[];
    error: string | null;
  }>({
    key: "",
    loading: false,
    items: [],
    error: null,
  });
  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    if (!activeSpaceId || !effectiveDate) return;
    const controller = new AbortController();
    const requestKey = `${activeSpaceId}:${effectiveDate}:${durationHours}:${refreshTick}`;
    const durParam = activeSpace?.rateUnit === "hora" ? durationHours : 1;

    void Promise.resolve()
      .then(() => {
        if (controller.signal.aborted) return null;
        setFreeSlotsState((prev) => ({
          key: requestKey,
          loading: true,
          items: prev.key === requestKey ? prev.items : [],
          error: null,
        }));
        return fetch(
          `/api/backend/spaces/${encodeURIComponent(activeSpaceId)}/rental-schedule?date=${encodeURIComponent(effectiveDate)}&duration=${durParam}`,
          {
            method: "GET",
            credentials: "same-origin",
            cache: "no-store",
            headers: { Accept: "application/json" },
            signal: controller.signal,
          },
        );
      })
      .then(async (res) => {
        if (!res || controller.signal.aborted) return;
        const body = (await res.json().catch(() => null)) as {
          data?: { slots?: FreeInterval[]; items?: FreeInterval[] };
        } | null;
        if (controller.signal.aborted) return;
        if (!res.ok) {
          setFreeSlotsState({
            key: requestKey,
            loading: false,
            items: [],
            error:
              "No fue posible consultar los horarios libres para esta fecha.",
          });
          return;
        }
        const slots = Array.isArray(body?.data?.slots)
          ? body!.data!.slots!
          : Array.isArray(body?.data?.items)
            ? body!.data!.items!
            : [];
        setFreeSlotsState({
          key: requestKey,
          loading: false,
          items: slots,
          error: null,
        });
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setFreeSlotsState({
            key: requestKey,
            loading: false,
            items: [],
            error: "Error de red al consultar disponibilidad.",
          });
        }
      });

    return () => controller.abort();
  }, [
    activeSpaceId,
    activeSpace?.rateUnit,
    effectiveDate,
    durationHours,
    refreshTick,
  ]);

  const reservationsOnDate = useMemo(() => {
    if (!activeSpaceId || !effectiveDate) return [];
    return hostReservations
      .filter((r) => {
        if (r.space_id !== activeSpaceId) return false;
        const tz = r.time_zone || "America/Santiago";
        const intervals =
          r.intervals && r.intervals.length > 0
            ? r.intervals
            : [{ start_at: r.start_at, end_at: r.end_at, units: r.units }];
        return intervals.some(
          (iv) => dateISOInZone(iv.start_at, tz) === effectiveDate,
        );
      })
      .sort((a, b) => Date.parse(a.start_at) - Date.parse(b.start_at));
  }, [hostReservations, activeSpaceId, effectiveDate]);

  const confirmedOnDate = reservationsOnDate.filter(
    (r) =>
      r.state === "pagada" ||
      r.state === "aprobada_host" ||
      r.state === "lista_para_checkin" ||
      r.state === "en_curso" ||
      r.state === "finalizada" ||
      r.state === "cerrada",
  );
  const pendingOnDate = reservationsOnDate.filter(
    (r) => r.state === "pendiente_de_pago",
  );
  const cancelledOnDate = reservationsOnDate.filter(
    (r) =>
      r.state === "vencida_pago" ||
      r.state === "cancelada_por_pago" ||
      r.state === "cancelada_arrendatario" ||
      r.state === "rechazada_arrendador",
  );

  // Dates that have confirmed or pending reservations for quick navigation pills
  const datesWithActivity = useMemo(() => {
    const byDate = new Map<
      string,
      { confirmed: number; pending: number; cancelled: number }
    >();
    for (const r of hostReservations) {
      if (activeSpaceId && r.space_id !== activeSpaceId) continue;
      const tz = r.time_zone || "America/Santiago";
      const intervals =
        r.intervals && r.intervals.length > 0
          ? r.intervals
          : [{ start_at: r.start_at, end_at: r.end_at, units: r.units }];
      const seenDates = new Set<string>();
      for (const iv of intervals) {
        const d = dateISOInZone(iv.start_at, tz);
        if (seenDates.has(d)) continue;
        seenDates.add(d);
        const entry = byDate.get(d) ?? {
          confirmed: 0,
          pending: 0,
          cancelled: 0,
        };
        if (r.state === "pagada") entry.confirmed += 1;
        else if (r.state === "pendiente_de_pago") entry.pending += 1;
        else entry.cancelled += 1;
        byDate.set(d, entry);
      }
    }
    return Array.from(byDate.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [hostReservations, activeSpaceId]);

  if (spacesSummary.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Resumen de espacios, pagos y agenda horaria del anfitrión"
      className="space-y-6 rounded-2xl border border-border bg-card p-5 shadow-sm sm:p-6"
    >
      {/* 1. Tarjetas de Espacios con Pagos y Contadores */}
      <div className="space-y-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-base font-semibold text-foreground flex items-center gap-2">
              <Building2 className="h-4 w-4 text-primary" />
              Espacios Publicados y Resumen de Pagos Confirmados
            </h2>
            <p className="text-xs text-muted-foreground">
              Selecciona un espacio para filtrar sus pagos y revisar su agenda
              de horarios confirmados, pendientes, cancelados y libres.
            </p>
          </div>
          {selectedSpaceId !== "all" && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => onSelectSpaceId("all")}
              className="text-xs"
            >
              Ver todos los espacios ({spacesSummary.length})
            </Button>
          )}
        </div>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {spacesSummary.map((space) => {
            const isSelected =
              selectedSpaceId === space.spaceId ||
              (selectedSpaceId === "all" &&
                spacesSummary.length === 1 &&
                space.spaceId === activeSpaceId);
            return (
              <div
                key={space.spaceId}
                className={`rounded-xl border p-4 transition-all ${
                  isSelected
                    ? "border-primary bg-primary/5 shadow-xs"
                    : "border-border bg-background hover:border-primary/40"
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <button
                      type="button"
                      onClick={() =>
                        onSelectSpaceId(
                          selectedSpaceId === space.spaceId
                            ? "all"
                            : space.spaceId,
                        )
                      }
                      className="text-left font-semibold text-foreground hover:text-primary transition-colors line-clamp-1"
                    >
                      {space.spaceTitle}
                    </button>
                    <p className="mt-0.5 font-mono text-[11px] text-muted-foreground break-all">
                      ID Espacio: {space.spaceId}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Button
                      asChild
                      variant="outline"
                      size="sm"
                      className="h-7 px-2 text-xs"
                    >
                      <Link
                        href={`/espacios/${encodeURIComponent(space.spaceId)}`}
                      >
                        <ExternalLink className="mr-1 h-3 w-3" />
                        Público
                      </Link>
                    </Button>
                    <Button
                      asChild
                      variant="outline"
                      size="sm"
                      className="h-7 px-2 text-xs"
                    >
                      <Link
                        href={`/anfitrion/espacios/${encodeURIComponent(space.spaceId)}/editar`}
                      >
                        Editar
                      </Link>
                    </Button>
                  </div>
                </div>

                {/* Contadores de horarios por estado */}
                <div className="mt-3 grid grid-cols-3 gap-2 text-center text-xs">
                  <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-2 py-1.5 text-emerald-800 dark:text-emerald-300">
                    <span className="block text-base font-bold">
                      {space.confirmedCount}
                    </span>
                    <span className="text-[11px]">Confirmados</span>
                  </div>
                  <div className="rounded-lg border border-amber-500/20 bg-amber-500/10 px-2 py-1.5 text-amber-800 dark:text-amber-300">
                    <span className="block text-base font-bold">
                      {space.pendingCount}
                    </span>
                    <span className="text-[11px]">Pendientes</span>
                  </div>
                  <div className="rounded-lg border border-border bg-muted/50 px-2 py-1.5 text-muted-foreground">
                    <span className="block text-base font-bold">
                      {space.cancelledCount}
                    </span>
                    <span className="text-[11px]">Vencidos/Canc.</span>
                  </div>
                </div>

                {/* Resumen financiero de Mercado Pago por espacio */}
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-2.5 text-xs">
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <DollarSign className="h-3.5 w-3.5 text-emerald-600" />
                    <span>Pagos acreditados:</span>
                    <strong className="text-foreground">
                      {formatBookingClp(space.confirmedGrossClp)} CLP
                    </strong>
                  </div>
                  <div className="text-emerald-700 dark:text-emerald-400 font-semibold">
                    Neto informado: {formatBookingClp(space.confirmedNetClp)}{" "}
                    CLP
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* 2. Visor de Agenda Horaria del Espacio (Confirmados, Pendientes, Cancelados y Libres) */}
      {activeSpace && (
        <div className="space-y-4 border-t border-border pt-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-sm font-semibold text-foreground flex items-center gap-2">
                <Calendar className="h-4 w-4 text-primary" />
                Agenda de Horarios · {activeSpace.spaceTitle}
              </h3>
              <p className="text-xs text-muted-foreground">
                Revisa para cada día qué horarios están confirmados con pago
                aprobado, en retención temporal, cancelados o libres para nuevos
                arriendos.
              </p>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <label
                htmlFor="host-agenda-date"
                className="text-xs font-medium text-muted-foreground"
              >
                Fecha:
              </label>
              <input
                id="host-agenda-date"
                type="date"
                value={effectiveDate}
                onChange={(e) => setSelectedDate(e.target.value)}
                className="rounded-lg border border-input bg-background px-2.5 py-1 text-xs font-medium text-foreground shadow-xs focus:border-primary focus:outline-none"
              />
              {activeSpace.rateUnit === "hora" && (
                <select
                  aria-label="Duración de bloque libre a consultar"
                  value={durationHours}
                  onChange={(e) => setDurationHours(Number(e.target.value))}
                  className="rounded-lg border border-input bg-background px-2.5 py-1 text-xs font-medium text-foreground shadow-xs focus:border-primary focus:outline-none"
                >
                  <option value={1}>Bloques libres de 1 hora</option>
                  <option value={2}>Bloques libres de 2 horas</option>
                  <option value={4}>Bloques libres de 4 horas</option>
                </select>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setRefreshTick((t) => t + 1)}
                disabled={freeSlotsState.loading}
                className="h-7 px-2.5 text-xs"
              >
                <RefreshCw
                  className={`mr-1 h-3 w-3 ${freeSlotsState.loading ? "animate-spin" : ""}`}
                />
                Actualizar horarios
              </Button>
            </div>
          </div>

          {/*Accesos rápidos a fechas con reservas en este espacio */}
          {datesWithActivity.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] font-medium text-muted-foreground mr-1">
                Fechas con actividad:
              </span>
              {datesWithActivity.map(([dateStr, counts]) => {
                const isCurrent = dateStr === effectiveDate;
                return (
                  <button
                    key={dateStr}
                    type="button"
                    onClick={() => setSelectedDate(dateStr)}
                    className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors ${
                      isCurrent
                        ? "border-primary bg-primary text-primary-foreground"
                        : counts.confirmed > 0
                          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-800 dark:text-emerald-300 hover:bg-emerald-500/20"
                          : counts.pending > 0
                            ? "border-amber-500/30 bg-amber-500/10 text-amber-800 dark:text-amber-300 hover:bg-amber-500/20"
                            : "border-border bg-muted/60 text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    <span>{dateStr}</span>
                    {counts.confirmed > 0 && (
                      <span className="rounded-full bg-emerald-600 px-1.5 text-[10px] font-bold text-white">
                        {counts.confirmed} pagada
                        {counts.confirmed > 1 ? "s" : ""}
                      </span>
                    )}
                    {counts.pending > 0 && (
                      <span className="rounded-full bg-amber-500 px-1.5 text-[10px] font-bold text-white">
                        {counts.pending} pend.
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          )}

          {/* Cuadrícula de 4 estados horarios para la fecha seleccionada */}
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Columna A: Horarios Reservados (Confirmados y Pendientes) */}
            <div className="space-y-3 rounded-xl border border-border bg-background p-4">
              <h4 className="text-xs font-semibold uppercase tracking-wider text-foreground flex items-center justify-between">
                <span>Horarios Reservados y en Proceso ({effectiveDate})</span>
                <span className="font-normal text-muted-foreground">
                  {confirmedOnDate.length} confirmados · {pendingOnDate.length}{" "}
                  pendientes
                </span>
              </h4>

              {confirmedOnDate.length === 0 && pendingOnDate.length === 0 ? (
                <p className="rounded-lg border border-dashed border-border p-4 text-center text-xs text-muted-foreground">
                  No hay horarios confirmados ni pagos pendientes para esta
                  fecha.
                </p>
              ) : (
                <div className="space-y-2.5">
                  {confirmedOnDate.map((res) => (
                    <div
                      key={res.id}
                      className="flex flex-col gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="inline-flex items-center gap-1 font-semibold text-emerald-800 dark:text-emerald-300">
                            <CheckCircle2 className="h-3.5 w-3.5" />
                            {formatTimeRangeInZone(
                              res.start_at,
                              res.end_at,
                              res.time_zone,
                            )}
                          </span>
                          <span className="rounded-full bg-emerald-600 px-2 py-0.5 text-[10px] font-semibold text-white">
                            Confirmado · Pagado
                          </span>
                          {res.provider_payment_id && (
                            <span className="inline-flex items-center gap-1 font-mono text-[11px] text-foreground">
                              <Receipt className="h-3 w-3 text-emerald-700" />
                              MP #{res.provider_payment_id}
                            </span>
                          )}
                        </div>
                        <p className="text-[11px] text-muted-foreground">
                          Reserva ID:{" "}
                          <span className="font-mono">
                            {res.id.slice(0, 8)}...
                          </span>{" "}
                          · Cobrado:{" "}
                          <strong>
                            {formatBookingClp(res.subtotal_clp)} CLP
                          </strong>{" "}
                          · Neto:{" "}
                          <strong className="text-emerald-700 dark:text-emerald-400">
                            {res.is_commercial &&
                            res.provider_settlement?.net_clp === undefined
                              ? "Pendiente de confirmar"
                              : `${formatBookingClp(getReservationFinancialBreakdown(res).netHostClp)} CLP`}
                          </strong>
                        </p>
                      </div>
                      <Button
                        asChild
                        size="sm"
                        variant="outline"
                        className="h-7 text-xs shrink-0"
                      >
                        <Link
                          href={`${detailBaseHref}/${encodeURIComponent(res.id)}`}
                        >
                          Ver detalle
                        </Link>
                      </Button>
                    </div>
                  ))}

                  {pendingOnDate.map((res) => (
                    <div
                      key={res.id}
                      className="flex flex-col gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="inline-flex items-center gap-1 font-semibold text-amber-800 dark:text-amber-300">
                            <Clock3 className="h-3.5 w-3.5" />
                            {formatTimeRangeInZone(
                              res.start_at,
                              res.end_at,
                              res.time_zone,
                            )}
                          </span>
                          <span className="rounded-full bg-amber-600 px-2 py-0.5 text-[10px] font-semibold text-white">
                            Pendiente de Pago (Retención)
                          </span>
                        </div>
                        <p className="text-[11px] text-muted-foreground">
                          Reserva ID:{" "}
                          <span className="font-mono">
                            {res.id.slice(0, 8)}...
                          </span>{" "}
                          · Monto:{" "}
                          <strong>
                            {formatBookingClp(res.subtotal_clp)} CLP
                          </strong>{" "}
                          · Vence:{" "}
                          {new Date(res.pay_expires_at).toLocaleTimeString(
                            "es-CL",
                            {
                              hour: "2-digit",
                              minute: "2-digit",
                            },
                          )}
                        </p>
                      </div>
                      <Button
                        asChild
                        size="sm"
                        variant="outline"
                        className="h-7 text-xs shrink-0"
                      >
                        <Link
                          href={`${detailBaseHref}/${encodeURIComponent(res.id)}`}
                        >
                          Ver estado
                        </Link>
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              {/* Historial de intentos cancelados / vencidos en esta fecha */}
              {cancelledOnDate.length > 0 && (
                <details className="pt-2 text-xs">
                  <summary className="cursor-pointer font-medium text-muted-foreground hover:text-foreground flex items-center gap-1.5">
                    <XCircle className="h-3.5 w-3.5 text-destructive" />
                    Ver {cancelledOnDate.length} intento
                    {cancelledOnDate.length > 1 ? "s" : ""} vencido
                    {cancelledOnDate.length > 1 ? "s" : ""} o cancelado
                    {cancelledOnDate.length > 1 ? "s" : ""} en esta fecha (cupo
                    liberado)
                  </summary>
                  <div className="mt-2 space-y-1.5 pl-2">
                    {cancelledOnDate.map((res) => (
                      <div
                        key={res.id}
                        className="flex items-center justify-between rounded-md border border-border/70 bg-muted/40 px-2.5 py-1.5 text-[11px] text-muted-foreground"
                      >
                        <span>
                          {formatTimeRangeInZone(
                            res.start_at,
                            res.end_at,
                            res.time_zone,
                          )}{" "}
                          · {res.state.replace(/_/g, " ")} ·{" "}
                          <span className="font-mono">
                            {res.id.slice(0, 8)}
                          </span>
                        </span>
                        <Link
                          href={`${detailBaseHref}/${encodeURIComponent(res.id)}`}
                          className="underline hover:text-foreground"
                        >
                          Ver
                        </Link>
                      </div>
                    ))}
                  </div>
                </details>
              )}
            </div>

            {/* Columna B: Horarios Libres Disponibles en el Espacio */}
            <div className="space-y-3 rounded-xl border border-border bg-background p-4">
              <h4 className="text-xs font-semibold uppercase tracking-wider text-foreground flex items-center justify-between">
                <span className="flex items-center gap-1.5">
                  <Clock className="h-3.5 w-3.5 text-primary" />
                  Horarios Libres Disponibles ({effectiveDate})
                </span>
                <span className="font-normal text-muted-foreground">
                  {freeSlotsState.loading
                    ? "Consultando…"
                    : `${freeSlotsState.items.length} opciones libres`}
                </span>
              </h4>

              {freeSlotsState.loading ? (
                <div className="flex items-center justify-center gap-2 py-8 text-xs text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin text-primary" />
                  <span>Calculando disponibilidad libre del espacio…</span>
                </div>
              ) : freeSlotsState.error ? (
                <p className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive">
                  {freeSlotsState.error}
                </p>
              ) : freeSlotsState.items.length === 0 ? (
                <p className="rounded-lg border border-dashed border-border p-4 text-center text-xs text-muted-foreground">
                  No quedan bloques libres para la fecha y duración
                  seleccionadas (o la fecha ya pasó).
                </p>
              ) : (
                <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-3 max-h-56 overflow-y-auto pr-1">
                  {freeSlotsState.items.map((slot) => {
                    const slotStyle =
                      slot.status === "booked"
                        ? "border-neutral-300 bg-neutral-200/80 text-neutral-600 line-through"
                        : slot.status === "held"
                          ? "border-orange-300 bg-orange-100 text-orange-900"
                          : "border-emerald-200 bg-emerald-50/80 text-emerald-950";
                    return (
                      <span
                        key={`${slot.start_at}-${slot.end_at}`}
                        className={`inline-flex items-center justify-center rounded-lg border px-2.5 py-1.5 text-xs font-semibold tabular-nums ${slotStyle}`}
                      >
                        {formatTimeRangeInZone(
                          slot.start_at,
                          slot.end_at,
                          activeSpace.timeZone,
                        )}
                      </span>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
