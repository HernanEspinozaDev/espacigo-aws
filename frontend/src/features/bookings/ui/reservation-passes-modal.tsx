"use client";

import { useEffect, useMemo, useState } from "react";
import QRCode from "qrcode";
import {
  Calendar,
  Check,
  Copy,
  Info,
  QrCode,
  Ticket,
  X,
} from "lucide-react";
import type { BookingReservation } from "../domain/types";
import {
  getReservationIntervals,
  formatTimeRange24h,
  usageStatusLabel,
} from "../domain/reservation-intervals";
import { Button } from "@/components/workspace/ui/button";

interface ReservationPassesModalProps {
  reservation: BookingReservation | null;
  onClose: () => void;
}

function PassQrImage({ payload }: { payload: string }) {
  const [dataUrl, setDataUrl] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    QRCode.toDataURL(payload, {
      width: 220,
      margin: 1,
      color: { dark: "#18181b", light: "#ffffff" },
    })
      .then((url) => {
        if (active) setDataUrl(url);
      })
      .catch(() => {
        if (active) setDataUrl(null);
      });
    return () => {
      active = false;
    };
  }, [payload]);

  if (!dataUrl) {
    return (
      <div className="flex h-36 w-36 items-center justify-center rounded-xl border border-border bg-muted/30 text-xs text-muted-foreground">
        Generando QR...
      </div>
    );
  }

  return (
    <img
      src={dataUrl}
      alt="Código QR de acceso"
      width={144}
      height={144}
      className="h-36 w-36 rounded-xl border border-border bg-white p-1.5 shadow-xs"
    />
  );
}

