import Link from "next/link";
import {
  marketplaceBookingHref,
  marketplaceSubtotal,
  type MarketplaceBookingSelection,
} from "../domain/marketplace-booking-selection";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";

type Props = {
  selection: MarketplaceBookingSelection | null;
  basePriceCLP: number | string | null;
  rateUnit: "hora" | "dia" | "mes";
  onChooseDates?: () => void;
  mobile?: boolean;
};

const disclaimer =
  "Estimación referencial: no ocupa ni confirma una reserva o pago. El siguiente paso prepara tu selección; no habilita una confirmación comercial ni un ensayo de pago.";

export function PublicSpaceBookingCta({
  selection,
  basePriceCLP,
  rateUnit,
  onChooseDates,
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
      className={`min-w-0 break-words [overflow-wrap:anywhere] rounded-xl border border-border bg-card p-4 text-card-foreground ${mobile ? "space-y-2" : "space-y-4"}`}
    >
      {mobile && (
        <p className="text-sm font-semibold">
          {unitPrice
            ? `${unitPrice} CLP / ${rateUnitLabel(rateUnit)}`
            : "Precio por unidad no disponible"}
        </p>
      )}
      {selection && durationLabel && (
        <p className="min-w-0 break-words text-sm [overflow-wrap:anywhere]">
          {durationLabel} · {durationText}
        </p>
      )}
      {formattedSubtotal && (
        <p className="text-lg font-semibold">
          Subtotal estimado: {formattedSubtotal} CLP
        </p>
      )}
      {activeHref ? (
        <Link
          href={activeHref}
          prefetch={false}
          className="flex min-h-11 min-w-0 w-full break-words [overflow-wrap:anywhere] items-center justify-center rounded-lg bg-primary px-4 py-2 text-center font-semibold text-primary-foreground"
        >
          Solicitar Arriendo — {formattedSubtotal} CLP
        </Link>
      ) : (
        <button
          type="button"
          onClick={onChooseDates}
          className="min-h-11 min-w-0 w-full break-words [overflow-wrap:anywhere] rounded-lg bg-primary px-4 py-2 font-semibold text-primary-foreground"
        >
          Selecciona fechas en el calendario
        </button>
      )}
      <p className="text-xs leading-relaxed text-muted-foreground">
        {disclaimer}
      </p>
    </section>
  );
}
