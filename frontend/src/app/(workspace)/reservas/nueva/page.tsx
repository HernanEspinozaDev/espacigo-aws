import Link from "next/link";
import { BookingQuoteCard } from "@/features/bookings/ui/booking-quote-card";
import { serializeRentalReturnSelection } from "@/features/public-catalog/domain/availability-return";
import { MarketplaceRentalCheckout } from "@/features/public-catalog/ui/marketplace-rental-checkout";
import {
  marketplaceBookingHref,
  parseMarketplaceBookingSelection,
} from "@/features/public-catalog/domain/marketplace-booking-selection";
import { CheckoutStepper } from "@/features/rental-checkout/ui/checkout-flow-chrome";

export const dynamic = "force-dynamic";

interface NuevaReservaPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function NuevaReservaPage({
  searchParams,
}: NuevaReservaPageProps) {
  const checkoutApiEnabled =
    process.env.NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED === "1" &&
    Boolean(process.env.NEXT_PUBLIC_MP_PUBLIC_KEY?.startsWith("TEST-"));
  const walletPurchaseEnabled =
    process.env.NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED === "1";
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
        className="mx-auto min-w-0 max-w-4xl space-y-6 break-words py-2"
        aria-labelledby="booking-handoff-title"
      >
        <CheckoutStepper activeStep={2} />
        <header className="space-y-1">
          <h2
            id="booking-handoff-title"
            className="text-2xl font-semibold tracking-tight text-foreground"
          >
            Cotizar arriendo y continuar al pago
          </h2>
          <p className="text-sm text-muted-foreground">
            Revisa tu selección y confirma la cotización oficial antes de pagar.
          </p>
        </header>
        <MarketplaceRentalCheckout
          selection={{
            spaceId: selection.spaceId,
            startAt: selection.startAt,
            endAt: selection.endAt,
            timeZone: selection.timeZone,
            duration: selection.duration,
            rateUnit: selection.rateUnit,
          }}
          signInReturn={canonicalHref}
          checkoutApiEnabled={checkoutApiEnabled}
          walletPurchaseEnabled={walletPurchaseEnabled}
        />
        <nav
          className="flex flex-wrap gap-4 border-t border-border/60 pt-4 text-sm"
          aria-label="Opciones para esta selección"
        >
          <Link
            href={`/espacios/${selection.spaceId}?${rentalQuery}`}
            className="inline-flex min-h-10 items-center text-muted-foreground underline underline-offset-4 hover:text-foreground"
          >
            Volver a la publicación
          </Link>
          <Link
            href={`/acceso?return_to=${encodeURIComponent(canonicalHref)}`}
            className="inline-flex min-h-10 items-center text-muted-foreground underline underline-offset-4 hover:text-foreground"
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
