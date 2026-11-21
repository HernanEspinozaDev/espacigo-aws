import Link from "next/link";
import {
  marketplaceBookingHref,
  marketplaceSubtotal,
  type MarketplaceBookingSelection,
} from "../domain/marketplace-booking-selection";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";

export type PublicSpaceBookingCtaProps = {
  selection: MarketplaceBookingSelection | null;
  basePriceCLP: number | string | null;
  rateUnit: "hora" | "dia" | "mes";
  onChooseDates?: () => void;
  mobile?: boolean;
};

type Props = PublicSpaceBookingCtaProps;

const disclaimer =
  "El siguiente paso calcula una cotización oficial con el precio y la disponibilidad vigentes. El pago se confirma al volver desde Mercado Pago.";

export function PublicSpaceBookingCta({
  selection,
  basePriceCLP,
  rateUnit,
  mobile = false,
}: Props) {
  const href = selection ? marketplaceBookingHref(selection) : null;
  const subtotal =
    selection && href
      ? marketplaceSubtotal(basePriceCLP, selection.duration)
      : null;
  const formattedSubtotal =
    subtotal === null ? null : formatPublicClp(subtotal);
  const activeHref = formattedSubtotal === null ? null : href;
  const unitPrice = formatPublicClp(basePriceCLP);
  const durationText = selection
    ? `${selection.duration} ${rateUnitLabel(selection.rateUnit)}${selection.duration === 1 ? "" : selection.rateUnit === "mes" ? "es" : "s"}`
    : null;
  const durationLabel = selection
    ? new Intl.DateTimeFormat("es-CL", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: selection.timeZone,
      }).formatRange(new Date(selection.startAt), new Date(selection.endAt))
    : null;

  return (
    <section
      aria-label={
        mobile ? "Solicitud de arriendo móvil" : "Solicitud de arriendo"
      }
      className={
        mobile
          ? "min-w-0 space-y-2 break-words [overflow-wrap:anywhere] rounded-xl border border-[var(--brand-border)] bg-card px-3 py-2.5 text-card-foreground shadow-xs"
          : "min-w-0 space-y-4 break-words [overflow-wrap:anywhere] rounded-2xl border border-[var(--brand-border)] bg-card p-5 text-card-foreground shadow-sm"
      }
    >
      {mobile ? (
        <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
          <p className="text-xs font-medium text-muted-foreground">
            {unitPrice
              ? `${unitPrice} CLP / ${rateUnitLabel(rateUnit)}`
              : "Precio por unidad no disponible"}
          </p>
          {formattedSubtotal ? (
            <p className="text-sm font-semibold tabular-nums text-foreground">
              Subtotal: {formattedSubtotal} CLP
            </p>
          ) : null}
        </div>
      ) : (
        <div className="flex items-baseline justify-between gap-2 border-b border-[var(--brand-border)]/70 pb-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Reserva en línea
            </p>
            <p className="mt-0.5 text-base font-semibold tabular-nums text-foreground">
              {unitPrice
                ? `${unitPrice} CLP`
                : "Precio por unidad no disponible"}
              {unitPrice ? (
                <span className="text-sm font-normal text-muted-foreground">
                  {" "}
                  / {rateUnitLabel(rateUnit)}
                </span>
              ) : null}
            </p>
          </div>
          {durationText ? (
            <span className="rounded-full border border-[var(--brand-border)] bg-[var(--brand-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-foreground">
              {durationText}
            </span>
          ) : null}
        </div>
      )}

      {selection && durationLabel ? (
        <p
          className={
            mobile
              ? "min-w-0 truncate text-xs font-medium text-foreground"
              : "min-w-0 break-words text-sm font-medium leading-snug text-foreground [overflow-wrap:anywhere]"
          }
        >
          {durationLabel}
          {mobile && durationText ? ` · ${durationText}` : ""}
        </p>
      ) : null}

      {!mobile && selection && formattedSubtotal && unitPrice ? (
        <div className="space-y-2.5 rounded-xl border border-[var(--brand-border)] bg-[var(--brand-surface-subtle)]/60 p-3.5 text-sm">
          <div className="flex items-baseline justify-between gap-2 text-xs text-muted-foreground">
            <span>
              {unitPrice} CLP × {durationText}
            </span>
            <span className="font-medium tabular-nums text-foreground">
              {formattedSubtotal} CLP
            </span>
          </div>
          <div className="flex items-baseline justify-between gap-2 border-t border-[var(--brand-border)]/70 pt-2">
            <span className="text-sm font-medium text-foreground">
              Subtotal estimado
            </span>
            <span className="text-lg font-semibold tabular-nums text-foreground">
              {formattedSubtotal} CLP
            </span>
          </div>
        </div>
      ) : null}

      {activeHref ? (
        <Link
          href={activeHref}
          prefetch={false}
          className="flex min-h-11 min-w-0 w-full break-words [overflow-wrap:anywhere] items-center justify-center rounded-xl bg-primary px-4 py-2.5 text-center text-sm font-semibold text-primary-foreground shadow-xs transition-colors hover:bg-primary/90"
        >
          Solicitar arriendo · {formattedSubtotal} CLP
        </Link>
      ) : (
        <button
          type="button"
          disabled={!selection}
          className="min-h-11 min-w-0 w-full break-words [overflow-wrap:anywhere] rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground shadow-xs transition-colors disabled:cursor-not-allowed disabled:opacity-55"
        >
          Selecciona fechas en el calendario
        </button>
      )}
      <p
        className={
          mobile
            ? "text-[11px] leading-snug text-muted-foreground"
            : "text-xs leading-relaxed text-muted-foreground"
        }
      >
        {disclaimer}
      </p>
    </section>
  );
}

export { PublicSpaceBookingCta as BookingSummaryCard };
