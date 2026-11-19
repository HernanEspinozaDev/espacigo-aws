import Link from "next/link";
import { BookingQuoteCard } from "@/features/bookings/ui/booking-quote-card";
import { serializeRentalReturnSelection } from "@/features/public-catalog/domain/availability-return";
import { MarketplaceRentalCheckout } from "@/features/public-catalog/ui/marketplace-rental-checkout";
import {
  marketplaceBookingHref,
  parseMarketplaceBookingSelection,
} from "@/features/public-catalog/domain/marketplace-booking-selection";

interface NuevaReservaPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function NuevaReservaPage({
  searchParams,
}: NuevaReservaPageProps) {
  const params = await searchParams;
  const sourceValues = params.source;
  const hasMarketplaceSource =
    sourceValues === "marketplace" ||
    (Array.isArray(sourceValues) && sourceValues.includes("marketplace"));
  if (sourceValues !== undefined && !hasMarketplaceSource)
    return <InvalidMarketplaceHandoff />;
  if (hasMarketplaceSource) {
    const entries = Object.entries(params).filter(
      ([, value]) => value !== undefined,
    );
    const legacyId = params.space_id;
    if (
      entries.some(([, value]) => Array.isArray(value)) ||
      (legacyId !== undefined && typeof legacyId !== "string") ||
      typeof params.spaceId !== "string" ||
      (typeof legacyId === "string" && legacyId !== params.spaceId)
    )
      return <InvalidMarketplaceHandoff />;
    const query = new URLSearchParams();
    for (const [key, value] of entries)
      if (key !== "space_id" && typeof value === "string")
        query.set(key, value);
    const selection = parseMarketplaceBookingSelection(query.toString());
    const canonicalHref = selection ? marketplaceBookingHref(selection) : null;
    if (!selection || !canonicalHref) return <InvalidMarketplaceHandoff />;
    const rentalQuery = serializeRentalReturnSelection({
      date: selection.date,
      duration: selection.duration,
      startAt: selection.startAt,
      endAt: selection.endAt,
    });
    return (
      <section
        className="mx-auto min-w-0 max-w-2xl break-words py-4"
        aria-labelledby="booking-handoff-title"
      >
        <h1 id="booking-handoff-title" className="text-2xl font-semibold">
          Cotizar arriendo y continuar al pago
        </h1>
        <dl className="mt-6 grid min-w-0 gap-3 break-words">
          <div>
            <dt className="font-medium">ID público</dt>
            <dd className="break-all">{selection.spaceId}</dd>
          </div>
          <div>
            <dt className="font-medium">Rango seleccionado</dt>
            <dd>
              {selection.startAt} – {selection.endAt}
            </dd>
          </div>
          <div>
            <dt className="font-medium">Zona horaria</dt>
            <dd>{selection.timeZone}</dd>
          </div>
          <div>
            <dt className="font-medium">Modalidad</dt>
            <dd>{selection.rateUnit}</dd>
          </div>
          <div>
            <dt className="font-medium">Duración</dt>
            <dd>
              {selection.duration}{" "}
              {selection.rateUnit === "hora"
                ? "hora(s)"
                : selection.rateUnit === "dia"
                  ? "día(s)"
                  : "mes"}
            </dd>
          </div>
        </dl>
        <div className="mt-8 border-t border-border pt-6">
          <MarketplaceRentalCheckout
            selection={{
              spaceId: selection.spaceId,
              startAt: selection.startAt,
              endAt: selection.endAt,
              timeZone: selection.timeZone,
            }}
            signInReturn={canonicalHref}
          />
        </div>
        <nav
          className="mt-6 flex flex-wrap gap-4 text-sm"
          aria-label="Opciones para esta selección"
        >
          <Link
            href={`/espacios/${selection.spaceId}?${rentalQuery}`}
            className="underline text-muted-foreground hover:text-foreground"
          >
            Volver a la publicación
          </Link>
          <Link
            href={`/acceso?return_to=${encodeURIComponent(canonicalHref)}`}
            className="underline text-muted-foreground hover:text-foreground"
          >
            Iniciar sesión
          </Link>
        </nav>
      </section>
    );
  }
  // Legacy local trial flow remains unchanged when source is absent.
  const spaceId = typeof params.space_id === "string" ? params.space_id : "";
  const spaceTitle =
    typeof params.title === "string" ? params.title : undefined;
  return (
    <div className="mx-auto max-w-2xl py-4">
      <BookingQuoteCard spaceId={spaceId} spaceTitle={spaceTitle} />
    </div>
  );
}

function InvalidMarketplaceHandoff() {
  return (
    <section className="mx-auto min-w-0 max-w-2xl break-words py-4">
      <h1 className="text-2xl font-semibold">
        No se pudo validar la selección
      </h1>
      <p className="mt-3">
        Vuelve a la publicación y selecciona nuevamente el horario.
      </p>
    </section>
  );
}
