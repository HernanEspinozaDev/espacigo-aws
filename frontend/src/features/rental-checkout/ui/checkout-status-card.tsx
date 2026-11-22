"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Clock3,
  ExternalLink,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { authFacade } from "@/features/auth/auth-runtime";
import { bookingsFacade } from "@/features/bookings/bookings-runtime";
import { ApiError } from "@/lib/api/errors";
import type { CheckoutRead } from "../domain/checkout";
import { rentalCheckout } from "../infrastructure/checkout-runtime";
import { ReservationCountdown } from "./reservation-countdown";

function loadError(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) {
    return "Inicia sesión para consultar el estado de esta cotización.";
  }
  if (error instanceof ApiError && error.status === 403) {
    return "Esta cotización no pertenece a la cuenta actual.";
  }
  return "No fue posible verificar el estado del pago. Vuelve a consultar.";
}

function statusCopy(status: CheckoutRead["status"]): {
  title: string;
  detail: string;
  tone: "success" | "pending" | "error" | "neutral";
} {
  switch (status) {
    case "approved":
      return {
        title: "Pago confirmado",
        detail:
          "EspaciGo verificó el resultado con Mercado Pago y guardó tu reserva.",
        tone: "success",
      };
    case "pending":
      return {
        title: "Pago pendiente de confirmación",
        detail:
          "EspaciGo todavía no confirma el resultado. Puedes consultar nuevamente.",
        tone: "pending",
      };
    case "reconciliation_required":
      return {
        title: "Pago en revisión",
        detail:
          "EspaciGo está verificando este resultado. No inicies otro pago para esta cotización.",
        tone: "pending",
      };
    case "rejected":
      return {
        title: "El pago no fue aprobado",
        detail: "No se confirmó un pago para esta cotización.",
        tone: "error",
      };
    case "expired":
      return {
        title: "El checkout venció",
        detail:
          "La cotización ya no admite este pago. Solicita una nueva cotización.",
        tone: "error",
      };
    case "cancelled":
      return {
        title: "Checkout cancelado",
        detail: "No se confirmó un pago para esta cotización.",
        tone: "neutral",
      };
    case "not_started":
      return {
        title: "No hay un pago iniciado",
        detail: "EspaciGo no registra un checkout para esta cotización.",
        tone: "neutral",
      };
  }
}

