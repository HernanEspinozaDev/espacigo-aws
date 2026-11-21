import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { isPublicUuid } from "@/features/public-catalog/domain/media";
import { CheckoutApiCardForm } from "@/features/rental-checkout/ui/checkout-api-card-form";
import { CheckoutStepper } from "@/features/rental-checkout/ui/checkout-flow-chrome";

export const dynamic = "force-dynamic";
export const metadata: Metadata = {
  title: "Pagar con tarjeta | Mis Reservas | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function CheckoutApiPaymentPage({
  params,
}: {
  params: Promise<{ quoteID: string }>;
}) {
  const { quoteID } = await params;
  if (!isPublicUuid(quoteID)) notFound();

  const testFeatureEnabled =
    process.env.NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED === "1";
  const publicKey = testFeatureEnabled
    ? (process.env.NEXT_PUBLIC_MP_PUBLIC_KEY ?? "")
    : "";

  return (
    <div className="mx-auto w-full min-w-0 max-w-4xl space-y-6 break-words py-2">
      <CheckoutStepper activeStep={3} />
      <CheckoutApiCardForm
        quoteId={quoteID}
        enabled={testFeatureEnabled}
        publicKey={publicKey}
      />
      <nav
        aria-label="Navegación de reserva"
        className="flex flex-wrap items-center gap-4 border-t border-border/60 pt-4 text-sm"
      >
        <Link
          href="/explorar"
          className="inline-flex min-h-11 items-center font-medium text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          Volver al marketplace
        </Link>
        <Link
          href="/reservas"
          className="inline-flex min-h-11 items-center font-medium text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          Mis reservas
        </Link>
      </nav>
    </div>
  );
}
