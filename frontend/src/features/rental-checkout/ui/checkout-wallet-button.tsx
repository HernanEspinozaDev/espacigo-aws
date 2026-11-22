"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { AlertCircle, Loader2, WalletCards } from "lucide-react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { authFacade } from "@/features/auth/auth-runtime";
import { ApiError } from "@/lib/api/errors";
import { rentalCheckout } from "../infrastructure/checkout-runtime";

function walletCheckoutError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "host_mp_not_connected":
        return "El anfitrión todavía no conectó Mercado Pago.";
      case "checkout_not_configured":
        return "El pago con Mercado Pago no está habilitado en este entorno.";
      case "quote_expired":
        return "La cotización venció. Vuelve a consultar el precio y la disponibilidad.";
      case "availability_changed":
        return "El horario ya no está disponible. Selecciona nuevas fechas.";
      case "idempotency_conflict":
        return "Ya existe una operación para esta cotización. Consulta su estado antes de continuar.";
      case "unauthenticated":
        return "Tu sesión venció. Inicia sesión nuevamente antes de continuar.";
      case "forbidden":
        return "Esta cotización no pertenece a la cuenta actual.";
    }
  }
  return "No se pudo preparar el ingreso a Mercado Pago. Consulta el estado del pago antes de volver a intentar.";
}

export function CheckoutWalletButton({ quoteId }: { quoteId: string }) {
  const { snapshot: auth } = useAuth();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const key = useRef<string | null>(null);

  async function startCheckout() {
    if (starting || auth.status !== "authenticated") return;
    setError(null);
    key.current ??= crypto.randomUUID();
    setStarting(true);
    try {
      const result = await authFacade.runAuthenticatedOperation(
        (token, signal) =>
          rentalCheckout.startWallet(quoteId, key.current!, token, signal),
      );
      window.location.assign(result.initPoint);
    } catch (cause) {
      setError(walletCheckoutError(cause));
    } finally {
      setStarting(false);
    }
  }

  return (
    <div className="space-y-3">
      {error ? (
        <div className="space-y-2 sm:col-span-2">
          <p
            role="alert"
            className="flex items-start gap-2 rounded-xl border border-destructive/20 bg-destructive/10 p-3.5 text-sm text-destructive"
          >
            <AlertCircle
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            <span>{error}</span>
          </p>
          <Link
            href={`/reservas/checkout/${encodeURIComponent(quoteId)}`}
            className="inline-flex min-h-11 items-center text-sm font-medium underline underline-offset-4"
          >
            Consultar estado del pago
          </Link>
        </div>
      ) : null}
      {auth.status === "checking" ? (
        <p className="text-sm text-muted-foreground">Comprobando tu sesión…</p>
      ) : null}
      <button
        type="button"
        data-mp-checkout-cta="wallet-purchase"
        className="flex min-h-32 w-full flex-col items-start justify-between gap-4 rounded-xl border border-[var(--brand-border)] bg-white p-5 text-left transition-colors hover:bg-[var(--brand-surface)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-wait disabled:opacity-70"
        onClick={startCheckout}
        disabled={starting || auth.status !== "authenticated"}
      >
        <span className="inline-flex size-10 items-center justify-center rounded-full bg-[var(--brand-surface)] text-[var(--brand-primary)]">
          {starting ? (
            <Loader2 className="size-5 animate-spin" aria-hidden="true" />
          ) : (
            <WalletCards className="size-5" aria-hidden="true" />
          )}
        </span>
        <span className="space-y-1">
          <span className="block text-base font-semibold">
            {starting
              ? "Abriendo Mercado Pago…"
              : "Iniciar sesión con Mercado Pago"}
          </span>
          <span className="block text-sm text-muted-foreground">
            Inicia sesión en Mercado Pago y paga con los medios guardados en tu
            cuenta.
          </span>
        </span>
      </button>
    </div>
  );
}