export function ReservationPassesModal({
  reservation,
  onClose,
}: ReservationPassesModalProps) {
  const [filter, setFilter] = useState<"active" | "all" | "used">("active");
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const passes = useMemo(() => {
    if (!reservation) return [];
    const tz = reservation.time_zone || "America/Santiago";
    return getReservationIntervals(reservation).map((iv, idx, arr) => {
      const status = iv.usage_status ?? "pendiente";
      let dateLabel = iv.start_at.slice(0, 10);
      try {
        dateLabel = new Intl.DateTimeFormat("es-CL", {
          weekday: "long",
          day: "2-digit",
          month: "short",
          year: "numeric",
          timeZone: tz,
        }).format(new Date(iv.start_at));
      } catch {
        // Keep ISO fallback
      }
      return {
        index: idx + 1,
        total: arr.length,
        id: iv.id ?? `${iv.start_at}-${idx}`,
        dateLabel,
        timeLabel: formatTimeRange24h(iv.start_at, iv.end_at, tz),
        units: iv.units,
        status,
        passCode: iv.pass_code ?? "",
        qrPayload: iv.qr_payload ?? "",
      };
    });
  }, [reservation]);

  if (!reservation) return null;

  const filteredPasses = passes.filter((p) => {
    if (filter === "active")
      return p.status === "pendiente" || p.status === "en_uso";
    if (filter === "used")
      return p.status === "completado" || p.status === "no_asistio";
    return true;
  });

  const handleCopy = async (code: string, id: string) => {
    try {
      await navigator.clipboard.writeText(code);
      setCopiedId(id);
      setTimeout(() => setCopiedId(null), 1800);
    } catch {
      // Ignore clipboard failure
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="espacigo-passes-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-3 backdrop-blur-xs sm:p-4"
    >
      <div className="relative max-h-[92vh] w-full max-w-2xl overflow-y-auto rounded-2xl border border-border bg-card text-foreground shadow-2xl">
        <div className="h-1.5 w-full bg-gradient-to-r from-[#EE8953] via-[#D46236] to-[#B94628]" />
        <div className="p-5 sm:p-6 space-y-5">
          {/* Header */}
          <div className="flex items-start justify-between gap-4 border-b border-border pb-4">
            <div className="flex items-center gap-3">
              <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#D46236]/10 text-[#D46236]">
                <QrCode className="h-6 w-6" />
              </div>
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[#D46236]">
                  Pases de Ingreso · Recepción
                </p>
                <h2 id="espacigo-passes-title" className="text-lg font-bold">
                  {reservation.space_title || "Reserva EspaciGo"}
                </h2>
              </div>
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label="Cerrar pases de ingreso"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          {/* User guide */}
          <div className="flex items-start gap-2.5 rounded-xl border border-emerald-500/25 bg-emerald-500/10 p-3.5 text-xs text-emerald-950 dark:text-emerald-200">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-emerald-700 dark:text-emerald-400" />
            <div className="space-y-1">
              <p className="font-semibold">
                Cómo usar tus pases al llegar al espacio:
              </p>
              <p className="leading-relaxed">
                1. Cada bloque continuo tiene su propio ticket (si reservaste
                horas separadas o días distintos, tendrás un pase por cada
                sesión). 2. Muestra el <strong>código QR</strong> o dicta el{" "}
                <strong>código corto</strong> al anfitrión para que confirme tu
                llegada.
              </p>
            </div>
          </div>

          {/* Filter pills */}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="inline-flex rounded-lg bg-muted p-1 text-xs">
              <button
                type="button"
                onClick={() => setFilter("active")}
                className={`rounded-md px-3 py-1 font-medium transition-colors ${
                  filter === "active"
                    ? "bg-background text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Por usar / En uso
              </button>
              <button
                type="button"
                onClick={() => setFilter("used")}
                className={`rounded-md px-3 py-1 font-medium transition-colors ${
                  filter === "used"
                    ? "bg-background text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Usados
              </button>
              <button
                type="button"
                onClick={() => setFilter("all")}
                className={`rounded-md px-3 py-1 font-medium transition-colors ${
                  filter === "all"
                    ? "bg-background text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Todos ({passes.length})
              </button>
            </div>
          </div>

          {/* Passes list */}
          {filteredPasses.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border p-8 text-center text-xs text-muted-foreground">
              No hay pases en esta categoría.
            </div>
          ) : (
            <div className="space-y-3.5">
              {filteredPasses.map((pass) => {
                const statusBadge =
                  pass.status === "en_uso"
                    ? "border-blue-300 bg-blue-50 text-blue-900 dark:border-blue-500/40 dark:bg-blue-500/15 dark:text-blue-300"
                    : pass.status === "completado"
                      ? "border-neutral-300 bg-neutral-200/80 text-neutral-700 dark:border-neutral-600 dark:bg-neutral-800 dark:text-neutral-300"
                      : "border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-500/40 dark:bg-emerald-500/15 dark:text-emerald-300";

                return (
                  <div
                    key={pass.id}
                    className="flex flex-col items-center justify-between gap-4 rounded-xl border border-border bg-background p-4 sm:flex-row"
                  >
                    <div className="w-full flex-1 space-y-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="inline-flex items-center gap-1 text-xs font-bold uppercase tracking-wider text-[#D46236]">
                          <Ticket className="h-3.5 w-3.5" />
                          Pase {pass.index} de {pass.total}
                        </span>
                        <span
                          className={`rounded-full border px-2.5 py-0.5 text-[11px] font-semibold ${statusBadge}`}
                        >
                          {usageStatusLabel(pass.status)}
                        </span>
                      </div>

                      <p className="flex items-center gap-1.5 text-sm font-bold capitalize text-foreground">
                        <Calendar className="h-4 w-4 text-muted-foreground" />
                        {pass.dateLabel}
                      </p>
                      <p className="font-mono text-sm font-semibold text-foreground">
                        {pass.timeLabel} · {pass.units}{" "}
                        {reservation.rate_unit === "hora"
                          ? pass.units === 1
                            ? "hora"
                            : "horas"
                          : reservation.rate_unit === "dia"
                            ? pass.units === 1
                              ? "día"
                              : "días"
                            : pass.units === 1
                              ? "mes"
                              : "meses"}
                      </p>

                      {pass.passCode && (
                        <div className="flex flex-wrap items-center gap-2 pt-1">
                          <span className="rounded-lg border border-border bg-muted/40 px-3 py-1.5 font-mono text-sm font-extrabold tracking-widest text-foreground">
                            {pass.passCode}
                          </span>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={() => handleCopy(pass.passCode, pass.id)}
                            className="h-8 gap-1 text-xs"
                          >
                            {copiedId === pass.id ? (
                              <>
                                <Check className="h-3.5 w-3.5 text-emerald-600" />
                                Copiado
                              </>
                            ) : (
                              <>
                                <Copy className="h-3.5 w-3.5" />
                                Copiar código
                              </>
                            )}
                          </Button>
                        </div>
                      )}
                    </div>

                    {pass.qrPayload && (
                      <div className="shrink-0">
                        <PassQrImage payload={pass.qrPayload} />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}

          <div className="flex justify-end border-t border-border pt-4">
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              Cerrar
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
