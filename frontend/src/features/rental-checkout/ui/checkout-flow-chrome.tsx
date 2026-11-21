import {
  CalendarCheck2,
  CalendarDays,
  Check,
  Clock3,
  CreditCard,
  FileCheck2,
  Globe,
} from "lucide-react";
import {
  formatPublicClp,
  rateUnitLabel,
} from "@/features/public-catalog/domain/presentation";
import { cn } from "@/lib/utils";

export type CheckoutStepNumber = 1 | 2 | 3;

const STEPS = [
  { step: 1 as const, label: "1. Horario", icon: CalendarCheck2 },
  { step: 2 as const, label: "2. Cotización", icon: FileCheck2 },
  { step: 3 as const, label: "3. Pago", icon: CreditCard },
] as const;

export function CheckoutStepper({
  activeStep,
  className,
}: {
  activeStep: CheckoutStepNumber;
  className?: string;
}) {
  return (
    <nav
      aria-label="Progreso del arriendo"
      className={cn(
        "rounded-2xl border border-[#ded5cc] bg-[#fbf7f2] px-4 py-3.5 shadow-xs sm:px-5",
        className,
      )}
    >
      <ol className="flex items-center justify-between gap-2 sm:gap-4">
        {STEPS.map((item, index) => {
          const isCompleted = item.step < activeStep;
          const isActive = item.step === activeStep;
          const Icon = item.icon;

          return (
            <li
              key={item.step}
              aria-current={isActive ? "step" : undefined}
              className={cn(
                "flex min-w-0 items-center gap-2.5",
                index < STEPS.length - 1 ? "flex-1" : "shrink-0",
              )}
            >
              <div className="flex min-w-0 items-center gap-2">
                <span
                  aria-hidden="true"
                  className={cn(
                    "flex size-8 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-colors",
                    isCompleted &&
                      "border-[#a44731] bg-[#a44731] text-white shadow-xs",
                    isActive &&
                      "border-[#e8c5b5] bg-[#f5e3db] text-[#702a1b] ring-2 ring-[#a44731]/25",
                    !isCompleted &&
                      !isActive &&
                      "border-[#ded5cc] bg-white text-[#66574e]",
                  )}
                >
                  {isCompleted ? (
                    <Check className="size-4" />
                  ) : (
                    <Icon className="size-4" />
                  )}
                </span>
                <span
                  className={cn(
                    "truncate text-xs sm:text-sm",
                    isActive
                      ? "font-semibold text-[#702a1b]"
                      : isCompleted
                        ? "font-semibold text-[#332720]"
                        : "font-medium text-[#66574e]",
                  )}
                >
                  {item.label}
                </span>
              </div>
              {index < STEPS.length - 1 ? (
                <div
                  aria-hidden="true"
                  className={cn(
                    "hidden h-0.5 flex-1 rounded-full sm:block",
                    isCompleted ? "bg-[#a44731]" : "bg-[#ded5cc]",
                  )}
                />
              ) : null}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

export interface CheckoutSummaryCardProps {
  spaceTitle?: string | null;
  startAt?: string | null;
  endAt?: string | null;
  timeZone?: string | null;
  duration?: number | string | null;
  rateUnit?: "hora" | "dia" | "mes";
  unitPriceClp?: number | string | null;
  baseClp?: number | string | null;
  commissionClp?: number | string | null;
  vatClp?: number | string | null;
  serviceFeeClp?: number | string | null;
  totalClp?: number | string | null;
  expiresAt?: string | null;
  quoteId?: string | null;
  statusLabel?: string | null;
  contextNote?: string | null;
  className?: string;
}

function normalizeIntlWhitespace(value: string): string {
  return value.replace(/[\u00a0\u2009\u202f]/g, " ");
}

function formatRange(startAt: string, endAt: string, timeZone: string): string {
  try {
    return normalizeIntlWhitespace(
      new Intl.DateTimeFormat("es-CL", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone,
      }).formatRange(new Date(startAt), new Date(endAt)),
    );
  } catch {
    return `${startAt} – ${endAt}`;
  }
}

function formatDuration(
  duration: number | string | null | undefined,
  rateUnit: "hora" | "dia" | "mes" | undefined,
): string | null {
  if (duration === null || duration === undefined || !rateUnit) return null;
  const numeric = Number(duration);
  if (!Number.isFinite(numeric) || numeric <= 0) return null;
  const unitLabel = rateUnitLabel(rateUnit);
  const plural = numeric === 1 ? "" : rateUnit === "mes" ? "es" : "s";
  return `${numeric} ${unitLabel}${plural}`;
}

function formatExpiration(
  expiresAt: string,
  timeZone = "America/Santiago",
): string {
  try {
    return normalizeIntlWhitespace(
      new Intl.DateTimeFormat("es-CL", {
        dateStyle: "short",
        timeStyle: "short",
        timeZone,
      }).format(new Date(expiresAt)),
    );
  } catch {
    return expiresAt;
  }
}

export function CheckoutSummaryCard({
  spaceTitle,
  startAt,
  endAt,
  timeZone,
  duration,
  rateUnit,
  unitPriceClp,
  baseClp,
  commissionClp,
  vatClp,
  serviceFeeClp,
  totalClp,
  expiresAt,
  quoteId,
  statusLabel,
  contextNote,
  className,
}: CheckoutSummaryCardProps) {
  const formattedRange =
    startAt && endAt
      ? formatRange(startAt, endAt, timeZone || "America/Santiago")
      : null;
  const formattedDuration = formatDuration(duration, rateUnit);
  const formattedUnit = formatPublicClp(unitPriceClp);
  const formattedBase = formatPublicClp(baseClp);
  const formattedCommission = formatPublicClp(commissionClp);
  const formattedVat = formatPublicClp(vatClp);
  const formattedFee = formatPublicClp(serviceFeeClp);
  const formattedTotal = formatPublicClp(totalClp);
  const hasBreakdown = Boolean(
    formattedBase ||
      formattedCommission ||
      formattedVat ||
      formattedFee ||
      formattedTotal,
  );
  const hasDetails = Boolean(
    formattedRange || timeZone || quoteId || statusLabel,
  );

  return (
    <aside
      aria-label="Resumen de arriendo"
      className={cn(
        "min-w-0 space-y-4 rounded-2xl border border-[#ded5cc] bg-white p-5 text-[#332720] shadow-sm",
        className,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <h3 className="text-base font-semibold tracking-tight text-[#332720]">
          Resumen del arriendo
        </h3>
        {expiresAt ? (
          <span className="inline-flex items-center gap-1.5 rounded-full border border-[#e8c5b5] bg-[#f5e3db] px-2.5 py-1 text-xs font-semibold text-[#702a1b]">
            <Clock3 className="size-3.5 text-[#a44731]" aria-hidden="true" />
            Vence {formatExpiration(expiresAt, timeZone || "America/Santiago")}
          </span>
        ) : null}
      </div>

      {spaceTitle ? (
        <p className="min-w-0 break-words text-sm font-semibold text-[#332720] [overflow-wrap:anywhere]">
          {spaceTitle}
        </p>
      ) : null}

      {hasDetails ? (
        <dl className="space-y-3 rounded-xl border border-[#ded5cc] bg-[#fbf7f2] p-3.5 text-sm">
          {formattedRange ? (
            <div className="space-y-1">
              <dt className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-[#66574e]">
                <CalendarDays
                  className="size-3.5 shrink-0 text-[#a44731]"
                  aria-hidden="true"
                />
                Horario y duración
              </dt>
              <dd
                suppressHydrationWarning
                className="min-w-0 break-words font-medium text-[#332720] [overflow-wrap:anywhere]"
              >
                {formattedRange}
                {formattedDuration ? (
                  <span className="text-[#66574e]"> · {formattedDuration}</span>
                ) : null}
              </dd>
            </div>
          ) : null}

          {timeZone ? (
            <div
              className={cn(
                "space-y-1",
                formattedRange && "border-t border-[#ded5cc] pt-2.5",
              )}
            >
              <dt className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-[#66574e]">
                <Globe
                  className="size-3.5 shrink-0 text-[#a44731]"
                  aria-hidden="true"
                />
                Zona horaria
              </dt>
              <dd className="break-all text-xs font-medium text-[#66574e]">
                {timeZone}
              </dd>
            </div>
          ) : null}

          {quoteId ? (
            <div
              className={cn(
                "space-y-1",
                (formattedRange || timeZone) &&
                  "border-t border-[#ded5cc] pt-2.5",
              )}
            >
              <dt className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-[#66574e]">
                <FileCheck2
                  className="size-3.5 shrink-0 text-[#a44731]"
                  aria-hidden="true"
                />
                Cotización
              </dt>
              <dd className="break-all font-mono text-xs text-[#332720]">
                {quoteId}
              </dd>
            </div>
          ) : null}

          {statusLabel ? (
            <div
              className={cn(
                "flex items-center justify-between gap-2",
                (formattedRange || timeZone || quoteId) &&
                  "border-t border-[#ded5cc] pt-2.5",
              )}
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-[#66574e]">
                Estado
              </dt>
              <dd>
                <span className="inline-flex items-center rounded-full border border-[#e8c5b5] bg-[#f5e3db] px-2.5 py-0.5 text-xs font-semibold text-[#702a1b]">
                  {statusLabel}
                </span>
              </dd>
            </div>
          ) : null}
        </dl>
      ) : null}

      {hasBreakdown ? (
        <div className="space-y-2 border-t border-[#ded5cc] pt-3.5 text-sm">
          {formattedBase ? (
            <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
              <span>
                Precio original del espacio
                {formattedUnit && formattedDuration
                  ? ` (${formattedUnit} × ${formattedDuration})`
                  : ""}
              </span>
              <span className="font-medium tabular-nums text-[#332720]">
                {formattedBase} CLP
              </span>
            </div>
          ) : null}

          {formattedCommission ? (
            <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
              <span>Comisión de servicio (3%)</span>
              <span className="font-medium tabular-nums text-[#332720]">
                {formattedCommission} CLP
              </span>
            </div>
          ) : null}

          {formattedVat ? (
            <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
              <span>IVA sobre comisión (19%)</span>
              <span className="font-medium tabular-nums text-[#332720]">
                {formattedVat} CLP
              </span>
            </div>
          ) : null}

          {formattedFee ? (
            <div className="flex items-baseline justify-between gap-3 border-t border-dashed border-[#ded5cc] pt-2 text-[#66574e]">
              <span>Comisión total con IVA (3,57%)</span>
              <span className="font-medium tabular-nums text-[#332720]">
                {formattedFee} CLP
              </span>
            </div>
          ) : null}

          {formattedTotal ? (
            <div className="flex items-baseline justify-between gap-3 rounded-xl border border-[#e8c5b5] bg-[#fbf7f2] px-3.5 py-2.5">
              <span className="font-semibold text-[#332720]">
                Total a pagar
              </span>
              <span className="text-lg font-bold tabular-nums text-[#702a1b]">
                {formattedTotal} CLP
              </span>
            </div>
          ) : null}
        </div>
      ) : null}

      {contextNote ? (
        <p className="border-t border-[#ded5cc] pt-3 text-xs leading-relaxed text-[#66574e]">
          {contextNote}
        </p>
      ) : null}
    </aside>
  );
}
