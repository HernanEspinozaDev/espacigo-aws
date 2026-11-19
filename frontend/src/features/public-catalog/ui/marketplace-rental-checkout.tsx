"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  CalendarDays,
  Clock3,
  Loader2,
  ShieldCheck,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import type { RentalQuote } from "../domain/rental-quote";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import { publicRentalQuote } from "../infrastructure/rental-quote-runtime";
import { CheckoutProButton } from "@/features/rental-checkout/ui/checkout-pro-button";

export interface MarketplaceCheckoutSelection {
  spaceId: string;
  startAt: string;
  endAt: string;
  timeZone: string;
}

function quoteError(status: number): string {
  if (status === 401)
    return "Inicia sesión para solicitar la cotización oficial.";
  if (status === 403)
    return "Esta cuenta no puede solicitar una cotización para este espacio.";
  if (status === 404)
    return "El espacio ya no está disponible. Vuelve a la publicación.";
  if (status === 409)
    return "El horario cambió o ya no está disponible. Selecciona otra fecha.";
  if (status === 422)
    return "El intervalo seleccionado no es válido. Vuelve al calendario.";
  if (status === 503)
    return "El servicio de cotizaciones no está disponible por ahora.";
  return "No fue posible calcular una cotización. Intenta nuevamente.";
}

function rentalInterval(quote: RentalQuote): string {
  try {
    return new Intl.DateTimeFormat("es-CL", {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: quote.time_zone,
    }).formatRange(new Date(quote.start_at), new Date(quote.end_at));
  } catch {
    return `${quote.start_at} – ${quote.end_at}`;
  }
}

