import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { isPublicUuid } from "@/features/public-catalog/domain/media";
import { CheckoutMethodChoice } from "@/features/rental-checkout/ui/checkout-method-choice";
import { CheckoutStepper } from "@/features/rental-checkout/ui/checkout-flow-chrome";

export const dynamic = "force-dynamic";
export const metadata: Metadata = {
  title: "Elegir cómo pagar | Mis Reservas | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function CheckoutMethodsPage({
  params,
}: {
  params: Promise<{ quoteID: string }>;
}) {
  const { quoteID } = await params;
  if (!isPublicUuid(quoteID)) notFound();
  const cardPaymentEnabled =
    process.env.NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED === "1" &&
    Boolean(process.env.NEXT_PUBLIC_MP_PUBLIC_KEY?.startsWith("TEST-"));
  const walletPurchaseEnabled =
    process.env.NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED === "1";
  if (!cardPaymentEnabled && !walletPurchaseEnabled) notFound();

  return (
    <div className="mx-auto w-full min-w-0 max-w-4xl space-y-6 break-words py-2">
      <CheckoutStepper activeStep={3} />
      <CheckoutMethodChoice
        quoteId={quoteID}
        cardPaymentEnabled={cardPaymentEnabled}
        walletPurchaseEnabled={walletPurchaseEnabled}
      />
      <nav
        aria-label="Navegación de pago"
        className="flex flex-wrap items-center gap-4 border-t border-border/60 pt-4 text-sm"
      >
        <Link
          href={`/reservas/checkout/${encodeURIComponent(quoteID)}`}
          className="inline-flex min-h-11 items-center font-medium text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          Volver a la cotización
        </Link>
        <Link
          href="/explorar"
          className="inline-flex min-h-11 items-center font-medium text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          Volver al marketplace
        </Link>
      </nav>
    </div>
  );
}
