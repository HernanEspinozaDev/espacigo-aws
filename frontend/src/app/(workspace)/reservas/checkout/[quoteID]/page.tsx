import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { isPublicUuid } from "@/features/public-catalog/domain/media";
import { CheckoutStatusCard } from "@/features/rental-checkout/ui/checkout-status-card";

export const metadata: Metadata = {
  title: "Estado del pago | Mis Reservas | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function RentalCheckoutReturnPage({
  params,
}: {
  params: Promise<{ quoteID: string }>;
}) {
  const { quoteID } = await params;
  if (!isPublicUuid(quoteID)) notFound();

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6 py-4">
      <header className="space-y-2">
        <Link
          href="/reservas"
          className="inline-flex items-center text-sm font-medium text-primary underline-offset-4 hover:underline"
        >
          Mis reservas
        </Link>
        <h1 className="text-2xl font-semibold tracking-tight">
          Volviste de Mercado Pago
        </h1>
        <p className="text-sm text-muted-foreground">
          Consultaremos a EspaciGo para mostrar el estado confirmado de tu pago.
        </p>
      </header>
      <CheckoutStatusCard quoteId={quoteID} />
      <nav
        aria-label="Navegación de retorno de checkout"
        className="flex flex-wrap items-center gap-4 border-t border-border/60 pt-4 text-sm"
      >
        <Link
          href="/reservas"
          className="inline-flex min-h-11 items-center font-medium underline underline-offset-4"
        >
          Ir a Mis reservas
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