export function MarketplaceRentalCheckout({
  selection,
  signInReturn,
}: {
  selection: MarketplaceCheckoutSelection;
  signInReturn: string;
}) {
  const { snapshot: auth } = useAuth();
  const [quoteState, setQuoteState] = useState<{
    authKey: string;
    quote: RentalQuote | null;
  } | null>(null);
  const [operationState, setOperationState] = useState<{
    authKey: string;
    loading: boolean;
    error: string | null;
  } | null>(null);
  const requestGeneration = useRef(0);
  const requestController = useRef<AbortController | null>(null);

  const currentAuthKey =
    auth.status === "authenticated"
      ? `${auth.accountId}:${auth.expiresAt}`
      : auth.status;
  useEffect(() => {
    requestGeneration.current += 1;
    requestController.current?.abort();
    requestController.current = null;
    return () => {
      requestGeneration.current += 1;
      requestController.current?.abort();
    };
  }, [currentAuthKey]);

  const quote =
    quoteState?.authKey === currentAuthKey ? quoteState.quote : null;
  const operation =
    operationState?.authKey === currentAuthKey
      ? operationState
      : { authKey: currentAuthKey, loading: false, error: null };
  const { loading, error } = operation;

  async function requestQuote() {
    if (operation.loading || auth.status !== "authenticated") return;
    const generation = ++requestGeneration.current;
    const controller = new AbortController();
    requestController.current?.abort();
    requestController.current = controller;
    setOperationState({ authKey: currentAuthKey, loading: true, error: null });
    setQuoteState({ authKey: currentAuthKey, quote: null });
    try {
      const result = await publicRentalQuote.createQuote(
        {
          space_id: selection.spaceId,
          start_at: selection.startAt,
          end_at: selection.endAt,
        },
        controller.signal,
      );
      if (generation !== requestGeneration.current) return;
      if (result.ok)
        setQuoteState({ authKey: currentAuthKey, quote: result.value });
      else
        setOperationState({
          authKey: currentAuthKey,
          loading: false,
          error: quoteError(result.error.status),
        });
    } catch {
      if (
        generation === requestGeneration.current &&
        !controller.signal.aborted
      ) {
        setOperationState({
          authKey: currentAuthKey,
          loading: false,
          error: "No fue posible calcular una cotización. Intenta nuevamente.",
        });
      }
    } finally {
      if (generation === requestGeneration.current) {
        requestController.current = null;
        setOperationState((previous) =>
          previous?.authKey === currentAuthKey
            ? { ...previous, loading: false }
            : previous,
        );
      }
    }
  }

  const formattedTotal = quote ? formatPublicClp(quote.subtotal_clp) : null;

  return (
    <section
      aria-labelledby="marketplace-checkout-title"
      className="space-y-5 rounded-2xl border border-border bg-card p-5 text-card-foreground shadow-sm sm:p-7"
    >
      <header className="space-y-2">
        <p className="text-sm font-medium text-primary">
          Solicitud desde el marketplace
        </p>
        <h2
          id="marketplace-checkout-title"
          className="text-xl font-semibold tracking-tight"
        >
          Cotización oficial y pago
        </h2>
        <p className="text-sm text-muted-foreground">
          EspaciGo confirma precio y disponibilidad desde el servidor antes de
          abrir Mercado Pago.
        </p>
      </header>

      <dl className="grid gap-3 rounded-lg bg-muted/40 p-4 text-sm sm:grid-cols-2">
        <div>
          <dt className="flex items-center gap-2 font-medium">
            <CalendarDays className="size-4 text-primary" aria-hidden="true" />
            Horario solicitado
          </dt>
          <dd className="mt-1 text-muted-foreground">
            {new Intl.DateTimeFormat("es-CL", {
              dateStyle: "medium",
              timeStyle: "short",
              timeZone: selection.timeZone,
            }).formatRange(
              new Date(selection.startAt),
              new Date(selection.endAt),
            )}
          </dd>
        </div>
        <div>
          <dt className="flex items-center gap-2 font-medium">
            <Clock3 className="size-4 text-primary" aria-hidden="true" />
            Zona horaria
          </dt>
          <dd className="mt-1 break-all text-muted-foreground">
            {selection.timeZone}
          </dd>
        </div>
      </dl>

      {error ? (
        <p
          role="alert"
          className="flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          {error}
        </p>
      ) : null}

      {!quote ? (
        <div className="space-y-3">
          {auth.status === "checking" ? (
            <p role="status" className="text-sm text-muted-foreground">
              Comprobando tu sesión…
            </p>
          ) : auth.status !== "authenticated" ? (
            <p role="status" className="text-sm text-muted-foreground">
              Inicia sesión para calcular la cotización oficial de tu arriendo.
            </p>
          ) : null}
          <Button
            type="button"
            variant="secondary"
            className="w-full"
            onClick={() => void requestQuote()}
            disabled={loading || auth.status !== "authenticated"}
          >
            {loading ? (
              <Loader2
                className="mr-2 size-4 animate-spin"
                aria-hidden="true"
              />
            ) : null}
            {loading
              ? "Validando precio y disponibilidad…"
              : "Calcular cotización oficial"}
          </Button>
          {auth.status !== "authenticated" && auth.status !== "checking" ? (
            <Link
              href={`/acceso?return_to=${encodeURIComponent(signInReturn)}`}
              className="inline-flex min-h-11 items-center font-medium underline underline-offset-4"
            >
              Iniciar sesión
            </Link>
          ) : null}
        </div>
      ) : (
        <div className="space-y-5 rounded-xl border border-primary/20 bg-primary/5 p-4 sm:p-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="flex items-center gap-2 font-semibold">
              <ShieldCheck className="size-4 text-primary" aria-hidden="true" />
              Cotización oficial
            </h3>
            <p className="text-xs text-muted-foreground">
              Vence {new Date(quote.expires_at).toLocaleString("es-CL")}
            </p>
          </div>
          <div className="space-y-2 text-sm">
            <p className="font-medium">{quote.space_title}</p>
            <p className="text-muted-foreground">
              {quote.units} {rateUnitLabel(quote.rate_unit)}
              {Number(quote.units) === 1 ? "" : "s"} · {rentalInterval(quote)}
            </p>
            <div className="flex items-baseline justify-between gap-3 border-t border-border/70 pt-3">
              <span>Total del arriendo</span>
              <span className="text-lg font-semibold">
                {formattedTotal
                  ? `${formattedTotal} CLP`
                  : "Monto no disponible"}
              </span>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            El pago continúa en Mercado Pago. El estado final se confirma con
            EspaciGo al regresar.
          </p>
          <CheckoutProButton quoteId={quote.id} expiresAt={quote.expires_at} />
          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={() => {
              setQuoteState({ authKey: currentAuthKey, quote: null });
              setOperationState({
                authKey: currentAuthKey,
                loading: false,
                error: null,
              });
            }}
          >
            Solicitar otra cotización
          </Button>
        </div>
      )}
    </section>
  );
}
