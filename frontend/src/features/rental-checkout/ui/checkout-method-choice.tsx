import Link from "next/link";
import { ChevronDown, CreditCard, WalletCards } from "lucide-react";
import { CheckoutWalletButton } from "./checkout-wallet-button";

export function CheckoutMethodChoice({
  quoteId,
  cardPaymentEnabled,
  walletPurchaseEnabled,
}: {
  quoteId: string;
  cardPaymentEnabled: boolean;
  walletPurchaseEnabled: boolean;
}) {
  return (
    <section
      aria-labelledby="checkout-methods-title"
      className="space-y-5 rounded-2xl border border-[var(--brand-border)] bg-card p-5 text-[var(--brand-ink)] shadow-sm sm:p-6"
    >
      <header className="space-y-2">
        <p className="text-xs font-semibold uppercase tracking-wider text-[var(--brand-primary)]">
          Pago seguro
        </p>
        <h1
          id="checkout-methods-title"
          className="text-2xl font-semibold tracking-tight"
        >
          Elige cómo pagar
        </h1>
        <p className="text-sm text-muted-foreground">
          EspaciGo confirma el resultado de tu pago al regresar.
        </p>
      </header>

      <details className="group">
        <summary className="flex min-h-24 w-full cursor-pointer list-none items-center justify-between gap-4 rounded-xl border border-[var(--brand-border)] bg-white p-5 text-left transition-colors hover:bg-[var(--brand-surface)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] [&::-webkit-details-marker]:hidden">
          <span className="flex items-center gap-4">
            <span className="inline-flex size-11 items-center justify-center rounded-full bg-[var(--brand-surface)] text-[var(--brand-primary)]">
              <WalletCards className="size-5" aria-hidden="true" />
            </span>
            <span className="space-y-1">
              <span className="block text-base font-semibold">
                Pagar con Mercado Pago
              </span>
              <span className="block text-sm text-muted-foreground">
                Paga con tarjeta o ingresa a tu cuenta.
              </span>
            </span>
          </span>
          <ChevronDown
            className="size-5 shrink-0 transition-transform group-open:rotate-180"
            aria-hidden="true"
          />
        </summary>

        <div
          id="checkout-payment-options"
          className="mt-4 grid gap-4 sm:grid-cols-2"
        >
          {walletPurchaseEnabled ? (
            <CheckoutWalletButton quoteId={quoteId} />
          ) : (
            <div className="flex min-h-32 flex-col items-start justify-between gap-4 rounded-xl border border-[var(--brand-border)] bg-muted/40 p-5 text-left text-muted-foreground">
              <span className="inline-flex size-10 items-center justify-center rounded-full bg-white">
                <WalletCards className="size-5" aria-hidden="true" />
              </span>
              <span className="space-y-1">
                <span className="block text-base font-semibold text-foreground">
                  Iniciar sesión con Mercado Pago
                </span>
                <span className="block text-sm">
                  Esta opción aún no está habilitada en este entorno.
                </span>
              </span>
            </div>
          )}
          {cardPaymentEnabled ? (
            <Link
              href={`/reservas/checkout/tarjeta/${encodeURIComponent(quoteId)}`}
              data-mp-checkout-cta="checkout-api-card"
              className="flex min-h-32 flex-col items-start justify-between gap-4 rounded-xl border border-[var(--brand-border)] bg-white p-5 text-left transition-colors hover:bg-[var(--brand-surface)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]"
            >
              <span className="inline-flex size-10 items-center justify-center rounded-full bg-[var(--brand-surface)] text-[var(--brand-primary)]">
                <CreditCard className="size-5" aria-hidden="true" />
              </span>
              <span className="space-y-1">
                <span className="block text-base font-semibold">
                  Pagar con tarjeta
                </span>
                <span className="block text-sm text-muted-foreground">
                  Ingresa los datos de tu tarjeta de forma segura en EspaciGo.
                </span>
              </span>
            </Link>
          ) : (
            <div className="flex min-h-32 flex-col items-start justify-between gap-4 rounded-xl border border-[var(--brand-border)] bg-muted/40 p-5 text-left text-muted-foreground">
              <span className="inline-flex size-10 items-center justify-center rounded-full bg-white">
                <CreditCard className="size-5" aria-hidden="true" />
              </span>
              <span className="space-y-1">
                <span className="block text-base font-semibold text-foreground">
                  Pagar con tarjeta
                </span>
                <span className="block text-sm">
                  Esta opción aún no está habilitada en este entorno.
                </span>
              </span>
            </div>
          )}
        </div>
        <p className="flex items-start gap-2 border-t border-border/60 pt-4 text-xs leading-relaxed text-muted-foreground">
          <WalletCards className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          Al ingresar a Mercado Pago, podrás iniciar sesión con la cuenta que
          usas para comprar.
        </p>
      </details>
    </section>
  );
}
