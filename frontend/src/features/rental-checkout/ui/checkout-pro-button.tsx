"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { AlertCircle, ExternalLink, Loader2 } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { authFacade } from "@/features/auth/auth-runtime";
import { ApiError } from "@/lib/api/errors";
import { rentalCheckout } from "../infrastructure/checkout-runtime";

function checkoutError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "host_mp_not_connected":
        return "El anfitrión todavía no conectó Mercado Pago. No se inició ningún pago.";
      case "checkout_not_configured":
        return "El pago con Mercado Pago no está habilitado en este entorno.";
      case "payment_method_unavailable":
        return "Mercado Pago no mostró un medio de pago compatible para esta compra. No se inició ningún pago.";
      case "quote_expired":
        return "La cotización venció. Vuelve a consultar el precio y la disponibilidad.";
      case "availability_changed":
        return "El horario ya no está disponible. Selecciona nuevas fechas.";
      case "idempotency_conflict":
        return "Ya existe una operación de pago distinta para esta cotización. Consulta su estado antes de continuar.";
      case "unauthenticated":
        return "Tu sesión venció. Inicia sesión nuevamente antes de continuar.";
      case "forbidden":
        return "Esta cotización no pertenece a la cuenta actual.";
    }
  }
  return "No se pudo iniciar el checkout. Consulta su estado antes de reintentar.";
}

export function CheckoutProButton({
  quoteId,
  expiresAt,
}: {
  quoteId: string;
  expiresAt: string;
}) {
  const { snapshot: auth } = useAuth();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const key = useRef<string | null>(null);
  const statusHref = `/reservas/checkout/${encodeURIComponent(quoteId)}`;
  async function startCheckout() {
    if (starting || auth.status !== "authenticated") return;
    setError(null);
    if (Date.parse(expiresAt) <= Date.now()) {
      setError("La cotización venció. Solicita una cotización actualizada.");
      return;
    }
    key.current ??= crypto.randomUUID();
    setStarting(true);
    try {
      const result = await authFacade.runAuthenticatedOperation(
        (token, signal) =>
          rentalCheckout.start(quoteId, key.current!, token, signal),
      );
      window.location.assign(result.initPoint);
    } catch (cause) {
      setError(checkoutError(cause));
    } finally {
      setStarting(false);
    }
  }

  return (
    <div className="space-y-3">
      {error ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>{error}</span>
        </div>
      ) : null}
      {auth.status === "checking" ? (
        <p className="text-sm text-muted-foreground">Comprobando tu sesión…</p>
      ) : null}
      <Button
        type="button"
        className="w-full"
        onClick={startCheckout}
        disabled={starting || auth.status !== "authenticated"}
      >
        {starting ? (
          <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" />
        ) : (
          <ExternalLink className="mr-2 size-4" aria-hidden="true" />
        )}
        {starting ? "Preparando checkout…" : "Pagar con Mercado Pago"}
      </Button>
      {error ? (
        <Link
          className="inline-flex min-h-11 items-center text-sm font-medium underline underline-offset-4"
          href={statusHref}
        >
          Consultar estado del pago
        </Link>
      ) : null}
    </div>
  );
}