export function CheckoutStatusCard({ quoteId }: { quoteId: string }) {
  const { snapshot: auth } = useAuth();
  const [resultState, setResultState] = useState<{
    key: string;
    loading: boolean;
    checkout: CheckoutRead | null;
    error: string | null;
  } | null>(null);
  const [version, setVersion] = useState(0);

  const refresh = useCallback(() => setVersion((previous) => previous + 1), []);

  const authKey = JSON.stringify([
    auth.status,
    auth.status === "authenticated" ? auth.accountId : null,
    auth.status === "authenticated" ? auth.expiresAt : null,
  ]);
  const requestKey = `${quoteId}:${version}:${authKey}`;
  const currentResult = resultState?.key === requestKey ? resultState : null;
  const checkout = currentResult?.checkout ?? null;
  const loading =
    auth.status === "authenticated" && (currentResult?.loading ?? true);
  const error = currentResult?.error ?? null;

  useEffect(() => {
    if (auth.status !== "authenticated") return;
    const controller = new AbortController();
    let active = true;
    void Promise.resolve()
      .then(() => {
        if (active)
          setResultState({
            key: requestKey,
            loading: true,
            checkout: null,
            error: null,
          });
        return authFacade.runAuthenticatedOperation((token, signal) =>
          rentalCheckout.read(
            quoteId,
            token,
            AbortSignal.any([signal, controller.signal]),
          ),
        );
      })
      .then((result) => {
        if (active) {
          setResultState({
            key: requestKey,
            loading: false,
            checkout: result,
            error: null,
          });
          if (result.status === "approved") {
            void bookingsFacade.loadReservations().catch(() => {});
          }
        }
      })
      .catch((cause: unknown) => {
        if (active && !controller.signal.aborted)
          setResultState({
            key: requestKey,
            loading: false,
            checkout: null,
            error: loadError(cause),
          });
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [
    quoteId,
    version,
    requestKey,
    auth.status,
    auth.accountId,
    auth.expiresAt,
    auth.roles,
    auth.capabilities,
    auth.restrictedMode,
  ]);

  const copy = checkout ? statusCopy(checkout.status) : null;
  const statusRole = copy?.tone === "error" ? "alert" : "status";

  return (
    <section
      aria-labelledby="checkout-status-title"
      className="space-y-4 rounded-xl border bg-card p-5"
    >
      <header className="space-y-1">
        <p className="text-sm font-medium text-primary">
          Checkout de Mercado Pago
        </p>
        <h2 id="checkout-status-title" className="text-xl font-semibold">
          Estado del pago
        </h2>
        <p className="break-all text-xs text-muted-foreground">
          Cotización {quoteId}
        </p>
      </header>

      {auth.status === "checking" ? (
        <p role="status" className="text-sm text-muted-foreground">
          Comprobando tu sesión…
        </p>
      ) : auth.status !== "authenticated" ? (
        <div className="space-y-2">
          <p role="status" className="text-sm text-muted-foreground">
            Inicia sesión para consultar el estado confirmado por EspaciGo.
          </p>
          <Link
            href={`/acceso?return_to=${encodeURIComponent(`/reservas/checkout/${quoteId}`)}`}
            className="inline-flex min-h-11 items-center font-medium underline underline-offset-4"
          >
            Iniciar sesión
          </Link>
        </div>
      ) : loading ? (
        <p
          role="status"
          className="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Consultando a EspaciGo…
        </p>
      ) : error ? (
        <div className="space-y-3">
          <p
            role="alert"
            className="flex items-start gap-2 text-sm text-destructive"
          >
            <AlertCircle
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {error}
          </p>
          <Button type="button" variant="outline" onClick={refresh}>
            <RefreshCw className="mr-2 size-4" aria-hidden="true" />
            Consultar estado
          </Button>
        </div>
      ) : checkout && copy ? (
        <div className="space-y-4">
          <div
            role={statusRole}
            className={`flex items-start gap-3 rounded-lg border p-4 text-sm ${
              copy.tone === "success"
                ? "border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200"
                : copy.tone === "error"
                  ? "border-destructive/20 bg-destructive/10 text-destructive"
                  : "border-border bg-muted/30 text-foreground"
            }`}
          >
            {copy.tone === "success" ? (
              <CheckCircle2
                className="mt-0.5 size-5 shrink-0"
                aria-hidden="true"
              />
            ) : copy.tone === "pending" ? (
              <Clock3 className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
            ) : (
              <AlertCircle
                className="mt-0.5 size-5 shrink-0"
                aria-hidden="true"
              />
            )}
            <div>
              <p className="font-semibold">{copy.title}</p>
              <p className="mt-1">{copy.detail}</p>
              {checkout.operationId ? (
                <p className="mt-1.5 font-mono text-xs opacity-90">
                  Reserva guardada: {checkout.operationId}
                </p>
              ) : null}
              {checkout.status === "pending" && checkout.expiresAt ? (
                <ReservationCountdown
                  expiresAt={checkout.expiresAt}
                  onExpire={() => setVersion((v) => v + 1)}
                  className="mt-2"
                />
              ) : null}
            </div>
          </div>

          {checkout.status === "approved" ? (
            <div className="flex flex-wrap items-center gap-3">
              {checkout.operationId ? (
                <Button asChild>
                  <Link
                    href={`/reservas/${encodeURIComponent(checkout.operationId)}`}
                  >
                    Ver detalle de la reserva
                  </Link>
                </Button>
              ) : null}
              <Button asChild variant="outline">
                <Link href="/reservas">Ir a Mis reservas</Link>
              </Button>
            </div>
          ) : null}

          {checkout.status === "pending" && checkout.initPoint ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => window.location.assign(checkout.initPoint!)}
            >
              <ExternalLink className="mr-2 size-4" aria-hidden="true" />
              Volver a Mercado Pago
            </Button>
          ) : null}

          {checkout.status !== "approved" ? (
            <div className="flex flex-wrap items-center gap-3">
              <Button type="button" variant="outline" onClick={refresh}>
                <RefreshCw className="mr-2 size-4" aria-hidden="true" />
                Actualizar estado
              </Button>
              {checkout.operationId ? (
                <Button asChild variant="outline">
                  <Link
                    href={`/reservas/${encodeURIComponent(checkout.operationId)}`}
                  >
                    Ver reserva
                  </Link>
                </Button>
              ) : null}
              <Button asChild variant="ghost">
                <Link href="/reservas">Mis reservas</Link>
              </Button>
            </div>
          ) : null}
          {checkout.updatedAt ? (
            <p className="text-xs text-muted-foreground">
              Última verificación:{" "}
              {new Date(checkout.updatedAt).toLocaleString("es-CL")}
            </p>
          ) : null}
        </div>
      ) : (
        <p role="status" className="text-sm text-muted-foreground">
          No hay un estado disponible para esta cotización.
        </p>
      )}
    </section>
  );
}
